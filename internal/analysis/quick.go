package analysis

import "fmt"

// quickNote says, on a quick run, where its climb started, and when it
// broke so close to that start that the warm-up may have set the break.
func quickNote(a *Analysis) string {
	q := a.Plan.Quick
	if q == nil {
		return ""
	}
	n := fmt.Sprintf(" A quick run: the load climbed to %s rps in %g s, %.0f%% of the %s rps where %s broke, then followed the profile from there; the first %s of the climb were skipped, and predictions aren't scored.",
		Count(q.StartRPS), q.WarmupS, q.StartRPS/q.BreakRPS*100, Count(q.BreakRPS), q.FromRun, Clock(q.SkippedS))
	if b := a.Verdict.Break; a.Verdict.Kind == VerdictBroke && b != nil && b.Offered < q.LowRPS {
		n += fmt.Sprintf(" It broke at %s rps offered, at or just above where the climb resumed: the warm-up's jump may have set the break, so run it without --quick to check.", Count(b.Offered))
	}
	return n
}
