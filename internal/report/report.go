// Package report renders an analysis as a markdown report: verdict,
// annotated diagram, charts, diagnosis with evidence, scored predictions,
// assumptions and environment. Every number comes from the analysis, which
// takes it from the run folder, and the report names the metric behind it.
//
// Charts are Mermaid xychart blocks, so a report is one markdown file that
// renders on GitHub (READMEs, issues, PRs) without image files beside it.
package report

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/diagram"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/mermaid"
	"github.com/akinfemi/load-lore/internal/plan"
)

// FileName is the report's name in the run folder.
const FileName = "report.md"

// AnalysisFile is where the analysis behind the report is saved.
const AnalysisFile = "analysis.json"

// Write analyzes a run folder and writes report.md and analysis.json into it.
func Write(dir string) (*analysis.Analysis, error) {
	a, err := analysis.Analyze(dir)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(Markdown(a)), 0o644); err != nil {
		return a, err
	}
	b, err := jsonIndent(a)
	if err != nil {
		return a, err
	}
	return a, os.WriteFile(filepath.Join(dir, AnalysisFile), b, 0o644)
}

// Colors used across charts and diagrams.
const (
	colOffered = "#9e9e9e"
	colServed  = "#1f77b4"
	colP50     = "#2ca02c"
	colP95     = "#ff7f0e"
	colP99     = "#d62728"
	colErrors  = "#d62728"
	colMarker  = "#f4cccc"
)

var cpuColors = []string{"#1f77b4", "#ff7f0e", "#2ca02c", "#9467bd", "#8c564b", "#e377c2", "#17becf", "#bcbd22"}

type doc struct{ strings.Builder }

func (d *doc) p(format string, args ...any)    { fmt.Fprintf(d, format+"\n\n", args...) }
func (d *doc) line(format string, args ...any) { fmt.Fprintf(d, format+"\n", args...) }

// Markdown renders the report.
func Markdown(a *analysis.Analysis) string {
	var d doc
	d.line("# %s: run report", a.Name)
	d.line("")
	meta := []string{"run `" + a.RunID + "`", a.Status}
	if a.Started != nil {
		meta = append(meta, a.Started.UTC().Format("2 Jan 2006 15:04 UTC"))
	}
	meta = append(meta, fmt.Sprintf("scale %g", a.Scale))
	if a.Seed != 0 {
		meta = append(meta, fmt.Sprintf("seed %d", a.Seed))
	}
	d.p("_%s_", strings.Join(meta, " · "))

	verdictSection(&d, a)
	if len(a.Windows) > 0 {
		diagramSection(&d, a)
		chartsSection(&d, a)
	}
	diagnosisSection(&d, a)
	expectSection(&d, a)
	timelineSection(&d, a)
	assumptionsSection(&d, a)
	thresholdsSection(&d)
	environmentSection(&d, a)
	if len(a.Windows) > 0 {
		windowsSection(&d, a)
	}
	d.line("## Design file")
	d.line("")
	d.line("The exact `design.yaml` this run used:")
	d.line("")
	d.line("```yaml")
	d.WriteString(strings.TrimRight(a.DesignYAML, "\n") + "\n")
	d.line("```")
	return d.String()
}

