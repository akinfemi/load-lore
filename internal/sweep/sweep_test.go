package sweep

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/runner"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/plan"
)

// Every case renders, validates and plans at every default size it runs
// at: the sweep never starts a design that can't run.
func TestCasesValidate(t *testing.T) {
	for _, c := range Cases {
		for _, cpu := range DefaultCPUs {
			if c.MaxCPU > 0 && cpu > c.MaxCPU {
				continue
			}
			if ok, _ := Fits(c, cpu); !ok {
				continue
			}
			calls, peak := Shape(c, cpu, MaxDoublings)
			if peak > MaxRPS && !c.Fixed {
				t.Errorf("%s at %g CPU: the top doubled ramp is %d rps", c.Name, cpu, peak)
			}
			raw, err := Render(c, cpu, calls, peak)
			if err != nil {
				t.Fatalf("%s: %v", c.Name, err)
			}
			d, diags := design.Load(raw)
			if diags.HasErrors() {
				t.Fatalf("%s at %g CPU: %v\n%s", c.Name, cpu, diags.Errors(), raw)
			}
			p, err := plan.Build(d, plan.Options{Scale: 1})
			if err != nil {
				t.Fatalf("%s at %g CPU: plan: %v", c.Name, cpu, err)
			}
			if s := p.Service(c.Target); s == nil {
				t.Errorf("%s: no target %s in the plan", c.Name, c.Target)
			}
		}
	}
}

// A point whose first run holds doubles its ramp top; runs whose break
// names another component don't count; a sweep resumed after a stop runs
// only what's left.
func TestDo(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _ := Find([]string{"pg-point-read"})
	calls := 0
	fake := func(o runner.Options) *runner.Result {
		calls++
		return &runner.Result{RunID: fmt.Sprintf("r%d", calls), Dir: fmt.Sprintf("peak=%g;n=%d", o.Sweep.Peak, calls)}
	}
	analyze := func(dir string) (*analysis.Analysis, error) {
		var peak float64
		var n int
		if _, err := fmt.Sscanf(dir, "peak=%g;n=%d", &peak, &n); err != nil {
			return nil, err
		}
		a := &analysis.Analysis{}
		switch n {
		case 1: // the first ramp was too low
			a.Verdict.Kind = analysis.VerdictHeld
		case 3: // the api gave out first
			a.Verdict = analysis.Verdict{Kind: analysis.VerdictBroke, Break: &analysis.Break{At: 100, Offered: peak / 2, Saturation: []analysis.Saturation{{Component: "api"}}}}
		default:
			a.Verdict = analysis.Verdict{Kind: analysis.VerdictBroke, Break: &analysis.Break{At: 120, Offered: peak / 2, Saturation: []analysis.Saturation{{Component: "db"}}}}
		}
		return a, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	o := Options{Cases: c, CPUs: []float64{0.5}, Repeat: 3, RunOne: func(_ context.Context, ro runner.Options, _ func(context.Context) (driver.Driver, error)) (*runner.Result, error) {
		r := fake(ro)
		if calls == 3 {
			cancel() // stop after the third run
		}
		return r, nil
	}, Analyze: analyze}
	st, _, err := Do(ctx, o)
	if err == nil {
		t.Fatal("the stopped sweep didn't say so")
	}
	_, peak1 := Shape(c[0], 0.5, 1)
	p := st.Points[0]
	if p.Peak != peak1 || len(p.Runs) != 3 || counted(p) != 2 {
		t.Fatalf("after the stop: peak %d, %d runs, %d counted", p.Peak, len(p.Runs), counted(p))
	}
	if p.Runs[2].Counts || !p.Runs[1].Counts || p.Runs[2].LimitedBy != "api" {
		t.Errorf("counting: %+v %+v", p.Runs[1], p.Runs[2])
	}
	o.Resume = st.ID
	o.RunOne = func(_ context.Context, ro runner.Options, _ func(context.Context) (driver.Driver, error)) (*runner.Result, error) {
		return fake(ro), nil
	}
	st, dir, err := Do(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if p := st.Points[0]; len(p.Runs) != 4 || counted(p) != 3 || !st.Done {
		t.Errorf("after resuming: %d runs, %d counted, done %v", len(p.Runs), counted(p), st.Done)
	}
	sum, err := os.ReadFile(filepath.Join(dir, "summary.md"))
	if err != nil || !strings.Contains(string(sum), fmt.Sprintf("| 0.5 | 10 | %d rps | held, broke, broke, broke | 2 of 3 |", peak1)) {
		t.Errorf("summary:\n%s", sum)
	}
}

// A full pool stands for its target: requests queue for connections
// because the target is slow.
func TestLimitedBy(t *testing.T) {
	ss := []analysis.Saturation{{Component: "api", What: "pool to db"}, {Component: "db", What: "CPU"}}
	if got := limitedBy(ss); got != "db" {
		t.Errorf("limited by %q", got)
	}
	ss = []analysis.Saturation{{Component: "db", What: "connections"}, {Component: "api", What: "CPU"}}
	if got := limitedBy(ss); got != "api" {
		t.Errorf("limited by %q", got)
	}
}

// A ramp past MaxRPS makes more calls a request instead, keeping the calls
// a second; a one-call case past it is skipped as not fitting.
func TestShapeAndFits(t *testing.T) {
	c := Case{Name: "w", Target: "db", PerCPU: 12000, Calls: 1}
	if calls, peak := Shape(c, 0.25, 0); calls != 1 || peak != 6000 {
		t.Errorf("0.25: %d calls, %d rps", calls, peak)
	}
	if calls, peak := Shape(c, 2, 0); calls != 5 || peak != 9600 {
		t.Errorf("2: %d calls, %d rps", calls, peak)
	}
	if calls, peak := Shape(c, 2, 2); calls != 20 || peak != 9600 {
		t.Errorf("2, doubled twice: %d calls, %d rps", calls, peak)
	}
	lb := Case{Name: "lb", Target: "lb", PerCPU: 29000, Calls: 1, Fixed: true}
	if ok, _ := Fits(lb, 0.25); !ok {
		t.Error("the lb at 0.25 should fit")
	}
	if ok, why := Fits(lb, 0.5); ok || !strings.Contains(why, "14,500") {
		t.Errorf("the lb at 0.5: %v %q", ok, why)
	}
	if calls, peak := Shape(lb, 0.25, 0); calls != 1 || peak != 14500 {
		t.Errorf("lb: %d calls, %d rps", calls, peak)
	}
}

// The fan-out's second shard counts for the first; resuming a sweep counts
// its runs again by today's rules.
func TestCountsAlso(t *testing.T) {
	c, _ := Find([]string{"pg-shard-fanout"})
	off := 1000.0
	p := &Point{Case: c[0].Name, Target: "db0", Calls: 5}
	for who, want := range map[string]bool{"db0": true, "db1": true, "api": false} {
		r := &Run{Verdict: "broke", BreakRPS: &off, LimitedBy: who}
		if got := counts(c[0], p, r); got != want {
			t.Errorf("limited by %s: counts %v", who, got)
		}
	}
	if counts(c[0], p, &Run{Verdict: "inconclusive", BreakRPS: &off, LimitedBy: "db0"}) {
		t.Error("an inconclusive run counted")
	}
}
