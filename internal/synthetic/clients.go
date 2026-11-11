package synthetic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/akinfemi/load-lore/internal/dataset"
)

// request is what an incoming request asks for, passed to every call.
type request struct {
	typ string // read | write
	key int64
}

// client is one target's connection pool and the ops it supports.
type client interface {
	// do runs one call attempt. hit is meaningful for get only.
	do(ctx context.Context, c *Call, r request) (hit bool, err error)
	snapshot() poolSnapshot
	ping(ctx context.Context) error
	close()
}

func newClient(name string, t Target, timeout time.Duration, m *metrics) (client, error) {
	switch t.Kind {
	case KindPostgres:
		return newPGClient(name, t, m)
	case KindRedis:
		return newRedisClient(name, t, timeout), nil
	case KindHTTP:
		return newHTTPClient(name, t, m), nil
	}
	return nil, fmt.Errorf("target %s: unknown kind %q", name, t.Kind)
}

// --- postgres (and pgbouncer) ---

type pgClient struct {
	name    string
	min     int
	pool    *pgxpool.Pool
	spec    dataset.Spec
	payload string
	m       *metrics
	pending atomic.Int64
}

func newPGClient(name string, t Target, m *metrics) (*pgClient, error) {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(t.User, t.Password),
		Host:     t.Addrs[0],
		Path:     "/" + t.Database,
		RawQuery: "sslmode=disable&application_name=loadlore-synthetic",
	}
	cfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		return nil, fmt.Errorf("target %s: %w", name, err)
	}
	cfg.MaxConns = int32(t.Pool)
	// With a minimum, pgxpool opens that many connections in the
	// background at once and keeps them open; without one, it opens
	// connections only when callers need them.
	cfg.MinConns = int32(t.PoolMin)
	cfg.ConnConfig.Tracer = &connectTracer{target: name, m: m}
	if t.SimpleProtocol {
		// pgbouncer in transaction mode can't keep server-side prepared
		// statements across transactions.
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("target %s: %w", name, err)
	}
	spec := dataset.Spec{Rows: t.Rows, RowBytes: t.RowBytes}
	return &pgClient{
		name: name, min: t.PoolMin, pool: pool, spec: spec, m: m,
		payload: strings.Repeat("w", spec.PayloadLen()),
	}, nil
}

func (p *pgClient) do(ctx context.Context, c *Call, r request) (bool, error) {
	p.pending.Add(1)
	start := time.Now()
	conn, err := p.pool.Acquire(ctx)
	p.pending.Add(-1)
	p.m.poolWait.WithLabelValues(p.name).Observe(time.Since(start).Seconds())
	if err != nil {
		// A deadline while waiting means the pool, not the database, was
		// the bottleneck. Connect errors (too many connections, refused)
		// keep their own cause.
		return false, poolWaitErr(ctx, err)
	}
	defer conn.Release()

	key := clampKey(r.key, p.spec.Rows)
	switch {
	case c.Op == "write":
		_, err = conn.Exec(ctx, dataset.Write, key, p.payload)
	case c.Query == "range":
		var rows pgx.Rows
		rows, err = conn.Query(ctx, dataset.RangeRead, key, c.Rows)
		if err == nil {
			for rows.Next() {
			}
			rows.Close()
			err = rows.Err()
		}
	case c.Query == "batch":
		// The request's key and the ones after it, as a page of items
		// fetched by id in one statement.
		keys := make([]int64, c.Rows)
		for i := range keys {
			keys[i] = clampKey(r.key+int64(i), p.spec.Rows)
		}
		var rows pgx.Rows
		rows, err = conn.Query(ctx, dataset.BatchRead, keys)
		if err == nil {
			for rows.Next() {
			}
			rows.Close()
			err = rows.Err()
		}
	case c.Query == "scan":
		var n, maxLen *int64
		err = conn.QueryRow(ctx, dataset.ScanRead, p.spec.UserFor(key)).Scan(&n, &maxLen)
	default:
		var payload string
		err = conn.QueryRow(ctx, dataset.PointRead, key).Scan(&payload)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		}
	}
	return false, err
}

func (p *pgClient) snapshot() poolSnapshot {
	s := p.pool.Stat()
	return poolSnapshot{
		target: p.name, max: int64(s.MaxConns()), inUse: int64(s.AcquiredConns()),
		idle: int64(s.IdleConns()), pending: p.pending.Load(),
	}
}

// ping checks the database answers and, for a pool with a minimum, that
// the minimum is open, so the load starts on a warm pool.
func (p *pgClient) ping(ctx context.Context) error {
	if err := p.pool.Ping(ctx); err != nil {
		return err
	}
	s := p.pool.Stat()
	if open := int(s.AcquiredConns() + s.IdleConns()); open < p.min {
		return fmt.Errorf("%d of the pool's minimum %d connections open", open, p.min)
	}
	return nil
}

// connectTracer times each new connection a pool opens.
type connectTracer struct {
	target string
	m      *metrics
}

type connectStartKey struct{}

func (t *connectTracer) TraceConnectStart(ctx context.Context, _ pgx.TraceConnectStartData) context.Context {
	return context.WithValue(ctx, connectStartKey{}, time.Now())
}

func (t *connectTracer) TraceConnectEnd(ctx context.Context, data pgx.TraceConnectEndData) {
	start, ok := ctx.Value(connectStartKey{}).(time.Time)
	if !ok {
		return
	}
	outcome := "ok"
	if data.Err != nil {
		outcome = "error"
	}
	t.m.connects.WithLabelValues(t.target, outcome).Observe(time.Since(start).Seconds())
}

