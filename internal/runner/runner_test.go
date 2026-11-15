package runner

import (
	"fmt"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/rundir"
	"github.com/akinfemi/load-lore/internal/scenario"
)

// fakeDriver records calls. Load lasts loadFor unless ctx ends first.
type fakeDriver struct {
	mu       sync.Mutex
	calls    []string
	upErr    error
	loadFor  time.Duration
	injected []string
	undone   []string
	speeds   []int // speed indexes pre-flight and then each Speed call measure

	sample   *driver.Sample // what Sample returns; nil: an empty sample
	metric   func(service, metric string) (float64, bool)
	scaled   []string // Scale calls, "service=n"
	stopOnce sync.Once
	stopCh   chan struct{} // closed by StopLoad
}

func (f *fakeDriver) stopped() chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopCh == nil {
		f.stopCh = make(chan struct{})
	}
	return f.stopCh
}

func (f *fakeDriver) StopLoad(context.Context) error {
	f.log("stop")
	ch := f.stopped()
	f.stopOnce.Do(func() { close(ch) })
	return nil
}

func (f *fakeDriver) log(s string) { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }

func (f *fakeDriver) Preflight(context.Context, *plan.Plan) (*driver.Preflight, error) {
	f.log("preflight")
	pf := &driver.Preflight{Host: driver.Host{Driver: "fake", CPUs: 64}}
	if len(f.speeds) > 0 {
		pf.Host.Speed = &bench.Speed{Score: bench.Score{Index: f.speeds[0]}}
		f.speeds = f.speeds[1:]
	}
	return pf, nil
}

func (f *fakeDriver) Speed(context.Context, *plan.Plan) (*bench.Speed, error) {
	f.log("speed")
	if len(f.speeds) == 0 {
		return nil, errors.New("no more speeds")
	}
	sp := &bench.Speed{Score: bench.Score{Index: f.speeds[0]}}
	f.speeds = f.speeds[1:]
	return sp, nil
}

func (f *fakeDriver) Up(_ context.Context, _ *plan.Plan, dir *rundir.Dir, _ func(string)) error {
	f.log("up")
	_ = dir.Write("compose.yaml", []byte("fake"))
	return f.upErr
}

func (f *fakeDriver) RunLoad(ctx context.Context, started func(time.Time)) (*driver.LoadResult, error) {
	f.log("load")
	started(time.Now())
	select {
	case <-time.After(f.loadFor):
		return &driver.LoadResult{Requests: 10}, nil
	case <-f.stopped():
		return &driver.LoadResult{Requests: 5, ExitCode: driver.K6Stopped}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *fakeDriver) Inject(_ context.Context, fl *plan.Fault) (func(context.Context) error, error) {
	f.mu.Lock()
	f.injected = append(f.injected, fl.Inject)
	f.mu.Unlock()
	return func(context.Context) error {
		f.mu.Lock()
		f.undone = append(f.undone, fl.Inject)
		f.mu.Unlock()
		return nil
	}, nil
}

func (f *fakeDriver) Metric(_ context.Context, service, metric string, _ time.Duration) (float64, bool, error) {
	if f.metric == nil {
		return 0, false, nil
	}
	v, ok := f.metric(service, metric)
	return v, ok, nil
}

func (f *fakeDriver) Scale(_ context.Context, service string, n int, ready func(string, time.Duration)) error {
	f.mu.Lock()
	f.scaled = append(f.scaled, fmt.Sprintf("%s=%d", service, n))
	f.mu.Unlock()
	return nil
}

func (f *fakeDriver) Sample(context.Context, time.Duration) (*driver.Sample, error) {
	if f.sample != nil {
		return f.sample, nil
	}
	return &driver.Sample{}, nil
}

func (f *fakeDriver) Collect(_ context.Context, dir *rundir.Dir, _, _ time.Time) error {
	f.log("collect")
	return dir.Write("metrics/x.json", []byte("{}"))
}

func (f *fakeDriver) Down(context.Context) error { f.log("down"); return nil }

const designSrc = `
version: 1
name: fake
services:
  api: { image: loadlore/synthetic, behavior: { calls: [ { target: db, op: read } ] } }
  db: { kind: postgres, dataset: { rows: 100 } }
workload:
  entry: api
  profile: [ { hold: { rps: 10, for: 1s } } ]
scenario:
  - { at: 0s, inject: latency, target: db, add_ms: 5, for: 100ms }
  - { at: 0s, inject: pause, target: db, for: 10m }
`

func init() { scrapeSettle = 0 }

func readStatus(t *testing.T, dir string) Status {
	t.Helper()
	var st Status
	b, err := os.ReadFile(filepath.Join(dir, "status.json"))
	if err != nil || json.Unmarshal(b, &st) != nil {
		t.Fatalf("status.json: %v %s", err, b)
	}
	return st
}

func runFake(t *testing.T, f *fakeDriver, ctx context.Context, opt Options) (*Result, error) {
	t.Helper()
	return runFakeDesign(t, f, ctx, opt, designSrc)
}

func runFakeDesign(t *testing.T, f *fakeDriver, ctx context.Context, opt Options, src string) (*Result, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "design.yaml")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	opt.DesignPath = path
	opt.Root = filepath.Join(t.TempDir(), "runs")
	return Run(ctx, opt, func(context.Context) (driver.Driver, error) { return f, nil })
}

