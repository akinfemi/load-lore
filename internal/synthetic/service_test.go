package synthetic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
)

// fakeClient records calls and answers with fn.
type fakeClient struct {
	name  string
	mu    sync.Mutex
	log   []string
	fn    func(ctx context.Context, c *Call, r request) (bool, error)
	count atomic.Int64
}

func (f *fakeClient) do(ctx context.Context, c *Call, r request) (bool, error) {
	f.count.Add(1)
	f.mu.Lock()
	f.log = append(f.log, fmt.Sprintf("%s:%s:%d", c.Op, r.typ, r.key))
	f.mu.Unlock()
	if f.fn == nil {
		return false, nil
	}
	return f.fn(ctx, c, r)
}
func (f *fakeClient) snapshot() poolSnapshot     { return poolSnapshot{target: f.name, max: 2} }
func (f *fakeClient) ping(context.Context) error { return nil }
func (f *fakeClient) close()                     {}
func (f *fakeClient) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.log...)
}

func baseConfig() *Config {
	return &Config{
		Service:     "api",
		Listen:      ":0",
		Concurrency: Concurrency{Model: "threads", Max: 0},
		Resilience:  Resilience{TimeoutMs: 1000},
		Targets: map[string]Target{
			"db":    {Kind: KindPostgres, Addrs: []string{"127.0.0.1:1"}, Pool: 2, Rows: 100, RowBytes: 100, Database: "x", User: "x"},
			"cache": {Kind: KindRedis, Addrs: []string{"127.0.0.1:1"}, Pool: 2},
			"other": {Kind: KindHTTP, Addrs: []string{"127.0.0.1:1"}, Pool: 2},
		},
	}
}

func call(target, op string, mods ...func(*Call)) Call {
	c := Call{Target: target, Op: op, Count: 1, When: "always", Mode: "sync"}
	if op == "read" {
		c.Query = "point"
	}
	for _, m := range mods {
		m(&c)
	}
	return c
}

// newTestService builds a service whose targets are fakes.
func newTestService(t *testing.T, cfg *Config) (*Service, map[string]*fakeClient) {
	t.Helper()
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	fakes := map[string]*fakeClient{}
	for name, c := range s.clients {
		c.close()
		f := &fakeClient{name: name}
		fakes[name] = f
		s.clients[name] = f
	}
	t.Cleanup(s.Close)
	return s, fakes
}

func TestCallsRunInOrderWithWhenAndCount(t *testing.T) {
	cfg := baseConfig()
	cfg.Calls = []Call{
		call("db", "read", func(c *Call) { c.Count = 3 }),
		call("db", "write", func(c *Call) { c.When = "write" }),
		call("cache", "del", func(c *Call) { c.When = "read" }),
	}
	s, f := newTestService(t, cfg)

	if err := s.handle(context.Background(), request{typ: "read", key: 7}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f["db"].calls(), ","); got != "read:read:7,read:read:7,read:read:7" {
		t.Errorf("db calls on a read: %s", got)
	}
	if got := len(f["cache"].calls()); got != 1 {
		t.Errorf("cache del on a read: %d calls", got)
	}

	if err := s.handle(context.Background(), request{typ: "write", key: 9}); err != nil {
		t.Fatal(err)
	}
	if got := f["db"].calls()[3:]; strings.Join(got, ",") != "read:write:9,read:write:9,read:write:9,write:write:9" {
		t.Errorf("db calls on a write: %v", got)
	}
	if got := len(f["cache"].calls()); got != 1 {
		t.Errorf("cache del ran on a write: %d calls", got)
	}
}

