// Package analysistest builds run folders with made-up metrics, for tests
// of analysis and reports.
package analysistest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/rundir"
)

// Design ramps 0→1000 rps over 100 s: one api replica in front of
// one postgres, pool 10.
const Design = `version: 1
name: fixture
services:
  api:
    image: loadlore/synthetic
    resources: { cpu: 1 }
    behavior: { calls: [ { target: db, op: read } ] }
    pools: { db: 10 }
  db:
    kind: postgres
    resources: { cpu: 1 }
    dataset: { rows: 1000 }
workload:
  entry: api
  profile: [ { ramp: { from: 0, to: 1000, over: 100s } } ]
expect:
  - p99_ms < 50 until load_rps >= 500
  - error_rate < 1%
  - db.cpu < 90% after 50s
  - rps >= 400 after 60s
`

// World describes what the fake system did at each second of the load.
type World struct {
	Served   func(t, offered float64) float64 // successful responses/s
	Failed   func(t float64) float64
	Dropped  func(t float64) float64
	P50, P99 func(t float64) float64 // seconds
	APICPU   func(t float64) float64 // fraction of limit
	DBCPU    func(t float64) float64
	K6CPU    func(t float64) float64
	// K6Throttled is the share of k6's CFS periods stalled at its quota;
	// nil leaves the series out.
	K6Throttled func(t float64) float64
	InUse       func(t float64) float64
	Pending     func(t float64) float64
	WaitS       func(t float64) float64 // mean pool wait per acquire

	// Events are written to events.jsonl at their time into the load.
	Events []Event
}

// Event is an events.jsonl line at At seconds into the load.
type Event struct {
	At     float64
	Kind   string
	Fields map[string]any
}

// Healthy is a system that keeps up with the whole ramp.
func Healthy() World {
	return World{
		Served:  func(_, o float64) float64 { return o },
		Failed:  func(float64) float64 { return 0 },
		Dropped: func(float64) float64 { return 0 },
		P50:     func(float64) float64 { return 0.002 },
		P99:     func(float64) float64 { return 0.005 },
		APICPU:  func(t float64) float64 { return 0.4 * t / 100 },
		DBCPU:   func(t float64) float64 { return 0.5 * t / 100 },
		K6CPU:   func(float64) float64 { return 0.2 },
		InUse:   func(float64) float64 { return 2 },
		Pending: func(float64) float64 { return 0 },
		WaitS:   func(float64) float64 { return 0.0001 },
	}
}

type metricFile struct {
	Result []map[string]any `json:"result"`
}