func TestCompletedRunTearsDownAndRecords(t *testing.T) {
	f := &fakeDriver{loadFor: 300 * time.Millisecond}
	res, err := runFake(t, f, context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls, ","); got != "preflight,up,load,collect,down" {
		t.Errorf("calls %s", got)
	}
	if res.Status != StatusCompleted || res.LoadStarted == nil || res.LoadEnded == nil {
		t.Errorf("result %+v", res)
	}
	if st := readStatus(t, res.Dir); st.Phase != PhaseFinished || st.Status != StatusCompleted || st.Verdict != "no_data" || st.PID != os.Getpid() || st.LoadStarted == nil || st.ProfileS != 1 {
		t.Errorf("status %+v", st)
	}
	// The short fault ended on time; the long one was ended with the load.
	if len(f.injected) != 2 || len(f.undone) != 2 || f.undone[0] != "latency" {
		t.Errorf("injected %v, undone %v", f.injected, f.undone)
	}
	for _, name := range []string{"design.yaml", "plan.json", "run.json", "events.jsonl", "compose.yaml", "metrics/x.json", "report.md", "analysis.json"} {
		if _, err := os.Stat(filepath.Join(res.Dir, name)); err != nil {
			t.Errorf("missing %s", name)
		}
	}
	b, _ := os.ReadFile(filepath.Join(res.Dir, "events.jsonl"))
	for _, kind := range []string{"run_start", "load_start", "fault_start", "fault_end", "load_end", "teardown", "run_end"} {
		if !strings.Contains(string(b), `"kind":"`+kind+`"`) {
			t.Errorf("events.jsonl lacks %s", kind)
		}
	}
	var rj Result
	b, _ = os.ReadFile(filepath.Join(res.Dir, "run.json"))
	// The fake driver collects no metrics, so the report can't judge it.
	if err := json.Unmarshal(b, &rj); err != nil || rj.Status != StatusCompleted || rj.Verdict != "no_data" || rj.Report == "" {
		t.Errorf("run.json: %v %s", err, b)
	}
}

func TestFailedUpStillTearsDown(t *testing.T) {
	f := &fakeDriver{upErr: errors.New("boom")}
	res, err := runFake(t, f, context.Background(), Options{})
	if err == nil || res.Status != StatusFailed || !strings.Contains(res.Error, "boom") {
		t.Fatalf("err %v, result %+v", err, res)
	}
	if got := strings.Join(f.calls, ","); got != "preflight,up,collect,down" {
		t.Errorf("calls %s", got)
	}
	if st := readStatus(t, res.Dir); st.Phase != PhaseFinished || st.Status != StatusFailed || !strings.Contains(st.Error, "boom") {
		t.Errorf("status %+v", st)
	}
}