func verdictSection(d *doc, a *analysis.Analysis) {
	v := a.Verdict
	label := map[analysis.VerdictKind]string{
		analysis.VerdictBroke: "Broke", analysis.VerdictHeld: "Held",
		analysis.VerdictInconclusive: "Inconclusive", analysis.VerdictNoData: "No data",
	}[v.Kind]
	d.line("## Verdict")
	d.line("")
	d.p("**%s.** %s", label, strings.TrimPrefix(v.Summary, "Inconclusive: "))
	if len(v.Inconclusive) > 1 {
		d.line("Other reasons the run is inconclusive:")
		d.line("")
		for _, r := range v.Inconclusive[1:] {
			d.line("- %s", r)
		}
		d.line("")
	}
	for _, c := range v.Caveats {
		d.p("Caveat: %s.", c)
	}
	var src []string
	if v.Break != nil || v.Kind == analysis.VerdictHeld || v.Kind == analysis.VerdictInconclusive {
		src = append(src, "offered load: "+analysis.SrcOffered, "served: "+analysis.SrcRPS, "p99: "+analysis.SrcK6Latency, "errors: "+analysis.SrcErrors)
		if b := v.Break; b != nil {
			if top, ok := analysis.MostSaturated(b.Saturation); ok {
				src = append(src, "saturation: "+top.Source)
			}
		}
		d.p("<sub>Sources: %s. Windows are %d seconds; what counts as broken is under [Thresholds](#thresholds).</sub>", strings.Join(src, "; "), analysis.WindowSeconds)
	}
	for _, r := range a.DataReuse {
		d.p("> **Data reused (`--reuse-data`).** %s started from data that %d earlier %s wrote to, not a fresh copy, so its starting data drifted: %s.",
			r.Service, len(r.EarlierRuns), plural(len(r.EarlierRuns), "run", "runs"), strings.Join(append([]string{}, r.EarlierRuns...), ", "))
	}
	for _, as := range a.Assumptions.Loadlore {
		if strings.HasSuffix(as.Field, ".replicas") {
			name := strings.TrimSuffix(strings.TrimPrefix(as.Field, "services."), ".replicas")
			d.p("> **No load balancer in front of %s.** Requests were spread across its %s replicas client-side, with no health checks: a dead or slow replica kept getting its share.", name, as.Value)
		}
	}
}

// --- annotated diagram ---

func diagramSection(d *doc, a *analysis.Analysis) {
	m := a.Verdict.Moment
	if m < 0 || m >= len(a.Windows) {
		return
	}
	w := a.Windows[m]
	why := "the busiest window: the most successful responses"
	if a.Verdict.Break != nil {
		why = "the break"
	}
	d.line("## Diagram at %s", analysis.Clock(w.Start))
	d.line("")
	scaled := ""
	if a.Scale != 1 {
		scaled = fmt.Sprintf(" Resources and rates in the first lines of each box are as the design states them; this run had them multiplied by --scale %g.", a.Scale)
	}
	d.p("Measured in the 10-second window from %s to %s (%s). Nodes are colored by their most saturated resource, CPU against its limit or a connection pool's connections in use: red at 90%% or more, amber from 70%%, green below.%s",
		analysis.Clock(w.Start), analysis.Clock(w.Start+analysis.WindowSeconds), why, scaled)
	d.line("```mermaid")
	d.WriteString(diagram.Annotated(a.Design, annotations(a, w)).String())
	d.line("```")
	d.line("")
	src := []string{"offered from " + analysis.SrcOffered, "load edge from " + analysis.SrcCompleted + " and " + analysis.SrcK6Latency, "CPU from " + analysis.SrcCPU}
	kinds := map[string]bool{}
	for _, s := range a.Plan.Services {
		kinds[s.Kind] = true
	}
	if kinds["app"] {
		src = append(src, "calls/s from "+analysis.SrcCalls+" and their p99 from "+analysis.SrcCallLatency)
	}
	if len(w.Pools) > 0 {
		src = append(src, "pools from "+analysis.SrcPoolInUse+" and "+analysis.SrcPoolPending)
	}
	if kinds["lb"] {
		src = append(src, "lb traffic from "+analysis.SrcLBRequests+", replicas up from "+analysis.SrcLBHealthy+", and the lb edge's p99 from "+analysis.SrcAppLatency)
	}
	if kinds["postgres"] {
		src = append(src, "conns from "+analysis.SrcPGConns)
	}
	if kinds["redis"] {
		src = append(src, "hit rate from "+analysis.SrcHitRate)
	}
	d.p("<sub>Sources: %s.</sub>", strings.Join(src, "; "))
}

