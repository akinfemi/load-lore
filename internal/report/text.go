package report

import (
	"fmt"
	"math"
	"strings"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/design"
)

// Text is a run's report as plain text for a terminal, width columns
// wide: the verdict, small charts of the load, the main diagnosis, the
// predictions and why requests failed. path is the full report's, which
// has the diagram and the full charts.
func Text(a *analysis.Analysis, path string, width int) string {
	width = min(max(width, 60), 120)
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	para := func(indent, s string) {
		for _, l := range wrap(s, width-len(indent)) {
			line("%s%s", indent, l)
		}
	}

	head := a.Name + " · run " + a.RunID
	if h := a.Environment.Host; h != nil && h.Speed != nil {
		head += fmt.Sprintf(" · speed index %d", h.Speed.Index)
	}
	line("%s", head)
	line("")
	para("", strings.ToUpper(string(a.Verdict.Kind))+". "+a.Verdict.Summary)

	ws := a.Windows[:a.Analyzed]
	if len(ws) > 0 {
		line("")
		textCharts(&b, a, ws, width)
	}

	if len(a.Diagnoses) > 0 {
		line("")
		line("Diagnosis")
		d := a.Diagnoses[0]
		line("  1. %s, from %s", d.Title, analysis.Clock(d.From))
		para("     ", d.Summary)
		for _, e := range d.Evidence {
			for i, l := range wrap(e.Text, width-7) {
				lead := "     - "
				if i > 0 {
					lead = "       "
				}
				line("%s%s", lead, l)
			}
		}
		if d.Next != "" {
			para("     ", "Try next: "+d.Next)
		}
		for i, d := range a.Diagnoses[1:] {
			line("  %d. %s, from %s", i+2, d.Title, analysis.Clock(d.From))
		}
	}

	if len(a.Expect) > 0 {
		line("")
		line("Predictions")
		for _, e := range a.Expect {
			var status, s string
			switch e.Status {
			case analysis.ExpectNote:
				continue
			case analysis.ExpectHeld, analysis.ExpectBroken:
				status = string(e.Status)
				s = e.Text + ": worst " + e.Shown
				if _, run := design.RunMetrics[e.Metric]; run {
					s = e.Text + ": measured " + e.Shown
				}
				if e.BrokeAt != nil {
					s += ", broken from " + analysis.Clock(*e.BrokeAt)
				}
				if e.Message != "" {
					s += " (" + e.Message + ")"
				}
			default:
				status, s = "unscored", e.Text+": "+e.Message
			}
			// The status is a column; the rest wraps under itself.
			for i, l := range wrap(s, width-12) {
				if i == 0 {
					line("  %-9s %s", status, l)
				} else {
					line("  %-9s %s", "", l)
				}
			}
		}
		if n := predictionNote(a.Expect); n != "" {
			line("")
			para("  ", n)
		}
	}

	if ecs := analysis.ErrorCauses(ws); len(ecs) > 0 {
		line("")
		line("Why requests failed")
		for _, ec := range ecs {
			para("  ", fmt.Sprintf("%s: %s failed requests (%s of its failures), because %s, from %s", ec.Service, analysis.Count(ec.Count), analysis.Pct(ec.Share), ec.Meaning, analysis.Clock(ec.First)))
		}
	}

	line("")
	line("Full report, with the diagram and charts: %s", path)
	return b.String()
}