func TestInterruptedRunTearsDown(t *testing.T) {
	f := &fakeDriver{loadFor: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	res, err := runFake(t, f, ctx, Options{})
	if err == nil || res.Status != StatusInterrupted {
		t.Fatalf("err %v, status %s", err, res.Status)
	}
	if !strings.HasSuffix(strings.Join(f.calls, ","), "down") {
		t.Errorf("calls %v", f.calls)
	}
}

func TestTimedOutRunTearsDown(t *testing.T) {
	f := &fakeDriver{loadFor: time.Hour}
	res, err := runFake(t, f, context.Background(), Options{Timeout: 200 * time.Millisecond})
	if err == nil || res.Status != StatusTimedOut {
		t.Fatalf("err %v, status %s", err, res.Status)
	}
	if !strings.HasSuffix(strings.Join(f.calls, ","), "down") {
		t.Errorf("calls %v", f.calls)
	}
}

func TestKeepSkipsTeardown(t *testing.T) {
	f := &fakeDriver{loadFor: 10 * time.Millisecond}
	res, err := runFake(t, f, context.Background(), Options{Keep: true})
	if err != nil || !res.Kept {
		t.Fatal(err, res)
	}
	for _, c := range f.calls {
		if c == "down" {
			t.Error("--keep still tore down")
		}
	}
}

func TestInvalidDesignCreatesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "design.yaml")
	if err := os.WriteFile(path, []byte("version: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "runs")
	_, err := Run(context.Background(), Options{DesignPath: path, Root: root}, func(context.Context) (driver.Driver, error) {
		t.Fatal("driver created for an invalid design")
		return nil, nil
	})
	var inv *InvalidDesignError
	if !errors.As(err, &inv) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Error("a run folder was created for an invalid design")
	}
}

