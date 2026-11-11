package report

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/bench"
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
	if lead := tieLead(a, b); lead != "" {
		d.p("%s", lead)
	}

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
		s := fmt.Sprintf("broke at %s (%s rps offered)", analysis.Clock(v.Break.At), analysis.Rate(v.Break.Offered))
		if v.Doubt != "" {
			s += ", doubtful"
		}
		return s
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

// Hosts count as the same size within HostMemoryTolerance: a VM's
// reported memory moves by a few KiB between runs.
const HostMemoryTolerance = 0.05

func apart(x, y, share float64) bool {
	return math.Abs(x-y) > share*math.Max(x, y)
}

// speedCaveat says when two runs' host speed indexes are more than
// bench.Tolerance apart, or "" when they aren't or one is missing.
func speedCaveat(x, y *bench.Speed, order string) string {
	if x == nil || y == nil || bench.Gap(x.Index, y.Index) <= bench.Tolerance {
		return ""
	}
	return fmt.Sprintf("The host ran at different speeds: speed index %d and %d (%s), %s apart, more than %s. A machine slows as it heats up or on battery, and CPU limits are fractions of its cores, so the slower run can break at a lower load for that reason alone.",
		x.Index, y.Index, order, analysis.Pct(bench.Gap(x.Index, y.Index)), analysis.Pct(bench.Tolerance))
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
	if ha, hb := a.Environment.Host, b.Environment.Host; ha != nil && hb != nil {
		var diff []string
		if ha.CPUs != hb.CPUs {
			diff = append(diff, fmt.Sprintf("%d CPUs and %d", ha.CPUs, hb.CPUs))
		}
		if apart(float64(ha.MemoryBytes), float64(hb.MemoryBytes), HostMemoryTolerance) {
			diff = append(diff, fmt.Sprintf("%s and %s of memory", gib(ha.MemoryBytes), gib(hb.MemoryBytes)))
		}
		if len(diff) > 0 {
			out = append(out, "The runs ran on different hosts: "+strings.Join(diff, ", ")+" (A, then B). CPU limits are fractions of the host's cores, so the same design can break at a different load.")
		}
		if c := speedCaveat(ha.Speed, hb.Speed, "A, then B"); c != "" {
			out = append(out, c)
		}
	}
	out = append(out, spreadCaveats(a, b)...)
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

// tieLead says how two runs with the same verdict differ, since the
// verdict alone would make them look alike: the most each served, and
// around a shared break, how much of the offered load each served after
// it and its p99 at the break.
func tieLead(a, b *analysis.Analysis) string {
	if shortVerdict(a) != shortVerdict(b) || a.Verdict.MaxRPS == nil || b.Verdict.MaxRPS == nil {
		return ""
	}
	ra, rb := *a.Verdict.MaxRPS, *b.Verdict.MaxRPS
	parts := []string{fmt.Sprintf("B served at most %s rps against A's %s (%s)", analysis.Rate(rb), analysis.Rate(ra), ratio(rb, ra))}
	ba, bb := a.Verdict.Break, b.Verdict.Break
	if ba != nil && bb != nil {
		sa, oka := servedShare(a, ba.Window)
		sb, okb := servedShare(b, bb.Window)
		if oka && okb {
			parts = append(parts, fmt.Sprintf("from the break on, B served %s of the offered load against A's %s", analysis.Pct(sb), analysis.Pct(sa)))
		}
		if ba.P99 != nil && bb.P99 != nil {
			parts = append(parts, fmt.Sprintf("p99 at the break was %s against %s", analysis.Ms(*bb.P99), analysis.Ms(*ba.P99)))
		}
	} else if a.Verdict.WorstP99 != nil && b.Verdict.WorstP99 != nil {
		parts = append(parts, fmt.Sprintf("B's worst 10-second p99 was %s against A's %s", analysis.Ms(*b.Verdict.WorstP99), analysis.Ms(*a.Verdict.WorstP99)))
	}
	both := "Both runs have the same verdict"
	switch {
	case ba != nil:
		both = fmt.Sprintf("Both broke at %s (%s rps offered)", analysis.Clock(ba.At), analysis.Rate(ba.Offered))
	case a.Verdict.Kind == analysis.VerdictHeld:
		both = "Both held the whole profile"
	}
	return fmt.Sprintf("**Same verdict, different results.** %s, but %s.\n\n<sub>Served: %s; offered: %s; p99: %s.</sub>",
		both, strings.Join(parts, "; "), analysis.SrcRPS, analysis.SrcOffered, analysis.SrcK6Latency)
}

func ratio(x, y float64) string {
	if y <= 0 {
		return "no ratio"
	}
	return fmt.Sprintf("%.2f×", x/y)
}

// servedShare is successful responses over offered arrivals from window i
// to the end of the analyzed windows.
func servedShare(x *analysis.Analysis, i int) (float64, bool) {
	var served, offered float64
	for _, w := range x.Windows[i:min(x.Analyzed, len(x.Windows))] {
		if w.RPS == nil {
			return 0, false
		}
		served += *w.RPS
		offered += w.Offered
	}
	return served / offered, offered > 0
}