func annotations(a *analysis.Analysis, w *analysis.Window) *diagram.Annotations {
	an := &diagram.Annotations{
		Nodes: map[string][]string{}, Edges: map[[2]string][]string{}, Classes: map[string]string{},
		ClassDefs: [][2]string{
			{"ok", "fill:#d4edda,stroke:#28a745,color:#000"},
			{"warm", "fill:#fff3cd,stroke:#d39e00,color:#000"},
			{"hot", "fill:#f8d7da,stroke:#dc3545,color:#000"},
		},
	}
	an.Nodes[diagram.LoadNode] = []string{"offered " + analysis.Rate(w.Offered) + " rps"}
	for _, s := range a.Plan.Services {
		sw := w.Services[s.Name]
		if sw == nil {
			continue
		}
		var lines []string
		sat, known := 0.0, false
		if sw.CPU != nil {
			lines = append(lines, "CPU "+analysis.Pct(*sw.CPU))
			sat, known = *sw.CPU, true
		}
		for _, p := range w.Pools {
			if p.Service == s.Name && p.InUse != nil && p.Max != nil && *p.Max > 0 {
				sat, known = math.Max(sat, *p.InUse / *p.Max), true
			}
		}
		if known {
			switch {
			case sat >= 0.9:
				an.Classes[s.Name] = "hot"
			case sat >= 0.7:
				an.Classes[s.Name] = "warm"
			default:
				an.Classes[s.Name] = "ok"
			}
		}
		switch {
		case sw.Connections != nil && sw.MaxConnections != nil:
			lines = append(lines, fmt.Sprintf("conns %s/%s", analysis.Count(*sw.Connections), analysis.Count(*sw.MaxConnections)))
		case sw.HitRate != nil:
			lines = append(lines, "hit rate "+analysis.Pct(*sw.HitRate))
		case sw.Healthy != nil && s.LB != nil:
			lines = append(lines, fmt.Sprintf("%s/%d replicas up", analysis.Count(*sw.Healthy), len(s.LB.Servers)))
		case sw.Inflight != nil:
			lines = append(lines, "in flight "+analysis.Count(*sw.Inflight))
		}
		an.Nodes[s.Name] = lines
	}
	for _, e := range w.Edges {
		var parts []string
		unit := " calls/s"
		if e.From == "load" || isLB(a.Plan, e.From) {
			unit = " rps"
		}
		if e.RPS != nil {
			parts = append(parts, analysis.Rate(*e.RPS)+unit)
		}
		if e.P99 != nil {
			parts = append(parts, "p99 "+analysis.Ms(*e.P99))
		}
		if e.ErrorRate != nil && *e.ErrorRate > 0 {
			parts = append(parts, analysis.Pct(*e.ErrorRate)+" errors")
		}
		var lines []string
		if len(parts) > 0 {
			lines = append(lines, strings.Join(parts, " · "))
		}
		for _, p := range w.Pools {
			if p.Service == e.From && p.Target == e.To && p.InUse != nil && p.Max != nil {
				l := fmt.Sprintf("in use %s/%s", analysis.Count(*p.InUse), analysis.Count(*p.Max))
				if p.Pending != nil && *p.Pending > 0 {
					l += ", " + analysis.Count(*p.Pending) + " waiting"
				}
				lines = append(lines, l)
			}
		}
		from := e.From
		if from == "load" {
			from = diagram.LoadNode
		}
		an.Edges[[2]string{from, e.To}] = lines
	}
	return an
}

func isLB(p *plan.Plan, name string) bool {
	s := p.Service(name)
	return s != nil && s.Kind == "lb"
}

// --- charts ---

type marker struct {
	window int
	label  string
}

// markers are the windows where something happened: faults, container
// deaths, the break.
func markers(a *analysis.Analysis) []marker {
	var out []marker
	for _, e := range a.Timeline {
		i := int(math.Floor(e.At / analysis.WindowSeconds))
		if i < 0 || i >= len(a.Windows) || e.Kind == "fault_end" {
			continue
		}
		out = append(out, marker{i, analysis.Clock(e.At) + " " + e.Detail})
	}
	if b := a.Verdict.Break; b != nil {
		out = append(out, marker{b.Window, analysis.Clock(b.At) + " break"})
	}
	return out
}

