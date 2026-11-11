package analysis

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/akinfemi/load-lore/internal/analysis/analysistest"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/synthetic"
)

func analyze(t *testing.T, dir string) *Analysis {
	t.Helper()
	a, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func near(got *float64, want, tol float64) bool { return got != nil && math.Abs(*got-want) <= tol }

func TestHealthyRunHolds(t *testing.T) {
	a := analyze(t, analysistest.WriteRun(t, analysistest.Healthy(), 1))
	if len(a.Windows) != 10 {
		t.Fatalf("%d windows", len(a.Windows))
	}
	w := a.Windows[5] // 50–60 s: offered 550 on average
	if math.Abs(w.Offered-550) > 1 || !near(w.RPS, 550, 6) || !near(w.P99, 5, 0.1) || !near(w.P50, 2, 0.05) {
		t.Errorf("window 5: offered %v rps %v p50 %v p99 %v", w.Offered, deref(w.RPS), deref(w.P50), deref(w.P99))
	}
	if !near(w.Services["db"].CPU, 0.275, 0.01) || !near(w.Pools[0].InUse, 2, 0) || !near(w.Pools[0].Max, 10, 0) {
		t.Errorf("db cpu %v, pool %v/%v", deref(w.Services["db"].CPU), deref(w.Pools[0].InUse), deref(w.Pools[0].Max))
	}
	if a.Verdict.Kind != VerdictHeld || len(a.Diagnoses) != 0 {
		t.Errorf("verdict %s %q, diagnoses %d", a.Verdict.Kind, a.Verdict.Summary, len(a.Diagnoses))
	}
	for _, e := range a.Expect {
		if e.Status != ExpectHeld {
			t.Errorf("%s: %s %s", e.Text, e.Status, e.Message)
		}
	}
}

func TestPoolExhaustion(t *testing.T) {
	w := analysistest.Healthy()
	w.InUse = func(t float64) float64 { return math.Min(10, 2+t/5) } // full from 40 s
	w.Pending = func(t float64) float64 { return math.Max(0, (t-40)*3) }
	w.WaitS = func(t float64) float64 { return math.Max(0.0001, (t-40)/200) }
	w.P99 = func(t float64) float64 { return math.Max(0.005, (t-40)/50) } // up to 1.2 s
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	var pool *Diagnosis
	for _, d := range a.Diagnoses {
		if d.Rule == "pool-exhausted" {
			pool = d
		}
	}
	if pool == nil {
		t.Fatalf("pool rule didn't fire: %+v", a.Diagnoses)
	}
	if pool.From != 40 || !strings.Contains(pool.Next, "1 replica × 10 connections = 10") {
		t.Errorf("from %v, next %q", pool.From, pool.Next)
	}
	text := ""
	for _, e := range pool.Evidence {
		text += e.Text + " | " + e.Source + "\n"
	}
	for _, want := range []string{"10 of 10 connections in use", "requests waiting", "db CPU", "not saturated", "loadlore_pool_pending"} {
		if !strings.Contains(text, want) {
			t.Errorf("evidence lacks %q:\n%s", want, text)
		}
	}
	if a.Verdict.Kind != VerdictBroke || a.Verdict.Break == nil || a.Verdict.Break.Signals[0] != "latency" {
		t.Errorf("verdict %+v", a.Verdict)
	}
	// Pool full, but the database saturated too: not this rule.
	w.DBCPU = func(float64) float64 { return 0.95 }
	a = analyze(t, analysistest.WriteRun(t, w, 1))
	for _, d := range a.Diagnoses {
		if d.Rule == "pool-exhausted" {
			t.Error("fired while the database's CPU was saturated")
		}
	}
}

func TestQueueingKnee(t *testing.T) {
	w := analysistest.Healthy()
	w.DBCPU = func(t float64) float64 { return math.Min(0.99, t/80) } // passes 70% at 56 s
	w.P99 = func(t float64) float64 {
		u := math.Min(0.99, t/80)
		return 0.005 / (1 - u) // M/M/1-like: 5 ms → 500 ms
	}
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	var knee *Diagnosis
	for _, d := range a.Diagnoses {
		if d.Rule == "queueing-knee" {
			knee = d
		}
	}
	if knee == nil {
		t.Fatalf("knee didn't fire; diagnoses %+v, baseline %v", a.Diagnoses, deref(a.Verdict.BaselineP99))
	}
	if !strings.Contains(knee.Title, "db CPU") || knee.From < 50 || knee.From > 80 {
		t.Errorf("knee %s from %v", knee.Title, knee.From)
	}
	if !strings.Contains(knee.Evidence[0].Text, "below 50%") || !strings.Contains(knee.Evidence[0].Text, "90% and up") {
		t.Errorf("bands: %s", knee.Evidence[0].Text)
	}
	// A single slow window isn't a knee, and isn't a break.
	w = analysistest.Healthy()
	w.DBCPU = func(float64) float64 { return 0.8 }
	w.P99 = func(t float64) float64 {
		if t > 50 && t <= 60 {
			return 0.4
		}
		return 0.005
	}
	a = analyze(t, analysistest.WriteRun(t, w, 1))
	if len(a.Diagnoses) != 0 || a.Verdict.Kind != VerdictHeld {
		t.Errorf("one spike: diagnoses %+v, verdict %s", a.Diagnoses, a.Verdict.Kind)
	}
}

func TestInconclusiveWhenTheLoadGeneratorDrops(t *testing.T) {
	w := analysistest.Healthy()
	w.Dropped = func(t float64) float64 { return 0.05 * boolF(t > 30) }
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	if a.Verdict.Kind != VerdictInconclusive || !strings.Contains(a.Verdict.Summary, "load generator dropped") {
		t.Errorf("verdict %s: %s", a.Verdict.Kind, a.Verdict.Summary)
	}
	w = analysistest.Healthy()
	w.K6CPU = func(t float64) float64 { return 0.95 * boolF(t > 70) }
	a = analyze(t, analysistest.WriteRun(t, w, 1))
	if a.Verdict.Kind != VerdictInconclusive || !strings.Contains(a.Verdict.Inconclusive[0], "CPU budget") {
		t.Errorf("k6 at its CPU limit: %s %v", a.Verdict.Kind, a.Verdict.Inconclusive)
	}
}

// A tool can be throttled in bursts while its 10 s average CPU looks fine.
func TestThrottledToolIsInconclusive(t *testing.T) {
	w := analysistest.Healthy()
	w.K6Throttled = func(t float64) float64 { return 0.08 * boolF(t > 40 && t <= 50) }
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	if a.Verdict.Kind != VerdictInconclusive || !strings.Contains(a.Verdict.Inconclusive[0], "at 0:40 the load generator hit its CPU limit and was stalled in 8.0%") {
		t.Errorf("heavy: %s %v", a.Verdict.Kind, a.Verdict.Inconclusive)
	}

	// Light throttling is a caveat, not a reason.
	w.K6Throttled = func(t float64) float64 { return 0.02 * boolF(t > 40 && t <= 50) }
	a = analyze(t, analysistest.WriteRun(t, w, 1))
	if a.Verdict.Kind != VerdictHeld || len(a.Verdict.Caveats) != 1 || !strings.Contains(a.Verdict.Caveats[0], "2.0% of its scheduling periods") {
		t.Errorf("light: %s %v", a.Verdict.Kind, a.Verdict.Caveats)
	}

	// After the break it's a consequence, not a cause.
	w.K6Throttled = func(t float64) float64 { return 0.5 * boolF(t > 80) }
	w.P99 = func(t float64) float64 { return 0.005 + 2*boolF(t > 60) }
	a = analyze(t, analysistest.WriteRun(t, w, 1))
	if a.Verdict.Kind != VerdictBroke || len(a.Verdict.Caveats) != 0 {
		t.Errorf("after the break: %s %v %v", a.Verdict.Kind, a.Verdict.Inconclusive, a.Verdict.Caveats)
	}
}

// A load that ends a second short keeps its last window, with rates over
// the seconds it covers; one that ends well short drops it.
func TestShortLastWindow(t *testing.T) {
	w := analysistest.Healthy()
	w.EndEarly = 900 * time.Millisecond
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	last := a.Windows[len(a.Windows)-1]
	if len(a.Windows) != 10 || last.RPS == nil || *last.RPS < 0.97*last.Offered {
		t.Errorf("%d windows, last served %v of %v offered", len(a.Windows), last.RPS, last.Offered)
	}
	w.EndEarly = 3 * time.Second
	if a := analyze(t, analysistest.WriteRun(t, w, 1)); len(a.Windows) != 9 {
		t.Errorf("%d windows", len(a.Windows))
	}
}

// k6 starting its VUs isn't evidence against the run.
func TestToolWarmupDoesntCount(t *testing.T) {
	w := analysistest.Healthy()
	w.K6Throttled = func(t float64) float64 { return 0.08 * boolF(t <= 10) }
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	if a.Verdict.Kind != VerdictHeld || len(a.Verdict.Caveats) != 0 {
		t.Errorf("%s %v %v", a.Verdict.Kind, a.Verdict.Inconclusive, a.Verdict.Caveats)
	}
}

// The verdict names what took CPU from the tooling only with evidence.
func TestShortCause(t *testing.T) {
	stalled := func(w analysistest.World) analysistest.World {
		w.K6Throttled = func(t float64) float64 { return 0.08 * boolF(t > 40 && t <= 50) }
		return w
	}
	during := func(v float64) func(float64) float64 {
		return func(t float64) float64 { return v * boolF(t > 40 && t <= 50) }
	}
	for _, c := range []struct {
		name  string
		world func(*analysistest.World)
		want  string // "" = no cause
	}{
		{"unknown host", func(*analysistest.World) {}, ""},
		{"in a VM", func(w *analysistest.World) { w.HostCPU = driver.HostCPUInVM }, "possibly took CPU from the run"},
		{"sampled, quiet", func(w *analysistest.World) { w.HostCPU = driver.HostCPUSampled }, ""},
		{"steal", func(w *analysistest.World) {
			w.HostCPU, w.Steal = driver.HostCPUSampled, during(0.05)
		}, "gave 5.0% of this machine's CPU time to other virtual machines at 0:40"},
		{"other programs", func(w *analysistest.World) {
			w.HostCPU, w.Others = driver.HostCPUSampled, during(2)
		}, "Other programs on this machine used 2 CPUs at 0:40"},
		{"other programs, too few", func(w *analysistest.World) {
			w.HostCPU, w.Others = driver.HostCPUSampled, during(0.4)
		}, ""},
	} {
		w := stalled(analysistest.Healthy())
		c.world(&w)
		a := analyze(t, analysistest.WriteRun(t, w, 1))
		if a.Verdict.Kind != VerdictInconclusive {
			t.Errorf("%s: %s", c.name, a.Verdict.Kind)
			continue
		}
		if got := a.Verdict.Cause; (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: cause %q", c.name, got)
		}
		if c.want != "" && !strings.Contains(a.Verdict.Summary, a.Verdict.Cause) {
			t.Errorf("%s: summary lacks the cause: %s", c.name, a.Verdict.Summary)
		}
	}
}

func TestErrorsBreak(t *testing.T) {
	w := analysistest.Healthy()
	w.Failed = func(t float64) float64 { return 0.2 * boolF(t > 70) }
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	b := a.Verdict.Break
	if b == nil || b.At != 70 || b.Signals[0] != "errors" || !strings.Contains(a.Verdict.Summary, "20% of requests failed") || strings.Contains(a.Verdict.Summary, "because") {
		t.Errorf("verdict %+v", a.Verdict)
	}
	if ecs := ErrorCauses(a.Windows); len(ecs) != 0 {
		t.Errorf("no service counted causes, got %+v", ecs)
	}
	for _, e := range a.Expect {
		if e.Text == "error_rate < 1%" && (e.Status != ExpectBroken || e.BrokeAt == nil || *e.BrokeAt != 70) {
			t.Errorf("error_rate line: %+v", e)
		}
	}
}

// The verdict says why requests failed, from the services' own counts,
// and names the target the failing calls went to.
func TestErrorsBreakNamesTheCause(t *testing.T) {
	w := analysistest.Healthy()
	w.Failed = func(t float64) float64 { return 0.2 * boolF(t > 70) }
	w.FailCause = synthetic.CauseTooManyConns
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	want := "succeeded. Of api's failed requests, 100% failed because db refused new connections (Postgres max_connections). Most saturated"
	if !strings.Contains(a.Verdict.Summary, want) {
		t.Errorf("summary %q\nwant it to contain %q", a.Verdict.Summary, want)
	}
	ecs := ErrorCauses(a.Windows)
	if len(ecs) != 1 || ecs[0].Service != "api" || ecs[0].Cause != synthetic.CauseTooManyConns || ecs[0].Target != "db" || ecs[0].First != 70 || ecs[0].Share != 1 || ecs[0].Count < 1 {
		t.Errorf("causes %+v", ecs)
	}
}

func TestCauseMeaning(t *testing.T) {
	for _, tc := range []struct{ cause, target, want string }{
		{"too_many_conns", "db", "db refused new connections (Postgres max_connections)"},
		{"pool_timeout", "db", "they timed out waiting for a pool connection to db"},
		{"timeout", "", "calls ran past their timeout"},
		{"circuit_open", "payments", "the circuit breaker to payments was open"},
		{"bulkhead_full", "payments", "the bulkhead in front of payments was full"},
		{"upstream_error", "", "a dependency answered with error statuses"},
		{"surprise", "db", "of other errors"},
	} {
		if got := causeMeaning(tc.cause, tc.target); got != tc.want {
			t.Errorf("causeMeaning(%q, %q) = %q, want %q", tc.cause, tc.target, got, tc.want)
		}
	}
}

func TestExpectRangesAndScaling(t *testing.T) {
	w := analysistest.Healthy()
	w.P99 = func(t float64) float64 { return 0.005 + 0.1*boolF(t > 60) } // 105 ms after 60 s
	a := analyze(t, analysistest.WriteRun(t, w, 0.5))
	got := map[string]ExpectResult{}
	for _, e := range a.Expect {
		got[e.Text] = e
	}
	// At scale 0.5 the ramp peaks at 500, so load_rps >= 500 (scaled to
	// 250) first holds in the window from 50 s.
	until := got["p99_ms < 50 until load_rps >= 500"]
	if until.Status != ExpectHeld || !strings.Contains(until.Range, "0:50") || !strings.Contains(until.Message, "scaled to 250") {
		t.Errorf("until: %+v", until)
	}
	after := got["db.cpu < 90% after 50s"]
	if after.Status != ExpectHeld || after.Windows != 5 || after.Source != SrcCPU {
		t.Errorf("after: %+v", after)
	}
	rps := got["rps >= 400 after 60s"] // scaled to 200; the scaled ramp averages 325 in the window from 60 s
	if rps.Status != ExpectHeld || !near(rps.Actual, 325, 5) {
		t.Errorf("rps: %+v", rps)
	}
}

func TestMissingDataIsNotScored(t *testing.T) {
	dir := analysistest.WriteRun(t, analysistest.Healthy(), 1)
	if err := os.Remove(filepath.Join(dir, "metrics", "container_cpu_usage_seconds_total.json")); err != nil {
		t.Fatal(err)
	}
	a := analyze(t, dir)
	for _, e := range a.Expect {
		if e.Metric == "db.cpu" && (e.Status != ExpectUnscored || !strings.Contains(e.Message, "no data")) {
			t.Errorf("db.cpu without data: %+v", e)
		}
	}
	if a.Windows[3].Services["db"].CPU != nil {
		t.Error("CPU invented without data")
	}
}

func TestCounterResetsAndBirths(t *testing.T) {
	st := newStore(t.TempDir())
	st.cache["c"] = []*series{
		{points: []point{{0, 10}, {1, 20}, {2, 5}, {3, 15}}}, // reset after t=1
		{points: []point{{2, 7}, {3, 9}}},                    // born at t=2
	}
	if v, ok := st.increase("c", nil, 0, 3); !ok || v != 10+5+10+9 {
		t.Errorf("increase %v", v)
	}
	// Counting from the first sample, the born series adds only its growth.
	if v, ok := st.increaseFromFirst("c", nil, 0, 3); !ok || v != 10+5+10+2 {
		t.Errorf("increase from first %v", v)
	}
}

func TestGaugesFromUnalignedScrapes(t *testing.T) {
	st := newStore(t.TempDir())
	// Two replicas scraped at different moments; the second dies at 4.
	// The first reports a second series only in some scrapes, as
	// postgres_exporter does for connection states.
	r0 := map[string]string{"instance": "r0"}
	st.cache["g"] = []*series{
		{labels: r0, points: []point{{0.2, 10}, {1.2, 10}, {2.2, 10}, {3.2, 10}, {4.2, 10}, {5.2, 10}, {6.2, 10}, {7.2, 10}, {8.2, 10}, {9.2, 10}}},
		{labels: r0, points: []point{{1.2, 100}}},
		{labels: map[string]string{"instance": "r1"}, points: []point{{0.7, 5}, {1.7, 7}, {2.7, 9}, {3.7, 9}}},
	}
	mx, _, ok := st.gaugeSum("g", nil, 3, 6)
	if !ok || mx != 19 {
		t.Errorf("max sum %v, want 19 (both replicas, and the state series only in its scrape)", mx)
	}
	if mn, _ := st.gaugeMin("g", nil, 1, 10); mn != 10 {
		t.Errorf("min sum %v, want 10 after the second stops reporting", mn)
	}
}

func TestQuantile(t *testing.T) {
	bs := []bucket{{0, 1, 50}, {1, 2, 49}, {2, math.Inf(1), 1}}
	if q, _ := quantile(bs, 0.5); q != 1 {
		t.Errorf("p50 %v", q)
	}
	if q, _ := quantile(bs, 0.99); math.Abs(q-2) > 1e-9 {
		t.Errorf("p99 %v", q)
	}
	if q, _ := quantile(bs, 0.999); q != 2 {
		t.Errorf("p99.9 in the open bucket %v, want its lower bound", q)
	}
	if _, ok := quantile(nil, 0.5); ok {
		t.Error("quantile of nothing")
	}
}

func TestNotARun(t *testing.T) {
	if _, err := Analyze(t.TempDir()); err == nil {
		t.Error("analyzed an empty folder")
	}
}

func boolF(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestToolDeathEndsTheAnalysis(t *testing.T) {
	w := analysistest.Healthy()
	w.Failed = func(t float64) float64 { return boolF(t > 70) } // every request fails once the proxy is gone
	w.Events = []analysistest.Event{{At: 71, Kind: "docker", Fields: map[string]any{
		"action": "oom", "container": "loadlore-fixture-run-lore-toxiproxy-1", "service": "lore-toxiproxy"}}}
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	if a.Analyzed != 7 || len(a.Windows) != 10 {
		t.Errorf("analyzed %d of %d windows", a.Analyzed, len(a.Windows))
	}
	if a.Verdict.Kind != VerdictInconclusive || !strings.Contains(a.Verdict.Inconclusive[0], "Toxiproxy") || !strings.Contains(a.Verdict.Inconclusive[0], "running out of memory") {
		t.Errorf("verdict %s: %v", a.Verdict.Kind, a.Verdict.Inconclusive)
	}
	for _, e := range a.Expect {
		if e.Text == "error_rate < 1%" && e.Status != ExpectHeld {
			t.Errorf("errors after the proxy died were scored against the system: %+v", e)
		}
	}
}

// p99 alone needs 3 windows in a row; 2 are a stall in the timeline.
func TestLatencyBreakNeedsThreeWindows(t *testing.T) {
	spike := func(from, to float64) func(float64) float64 {
		return func(t float64) float64 {
			if t >= from && t < to {
				return 0.3
			}
			return 0.005
		}
	}
	w := analysistest.Healthy()
	w.P99 = spike(40, 60)
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	if a.Verdict.Kind != VerdictHeld {
		t.Fatalf("20 s spike: %s %q", a.Verdict.Kind, a.Verdict.Summary)
	}
	var stalls []Event
	for _, e := range a.Timeline {
		if e.Kind == "stall" {
			stalls = append(stalls, e)
		}
	}
	if len(stalls) != 1 || stalls[0].At != 40 || !strings.Contains(stalls[0].Detail, "p99 reached 300 ms (5.0 ms at low load) for 20 s, then recovered") {
		t.Errorf("stalls %+v", stalls)
	}

	w.P99 = spike(40, 70)
	a = analyze(t, analysistest.WriteRun(t, w, 1))
	if b := a.Verdict.Break; a.Verdict.Kind != VerdictBroke || b == nil || b.At != 40 {
		t.Fatalf("30 s spike: %s %+v", a.Verdict.Kind, b)
	}
	for _, e := range a.Timeline {
		if e.Kind == "stall" {
			t.Errorf("a break is also a stall: %+v", e)
		}
	}
}

// A shortfall breaks in 2 windows, and windows from the first shortfall on
// don't count toward the low-load p99.
func TestShortfallBreakAndBaseline(t *testing.T) {
	w := analysistest.Healthy()
	w.Served = func(t, o float64) float64 {
		if t >= 10 {
			return 0.5 * o
		}
		return o
	}
	w.P99 = func(t float64) float64 {
		if t >= 10 {
			return 2
		}
		return 0.005
	}
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	v := a.Verdict
	if v.Kind != VerdictBroke || v.Break == nil || v.Break.At != 10 {
		t.Fatalf("%s %+v", v.Kind, v.Break)
	}
	if !near(v.BaselineP99, 5, 0.1) {
		t.Errorf("baseline %v, want 5 ms from the window before the shortfall", deref(v.BaselineP99))
	}
}

// One shortfall window is a stall, not a break.
func TestOneShortfallWindowIsAStall(t *testing.T) {
	w := analysistest.Healthy()
	w.Served = func(t, o float64) float64 {
		if t >= 50 && t < 60 {
			return 0.5 * o
		}
		return o
	}
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	if a.Verdict.Kind != VerdictHeld {
		t.Fatalf("%s %q", a.Verdict.Kind, a.Verdict.Summary)
	}
	found := false
	for _, e := range a.Timeline {
		found = found || e.Kind == "stall" && e.At == 50 && strings.Contains(e.Detail, "of the offered rate was served for 10 s")
	}
	if !found {
		t.Errorf("timeline %+v", a.Timeline)
	}
}

// Regression, from run single-postgres-breaking-point-one-query-per-page-
// 20261110-084325: the api → db pool was the most saturated resource at
// the knee, but db was the bottleneck (CPU 70–82%, CPU-throttled 10.5 s,
// 6.7 ms per call with a connection against 0.4 ms at low load) while api
// stayed below 73% CPU. The knee is db's; blaming api's pool would point
// at the wrong fix.
func TestKneeFollowsPoolToBusyTarget(t *testing.T) {
	dir := filepath.Join("testdata", "one-query-per-page")
	a := &Analysis{}
	var p plan.Plan
	if err := readJSON(filepath.Join(dir, "plan.json"), &p); err != nil {
		t.Fatal(err)
	}
	if err := readJSON(filepath.Join(dir, "windows.json"), &a.Windows); err != nil {
		t.Fatal(err)
	}
	a.Plan, a.PeakOffered = &p, p.Workload.PeakRate
	a.Verdict = verdict(a)
	if b := a.Verdict.Break; b == nil || b.At != 210 { // 3:30: p99 1.66 s, over the 500 ms default SLO
		t.Fatalf("break %+v", b)
	}
	if want := "Most saturated then: api → db pool 96 of 96 connections in use, waiting on db (CPU 100%, CPU-throttled 45.8 s, 22 ms per call with a connection, 0.4 ms at low load)."; !strings.Contains(a.Verdict.Summary, want) {
		t.Errorf("summary lacks %q:\n%s", want, a.Verdict.Summary)
	}
	d := byRule(t, diagnose(a), "queueing-knee")
	if d.Title != "Queueing knee (db CPU)" || d.From != 110 || !strings.Contains(d.Next, "on db,") {
		t.Errorf("%s from %v, next %q", d.Title, d.From, d.Next)
	}
	text := ""
	for _, e := range d.Evidence {
		text += e.Text + "\n"
	}
	for _, want := range []string{"api → db pool 73 of 96 connections in use at 1:50: a symptom", "db was CPU-throttled for 10.5 s", "6.7 ms each once they had a connection at 1:50, against 0.4 ms at low load"} {
		if !strings.Contains(text, want) {
			t.Errorf("evidence lacks %q:\n%s", want, text)
		}
	}
}

// Regression, from run single-postgres-breaking-point-one-query-per-page-
// 20261111-052813: api CPU (70%) was the most saturated resource when p99
// first stayed high at 3:20, ahead of db (62%), but api's requests spent
// 92% of their time in calls to db, which took 1.6 ms each with a
// connection against 0.2 ms at low load; db reached 91% CPU a window later
// and broke the run. The knee is db's; blaming api's CPU would point at
// the wrong fix.
func TestKneeFollowsCallerToBusyTarget(t *testing.T) {
	dir := filepath.Join("testdata", "caller-waits-on-db")
	a := &Analysis{}
	var p plan.Plan
	if err := readJSON(filepath.Join(dir, "plan.json"), &p); err != nil {
		t.Fatal(err)
	}
	if err := readJSON(filepath.Join(dir, "windows.json"), &a.Windows); err != nil {
		t.Fatal(err)
	}
	a.Plan, a.PeakOffered = &p, p.Workload.PeakRate
	a.Verdict = verdict(a)
	knee := byRule(t, diagnose(a), "queueing-knee")
	if knee.Title != "Queueing knee (db CPU)" || knee.From != 200 || !strings.Contains(knee.Next, "on db,") {
		t.Fatalf("knee %+v", knee)
	}
	text := ""
	for _, e := range knee.Evidence {
		text += e.Text + "\n"
	}
	for _, want := range []string{
		"api CPU at 70% of its limit at 3:20: not where requests queued",
		"api's requests spent 92% of their time in calls to db at 3:20: 1.1 calls per request × 5.3 ms each, against 6.4 ms per request",
		"1.6 ms each once they had a connection at 3:20, against 0.2 ms at low load",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("evidence lacks %q:\n%s", want, text)
		}
	}

	// With db answering at its usual speed, api's CPU is the knee.
	for _, w := range a.Windows {
		for _, pw := range w.Pools {
			if pw.HeldMs != nil {
				v := 0.2
				pw.HeldMs = &v
			}
		}
		if c := w.Services["db"].CPU; c != nil && *c >= PoolTargetCPUMax {
			v := 0.6
			w.Services["db"].CPU = &v
		}
	}
	// (The full pool after the break is then the pool's fault too.)
	var titles []string
	for _, d := range diagnose(a) {
		titles = append(titles, d.Title)
	}
	if !slices.Contains(titles, "Queueing knee (api CPU)") {
		t.Errorf("idle db: %v", titles)
	}
}

func byRule(t *testing.T, ds []*Diagnosis, rule string) *Diagnosis {
	t.Helper()
	var found *Diagnosis
	for _, d := range ds {
		if d.Rule == rule {
			if found != nil {
				t.Fatalf("two %s diagnoses", rule)
			}
			found = d
		}
	}
	if found == nil {
		t.Fatalf("no %s diagnosis", rule)
	}
	return found
}

// Regression, from the same run: at 1:30 api's pools to db went from 25
// to 96 open connections in one window as p99 reached 1.00 s (2.8 ms at
// low load), a stall in the timeline. Runs from before the connect
// histogram have only the open counts.
func TestConnectionStorm(t *testing.T) {
	dir := filepath.Join("testdata", "caller-waits-on-db")
	a := &Analysis{}
	var p plan.Plan
	if err := readJSON(filepath.Join(dir, "plan.json"), &p); err != nil {
		t.Fatal(err)
	}
	if err := readJSON(filepath.Join(dir, "windows.json"), &a.Windows); err != nil {
		t.Fatal(err)
	}
	a.Plan, a.PeakOffered = &p, p.Workload.PeakRate
	a.Verdict = verdict(a)
	d := byRule(t, diagnose(a), "connection-storm")
	if d.Title != "Connection storm (api → db)" || d.From != 90 || d.To != 100 {
		t.Errorf("%s from %v to %v", d.Title, d.From, d.To)
	}
	for _, want := range []string{"At 1:30, api's pools opened 71 connections to db within 10 seconds, at 1,652 rps offered, and p99 reached 1.00 s (2.8 ms at low load)", "pools: { db: { size: 24, min: 24 } }"} {
		if !strings.Contains(d.Summary+d.Next, want) {
			t.Errorf("lacks %q:\n%s\n%s", want, d.Summary, d.Next)
		}
	}
	if e := d.Evidence[0]; e.Text != "71 new connections to db from 1:30 to 1:40: from 25 open to 96 of 96" || e.Source != SrcPoolOpen {
		t.Errorf("evidence %+v", e)
	}

	// Counted by the connect histogram, a warm pool opens none, so no storm.
	for _, w := range a.Windows {
		for _, pw := range w.Pools {
			zero := 0.0
			pw.Opened = &zero
		}
	}
	for _, d := range diagnose(a) {
		if d.Rule == "connection-storm" {
			t.Errorf("warm pool: %s", d.Summary)
		}
	}
}

// A full pool in front of a database whose calls slowed down is the
// database's knee, not pool exhaustion, even with its average CPU at 50%.
// With the same throttling but calls at their usual speed, the pool is to
// blame: the connection pool trap.
func TestFullPoolFollowsTarget(t *testing.T) {
	w := analysistest.Healthy()
	w.DBCPU = func(float64) float64 { return 0.5 }
	w.InUse = func(t float64) float64 { return math.Min(10, 2+t/5) } // full from 40 s
	w.Pending = func(t float64) float64 { return math.Max(0, (t-40)*3) }
	w.WaitS = func(t float64) float64 { return math.Max(0.0001, (t-40)/200) }
	w.P99 = func(t float64) float64 { return math.Max(0.005, (t-40)/50) }
	w.DBThrottled = func(t float64) float64 { return 0.3 } // 3 s per window throughout
	titles := func(w analysistest.World) []string {
		var out []string
		for _, d := range analyze(t, analysistest.WriteRun(t, w, 1)).Diagnoses {
			out = append(out, d.Title)
		}
		return out
	}

	w.HeldS = func(t float64) float64 {
		if t >= 40 {
			return 0.005
		}
		return 0.0005
	}
	if got := titles(w); len(got) != 1 || got[0] != "Queueing knee (db CPU)" {
		t.Errorf("slow db: diagnoses %q", got)
	}

	w.HeldS = func(float64) float64 { return 0.0005 }
	if got := titles(w); len(got) == 0 || got[0] != "Connection pool exhausted (api → db)" || slices.Contains(got, "Queueing knee (db CPU)") {
		t.Errorf("db throttled but answering at its usual speed: diagnoses %q", got)
	}
}

// A break on latency alone while k6 was stalled is cross-checked against
// the service's own p99: when that stays below the limit, the break is
// doubtful, but the verdict stays what it was.
func TestLatencyBreakCrossCheck(t *testing.T) {
	dir := filepath.Join("testdata", "one-query-per-page")
	load := func() *Analysis {
		a := &Analysis{}
		var p plan.Plan
		if err := readJSON(filepath.Join(dir, "plan.json"), &p); err != nil {
			t.Fatal(err)
		}
		if err := readJSON(filepath.Join(dir, "windows.json"), &a.Windows); err != nil {
			t.Fatal(err)
		}
		a.Plan, a.PeakOffered = &p, p.Workload.PeakRate
		return a
	}
	// As measured: k6 stalled 6.4% at 3:30 and api's p99 agreed (1.90 s).
	a := load()
	a.Verdict = verdict(a)
	if a.Verdict.Doubt != "" {
		t.Errorf("api's p99 agreed, yet: %s", a.Verdict.Doubt)
	}
	// The same run if api had answered in 200 ms at 3:40.
	a = load()
	fast := 200.0
	a.Windows[22].Services["api"].P99 = &fast
	a.Verdict = verdict(a)
	if a.Verdict.Kind != VerdictInconclusive || !strings.Contains(a.Verdict.Doubt, "api's own p99 was 200 ms at 3:40") || !strings.Contains(a.Verdict.Summary, "Doubtful: ") {
		t.Errorf("kind %s, doubt %q", a.Verdict.Kind, a.Verdict.Doubt)
	}
}

// pgbWindows builds api → pgb → db: four low-load windows, then three
// with p99 up, api's calls to pgb slow, PgBouncer's server pool full and
// db's transactions slow (dbSlow) or not.
func pgbWindows(dbSlow, apiPoolFull bool) *Analysis {
	p := &plan.Plan{Workload: plan.Workload{Entry: "api", PeakRate: 1000}, Services: []*plan.Service{
		{Name: "api", Kind: "app", CPU: 1, Containers: []string{"api-0"}, Synthetic: []*synthetic.Config{{Targets: map[string]synthetic.Target{"pgb": {Kind: "postgres", Pool: 50}}}}},
		{Name: "pgb", Kind: "pgbouncer", CPU: 0.5, Containers: []string{"pgb"}, PgBouncer: &plan.PgBouncer{Target: "db", PoolMode: "transaction", DefaultPoolSize: 20, MaxClientConn: 200}},
		{Name: "db", Kind: "postgres", CPU: 1, Containers: []string{"db"}},
	}}
	f := func(v float64) *float64 { return &v }
	a := &Analysis{Plan: p, PeakOffered: 1000}
	for i := range 7 {
		hot := i >= 4
		w := &Window{Index: i, Start: float64(i * WindowSeconds), Offered: 100 + float64(i)*150, Services: map[string]*ServiceWindow{}}
		w.RPS, w.Completed, w.ErrorRate, w.Dropped = f(w.Offered), f(w.Offered), f(0), f(0)
		w.P99 = f(3)
		apiHeld, pgbHeld, dbCPU, inUse, apiInUse := 1.0, 0.5, 0.3, 4.0, 4.0
		if hot {
			w.P99, apiHeld, inUse, apiInUse = f(400), 40, 20, 20
			if apiPoolFull {
				inUse, apiInUse = 15, 50
			}
			if dbSlow {
				pgbHeld, dbCPU = 8, 0.75
			}
		}
		w.Services["api"] = &ServiceWindow{CPU: f(0.5), RPS: w.RPS, MeanMs: f(apiHeld + 1)}
		w.Services["pgb"] = &ServiceWindow{CPU: f(0.2)}
		w.Services["db"] = &ServiceWindow{CPU: f(dbCPU)}
		w.Edges = []*EdgeWindow{{From: "api", To: "pgb", RPS: w.RPS}}
		w.Pools = []*PoolWindow{
			{Service: "api", Target: "pgb", InUse: f(apiInUse), Max: f(50), Pending: f(0), WaitMs: f(0), HeldMs: f(apiHeld)},
			{Service: "pgb", Target: "db", InUse: f(inUse), Max: f(20), Pending: f(0), WaitMs: f(0), HeldMs: f(pgbHeld)},
		}
		a.Windows = append(a.Windows, w)
	}
	a.Verdict = verdict(a)
	return a
}

// A knee behind PgBouncer is its Postgres's when that was slow, and
// PgBouncer's server pool's when the Postgres kept up; never PgBouncer's
// CPU, which stayed at 20%.
func TestKneeThroughPgBouncer(t *testing.T) {
	evidence := func(d *Diagnosis) string {
		text := ""
		for _, e := range d.Evidence {
			text += e.Text + "\n"
		}
		return text
	}
	// PgBouncer's full server pool tops the saturation list.
	d := byRule(t, diagnose(pgbWindows(true, false)), "queueing-knee")
	if text := evidence(d); d.Title != "Queueing knee (db CPU)" || !strings.Contains(text, "pgb → db pool 20 of 20 server connections in use at 0:40: a symptom") || !strings.Contains(text, "pgb's transactions to db took 8.0 ms each") {
		t.Errorf("%s:\n%s", d.Title, text)
	}
	// api's pool to pgb tops it, and its calls to pgb were slow: through
	// PgBouncer to db.
	d = byRule(t, diagnose(pgbWindows(true, true)), "queueing-knee")
	if text := evidence(d); d.Title != "Queueing knee (db CPU)" || !strings.Contains(text, "api → pgb pool 50 of 50 connections in use at 0:40: a symptom") || !strings.Contains(text, "pgb's transactions to db took 8.0 ms each") {
		t.Errorf("%s:\n%s", d.Title, text)
	}
	// db slower per transaction with its CPU at 20%: its slowdown, not
	// its CPU.
	a := pgbWindows(true, false)
	for _, w := range a.Windows[4:] {
		v := 0.2
		w.Services["db"].CPU = &v
	}
	d = byRule(t, diagnose(a), "queueing-knee")
	if text := evidence(d); d.Title != "Queueing knee (db, slower without running out of CPU)" || !strings.Contains(text, "db CPU at most 20% of its limit") || !strings.Contains(d.Next, "besides CPU") {
		t.Errorf("%s: %s\n%s", d.Title, d.Next, text)
	}
	d = byRule(t, diagnose(pgbWindows(false, false)), "queueing-knee")
	if d.Title != "Queueing knee (pgb pool to db)" || !strings.Contains(d.Next, "default_pool_size 20") {
		t.Errorf("%s: %s", d.Title, d.Next)
	}
}
