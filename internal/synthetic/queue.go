package synthetic

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// A queue is a Redis stream with one consumer group. Publishing adds an
// entry; each of a worker's job runners reads the next undelivered entry
// for the group, runs the job, then acknowledges and deletes it, so the
// stream's length is the jobs not yet finished: waiting, or running.

// Job fields in a stream entry. The entry's ID carries the time Redis
// added it, which is when the job was published.
const (
	jobType    = "t"
	jobKey     = "k"
	jobPayload = "p"
	// jobAttempt is how many times the job failed before: 0 when absent.
	jobAttempt = "a"
)

// A failed job, with retries, waits in the queue's retry set, a sorted set
// scored by when it is due; each worker replica moves due jobs back onto
// the stream, as Sidekiq's poller does. A job out of retries goes on the
// dead list, capped at deadKeep, as Sidekiq's dead set is.
const (
	retryPoll = 250 * time.Millisecond
	deadKeep  = 10000
)

// readBlock is how long a job runner waits for a job before asking again.
const readBlock = 2 * time.Second

type queueClient struct {
	name, stream, group string
	rdb                 *redis.Client
	payload             string
}

func newQueueClient(name string, t Target, timeout time.Duration) *queueClient {
	opts := &redis.Options{
		Addr:                  t.Addrs[0],
		PoolSize:              t.Pool,
		MaxRetries:            -1, // retries only from the design's resilience settings
		ContextTimeoutEnabled: true,
		ReadTimeout:           -1,
		WriteTimeout:          -1,
		PoolTimeout:           time.Hour,
	}
	if timeout > 0 {
		opts.PoolTimeout = timeout
	}
	return &queueClient{
		name: name, stream: t.Stream, group: t.Group,
		rdb: redis.NewClient(opts), payload: strings.Repeat("j", max(0, t.RowBytes)),
	}
}

func (c *queueClient) do(ctx context.Context, call *Call, r request) (bool, error) {
	if call.Op != "publish" {
		return false, fmt.Errorf("queue: unsupported op %q", call.Op)
	}
	err := c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: c.stream,
		Values: []any{jobType, r.typ, jobKey, r.key, jobPayload, c.payload},
	}).Err()
	return false, queueErr(err)
}

// queueErr labels Redis server errors: a full queue (maxmemory with
// noeviction) refuses new jobs with an OOM error.
func queueErr(err error) error {
	var re redis.Error
	if !errors.As(err, &re) {
		return err
	}
	if strings.HasPrefix(err.Error(), "OOM ") {
		return withCause(CauseQueueFull, err)
	}
	return withCause(CauseQueueError, err)
}

func (c *queueClient) snapshot() poolSnapshot {
	s := c.rdb.PoolStats()
	waits := int64(s.WaitCount)
	secs := float64(s.WaitDurationNs) / 1e9
	return poolSnapshot{
		target: c.name, max: int64(c.rdb.Options().PoolSize),
		inUse: int64(s.TotalConns) - int64(s.IdleConns), idle: int64(s.IdleConns),
		pending: int64(s.PendingRequests), waits: &waits, waitSeconds: &secs,
	}
}

func (c *queueClient) ping(ctx context.Context) error { return c.rdb.Ping(ctx).Err() }
func (c *queueClient) close()                         { _ = c.rdb.Close() }

// ensureGroup creates the consumer group, and the stream with it, if
// they don't exist. The group starts at the stream's first entry, so jobs
// published before any worker started are still taken.
func (c *queueClient) ensureGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, c.stream, c.group, "0").Err()
	if err != nil && strings.HasPrefix(err.Error(), "BUSYGROUP") {
		return nil
	}
	return err
}

// next waits up to readBlock for the next job for this consumer; nil when
// none came.
func (c *queueClient) next(ctx context.Context, consumer string) (*redis.XMessage, error) {
	res, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: c.group, Consumer: consumer, Streams: []string{c.stream, ">"},
		Count: 1, Block: readBlock,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, s := range res {
		for i := range s.Messages {
			return &s.Messages[i], nil
		}
	}
	return nil, nil
}

// done acknowledges a job and deletes it from the stream.
func (c *queueClient) done(ctx context.Context, id string) error {
	_, err := c.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.XAck(ctx, c.stream, c.group, id)
		p.XDel(ctx, c.stream, id)
		return nil
	})
	return err
}

func (c *queueClient) retryKey() string { return c.stream + ":retry" }
func (c *queueClient) deadKey() string  { return c.stream + ":dead" }

