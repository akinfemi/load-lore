package synthetic

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metric names are part of Loadlore's contract with the collector and the
// diagnosis rules. Every series carries const labels service and replica.
const metricPrefix = "loadlore_"

// latencyBuckets resolve the thresholds people write in expect lines
// (50 ms, 300 ms, 1 s, ...) without interpolating across wide buckets.
var latencyBuckets = []float64{
	0.0005, 0.001, 0.002, 0.003, 0.005, 0.0075, 0.01, 0.015, 0.02, 0.03, 0.05, 0.075,
	0.1, 0.15, 0.2, 0.25, 0.3, 0.4, 0.5, 0.75, 1, 1.5, 2, 3, 5, 7.5, 10, 15, 30, 60,
}

type metrics struct {
	reg *prometheus.Registry

	requests     *prometheus.CounterVec   // type, code
	reqDuration  *prometheus.HistogramVec // type
	reqErrors    *prometheus.CounterVec   // cause
	inflight     prometheus.Gauge
	queued       prometheus.Gauge
	queueWait    prometheus.Observer
	cpuSeconds   prometheus.Counter
	calls        *prometheus.CounterVec   // target, op, outcome
	callDuration *prometheus.HistogramVec // target, op
	callErrors   *prometheus.CounterVec   // target, op, cause
	retries      *prometheus.CounterVec   // target, op
	cacheLookups *prometheus.CounterVec   // target, result
	poolWait     *prometheus.HistogramVec // target
	connects     *prometheus.HistogramVec // target, outcome
	poolHold     *prometheus.HistogramVec // target
	asyncDropped prometheus.Counter
	coalesced    *prometheus.CounterVec // target
	// readAfterWrite counts point reads of a key the request wrote.
	readAfterWrite *prometheus.CounterVec // target, result
	// Workers.
	jobs          *prometheus.CounterVec   // queue, result
	jobErrors     *prometheus.CounterVec   // queue, cause
	jobReadErrors *prometheus.CounterVec   // queue, cause
	jobWait       *prometheus.HistogramVec // queue
	jobDuration   *prometheus.HistogramVec // queue
	jobsRunning   prometheus.Gauge
}

// jobWaitBuckets reach further than latencyBuckets: a backed-up queue
// holds jobs for minutes.
var jobWaitBuckets = append(append([]float64{}, latencyBuckets...), 90, 120, 180, 300, 600, 1200, 1800)

func newMetrics(cfg *Config, pools func() []poolSnapshot) *metrics {
	reg := prometheus.NewRegistry()
	f := func(c prometheus.Collector) { reg.MustRegister(c) }
	f(collectors.NewGoCollector())
	f(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	labels := prometheus.Labels{"service": cfg.Service, "replica": strconv.Itoa(cfg.Replica)}
	hist := func(name, help string, buckets []float64, lbls ...string) *prometheus.HistogramVec {
		h := prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: metricPrefix + name, Help: help, ConstLabels: labels, Buckets: buckets,
			NativeHistogramBucketFactor: 1.1,
		}, lbls)
		f(h)
		return h
	}
	counter := func(name, help string, lbls ...string) *prometheus.CounterVec {
		c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: metricPrefix + name, Help: help, ConstLabels: labels}, lbls)
		f(c)
		return c
	}
	gauge := func(name, help string) prometheus.Gauge {
		g := prometheus.NewGauge(prometheus.GaugeOpts{Name: metricPrefix + name, Help: help, ConstLabels: labels})
		f(g)
		return g
	}

	m := &metrics{reg: reg}
	m.requests = counter("requests_total", "Requests handled, by request type and HTTP status code.", "type", "code")
	m.reqDuration = hist("request_duration_seconds", "Request latency from arrival to response, including time queued for a concurrency slot.", latencyBuckets, "type")
	m.reqErrors = counter("request_errors_total", "Failed requests, by the cause of the failure.", "cause")
	m.inflight = gauge("requests_inflight", "Requests holding a concurrency slot.")
	m.queued = gauge("requests_queued", "Requests waiting for a concurrency slot.")
	m.queueWait = hist("queue_wait_seconds", "Time requests waited for a concurrency slot.", latencyBuckets).WithLabelValues()
	m.cpuSeconds = counter("cpu_burned_seconds_total", "CPU time spent on configured per-request work (cpu_ms).").WithLabelValues()
	m.calls = counter("calls_total", "Outbound call attempts, by target, op and outcome (ok or error).", "target", "op", "outcome")
	m.callDuration = hist("call_duration_seconds", "Outbound call attempt latency, including the wait for a pool connection.", latencyBuckets, "target", "op")
	m.callErrors = counter("call_errors_total", "Failed outbound call attempts, by cause.", "target", "op", "cause")
	m.retries = counter("call_retries_total", "Retries of outbound calls.", "target", "op")
	m.cacheLookups = counter("cache_lookups_total", "Cache gets, by result (hit or miss).", "target", "result")
	m.poolWait = hist("pool_wait_seconds", "Time spent waiting for a connection from a target's pool (postgres and http targets).", latencyBuckets, "target")
	m.poolHold = hist("pool_hold_seconds", "How long a caller held each connection from a target's pool, from acquiring it to releasing it: one call, or a whole request with hold request (postgres targets).", latencyBuckets, "target")
	m.connects = hist("pool_connect_seconds", "New connections a target's pool opened, by outcome (ok or error), and how long each took to open: TCP, startup and authentication (postgres targets).", latencyBuckets, "target", "outcome")
	m.asyncDropped = counter("async_calls_failed_total", "Async calls that failed; they never fail the request.").WithLabelValues()
	m.readAfterWrite = counter("read_after_write_total", "Point reads of a key the same request had written, by target and result: fresh (the read saw the write) or stale (it didn't, as when a replica hasn't replayed it yet).", "target", "result")
	m.coalesced = counter("cache_coalesced_total", "Cache misses that waited for another request's on_miss calls for the same key instead of running their own (coalesce).", "target")

	if cfg.Role == RoleWorker {
		m.jobs = counter("jobs_total", "Jobs a worker finished, by queue and result: ok, or error when one of its calls failed (the job is dropped).", "queue", "result")
		m.jobErrors = counter("job_errors_total", "Jobs that failed, by queue and the cause of the failed call.", "queue", "cause")
		m.jobReadErrors = counter("job_read_errors_total", "Failed attempts to take a job from the queue, or to acknowledge one, by cause.", "queue", "cause")
		m.jobWait = hist("job_wait_seconds", "Time from a job's publish to a worker taking it: its wait in the queue.", jobWaitBuckets, "queue")
		m.jobDuration = hist("job_duration_seconds", "Time a worker spent running each job, its calls included.", latencyBuckets, "queue")
		m.jobsRunning = gauge("jobs_running", "Jobs a worker replica is running now.")
	}
	f(&poolCollector{labels: labels, snapshots: pools})
	return m
}

