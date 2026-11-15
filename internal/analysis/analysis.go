// Package analysis turns a run folder into findings: per-window numbers, a
// verdict, diagnosis rules that fired with their evidence, and scored
// predictions. It reads only the run folder (never a live stack), and every
// number it produces comes from the exported metrics or the run's own
// records. Missing data stays missing: fields are null, never estimated.
package analysis

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/hoststat"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/rundir"
)

// WindowSeconds is the length of the windows percentiles and rates are
// computed over, as the expect grammar defines.
const WindowSeconds = 10

// MinWindowShare is how much of the last window the load must cover for
// it to count.
const MinWindowShare = 0.9

// Analysis is everything a report says about one run.
type Analysis struct {
	RunID     string     `json:"run_id"`
	Name      string     `json:"name"`
	Dir       string     `json:"dir"`
	Status    string     `json:"status"`
	Error     string     `json:"error,omitempty"`
	Scale     float64    `json:"scale"`
	Seed      int64      `json:"seed"`
	ReuseData bool       `json:"reuse_data"`
	Started   *time.Time `json:"load_started,omitempty"`
	Ended     *time.Time `json:"load_ended,omitempty"`

	// PeakVUs is the most load generator VUs busy at once before the break,
	// or in the whole run if it held: what a scenario's prealloc_vus is
	// measured from.
	PeakVUs *float64 `json:"peak_vus,omitempty"`
	// PeakVUsAll is the most busy at once in the whole run, break included:
	// what a scenario that judges recovery after a break needs.
	PeakVUsAll *float64 `json:"peak_vus_all,omitempty"`

	WindowSeconds int       `json:"window_seconds"`
	PeakOffered   float64   `json:"peak_offered_rps"`
	Windows       []*Window `json:"windows"`
	// Analyzed is how many windows the verdict, rules and predictions use:
	// all of them, unless one of Loadlore's own containers died, which ends
	// what the run can say about the system.
	Analyzed int    `json:"analyzed_windows"`
	Cutoff   string `json:"cutoff,omitempty"`
	// StoppedEarly is set when the run stopped the load after a break
	// instead of finishing the profile.
	StoppedEarly *EarlyStop `json:"stopped_early,omitempty"`
	// AcceptStalls are where Toxiproxy held up new connections on an
	// edge (see accepts.go), over the whole load.
	AcceptStalls []AcceptStall `json:"accept_stalls,omitempty"`

	Verdict     Verdict        `json:"verdict"`
	Diagnoses   []*Diagnosis   `json:"diagnoses"`
	Expect      []ExpectResult `json:"expect"`
	Assumptions Assumptions    `json:"assumptions"`
	Timeline    []Event        `json:"timeline"`
	Environment Environment    `json:"environment"`
	DataReuse   []DataReuse    `json:"data_reuse,omitempty"`
	Notes       []string       `json:"notes"`    // the plan's notes: scaling and budgets
	Warnings    []string       `json:"warnings"` // from the run and from analysis

	// SeriesRef places the run in a run --repeat series, and Series is that
	// series as written when the run was analyzed; nil for a single run.
	SeriesRef *SeriesRef `json:"series_ref,omitempty"`
	Series    *Series    `json:"series,omitempty"`

	// For report rendering.
	Design     *design.Design `json:"-"`
	Plan       *plan.Plan     `json:"-"`
	DesignYAML string         `json:"-"`
}

// Window holds one 10-second window's numbers. Nil means no data.
type Window struct {
	Index   int     `json:"index"`
	Start   float64 `json:"start_s"` // seconds from the start of the load
	Offered float64 `json:"offered_rps"`

	// End to end, from the load generator.
	RPS       *float64 `json:"rps"`           // successful responses per second
	Completed *float64 `json:"completed_rps"` // all responses, failed included
	Dropped   *float64 `json:"dropped"`       // arrivals the load generator couldn't start
	// VUs is the most load generator VUs busy with a request at once.
	VUs       *float64 `json:"vus,omitempty"`
	ErrorRate *float64 `json:"error_rate"`
	P50       *float64 `json:"p50_ms"`
	P95       *float64 `json:"p95_ms"`
	P99       *float64 `json:"p99_ms"`
	MeanMs    *float64 `json:"mean_ms"`

	Services map[string]*ServiceWindow `json:"services"`
	Pools    []*PoolWindow             `json:"pools"`
	Edges    []*EdgeWindow             `json:"edges"`

	// Loadlore's own containers, as a fraction of their CPU budgets, and
	// the share of CFS periods in which each hit its quota and was stalled.
	LoadCPU        *float64 `json:"load_generator_cpu"`
	ProxyCPU       *float64 `json:"proxy_cpu"`
	LoadThrottled  *float64 `json:"load_generator_throttled_periods"`
	ProxyThrottled *float64 `json:"proxy_throttled_periods"`

	// The kernel Docker runs on, when loadl sampled its /proc/stat: the
	// share of CPU time the hypervisor gave to other virtual machines
	// (steal), and the CPUs used by everything except this run's
	// containers.
	HostSteal  *float64 `json:"host_steal,omitempty"`
	HostOthers *float64 `json:"host_other_cpus,omitempty"`
}