func TestOnMissRunsOnlyOnMiss(t *testing.T) {
	cfg := baseConfig()
	cfg.Calls = []Call{call("cache", "get", func(c *Call) {
		c.OnMiss = []Call{call("db", "read"), call("cache", "set", func(c *Call) { c.TTLS = 60 })}
	})}
	s, f := newTestService(t, cfg)
	hit := false
	f["cache"].fn = func(_ context.Context, c *Call, _ request) (bool, error) { return c.Op == "get" && hit, nil }

	if err := s.handle(context.Background(), request{typ: "read", key: 1}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f["cache"].calls(), ","); got != "get:read:1,set:read:1" || len(f["db"].calls()) != 1 {
		t.Fatalf("on a miss: cache %s, db %v", got, f["db"].calls())
	}
	hit = true
	if err := s.handle(context.Background(), request{typ: "read", key: 1}); err != nil {
		t.Fatal(err)
	}
	if len(f["db"].calls()) != 1 || len(f["cache"].calls()) != 3 {
		t.Fatalf("on a hit: cache %v, db %v", f["cache"].calls(), f["db"].calls())
	}
}

func TestSyncFailureFailsRequestAndStops(t *testing.T) {
	cfg := baseConfig()
	cfg.Calls = []Call{call("db", "read"), call("cache", "set")}
	s, f := newTestService(t, cfg)
	f["db"].fn = func(context.Context, *Call, request) (bool, error) { return false, syscall.ECONNREFUSED }

	err := s.handle(context.Background(), request{typ: "read", key: 1})
	if Cause(err) != CauseConnRefused {
		t.Fatalf("cause %q (%v)", Cause(err), err)
	}
	if len(f["cache"].calls()) != 0 {
		t.Error("calls after a failed sync call still ran")
	}
}

func TestAsyncFailureDoesNotFailRequest(t *testing.T) {
	cfg := baseConfig()
	cfg.Calls = []Call{call("other", "http", func(c *Call) { c.Mode = "async" })}
	s, f := newTestService(t, cfg)
	release := make(chan struct{})
	f["other"].fn = func(context.Context, *Call, request) (bool, error) {
		<-release
		return false, errors.New("boom")
	}
	start := time.Now()
	if err := s.handle(context.Background(), request{typ: "read", key: 1}); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Error("request waited for an async call")
	}
	close(release)
	s.async.Wait()
	if f["other"].count.Load() != 1 {
		t.Error("async call didn't run")
	}
}

func TestRetries(t *testing.T) {
	cfg := baseConfig()
	cfg.Calls = []Call{call("db", "read")}
	cfg.Resilience.Retries = &Retries{Max: 3, Backoff: "none"}
	s, f := newTestService(t, cfg)
	var n atomic.Int64
	f["db"].fn = func(context.Context, *Call, request) (bool, error) {
		if n.Add(1) < 3 {
			return false, syscall.ECONNRESET
		}
		return false, nil
	}
	if err := s.handle(context.Background(), request{typ: "read", key: 1}); err != nil {
		t.Fatalf("succeeded on the third attempt but got %v", err)
	}
	if n.Load() != 3 {
		t.Errorf("%d attempts", n.Load())
	}

	n.Store(-100) // always fail
	err := s.handle(context.Background(), request{typ: "read", key: 1})
	if Cause(err) != CauseConnReset || n.Load() != -100+4 {
		t.Errorf("cause %q after %d attempts", Cause(err), n.Load()+100)
	}
}

func TestTimeoutCause(t *testing.T) {
	cfg := baseConfig()
	cfg.Calls = []Call{call("db", "read")}
	cfg.Resilience.TimeoutMs = 20
	s, f := newTestService(t, cfg)
	f["db"].fn = func(ctx context.Context, _ *Call, _ request) (bool, error) {
		<-ctx.Done()
		return false, ctx.Err()
	}
	start := time.Now()
	err := s.handle(context.Background(), request{typ: "read", key: 1})
	if Cause(err) != CauseTimeout {
		t.Fatalf("cause %q (%v)", Cause(err), err)
	}
	if d := time.Since(start); d < 20*time.Millisecond || d > time.Second {
		t.Errorf("timed out after %v", d)
	}
}

