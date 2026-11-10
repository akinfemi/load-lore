package analysis

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/akinfemi/load-lore/internal/driver"
)

// VerdictKind is the one-word outcome of a run.
type VerdictKind string

// Verdict kinds.
const (
	VerdictBroke        VerdictKind = "broke"
	VerdictHeld         VerdictKind = "held"
	VerdictInconclusive VerdictKind = "inconclusive"
	VerdictNoData       VerdictKind = "no_data"
)

// Thresholds that define "broke" and "inconclusive". They are fixed so runs
// compare, and every report prints them.
const (
	BreakErrorRate     = 0.01 // a window with at least 1% failed requests
	BreakLatencyFactor = 10.0 // p99 at least 10x the low-load p99 ...
	BreakLatencyMinMs  = 100.0
	BreakShortfall     = 0.9 // successful responses below 90% of the offered rate
	BaselineShare      = 0.25
	DroppedShareLimit  = 0.01 // the load generator dropped >1% of a window's arrivals
	ToolCPULimit       = 0.9  // a tool used >=90% of its CPU budget
	ToolThrottleLimit  = 0.05 // a tool was stalled by CFS in >=5% of its periods
	ToolThrottleNote   = 0.01 // ... or in >=1%: noted, since it can still move p99
	// The first seconds of load don't count for the tooling checks: k6
	// starts its VUs and every connection opens then.
	ToolWarmupSeconds = 10.0
	// Evidence for naming the cause when tooling was short of CPU.
	HostStealLimit  = 0.02 // the hypervisor gave >=2% of CPU time to other VMs
	HostOthersShare = 0.1  // other programs used >=10% of the host's CPUs ...
	HostOthersMin   = 0.5  // ... and at least half a CPU
)

// Verdict says whether and where the system broke.
type Verdict struct {
	Kind         VerdictKind `json:"kind"`
	Summary      string      `json:"summary"`
	Break        *Break      `json:"break,omitempty"`
	Inconclusive []string    `json:"inconclusive,omitempty"`
	Caveats      []string    `json:"caveats,omitempty"`            // tooling strain too small to make the run inconclusive
	Cause        string      `json:"inconclusive_cause,omitempty"` // what took CPU from the tooling, when there's evidence
	BaselineP99  *float64    `json:"baseline_p99_ms,omitempty"`
	MaxRPS       *float64    `json:"max_rps,omitempty"`
	MaxRPSAt     float64     `json:"max_rps_at_s"`
	WorstP99     *float64    `json:"worst_p99_ms,omitempty"`
	WorstErrors  *float64    `json:"worst_error_rate,omitempty"`
	Moment       int         `json:"moment_window"` // the window the annotated diagram shows
}

// Break is the first sustained sign the system couldn't keep up.
type Break struct {
	Window     int          `json:"window"`
	At         float64      `json:"at_s"`
	Offered    float64      `json:"offered_rps"`
	RPS        *float64     `json:"rps"`
	P99        *float64     `json:"p99_ms"`
	ErrorRate  *float64     `json:"error_rate"`
	Signals    []string     `json:"signals"` // latency, errors, throughput
	Saturation []Saturation `json:"saturation"`
}

// Saturation is how close one resource was to its limit.
type Saturation struct {
	Component string  `json:"component"`
	What      string  `json:"what"` // "CPU", "pool to db", "connections"
	Value     float64 `json:"value"`
	Detail    string  `json:"detail"`
	Source    string  `json:"source"`
}