func chartsSection(d *doc, a *analysis.Analysis) {
	ws := a.Windows
	ms := markers(a)
	// Points are evenly spaced from XMin to XMax, so each sits at its
	// window's start.
	xMax := float64(max(1, len(ws)-1) * analysis.WindowSeconds)
	// Markers are a line that spikes to the top of the chart at each
	// marked window. Mermaid draws a bar at least a few pixels tall even at
	// 0, and as wide as the axis's tick spacing rather than one window, so
	// bars would smear across neighbouring windows.
	mark := func(c *mermaid.XYChart) {
		if len(ms) == 0 {
			return
		}
		spikes := make([]float64, len(ws))
		for _, m := range ms {
			spikes[m.window] = c.YMax
		}
		c.Line(colMarker, spikes)
	}
	var labels []string
	for _, m := range ms {
		labels = append(labels, m.label)
	}
	markNote := ""
	if len(ms) > 0 {
		markNote = " Pink spikes mark " + strings.Join(labels, "; ") + "."
	}
	series := func(get func(*analysis.Window) *float64, scale float64) ([]float64, bool) {
		out := make([]float64, len(ws))
		gaps := false
		for i, w := range ws {
			if v := get(w); v != nil {
				out[i] = *v * scale
			} else {
				gaps = true
			}
		}
		return out, gaps
	}
	gapNote := func(gaps bool) string {
		if gaps {
			return " Windows without data are drawn at 0."
		}
		return ""
	}

	d.line("## Charts")
	d.line("")
	d.p("One point per %d-second window, at the window's start; x is seconds from the start of the load.", analysis.WindowSeconds)

	// Throughput.
	offered := make([]float64, len(ws))
	for i, w := range ws {
		offered[i] = w.Offered
	}
	served, gaps := series(func(w *analysis.Window) *float64 { return w.RPS }, 1)
	yMax := niceCeil(math.Max(maxOf(offered), maxOf(served)) * 1.05)
	c := &mermaid.XYChart{Title: "Throughput", XLabel: "seconds", XMax: xMax, YLabel: "requests/s", YMax: yMax}
	mark(c)
	c.Line(colOffered, offered)
	c.Line(colServed, served)
	chart(d, c, "Grey: offered load ("+analysis.SrcOffered+"). Blue: successful responses ("+analysis.SrcRPS+")."+markNote+gapNote(gaps))

	// Latency.
	p50, g1 := series(func(w *analysis.Window) *float64 { return w.P50 }, 1)
	p95, g2 := series(func(w *analysis.Window) *float64 { return w.P95 }, 1)
	p99, g3 := series(func(w *analysis.Window) *float64 { return w.P99 }, 1)
	yMax = niceCeil(maxOf(p99) * 1.05)
	c = &mermaid.XYChart{Title: "Latency, end to end", XLabel: "seconds", XMax: xMax, YLabel: "ms", YMax: yMax}
	mark(c)
	c.Line(colP50, p50)
	c.Line(colP95, p95)
	c.Line(colP99, p99)
	chart(d, c, "Green: p50. Orange: p95. Red: p99 ("+analysis.SrcK6Latency+")."+markNote+gapNote(g1 || g2 || g3))

	// Errors.
	errs, g := series(func(w *analysis.Window) *float64 { return w.ErrorRate }, 100)
	yMax = niceCeil(math.Max(maxOf(errs)*1.05, 1))
	c = &mermaid.XYChart{Title: "Errors", XLabel: "seconds", XMax: xMax, YLabel: "% of requests", YMax: yMax}
	mark(c)
	c.Line(colErrors, errs)
	chart(d, c, "Failed requests as a share of all responses ("+analysis.SrcErrors+"), timeouts included."+markNote+gapNote(g))

	// CPU.
	c = &mermaid.XYChart{Title: "CPU against each limit", XLabel: "seconds", XMax: xMax, YLabel: "% of limit", YMax: 100}
	var legend []string
	var lines []struct {
		col  string
		vals []float64
	}
	gaps = false
	cpuMax := 0.0
	for i, s := range a.Plan.Services {
		name := s.Name
		vals, g := series(func(w *analysis.Window) *float64 {
			if sw := w.Services[name]; sw != nil {
				return sw.CPU
			}
			return nil
		}, 100)
		if maxOf(vals) == 0 && g {
			continue
		}
		col := cpuColors[i%len(cpuColors)]
		lines = append(lines, struct {
			col  string
			vals []float64
		}{col, vals})
		cpuMax = math.Max(cpuMax, maxOf(vals))
		legend = append(legend, colorName(col)+": "+name)
		gaps = gaps || g
	}
	if cpuMax > 100.5 {
		c.YMax = niceCeil(cpuMax * 1.05)
	}
	mark(c)
	for _, l := range lines {
		c.Line(l.col, l.vals)
	}
	chart(d, c, strings.Join(legend, ". ")+" ("+analysis.SrcCPU+", mean over the window)."+markNote+gapNote(gaps))
}