// ServiceWindow is one design service in one window.
type ServiceWindow struct {
	CPU       *float64 `json:"cpu"`             // mean use as a fraction of the CPU limit
	Throttled *float64 `json:"cpu_throttled_s"` // CFS throttled time, summed over replicas
	Memory    *float64 `json:"memory"`          // worst replica's peak working set / its limit
	Replicas  int      `json:"replicas_reporting"`

	// Application services (synthetic) and lb.
	RPS          *float64 `json:"rps,omitempty"`
	ErrorRate    *float64 `json:"error_rate,omitempty"`
	P99          *float64 `json:"p99_ms,omitempty"`
	MeanMs       *float64 `json:"mean_ms,omitempty"`
	Inflight     *float64 `json:"inflight,omitempty"`      // peak, summed over replicas
	InflightMean *float64 `json:"inflight_mean,omitempty"` // mean, summed over replicas
	// ErrorCauses counts failed requests by cause (synthetic.Cause*).
	ErrorCauses map[string]float64 `json:"error_causes,omitempty"`

	// Postgres.
	Connections    *float64 `json:"connections,omitempty"`
	MaxConnections *float64 `json:"max_connections,omitempty"`

	// Postgres: the share of block reads shared_buffers answered, and the
	// blocks per second it read from outside them (the OS cache or disk).
	BufferHitRate *float64 `json:"buffer_hit_rate,omitempty"`
	BlockReads    *float64 `json:"block_reads,omitempty"`
	// Postgres with track_io_timing on (a design's disk block turns it on):
	// the mean milliseconds a block read from outside shared_buffers took.
	BlockReadMs *float64 `json:"block_read_ms,omitempty"`
	// Postgres: backends waiting on a heavyweight lock, such as a row lock,
	// on average over the window's scrapes.
	LockWaits *float64 `json:"lock_waits,omitempty"`
	// Postgres read replicas: how far behind the primary it was, at most
	// over the window's scrapes. ReplicationLagS is time since the last
	// transaction it replayed, while it had WAL left to replay (0 when
	// caught up); ReplicationLagBytes is the WAL the primary had written
	// that the replica hadn't replayed.
	ReplicationLagS     *float64 `json:"replication_lag_s,omitempty"`
	ReplicationLagBytes *float64 `json:"replication_lag_bytes,omitempty"`

	// Redis.
	HitRate   *float64 `json:"hit_rate,omitempty"`
	Evictions *float64 `json:"evictions,omitempty"`

	// lb: replicas in rotation (the minimum seen in the window).
	Healthy *float64 `json:"healthy_replicas,omitempty"`
}

// PoolWindow is one caller's connection pool to one target, summed over the
// caller's replicas.
type PoolWindow struct {
	Service string   `json:"service"`
	Target  string   `json:"target"`
	InUse   *float64 `json:"in_use"`  // peak
	Max     *float64 `json:"max"`     // pool size
	Pending *float64 `json:"pending"` // peak callers waiting for a connection
	WaitMs  *float64 `json:"wait_ms"` // mean wait per acquire
	// HeldMs is the mean time per call after getting a connection: call
	// time minus pool wait. It's the target's own latency, network
	// included, without the queue for a connection in front of it.
	HeldMs *float64 `json:"held_ms"`
	// Open is the most connections open at once (in use or idle) and
	// OpenEnd how many were open at the window's end. Opened is how many
	// new connections the pools opened, and ConnectMs the mean time each
	// took to open, from the connect histogram; runs from before it have
	// only the open counts.
	Open      *float64 `json:"open"`
	OpenEnd   *float64 `json:"open_end"`
	Opened    *float64 `json:"opened,omitempty"`
	ConnectMs *float64 `json:"connect_ms,omitempty"`
	// HoldMs is the mean time a caller kept a connection, from acquiring
	// it to releasing it: about HeldMs when it holds one for each call,
	// and the request's time from its first call when it holds one per
	// request.
	HoldMs *float64 `json:"hold_ms,omitempty"`
}

// EdgeWindow is the traffic on one connection between components.
type EdgeWindow struct {
	From      string   `json:"from"` // service, or "load"
	To        string   `json:"to"`
	RPS       *float64 `json:"rps"`
	ErrorRate *float64 `json:"error_rate"`
	P99       *float64 `json:"p99_ms"`
	P99Source string   `json:"p99_source,omitempty"`
	// ErrorCauses counts failed call attempts by cause, retries included.
	ErrorCauses map[string]float64 `json:"error_causes,omitempty"`
	// Retries is retried call attempts per second (included in RPS).
	Retries *float64 `json:"retries,omitempty"`
	// OpMeanMs is the mean time per call by op (read, write, get, ...),
	// pool wait included.
	OpMeanMs map[string]float64 `json:"op_mean_ms,omitempty"`
}

// Assumptions are the design's own and the ones Loadlore made for the run.
type Assumptions struct {
	Design   []*design.Assumption `json:"design"`
	Loadlore []plan.Assumption    `json:"loadlore"`
}

// Event is something that happened during the load, from events.jsonl.
type Event struct {
	At     float64 `json:"at_s"` // seconds from the start of the load
	Kind   string  `json:"kind"`
	Detail string  `json:"detail"`
}

// Environment is where and how the run happened.
type Environment struct {
	Host     *driver.Host       `json:"host,omitempty"`
	Loadl    string             `json:"loadl_version"`
	Images   map[string]string  `json:"images"` // container -> image
	Load     *driver.LoadResult `json:"load,omitempty"`
	Duration string             `json:"profile"`
}

// DataReuse records that a run reused data earlier runs wrote to.
type DataReuse struct {
	Service     string   `json:"service"`
	EarlierRuns []string `json:"earlier_runs"`
}