// textCharts draws offered and served load, p99 and errors as one row of
// blocks each, a block per window (windows are averaged when there are
// more than fit).
func textCharts(b *strings.Builder, a *analysis.Analysis, ws []*analysis.Window, width int) {
	const label = 10 // "  offered " and the like
	cols := width - label - 20
	get := func(f func(*analysis.Window) *float64) []*float64 {
		out := make([]*float64, len(ws))
		for i, w := range ws {
			out[i] = f(w)
		}
		return fit(out, cols)
	}
	offered := get(func(w *analysis.Window) *float64 { v := w.Offered; return &v })
	served := get(func(w *analysis.Window) *float64 { return w.RPS })
	top := 0.0
	for _, v := range append(append([]*float64{}, offered...), served...) {
		if v != nil {
			top = math.Max(top, *v)
		}
	}
	scale := func(v float64) float64 { return v / math.Max(top, 1) }
	fmt.Fprintf(b, "  %-8s%s  up to %s rps\n", "offered", spark(offered, scale), analysis.Rate(top))
	fmt.Fprintf(b, "  %-8s%s\n", "served", spark(served, scale))

	p99 := get(func(w *analysis.Window) *float64 { return w.P99 })
	lo, hi := math.Inf(1), 0.0
	for _, v := range p99 {
		if v != nil && *v > 0 {
			lo, hi = math.Min(lo, *v), math.Max(hi, *v)
		}
	}
	if hi > 0 {
		// Log scale, as p99 spans milliseconds to seconds. The top stands for
		// at least 10x the lowest, the break rule's factor, so a small bump
		// stays small.
		top := math.Max(hi, analysis.BreakLatencyFactor*lo)
		span := math.Log(top / lo)
		logScale := func(v float64) float64 {
			if span == 0 || v <= 0 {
				return 0.5
			}
			return math.Log(v/lo) / span
		}
		fmt.Fprintf(b, "  %-8s%s  %s to %s (log scale, top %s)\n", "p99", spark(p99, logScale), analysis.Ms(lo), analysis.Ms(hi), analysis.Ms(top))
	}
	errs := get(func(w *analysis.Window) *float64 { return w.ErrorRate })
	worst := 0.0
	for _, v := range errs {
		if v != nil {
			worst = math.Max(worst, *v)
		}
	}
	if worst > 0 {
		fmt.Fprintf(b, "  %-8s%s  up to %s\n", "errors", spark(errs, func(v float64) float64 { return v / worst }), analysis.Pct(worst))
	}
	end := float64(len(ws) * analysis.WindowSeconds)
	axis := "0:00" + strings.Repeat(" ", max(1, len(offered)-4-len(analysis.Clock(end)))) + analysis.Clock(end)
	fmt.Fprintf(b, "  %-8s%s\n", "", axis)
	if br := a.Verdict.Break; br != nil {
		pos := int(math.Round(br.At / end * float64(len(offered))))
		fmt.Fprintf(b, "  %-8s%s^ break at %s\n", "", strings.Repeat(" ", min(pos, max(0, len(offered)-1))), analysis.Clock(br.At))
	}
}

var blocks = []rune("▁▂▃▄▅▆▇█")

// spark draws one block per value, scaled by f to 0–1; a gap is a space.
func spark(vs []*float64, f func(float64) float64) string {
	var b strings.Builder
	for _, v := range vs {
		if v == nil || math.IsNaN(*v) {
			b.WriteRune(' ')
			continue
		}
		x := math.Max(0, math.Min(1, f(*v)))
		b.WriteRune(blocks[int(math.Round(x*float64(len(blocks)-1)))])
	}
	return b.String()
}

// fit averages neighbouring values until there are at most n.
func fit(vs []*float64, n int) []*float64 {
	if len(vs) <= n || n <= 0 {
		return vs
	}
	k := int(math.Ceil(float64(len(vs)) / float64(n)))
	var out []*float64
	for i := 0; i < len(vs); i += k {
		sum, cnt := 0.0, 0
		for _, v := range vs[i:min(i+k, len(vs))] {
			if v != nil {
				sum += *v
				cnt++
			}
		}
		if cnt == 0 {
			out = append(out, nil)
			continue
		}
		m := sum / float64(cnt)
		out = append(out, &m)
	}
	return out
}

// wrap breaks s into lines of at most n columns, at spaces.
func wrap(s string, n int) []string {
	var out []string
	var cur strings.Builder
	for _, w := range strings.Fields(s) {
		if cur.Len() > 0 && len([]rune(cur.String()))+1+len([]rune(w)) > n {
			out = append(out, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
