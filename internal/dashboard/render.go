package dashboard

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/runner"
)

// Saturation bands, as the report colors its diagram.
const (
	hotShare  = 0.9
	warmShare = 0.7
)

var (
	styleTitle = lipgloss.NewStyle().Bold(true)
	styleDim   = lipgloss.NewStyle().Faint(true)
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleWarm  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleHot   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleBreak = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleFault = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
)

func bySaturation(share float64, ok bool) lipgloss.Style {
	switch {
	case !ok:
		return styleDim
	case share >= hotShare:
		return styleHot
	case share >= warmShare:
		return styleWarm
	}
	return styleOK
}

// Render draws the dashboard for a snapshot, width columns wide. hint is
// the key help on the title line.
func Render(s *Snapshot, width int, hint string) string {
	width = max(width, 60)
	var b strings.Builder
	st := s.Status
	var last *driver.Sample
	if n := len(s.Samples); n > 0 {
		last = s.Samples[n-1].Sample
	}

	// Title and where the run is.
	where := phaseText(st)
	if st.Phase == runner.PhaseLoad {
		where = fmt.Sprintf("load %s of %s", analysis.Clock(st.ElapsedS), analysis.Clock(st.ProfileS))
	}
	title := styleTitle.Render("loadl "+st.RunID) + "  " + where
	pad := width - lipgloss.Width(title) - lipgloss.Width(hint)
	b.WriteString(title + strings.Repeat(" ", max(1, pad)) + styleDim.Render(hint) + "\n")
	if st.BreakConfirmedS != nil && st.BreakAtS != nil && st.StopsAtS != nil && st.Phase != runner.PhaseFinished {
		b.WriteString(styleBreak.Render(fmt.Sprintf("break confirmed at %s (from %s); the load stops at %s", analysis.Clock(*st.BreakConfirmedS), analysis.Clock(*st.BreakAtS), analysis.Clock(*st.StopsAtS))) + "\n")
	}
	if st.Phase == runner.PhaseFinished {
		for _, l := range wrap(strings.ToUpper(st.Verdict)+". "+st.VerdictSummary, width) {
			b.WriteString(l + "\n")
		}
		if st.Report != "" {
			b.WriteString(styleDim.Render("report: "+st.Report) + "\n")
		}
	}
	b.WriteString("\n")

	if s.Plan != nil {
		b.WriteString(styleTitle.Render("Topology") + "\n")
		for _, l := range topology(s.Plan, last) {
			b.WriteString("  " + l + "\n")
		}
		b.WriteString("\n")
	}

	if len(s.Samples) > 0 {
		b.WriteString(styleTitle.Render("Load") + "\n")
		charts(&b, s, width)
		b.WriteString("\n")
		if last != nil && s.Plan != nil {
			b.WriteString(styleTitle.Render("Components") + "  " + styleDim.Render("last 10 s") + "\n")
			components(&b, s.Plan, last)
			b.WriteString("\n")
		}
	}

	if len(s.Events) > 0 {
		b.WriteString(styleTitle.Render("Events") + "\n")
		evs := s.Events[max(0, len(s.Events)-5):]
		for _, e := range evs {
			fmt.Fprintf(&b, "  %s  %s\n", analysis.Clock(e.AtS), map[string]string{"fault_start": "fault: ", "fault_end": "fault ended: ", "load_stop": ""}[e.Kind]+e.Detail)
		}
	}
	return b.String()
}

func phaseText(st runner.Status) string {
	switch st.Phase {
	case runner.PhasePreflight:
		return "checking the host"
	case runner.PhaseStarting:
		return "starting the stack"
	case runner.PhaseCollecting:
		return "collecting metrics and logs"
	case runner.PhaseTeardown:
		return "tearing down"
	case runner.PhaseFinished:
		return st.Status
	case "":
		return "starting"
	}
	return st.Phase
}

// topology draws the design as a tree from the load generator, each
// component colored by its most saturated resource in the last sample.
func topology(p *plan.Plan, s *driver.Sample) []string {
	children := func(svc *plan.Service) []string {
		switch {
		case svc.LB != nil:
			return []string{svc.LB.Target}
		case svc.PgBouncer != nil:
			return []string{svc.PgBouncer.Target}
		case len(svc.Synthetic) > 0:
			var ts []string
			for t := range svc.Synthetic[0].Targets {
				ts = append(ts, t)
			}
			sort.Strings(ts)
			return ts
		}
		return nil
	}
	var out []string
	seen := map[string]bool{}
	var walk func(name, parent, indent string, last bool)
	walk = func(name, parent, indent string, last bool) {
		svc := p.Service(name)
		if svc == nil {
			return
		}
		branch := "├▶ "
		next := indent + "│  "
		if last {
			branch, next = "└▶ ", indent+"   "
		}
		label := name
		if n := len(svc.Containers); n > 1 {
			label += fmt.Sprintf(" ×%d", n)
		}
		text, share, ok := nodeMetrics(svc, parent, s)
		line := bySaturation(share, ok).Render(label)
		if text != "" {
			line += "  " + text
		}
		if seen[name] {
			out = append(out, indent+branch+line+styleDim.Render("  (above)"))
			return
		}
		seen[name] = true
		out = append(out, indent+branch+line)
		cs := children(svc)
		for i, c := range cs {
			walk(c, name, next, i == len(cs)-1)
		}
	}
	out = append(out, "load")
	walk(p.Workload.Entry, "", "", true)
	return out
}