// retryMember is a job in the retry set or dead list: its type, key and
// failures so far, and the stream ID it last had, which keeps it unique.
func retryMember(r request, attempt int, id string) string {
	return fmt.Sprintf("%s|%d|%d|%s", r.typ, r.key, attempt, id)
}

func parseRetryMember(m string) (request, int, bool) {
	parts := strings.Split(m, "|")
	if len(parts) != 4 || (parts[0] != "read" && parts[0] != "write") {
		return request{}, 0, false
	}
	key, err1 := strconv.ParseInt(parts[1], 10, 64)
	attempt, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil {
		return request{}, 0, false
	}
	return request{typ: parts[0], key: key}, attempt, true
}

// retryLater puts a failed job in the retry set, due at due.
func (c *queueClient) retryLater(ctx context.Context, member string, due time.Time) error {
	return c.rdb.ZAdd(ctx, c.retryKey(), redis.Z{Score: float64(due.UnixMilli()), Member: member}).Err()
}

// bury puts a job out of retries on the dead list, keeping the newest.
func (c *queueClient) bury(ctx context.Context, member string) error {
	_, err := c.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.LPush(ctx, c.deadKey(), member)
		p.LTrim(ctx, c.deadKey(), 0, deadKeep-1)
		return nil
	})
	return err
}

// requeueDue moves the retries due by now back onto the stream, and says
// how many it moved. A job is moved by the replica whose ZREM took it, so
// two replicas polling at once don't both add it.
func (c *queueClient) requeueDue(ctx context.Context, now time.Time) (int, error) {
	due, err := c.rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key: c.retryKey(), Start: "-inf", Stop: strconv.FormatInt(now.UnixMilli(), 10),
		ByScore: true, Count: 200,
	}).Result()
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, m := range due {
		n, err := c.rdb.ZRem(ctx, c.retryKey(), m).Result()
		if err != nil {
			return moved, err
		}
		if n != 1 {
			continue // another replica took it
		}
		r, attempt, ok := parseRetryMember(m)
		if !ok {
			continue
		}
		if err := c.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: c.stream,
			Values: []any{jobType, r.typ, jobKey, r.key, jobAttempt, attempt, jobPayload, c.payload},
		}).Err(); err != nil {
			// The queue refused it, full or down: back in the retry set,
			// due again shortly.
			_ = c.retryLater(ctx, m, now.Add(time.Second))
			return moved, queueErr(err)
		}
		moved++
	}
	return moved, nil
}

