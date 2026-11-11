package analysis

import (
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/driver"
)

func seriesRun(id string, kind VerdictKind, speed int, at, peak float64) *Analysis {
	a := &Analysis{RunID: id, Name: "d", Scale: 1, Seed: 7, Verdict: Verdict{Kind: kind, MaxRPS: &peak},
		Environment: Environment{Host: &driver.Host{Speed: &bench.Speed{Score: bench.Score{Index: speed}}}}}
	if kind == VerdictBroke {
		p99 := 900.0
		a.Verdict.Break = &Break{At: at, Offered: at * 10, P99: &p99}
	}
	return a
}

func TestSummarizeSpreads(t *testing.T) {
	s := Summarize("s", 4, []*Analysis{
		seriesRun("r1", VerdictBroke, 1000, 180, 2950),
		seriesRun("r2", VerdictBroke, 960, 220, 3100),
		seriesRun("r3", VerdictInconclusive, 880, 200, 3500),
	})
	if s.BreakAt == nil || s.BreakAt.N != 2 || s.BreakAt.Min != 180 || s.BreakAt.Max != 220 || s.BreakAt.Median != 200 {
		t.Errorf("break at %+v", s.BreakAt)
	}
	// The inconclusive run's peak is left out.
	if s.PeakServed == nil || s.PeakServed.Max != 3100 || s.Seed != 7 {
		t.Errorf("peak %+v, seed %d", s.PeakServed, s.Seed)
	}
	all := strings.Join(s.Warnings, "\n")
	for _, want := range []string{"only 3 of the 4 planned runs", "1 run is left out of the spreads", "r3", "speed index was 880, against 1000 for the first run, 12% apart"} {
		if !strings.Contains(all, want) {
			t.Errorf("warnings lack %q:\n%s", want, all)
		}
	}
	// 960 is 4% from 1000: no speed warning for r2.
	if strings.Contains(all, "r2's") {
		t.Errorf("r2 flagged:\n%s", all)
	}
}