// nodeMetrics are a component's numbers in the tree, and its most
// saturated share; ok is false without data.
func nodeMetrics(svc *plan.Service, parent string, s *driver.Sample) (string, float64, bool) {
	if s == nil {
		return "", 0, false
	}
	var parts []string
	share, ok := 0.0, false
	use := func(v float64) { share, ok = math.Max(share, v), true }
	if v, has := s.CPU[svc.Name]; has {
		parts = append(parts, "cpu "+analysis.Pct(v))
		use(v)
	}
	if v, has := s.Memory[svc.Name]; has && v >= 0.5 {
		parts = append(parts, "mem "+analysis.Pct(v))
		use(v)
	}
	if c, has := s.Connections[svc.Name]; has {
		t := "conns " + analysis.Count(c)
		if m := s.MaxConnections[svc.Name]; m > 0 {
			t += "/" + analysis.Count(m)
			use(c / m)
		}
		parts = append(parts, t)
	}
	if h, has := s.HitRate[svc.Name]; has {
		parts = append(parts, "hit "+analysis.Pct(h))
	}
	if l, has := s.ReplicationLagS[svc.Name]; has {
		parts = append(parts, "lag "+analysis.Ms(l*1000))
	}
	if j, has := s.QueueJobs[svc.Name]; has {
		parts = append(parts, analysis.Count(j)+" jobs")
	}
	if pool, has := s.Pools[parent+">"+svc.Name]; has && pool.Max > 0 {
		t := fmt.Sprintf("%s's pool %s/%s", parent, analysis.Count(pool.InUse), analysis.Count(pool.Max))
		if pool.Pending > 0 {
			t += fmt.Sprintf(", %s waiting", analysis.Count(pool.Pending))
		}
		parts = append(parts, t)
		use(pool.InUse / pool.Max)
	}
	return strings.Join(parts, " · "), share, ok
}

// charts draws offered and served, p50/p95/p99 and errors over the
// profile, a block per window, with faults and the break marked.
func charts(b *strings.Builder, s *Snapshot, width int) {
	const label = 9
	cols := width - label - 2 - 26
	windows := int(math.Ceil(s.Status.ProfileS / analysis.WindowSeconds))
	if windows <= 0 {
		windows = len(s.Samples)
	}
	per := max(1, int(math.Ceil(float64(windows)/float64(cols))))
	n := int(math.Ceil(float64(windows) / float64(per)))
	col := func(atS float64) int { return min(n-1, max(0, int(atS/analysis.WindowSeconds)/per)) }
	series := func(get func(*driver.Sample) *float64) []*float64 {
		sum, cnt := make([]float64, n), make([]int, n)
		for _, l := range s.Samples {
			if v := get(l.Sample); v != nil && !math.IsNaN(*v) {
				i := col(math.Max(0, l.ElapsedS-analysis.WindowSeconds/2))
				sum[i] += *v
				cnt[i]++
			}
		}
		out := make([]*float64, n)
		for i := range out {
			if cnt[i] > 0 {
				m := sum[i] / float64(cnt[i])
				out[i] = &m
			}
		}
		return out
	}
	offered := series(func(x *driver.Sample) *float64 { v := x.Offered; return &v })
	served := series(func(x *driver.Sample) *float64 { return x.Served })
	top := 1.0
	for _, v := range slices.Concat(offered, served) {
		if v != nil {
			top = math.Max(top, *v)
		}
	}
	lin := func(v float64) float64 { return v / top }
	row := func(name string, vs []*float64, scale func(float64) float64, note string) {
		fmt.Fprintf(b, "  %-*s%s  %s\n", label, name, spark(vs, scale), styleDim.Render(note))
	}
	lastOf := func(vs []*float64) string {
		for i := len(vs) - 1; i >= 0; i-- {
			if vs[i] != nil {
				return analysis.Rate(*vs[i]) + " rps"
			}
		}
		return "–"
	}
	row("offered", offered, lin, lastOf(offered))
	row("served", served, lin, lastOf(served))

	p50 := series(func(x *driver.Sample) *float64 { return x.P50Ms })
	p95 := series(func(x *driver.Sample) *float64 { return x.P95Ms })
	p99 := series(func(x *driver.Sample) *float64 { return x.P99Ms })
	lo, hi := math.Inf(1), 0.0
	for _, v := range slices.Concat(p50, p95, p99) {
		if v != nil && *v > 0 {
			lo, hi = math.Min(lo, *v), math.Max(hi, *v)
		}
	}
	if hi > 0 {
		ceil := math.Max(hi, analysis.BreakLatencyFactor*lo)
		logScale := func(v float64) float64 {
			if v <= 0 {
				return 0
			}
			return math.Log(v/lo) / math.Log(ceil/lo)
		}
		ms := func(vs []*float64) string {
			for i := len(vs) - 1; i >= 0; i-- {
				if vs[i] != nil {
					return analysis.Ms(*vs[i])
				}
			}
			return "–"
		}
		if hasAny(p50) {
			row("p50", p50, logScale, ms(p50))
		}
		if hasAny(p95) {
			row("p95", p95, logScale, ms(p95))
		}
		row("p99", p99, logScale, ms(p99)+" (log scale)")
	}
	errs := series(func(x *driver.Sample) *float64 { return x.Errors })
	worst := 0.0
	for _, v := range errs {
		if v != nil {
			worst = math.Max(worst, *v)
		}
	}
	errNote := "0%"
	if worst > 0 {
		errNote = "up to " + analysis.Pct(worst)
	}
	row("errors", errs, func(v float64) float64 { return v / math.Max(worst, 0.01) }, errNote)

	// Markers: faults and the break, under the charts.
	marks := []rune(strings.Repeat(" ", n))
	for _, e := range s.Events {
		if e.Kind == "fault_start" {
			marks[col(e.AtS)] = '▲'
		}
	}
	if s.Status.BreakAtS != nil {
		marks[col(*s.Status.BreakAtS)] = '^'
	}
	axis := "0:00" + strings.Repeat(" ", max(1, n-4-len(analysis.Clock(s.Status.ProfileS)))) + analysis.Clock(s.Status.ProfileS)
	fmt.Fprintf(b, "  %-*s%s\n", label, "", colorMarks(string(marks)))
	fmt.Fprintf(b, "  %-*s%s\n", label, "", styleDim.Render(axis))
	if strings.ContainsAny(string(marks), "▲^") {
		fmt.Fprintf(b, "  %-*s%s\n", label, "", styleDim.Render("▲ fault  ^ break"))
	}
}

