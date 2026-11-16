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
// behavior for it and acknowledging it. A job whose calls fail is counted
// and dropped. It returns once every runner has finished its job.
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
	for i := range c.Concurrency {
		consumer := fmt.Sprintf("%s-%d", c.Consumer, i)
		wg.Go(func() { s.runJobs(ctx, q, consumer) })
	}
	wg.Wait()
	return nil
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