func chart(d *doc, c *mermaid.XYChart, caption string) {
	d.line("```mermaid")
	d.WriteString(c.String())
	d.line("```")
	d.line("")
	d.p("<sub>%s</sub>", caption)
}

var colorNames = map[string]string{
	"#1f77b4": "Blue", "#ff7f0e": "Orange", "#2ca02c": "Green", "#9467bd": "Purple",
	"#8c564b": "Brown", "#e377c2": "Pink", "#17becf": "Cyan", "#bcbd22": "Olive",
}

func colorName(c string) string { return colorNames[c] }

func maxOf(xs []float64) float64 {
	m := 0.0
	for _, x := range xs {
		m = math.Max(m, x)
	}
	return m
}

// niceCeil rounds up to a round number of a power of ten, so axes end
// near the data.
func niceCeil(v float64) float64 {
	if v <= 0 {
		return 1
	}
	p := math.Pow(10, math.Floor(math.Log10(v)))
	for _, m := range []float64{1, 1.2, 1.5, 2, 2.5, 3, 4, 5, 6, 8, 10} {
		if m*p >= v {
			return m * p
		}
	}
	return 10 * p
}

// --- diagnosis ---

func diagnosisSection(d *doc, a *analysis.Analysis) {
	d.line("## Diagnosis")
	d.line("")
	if len(a.Diagnoses) == 0 {
		d.p("No diagnosis rule fired. Rules checked: connection pool exhausted, queueing knee (rules version %s; thresholds below).", analysis.RuleVersion)
		return
	}
	for i, x := range a.Diagnoses {
		d.line("### %d. %s, from %s", i+1, x.Title, analysis.Clock(x.From))
		d.line("")
		d.p("%s", x.Summary)
		d.line("Evidence:")
		d.line("")
		for _, e := range x.Evidence {
			d.line("- %s <sub>(%s)</sub>", e.Text, e.Source)
		}
		d.line("")
		d.p("**Try next:** %s", x.Next)
	}
	d.p("<sub>Rules version %s. Rules are deterministic and offline; their thresholds are listed below.</sub>", analysis.RuleVersion)
}

// --- predictions ---

func expectSection(d *doc, a *analysis.Analysis) {
	d.line("## Predictions")
	d.line("")
	if len(a.Expect) == 0 {
		d.p("The design has no `expect` lines. Writing a prediction before a run turns it into a test of your mental model.")
		return
	}
	held, scored := 0, 0
	for _, e := range a.Expect {
		switch e.Status {
		case analysis.ExpectHeld:
			held++
			scored++
		case analysis.ExpectBroken:
			scored++
		}
	}
	d.p("%d of %d scored %s held.", held, scored, plural(scored, "prediction", "predictions"))
	d.line("| Prediction | Result | Worst value | Checked over | Notes |")
	d.line("| --- | --- | --- | --- | --- |")
	for _, e := range a.Expect {
		if e.Status == analysis.ExpectNote {
			d.line("| %s | note, not scored | | | |", cell(e.Note))
			continue
		}
		result := map[analysis.ExpectStatus]string{analysis.ExpectHeld: "**held**", analysis.ExpectBroken: "**broken**", analysis.ExpectUnscored: "not scored"}[e.Status]
		if e.BrokeAt != nil {
			result += " at " + analysis.Clock(*e.BrokeAt)
		}
		notes := e.Message
		if e.Source != "" {
			notes = strings.TrimPrefix(strings.Join([]string{notes, "source: " + e.Source}, "; "), "; ")
		}
		d.line("| `%s` | %s | %s | %s | %s |", e.Text, result, cell(e.Shown), cell(e.Range), cell(notes))
	}
	d.line("")
}

// --- timeline ---

