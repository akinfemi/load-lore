package report

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/mermaid"
)

// Compare renders two runs side by side, such as before and after a design
// change. Run A is the baseline.
func Compare(a, b *analysis.Analysis) string {
	var d doc
	title := a.Name
	if b.Name != a.Name {
		title = a.Name + " vs " + b.Name
	}
	d.line("# Compare: %s", title)
	d.line("")
	d.p("_A: run `%s` · B: run `%s`_", a.RunID, b.RunID)

	d.line("| | A | B |")
	d.line("| --- | --- | --- |")
	row := func(name string, f func(*analysis.Analysis) string) {
		d.line("| %s | %s | %s |", name, cell(f(a)), cell(f(b)))
	}
	row("Design", func(x *analysis.Analysis) string { return x.Name })
	row("Verdict", shortVerdict)
	row("Most served", func(x *analysis.Analysis) string {
		if x.Verdict.MaxRPS == nil {
			return "no data"
		}
		return fmt.Sprintf("%s rps at %s", analysis.Rate(*x.Verdict.MaxRPS), analysis.Clock(x.Verdict.MaxRPSAt))
	})
	row("Worst 10-second p99", func(x *analysis.Analysis) string { return optMs(x.Verdict.WorstP99) })
	row("Worst error rate", func(x *analysis.Analysis) string {
		if x.Verdict.WorstErrors == nil {
			return "no data"
		}
		return analysis.Pct(*x.Verdict.WorstErrors)
	})
	row("Diagnoses", func(x *analysis.Analysis) string {
		if len(x.Diagnoses) == 0 {
			return "none fired"
		}
		var ts []string
		for _, dg := range x.Diagnoses {
			ts = append(ts, dg.Title+" from "+analysis.Clock(dg.From))
		}
		return strings.Join(ts, "; ")
	})
	row("Predictions held", func(x *analysis.Analysis) string {
		held, scored := 0, 0
		for _, e := range x.Expect {
			if e.Status == analysis.ExpectHeld || e.Status == analysis.ExpectBroken {
				scored++
			}
			if e.Status == analysis.ExpectHeld {
				held++
			}
		}
		return fmt.Sprintf("%d of %d", held, scored)
	})
	row("Peak offered", func(x *analysis.Analysis) string { return analysis.Rate(x.PeakOffered) + " rps" })
	row("Scale, seed", func(x *analysis.Analysis) string { return fmt.Sprintf("%g, %d", x.Scale, x.Seed) })
	row("Host", func(x *analysis.Analysis) string {
		if h := x.Environment.Host; h != nil {
			return fmt.Sprintf("%d CPUs, %s, Docker %s", h.CPUs, gib(h.MemoryBytes), h.EngineVersion)
		}
		return "unknown"
	})
	row("Data", func(x *analysis.Analysis) string {
		if x.ReuseData {
			return "reused (drifted)"
		}
		return "fresh copy"
	})
	d.line("")
	d.p("<sub>Sources as in each run's report: served from %s, p99 from %s, errors from %s.</sub>", analysis.SrcRPS, analysis.SrcK6Latency, analysis.SrcErrors)

	if cs := caveats(a, b); len(cs) > 0 {
		d.line("**Before reading the difference:**")
		d.line("")
		for _, c := range cs {
			d.line("- %s", c)
		}
		d.line("")
	}

	d.line("## Verdicts")
	d.line("")
	d.p("**A.** %s", a.Verdict.Summary)
	d.p("**B.** %s", b.Verdict.Summary)

	if len(a.Windows) > 0 && len(b.Windows) > 0 {
		compareCharts(&d, a, b)
		compareWindows(&d, a, b)
	}

	d.line("## Diagnosis")
	d.line("")
	for _, x := range []struct {
		label string
		an    *analysis.Analysis
	}{{"A", a}, {"B", b}} {
		d.line("### %s", x.label)
		d.line("")
		if len(x.an.Diagnoses) == 0 {
			d.p("No diagnosis rule fired.")
			continue
		}
		for _, dg := range x.an.Diagnoses {
			d.line("- **%s, from %s.** %s", dg.Title, analysis.Clock(dg.From), dg.Summary)
		}
		d.line("")
	}

	comparePredictions(&d, a, b)

	d.line("## Design changes")
	d.line("")
	if diff := unifiedDiff(a.DesignYAML, b.DesignYAML, "A/design.yaml", "B/design.yaml"); diff == "" {
		d.p("Both runs used the same design file.")
	} else {
		d.line("```diff")
		d.WriteString(diff)
		d.line("```")
	}
	return d.String()
}

