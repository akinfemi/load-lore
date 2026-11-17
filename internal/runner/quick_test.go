package runner

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/plan"
)

func TestFindQuickSource(t *testing.T) {
	root := t.TempDir()
	profile := plan.Workload{StartRate: 100, Stages: []plan.Stage{{Target: 1000, Duration: plan.Dur(3 * time.Minute)}}}
	other := plan.Workload{StartRate: 100, Stages: []plan.Stage{{Target: 2000, Duration: plan.Dur(3 * time.Minute)}}}
	write := func(id, design string, started time.Time, w plan.Workload, breakAt float64, quick bool) {
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		j := func(name string, v any) {
			b, _ := json.Marshal(v)
			if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		j("run.json", Result{RunID: id, Design: design, Status: StatusCompleted, Started: started})
		p := plan.Plan{Scale: 1, Workload: w}
		if quick {
			p.Quick = &plan.Quick{}
		}
		j("plan.json", p)
		an := analysis.Analysis{Verdict: analysis.Verdict{Kind: analysis.VerdictHeld}}
		if breakAt > 0 {
			an.Verdict = analysis.Verdict{Kind: analysis.VerdictBroke, Break: &analysis.Break{Offered: breakAt}}
		}
		j("analysis.json", an)
	}
	t0 := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	write("d-1", "d", t0, profile, 700, false)
	write("d-2", "d", t0.Add(time.Hour), profile, 800, false)   // the latest that broke
	write("d-3", "d", t0.Add(2*time.Hour), profile, 0, false)   // held
	write("d-4", "d", t0.Add(3*time.Hour), other, 900, false)   // another profile
	write("d-5", "d", t0.Add(4*time.Hour), profile, 600, true)  // a quick run
	write("e-1", "e", t0.Add(5*time.Hour), profile, 500, false) // another design
	p := &plan.Plan{Scale: 1, Workload: profile}

	src, err := findQuickSource(root, "", "/nowhere/d.yaml", "d", p)
	if err != nil || src.id != "d-2" || src.breakRPS != 800 {
		t.Fatalf("source %+v, %v", src, err)
	}
	if src, err := findQuickSource(root, "d-1", "/nowhere/d.yaml", "d", p); err != nil || src.id != "d-1" || src.breakRPS != 700 {
		t.Errorf("named: %+v, %v", src, err)
	}
	var qe *plan.QuickError
	if _, err := findQuickSource(root, "d-3", "/nowhere/d.yaml", "d", p); !errors.As(err, &qe) || !strings.Contains(qe.Reason, "it didn't break") {
		t.Errorf("named, held: %v", err)
	}
	if _, err := findQuickSource(root, "", "/nowhere/f.yaml", "f", p); !errors.As(err, &qe) || !strings.Contains(qe.Reason, "run it once without --quick") {
		t.Errorf("none: %v", err)
	}
}
