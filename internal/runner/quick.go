package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/scenario"
)

// quickSource is the earlier run a quick run starts near.
type quickSource struct {
	id       string
	breakRPS float64 // offered rate at its break, at its scale
	scale    float64
}

// findQuickSource picks the run a quick run starts from: the one named, or
// else the latest completed run that broke, of a design from the same
// scenario (a variant or solution of it counts: they share its profile), or
// with the same name outside a scenario, on the same profile and scale.
func findQuickSource(root, named, designPath, designName string, p *plan.Plan) (*quickSource, error) {
	type cand struct {
		res  Result
		pl   plan.Plan
		dir  string
		brk  float64
		why  string
		same bool
	}
	read := func(dir string) (*cand, error) {
		c := &cand{dir: dir}
		b, err := os.ReadFile(filepath.Join(dir, "run.json"))
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &c.res); err != nil {
			return nil, err
		}
		if b, err = os.ReadFile(filepath.Join(dir, "plan.json")); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &c.pl); err != nil {
			return nil, err
		}
		var an analysis.Analysis
		if b, err = os.ReadFile(filepath.Join(dir, "analysis.json")); err == nil && json.Unmarshal(b, &an) == nil && an.Verdict.Kind == analysis.VerdictBroke && an.Verdict.Break != nil {
			c.brk = an.Verdict.Break.Offered
		}
		switch {
		case c.brk == 0:
			c.why = "it didn't break"
		case c.pl.Quick != nil:
			c.why = "it was a quick run itself"
		case !sameProfile(&c.pl.Workload, &p.Workload) || c.pl.Scale != p.Scale:
			c.why = "its profile or scale differs from this run's"
		default:
			c.same = true
		}
		return c, nil
	}

	if named != "" {
		c, err := read(filepath.Join(root, named))
		if err != nil {
			return nil, &plan.QuickError{Reason: fmt.Sprintf("no run %s in %s: %v", named, root, err)}
		}
		if !c.same {
			return nil, &plan.QuickError{Reason: fmt.Sprintf("can't start from %s: %s", named, c.why)}
		}
		return &quickSource{id: named, breakRPS: c.brk, scale: c.pl.Scale}, nil
	}

	sc, inScenario := scenario.ForDesign(designPath)
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var best *cand
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		c, err := read(filepath.Join(root, e.Name()))
		if err != nil || !c.same || c.res.Status != StatusCompleted {
			continue
		}
		if inScenario {
			other, ok := scenario.ForDesign(c.res.DesignPath)
			if !ok || other.Path != sc.Path {
				continue
			}
		} else if c.res.Design != designName {
			continue
		}
		if best == nil || c.res.Started.After(best.res.Started) {
			best = c
		}
	}
	if best == nil {
		what := "of " + designName
		if inScenario {
			what = "of a design in the scenario " + sc.Name
		}
		return nil, &plan.QuickError{Reason: fmt.Sprintf("no earlier run %s broke on this profile at this scale, so there is no break to start near: run it once without --quick", what)}
	}
	return &quickSource{id: best.res.RunID, breakRPS: best.brk, scale: best.pl.Scale}, nil
}

func sameProfile(a, b *plan.Workload) bool {
	return a.StartRate == b.StartRate && slices.Equal(a.Stages, b.Stages)
}