func shortVerdict(x *analysis.Analysis) string {
	v := x.Verdict
	switch {
	case v.Kind == analysis.VerdictInconclusive:
		return "inconclusive"
	case v.Break != nil:
		return fmt.Sprintf("broke at %s (%s rps offered)", analysis.Clock(v.Break.At), analysis.Rate(v.Break.Offered))
	case v.Kind == analysis.VerdictHeld:
		return "held"
	}
	return "no data"
}

func optMs(p *float64) string {
	if p == nil {
		return "no data"
	}
	return analysis.Ms(*p)
}

// caveats are differences that make a comparison less than like for like.
func caveats(a, b *analysis.Analysis) []string {
	var out []string
	if a.Scale != b.Scale {
		out = append(out, fmt.Sprintf("The runs used different scales (%g and %g): resources and load differ by more than the design change.", a.Scale, b.Scale))
	}
	if a.Environment.Duration != b.Environment.Duration || a.PeakOffered != b.PeakOffered {
		out = append(out, "The load profiles differ, so windows at the same time had different offered load. Compare the offered columns.")
	}
	if ha, hb := a.Environment.Host, b.Environment.Host; ha != nil && hb != nil && (ha.CPUs != hb.CPUs || ha.MemoryBytes != hb.MemoryBytes) {
		out = append(out, "The runs ran on hosts of different sizes.")
	}
	if a.Seed != b.Seed || a.Seed == 0 {
		out = append(out, "The runs didn't share a --seed, so they drew different keys and read/write sequences.")
	}
	for _, x := range []*analysis.Analysis{a, b} {
		if x.ReuseData {
			out = append(out, fmt.Sprintf("Run %s reused data earlier runs wrote to (--reuse-data).", x.RunID))
		}
		if x.Verdict.Kind == analysis.VerdictInconclusive {
			out = append(out, fmt.Sprintf("Run %s is inconclusive: %s.", x.RunID, x.Verdict.Inconclusive[0]))
		}
	}
	return out
}

func compareCharts(d *doc, a, b *analysis.Analysis) {
	n := max(len(a.Windows), len(b.Windows))
	get := func(x *analysis.Analysis, f func(*analysis.Window) *float64) []float64 {
		out := make([]float64, n)
		for i := range n {
			if i < len(x.Windows) {
				if v := f(x.Windows[i]); v != nil {
					out[i] = *v
				}
			}
		}
		return out
	}
	offered := func(w *analysis.Window) *float64 { v := w.Offered; return &v }
	rps := func(w *analysis.Window) *float64 { return w.RPS }
	p99 := func(w *analysis.Window) *float64 { return w.P99 }
	xMax := float64(max(1, n-1) * analysis.WindowSeconds)

	d.line("## Charts")
	d.line("")
	oa, ob, ra, rb := get(a, offered), get(b, offered), get(a, rps), get(b, rps)
	c := &mermaid.XYChart{Title: "Throughput", XLabel: "seconds", XMax: xMax, YLabel: "requests/s",
		YMax: niceCeil(math.Max(math.Max(maxOf(oa), maxOf(ob)), math.Max(maxOf(ra), maxOf(rb))) * 1.05)}
	c.Line(colOffered, oa)
	legend := "Grey: offered (A)"
	if !slices.Equal(oa, ob) {
		c.Line("#c7c7c7", ob)
		legend += ". Light grey: offered (B)"
	}
	c.Line(colServed, ra)
	c.Line(colP95, rb)
	chart(d, c, legend+". Blue: served, A. Orange: served, B ("+analysis.SrcRPS+"). Windows without data are drawn at 0.")

	pa, pb := get(a, p99), get(b, p99)
	c = &mermaid.XYChart{Title: "p99 latency, end to end", XLabel: "seconds", XMax: xMax, YLabel: "ms", YMax: niceCeil(math.Max(maxOf(pa), maxOf(pb)) * 1.05)}
	c.Line(colServed, pa)
	c.Line(colP95, pb)
	chart(d, c, "Blue: A. Orange: B ("+analysis.SrcK6Latency+"). Windows without data are drawn at 0.")
}

