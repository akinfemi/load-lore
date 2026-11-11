package scenario

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/analysis/analysistest"
	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/driver"
)

// speedRun is a stand-in run: the fixture run it analyzes as and the host
// speed index it measured.
type speedRun struct {
	fixture string
	speed   int
}

// speedRuns hands out each design's planned runs in turn, as run folders
// named <design>-<n>, and records what each run waited for.
type speedRuns struct {
	plan  map[string][]speedRun // by design: "demo" or a solution's name
	runs  map[string]speedRun   // by run folder
	calls []string              // "<run folder> cooled to <speed index>"
}

func (f *speedRuns) run(_ context.Context, p string, coolTo int) (string, error) {
	name := filepath.Base(filepath.Dir(p))
	n := 1
	for len(f.runs) > 0 && f.runs[fmt.Sprintf("%s-%d", name, n)] != (speedRun{}) {
		n++
	}
	if n > len(f.plan[name]) {
		return "", fmt.Errorf("run %d of %s wasn't planned", n, name)
	}
	dir := fmt.Sprintf("%s-%d", name, n)
	f.runs[dir] = f.plan[name][n-1]
	f.calls = append(f.calls, fmt.Sprintf("%s cooled to %d", dir, coolTo))
	return dir, nil
}

// checkSpeeds checks a two-solution demo scenario on the planned runs: the
// analysis is the fixture's, with the planned speed index.
func checkSpeeds(t *testing.T, plan map[string][]speedRun) (*CheckResult, []string) {
	t.Helper()
	files := demoFiles()
	files["meta.yaml"] = strings.Replace(strings.Replace(files["meta.yaml"], "{ verdict: broke, diagnoses: [pool-exhausted] }", "{ verdict: broke }", 1),
		"bigger: { min_gain: 1.5 }", "bigger: {}\n    pooled: {}", 1)
	files["solutions/pooled/design.yaml"] = strings.Replace(demoDesign, "name: demo", "name: demo-pooled", 1)
	files["solutions/pooled/README.md"] = "# Pooled\n"
	dir := writeScenario(t, files)
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ps := s.Lint(); HasErrors(ps) {
		t.Fatal(ps)
	}

	f := &speedRuns{plan: plan, runs: map[string]speedRun{}}
	analyze = func(dir string) (*analysis.Analysis, error) {
		r, ok := f.runs[dir]
		if !ok {
			return nil, errors.New("no run " + dir)
		}
		a, err := analysis.Analyze(r.fixture)
		if err != nil {
			return nil, err
		}
		h := driver.Host{}
		if a.Environment.Host != nil {
			h = *a.Environment.Host
		}
		h.Speed = &bench.Speed{Score: bench.Score{Index: r.speed}}
		a.Environment.Host = &h
		return a, nil
	}
	t.Cleanup(func() { analyze = analysis.Analyze })
	return Check(context.Background(), s, dir, f.run), f.calls
}

// speedFixtures are the fixture's system breaking at 70 s, breaking at
// 50 s, and holding.
func speedFixtures(t *testing.T) (broke70, broke50, healthy string) {
	breakAt := func(at float64) string {
		w := analysistest.Healthy()
		w.P99 = func(t float64) float64 { return 0.005 + 2*boolF(t > at) }
		return analysistest.WriteRun(t, w, 1)
	}
	return breakAt(70), breakAt(50), analysistest.WriteRun(t, analysistest.Healthy(), 1)
}