// WriteRun builds a run folder for Design whose metrics follow w.
func WriteRun(t testing.TB, w World, scale float64) string {
	t.Helper()
	dir := t.TempDir()
	d, diags := design.Load([]byte(Design))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	p, err := plan.Build(d, plan.Options{RunID: "fixture-run", Scale: scale})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	end := start.Add(p.Duration)
	rec := map[string]any{
		"run_id": "fixture-run", "status": "completed", "scale": scale, "seed": 1,
		"load_started": start, "load_ended": end, "host": driver.Host{CPUs: 8}, "warnings": []string{}, "loadl_version": "test",
	}
	write := func(name string, v any) {
		b, _ := json.Marshal(v)
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("run.json", rec)
	write("plan.json", p)
	if err := os.WriteFile(filepath.Join(dir, "design.yaml"), []byte(Design), 0o644); err != nil {
		t.Fatal(err)
	}
	var events []byte
	for _, e := range w.Events {
		b, _ := json.Marshal(rundir.Event{Time: start.Add(time.Duration(e.At * float64(time.Second))), Kind: e.Kind, Fields: e.Fields})
		events = append(append(events, b...), '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), events, 0o644); err != nil {
		t.Fatal(err)
	}

	t0 := float64(start.Unix())
	dur := p.Duration.Seconds()
	type counter struct {
		labels map[string]string
		vals   [][2]any
		total  float64
	}
	counters := map[string][]*counter{}
	gauges := map[string][]*counter{}
	newC := func(m map[string][]*counter, name string, labels map[string]string) *counter {
		c := &counter{labels: labels}
		m[name] = append(m[name], c)
		return c
	}
	okReqs := newC(counters, "k6_http_reqs_total", map[string]string{"expected_response": "true"})
	badReqs := newC(counters, "k6_http_reqs_total", map[string]string{"expected_response": "false"})
	drops := newC(counters, "k6_dropped_iterations_total", nil)
	apiCPU := newC(counters, "container_cpu_usage_seconds_total", map[string]string{"service": "api", "role": "app", "name": "api-0"})
	dbCPU := newC(counters, "container_cpu_usage_seconds_total", map[string]string{"service": "db", "role": "datastore", "name": "db"})
	k6CPU := newC(counters, "container_cpu_usage_seconds_total", map[string]string{"service": "lore-k6", "role": "load", "name": "k6"})
	k6Periods := newC(counters, "container_cpu_cfs_periods_total", map[string]string{"service": "lore-k6", "role": "load", "name": "k6"})
	k6Throttled := newC(counters, "container_cpu_cfs_throttled_periods_total", map[string]string{"service": "lore-k6", "role": "load", "name": "k6"})
	waitSum := newC(counters, "loadlore_pool_wait_seconds_sum", map[string]string{"service": "api", "target": "db"})
	waitCnt := newC(counters, "loadlore_pool_wait_seconds_count", map[string]string{"service": "api", "target": "db"})
	inUse := newC(gauges, "loadlore_pool_connections", map[string]string{"service": "api", "target": "db", "state": "in_use"})
	maxC := newC(gauges, "loadlore_pool_max_connections", map[string]string{"service": "api", "target": "db"})
	pending := newC(gauges, "loadlore_pool_pending", map[string]string{"service": "api", "target": "db"})
	var hists [][2]any
	hb := map[[2]float64]float64{}
	var hcount, hsum float64

	for s := -15.0; s <= dur+5; s++ {
		ts := t0 + s
		el := s
		live := el > 0 && el <= dur
		offered := 0.0
		if live {
			offered = p.Workload.OfferedRate(time.Duration((el - 0.5) * float64(time.Second)))
		}
		add := func(c *counter, v float64) {
			c.total += v
			c.vals = append(c.vals, [2]any{ts, fmt.Sprint(c.total)})
		}
		set := func(c *counter, v float64) { c.vals = append(c.vals, [2]any{ts, fmt.Sprint(v)}) }
		if live {
			ok := w.Served(el, offered)
			bad := w.Failed(el) * ok
			add(okReqs, ok-bad)
			add(badReqs, bad)
			add(drops, w.Dropped(el)*offered)
			add(apiCPU, w.APICPU(el)*p.Service("api").CPU)
			add(dbCPU, w.DBCPU(el)*p.Service("db").CPU)
			add(k6CPU, w.K6CPU(el)*p.Infra.LoadCPU)
			if w.K6Throttled != nil {
				add(k6Periods, 10) // one 100 ms period per tenth of a second
				add(k6Throttled, 10*w.K6Throttled(el))
			}
			add(waitSum, w.WaitS(el)*ok)
			add(waitCnt, ok)
			set(inUse, w.InUse(el))
			set(pending, w.Pending(el))
			// 98% of requests at p50, 2% at p99, in narrow buckets.
			for _, part := range []struct{ share, v float64 }{{0.98, w.P50(el)}, {0.02, w.P99(el)}} {
				k := [2]float64{part.v * 0.995, part.v * 1.005}
				hb[k] += part.share * ok
				hcount += part.share * ok
				hsum += part.share * ok * part.v
			}
			var buckets [][]any
			for k, c := range hb {
				buckets = append(buckets, []any{0, fmt.Sprint(k[0]), fmt.Sprint(k[1]), fmt.Sprint(c)})
			}
			hists = append(hists, [2]any{ts, map[string]any{"count": fmt.Sprint(hcount), "sum": fmt.Sprint(hsum), "buckets": buckets}})
		} else if el <= 0 { // k6 series are born with the load; the stack's exist before
			add(apiCPU, 0.01)
			add(dbCPU, 0.01)
			set(inUse, 0)
			set(pending, 0)
		}
		set(maxC, 10)
	}
	for _, m := range []map[string][]*counter{counters, gauges} {
		for name, cs := range m {
			var mf metricFile
			for _, c := range cs {
				labels := map[string]string{"__name__": name}
				for k, v := range c.labels {
					labels[k] = v
				}
				mf.Result = append(mf.Result, map[string]any{"metric": labels, "values": c.vals})
			}
			write("metrics/"+name+".json", mf)
		}
	}
	write("metrics/k6_http_req_duration_seconds.json", metricFile{Result: []map[string]any{{"metric": map[string]string{"__name__": "k6_http_req_duration_seconds"}, "histograms": hists}}})
	return dir
}
