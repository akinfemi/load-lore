//go:build integration

package synthetic

// Integration tests against real Postgres and Redis containers. Run with
// `make integration`. On Colima, set DOCKER_HOST to the Colima socket and
// TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock.

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/akinfemi/load-lore/internal/dataset"
)

const (
	pgImage    = "postgres:17.11"
	redisImage = "redis:8.2"
	testRows   = 10_000
)

var testSpec = dataset.Spec{Rows: testRows, RowBytes: 256}

type pgEnv struct {
	addr string
	dsn  string
}

// startPostgres runs Postgres with extra server args and seeds the dataset.
func startPostgres(t *testing.T, args ...string) pgEnv {
	t.Helper()
	ctx := context.Background()
	opts := []testcontainers.ContainerCustomizer{
		tcpostgres.WithDatabase("lab"),
		tcpostgres.WithUsername("lab"),
		tcpostgres.WithPassword("lab"),
		tcpostgres.BasicWaitStrategies(),
		testcontainers.WithLabels(map[string]string{"io.loadlore.test": "1"}),
	}
	if len(args) > 0 {
		opts = append(opts, testcontainers.WithCmdArgs(args...))
	}
	c, err := tcpostgres.Run(ctx, pgImage, opts...)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for _, stmt := range testSpec.SeedSQL() {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("seeding: %v\n%s", err, stmt)
		}
	}
	u, _ := url.Parse(dsn)
	return pgEnv{addr: u.Host, dsn: dsn}
}

func startRedis(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	c, err := tcredis.Run(ctx, redisImage, testcontainers.WithLabels(map[string]string{"io.loadlore.test": "1"}))
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := c.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(cs)
	return u.Host
}

func pgTarget(env pgEnv, pool int) Target {
	return Target{Kind: KindPostgres, Addrs: []string{env.addr}, Pool: pool, Database: "lab", User: "lab", Password: "lab", Rows: testSpec.Rows, RowBytes: testSpec.RowBytes}
}