func wantCalls(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("runs:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func solution(res *CheckResult, name string) *RunCheck {
	for _, rc := range res.Solutions {
		if rc.Name == name {
			return rc
		}
	}
	return nil
}

// Within bench.Tolerance of the starting design's speed, nothing runs again.
func TestCheckSpeedWithinTolerance(t *testing.T) {
	broke, _, healthy := speedFixtures(t)
	res, calls := checkSpeeds(t, map[string][]speedRun{
		"demo":   {{broke, 1000}},
		"bigger": {{healthy, 950}},
		"pooled": {{healthy, 1080}},
	})
	wantCalls(t, calls, "demo-1 cooled to 0", "bigger-1 cooled to 1000", "pooled-1 cooled to 1000")
	if !res.OK || res.Status != StatusOK {
		t.Errorf("status %s: %+v", res.Status, res)
	}
	for _, rc := range append([]*RunCheck{res.Problem}, res.Solutions...) {
		if rc.Status != StatusOK || len(rc.Replaced) != 0 || rc.Inconclusive != "" {
			t.Errorf("%s: %+v", rc.Name, rc)
		}
	}
}

// A slower solution runs again, cooled to the starting design's speed, and
// the new run is the one judged.
func TestCheckRerunsSlowerSolution(t *testing.T) {
	broke, _, healthy := speedFixtures(t)
	res, calls := checkSpeeds(t, map[string][]speedRun{
		"demo":   {{broke, 1000}},
		"bigger": {{healthy, 850}, {healthy, 980}},
		"pooled": {{healthy, 1000}},
	})
	wantCalls(t, calls, "demo-1 cooled to 0", "bigger-1 cooled to 1000", "bigger-2 cooled to 1000", "pooled-1 cooled to 1000")
	if !res.OK || res.Status != StatusOK {
		t.Errorf("status %s", res.Status)
	}
	sol := solution(res, "bigger")
	want := []Rerun{{RunDir: "bigger-1", SpeedIndex: 850, Against: "problem", AgainstSpeed: 1000, NewSpeed: 980}}
	if sol.RunDir != "bigger-2" || sol.SpeedIndex != 980 || !slices.Equal(sol.Replaced, want) || sol.Status != StatusOK {
		t.Errorf("bigger: %+v", sol)
	}
	if len(res.Problem.Replaced) != 0 {
		t.Errorf("the starting design ran again: %+v", res.Problem.Replaced)
	}
}

// A slower starting design runs again, cooled to the solution's speed; its
// new run replaces the old one, and the solutions before it are judged
// against the new one, running again only when they are now too far apart.
func TestCheckRerunsSlowerProblem(t *testing.T) {
	broke70, broke50, healthy := speedFixtures(t)
	res, calls := checkSpeeds(t, map[string][]speedRun{
		"demo":   {{broke70, 1000}, {broke50, 1150}},
		"bigger": {{healthy, 1060}},
		"pooled": {{healthy, 1200}},
	})
	wantCalls(t, calls, "demo-1 cooled to 0", "bigger-1 cooled to 1000", "pooled-1 cooled to 1000", "demo-2 cooled to 1200")
	if !res.OK || res.Status != StatusOK {
		t.Errorf("status %s", res.Status)
	}
	p := res.Problem
	want := []Rerun{{RunDir: "demo-1", SpeedIndex: 1000, Against: "pooled", AgainstSpeed: 1200, NewSpeed: 1150}}
	if p.RunDir != "demo-2" || p.SpeedIndex != 1150 || !slices.Equal(p.Replaced, want) || p.Status != StatusOK || p.BreakLoad == nil {
		t.Fatalf("problem: %+v", p)
	}
	// bigger, 1060 against 1150, is within the tolerance: judged again
	// against the new run's break, without running again.
	sol := solution(res, "bigger")
	if *p.BreakLoad == 750 || sol.RunDir != "bigger-1" || len(sol.Replaced) != 0 {
		t.Errorf("bigger ran again, or the new run broke where the old one did: %+v", sol)
	}
	if c := sol.Claims[0]; !c.OK || !strings.HasSuffix(c.Detail, "the starting design broke at "+analysis.Rate(*p.BreakLoad)+" rps offered") {
		t.Errorf("bigger isn't judged against the new run: %+v", c)
	}

	// Here bigger, 1000 against the new 1150, is too far apart and slower,
	// so it runs again too, cooled to the new run's speed.
	res, calls = checkSpeeds(t, map[string][]speedRun{
		"demo":   {{broke70, 1000}, {broke50, 1150}},
		"bigger": {{healthy, 1000}, {healthy, 1120}},
		"pooled": {{healthy, 1200}},
	})
	wantCalls(t, calls, "demo-1 cooled to 0", "bigger-1 cooled to 1000", "pooled-1 cooled to 1000", "demo-2 cooled to 1200", "bigger-2 cooled to 1150")
	sol = solution(res, "bigger")
	want = []Rerun{{RunDir: "bigger-1", SpeedIndex: 1000, Against: "problem", AgainstSpeed: 1150, NewSpeed: 1120}}
	if !res.OK || sol.RunDir != "bigger-2" || !slices.Equal(sol.Replaced, want) {
		t.Errorf("status %s, bigger: %+v", res.Status, sol)
	}
}

// Still too far apart after a re-run, a solution is inconclusive: not ok,
// not failed. A failed claim elsewhere still fails the check.
func TestCheckInconclusive(t *testing.T) {
	broke, _, healthy := speedFixtures(t)
	res, calls := checkSpeeds(t, map[string][]speedRun{
		"demo":   {{broke, 1000}},
		"bigger": {{healthy, 800}, {healthy, 850}},
		"pooled": {{healthy, 1000}},
	})
	wantCalls(t, calls, "demo-1 cooled to 0", "bigger-1 cooled to 1000", "bigger-2 cooled to 1000", "pooled-1 cooled to 1000")
	if res.OK || res.Status != StatusInconclusive {
		t.Errorf("status %s, ok %v", res.Status, res.OK)
	}
	sol := solution(res, "bigger")
	if sol.OK || sol.Status != StatusInconclusive || sol.Inconclusive != "the host's speed index was 850, against 1000 in the starting design's run, 15% apart, more than 10% even after a re-run: the host's speed, not the design, could explain the difference" {
		t.Errorf("bigger: %+v", sol)
	}
	if p := solution(res, "pooled"); p.Status != StatusOK || res.Problem.Status != StatusOK {
		t.Errorf("pooled %s, problem %s", p.Status, res.Problem.Status)
	}

	// The starting design ran again for bigger and is still too slow; pooled
	// then waits for the new run's speed.
	res, calls = checkSpeeds(t, map[string][]speedRun{
		"demo":   {{broke, 1000}, {broke, 1050}},
		"bigger": {{healthy, 1200}},
		"pooled": {{healthy, 1100}},
	})
	wantCalls(t, calls, "demo-1 cooled to 0", "bigger-1 cooled to 1000", "demo-2 cooled to 1200", "pooled-1 cooled to 1050")
	if sol := solution(res, "bigger"); res.Status != StatusInconclusive || sol.Status != StatusInconclusive || !strings.Contains(sol.Inconclusive, "was 1200, against 1050") {
		t.Errorf("status %s, bigger: %+v", res.Status, sol)
	}

	// Failed wins over inconclusive: pooled serves no more than the
	// starting design.
	res, _ = checkSpeeds(t, map[string][]speedRun{
		"demo":   {{broke, 1000}},
		"bigger": {{healthy, 800}, {healthy, 850}},
		"pooled": {{broke, 1000}},
	})
	if res.OK || res.Status != StatusFailed || solution(res, "bigger").Status != StatusInconclusive || solution(res, "pooled").Status != StatusFailed {
		t.Errorf("status %s: %+v", res.Status, res.Solutions)
	}
}