// poolSnapshot is a target pool's state at scrape time.
type poolSnapshot struct {
	target           string
	max, inUse, idle int64
	pending          int64
	// waits and waitSeconds are cumulative; set for redis, whose client
	// reports totals instead of per-acquire timings.
	waits       *int64
	waitSeconds *float64
	open        bool // circuit breaker state
	hasBreaker  bool
}

type poolCollector struct {
	labels    prometheus.Labels
	snapshots func() []poolSnapshot
}

var (
	poolConnsDesc   = desc("pool_connections", "Connections in a target's pool, by state (in_use or idle).", "target", "state")
	poolMaxDesc     = desc("pool_max_connections", "Configured pool size for a target.", "target")
	poolPendingDesc = desc("pool_pending", "Callers waiting for a connection from a target's pool.", "target")
	poolWaitsDesc   = desc("pool_waits_total", "Times a caller had to wait for a pool connection (redis targets).", "target")
	poolWaitSecDesc = desc("pool_wait_seconds_total", "Total time callers waited for pool connections (redis targets).", "target")
	circuitDesc     = desc("circuit_open", "1 while the circuit breaker for a target is open or half-open.", "target")
)

func desc(name, help string, labels ...string) *prometheus.Desc {
	return prometheus.NewDesc(metricPrefix+name, help, append([]string{"service", "replica"}, labels...), nil)
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{poolConnsDesc, poolMaxDesc, poolPendingDesc, poolWaitsDesc, poolWaitSecDesc, circuitDesc} {
		ch <- d
	}
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	svc, rep := c.labels["service"], c.labels["replica"]
	for _, s := range c.snapshots() {
		ch <- prometheus.MustNewConstMetric(poolConnsDesc, prometheus.GaugeValue, float64(s.inUse), svc, rep, s.target, "in_use")
		ch <- prometheus.MustNewConstMetric(poolConnsDesc, prometheus.GaugeValue, float64(s.idle), svc, rep, s.target, "idle")
		ch <- prometheus.MustNewConstMetric(poolMaxDesc, prometheus.GaugeValue, float64(s.max), svc, rep, s.target)
		ch <- prometheus.MustNewConstMetric(poolPendingDesc, prometheus.GaugeValue, float64(s.pending), svc, rep, s.target)
		if s.waits != nil {
			ch <- prometheus.MustNewConstMetric(poolWaitsDesc, prometheus.CounterValue, float64(*s.waits), svc, rep, s.target)
			ch <- prometheus.MustNewConstMetric(poolWaitSecDesc, prometheus.CounterValue, *s.waitSeconds, svc, rep, s.target)
		}
		if s.hasBreaker {
			v := 0.0
			if s.open {
				v = 1
			}
			ch <- prometheus.MustNewConstMetric(circuitDesc, prometheus.GaugeValue, v, svc, rep, s.target)
		}
	}
}