func TestCoolDownWaitsForTheFirstRunsSpeed(t *testing.T) {
	defer func(d time.Duration) { coolEvery = d }(coolEvery)
	coolEvery = time.Millisecond

	// 874 and 940 are more than 5% below 1000; 960 is within it.
	f := &fakeDriver{loadFor: time.Millisecond, speeds: []int{874, 940, 960}}
	res, err := runFake(t, f, context.Background(), Options{CoolTo: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls, ","); got != "preflight,speed,speed,up,load,collect,down" {
		t.Errorf("calls %s", got)
	}
	c := res.Host.Cooled
	if c == nil || !c.Reached || c.First != 874 || c.Target != 1000 || res.Host.Speed.Index != 960 {
		t.Errorf("cooled %+v, speed %+v", c, res.Host.Speed)
	}

	// A faster host, or no series, doesn't wait.
	f = &fakeDriver{loadFor: time.Millisecond, speeds: []int{1100}}
	res, _ = runFake(t, f, context.Background(), Options{CoolTo: 1000})
	if strings.Contains(strings.Join(f.calls, ","), "speed") || res.Host.Cooled == nil || res.Host.Cooled.Waited != 0 {
		t.Errorf("fast host: calls %v, cooled %+v", f.calls, res.Host.Cooled)
	}
	f = &fakeDriver{loadFor: time.Millisecond, speeds: []int{500}}
	if res, _ = runFake(t, f, context.Background(), Options{}); res.Host.Cooled != nil {
		t.Errorf("no series: cooled %+v", res.Host.Cooled)
	}
}

func TestScenarioHostWarnings(t *testing.T) {
	sc := &scenario.Scenario{Name: "s", Meta: scenario.Meta{Host: scenario.Host{CPUs: 8, Memory: 8 << 30, Reference: "ref machine", Speed: 1000}}}
	host := driver.Host{CPUs: 8, MemoryBytes: 10 << 30, Speed: &bench.Speed{Score: bench.Score{Index: 800}}}
	if w := scenarioHostWarnings(sc, 1, host); len(w) != 0 {
		t.Errorf("fast enough: %v", w)
	}
	host.Speed.Index = 300
	w := scenarioHostWarnings(sc, 1, host)
	if len(w) != 1 || !strings.Contains(w[0], "speed index is 300, against 1000") || !strings.Contains(w[0], "ref machine") {
		t.Errorf("slow: %v", w)
	}
	host.Speed, host.CPUs = nil, 4
	if w := scenarioHostWarnings(sc, 1, host); len(w) != 1 || !strings.Contains(w[0], "try --scale 0.5") {
		t.Errorf("small: %v", w)
	}
}

func TestRepeatRunsASeries(t *testing.T) {
	defer func(d time.Duration) { coolEvery = d }(coolEvery)
	coolEvery = time.Millisecond

	path := filepath.Join(t.TempDir(), "design.yaml")
	if err := os.WriteFile(path, []byte(designSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	// Run 2 starts hot (900 against 1000) and cools to 980; run 3 doesn't wait.
	f := &fakeDriver{loadFor: time.Millisecond, speeds: []int{1000, 900, 980, 990}}
	opt := Options{DesignPath: path, Root: filepath.Join(base, "runs")}
	sr, err := Repeat(context.Background(), opt, 3, func(context.Context) (driver.Driver, error) { return f, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(sr.Runs) != 3 || len(sr.Series.Runs) != 3 || filepath.Dir(sr.Dir) != filepath.Join(base, "series") {
		t.Fatalf("series %+v", sr)
	}
	seed := sr.Runs[0].Seed
	for i, r := range sr.Runs {
		if r.Seed == 0 || r.Seed != seed || r.Series == nil || r.Series.Index != i+1 || r.Series.Of != 3 {
			t.Errorf("run %d: seed %d, series %+v", i+1, r.Seed, r.Series)
		}
	}
	if c := sr.Runs[1].Host.Cooled; c == nil || !c.Reached || c.First != 900 || c.Target != 1000 {
		t.Errorf("run 2 cooled %+v", c)
	}
	if c := sr.Runs[2].Host.Cooled; c == nil || c.Waited != 0 {
		t.Errorf("run 3 cooled %+v", c)
	}
	for _, name := range []string{"series.json", "series.md"} {
		if _, err := os.Stat(filepath.Join(sr.Dir, name)); err != nil {
			t.Error(err)
		}
	}
	// The fake runs have no metrics, so none is conclusive.
	if len(sr.Series.Warnings) == 0 || !strings.Contains(sr.Series.Warnings[0], "left out of the spreads") {
		t.Errorf("warnings %v", sr.Series.Warnings)
	}
}

// A long profile whose load keeps failing: the live check confirms the
// break and stops the load EarlyStopAfter later, unless the run is --full.
const longDesignSrc = `
version: 1
name: fake-long
services:
  api: { image: loadlore/synthetic, behavior: { calls: [ { target: db, op: read } ] } }
  db: { kind: postgres, dataset: { rows: 100 } }
workload:
  entry: api
  profile: [ { hold: { rps: 10, for: 10m } } ]
`

func failingSample() *driver.Sample {
	served, errs := 5.0, 0.5
	return &driver.Sample{Offered: 10, Served: &served, Errors: &errs}
}

func TestBrokenRunStopsEarly(t *testing.T) {
	defer func(p, a time.Duration) { progressEvery, EarlyStopAfter = p, a }(progressEvery, EarlyStopAfter)
	progressEvery, EarlyStopAfter = 20*time.Millisecond, 50*time.Millisecond
	f := &fakeDriver{loadFor: 10 * time.Second, sample: failingSample()}
	start := time.Now()
	res, err := runFakeDesign(t, f, context.Background(), Options{}, longDesignSrc)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("the load wasn't stopped: the run took %s", time.Since(start))
	}
	if res.Status != StatusCompleted || res.StoppedEarly == nil || res.Load == nil || res.Load.ExitCode != driver.K6Stopped {
		t.Fatalf("result %+v", res)
	}
	if !slices.Contains(f.calls, "stop") {
		t.Errorf("calls %v", f.calls)
	}
	if st := readStatus(t, res.Dir); st.BreakConfirmedS == nil || st.StopsAtS == nil || st.Sample == nil || st.Phase != PhaseFinished {
		t.Errorf("status %+v", st)
	}
	b, _ := os.ReadFile(filepath.Join(res.Dir, "events.jsonl"))
	if !strings.Contains(string(b), `"kind":"load_stop"`) {
		t.Error("events.jsonl lacks load_stop")
	}
	var rj Result
	b, _ = os.ReadFile(filepath.Join(res.Dir, "run.json"))
	if err := json.Unmarshal(b, &rj); err != nil || rj.StoppedEarly == nil {
		t.Errorf("run.json: %v %s", err, b)
	}
}

func TestFullRunDoesntStop(t *testing.T) {
	defer func(p, a time.Duration) { progressEvery, EarlyStopAfter = p, a }(progressEvery, EarlyStopAfter)
	progressEvery, EarlyStopAfter = 20*time.Millisecond, 50*time.Millisecond
	f := &fakeDriver{loadFor: 400 * time.Millisecond, sample: failingSample()}
	res, err := runFakeDesign(t, f, context.Background(), Options{Full: true}, longDesignSrc)
	if err != nil {
		t.Fatal(err)
	}
	if res.StoppedEarly != nil || slices.Contains(f.calls, "stop") {
		t.Errorf("a --full run stopped early: %+v, calls %v", res.StoppedEarly, f.calls)
	}
}
