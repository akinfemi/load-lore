package synthetic

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/singleflight"
)

// Service is one running replica of the synthetic service.
type Service struct {
	cfg       *Config
	m         *metrics
	clients   map[string]client
	gate      chan struct{} // concurrency slots; nil = unlimited
	cpuMu     sync.Mutex    // event-loop: CPU work runs one request at a time
	breakers  map[string]*breaker
	bulkheads map[string]bulkhead
	rnd       func() float64
	async     sync.WaitGroup
	flight    singleflight.Group // coalesced misses, by call and key
}

// New builds a service from its config. Connections are opened lazily, on
// the first call that needs one, as real pools do.
func New(cfg *Config) (*Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	s := &Service{
		cfg:       cfg,
		clients:   map[string]client{},
		breakers:  map[string]*breaker{},
		bulkheads: map[string]bulkhead{},
		rnd:       defaultRand,
	}
	s.m = newMetrics(cfg, s.snapshots)
	timeout := time.Duration(cfg.Resilience.TimeoutMs) * time.Millisecond
	for _, name := range sortedNames(cfg.Targets) {
		c, err := newClient(name, cfg.Targets[name], timeout, s.m)
		if err != nil {
			s.Close()
			return nil, err
		}
		s.clients[name] = c
		if b := cfg.Resilience.CircuitBreaker; b != nil {
			s.breakers[name] = newBreaker(*b, time.Now)
		}
		if b := cfg.Resilience.Bulkhead; b != nil {
			s.bulkheads[name] = make(bulkhead, b.MaxConcurrent)
		}
	}
	if cfg.Concurrency.Max > 0 {
		s.gate = make(chan struct{}, cfg.Concurrency.Max)
	}
	return s, nil
}

func sortedNames[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}

func (s *Service) snapshots() []poolSnapshot {
	out := make([]poolSnapshot, 0, len(s.clients))
	for _, name := range sortedNames(s.clients) {
		snap := s.clients[name].snapshot()
		if b, ok := s.breakers[name]; ok {
			snap.hasBreaker, snap.open = true, b.isOpen()
		}
		out = append(out, snap)
	}
	return out
}

// Close waits for async calls and closes every pool.
func (s *Service) Close() {
	s.async.Wait()
	for _, c := range s.clients {
		c.close()
	}
}

// Handler serves the request endpoint, metrics and health checks.
//
//	GET|POST /r?type=read|write&key=N   one request (the body, if any, is read and dropped)
//	GET /metrics                        Prometheus metrics
//	GET /healthz                        the process is up
//	GET /readyz                         every dependency answers a ping
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/r", s.serveRequest)
	mux.Handle("/metrics", promhttp.HandlerFor(s.m.reg, promhttp.HandlerOpts{EnableOpenMetrics: true}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok\n") })
	mux.HandleFunc("/readyz", s.serveReady)
	return mux
}

func (s *Service) serveReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	for _, name := range sortedNames(s.clients) {
		if err := s.clients[name].ping(ctx); err != nil {
			http.Error(w, fmt.Sprintf("%s: %v", name, err), http.StatusServiceUnavailable)
			return
		}
	}
	_, _ = io.WriteString(w, "ready\n")
}

func (s *Service) serveRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	q := r.URL.Query()
	req := request{typ: q.Get("type"), key: 1}
	if req.typ == "" {
		req.typ = "read"
	}
	if req.typ != "read" && req.typ != "write" {
		http.Error(w, "type must be read or write", http.StatusBadRequest)
		return
	}
	if k := q.Get("key"); k != "" {
		n, err := strconv.ParseInt(k, 10, 64)
		if err != nil || n < 1 {
			http.Error(w, "key must be a positive integer", http.StatusBadRequest)
			return
		}
		req.key = n
	}
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, r.Body)
	}

	err := s.handle(r.Context(), req)
	code := http.StatusOK
	if err != nil {
		cause := Cause(err)
		code = statusFor(cause)
		s.m.reqErrors.WithLabelValues(cause).Inc()
		w.Header().Set("X-Loadlore-Cause", cause)
		w.WriteHeader(code)
		_, _ = io.WriteString(w, cause+"\n")
	} else {
		_, _ = io.WriteString(w, "ok\n")
	}
	s.m.requests.WithLabelValues(req.typ, strconv.Itoa(code)).Inc()
	s.m.reqDuration.WithLabelValues(req.typ).Observe(time.Since(start).Seconds())
}

// statusFor maps a failure cause to the HTTP status the service returns.
func statusFor(cause string) int {
	switch cause {
	case CauseTimeout, CausePoolTimeout:
		return http.StatusGatewayTimeout
	case CauseCircuitOpen, CauseBulkheadFull:
		return http.StatusServiceUnavailable
	case CauseCanceled:
		return 499 // client closed the request
	}
	return http.StatusInternalServerError
}

