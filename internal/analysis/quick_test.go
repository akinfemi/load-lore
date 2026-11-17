package analysis

import (
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/plan"
)

func TestQuickNote(t *testing.T) {
	a := &Analysis{Plan: &plan.Plan{}}
	if n := quickNote(a); n != "" {
		t.Errorf("note on a full run: %q", n)
	}
	a.Plan.Quick = &plan.Quick{FromRun: "prev", BreakRPS: 2525, StartRPS: 1893.75, WarmupS: 30, SkippedS: 83, LowRPS: 1994}
	a.Verdict = Verdict{Kind: VerdictBroke, Break: &Break{Offered: 2825}}
	n := quickNote(a)
	if !strings.Contains(n, "A quick run: the load climbed to 1,894 rps in 30 s, 75% of the 2,525 rps where prev broke") || strings.Contains(n, "without --quick") {
		t.Errorf("note %q", n)
	}
	a.Verdict.Break.Offered = 1950
	if n := quickNote(a); !strings.Contains(n, "It broke at 1,950 rps offered, at or just above where the climb resumed") {
		t.Errorf("low break %q", n)
	}
}