func TestBulkheadRejects(t *testing.T) {
	cfg := baseConfig()
	cfg.Calls = []Call{call("db", "read")}
	cfg.Resilience.Bulkhead = &Bulkhead{MaxConcurrent: 1}
	s, f := newTestService(t, cfg)
	entered, release := make(chan struct{}), make(chan struct{})
	f["db"].fn = func(context.Context, *Call, request) (bool, error) {
		entered <- struct{}{}
		<-release
		return false, nil
	}
	done := make(chan error)
	go func() { done <- s.handle(context.Background(), request{typ: "read", key: 1}) }()
	<-entered
	f["db"].fn = nil
	if err := s.handle(context.Background(), request{typ: "read", key: 2}); Cause(err) != CauseBulkheadFull {
		t.Errorf("cause %q", Cause(err))
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCircuitBreakerOpensAndFailsFast(t *testing.T) {
	cfg := baseConfig()
	cfg.Calls = []Call{call("db", "read")}
	cfg.Resilience.CircuitBreaker = &CircuitBreaker{ErrorRate: 0.5, WindowS: 10, OpenS: 30}
	s, f := newTestService(t, cfg)
	f["db"].fn = func(context.Context, *Call, request) (bool, error) { return false, syscall.ECONNREFUSED }
	for range BreakerMinCalls {
		_ = s.handle(context.Background(), request{typ: "read", key: 1})
	}
	before := f["db"].count.Load()
	err := s.handle(context.Background(), request{typ: "read", key: 1})
	if Cause(err) != CauseCircuitOpen || f["db"].count.Load() != before {
		t.Errorf("cause %q, calls reached the target: %v", Cause(err), f["db"].count.Load() != before)
	}
}

func TestConcurrencyGateQueues(t *testing.T) {
	cfg := baseConfig()
	cfg.Concurrency = Concurrency{Model: "threads", Max: 1}
	cfg.Calls = []Call{call("db", "read")}
	s, f := newTestService(t, cfg)
	entered, release := make(chan struct{}, 2), make(chan struct{})
	f["db"].fn = func(context.Context, *Call, request) (bool, error) {
		entered <- struct{}{}
		<-release
		return false, nil
	}
	go func() { _ = s.handle(context.Background(), request{typ: "read", key: 1}) }()
	<-entered
	second := make(chan error)
	go func() { second <- s.handle(context.Background(), request{typ: "read", key: 2}) }()
	select {
	case <-entered:
		t.Fatal("second request ran while the only slot was taken")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatal(err)
	}

	<-entered // the second request's

	// A request that gives up while queued reports canceled.
	hold := make(chan struct{})
	defer close(hold)
	f["db"].fn = func(ctx context.Context, _ *Call, _ request) (bool, error) {
		entered <- struct{}{}
		select {
		case <-hold:
		case <-ctx.Done():
		}
		return false, nil
	}
	go func() { _ = s.handle(context.Background(), request{typ: "read", key: 3}) }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.handle(ctx, request{typ: "read", key: 4}); Cause(err) != CauseCanceled {
		t.Errorf("cause %q", Cause(err))
	}
}

func TestEventLoopSerializesCPU(t *testing.T) {
	elapsed := func(model string) time.Duration {
		cfg := baseConfig()
		cfg.Concurrency = Concurrency{Model: model}
		cfg.CPUMs = 60
		s, _ := newTestService(t, cfg)
		var wg sync.WaitGroup
		start := time.Now()
		for range 2 {
			wg.Go(func() { _ = s.handle(context.Background(), request{typ: "read", key: 1}) })
		}
		wg.Wait()
		return time.Since(start)
	}
	if d := elapsed("event-loop"); d < 115*time.Millisecond {
		t.Errorf("event-loop: two 60 ms requests took %v; CPU work should not overlap", d)
	}
	if d := elapsed("threads"); d > 110*time.Millisecond {
		t.Errorf("threads: two 60 ms requests took %v; CPU work should overlap", d)
	}
}

func TestBurnCPUUsesCPUTime(t *testing.T) {
	start := time.Now()
	burnCPU(30 * time.Millisecond)
	if d := time.Since(start); d < 30*time.Millisecond {
		t.Errorf("burned for %v", d)
	}
}

func TestHTTPHandler(t *testing.T) {
	cfg := baseConfig()
	cfg.Calls = []Call{call("db", "read")}
	cfg.Resilience.TimeoutMs = 10
	s, f := newTestService(t, cfg)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	type result struct {
		StatusCode int
		Header     http.Header
	}
	get := func(path string) (result, string) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return result{resp.StatusCode, resp.Header}, string(b)
	}
	if resp, _ := get("/r?type=read&key=5"); resp.StatusCode != 200 {
		t.Errorf("status %d", resp.StatusCode)
	}
	if got := f["db"].calls(); len(got) != 1 || got[0] != "read:read:5" {
		t.Errorf("calls %v", got)
	}
	if resp, _ := get("/r?type=delete"); resp.StatusCode != 400 {
		t.Errorf("bad type: status %d", resp.StatusCode)
	}
	if resp, _ := get("/r?key=-1"); resp.StatusCode != 400 {
		t.Errorf("bad key: status %d", resp.StatusCode)
	}

	f["db"].fn = func(ctx context.Context, _ *Call, _ request) (bool, error) { <-ctx.Done(); return false, ctx.Err() }
	resp, _ := get("/r?type=write&key=1")
	if resp.StatusCode != http.StatusGatewayTimeout || resp.Header.Get("X-Loadlore-Cause") != CauseTimeout {
		t.Errorf("timeout: status %d, cause %q", resp.StatusCode, resp.Header.Get("X-Loadlore-Cause"))
	}

	_, body := get("/metrics")
	if len(body) < 600 || !strings.HasPrefix(body, "#") {
		t.Fatalf("metrics body: %.600s", body)
	}
	for _, want := range []string{
		`loadlore_requests_total{code="200",replica="0",service="api",type="read"} 1`,
		`loadlore_requests_total{code="504",replica="0",service="api",type="write"} 1`,
		`loadlore_request_errors_total{cause="timeout",replica="0",service="api"} 1`,
		`loadlore_call_errors_total{cause="timeout",op="read",replica="0",service="api",target="db"} 1`,
		`loadlore_request_duration_seconds_bucket{replica="0",service="api",type="read",le="0.3"}`,
		`loadlore_requests_inflight{replica="0",service="api"} 0`,
		`loadlore_pool_max_connections{replica="0",service="api",target="db"}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %s\n%s", want, grepLines(body, strings.SplitN(want, "{", 2)[0]))
		}
	}
	if resp, _ := get("/healthz"); resp.StatusCode != 200 {
		t.Errorf("healthz %d", resp.StatusCode)
	}
}

func TestCause(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, CauseTimeout},
		{fmt.Errorf("query: %w", context.DeadlineExceeded), CauseTimeout},
		{context.Canceled, CauseCanceled},
		{syscall.ECONNREFUSED, CauseConnRefused},
		{fmt.Errorf("read: %w", syscall.ECONNRESET), CauseConnReset},
		{io.ErrUnexpectedEOF, CauseConnReset},
		{&pgconn.PgError{Code: "53300"}, CauseTooManyConns},
		{&pgconn.PgError{Code: "57014"}, CauseTimeout},
		{&pgconn.PgError{Code: "40P01"}, CauseDBError},
		{redis.ErrPoolTimeout, CausePoolTimeout},
		{withCause(CausePoolTimeout, context.DeadlineExceeded), CausePoolTimeout},
		{errCircuitOpen, CauseCircuitOpen},
		{errors.New("mystery"), CauseOther},
	}
	for _, tt := range tests {
		if got := Cause(tt.err); got != tt.want {
			t.Errorf("Cause(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

func TestBackoff(t *testing.T) {
	half := func() float64 { return 0.5 }
	tests := []struct {
		r    Retries
		n    int
		want time.Duration
	}{
		{Retries{Backoff: "none"}, 3, 0},
		{Retries{Backoff: "constant", BaseMs: 50}, 3, 50 * time.Millisecond},
		{Retries{Backoff: "exponential", BaseMs: 50}, 1, 50 * time.Millisecond},
		{Retries{Backoff: "exponential", BaseMs: 50}, 3, 200 * time.Millisecond},
		{Retries{Backoff: "exponential", BaseMs: 50, Jitter: true}, 3, 100 * time.Millisecond},
	}
	for _, tt := range tests {
		if got := backoff(&tt.r, tt.n, half); got != tt.want {
			t.Errorf("%+v attempt %d: %v, want %v", tt.r, tt.n, got, tt.want)
		}
	}
}

func TestBreakerStateMachine(t *testing.T) {
	now := time.Unix(1000, 0)
	b := newBreaker(CircuitBreaker{ErrorRate: 0.5, WindowS: 10, OpenS: 30}, func() time.Time { return now })
	for range BreakerMinCalls - 1 {
		b.record(true)
	}
	if !b.allow() {
		t.Fatal("opened below the minimum number of calls")
	}
	b.record(true)
	if b.allow() {
		t.Fatal("still closed at 100% errors")
	}
	now = now.Add(31 * time.Second)
	if !b.allow() {
		t.Fatal("no half-open trial after open_s")
	}
	if b.allow() {
		t.Fatal("a second call got through while half-open")
	}
	b.record(true)
	if b.allow() {
		t.Fatal("failed trial should reopen")
	}
	now = now.Add(31 * time.Second)
	b.allow()
	b.record(false)
	if !b.allow() || b.isOpen() {
		t.Fatal("successful trial should close")
	}

	// Failures older than the window don't count.
	b = newBreaker(CircuitBreaker{ErrorRate: 0.5, WindowS: 10, OpenS: 30}, func() time.Time { return now })
	for range BreakerMinCalls - 1 {
		b.record(true)
	}
	now = now.Add(11 * time.Second)
	b.record(true)
	if b.isOpen() {
		t.Fatal("opened on failures that left the window")
	}
}

func TestConfigValidation(t *testing.T) {
	good := baseConfig()
	good.Calls = []Call{call("db", "read")}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	for name, mod := range map[string]func(*Config){
		"no service":     func(c *Config) { c.Service = "" },
		"bad model":      func(c *Config) { c.Concurrency.Model = "fibers" },
		"workers zero":   func(c *Config) { c.Concurrency = Concurrency{Model: "workers"} },
		"unknown target": func(c *Config) { c.Calls = []Call{call("nope", "read")} },
		"op mismatch":    func(c *Config) { c.Calls = []Call{call("cache", "read")} },
		"zero count":     func(c *Config) { c.Calls = []Call{call("db", "read", func(c *Call) { c.Count = 0 })} },
		"pool zero":      func(c *Config) { c.Targets["x"] = Target{Kind: KindRedis, Addrs: []string{"a:1"}} },
		"pg no rows":     func(c *Config) { c.Targets["x"] = Target{Kind: KindPostgres, Addrs: []string{"a:1"}, Pool: 1} },
		"bad retries":    func(c *Config) { c.Resilience.Retries = &Retries{Max: 1, Backoff: "exponential"} },
		"nested": func(c *Config) {
			c.Calls = []Call{call("cache", "get", func(c *Call) { c.OnMiss = []Call{call("db", "get")} })}
		},
	} {
		c := baseConfig()
		c.Calls = []Call{call("db", "read")}
		mod(c)
		if c.Validate() == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := ParseConfig([]byte(`{"service":"api","concurrency":{"model":"threads"}}`)); err != nil {
		t.Errorf("minimal config: %v", err)
	}
}

func grepLines(body, prefix string) string {
	var out []string
	for l := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