func compareWindows(d *doc, a, b *analysis.Analysis) {
	n := max(len(a.Windows), len(b.Windows))
	at := func(x *analysis.Analysis, i int) *analysis.Window {
		if i < len(x.Windows) {
			return x.Windows[i]
		}
		return nil
	}
	f := func(w *analysis.Window, get func(*analysis.Window) *float64, fm func(float64) string) string {
		if w == nil {
			return "–"
		}
		if v := get(w); v != nil {
			return fm(*v)
		}
		return "–"
	}
	offered := func(w *analysis.Window) *float64 { v := w.Offered; return &v }
	d.line("<details><summary>Per-window numbers, side by side</summary>")
	d.line("")
	d.line("| t | offered A | offered B | served A | served B | p99 A | p99 B | errors A | errors B |")
	d.line("| --- | --- | --- | --- | --- | --- | --- | --- | --- |")
	for i := range n {
		wa, wb := at(a, i), at(b, i)
		d.line("| %s | %s | %s | %s | %s | %s | %s | %s | %s |", analysis.Clock(float64(i*analysis.WindowSeconds)),
			f(wa, offered, analysis.Rate), f(wb, offered, analysis.Rate),
			f(wa, func(w *analysis.Window) *float64 { return w.RPS }, analysis.Rate), f(wb, func(w *analysis.Window) *float64 { return w.RPS }, analysis.Rate),
			f(wa, func(w *analysis.Window) *float64 { return w.P99 }, analysis.Ms), f(wb, func(w *analysis.Window) *float64 { return w.P99 }, analysis.Ms),
			f(wa, func(w *analysis.Window) *float64 { return w.ErrorRate }, analysis.Pct), f(wb, func(w *analysis.Window) *float64 { return w.ErrorRate }, analysis.Pct))
	}
	d.line("")
	d.line("</details>")
	d.line("")
}

func comparePredictions(d *doc, a, b *analysis.Analysis) {
	type pair struct{ a, b *analysis.ExpectResult }
	var keys []string
	byText := map[string]*pair{}
	add := func(x *analysis.Analysis, isA bool) {
		for i := range x.Expect {
			e := &x.Expect[i]
			k := e.Text
			if k == "" {
				k = "note: " + e.Note
			}
			p, ok := byText[k]
			if !ok {
				p = &pair{}
				byText[k] = p
				keys = append(keys, k)
			}
			if isA {
				p.a = e
			} else {
				p.b = e
			}
		}
	}
	add(a, true)
	add(b, false)
	if len(keys) == 0 {
		return
	}
	show := func(e *analysis.ExpectResult) string {
		switch {
		case e == nil:
			return "not in this design"
		case e.Status == analysis.ExpectNote:
			return "note"
		case e.Status == analysis.ExpectUnscored:
			return "not scored"
		}
		s := string(e.Status) + ", worst " + e.Shown
		if e.BrokeAt != nil {
			s += " (broke at " + analysis.Clock(*e.BrokeAt) + ")"
		}
		return s
	}
	d.line("## Predictions")
	d.line("")
	d.line("| Prediction | A | B |")
	d.line("| --- | --- | --- |")
	for _, k := range keys {
		p := byText[k]
		d.line("| `%s` | %s | %s |", cell(k), cell(show(p.a)), cell(show(p.b)))
	}
	d.line("")
}

// unifiedDiff is a line diff with three lines of context, or "" when the
// texts are equal.
func unifiedDiff(x, y, nameX, nameY string) string {
	if x == y {
		return ""
	}
	a := strings.Split(strings.TrimRight(x, "\n"), "\n")
	b := strings.Split(strings.TrimRight(y, "\n"), "\n")
	// Longest common subsequence table, from the end.
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	type op struct {
		kind byte // ' ', '-', '+'
		text string
		ai   int // line in a (0-based) for ' ' and '-'; next line in a for '+'
		bi   int
	}
	var ops []op
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			ops = append(ops, op{' ', a[i], i, j})
			i++
			j++
		case i < len(a) && (j == len(b) || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, op{'-', a[i], i, j})
			i++
		default:
			ops = append(ops, op{'+', b[j], i, j})
			j++
		}
	}
	const ctx = 3
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", nameX, nameY)
	for k := 0; k < len(ops); {
		if ops[k].kind == ' ' {
			k++
			continue
		}
		start := max(0, k-ctx)
		end := k
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].kind == ' ' {
				run++
			}
			if run-end > 2*ctx || run == len(ops) {
				end = min(len(ops), end+ctx)
				break
			}
			end = run
		}
		var na, nb int
		for _, o := range ops[start:end] {
			if o.kind != '+' {
				na++
			}
			if o.kind != '-' {
				nb++
			}
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", ops[start].ai+1, na, ops[start].bi+1, nb)
		for _, o := range ops[start:end] {
			out.WriteByte(o.kind)
			out.WriteString(o.text)
			out.WriteByte('\n')
		}
		k = end
	}
	return out.String()
}
