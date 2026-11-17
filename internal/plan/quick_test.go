package plan

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

const quickDesign = `
version: 1
name: q
services:
  api: { image: loadlore/synthetic, behavior: { calls: [ { target: db, op: read } ] } }
  db: { kind: postgres, dataset: { rows: 1000 } }
workload:
  entry: api
  profile:
    - ramp: { from: 200, to: 2900, over: 3m }
    - hold: { rps: 2900, for: 2m }
`

// The viral post's profile, quick from a break at 2,525 rps: a 30 s
// warm-up to 75% of it, then the rest of the ramp at 15 rps a second.
func TestQuicken(t *testing.T) {
	p, err := Build(load(t, quickDesign), Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := p.Quicken("prev", 2525)
	if !ok || err != nil {
		t.Fatalf("quicken: %v %v", ok, err)
	}
	w := p.Workload
	target := 0.75 * 2525
	resume := time.Duration((target - 200) / 15 * float64(time.Second))
	if len(w.Stages) != 3 || w.StartRate != 200 || w.Stages[0].Target != target || w.Stages[0].Duration.D() != QuickWarmup ||
		w.Stages[1].Target != 2900 || absDur(w.Stages[1].Duration.D()-(3*time.Minute-resume)) > time.Millisecond || w.Stages[2].Duration.D() != 2*time.Minute {
		t.Fatalf("stages %+v", w.Stages)
	}
	// The climb past the warm-up keeps the profile's slope.
	if r := w.OfferedRate(QuickWarmup + 10*time.Second); math.Abs(r-(target+150)) > 0.5 {
		t.Errorf("rate 10 s after the warm-up: %v", r)
	}
	if got := p.Duration; absDur(got-(3*time.Minute+37*time.Second)) > time.Second || got != w.Duration() {
		t.Errorf("duration %v", got)
	}
	q := p.Quick
	if q == nil || q.FromRun != "prev" || q.StartRPS != target || math.Abs(q.SkippedS-(resume-QuickWarmup).Seconds()) > 1e-6 || q.LowRPS != target+0.1*(2900-target) {
		t.Errorf("quick %+v", q)
	}
	if !strings.Contains(strings.Join(p.Notes, "\n"), "quick: the load climbs from 200 to 1893.75 rps in 30s") {
		t.Errorf("notes %v", p.Notes)
	}
}

func TestQuickenRefusesAndSkips(t *testing.T) {
	build := func(src string) *Plan {
		p, err := Build(load(t, src), Options{RunID: "r"})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	// A break near the bottom leaves too little to skip.
	if ok, err := build(quickDesign).Quicken("prev", 500); ok || err != nil {
		t.Errorf("early break: %v %v", ok, err)
	}
	// A fault in the skipped climb refuses; one after it moves with it.
	var qe *QuickError
	p := build(quickDesign + "scenario:\n  - { at: 1m, inject: kill, target: api }\n")
	if _, err := p.Quicken("prev", 2525); !errors.As(err, &qe) || !strings.Contains(qe.Reason, "a kill fault at 1m0s") {
		t.Errorf("early fault: %v", err)
	}
	p = build(quickDesign + "scenario:\n  - { at: 4m, inject: kill, target: api }\n")
	if ok, err := p.Quicken("prev", 2525); !ok || err != nil || absDur(p.Faults[0].At.D()-(4*time.Minute-time.Duration(p.Quick.SkippedS*float64(time.Second)))) > time.Millisecond {
		t.Errorf("late fault: %v %v %+v", ok, err, p.Faults[0])
	}
	// Autoscaling refuses.
	src := strings.Replace(quickDesign, "api: { image: loadlore/synthetic,", "lb: { kind: lb, target: api }\n  api: { image: loadlore/synthetic, autoscale: { min: 1, max: 3, metric: cpu, target: 0.7 },", 1)
	src = strings.Replace(src, "entry: api", "entry: lb", 1)
	if _, err := build(src).Quicken("prev", 2525); !errors.As(err, &qe) || !strings.Contains(qe.Reason, "api autoscales") {
		t.Errorf("autoscale: %v", err)
	}
	// A break above anything the profile climbs through.
	if _, err := build(quickDesign).Quicken("prev", 5000); !errors.As(err, &qe) || !strings.Contains(qe.Reason, "never climbs through 3750 rps") {
		t.Errorf("break above the peak: %v", err)
	}
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
