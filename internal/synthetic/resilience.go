package synthetic

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
)

// Error causes. They label metrics, so the set is small and fixed. The
// diagnosis rules match on them.
const (
	CausePoolTimeout   = "pool_timeout"   // waited for a pool connection until the call timed out
	CauseTimeout       = "timeout"        // the call itself ran past its timeout
	CauseCanceled      = "canceled"       // the caller went away
	CauseConnRefused   = "conn_refused"   // nothing listening, or the dependency is down
	CauseConnReset     = "conn_reset"     // connection reset or closed mid-call
	CauseTooManyConns  = "too_many_conns" // Postgres refused a new connection (max_connections)
	CauseDBError       = "db_error"       // any other Postgres error
	CauseCacheError    = "cache_error"    // any other Redis error
	CauseUpstreamError = "upstream_error" // an HTTP dependency answered with an error status
	CauseCircuitOpen   = "circuit_open"   // the circuit breaker rejected the call
	CauseBulkheadFull  = "bulkhead_full"  // the bulkhead rejected the call
	CauseOther         = "other"
)

// callError is an error with a known cause.
type callError struct {
	cause string
	err   error
}

func (e *callError) Error() string { return e.cause + ": " + e.err.Error() }
func (e *callError) Unwrap() error { return e.err }

func withCause(cause string, err error) error { return &callError{cause: cause, err: err} }

var errCircuitOpen = withCause(CauseCircuitOpen, errors.New("circuit breaker is open"))
var errBulkheadFull = withCause(CauseBulkheadFull, errors.New("bulkhead is full"))

// Cause classifies an error from a call.
func Cause(err error) string {
	if err == nil {
		return ""
	}
	var ce *callError
	if errors.As(err, &ce) {
		return ce.cause
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "53300" { // too_many_connections
			return CauseTooManyConns
		}
		if pgErr.Code == "57014" { // query_canceled, by our context deadline
			return CauseTimeout
		}
		return CauseDBError
	}
	switch {
	case errors.Is(err, redis.ErrPoolTimeout):
		return CausePoolTimeout
	case errors.Is(err, context.DeadlineExceeded) || isTimeout(err):
		return CauseTimeout
	case errors.Is(err, context.Canceled):
		return CauseCanceled
	case errors.Is(err, syscall.ECONNREFUSED):
		return CauseConnRefused
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EPIPE), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, net.ErrClosed):
		return CauseConnReset
	}
	var pgConnErr *pgconn.ConnectError
	if errors.As(err, &pgConnErr) {
		return CauseConnRefused
	}
	return CauseOther
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// retryable reports whether a failed call should be retried. Rejections by
// the breaker or bulkhead, and callers that went away, are not retried.
func retryable(cause string) bool {
	switch cause {
	case CauseCircuitOpen, CauseBulkheadFull, CauseCanceled:
		return false
	}
	return true
}

// backoff returns the delay before retry number n (1-based).
func backoff(r *Retries, n int, rnd func() float64) time.Duration {
	var d time.Duration
	switch r.Backoff {
	case "constant":
		d = time.Duration(r.BaseMs) * time.Millisecond
	case "exponential":
		d = time.Duration(r.BaseMs) * time.Millisecond << min(n-1, 20)
	default:
		return 0
	}
	if r.Jitter {
		d = time.Duration(rnd() * float64(d)) // full jitter
	}
	return d
}

func defaultRand() float64 { return rand.Float64() }

// sleepCtx waits for d or until ctx ends. It is only used for retry
// backoff, which is a real wait in real clients too.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// BreakerMinCalls is the fewest calls in the window before the breaker can
// open, so a single early failure doesn't trip it.
const BreakerMinCalls = 20

// breaker is a count-based circuit breaker over a sliding window of
// one-second buckets. Closed: calls pass and are counted. Open: calls fail
// fast until open_s passes. Half-open: one trial call decides.
type breaker struct {
	cfg CircuitBreaker
	now func() time.Time

	mu        sync.Mutex
	buckets   []bucket // ring, one per second of the window
	state     int
	openUntil time.Time
	trial     bool // a half-open trial call is in flight
}

type bucket struct {
	sec          int64
	calls, fails int
}

const (
	breakerClosed = iota
	breakerOpen
	breakerHalfOpen
)

func newBreaker(cfg CircuitBreaker, now func() time.Time) *breaker {
	return &breaker{cfg: cfg, now: now, buckets: make([]bucket, cfg.WindowS)}
}

// allow reports whether a call may proceed.
func (b *breaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case breakerOpen:
		if b.now().Before(b.openUntil) {
			return false
		}
		b.state, b.trial = breakerHalfOpen, true
		return true
	case breakerHalfOpen:
		if b.trial {
			return false
		}
		b.trial = true
		return true
	}
	return true
}

// record reports a call's outcome.
func (b *breaker) record(failed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if b.state == breakerHalfOpen {
		b.trial = false
		if failed {
			b.state, b.openUntil = breakerOpen, now.Add(time.Duration(b.cfg.OpenS)*time.Second)
		} else {
			b.state = breakerClosed
			clear(b.buckets)
		}
		return
	}
	if b.state == breakerOpen {
		return
	}
	sec := now.Unix()
	bk := &b.buckets[int(sec%int64(len(b.buckets)))]
	if bk.sec != sec {
		*bk = bucket{sec: sec}
	}
	bk.calls++
	if failed {
		bk.fails++
	}
	var calls, fails int
	for _, x := range b.buckets {
		if sec-x.sec < int64(len(b.buckets)) {
			calls += x.calls
			fails += x.fails
		}
	}
	if calls >= BreakerMinCalls && float64(fails)/float64(calls) >= b.cfg.ErrorRate {
		b.state, b.openUntil = breakerOpen, now.Add(time.Duration(b.cfg.OpenS)*time.Second)
	}
}

// abandon ends a call without a verdict: a half-open trial is released so
// another call can try.
func (b *breaker) abandon() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == breakerHalfOpen {
		b.trial = false
	}
}

func (b *breaker) isOpen() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state != breakerClosed
}

// bulkhead rejects calls beyond a concurrency cap instead of queueing them.
type bulkhead chan struct{}

func (b bulkhead) tryAcquire() bool {
	select {
	case b <- struct{}{}:
		return true
	default:
		return false
	}
}

func (b bulkhead) release() { <-b }
