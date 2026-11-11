package report

import (
	"fmt"
	"math"
	"strings"

	"github.com/akinfemi/load-lore/internal/analysis"
)

// SeriesFileName is the series summary's name in the series folder.
const SeriesFileName = "series.md"

// Series renders a run --repeat series: each run, and how far apart the
// runs were.
func Series(s *analysis.Series) string {
	var d doc
	d.line("# Series: %s", s.Design)
	d.line("")
	d.p("_%s of `%s` at scale %g with seed %d: the same keys and mix each time, so what differs between runs is the host. Runs after the first waited for the host speed index to get back near the first run's, since a machine slows as it heats up._",
		plural(s.Planned, "1 run", fmt.Sprintf("%d runs", s.Planned)), s.Design, s.Scale, s.Seed)

	d.line("| Run | Speed index | Cool-down | Verdict | Broke at | Offered at the break | Peak served | p99 at the break |")
	d.line("| --- | --- | --- | --- | --- | --- | --- | --- |")
	for i, r := range s.Runs {
		speed := "not measured"
		if r.Speed > 0 {
			speed = fmt.Sprint(r.Speed)
		}
		d.line("| %d. `%s` | %s | %s | %s | %s | %s | %s | %s |", i+1, r.RunID, speed, cell(cooled(i, r)), r.Verdict,
			optF(r.BreakAt, analysis.Clock), optF(r.BreakOffered, rps), optF(r.PeakServed, rps), optF(r.P99AtBreak, analysis.Ms))
	}
	d.line("")

	d.line("## Spread")
	d.line("")
	spread := func(name string, sp *analysis.Spread, f func(float64) string, over string) {
		if sp == nil {
			d.line("- %s: no conclusive run gave one", name)
			return
		}
		switch {
		case sp.N == 1:
			d.line("- %s: %s, from 1 run %s", name, f(sp.Median), over)
			return
		case f(sp.Min) == f(sp.Max):
			d.line("- %s: %s in all %d runs %s", name, f(sp.Min), sp.N, over)
			return
		}
		d.line("- %s: %s to %s (median %s), %s apart over %d runs %s", name, f(sp.Min), f(sp.Max), f(sp.Median), width(sp, f), sp.N, over)
	}
	spread("Break time", s.BreakAt, analysis.Clock, "that broke")
	spread("Offered at the break", s.BreakOffered, rps, "that broke")
	spread("Peak served", s.PeakServed, rps, "that were conclusive")
	spread("p99 at the break", s.P99AtBreak, analysis.Ms, "that broke")
	d.line("")
	d.p("A difference between two designs smaller than these spreads may be the host, not the design. `loadl compare` says so when one of its runs is in a series.")

	if len(s.Warnings) > 0 {
		d.line("## Warnings")
		d.line("")
		for _, w := range s.Warnings {
			d.line("- %s", capitalize(w))
		}
		d.line("")
	}
	return d.String()
}

func cooled(i int, r analysis.SeriesRun) string {
	c := r.Cooled
	switch {
	case i == 0:
		return "first run"
	case c == nil:
		return "not measured"
	case c.Waited == 0:
		return "none needed"
	case c.Reached:
		return fmt.Sprintf("waited %s (%d to %d)", waited(c.Waited), c.First, r.Speed)
	default:
		return fmt.Sprintf("stopped after %s at %d, short of %d", waited(c.Waited), r.Speed, int(math.Ceil((1-c.Tolerance)*float64(c.Target))))
	}
}

func optF(v *float64, f func(float64) string) string {
	if v == nil {
		return "–"
	}
	return f(*v)
}

func rps(v float64) string { return analysis.Rate(v) + " rps" }

// width says how far apart a spread's extremes are, and as a share of its
// median when that's meaningful.
func width(sp *analysis.Spread, f func(float64) string) string {
	w := f(sp.Width())
	if sp.Median > 0 {
		w += fmt.Sprintf(" (%s of the median)", analysis.Pct(sp.Width()/sp.Median))
	}
	return w
}

// spreadCaveats say whether a difference between two runs is within the
// run-to-run spread of a series either belongs to, or that neither has one.
func spreadCaveats(a, b *analysis.Analysis) []string {
	if a.Series == nil && b.Series == nil {
		return []string{"Both are single runs. The same design varies from run to run on one machine, so a small difference may be noise: `loadl run --repeat 3` on each design measures how much on this host."}
	}
	var out []string
	check := func(name string, x, y *float64, pick func(*analysis.Series) *analysis.Spread, f func(float64) string) {
		if x == nil || y == nil {
			return
		}
		diff := math.Abs(*x - *y)
		for _, r := range []struct {
			who string
			s   *analysis.Series
		}{{"A", a.Series}, {"B", b.Series}} {
			if r.s == nil {
				continue
			}
			sp := pick(r.s)
			if sp == nil || sp.N < 2 || diff > sp.Width() {
				continue
			}
			out = append(out, fmt.Sprintf("The %s differs by %s, within the %s spread of run %s's series (%d runs, %s to %s): it may be noise, not the design.",
				name, f(diff), f(sp.Width()), r.who, sp.N, f(sp.Min), f(sp.Max)))
			return
		}
	}
	breakAt := func(x *analysis.Analysis) *float64 {
		if x.Verdict.Kind != analysis.VerdictBroke || x.Verdict.Break == nil {
			return nil
		}
		return &x.Verdict.Break.At
	}
	check("break time", breakAt(a), breakAt(b), func(s *analysis.Series) *analysis.Spread { return s.BreakAt }, analysis.Clock)
	check("peak served", a.Verdict.MaxRPS, b.Verdict.MaxRPS, func(s *analysis.Series) *analysis.Spread { return s.PeakServed }, rps)
	return out
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