func verdict(a *Analysis) Verdict {
	v := Verdict{Kind: VerdictHeld}
	ws := a.Windows
	if len(ws) == 0 || !anyRPS(ws) {
		v.Kind = VerdictNoData
		v.Summary = statusPrefix(a) + "No end-to-end data was collected during the load, so the run can't be judged."
		return v
	}
	v.BaselineP99 = baselineP99(ws, a.PeakOffered)
	for _, w := range ws {
		if w.RPS != nil && (v.MaxRPS == nil || *w.RPS > *v.MaxRPS) {
			v.MaxRPS, v.MaxRPSAt = w.RPS, w.Start
		}
		v.WorstP99 = maxPtr(v.WorstP99, w.P99)
		v.WorstErrors = maxPtr(v.WorstErrors, w.ErrorRate)
	}

	sig := make([][]string, len(ws))
	for i, w := range ws {
		sig[i] = signals(w, v.BaselineP99)
	}
	// A break is sustained: two windows in a row. One bad window, such as
	// a CPU-throttling stall, isn't a break.
	breakAt := -1
	for i := 0; i+1 < len(ws); i++ {
		if len(sig[i]) > 0 && len(sig[i+1]) > 0 {
			breakAt = i
			break
		}
	}
	v.Moment = v.momentWindow(ws, breakAt)
	if breakAt >= 0 {
		w := ws[breakAt]
		v.Kind = VerdictBroke
		v.Break = &Break{Window: breakAt, At: w.Start, Offered: w.Offered, RPS: w.RPS, P99: w.P99, ErrorRate: w.ErrorRate,
			Signals: sig[breakAt], Saturation: saturation(a, w)}
	}
	var short []*Window
	v.Inconclusive, v.Caveats, short = inconclusive(ws, breakAt)
	v.Cause = shortCause(a.Environment.Host, short)
	if a.Cutoff != "" && breakAt < 0 {
		v.Inconclusive = append([]string{"before the system broke, " + a.Cutoff}, v.Inconclusive...)
	}
	v.Summary = v.summary(a)
	if breakAt >= 0 {
		v.Summary += afterBreak(a, ws[breakAt+1:])
	}
	if a.Cutoff != "" && breakAt >= 0 {
		v.Summary += " After that, " + a.Cutoff + "."
	}
	if len(v.Inconclusive) > 0 {
		v.Kind = VerdictInconclusive
	}
	return v
}

func anyRPS(ws []*Window) bool {
	return slices.ContainsFunc(ws, func(w *Window) bool { return w.RPS != nil })
}

func maxPtr(cur, v *float64) *float64 {
	if v == nil || (cur != nil && *cur >= *v) {
		return cur
	}
	return v
}

// baselineP99 is the median p99 of the low-load windows: those offered at
// most a quarter of the peak, or the first three if the profile starts
// high.
func baselineP99(ws []*Window, peak float64) *float64 {
	var xs []float64
	for _, w := range ws {
		if w.P99 != nil && w.Offered <= BaselineShare*peak {
			xs = append(xs, *w.P99)
		}
	}
	if len(xs) == 0 {
		for _, w := range ws[:min(3, len(ws))] {
			if w.P99 != nil {
				xs = append(xs, *w.P99)
			}
		}
	}
	if len(xs) == 0 {
		return nil
	}
	sort.Float64s(xs)
	m := median(xs)
	return &m
}

// LatencyLimit is the p99 above which a window counts as broken.
func LatencyLimit(baseline *float64) (float64, bool) {
	if baseline == nil {
		return 0, false
	}
	return math.Max(BreakLatencyFactor**baseline, BreakLatencyMinMs), true
}

func signals(w *Window, baseline *float64) []string {
	var s []string
	if lim, ok := LatencyLimit(baseline); ok && w.P99 != nil && *w.P99 >= lim {
		s = append(s, "latency")
	}
	if w.ErrorRate != nil && *w.ErrorRate >= BreakErrorRate {
		s = append(s, "errors")
	}
	if w.RPS != nil && w.Offered >= 1 && *w.RPS < BreakShortfall*w.Offered {
		s = append(s, "throughput")
	}
	return s
}

// momentWindow is the window the annotated diagram shows: the break, or
// the window with the most successful responses when nothing broke.
func (v *Verdict) momentWindow(ws []*Window, breakAt int) int {
	if breakAt >= 0 {
		return breakAt
	}
	for i, w := range ws {
		if w.RPS != nil && v.MaxRPS != nil && *w.RPS == *v.MaxRPS {
			return i
		}
	}
	return len(ws) - 1
}