// attemptOf is how many times a job failed before this run of it.
func attemptOf(m *redis.XMessage) int {
	if a, ok := m.Values[jobAttempt].(string); ok {
		if n, err := strconv.Atoi(a); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// jobBackoff is the wait before a job's nth retry: BaseMs doubling for
// each retry, give or take a quarter.
func jobBackoff(r *JobRetries, n int, rnd func() float64) time.Duration {
	d := time.Duration(r.BaseMs) * time.Millisecond << min(n-1, 20)
	return time.Duration(float64(d) * (0.75 + 0.5*rnd()))
}

// enqueuedAt is when Redis added the entry: the milliseconds part of its ID.
func enqueuedAt(id string) (time.Time, bool) {
	ms, _, ok := strings.Cut(id, "-")
	n, err := strconv.ParseInt(ms, 10, 64)
	if !ok || err != nil {
		return time.Time{}, false
	}
	return time.UnixMilli(n), true
}

// jobRequest is the request a job carries.
func jobRequest(m *redis.XMessage) request {
	r := request{typ: "write", key: 1}
	if t, ok := m.Values[jobType].(string); ok && (t == "read" || t == "write") {
		r.typ = t
	}
	if k, ok := m.Values[jobKey].(string); ok {
		if n, err := strconv.ParseInt(k, 10, 64); err == nil && n > 0 {
			r.key = n
		}
	}
	return r
}

// RunWorker takes jobs from the worker's queue until ctx ends: consume.
// concurrency job runners, each reading one job at a time, running the
// behavior for it and acknowledging it. A job whose calls fail is retried
// after a growing wait if consume has retries, and dead once they run
// out; without retries it is counted and dropped. It returns once every
// runner has finished its job.
func (s *Service) RunWorker(ctx context.Context) error {
	c := s.cfg.Consume
	if s.cfg.Role != RoleWorker || c == nil {
		return errors.New("not a worker")
	}
	q, ok := s.clients[c.Target].(*queueClient)
	if !ok {
		return fmt.Errorf("consume target %s isn't a queue", c.Target)
	}
	for {
		err := q.ensureGroup(ctx)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		time.Sleep(500 * time.Millisecond) // the queue isn't up yet
	}
	s.ready.Store(true)
	var wg sync.WaitGroup
	if c.Retries != nil {
		wg.Go(func() { s.pollRetries(ctx, q) })
	}
	for i := range c.Concurrency {
		consumer := fmt.Sprintf("%s-%d", c.Consumer, i)
		wg.Go(func() { s.runJobs(ctx, q, consumer) })
	}
	wg.Wait()
	return nil
}

// pollRetries moves due retries back onto the queue until ctx ends, and
// keeps the count of jobs waiting to retry.
func (s *Service) pollRetries(ctx context.Context, q *queueClient) {
	t := time.NewTicker(retryPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		n, err := q.requeueDue(ctx, time.Now())
		s.m.jobsRequeued.WithLabelValues(q.name).Add(float64(n))
		if err != nil && ctx.Err() == nil {
			s.m.jobReadErrors.WithLabelValues(q.name, Cause(queueErr(err))).Inc()
		}
		if pending, err := q.rdb.ZCard(ctx, q.retryKey()).Result(); err == nil {
			s.m.retryPending.WithLabelValues(q.name).Set(float64(pending))
		}
	}
}

// failed handles a job whose calls failed: into the retry set after its
// backoff while it has retries left, else onto the dead list. Without
// retries it is dropped, and counted as dead.
func (s *Service) failed(ctx context.Context, q *queueClient, msg *redis.XMessage, req request) {
	r := s.cfg.Consume.Retries
	attempt := attemptOf(msg) + 1
	member := retryMember(req, attempt, msg.ID)
	if r != nil && attempt <= r.Max {
		if err := q.retryLater(ctx, member, time.Now().Add(jobBackoff(r, attempt, s.rnd))); err != nil {
			s.m.jobReadErrors.WithLabelValues(q.name, Cause(queueErr(err))).Inc()
		}
		s.m.jobsRetried.WithLabelValues(q.name).Inc()
		return
	}
	if r != nil {
		if err := q.bury(ctx, member); err != nil {
			s.m.jobReadErrors.WithLabelValues(q.name, Cause(queueErr(err))).Inc()
		}
	}
	s.m.jobsDead.WithLabelValues(q.name).Inc()
}

func (s *Service) runJobs(ctx context.Context, q *queueClient, consumer string) {
	for ctx.Err() == nil {
		fetched := time.Now()
		msg, err := q.next(ctx, consumer)
		if err != nil {
			if ctx.Err() == nil {
				s.m.jobReadErrors.WithLabelValues(q.name, Cause(queueErr(err))).Inc()
				// The queue is down or unreachable: try again shortly.
				time.Sleep(time.Duration(100+rand.IntN(200)) * time.Millisecond)
			}
			continue
		}
		if msg == nil {
			continue
		}
		// The runner is busy from when it asked for this job, or from when
		// the job arrived if it had to wait for one, until it's acknowledged.
		busyFrom := fetched
		if at, ok := enqueuedAt(msg.ID); ok {
			s.m.jobWait.WithLabelValues(q.name).Observe(max(0, time.Since(at).Seconds()))
			if at.After(busyFrom) {
				busyFrom = at
			}
		}
		// A job runs to the end once taken, even as the worker stops.
		jctx := context.WithoutCancel(ctx)
		start := time.Now()
		err = s.runJob(jctx, jobRequest(msg))
		result := "ok"
		if err != nil {
			result = "error"
			s.m.jobErrors.WithLabelValues(q.name, Cause(err)).Inc()
			s.failed(jctx, q, msg, jobRequest(msg))
		}
		s.m.jobDuration.WithLabelValues(q.name).Observe(time.Since(start).Seconds())
		if derr := q.done(jctx, msg.ID); derr != nil {
			s.m.jobReadErrors.WithLabelValues(q.name, Cause(queueErr(derr))).Inc()
		}
		s.m.jobs.WithLabelValues(q.name, result).Inc()
		s.m.runnerBusy.Add(max(0, time.Since(busyFrom).Seconds()))
	}
}

// runJob does a job's work: the behavior's CPU and memory, then its calls,
// with the request the job carries.
func (s *Service) runJob(ctx context.Context, req request) error {
	s.m.jobsRunning.Inc()
	defer s.m.jobsRunning.Dec()
	var buf []byte
	if s.cfg.AllocKB > 0 {
		buf = make([]byte, s.cfg.AllocKB<<10)
		for i := 0; i < len(buf); i += 4096 {
			buf[i] = 1
		}
	}
	ctx, releaseHeld := withSession(ctx)
	defer releaseHeld()
	s.burn()
	err := s.runCalls(ctx, s.cfg.Calls, req)
	runtime.KeepAlive(buf)
	return err
}