func timelineSection(d *doc, a *analysis.Analysis) {
	if len(a.Timeline) == 0 {
		return
	}
	d.line("## Timeline")
	d.line("")
	d.line("| Time | Event |")
	d.line("| --- | --- |")
	for _, e := range a.Timeline {
		kind := map[string]string{"fault_start": "fault started", "fault_end": "fault ended", "fault_error": "fault failed", "container": "container"}[e.Kind]
		d.line("| %s | %s: %s |", analysis.Clock(e.At), kind, cell(e.Detail))
	}
	d.line("")
	d.p("<sub>From events.jsonl: the fault timeline and Docker container events.</sub>")
}

// --- assumptions ---

func assumptionsSection(d *doc, a *analysis.Analysis) {
	d.line("## Assumptions")
	d.line("")
	d.line("### From the design")
	d.line("")
	if len(a.Assumptions.Design) == 0 {
		d.p("The design lists no assumptions.")
	} else {
		d.line("| Field | Value | Basis | Why |")
		d.line("| --- | --- | --- | --- |")
		for _, as := range a.Assumptions.Design {
			d.line("| `%s` | %s | %s | %s |", as.Field, cell(as.Value.String()), as.Basis, cell(as.Why))
		}
		d.line("")
		guessed := 0
		for _, as := range a.Assumptions.Design {
			if as.Basis == "guessed" {
				guessed++
			}
		}
		if guessed > 0 {
			d.p("%d of these %s guessed, not measured: the result depends on them.", guessed, plural(guessed, "is", "are"))
		}
	}
	d.line("### Chosen by Loadlore")
	d.line("")
	if len(a.Assumptions.Loadlore) == 0 {
		d.p("None recorded (runs from loadl before M4 don't record them).")
	} else {
		d.line("| Field | Value | Why |")
		d.line("| --- | --- | --- |")
		for _, as := range a.Assumptions.Loadlore {
			d.line("| %s | %s | %s |", cell(as.Field), cell(as.Value), cell(as.Why))
		}
		d.line("")
	}
	if len(a.Notes) > 0 {
		d.line("### How the run was planned")
		d.line("")
		for _, n := range a.Notes {
			d.line("- %s", n)
		}
		d.line("")
	}
}

func thresholdsSection(d *doc) {
	lat := fmt.Sprintf("%gx the low-load p99 (the median p99 of windows offered at most %s of the peak) and at least %g ms", analysis.BreakLatencyFactor, analysis.Pct(analysis.BaselineShare), analysis.BreakLatencyMinMs)
	d.line("## Thresholds")
	d.line("")
	d.line("- **Broke:** two windows in a row with p99 at least %s, at least %s of requests failing, or successful responses below %s of the offered rate.", lat, analysis.Pct(analysis.BreakErrorRate), analysis.Pct(analysis.BreakShortfall))
	d.line("- **Inconclusive:** before the break (or anywhere, if nothing broke), the load generator dropped more than %s of a window's arrivals, or the load generator or Toxiproxy used %s or more of its CPU budget, or was stalled at its CPU limit in %s or more of its scheduling periods (from %s it's noted as a caveat). The first %g s of load are a warm-up and don't count: the load generator starts its VUs and connections open then. A cause is named only with evidence: on a Linux host, the hypervisor giving %s or more of CPU time to other virtual machines (steal), or other programs using %s or more of the host's CPUs (at least %g); inside a VM on this machine, only as a possibility.", analysis.Pct(analysis.DroppedShareLimit), analysis.Pct(analysis.ToolCPULimit), analysis.Pct(analysis.ToolThrottleLimit), analysis.Pct(analysis.ToolThrottleNote), analysis.ToolWarmupSeconds, analysis.Pct(analysis.HostStealLimit), analysis.Pct(analysis.HostOthersShare), analysis.HostOthersMin)
	d.line("- **Connection pool exhausted:** for two windows in a row, a pool at %s or more of its size with callers waiting, while the target's CPU stayed below %s and the caller's below %s.", analysis.Pct(analysis.PoolFullShare), analysis.Pct(analysis.PoolTargetCPUMax), analysis.Pct(analysis.PoolCallerCPUMax))
	d.line("- **Queueing knee:** for %d windows in a row, p99 at least %gx the low-load p99 and %g ms above it, with a CPU or pool at %s or more.", analysis.KneeWindows, analysis.KneeLatencyFactor, analysis.KneeLatencyMinMs, analysis.Pct(analysis.KneeUtilization))
	d.line("")
}