func newService(t *testing.T, cfg *Config) *Service {
	t.Helper()
	if cfg.Concurrency.Model == "" {
		cfg.Concurrency.Model = "threads"
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestIntegrationPostgresOps(t *testing.T) {
	env := startPostgres(t)
	s := newService(t, &Config{
		Service:    "api",
		Resilience: Resilience{TimeoutMs: 5000},
		Targets:    map[string]Target{"db": pgTarget(env, 4)},
		Calls: []Call{
			call("db", "read"),
			call("db", "read", func(c *Call) { c.Query = "range"; c.Rows = 25 }),
			call("db", "read", func(c *Call) { c.Query = "batch"; c.Rows = 3 }),
			call("db", "read", func(c *Call) { c.Query = "scan" }),
			call("db", "write", func(c *Call) { c.When = "write" }),
		},
	})
	ctx := context.Background()
	for i := range 20 {
		typ := "read"
		if i%4 == 0 {
			typ = "write"
		}
		if err := s.handle(ctx, request{typ: typ, key: int64(i*397 + 1)}); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if n := metricValue(t, s, "loadlore_calls_total", map[string]string{"target": "db", "op": "read", "outcome": "ok"}); n != 80 {
		t.Errorf("%v successful reads, want 80", n)
	}
	if n := metricValue(t, s, "loadlore_calls_total", map[string]string{"target": "db", "op": "write", "outcome": "ok"}); n != 5 {
		t.Errorf("%v successful writes, want 5", n)
	}

	// The write really changed the row.
	conn, err := pgx.Connect(ctx, env.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var payload string
	if err := conn.QueryRow(ctx, dataset.PointRead, 1).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload != strings.Repeat("w", testSpec.PayloadLen()) {
		t.Errorf("row 1 not updated: %.20q…", payload)
	}

	// Pool gauges reflect the configured size.
	if n := metricValue(t, s, "loadlore_pool_max_connections", map[string]string{"target": "db"}); n != 4 {
		t.Errorf("pool max %v", n)
	}
}

func TestIntegrationCacheAside(t *testing.T) {
	env := startPostgres(t)
	redisAddr := startRedis(t)
	s := newService(t, &Config{
		Service:    "api",
		Resilience: Resilience{TimeoutMs: 5000},
		Targets: map[string]Target{
			"db":    pgTarget(env, 2),
			"cache": {Kind: KindRedis, Addrs: []string{redisAddr}, Pool: 2, RowBytes: 256},
		},
		Calls: []Call{call("cache", "get", func(c *Call) {
			c.OnMiss = []Call{call("db", "read"), call("cache", "set", func(c *Call) { c.TTLS = 60 })}
		})},
	})
	ctx := context.Background()
	for range 3 {
		if err := s.handle(ctx, request{typ: "read", key: 42}); err != nil {
			t.Fatal(err)
		}
	}
	hits := metricValue(t, s, "loadlore_cache_lookups_total", map[string]string{"target": "cache", "result": "hit"})
	misses := metricValue(t, s, "loadlore_cache_lookups_total", map[string]string{"target": "cache", "result": "miss"})
	dbReads := metricValue(t, s, "loadlore_calls_total", map[string]string{"target": "db", "op": "read"})
	if hits != 2 || misses != 1 || dbReads != 1 {
		t.Errorf("hits %v, misses %v, db reads %v; want 2, 1, 1", hits, misses, dbReads)
	}
}

// lockRow holds a row lock in an open transaction until the returned func
// is called, so writes to that row block inside Postgres.
func lockRow(t *testing.T, dsn string, id int) func() {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM items WHERE id = $1 FOR UPDATE`, id); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			_ = tx.Rollback(ctx)
			_ = conn.Close(ctx)
		})
	}
	t.Cleanup(release)
	return release
}

// Two writes block on a locked row and hold both pool connections. A read
// of another row then waits for a connection until its deadline: it fails
// in the pool, not in the database, which is the pool-exhaustion signature.
// The writes have no call timeout so they keep holding the pool.
func TestIntegrationPoolExhaustion(t *testing.T) {
	env := startPostgres(t)
	s := newService(t, &Config{
		Service:    "api",
		Resilience: Resilience{TimeoutMs: 0},
		Targets:    map[string]Target{"db": pgTarget(env, 2)},
		Calls:      []Call{call("db", "read", func(c *Call) { c.When = "read" }), call("db", "write", func(c *Call) { c.When = "write" })},
	})
	release := lockRow(t, env.dsn, 1)
	defer release()

	ctx := context.Background()
	var wg sync.WaitGroup
	writeErrs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() { writeErrs[i] = s.handle(ctx, request{typ: "write", key: 1}) })
	}
	waitFor(t, func() bool {
		return metricValue(t, s, "loadlore_pool_connections", map[string]string{"target": "db", "state": "in_use"}) == 2
	})
	readCtx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	readErr := s.handle(readCtx, request{typ: "read", key: 2})
	cancel()
	if Cause(readErr) != CausePoolTimeout {
		t.Errorf("read: cause %q (%v), want pool_timeout", Cause(readErr), readErr)
	}
	if n := metricValue(t, s, "loadlore_call_errors_total", map[string]string{"target": "db", "cause": "pool_timeout"}); n != 1 {
		t.Errorf("pool_timeout errors: %v", n)
	}
	if w := histogramSum(t, s, "loadlore_pool_wait_seconds", map[string]string{"target": "db"}); w < 0.35 {
		t.Errorf("total pool wait %.3fs, want about 0.4s from the read", w)
	}

	// A caller that leaves while waiting is canceled, not a pool timeout.
	leaveCtx, leave := context.WithCancel(ctx)
	time.AfterFunc(50*time.Millisecond, leave)
	if err := s.handle(leaveCtx, request{typ: "read", key: 3}); Cause(err) != CauseCanceled {
		t.Errorf("caller left: cause %q (%v), want canceled", Cause(err), err)
	}

	// Once the lock is gone the writes finish and the pool recovers.
	release()
	wg.Wait()
	for i, err := range writeErrs {
		if err != nil {
			t.Errorf("write %d: %v", i, err)
		}
	}
	if err := s.handle(ctx, request{typ: "read", key: 2}); err != nil {
		t.Errorf("after release: %v", err)
	}
}

// More pool connections than Postgres allows: new connections are refused
// by the server, which shows as too_many_conns, distinct from pool waits.
func TestIntegrationTooManyConnections(t *testing.T) {
	env := startPostgres(t, "-c", "max_connections=4", "-c", "superuser_reserved_connections=0", "-c", "reserved_connections=0")
	s := newService(t, &Config{
		Service:    "api",
		Resilience: Resilience{TimeoutMs: 2000},
		Targets:    map[string]Target{"db": pgTarget(env, 10)},
		Calls:      []Call{call("db", "write")},
	})
	release := lockRow(t, env.dsn, 1) // uses one of the four connections
	defer release()

	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex
	causes := map[string]int{}
	for range 8 {
		wg.Go(func() {
			err := s.handle(ctx, request{typ: "write", key: 1})
			mu.Lock()
			causes[Cause(err)]++
			mu.Unlock()
		})
	}
	wg.Wait()
	if causes[CauseTooManyConns] < 4 {
		t.Errorf("causes %v: want at least 4 too_many_conns (3 connections left for 8 callers)", causes)
	}
	if causes[CauseTimeout] != 3 {
		t.Errorf("causes %v: want 3 timeouts from the writes that got a connection and blocked", causes)
	}
}

func TestIntegrationDatasetShape(t *testing.T) {
	env := startPostgres(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, env.dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	var rows, users int64
	var avg float64
	if err := conn.QueryRow(ctx, `SELECT count(*), count(DISTINCT user_id), avg(pg_column_size(items.*)) FROM items`).Scan(&rows, &users, &avg); err != nil {
		t.Fatal(err)
	}
	if rows != testRows || users != testSpec.Users() {
		t.Errorf("rows %d, users %d", rows, users)
	}
	if avg < float64(testSpec.RowBytes)*0.9 || avg > float64(testSpec.RowBytes)*1.1 {
		t.Errorf("average row %.0f bytes, want about %d", avg, testSpec.RowBytes)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 5s")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A pool with a minimum opens it before the service reports ready; one
// without opens connections as callers need them. Each new connection is
// counted and timed.
func TestIntegrationPoolMinimum(t *testing.T) {
	env := startPostgres(t)
	warm := pgTarget(env, 6)
	warm.PoolMin = 5
	s := newService(t, &Config{
		Service:    "api",
		Resilience: Resilience{TimeoutMs: 5000},
		Targets:    map[string]Target{"db": warm, "cold": pgTarget(env, 6)},
		Calls:      []Call{call("db", "read"), call("cold", "read")},
	})
	ctx := context.Background()
	waitFor(t, func() bool { return s.clients["db"].ping(ctx) == nil })
	if n := metricValue(t, s, "loadlore_pool_connect_seconds", map[string]string{"target": "db", "outcome": "ok"}); n < 5 {
		t.Errorf("warm pool opened %v connections before the load, want 5", n)
	}
	if err := s.clients["cold"].ping(ctx); err != nil {
		t.Fatal(err)
	}
	if n := metricValue(t, s, "loadlore_pool_connect_seconds", map[string]string{"target": "cold", "outcome": "ok"}); n != 1 {
		t.Errorf("cold pool opened %v connections for one ping, want 1", n)
	}
	if open := s.clients["db"].snapshot(); open.inUse+open.idle < 5 {
		t.Errorf("warm pool has %d open", open.inUse+open.idle)
	}
}

// With hold request, a request's queries share one connection, held until
// the request responds; with hold call, each query takes its own.
func TestIntegrationHoldPerRequest(t *testing.T) {
	env := startPostgres(t)
	held := pgTarget(env, 2)
	held.Hold = HoldRequest
	s := newService(t, &Config{
		Service:    "api",
		Resilience: Resilience{TimeoutMs: 5000},
		Targets:    map[string]Target{"db": held, "each": pgTarget(env, 2)},
		Calls:      []Call{call("db", "read"), call("each", "read"), call("db", "read"), call("each", "read")},
	})
	ctx := context.Background()
	for i := range 5 {
		if err := s.handle(ctx, request{typ: "read", key: int64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	for target, want := range map[string]float64{"db": 5, "each": 10} {
		if n := metricValue(t, s, "loadlore_pool_hold_seconds", map[string]string{"target": target}); n != want {
			t.Errorf("%s: %v connections held, want %v", target, n, want)
		}
		if n := metricValue(t, s, "loadlore_pool_wait_seconds", map[string]string{"target": target}); n != want {
			t.Errorf("%s: %v acquires, want %v", target, n, want)
		}
	}
	// Released at the end of each request: the pool is free again.
	if snap := s.clients["db"].snapshot(); snap.inUse != 0 {
		t.Errorf("%d connections still in use", snap.inUse)
	}
}

func queueTarget(addr string, pool int) Target {
	return Target{Kind: KindQueue, Addrs: []string{addr}, Pool: pool, Stream: "jobs", Group: "workers", RowBytes: 100}
}

// An api publishes jobs; a worker with two job runners takes each one
// once, reads its key from Postgres, and deletes it when done, so the
// stream ends empty. Jobs published before the worker started are taken.
func TestIntegrationQueue(t *testing.T) {
	env := startPostgres(t)
	addr := startRedis(t)
	api := newService(t, &Config{
		Service: "api", Resilience: Resilience{TimeoutMs: 5000},
		Targets: map[string]Target{"jobs": queueTarget(addr, 2)},
		Calls:   []Call{call("jobs", "publish", func(c *Call) { c.When = "write" })},
	})
	ctx := context.Background()
	const n = 20
	for i := range n {
		if err := api.handle(ctx, request{typ: "write", key: int64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := api.handle(ctx, request{typ: "read", key: 1}); err != nil { // publishes nothing
		t.Fatal(err)
	}
	w := newService(t, &Config{
		Service: "sender", Role: RoleWorker, Resilience: Resilience{TimeoutMs: 5000},
		Consume: &Consume{Target: "jobs", Concurrency: 2, Consumer: "sender-0"},
		Targets: map[string]Target{"jobs": queueTarget(addr, 3), "db": pgTarget(env, 2)},
		Calls:   []Call{call("db", "read", func(c *Call) { c.When = "write" })},
	})
	wctx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- w.RunWorker(wctx) }()
	deadline := time.Now().Add(15 * time.Second)
	for metricValue(t, w, "loadlore_jobs_total", map[string]string{"queue": "jobs", "result": "ok"}) < n {
		if time.Now().After(deadline) {
			t.Fatalf("worker finished %v of %d jobs", metricValue(t, w, "loadlore_jobs_total", nil), n)
		}
		time.Sleep(50 * time.Millisecond)
	}
	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := metricValue(t, w, "loadlore_calls_total", map[string]string{"target": "db", "op": "read", "outcome": "ok"}); got != n {
		t.Errorf("worker made %v db reads, want %d (one per write job)", got, n)
	}
	if got := metricValue(t, w, "loadlore_job_wait_seconds", map[string]string{"queue": "jobs"}); got != n {
		t.Errorf("job waits observed: %v, want %d", got, n)
	}
	// Busy time covers the queue's round trips as well as the jobs, and
	// never the waits for a job to arrive.
	busy := metricValue(t, w, "loadlore_job_runner_busy_seconds_total", nil)
	running := metricValue(t, w, "loadlore_job_duration_seconds", nil) // the count; the sum is below
	if sum := histogramSum(t, w, "loadlore_job_duration_seconds", nil); busy < sum || busy <= 0 || running != n {
		t.Errorf("runner busy %.4f s against %.4f s in jobs (%v jobs)", busy, sum, running)
	}
	q := w.clients["jobs"].(*queueClient)
	if l, err := q.rdb.XLen(ctx, "jobs").Result(); err != nil || l != 0 {
		t.Errorf("stream length %d (%v), want 0", l, err)
	}
	if pending, err := q.rdb.XPending(ctx, "jobs", "workers").Result(); err != nil || pending.Count != 0 {
		t.Errorf("pending %+v (%v), want none", pending, err)
	}
}

// A job whose calls fail goes to the retry set, back on the queue once
// due, and onto the dead list after its last retry: with max 2, each job
// runs three times, and the stream and retry set end empty.
func TestIntegrationQueueRetries(t *testing.T) {
	addr := startRedis(t)
	api := newService(t, &Config{
		Service: "api", Resilience: Resilience{TimeoutMs: 5000},
		Targets: map[string]Target{"jobs": queueTarget(addr, 2)},
		Calls:   []Call{call("jobs", "publish")},
	})
	ctx := context.Background()
	const n = 5
	for i := range n {
		if err := api.handle(ctx, request{typ: "write", key: int64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	w := newService(t, &Config{
		Service: "sender", Role: RoleWorker, Resilience: Resilience{TimeoutMs: 500},
		Consume: &Consume{Target: "jobs", Concurrency: 2, Consumer: "sender-0", Retries: &JobRetries{Max: 2, BaseMs: 100}},
		// Nothing listens on port 1: every call fails at once.
		Targets: map[string]Target{"jobs": queueTarget(addr, 3), "provider": {Kind: KindHTTP, Addrs: []string{"127.0.0.1:1"}, Pool: 2}},
		Calls:   []Call{call("provider", "http")},
	})
	wctx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- w.RunWorker(wctx) }()
	deadline := time.Now().Add(15 * time.Second)
	for metricValue(t, w, "loadlore_jobs_dead_total", map[string]string{"queue": "jobs"}) < n {
		if time.Now().After(deadline) {
			t.Fatalf("dead %v, retried %v of %d jobs", metricValue(t, w, "loadlore_jobs_dead_total", nil), metricValue(t, w, "loadlore_jobs_retried_total", nil), n)
		}
		time.Sleep(50 * time.Millisecond)
	}
	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]float64{
		"loadlore_jobs_retried_total":  2 * n,
		"loadlore_jobs_requeued_total": 2 * n,
		"loadlore_jobs_dead_total":     n,
	} {
		if got := metricValue(t, w, name, map[string]string{"queue": "jobs"}); got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	if got := metricValue(t, w, "loadlore_jobs_total", map[string]string{"queue": "jobs", "result": "error"}); got != 3*n {
		t.Errorf("failed runs %v, want %d (three per job)", got, 3*n)
	}
	q := w.clients["jobs"].(*queueClient)
	if l, err := q.rdb.XLen(ctx, "jobs").Result(); err != nil || l != 0 {
		t.Errorf("stream length %d (%v), want 0", l, err)
	}
	if l, err := q.rdb.ZCard(ctx, q.retryKey()).Result(); err != nil || l != 0 {
		t.Errorf("retry set %d (%v), want 0", l, err)
	}
	dead, err := q.rdb.LRange(ctx, q.deadKey(), 0, -1).Result()
	if err != nil || len(dead) != n {
		t.Fatalf("dead list %v (%v), want %d", dead, err, n)
	}
	if r, attempt, ok := parseRetryMember(dead[0]); !ok || attempt != 3 || r.typ != "write" {
		t.Errorf("dead member %q: %+v attempt %d", dead[0], r, attempt)
	}
}

// A full queue refuses jobs, and the publish fails with queue_full: the
// request fails with it.
func TestIntegrationQueueFull(t *testing.T) {
	addr := startRedis(t)
	api := newService(t, &Config{
		Service: "api", Resilience: Resilience{TimeoutMs: 5000},
		Targets: map[string]Target{"jobs": {Kind: KindQueue, Addrs: []string{addr}, Pool: 2, Stream: "jobs", Group: "workers", RowBytes: 64 << 10}},
		Calls:   []Call{call("jobs", "publish")},
	})
	ctx := context.Background()
	q := api.clients["jobs"].(*queueClient)
	if err := q.rdb.ConfigSet(ctx, "maxmemory", "4mb").Err(); err != nil {
		t.Fatal(err)
	}
	if err := q.rdb.ConfigSet(ctx, "maxmemory-policy", "noeviction").Err(); err != nil {
		t.Fatal(err)
	}
	var err error
	for i := 0; i < 1000 && err == nil; i++ {
		err = api.handle(ctx, request{typ: "write", key: 1})
	}
	if Cause(err) != CauseQueueFull {
		t.Fatalf("after filling the queue: %v (cause %q), want %q", err, Cause(err), CauseQueueFull)
	}
	if got := metricValue(t, api, "loadlore_call_errors_total", map[string]string{"target": "jobs", "cause": CauseQueueFull}); got < 1 {
		t.Errorf("queue_full call errors: %v", got)
	}
}