// runRecord is what analysis needs from run.json.
type runRecord struct {
	RunID        string             `json:"run_id"`
	Status       string             `json:"status"`
	Error        string             `json:"error"`
	Scale        float64            `json:"scale"`
	Seed         int64              `json:"seed"`
	ReuseData    bool               `json:"reuse_data"`
	LoadStarted  *time.Time         `json:"load_started"`
	LoadEnded    *time.Time         `json:"load_ended"`
	Host         *driver.Host       `json:"host"`
	Warnings     []string           `json:"warnings"`
	Notes        []string           `json:"notes"`
	Load         *driver.LoadResult `json:"load"`
	Loadl        string             `json:"loadl_version"`
	Series       *SeriesRef         `json:"series"`
	StoppedEarly *EarlyStop         `json:"stopped_early"`
}

// Analyze reads a run folder and analyzes it.
func Analyze(dir string) (*Analysis, error) {
	var rec runRecord
	if err := readJSON(filepath.Join(dir, "run.json"), &rec); err != nil {
		return nil, fmt.Errorf("%s isn't a finished run folder: %w", dir, err)
	}
	var p plan.Plan
	if err := readJSON(filepath.Join(dir, "plan.json"), &p); err != nil {
		return nil, fmt.Errorf("reading plan.json: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "design.yaml"))
	if err != nil {
		return nil, err
	}
	d, diags := design.Load(raw)
	if diags.HasErrors() {
		return nil, fmt.Errorf("the run's design.yaml no longer validates with this version of loadl: %v", diags.Errors())
	}

	a := &Analysis{
		RunID: rec.RunID, Name: p.Name, Dir: dir, Status: rec.Status, Error: rec.Error,
		Scale: rec.Scale, Seed: rec.Seed, ReuseData: rec.ReuseData, Started: rec.LoadStarted, Ended: rec.LoadEnded,
		WindowSeconds: WindowSeconds, PeakOffered: p.Workload.PeakRate,
		Notes: orEmpty(p.Notes), Warnings: orEmpty(slices.Clone(rec.Warnings)),
		Design: d, Plan: &p, DesignYAML: string(raw),
		Diagnoses: []*Diagnosis{}, Windows: []*Window{}, StoppedEarly: rec.StoppedEarly,
	}
	a.Assumptions = Assumptions{Design: d.Assumptions, Loadlore: p.Assumptions}
	if a.Assumptions.Design == nil {
		a.Assumptions.Design = []*design.Assumption{}
	}
	if a.Assumptions.Loadlore == nil {
		a.Assumptions.Loadlore = []plan.Assumption{}
	}
	a.SeriesRef = rec.Series
	if a.Series, err = loadSeries(rec.Series); err != nil {
		a.Warnings = append(a.Warnings, "the run's series: "+err.Error())
	}
	a.Environment = Environment{Host: rec.Host, Loadl: rec.Loadl, Load: rec.Load, Duration: p.Duration.String(), Images: images(dir)}

	events, err := readEvents(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		a.Warnings = append(a.Warnings, "events.jsonl: "+err.Error())
	}
	if rec.LoadStarted == nil {
		a.Verdict = Verdict{Kind: VerdictNoData, Summary: "The load never started, so there is nothing to analyze."}
		if rec.Error != "" {
			a.Verdict.Summary += " The run ended with: " + rec.Error + "."
		}
		a.Expect = scoreAll(a, nil)
		return a, nil
	}
	t0 := float64(rec.LoadStarted.UnixMilli()) / 1000
	a.Timeline, a.DataReuse = timeline(events, t0, "loadlore-"+rec.RunID+"-")

	end := p.Duration.Seconds()
	if rec.LoadEnded != nil {
		end = math.Min(end, rec.LoadEnded.Sub(*rec.LoadStarted).Seconds())
	}
	// A last window that is nearly complete counts, over the seconds it
	// covers: k6 runs on Docker's clock, which in a VM can run a little
	// fast, so a load can end a second short of its profile.
	n := int(math.Floor(end/WindowSeconds + 1 - MinWindowShare + 1e-9))
	st := newStore(filepath.Join(dir, "metrics"))
	b := &builder{st: st, p: &p, d: d, t0: t0, end: end}
	for i := range n {
		a.Windows = append(a.Windows, b.window(i))
	}
	if !st.has("k6_http_reqs_total") {
		a.Warnings = append(a.Warnings, "no load generator metrics were exported (k6_http_reqs_total), so end-to-end throughput, latency and errors are missing")
	}

	// The lock-wait query rides on an exporter flag marked deprecated; say
	// so when it stops reporting, rather than letting hot rows go quiet.
	if st.has("pg_stat_database_blks_read") && !st.has("pg_lock_waits_waiting") {
		a.Warnings = append(a.Warnings, "postgres_exporter didn't report pg_lock_waits_waiting (Loadlore's lock-wait query, loaded with --extend.query-path), so hot rows couldn't be checked")
	}

	a.Analyzed = len(a.Windows)
	if at, what, ok := toolDeath(a.Timeline); ok && int(at/WindowSeconds) < a.Analyzed {
		a.Analyzed = int(at / WindowSeconds)
		a.Cutoff = fmt.Sprintf("%s at %s, which broke the run's tooling, so windows from %s on aren't analyzed", what, Clock(at), Clock(float64(a.Analyzed*WindowSeconds)))
		a.Warnings = append(a.Warnings, "Analysis stops at "+Clock(float64(a.Analyzed*WindowSeconds))+": "+a.Cutoff+".")
	}
	if a.AcceptStalls, err = acceptStalls(dir, t0); err != nil {
		a.Warnings = append(a.Warnings, "Toxiproxy's log: "+err.Error())
	}
	all := a.Windows
	a.Windows = all[:a.Analyzed]
	a.Verdict = verdict(a)
	a.PeakVUs = peakVUs(a)
	for _, w := range a.Windows {
		a.PeakVUsAll = maxPtr(a.PeakVUsAll, w.VUs)
	}
	a.Diagnoses = diagnose(a)
	a.Expect = scoreAll(a, a.Windows)
	a.Windows = all
	return a, nil
}

var toolNames = map[string]string{
	"lore-toxiproxy": "Toxiproxy, which carries every connection between services,", "lore-k6": "the load generator (k6)",
	"lore-prometheus": "Prometheus", "lore-cadvisor": "cAdvisor",
}

// toolDeath finds the first of Loadlore's own containers to be OOM-killed
// or to exit with an error during the load.
func toolDeath(tl []Event) (at float64, what string, ok bool) {
	for _, e := range tl {
		name, action, found := strings.Cut(e.Detail, " ")
		if e.Kind != "container" || !found || !strings.HasPrefix(name, "lore-") {
			continue
		}
		who := toolNames[name]
		if who == "" {
			who = name
		}
		switch {
		case action == "oom":
			return e.At, who + " was killed for running out of memory", true
		case strings.HasPrefix(action, "die"):
			return e.At, who + " exited" + strings.TrimPrefix(action, "die"), true
		}
	}
	return 0, "", false
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func images(dir string) map[string]string {
	var cs []struct {
		Name  string `json:"name"`
		Image string `json:"image"`
	}
	out := map[string]string{}
	if readJSON(filepath.Join(dir, "containers.json"), &cs) == nil {
		for _, c := range cs {
			out[c.Name] = c.Image
		}
	}
	return out
}

func readEvents(path string) ([]rundir.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []rundir.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e rundir.Event
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// timeline keeps what happened during the load: faults and container
// lifecycle changes. Data reuse is reported separately.
func timeline(events []rundir.Event, t0 float64, project string) ([]Event, []DataReuse) {
	out := []Event{}
	var reuse []DataReuse
	for _, e := range events {
		at := float64(e.Time.UnixMilli())/1000 - t0
		str := func(k string) string { s, _ := e.Fields[k].(string); return s }
		switch e.Kind {
		case "data_reuse":
			dr := DataReuse{Service: str("service"), EarlierRuns: []string{}}
			if xs, ok := e.Fields["earlier_runs"].([]any); ok {
				for _, x := range xs {
					if s, ok := x.(string); ok {
						dr.EarlierRuns = append(dr.EarlierRuns, s)
					}
				}
			}
			reuse = append(reuse, dr)
		case "fault_start", "fault_end", "fault_error":
			detail := str("detail")
			if detail == "" {
				detail = str("inject") + " on " + str("target")
			}
			if msg := str("error"); msg != "" {
				detail += ": " + msg
			}
			out = append(out, Event{At: at, Kind: e.Kind, Detail: detail})
		case "load_stop":
			num := func(k string) float64 { f, _ := e.Fields[k].(float64); return f }
			out = append(out, Event{At: at, Kind: "load_stop", Detail: fmt.Sprintf("the live check confirmed the break at %s; the rest of the profile didn't run", Clock(num("confirmed_at_s")))})
		case "docker":
			action := str("action")
			if at < -1 || action == "start" && at < 1 {
				continue // the stack starting up
			}
			// Loadlore's own containers matter only when they fail.
			if strings.HasPrefix(str("service"), "lore-") && action != "oom" && (action != "die" || str("exit_code") == "0") {
				continue
			}
			name := strings.TrimSuffix(strings.TrimPrefix(str("container"), project), "-1")
			detail := name + " " + action
			if code := str("exit_code"); code != "" && action == "die" {
				detail += " (exit code " + code + ")"
			}
			out = append(out, Event{At: at, Kind: "container", Detail: detail})
		}
	}
	return out, reuse
}

// --- per-window numbers ---

type builder struct {
	st  *store
	p   *plan.Plan
	d   *design.Design
	t0  float64
	end float64 // seconds of load
}

func num(v float64, ok bool) *float64 {
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

func (b *builder) window(i int) *Window {
	start := float64(i * WindowSeconds)
	wa, wb := b.t0+start, b.t0+math.Min(start+WindowSeconds, b.end)
	w := &Window{Index: i, Start: start, Services: map[string]*ServiceWindow{}, Pools: []*PoolWindow{}, Edges: []*EdgeWindow{}}
	w.Offered = b.offered(start)
	st := b.st

	if total, ok := st.increase("k6_http_reqs_total", nil, wa, wb); ok {
		good, _ := st.increase("k6_http_reqs_total", eq("expected_response", "true"), wa, wb)
		w.Completed = num(total/(wb-wa), true)
		w.RPS = num(good/(wb-wa), true)
		w.ErrorRate = num((total-good)/total, total > 0)
		dropped, _ := st.increase("k6_dropped_iterations_total", nil, wa, wb)
		w.Dropped = num(dropped, true)
		if mx, _, ok := st.gaugeSum("k6_vus", nil, wa, wb); ok {
			w.VUs = num(mx, true)
		}
	}
	bs, sum, count := st.nativeBuckets("k6_http_req_duration_seconds", nil, wa, wb)
	w.P50, w.P95, w.P99 = ms(quantile(bs, 0.5)), ms(quantile(bs, 0.95)), ms(quantile(bs, 0.99))
	w.MeanMs = num(sum/count*1000, count > 0)
	if b.p.Workload.Entry != "" {
		w.Edges = append(w.Edges, &EdgeWindow{From: "load", To: b.p.Workload.Entry, RPS: w.Completed, ErrorRate: w.ErrorRate, P99: w.P99, P99Source: SrcK6Latency})
	}

	for _, s := range b.p.Services {
		w.Services[s.Name] = b.service(s, wa, wb)
	}
	for _, s := range b.p.Services {
		switch {
		case s.Kind == "app" && len(s.Synthetic) > 0:
			b.appEdges(w, s, wa, wb)
		case s.Kind == "pgbouncer":
			b.pgbouncerPool(w, s, wa, wb)
		case s.Kind == "lb":
			sw := w.Services[s.Name]
			target := w.Services[s.LB.Target]
			e := &EdgeWindow{From: s.Name, To: s.LB.Target, RPS: sw.RPS, ErrorRate: sw.ErrorRate}
			if target != nil && target.P99 != nil {
				e.P99, e.P99Source = target.P99, SrcAppLatency
			}
			w.Edges = append(w.Edges, e)
		}
	}
	w.LoadCPU = b.toolCPU("lore-k6", b.p.Infra.LoadCPU, wa, wb)
	w.ProxyCPU = b.toolCPU(plan.ProxyHost, b.p.Infra.ProxyCPU, wa, wb)
	w.LoadThrottled = b.throttledPeriods("lore-k6", wa, wb)
	w.ProxyThrottled = b.throttledPeriods(plan.ProxyHost, wa, wb)
	b.host(w, wa, wb)
	return w
}

// host fills the window's steal and other programs' CPU from the host
// samples, when there are any.
func (b *builder) host(w *Window, wa, wb float64) {
	modes := map[string]float64{}
	total := 0.0
	for _, m := range hoststat.Modes {
		v, ok := b.st.increase(hoststat.Metric, eq("mode", m), wa, wb)
		if !ok {
			return
		}
		modes[m] = v
		total += v
	}
	if total <= 0 {
		return
	}
	w.HostSteal = num(modes["steal"]/total, true)
	// irq and softirq time isn't charged to containers, so it isn't
	// counted on either side.
	busy := modes["user"] + modes["nice"] + modes["system"]
	ours := 0.0
	for _, ss := range b.st.match("container_cpu_usage_seconds_total", not("service", "")) {
		if inc, ok := b.st.increaseOne(ss, wa, wb); ok {
			ours += inc
		}
	}
	w.HostOthers = num(math.Max(0, busy-ours)/(wb-wa), true)
}

func ms(v float64, ok bool) *float64 { return num(v*1000, ok) }

// offered is the profile's mean arrival rate over a window.
func (b *builder) offered(start float64) float64 {
	const steps = 40
	total := 0.0
	for k := range steps {
		t := start + (float64(k)+0.5)*WindowSeconds/steps
		total += b.p.Workload.OfferedRate(time.Duration(t * float64(time.Second)))
	}
	return total / steps
}

func roleOf(kind string) string {
	switch kind {
	case "postgres", "redis":
		return "datastore"
	}
	return kind // app, lb, pgbouncer
}

// cpu is a set of containers' mean CPU use over the window as a fraction of
// their limits, counting only containers that reported in the window (a
// killed replica stops counting).
func (b *builder) cpu(m matcher, limit float64, wa, wb float64) (util, throttled *float64, replicas int) {
	used := 0.0
	for _, ss := range b.st.match("container_cpu_usage_seconds_total", m) {
		if inc, ok := b.st.increaseOne(ss, wa, wb); ok {
			used += inc
			replicas++
		}
	}
	if replicas == 0 || limit <= 0 {
		return nil, nil, 0
	}
	util = num(used/(wb-wa)/(limit*float64(replicas)), true)
	if th, ok := b.st.increase("container_cpu_cfs_throttled_seconds_total", m, wa, wb); ok {
		throttled = num(th, true)
	}
	return util, throttled, replicas
}

// throttledPeriods is the share of a container's CFS periods in which it
// used up its quota and was stalled until the next period. A 10 s average
// CPU can sit well under the limit while bursts are throttled.
func (b *builder) throttledPeriods(service string, wa, wb float64) *float64 {
	periods, ok := b.st.increase("container_cpu_cfs_periods_total", eq("service", service), wa, wb)
	if !ok || periods <= 0 {
		return nil
	}
	th, ok := b.st.increase("container_cpu_cfs_throttled_periods_total", eq("service", service), wa, wb)
	if !ok {
		return nil
	}
	return num(th/periods, true)
}

func (b *builder) toolCPU(service string, budget, wa, wb float64) *float64 {
	u, _, _ := b.cpu(eq("service", service), budget, wa, wb)
	return u
}

func (b *builder) service(s *plan.Service, wa, wb float64) *ServiceWindow {
	st := b.st
	sw := &ServiceWindow{}
	m := eq("service", s.Name, "role", roleOf(s.Kind))
	sw.CPU, sw.Throttled, sw.Replicas = b.cpu(m, s.CPU, wa, wb)
	if mem := st.gaugeEach("container_memory_working_set_bytes", m, "name", wa, wb); len(mem) > 0 && s.Memory > 0 {
		worst := 0.0
		for _, v := range mem {
			worst = math.Max(worst, v)
		}
		sw.Memory = num(worst/float64(s.Memory), true)
	}
	svc := eq("service", s.Name)
	switch s.Kind {
	case "app":
		if total, ok := st.increase("loadlore_requests_total", svc, wa, wb); ok {
			bad, _ := st.increase("loadlore_requests_total", and(svc, not("code", "200")), wa, wb)
			sw.RPS = num(total/(wb-wa), true)
			sw.ErrorRate = num(bad/total, total > 0)
		}
		sw.P99 = ms(quantile(st.classicBuckets("loadlore_request_duration_seconds", svc, wa, wb), 0.99))
		sum, ok1 := st.increase("loadlore_request_duration_seconds_sum", svc, wa, wb)
		cnt, ok2 := st.increase("loadlore_request_duration_seconds_count", svc, wa, wb)
		sw.MeanMs = num(sum/cnt*1000, ok1 && ok2 && cnt > 0)
		if mx, mean, ok := st.gaugeSum("loadlore_requests_inflight", svc, wa, wb); ok {
			sw.Inflight, sw.InflightMean = num(mx, true), num(mean, true)
		}
		if c := st.increaseBy("loadlore_request_errors_total", svc, "cause", wa, wb); len(c) > 0 {
			sw.ErrorCauses = c
		}
	case "postgres":
		if mx, _, ok := st.gaugeSum("pg_stat_activity_count", svc, wa, wb); ok {
			sw.Connections = num(mx, true)
		}
		if mx, _, ok := st.gaugeSum("pg_settings_max_connections", svc, wa, wb); ok {
			sw.MaxConnections = num(mx, true)
		}
		lab := and(svc, eq("datname", plan.DBName))
		read, ok1 := st.increase("pg_stat_database_blks_read", lab, wa, wb)
		hit, ok2 := st.increase("pg_stat_database_blks_hit", lab, wa, wb)
		if ok1 && ok2 {
			sw.BlockReads = num(read/(wb-wa), true)
			sw.BufferHitRate = num(hit/(hit+read), hit+read > 0)
			// blk_read_time stays 0 with track_io_timing off.
			if ms, ok := st.increase("pg_stat_database_blk_read_time", lab, wa, wb); ok && ms > 0 && read > 0 {
				sw.BlockReadMs = num(ms/read, true)
			}
		}
		if _, mean, ok := st.gaugeSum("pg_lock_waits_waiting", svc, wa, wb); ok {
			sw.LockWaits = num(mean, true)
		}
		if s.Primary != "" {
			if mx, _, ok := st.gaugeSum("pg_replication_lag_seconds", svc, wa, wb); ok {
				sw.ReplicationLagS = num(mx, true)
			}
			if mx, _, ok := st.gaugeSum("pg_stat_replication_pg_wal_lsn_diff", eq("service", s.Primary, "application_name", s.Name), wa, wb); ok {
				sw.ReplicationLagBytes = num(mx, true)
			}
		}
	case "pgbouncer":
		db := and(svc, eq("database", plan.DBName))
		active, _, ok1 := st.gaugeSum("pgbouncer_pools_client_active_connections", db, wa, wb)
		waiting, _, ok2 := st.gaugeSum("pgbouncer_pools_client_waiting_connections", db, wa, wb)
		if ok1 && ok2 {
			sw.Connections = num(active+waiting, true)
		}
		if mx, _, ok := st.gaugeSum("pgbouncer_config_max_client_connections", svc, wa, wb); ok {
			sw.MaxConnections = num(mx, true)
		}
		if n, ok := st.increase("pgbouncer_stats_totals_sql_transactions_pooled_total", db, wa, wb); ok {
			sw.RPS = num(n/(wb-wa), true)
		}
	case "redis":
		hits, ok1 := st.increase("redis_keyspace_hits_total", svc, wa, wb)
		misses, ok2 := st.increase("redis_keyspace_misses_total", svc, wa, wb)
		sw.HitRate = num(hits/(hits+misses), ok1 && ok2 && hits+misses > 0)
		if ev, ok := st.increase("redis_evicted_keys_total", svc, wa, wb); ok {
			sw.Evictions = num(ev, true)
		}
	case "lb":
		be := and(svc, eq("proxy", "replicas"))
		if total, ok := st.increase("haproxy_backend_http_requests_total", be, wa, wb); ok {
			sw.RPS = num(total/(wb-wa), true)
			resp, _ := st.increase("haproxy_backend_http_responses_total", be, wa, wb)
			bad, _ := st.increase("haproxy_backend_http_responses_total", and(be, eq("code", "5xx")), wa, wb)
			sw.ErrorRate = num(bad/resp, resp > 0)
		}
		if v, ok := st.gaugeMin("haproxy_backend_active_servers", be, wa, wb); ok {
			sw.Healthy = num(v, true)
		}
	}
	return sw
}

func (b *builder) appEdges(w *Window, s *plan.Service, wa, wb float64) {
	st := b.st
	cfg := s.Synthetic[0]
	targets := make([]string, 0, len(cfg.Targets))
	for t := range cfg.Targets {
		targets = append(targets, t)
	}
	slices.Sort(targets)
	for _, t := range targets {
		tgt := cfg.Targets[t]
		m := eq("service", s.Name, "target", t)
		e := &EdgeWindow{From: s.Name, To: t, P99Source: SrcCallLatency}
		if total, ok := st.increase("loadlore_calls_total", m, wa, wb); ok {
			bad, _ := st.increase("loadlore_calls_total", and(m, eq("outcome", "error")), wa, wb)
			e.RPS = num(total/(wb-wa), true)
			e.ErrorRate = num(bad/total, total > 0)
		}
		e.P99 = ms(quantile(st.classicBuckets("loadlore_call_duration_seconds", m, wa, wb), 0.99))
		if c := st.increaseBy("loadlore_call_errors_total", m, "cause", wa, wb); len(c) > 0 {
			e.ErrorCauses = c
		}
		if n, ok := st.increase("loadlore_call_retries_total", m, wa, wb); ok {
			e.Retries = num(n/(wb-wa), true)
		}
		sums := st.increaseBy("loadlore_call_duration_seconds_sum", m, "op", wa, wb)
		cnts := st.increaseBy("loadlore_call_duration_seconds_count", m, "op", wa, wb)
		for op, c := range cnts {
			if c > 0 {
				if e.OpMeanMs == nil {
					e.OpMeanMs = map[string]float64{}
				}
				e.OpMeanMs[op] = sums[op] / c * 1000
			}
		}
		w.Edges = append(w.Edges, e)

		if tgt.Pool <= 0 || !st.has("loadlore_pool_max_connections") {
			continue
		}
		pw := &PoolWindow{Service: s.Name, Target: t}
		if mx, _, ok := st.gaugeSum("loadlore_pool_connections", and(m, eq("state", "in_use")), wa, wb); ok {
			pw.InUse = num(mx, true)
		}
		if mx, _, ok := st.gaugeSum("loadlore_pool_max_connections", m, wa, wb); ok {
			pw.Max = num(mx, true)
		}
		if open := st.gaugeGrid("loadlore_pool_connections", m, wa, wb); len(open) > 0 {
			pw.Open, pw.OpenEnd = num(slices.Max(open), true), num(open[len(open)-1], true)
		}
		// The services are first scraped at about the load's start, so in
		// the first window a pool's connections count from that scrape:
		// the ones a minimum opened before the load don't count.
		inc := st.increase
		if w.Index == 0 {
			inc = st.increaseFromFirst
		}
		if n, ok := inc("loadlore_pool_connect_seconds_count", m, wa, wb); ok {
			pw.Opened = num(n, true)
			if secs, ok := inc("loadlore_pool_connect_seconds_sum", m, wa, wb); ok && n > 0 {
				pw.ConnectMs = num(secs/n*1000, true)
			}
		}
		if mx, _, ok := st.gaugeSum("loadlore_pool_pending", m, wa, wb); ok {
			pw.Pending = num(mx, true)
		}
		sum, ok1 := st.increase("loadlore_pool_wait_seconds_sum", m, wa, wb)
		cnt, ok2 := st.increase("loadlore_pool_wait_seconds_count", m, wa, wb)
		pw.WaitMs = num(sum/cnt*1000, ok1 && ok2 && cnt > 0)
		if !ok1 { // redis pools report total waits instead of per-acquire timings
			sum, ok1 = st.increase("loadlore_pool_wait_seconds_total", m, wa, wb)
			cnt, ok2 = st.increase("loadlore_pool_waits_total", m, wa, wb)
			pw.WaitMs = num(sum/cnt*1000, ok1 && ok2 && cnt > 0)
		}
		if ok1 && ok2 {
			csum, ok3 := st.increase("loadlore_call_duration_seconds_sum", m, wa, wb)
			ccnt, ok4 := st.increase("loadlore_call_duration_seconds_count", m, wa, wb)
			pw.HeldMs = num(math.Max(0, csum-sum)/ccnt*1000, ok3 && ok4 && ccnt > 0)
		}
		hsum, ok5 := st.increase("loadlore_pool_hold_seconds_sum", m, wa, wb)
		hcnt, ok6 := st.increase("loadlore_pool_hold_seconds_count", m, wa, wb)
		pw.HoldMs = num(hsum/hcnt*1000, ok5 && ok6 && hcnt > 0)
		if pw.Max != nil {
			w.Pools = append(w.Pools, pw)
		}
	}
}

// pgbouncerPool reads a PgBouncer's server connections to its Postgres as
// a pool: in use against default_pool_size plus reserve_pool, clients
// waiting for one, the mean wait per transaction and the time each held a
// server connection. The transactions it forwards are its edge to the
// Postgres.
func (b *builder) pgbouncerPool(w *Window, s *plan.Service, wa, wb float64) {
	st := b.st
	svc := eq("service", s.Name)
	db, dbName := and(svc, eq("database", plan.DBName)), and(svc, eq("name", plan.DBName))
	target := s.PgBouncer.Target
	e := &EdgeWindow{From: s.Name, To: target}
	if sw := w.Services[s.Name]; sw != nil {
		e.RPS = sw.RPS
	}
	w.Edges = append(w.Edges, e)

	pw := &PoolWindow{Service: s.Name, Target: target}
	if mx, _, ok := st.gaugeSum("pgbouncer_pools_server_active_connections", db, wa, wb); ok {
		pw.InUse = num(mx, true)
	}
	if size, _, ok := st.gaugeSum("pgbouncer_databases_pool_size", dbName, wa, wb); ok {
		reserve, _, _ := st.gaugeSum("pgbouncer_databases_reserve_pool", dbName, wa, wb)
		pw.Max = num(size+reserve, true)
	}
	if mx, _, ok := st.gaugeSum("pgbouncer_pools_client_waiting_connections", db, wa, wb); ok {
		pw.Pending = num(mx, true)
	}
	if open := st.gaugeGrid("pgbouncer_databases_current_connections", dbName, wa, wb); len(open) > 0 {
		pw.Open, pw.OpenEnd = num(slices.Max(open), true), num(open[len(open)-1], true)
	}
	// PgBouncer is first scraped at about the load's start; in the first
	// window its counters count from that scrape, not from zero.
	inc := st.increase
	if w.Index == 0 {
		inc = st.increaseFromFirst
	}
	assigned, ok1 := inc("pgbouncer_stats_totals_server_assignments_total", db, wa, wb)
	waited, ok2 := inc("pgbouncer_stats_totals_client_wait_seconds_total", db, wa, wb)
	pw.WaitMs = num(waited/assigned*1000, ok1 && ok2 && assigned > 0)
	txs, ok3 := inc("pgbouncer_stats_totals_sql_transactions_pooled_total", db, wa, wb)
	inTx, ok4 := inc("pgbouncer_stats_totals_server_in_transaction_seconds_total", db, wa, wb)
	pw.HeldMs = num(inTx/txs*1000, ok3 && ok4 && txs > 0)
	if pw.Max != nil {
		w.Pools = append(w.Pools, pw)
	}
}

// Metric sources, cited wherever a number appears.
const (
	SrcOffered          = "the load profile in plan.json (offered arrival rate)"
	SrcRPS              = `k6_http_reqs_total{expected_response="true"} (k6: successful responses)`
	SrcCompleted        = "k6_http_reqs_total (k6: all responses)"
	SrcErrors           = `k6_http_reqs_total{expected_response="false"} ÷ k6_http_reqs_total (k6)`
	SrcDropped          = "k6_dropped_iterations_total (k6: arrivals it couldn't start)"
	SrcK6Latency        = "k6_http_req_duration_seconds (k6 histogram, end to end)"
	SrcCPU              = "container_cpu_usage_seconds_total (cAdvisor) ÷ the container's CPU limit"
	SrcThrottled        = "container_cpu_cfs_throttled_seconds_total (cAdvisor)"
	SrcMemory           = "container_memory_working_set_bytes (cAdvisor) ÷ the container's memory limit"
	SrcAppLatency       = "loadlore_request_duration_seconds (the service's own histogram)"
	SrcAppErrors        = `loadlore_requests_total{code!="200"} ÷ loadlore_requests_total`
	SrcInflight         = "loadlore_requests_inflight (summed over replicas)"
	SrcCallLatency      = "loadlore_call_duration_seconds (the caller's histogram, pool wait included)"
	SrcCalls            = "loadlore_calls_total (the caller's outbound call attempts)"
	SrcPoolInUse        = `loadlore_pool_connections{state="in_use"} and loadlore_pool_max_connections`
	SrcPoolPending      = "loadlore_pool_pending (callers waiting for a connection)"
	SrcPoolWait         = "loadlore_pool_wait_seconds (time to get a connection)"
	SrcPgbPool          = `pgbouncer_pools_server_active_connections against pgbouncer_databases_pool_size plus reserve_pool (PgBouncer's server connections)`
	SrcPgbWaiting       = "pgbouncer_pools_client_waiting_connections (clients waiting in PgBouncer for a server connection)"
	SrcPgbWait          = "pgbouncer_stats_totals_client_wait_seconds_total ÷ server_assignments_total (PgBouncer)"
	SrcPgbHeld          = "pgbouncer_stats_totals_server_in_transaction_seconds_total ÷ sql_transactions_pooled_total (PgBouncer)"
	SrcPgbClients       = "pgbouncer_pools_client_{active,waiting}_connections and pgbouncer_config_max_client_connections"
	SrcPoolHold         = "loadlore_pool_hold_seconds (how long callers kept each connection)"
	SrcPoolOpen         = "loadlore_pool_connections (in use and idle, summed over replicas)"
	SrcPoolConnect      = "loadlore_pool_connect_seconds (new connections the pools opened, and how long each took)"
	SrcPoolHeld         = "loadlore_call_duration_seconds minus loadlore_pool_wait_seconds, per call (time with a connection)"
	SrcPGConns          = "pg_stat_activity_count and pg_settings_max_connections (postgres_exporter)"
	SrcReplLagS         = "pg_replication_lag_seconds (postgres_exporter, on the replica): time since its last replayed transaction while WAL is left to replay"
	SrcReplLagBytes     = "pg_stat_replication_pg_wal_lsn_diff (postgres_exporter, on the primary): WAL written that the replica hasn't replayed"
	SrcHitRate          = "redis_keyspace_hits_total and redis_keyspace_misses_total (redis_exporter)"
	SrcLBRequests       = "haproxy_backend_http_requests_total (HAProxy)"
	SrcLBHealthy        = "haproxy_backend_active_servers (HAProxy)"
	SrcLoadCPU          = "container_cpu_usage_seconds_total{service=\"lore-k6\"} ÷ the load generator's CPU budget"
	SrcProxyCPU         = "container_cpu_usage_seconds_total{service=\"lore-toxiproxy\"} ÷ Toxiproxy's CPU budget"
	SrcThrottledPeriods = "container_cpu_cfs_throttled_periods_total ÷ container_cpu_cfs_periods_total (cAdvisor)"
	SrcHostSteal        = "steal time ÷ all CPU time in /proc/stat of the kernel Docker runs on"
	SrcHostOthers       = "user and system time in /proc/stat minus container_cpu_usage_seconds_total of this run's containers (cAdvisor)"
)

// Clock formats seconds from the start of the load as m:ss.
func Clock(s float64) string {
	if s < 0 {
		return "-" + Clock(-s)
	}
	t := int(math.Round(s))
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}

// ErrNotRun is returned for folders without run records.
var ErrNotRun = errors.New("not a run folder")

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func joinAnd(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}

// EarlyStop is when a run's live check stopped the load before the
// profile ended, in seconds from the load's start: the break's first
// window, when the check confirmed it, and when the load stopped.
type EarlyStop struct {
	BreakAt     float64 `json:"break_at_s"`
	ConfirmedAt float64 `json:"confirmed_at_s"`
	At          float64 `json:"at_s"`
}

// peakVUs is the most VUs busy at once in the windows before the break
// (all of them if it held): past the break slow responses pile VUs up,
// and the run is already judged.
func peakVUs(a *Analysis) *float64 {
	ws := a.Windows
	if b := a.Verdict.Break; b != nil {
		ws = ws[:b.Window]
	}
	var peak *float64
	for _, w := range ws {
		peak = maxPtr(peak, w.VUs)
	}
	return peak
}