// --- environment ---

func environmentSection(d *doc, a *analysis.Analysis) {
	e := a.Environment
	d.line("## Environment")
	d.line("")
	d.line("- loadl %s, profile %s, scale %g, seed %d", e.Loadl, e.Duration, a.Scale, a.Seed)
	if h := e.Host; h != nil {
		d.line("- Host: %d CPUs, %s memory, %s (%s, kernel %s), cgroup v%s", h.CPUs, gib(h.MemoryBytes), h.OS, h.Arch, h.Kernel, h.CgroupVersion)
		d.line("- Docker %s, Compose %s", h.EngineVersion, h.ComposeVersion)
		if sp := h.Speed; sp != nil {
			d.line("- Host speed index %d (hash %d, memory %d, syscalls %d): one CPU's work against the reference machine's 1000 (%s). CPU limits are fractions of this host's cores, so a lower index means the design breaks at a lower load", sp.Index, sp.Hash, sp.Memory, sp.Syscalls, bench.ReferenceMachine)
		} else {
			d.line("- Host speed index: not measured")
		}
		switch h.HostCPU {
		case driver.HostCPUSampled:
			d.line("- Host CPU: sampled from /proc/stat of the kernel Docker runs on, for steal and other programs' use")
		case driver.HostCPUInVM:
			d.line("- Host CPU: Docker runs in a VM on this machine, so other programs' use of the CPU can't be seen")
		}
	}
	if a.ReuseData {
		d.line("- Data: reused with --reuse-data (see the verdict)")
	} else {
		d.line("- Data: a fresh copy of the cached seeded dataset")
	}
	if l := e.Load; l != nil {
		d.line("- Load generator summary (k6): %s requests, %s dropped arrivals, %s failed", analysis.Count(l.Requests), analysis.Count(l.DroppedIterations), analysis.Pct(l.FailedShare))
	}
	d.line("")
	if len(e.Images) > 0 {
		names := make([]string, 0, len(e.Images))
		for n := range e.Images {
			names = append(names, n)
		}
		slices.Sort(names)
		d.line("<details><summary>Containers and images</summary>")
		d.line("")
		d.line("| Container | Image |")
		d.line("| --- | --- |")
		for _, n := range names {
			d.line("| %s | %s |", n, e.Images[n])
		}
		d.line("")
		d.line("</details>")
		d.line("")
	}
	if len(a.Warnings) > 0 {
		d.line("Warnings:")
		d.line("")
		for _, w := range a.Warnings {
			d.line("- %s", w)
		}
		d.line("")
	}
}

func jsonIndent(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	return append(b, '\n'), err
}

func gib(b int64) string { return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30)) }

// --- per-window table ---

