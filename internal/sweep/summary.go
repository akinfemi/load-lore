package sweep

import (
	"fmt"
	"slices"
	"strings"

	"github.com/akinfemi/load-lore/internal/analysis"
)

// Summary is the sweep's summary.md: per case, each CPU size's breaks and
// the calls a second per CPU they stand for.
func Summary(st *State) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Sweep %s\n\n", st.ID)
	fmt.Fprintf(&b, "_Each case stresses one operation on one component, the target, at several CPU sizes, %d runs each with seed %d. A run counts when it broke conclusively with the target the most saturated component; the rate per call is the offered rate at the break times the target's calls per request, divided by its CPU._\n\n", st.Repeat, st.Seed)
	if !st.Done {
		b.WriteString("_Not finished: `loadl sweep --resume " + st.ID + "` runs the rest._\n\n")
	}
	var names []string
	for _, p := range st.Points {
		if !slices.Contains(names, p.Case) {
			names = append(names, p.Case)
		}
	}
	for _, name := range names {
		c := caseNamed(nil, name)
		fmt.Fprintf(&b, "## %s\n\n%s, on %s (%d calls a request).\n\n", name, c.What, c.Target, c.Calls)
		b.WriteString("| CPU | Ramp to | Runs | Counted | Break (median) | Calls/s per CPU (median) | Limited by |\n| --- | --- | --- | --- | --- | --- | --- |\n")
		for _, p := range st.Points {
			if p.Case != name {
				continue
			}
			var verdicts, limits []string
			var breaks, per []float64
			n := 0
			for _, r := range p.Runs {
				v := r.Verdict
				if r.Error != "" {
					v = "error"
				}
				verdicts = append(verdicts, v)
				if r.LimitedBy != "" && !slices.Contains(limits, r.LimitedBy) {
					limits = append(limits, r.LimitedBy)
				}
				if r.Counts {
					n++
					breaks = append(breaks, *r.BreakRPS)
					per = append(per, *r.PerCall)
				}
			}
			peak := fmt.Sprintf("%d rps", p.Peak)
			if p.Held {
				peak += " (held)"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %d of %d | %s | %s | %s |\n", cpuLabel(p.CPU), peak, orDash(strings.Join(verdicts, ", ")), n, st.Repeat,
				medianText(breaks, "%s rps"), medianText(per, "%s"), orDash(strings.Join(limits, ", ")))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func medianText(xs []float64, format string) string {
	if len(xs) == 0 {
		return "–"
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	m := s[len(s)/2]
	if len(s)%2 == 0 {
		m = (s[len(s)/2-1] + s[len(s)/2]) / 2
	}
	return fmt.Sprintf(format, analysis.Count(m))
}

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}
