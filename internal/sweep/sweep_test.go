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
			raw, err := Render(c, cpu, Peak(c, cpu))
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
	p := st.Points[0]
	if p.Peak != 2*Peak(c[0], 0.5) || len(p.Runs) != 3 || counted(p) != 2 {
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
	if err != nil || !strings.Contains(string(sum), fmt.Sprintf("| 0.5 | %d rps | held, broke, broke, broke | 2 of 3 |", 2*Peak(c[0], 0.5))) {
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