func windowsSection(d *doc, a *analysis.Analysis) {
	type col struct {
		head string
		get  func(*analysis.Window) string
	}
	opt := func(p *float64, f func(float64) string) string {
		if p == nil {
			return "–"
		}
		return f(*p)
	}
	cols := []col{
		{"t", func(w *analysis.Window) string { return analysis.Clock(w.Start) }},
		{"offered", func(w *analysis.Window) string { return analysis.Rate(w.Offered) }},
		{"served", func(w *analysis.Window) string { return opt(w.RPS, analysis.Rate) }},
		{"dropped", func(w *analysis.Window) string { return opt(w.Dropped, analysis.Count) }},
		{"errors", func(w *analysis.Window) string { return opt(w.ErrorRate, analysis.Pct) }},
		{"p50", func(w *analysis.Window) string { return opt(w.P50, analysis.Ms) }},
		{"p99", func(w *analysis.Window) string { return opt(w.P99, analysis.Ms) }},
	}
	for _, s := range a.Plan.Services {
		name := s.Name
		cols = append(cols, col{name + " CPU", func(w *analysis.Window) string {
			if sw := w.Services[name]; sw != nil {
				return opt(sw.CPU, analysis.Pct)
			}
			return "–"
		}})
		switch s.Kind {
		case "postgres":
			cols = append(cols, col{name + " conns", func(w *analysis.Window) string {
				if sw := w.Services[name]; sw != nil {
					return opt(sw.Connections, analysis.Count)
				}
				return "–"
			}})
		case "redis":
			cols = append(cols, col{name + " hit rate", func(w *analysis.Window) string {
				if sw := w.Services[name]; sw != nil {
					return opt(sw.HitRate, analysis.Pct)
				}
				return "–"
			}})
		case "lb":
			cols = append(cols, col{name + " up", func(w *analysis.Window) string {
				if sw := w.Services[name]; sw != nil {
					return opt(sw.Healthy, analysis.Count)
				}
				return "–"
			}})
		}
	}
	if len(a.Windows) > 0 {
		for _, p := range a.Windows[0].Pools {
			svc, tgt := p.Service, p.Target
			find := func(w *analysis.Window) *analysis.PoolWindow {
				for _, x := range w.Pools {
					if x.Service == svc && x.Target == tgt {
						return x
					}
				}
				return nil
			}
			cols = append(cols, col{svc + "→" + tgt + " pool", func(w *analysis.Window) string {
				x := find(w)
				if x == nil || x.InUse == nil || x.Max == nil {
					return "–"
				}
				s := analysis.Count(*x.InUse) + "/" + analysis.Count(*x.Max)
				if x.Pending != nil && *x.Pending > 0 {
					s += " +" + analysis.Count(*x.Pending)
				}
				return s
			}}, col{"wait", func(w *analysis.Window) string {
				if x := find(w); x != nil {
					return opt(x.WaitMs, analysis.Ms)
				}
				return "–"
			}})
		}
	}
	cols = append(cols,
		col{"k6 CPU", func(w *analysis.Window) string { return opt(w.LoadCPU, analysis.Pct) }},
		col{"Toxiproxy CPU", func(w *analysis.Window) string { return opt(w.ProxyCPU, analysis.Pct) }},
		col{"k6 / Toxiproxy stalled", func(w *analysis.Window) string {
			return opt(w.LoadThrottled, analysis.Pct) + " / " + opt(w.ProxyThrottled, analysis.Pct)
		}},
	)
	hostSampled := slices.ContainsFunc(a.Windows, func(w *analysis.Window) bool { return w.HostSteal != nil })
	if hostSampled {
		cols = append(cols,
			col{"host steal", func(w *analysis.Window) string { return opt(w.HostSteal, analysis.Pct) }},
			col{"other CPUs", func(w *analysis.Window) string {
				return opt(w.HostOthers, func(v float64) string { return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64) })
			}},
		)
	}

	d.line("## Per-window data")
	d.line("")
	d.line("<details><summary>%d windows of %d seconds</summary>", len(a.Windows), analysis.WindowSeconds)
	d.line("")
	heads := make([]string, len(cols))
	seps := make([]string, len(cols))
	for i, c := range cols {
		heads[i], seps[i] = c.head, "---"
	}
	d.line("| %s |", strings.Join(heads, " | "))
	d.line("| %s |", strings.Join(seps, " | "))
	for _, w := range a.Windows {
		vals := make([]string, len(cols))
		for i, c := range cols {
			vals[i] = c.get(w)
		}
		d.line("| %s |", strings.Join(vals, " | "))
	}
	d.line("")
	d.line("Columns: offered is %s; served is %s; dropped is %s; errors are %s; p50 and p99 are from %s; CPU is %s, mean over the window; conns are %s; hit rate is from %s; up is %s; pools are %s (peak in use/size, +peak waiting), wait is the mean from %s; k6 and Toxiproxy CPU are against their budgets.",
		analysis.SrcOffered, analysis.SrcRPS, analysis.SrcDropped, analysis.SrcErrors, analysis.SrcK6Latency, analysis.SrcCPU, analysis.SrcPGConns, analysis.SrcHitRate, analysis.SrcLBHealthy, analysis.SrcPoolInUse, analysis.SrcPoolWait)
	if hostSampled {
		d.line("")
		d.line("Host steal is %s; other CPUs are %s.", analysis.SrcHostSteal, analysis.SrcHostOthers)
	}
	d.line("")
	d.line("</details>")
	d.line("")
}

// --- helpers ---

func cell(s string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ").Replace(s)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