// pgx calls these for every query; only connections are traced.
func (t *connectTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}
func (t *connectTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (p *pgClient) close()                                                               { p.pool.Close() }

// poolWaitErr labels an error that ended a wait for a pool connection. A
// deadline means the pool was the bottleneck; a cancel means the caller left.
func poolWaitErr(ctx context.Context, err error) error {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return withCause(CausePoolTimeout, err)
	case ctx.Err() != nil:
		return withCause(CauseCanceled, err)
	}
	return err
}

// clampKey keeps keys inside the dataset so every point read finds a row.
func clampKey(key, rows int64) int64 {
	if rows < 1 {
		return max(key, 1)
	}
	return (max(key, 1)-1)%rows + 1
}

// --- redis ---

type redisClient struct {
	name  string
	rdb   *redis.Client
	value []byte
}

func newRedisClient(name string, t Target, timeout time.Duration) *redisClient {
	opts := &redis.Options{
		Addr:     t.Addrs[0],
		PoolSize: t.Pool,
		// No hidden retries: retries come only from the design's resilience
		// settings, so they show up in the metrics.
		MaxRetries: -1,
		// Per-call deadlines come from the context. Without a call timeout,
		// reads, writes and pool waits never time out, like a client
		// configured with no timeouts.
		ContextTimeoutEnabled: true,
		ReadTimeout:           -1,
		WriteTimeout:          -1,
		PoolTimeout:           time.Hour,
	}
	if timeout > 0 {
		opts.PoolTimeout = timeout
	}
	size := t.RowBytes
	if size < 1 {
		size = 512
	}
	return &redisClient{name: name, rdb: redis.NewClient(opts), value: []byte(strings.Repeat("v", size))}
}

func cacheKey(key int64) string { return "item:" + strconv.FormatInt(key, 10) }

func (c *redisClient) do(ctx context.Context, call *Call, r request) (bool, error) {
	k := cacheKey(r.key)
	switch call.Op {
	case "get":
		err := c.rdb.Get(ctx, k).Err()
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return err == nil, cacheErr(err)
	case "set":
		return false, cacheErr(c.rdb.Set(ctx, k, c.value, time.Duration(call.TTLS)*time.Second).Err())
	case "del":
		return false, cacheErr(c.rdb.Del(ctx, k).Err())
	}
	return false, fmt.Errorf("redis: unsupported op %q", call.Op)
}

// cacheErr labels Redis server errors; network and timeout errors keep
// their own cause.
func cacheErr(err error) error {
	var re redis.Error
	if errors.As(err, &re) {
		return withCause(CauseCacheError, err)
	}
	return err
}

func (c *redisClient) snapshot() poolSnapshot {
	s := c.rdb.PoolStats()
	waits := int64(s.WaitCount)
	secs := float64(s.WaitDurationNs) / 1e9
	return poolSnapshot{
		target: c.name, max: int64(c.rdb.Options().PoolSize),
		inUse: int64(s.TotalConns) - int64(s.IdleConns), idle: int64(s.IdleConns),
		pending: int64(s.PendingRequests), waits: &waits, waitSeconds: &secs,
	}
}

func (c *redisClient) ping(ctx context.Context) error { return c.rdb.Ping(ctx).Err() }
func (c *redisClient) close()                         { _ = c.rdb.Close() }

// --- http (other application services) ---

type httpClient struct {
	name    string
	bases   []string
	next    atomic.Uint64
	hc      *http.Client
	m       *metrics
	pool    int
	pending atomic.Int64
	active  atomic.Int64
}

func newHTTPClient(name string, t Target, m *metrics) *httpClient {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxConnsPerHost:     t.Pool,
		MaxIdleConnsPerHost: t.Pool,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	bases := make([]string, len(t.Addrs))
	for i, a := range t.Addrs {
		bases[i] = "http://" + a
	}
	return &httpClient{name: name, bases: bases, hc: &http.Client{Transport: tr}, m: m, pool: t.Pool}
}

func (h *httpClient) do(ctx context.Context, _ *Call, r request) (bool, error) {
	var waitStart time.Time
	var gotConn bool
	trace := &httptrace.ClientTrace{
		GetConn: func(string) { waitStart = time.Now(); h.pending.Add(1) },
		GotConn: func(httptrace.GotConnInfo) {
			gotConn = true
			h.pending.Add(-1)
			h.active.Add(1)
			h.m.poolWait.WithLabelValues(h.name).Observe(time.Since(waitStart).Seconds())
		},
	}
	base := h.bases[(h.next.Add(1)-1)%uint64(len(h.bases))] // round-robin across replicas
	u := fmt.Sprintf("%s/r?type=%s&key=%d", base, r.typ, r.key)
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	resp, err := h.hc.Do(req)
	if gotConn {
		defer h.active.Add(-1)
	} else if !waitStart.IsZero() {
		h.pending.Add(-1)
		if err != nil {
			return false, poolWaitErr(ctx, err)
		}
	}
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return false, withCause(CauseUpstreamError, fmt.Errorf("%s answered %s", h.name, resp.Status))
	}
	return false, nil
}

func (h *httpClient) snapshot() poolSnapshot {
	inUse := h.active.Load()
	return poolSnapshot{target: h.name, max: int64(h.pool), inUse: inUse, pending: h.pending.Load()}
}

func (h *httpClient) ping(ctx context.Context) error {
	for _, base := range h.bases {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
		if err != nil {
			return err
		}
		resp, err := h.hc.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s at %s: %s", h.name, base, resp.Status)
		}
	}
	return nil
}

func (h *httpClient) close() { h.hc.CloseIdleConnections() }