func colorMarks(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '▲':
			b.WriteString(styleFault.Render(string(r)))
		case '^':
			b.WriteString(styleBreak.Render(string(r)))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasAny(vs []*float64) bool {
	return slices.ContainsFunc(vs, func(v *float64) bool { return v != nil })
}

// components is a line per component with every number the last sample
// has for it.
func components(b *strings.Builder, p *plan.Plan, s *driver.Sample) {
	for _, svc := range p.Services {
		var parts []string
		share, ok := 0.0, false
		if v, has := s.CPU[svc.Name]; has {
			parts = append(parts, "cpu "+analysis.Pct(v))
			share, ok = math.Max(share, v), true
		}
		if v, has := s.Memory[svc.Name]; has {
			parts = append(parts, "mem "+analysis.Pct(v))
			share, ok = math.Max(share, v), true
		}
		if c, has := s.Connections[svc.Name]; has {
			t := "conns " + analysis.Count(c)
			if m := s.MaxConnections[svc.Name]; m > 0 {
				t += " of " + analysis.Count(m)
				share, ok = math.Max(share, c/m), true
			}
			parts = append(parts, t)
		}
		if h, has := s.HitRate[svc.Name]; has {
			parts = append(parts, "hit rate "+analysis.Pct(h))
		}
		if l, has := s.ReplicationLagS[svc.Name]; has {
			parts = append(parts, "lag "+analysis.Ms(l*1000))
		}
		if j, has := s.QueueJobs[svc.Name]; has {
			parts = append(parts, analysis.Count(j)+" jobs queued")
		}
		var keys []string
		for k := range s.Pools {
			if strings.HasPrefix(k, svc.Name+">") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			pool := s.Pools[k]
			t := fmt.Sprintf("pool to %s %s/%s", strings.TrimPrefix(k, svc.Name+">"), analysis.Count(pool.InUse), analysis.Count(pool.Max))
			if pool.Pending > 0 {
				t += fmt.Sprintf(" +%s waiting", analysis.Count(pool.Pending))
			}
			parts = append(parts, t)
			if pool.Max > 0 {
				share, ok = math.Max(share, pool.InUse/pool.Max), true
			}
		}
		if len(parts) == 0 {
			continue
		}
		fmt.Fprintf(b, "  %s  %s\n", bySaturation(share, ok).Render(fmt.Sprintf("%-10s", svc.Name)), strings.Join(parts, " · "))
	}
}

var blocks = []rune("▁▂▃▄▅▆▇█")

func spark(vs []*float64, f func(float64) float64) string {
	var b strings.Builder
	for _, v := range vs {
		if v == nil {
			b.WriteRune(' ')
			continue
		}
		x := math.Max(0, math.Min(1, f(*v)))
		b.WriteRune(blocks[int(math.Round(x*float64(len(blocks)-1)))])
	}
	return b.String()
}

func wrap(s string, n int) []string {
	var out []string
	var cur []string
	length := 0
	for _, w := range strings.Fields(s) {
		if length > 0 && length+1+len([]rune(w)) > n {
			out = append(out, strings.Join(cur, " "))
			cur, length = nil, 0
		}
		cur = append(cur, w)
		length += len([]rune(w)) + min(1, len(cur)-1)
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, " "))
	}
	return out
}