// handle runs one request: wait for a concurrency slot, do the CPU and
// memory work, then the calls in order.
func (s *Service) handle(ctx context.Context, req request) error {
	if s.gate != nil {
		waitStart := time.Now()
		s.m.queued.Inc()
		select {
		case s.gate <- struct{}{}:
			s.m.queued.Dec()
			s.m.queueWait.Observe(time.Since(waitStart).Seconds())
			defer func() { <-s.gate }()
		case <-ctx.Done():
			s.m.queued.Dec()
			s.m.queueWait.Observe(time.Since(waitStart).Seconds())
			return withCause(CauseCanceled, ctx.Err())
		}
	}
	s.m.inflight.Inc()
	defer s.m.inflight.Dec()

	var buf []byte
	if s.cfg.AllocKB > 0 {
		buf = make([]byte, s.cfg.AllocKB<<10)
		for i := 0; i < len(buf); i += 4096 {
			buf[i] = 1 // touch every page so the memory is really used
		}
	}
	ctx, releaseHeld := withSession(ctx)
	defer releaseHeld()
	s.burn()
	err := s.runCalls(ctx, s.cfg.Calls, req)
	runtime.KeepAlive(buf)
	return err
}

// burn spends cpu_ms of CPU time, varied by cpu_jitter. In the event-loop
// model only one request burns CPU at a time.
func (s *Service) burn() {
	if s.cfg.CPUMs <= 0 {
		return
	}
	f := 1 + s.cfg.CPUJitter*rand.NormFloat64()
	d := time.Duration(math.Max(0, f) * s.cfg.CPUMs * float64(time.Millisecond))
	if s.cfg.Concurrency.Model == "event-loop" {
		s.cpuMu.Lock()
		defer s.cpuMu.Unlock()
	}
	burnCPU(d)
	s.m.cpuSeconds.Add(d.Seconds())
}

func applies(when, typ string) bool { return when == "always" || when == typ }

// runCalls runs calls in order. A failed sync call fails the request; async
// calls run on their own and never fail it.
func (s *Service) runCalls(ctx context.Context, calls []Call, req request) error {
	for i := range calls {
		c := &calls[i]
		if !applies(c.When, req.typ) {
			continue
		}
		for range c.Count {
			if c.Mode == "async" {
				s.async.Add(1)
				go func() {
					defer s.async.Done()
					if err := s.call(withoutSession(context.WithoutCancel(ctx)), c, req); err != nil {
						s.m.asyncDropped.Inc()
					}
				}()
				continue
			}
			if err := s.call(ctx, c, req); err != nil {
				return err
			}
		}
	}
	return nil
}

// call runs one call with retries, then its on_miss calls on a cache miss.
func (s *Service) call(ctx context.Context, c *Call, req request) error {
	maxAttempts := 1
	r := s.cfg.Resilience.Retries
	if r != nil {
		maxAttempts += r.Max
	}
	for n := 1; ; n++ {
		hit, err := s.attempt(ctx, c, req)
		if err == nil {
			if c.Op == "get" && !hit && len(c.OnMiss) > 0 {
				if c.Coalesce {
					return s.coalesced(ctx, c, req)
				}
				return s.runCalls(ctx, c.OnMiss, req)
			}
			return nil
		}
		if n >= maxAttempts || !retryable(Cause(err)) || ctx.Err() != nil {
			return err
		}
		s.m.retries.WithLabelValues(c.Target, c.Op).Inc()
		if werr := sleepCtx(ctx, backoff(r, n, s.rnd)); werr != nil {
			return err
		}
	}
}

// coalesced runs a get's on_miss calls once for all the requests that
// miss the same key at the same time: the first runs them, the others wait
// for its result, each until its own deadline. The run is detached from
// the first request, so its caller giving up doesn't fail the others; the
// calls' own timeouts still bound it.
func (s *Service) coalesced(ctx context.Context, c *Call, req request) error {
	led := false
	ch := s.flight.DoChan(fmt.Sprintf("%p:%d", c, req.key), func() (any, error) {
		led = true
		return nil, s.runCalls(withoutSession(context.WithoutCancel(ctx)), c.OnMiss, req)
	})
	select {
	case r := <-ch:
		if !led {
			s.m.coalesced.WithLabelValues(c.Target).Inc()
		}
		return r.Err
	case <-ctx.Done():
		return withCause(CauseCanceled, ctx.Err())
	}
}

// attempt makes one call attempt through the breaker, bulkhead and timeout.
func (s *Service) attempt(ctx context.Context, c *Call, req request) (hit bool, err error) {
	start := time.Now()
	defer func() {
		outcome := "ok"
		if err != nil {
			outcome = "error"
			s.m.callErrors.WithLabelValues(c.Target, c.Op, Cause(err)).Inc()
		}
		s.m.calls.WithLabelValues(c.Target, c.Op, outcome).Inc()
		s.m.callDuration.WithLabelValues(c.Target, c.Op).Observe(time.Since(start).Seconds())
		if err == nil && c.Op == "get" {
			result := "miss"
			if hit {
				result = "hit"
			}
			s.m.cacheLookups.WithLabelValues(c.Target, result).Inc()
		}
	}()

	// The bulkhead goes first: a rejection there says nothing about the
	// target's health, so it must not count against the breaker.
	if bh, ok := s.bulkheads[c.Target]; ok {
		if !bh.tryAcquire() {
			return false, errBulkheadFull
		}
		defer bh.release()
	}
	br := s.breakers[c.Target]
	if br != nil && !br.allow() {
		return false, errCircuitOpen
	}
	if t := s.cfg.Resilience.TimeoutMs; t > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(t)*time.Millisecond)
		defer cancel()
	}
	hit, err = s.clients[c.Target].do(ctx, c, req)
	if br != nil {
		if Cause(err) == CauseCanceled {
			br.abandon() // the caller left; no verdict on the target
		} else {
			br.record(err != nil)
		}
	}
	return hit, err
}