// saturation lists every resource's use in a window, most saturated first.
func saturation(a *Analysis, w *Window) []Saturation {
	var out []Saturation
	for _, s := range a.Plan.Services {
		sw := w.Services[s.Name]
		if sw == nil {
			continue
		}
		if sw.CPU != nil {
			out = append(out, Saturation{Component: s.Name, What: "CPU", Value: *sw.CPU,
				Detail: fmt.Sprintf("%s CPU at %s of its limit", s.Name, Pct(*sw.CPU)), Source: SrcCPU})
		}
		if sw.Connections != nil && sw.MaxConnections != nil && *sw.MaxConnections > 0 {
			u := *sw.Connections / *sw.MaxConnections
			out = append(out, Saturation{Component: s.Name, What: "connections", Value: u,
				Detail: fmt.Sprintf("%s connections %s of max_connections %s", s.Name, Count(*sw.Connections), Count(*sw.MaxConnections)), Source: SrcPGConns})
		}
	}
	for _, p := range w.Pools {
		if p.InUse != nil && p.Max != nil && *p.Max > 0 {
			u := *p.InUse / *p.Max
			out = append(out, Saturation{Component: p.Service, What: "pool to " + p.Target, Value: u,
				Detail: fmt.Sprintf("%s → %s pool %s of %s connections in use", p.Service, p.Target, Count(*p.InUse), Count(*p.Max)), Source: SrcPoolInUse})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	return out
}

func faultNear(tl []Event, at float64) bool {
	for _, e := range tl {
		if e.Kind == "fault_start" && e.At <= at+0.5 && at-e.At < 3 {
			return true
		}
	}
	return false
}

// afterBreak says when the load generator stopped sending part of the
// profile once the system slowed: open-model arrivals need a free VU, and
// slow responses hold them. Those arrivals were never sent.
func afterBreak(a *Analysis, ws []*Window) string {
	dropped, offered := 0.0, 0.0
	var first *Window
	for _, w := range ws {
		if w.Dropped != nil && *w.Dropped > 0 {
			dropped += *w.Dropped
			if first == nil {
				first = w
			}
		}
		offered += w.Offered * WindowSeconds
	}
	if first == nil || offered == 0 || dropped/offered < DroppedShareLimit {
		return ""
	}
	return fmt.Sprintf(" After the break the load generator couldn't send %s of the arrivals the profile asked for (%s arrivals from %s; %s): slow responses held its %s VUs, so served throughput from then on is partly the generator's limit, not only the system's.",
		Pct(dropped/offered), Count(dropped), Clock(first.Start), SrcDropped, Count(float64(a.Plan.Workload.MaxVUs)))
}

// MostSaturated is the busiest CPU or pool. Open database connections
// aren't work: pools keep idle ones open, so they only lead when nothing
// else was measured.
func MostSaturated(ss []Saturation) (Saturation, bool) {
	for _, s := range ss {
		if s.What != "connections" {
			return s, true
		}
	}
	if len(ss) > 0 {
		return ss[0], true
	}
	return Saturation{}, false
}

// inconclusive finds signs that Loadlore's own tooling, not the system, set
// the limit before the break (or anywhere, if nothing broke). Dropped
// arrivals in the break window itself are a consequence of the break.
func inconclusive(ws []*Window, breakAt int) (reasons, caveats []string, short []*Window) {
	end := len(ws)
	if breakAt >= 0 {
		end = breakAt + 1
	}
	var drop, load, proxy *Window
	tools := []struct {
		name  string
		get   func(*Window) *float64
		worst *Window
	}{
		{"the load generator", func(w *Window) *float64 { return w.LoadThrottled }, nil},
		{"Toxiproxy", func(w *Window) *float64 { return w.ProxyThrottled }, nil},
	}
	for i, w := range ws[:end] {
		if w.Start < ToolWarmupSeconds {
			continue
		}
		if i < end-1 || breakAt < 0 {
			if w.Dropped != nil && w.Offered > 0 && *w.Dropped/(w.Offered*WindowSeconds) > DroppedShareLimit && drop == nil {
				drop = w
			}
		}
		if w.LoadCPU != nil && *w.LoadCPU >= ToolCPULimit && load == nil {
			load = w
		}
		if w.ProxyCPU != nil && *w.ProxyCPU >= ToolCPULimit && proxy == nil {
			proxy = w
		}
		for i := range tools {
			t := &tools[i]
			if v := t.get(w); v != nil && *v >= ToolThrottleNote && (t.worst == nil || *v > *t.get(t.worst)) {
				t.worst = w
			}
		}
	}
	for _, w := range []*Window{drop, load, proxy} {
		if w != nil {
			short = append(short, w)
		}
	}
	if drop != nil {
		reasons = append(reasons, fmt.Sprintf("at %s the load generator dropped %s arrivals, %s of what the profile offered in that window (%s), before the system showed strain: it couldn't start requests fast enough, so the offered load was lower than the profile",
			Clock(drop.Start), Count(*drop.Dropped), Pct(*drop.Dropped/(drop.Offered*WindowSeconds)), SrcDropped))
	}
	if load != nil {
		reasons = append(reasons, fmt.Sprintf("at %s the load generator used %s of its CPU budget (%s), so it may have been slowing requests down itself", Clock(load.Start), Pct(*load.LoadCPU), SrcLoadCPU))
	}
	if proxy != nil {
		reasons = append(reasons, fmt.Sprintf("at %s Toxiproxy, which carries every connection between services, used %s of its CPU budget (%s), so it may have added latency of its own", Clock(proxy.Start), Pct(*proxy.ProxyCPU), SrcProxyCPU))
	}
	for _, t := range tools {
		if t.worst == nil {
			continue
		}
		share := *t.get(t.worst)
		s := fmt.Sprintf("at %s %s hit its CPU limit and was stalled in %s of its scheduling periods (%s)", Clock(t.worst.Start), t.name, Pct(share), SrcThrottledPeriods)
		if share >= ToolThrottleLimit {
			short = append(short, t.worst)
			reasons = append(reasons, s+", so it was short of CPU in bursts even if its average use looked fine")
		} else {
			caveats = append(caveats, s+"; work caught in a stall waits for the next 100 ms period, which can show up in p99")
		}
	}
	return reasons, caveats, short
}

// shortCause names what took CPU from the tooling in the windows where it
// was short, only from evidence: steal or other programs' CPU in the
// host's /proc/stat, or, when Docker runs in a VM on this machine, the
// possibility that something outside the VM did. When the host was
// sampled and neither shows, it says nothing: the tooling's own bursts are
// then the likelier cause.
func shortCause(h *driver.Host, short []*Window) string {
	if h == nil || len(short) == 0 {
		return ""
	}
	switch h.HostCPU {
	case driver.HostCPUInVM:
		return "Docker runs in a virtual machine here, which can't see what the rest of this machine is doing: if other programs were busy, they possibly took CPU from the run. Close them and run again"
	case driver.HostCPUSampled:
		var steal, others *Window
		for _, w := range short {
			if w.HostSteal != nil && *w.HostSteal >= HostStealLimit && (steal == nil || *w.HostSteal > *steal.HostSteal) {
				steal = w
			}
			limit := math.Max(HostOthersMin, HostOthersShare*float64(h.CPUs))
			if w.HostOthers != nil && *w.HostOthers >= limit && (others == nil || *w.HostOthers > *others.HostOthers) {
				others = w
			}
		}
		var c []string
		if others != nil {
			c = append(c, fmt.Sprintf("other programs on this machine used %s CPUs at %s (%s), taking CPU from the run. Close them and run again", strconv.FormatFloat(math.Round(*others.HostOthers*10)/10, 'f', -1, 64), Clock(others.Start), SrcHostOthers))
		}
		if steal != nil {
			c = append(c, fmt.Sprintf("the hypervisor gave %s of this machine's CPU time to other virtual machines at %s (%s). Run again, or on a machine with dedicated CPUs", Pct(*steal.HostSteal), Clock(steal.Start), SrcHostSteal))
		}
		if len(c) > 0 {
			s := strings.Join(c, ". Also, ")
			return strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return ""
}

func (v *Verdict) summary(a *Analysis) string {
	var s string
	if b := v.Break; b != nil {
		var what []string
		for _, sg := range b.Signals {
			switch sg {
			case "latency":
				t := "p99 reached " + Ms(*b.P99)
				if v.BaselineP99 != nil {
					t += " (" + Ms(*v.BaselineP99) + " at low load)"
				}
				what = append(what, t)
			case "errors":
				what = append(what, Pct(*b.ErrorRate)+" of requests failed")
			case "throughput":
				what = append(what, "only "+Rate(*b.RPS)+" rps succeeded")
			}
		}
		s = fmt.Sprintf("Broke %s into the load, at %s rps offered: %s.", Clock(b.At), Rate(b.Offered), joinAnd(what))
		if top, ok := MostSaturated(b.Saturation); ok {
			s += " Most saturated then: " + top.Detail + "."
		}
		// Faults and container deaths near the break; a death a fault
		// caused is already described by the fault.
		var during []string
		near := func(e Event) bool { return e.At >= b.At-WindowSeconds && e.At < b.At+WindowSeconds }
		for _, e := range a.Timeline {
			if !near(e) {
				continue
			}
			switch {
			case e.Kind == "fault_start":
				during = append(during, e.Detail+" at "+Clock(e.At))
			case e.Kind == "container" && (strings.HasSuffix(e.Detail, " oom") || strings.Contains(e.Detail, " die") && !faultNear(a.Timeline, e.At)):
				during = append(during, e.Detail+" at "+Clock(e.At))
			}
		}
		if len(during) > 0 {
			s += " Around then: " + strings.Join(during, "; ") + "."
		}
	} else {
		s = fmt.Sprintf("Held the whole profile: up to %s rps offered", Rate(a.PeakOffered))
		if v.MaxRPS != nil {
			s += ", " + Rate(*v.MaxRPS) + " rps served at best"
		}
		if v.WorstP99 != nil {
			s += ", worst 10-second p99 " + Ms(*v.WorstP99)
		}
		if v.WorstErrors != nil {
			s += ", at most " + Pct(*v.WorstErrors) + " errors"
		}
		s += "."
	}
	if len(v.Inconclusive) > 0 {
		cause := ""
		if v.Cause != "" {
			cause = v.Cause + ". "
		}
		s = "Inconclusive: " + v.Inconclusive[0] + ". " + cause + "Taken at face value: " + s
	}
	return statusPrefix(a) + s
}

// statusPrefix says when the run didn't complete, so a verdict isn't read
// as covering the whole profile.
func statusPrefix(a *Analysis) string {
	if a.Status == "" || a.Status == "completed" {
		return ""
	}
	return fmt.Sprintf("The run %s (%s), so this covers only the data collected. ", strings.ReplaceAll(a.Status, "_", " "), a.Error)
}

// --- formatting, shared with reports ---

// Rate formats requests per second with thousands separators.
func Rate(f float64) string {
	if f < 10 && f != math.Trunc(f) {
		return strconv.FormatFloat(f, 'f', 1, 64)
	}
	return Count(f)
}

// Count formats a whole number with thousands separators.
func Count(f float64) string {
	s := strconv.FormatFloat(math.Round(f), 'f', 0, 64)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// Ms formats milliseconds: 0.8 ms, 14 ms, 1.2 s.
func Ms(v float64) string {
	switch {
	case v >= 1000:
		return strconv.FormatFloat(v/1000, 'f', 2, 64) + " s"
	case v >= 10:
		return strconv.FormatFloat(v, 'f', 0, 64) + " ms"
	}
	return strconv.FormatFloat(v, 'f', 1, 64) + " ms"
}

// Pct formats a fraction as a percentage.
func Pct(f float64) string {
	switch {
	case f == 0:
		return "0%"
	case f < 0.001:
		return "<0.1%"
	case f < 0.1:
		return strconv.FormatFloat(f*100, 'f', 1, 64) + "%"
	}
	return strconv.FormatFloat(f*100, 'f', 0, 64) + "%"
}
