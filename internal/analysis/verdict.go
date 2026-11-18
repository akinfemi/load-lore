package analysis

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/akinfemi/load-lore/internal/design"
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
	BreakLatencyFactor = 10.0 // p99 at least 10x the low-load p99 and the SLO
	BreakShortfall     = 0.9  // successful responses below 90% of the offered rate
	// A shortfall or errors break in 2 windows in a row; p99 alone needs 3,
	// since a short latency episode, such as a CPU-throttling stall, often
	// recovers by itself. Shorter episodes are stalls in the timeline.
	BreakWindows        = 2
	BreakLatencyWindows = 3
	BaselineShare       = 0.25
	DroppedShareLimit   = 0.01 // the load generator dropped >1% of a window's arrivals
	// In a window where the system itself failed BreakErrorRate or more of
	// requests, slow failing responses hold VUs, so some drops are expected:
	// up to this share they're a caveat; above it the missing load could
	// flatter whatever absorbed the trouble, and the run is inconclusive.
	DroppedFailingLimit = 0.05
	ToolCPULimit        = 0.9  // a tool used >=90% of its CPU budget
	ToolThrottleLimit   = 0.05 // a tool was stalled by CFS in >=5% of its periods
	ToolThrottleNote    = 0.01 // ... or in >=1%: noted, since it can still move p99
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
	Kind    VerdictKind `json:"kind"`
	Summary string      `json:"summary"`
	Break   *Break      `json:"break,omitempty"`
	// RecoveredAt is when the system was healthy again after a break, for
	// good: the start of the first window from which every window to the
	// end of the load, at least RecoverWindows of them, showed no break
	// signal. Nil when it broke and didn't recover, or didn't break.
	RecoveredAt  *float64 `json:"recovered_at_s,omitempty"`
	Inconclusive []string `json:"inconclusive,omitempty"`
	Caveats      []string `json:"caveats,omitempty"`            // tooling strain too small to make the run inconclusive
	Cause        string   `json:"inconclusive_cause,omitempty"` // what took CPU from the tooling, when there's evidence
	Doubt        string   `json:"doubt,omitempty"`              // why a latency break may be partly the load generator's own
	BaselineP99  *float64 `json:"baseline_p99_ms,omitempty"`
	SLOP99Ms     float64  `json:"slo_p99_ms"`
	LatencyLimit *float64 `json:"latency_limit_ms,omitempty"` // p99 at or above this is a latency signal
	MaxRPS       *float64 `json:"max_rps,omitempty"`
	MaxRPSAt     float64  `json:"max_rps_at_s"`
	WorstP99     *float64 `json:"worst_p99_ms,omitempty"`
	WorstErrors  *float64 `json:"worst_error_rate,omitempty"`
	Moment       int      `json:"moment_window"` // the window the annotated diagram shows
	// RecoveryDoubt is why a run that broke and didn't recover may have
	// stayed down partly because of the lab: Toxiproxy stalled after the
	// break. Claims that it doesn't recover can't rest on such a run.
	RecoveryDoubt string `json:"recovery_doubt,omitempty"`
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
	hard := make([][]string, len(ws))
	for i, w := range ws {
		hard[i] = hardSignals(w)
	}
	v.BaselineP99 = baselineP99(ws, a.PeakOffered)
	for _, w := range ws {
		if w.RPS != nil && (v.MaxRPS == nil || *w.RPS > *v.MaxRPS) {
			v.MaxRPS, v.MaxRPSAt = w.RPS, w.Start
		}
		v.WorstP99 = maxPtr(v.WorstP99, w.P99)
		v.WorstErrors = maxPtr(v.WorstErrors, w.ErrorRate)
	}

	v.SLOP99Ms = design.DefaultSLOP99Ms
	if a.Plan != nil && a.Plan.Workload.SLOP99Ms > 0 {
		v.SLOP99Ms = a.Plan.Workload.SLOP99Ms // runs planned before the SLO existed get the default
	}
	if lim, ok := LatencyLimit(v.BaselineP99, v.SLOP99Ms); ok {
		v.LatencyLimit = &lim
	}
	sig := withLatency(ws, hard, v.LatencyLimit)
	breakAt := findBreak(hard, sig)
	a.Timeline = withStalls(a.Timeline, ws, sig, breakAt, v.BaselineP99, v.LatencyLimit)
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
	stallsBefore, stallsAfter := splitStalls(a, breakAt)
	if breakAt >= 0 && len(stallsBefore) > 0 {
		v.Inconclusive = append(v.Inconclusive, stallText(stallsBefore)+
			", by the end of the break's first window: a connection queue on one edge that a real network doesn't have, which may have made the break")
	}
	hostStallVerdict(a, &v, breakAt)
	if a.Cutoff != "" && breakAt < 0 {
		v.Inconclusive = append([]string{"before the system broke, " + a.Cutoff}, v.Inconclusive...)
	}
	if why := suspended(a); why != "" {
		v.Inconclusive = append([]string{why}, v.Inconclusive...)
	}
	if why := labFailed(a.Timeline); why != "" {
		v.Inconclusive = append([]string{why}, v.Inconclusive...)
	}
	hardBreak := breakAt >= 0 && breakAt+1 < len(hard) && len(hard[breakAt]) > 0 && len(hard[breakAt+1]) > 0
	if breakAt >= 0 && !hardBreak { // made by latency alone
		v.Doubt = latencyDoubt(a, ws[breakAt:min(breakAt+BreakLatencyWindows, len(ws))], *v.LatencyLimit)
	}
	v.Summary = v.summary(a)
	if v.Doubt != "" {
		v.Summary += " Doubtful: " + v.Doubt + "."
	}
	if breakAt >= 0 {
		v.Summary += afterBreak(a, ws[breakAt+1:])
		v.RecoveredAt = recoveredAt(ws, hard, sig, breakAt)
		switch {
		case v.RecoveredAt != nil:
			v.Summary += fmt.Sprintf(" It recovered at %s, %s after the break, and stayed healthy to the end.", Clock(*v.RecoveredAt), Clock(*v.RecoveredAt-ws[breakAt].Start))
		case a.StoppedEarly == nil:
			v.Summary += " It hadn't recovered when the load ended."
		}
	}
	switch {
	case breakAt < 0 && len(stallsBefore) > 0:
		v.Caveats = append(v.Caveats, stallText(stallsBefore)+"; it held anyway, and the stall only made things harder")
	case len(stallsAfter) > 0 && v.RecoveredAt != nil:
		v.Caveats = append(v.Caveats, stallText(stallsAfter)+", after the break; it recovered anyway")
	case len(stallsAfter) > 0:
		v.RecoveryDoubt = stallText(stallsAfter) + ", after the break: a connection queue on one edge that a real network doesn't have, so the failure after the break may be deeper and longer than the system's own"
		v.Caveats = append(v.Caveats, v.RecoveryDoubt)
	}
	if a.Cutoff != "" && breakAt >= 0 {
		v.Summary += " After that, " + a.Cutoff + "."
	}
	if es := a.StoppedEarly; es != nil {
		v.Summary += fmt.Sprintf(" The load stopped at %s, %.0f s after the live check confirmed the break; `loadl run --full` runs the whole profile.", Clock(es.At), es.At-es.ConfirmedAt)
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
// high. Only windows before the first shortfall or errors in a row count:
// a system that is already failing has no low-load latency to compare
// against. A single short window, as when k6 is still starting its VUs,
// doesn't end the baseline.
func baselineP99(ws []*Window, peak float64) *float64 {
	return baselineFrom(healthy(ws), peak)
}

// healthy is ws up to the first shortfall or errors in a row: the windows
// a low-load baseline may come from.
func healthy(ws []*Window) []*Window {
	hard := make([][]string, len(ws))
	for i, w := range ws {
		hard[i] = hardSignals(w)
	}
	if i := findBreak(hard, nil); i >= 0 {
		return ws[:i]
	}
	return ws
}

func baselineFrom(healthy []*Window, peak float64) *float64 {
	var xs []float64
	for _, w := range healthy {
		if w.P99 != nil && w.Offered <= BaselineShare*peak {
			xs = append(xs, *w.P99)
		}
	}
	if len(xs) == 0 {
		for _, w := range healthy[:min(3, len(healthy))] {
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

// LatencyLimit is the p99 at or above which a window counts as broken:
// 10x the low-load p99, and at least the SLO.
func LatencyLimit(baseline *float64, sloMs float64) (float64, bool) {
	if baseline == nil {
		return 0, false
	}
	return math.Max(BreakLatencyFactor**baseline, sloMs), true
}

// BreakAt applies the verdict's break rule to ws, as a run's live check
// does while the load runs: it returns the first window of a sustained
// break, or -1. peak is the profile's peak offered rate and sloMs the
// design's p99 target.
func BreakAt(ws []*Window, peak, sloMs float64) int {
	hard := make([][]string, len(ws))
	for i, w := range ws {
		hard[i] = hardSignals(w)
	}
	var limit *float64
	if lim, ok := LatencyLimit(baselineP99(ws, peak), sloMs); ok {
		limit = &lim
	}
	return findBreak(hard, withLatency(ws, hard, limit))
}

// withLatency adds the latency signal to each window whose p99 is at or
// above limit (nil: no latency signal).
func withLatency(ws []*Window, hard [][]string, limit *float64) [][]string {
	sig := make([][]string, len(ws))
	for i, w := range ws {
		sig[i] = hard[i]
		if limit != nil && w.P99 != nil && *w.P99 >= *limit {
			sig[i] = append([]string{"latency"}, hard[i]...)
		}
	}
	return sig
}

// hardSignals are the signs that requests weren't served: errors and a
// shortfall. Latency is judged separately, against the baseline.
func hardSignals(w *Window) []string {
	var s []string
	if w.ErrorRate != nil && *w.ErrorRate >= BreakErrorRate {
		s = append(s, "errors")
	}
	if w.RPS != nil && w.Offered >= 1 && *w.RPS < BreakShortfall*w.Offered {
		s = append(s, "throughput")
	}
	return s
}

// findBreak is the first window of a sustained break: BreakWindows in a
// row with errors or a shortfall, or BreakLatencyWindows in a row with
// any signal (sig nil: errors and shortfalls only). It returns -1 when
// nothing broke.
func findBreak(hard, sig [][]string) int {
	run := func(i, n int, xs [][]string) bool {
		if i+n > len(xs) {
			return false
		}
		for _, x := range xs[i : i+n] {
			if len(x) == 0 {
				return false
			}
		}
		return true
	}
	for i := range hard {
		if run(i, BreakWindows, hard) || sig != nil && run(i, BreakLatencyWindows, sig) {
			return i
		}
	}
	return -1
}

// withStalls adds to the timeline each episode before the break (or in
// the whole run, if nothing broke) that had a break signal but was too
// short to count as one.
func withStalls(tl []Event, ws []*Window, sig [][]string, breakAt int, baseline, limit *float64) []Event {
	end := len(ws)
	if breakAt >= 0 {
		end = breakAt
	}
	added := false
	for i := 0; i < end; {
		if len(sig[i]) == 0 {
			i++
			continue
		}
		j := i
		for j < end && len(sig[j]) > 0 {
			j++
		}
		tl = append(tl, Event{At: ws[i].Start, To: ws[j-1].Start + WindowSeconds, Kind: "stall", Detail: stallDetail(ws[i:j], baseline, limit, j == len(ws))})
		added = true
		i = j
	}
	if added {
		sort.SliceStable(tl, func(i, j int) bool { return tl[i].At < tl[j].At })
	}
	return tl
}

func stallDetail(ws []*Window, baseline, limit *float64, last bool) string {
	var p99, errs, served *float64
	for _, w := range ws {
		p99 = maxPtr(p99, w.P99)
		errs = maxPtr(errs, w.ErrorRate)
		if w.RPS != nil && w.Offered >= 1 {
			share := *w.RPS / w.Offered
			if served == nil || share < *served {
				served = &share
			}
		}
	}
	var parts []string
	if limit != nil && p99 != nil && *p99 >= *limit {
		parts = append(parts, fmt.Sprintf("p99 reached %s (%s at low load)", Ms(*p99), Ms(*baseline)))
	}
	if errs != nil && *errs >= BreakErrorRate {
		parts = append(parts, fmt.Sprintf("%s of requests failed", Pct(*errs)))
	}
	if served != nil && *served < BreakShortfall {
		parts = append(parts, fmt.Sprintf("only %s of the offered rate was served", Pct(*served)))
	}
	then := "then recovered"
	if last {
		then = "until the load ended"
	}
	return fmt.Sprintf("%s for %d s, %s; too short to count as a break", strings.Join(parts, ", "), len(ws)*WindowSeconds, then)
}

// nameStalls says which diagnoses cover each stall: those whose first
// stretch overlaps it, or ends as it starts, since a cause in one window
// can show up in the next one's p99 (the connection storm counts that
// one too). A stall no diagnosis covers says so.
func nameStalls(a *Analysis) {
	for i := range a.Timeline {
		e := &a.Timeline[i]
		if e.Kind != "stall" {
			continue
		}
		var titles []string
		for _, h := range a.HostStalls {
			if h.At < e.To && e.At <= h.To() && !slices.Contains(e.Rules, "host-stall") {
				titles = append(titles, "a stall of the Docker host's disk (the machine, not the design)")
				e.Rules = append(e.Rules, "host-stall")
			}
		}
		for _, d := range a.Diagnoses {
			if d.From < e.To && e.At <= d.To && !slices.Contains(titles, d.Title) {
				titles = append(titles, d.Title)
				if !slices.Contains(e.Rules, d.Rule) {
					e.Rules = append(e.Rules, d.Rule)
				}
			}
		}
		if len(titles) == 0 {
			e.Detail += "; no diagnosis covers it"
		} else {
			e.Detail += "; diagnosed as " + strings.Join(titles, " and ")
		}
	}
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
				Detail: fmt.Sprintf("%s connections %s of %s %s", s.Name, Count(*sw.Connections), connLimit(a, s.Name), Count(*sw.MaxConnections)), Source: connSource(a, s.Name)})
		}
	}
	for _, p := range w.Pools {
		if p.InUse != nil && p.Max != nil && *p.Max > 0 {
			u := *p.InUse / *p.Max
			out = append(out, Saturation{Component: p.Service, What: "pool to " + p.Target, Value: u,
				Detail: fmt.Sprintf("%s → %s pool %s of %s %s in use", p.Service, p.Target, Count(*p.InUse), Count(*p.Max), poolSources(a, p.Service).conns), Source: poolSources(a, p.Service).inUse})
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

// latencyDoubt cross-checks a break made by latency alone when the load
// generator was stalled at its CPU limit in one of its windows: its stalls
// add to the very latency it measures. The entry service's own p99 has no
// such bias; if it stayed below the limit in any of the windows, it
// wouldn't have made the break. It returns "" when the check agrees or
// can't be made.
func latencyDoubt(a *Analysis, ws []*Window, limit float64) string {
	app := entryApp(a)
	var stall float64
	var below []string
	for _, w := range ws {
		if w.LoadThrottled != nil {
			stall = math.Max(stall, *w.LoadThrottled)
		}
		sw := w.Services[app]
		if app == "" || sw == nil || sw.P99 == nil {
			return ""
		}
		if *sw.P99 < limit {
			below = append(below, fmt.Sprintf("%s at %s", Ms(*sw.P99), Clock(w.Start)))
		}
	}
	if stall < ToolThrottleNote || len(below) == 0 {
		return ""
	}
	return fmt.Sprintf("the load generator was stalled at its CPU limit in up to %s of its scheduling periods in the break's windows (%s), while %s's own p99 was %s (%s), below the %s limit. Part of the latency the break rests on may be the load generator's own",
		Pct(stall), SrcThrottledPeriods, app, joinAnd(below), SrcAppLatency, Ms(limit))
}

// waitingOn names a full pool's target when the target was busy: the pool
// is what was observed, the target why. Throttling is listed as the
// likely reason when the target was busy.
func waitingOn(a *Analysis, i int, top Saturation) string {
	target, ok := strings.CutPrefix(top.What, "pool to ")
	if !ok || i >= len(a.Windows) {
		return ""
	}
	w := a.Windows[i]
	if len(targetBusy(a, w, top.Component, target)) == 0 {
		return ""
	}
	var parts []string
	if c := cpuOf(w, target); c != nil {
		parts = append(parts, "CPU "+Pct(*c))
	}
	if sw := w.Services[target]; sw != nil && sw.Throttled != nil && *sw.Throttled >= ThrottledMin {
		parts = append(parts, fmt.Sprintf("CPU-throttled %.1f s", *sw.Throttled))
	}
	if p := poolAt(w, top.Component, target); p != nil && p.HeldMs != nil {
		if base, ok := heldBaseline(a, top.Component, target); ok {
			parts = append(parts, fmt.Sprintf("%s per call with a connection, %s at low load", Ms(*p.HeldMs), Ms(base)))
		}
	}
	return fmt.Sprintf(", waiting on %s (%s)", target, strings.Join(parts, ", "))
}

// inconclusive finds signs that Loadlore's own tooling, not the system, set
// the limit before the break (or anywhere, if nothing broke). Dropped
// arrivals in the break window itself are a consequence of the break.
func inconclusive(ws []*Window, breakAt int) (reasons, caveats []string, short []*Window) {
	end := len(ws)
	if breakAt >= 0 {
		end = breakAt + 1
	}
	var drop, failingDrop, failingNote, load, proxy *Window
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
			if w.Dropped != nil && *w.Dropped > 0 && w.Offered > 0 {
				// Arrivals k6 couldn't start lighten the load. Judged by
				// size: where the system was failing requests itself, slow
				// failing responses hold VUs, so a few are a caveat.
				share := *w.Dropped / (w.Offered * WindowSeconds)
				failing := w.ErrorRate != nil && *w.ErrorRate >= BreakErrorRate
				switch {
				case !failing && share > DroppedShareLimit && drop == nil:
					drop = w
				case failing && share > DroppedFailingLimit && failingDrop == nil:
					failingDrop = w
				case failing && share <= DroppedFailingLimit && (failingNote == nil || share > *failingNote.Dropped/(failingNote.Offered*WindowSeconds)):
					failingNote = w
				}
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
	for _, w := range []*Window{drop, failingDrop, load, proxy} {
		if w != nil {
			short = append(short, w)
		}
	}
	if drop != nil {
		reasons = append(reasons, fmt.Sprintf("at %s the load generator dropped %s arrivals, %s of what the profile offered in that window (%s), before the system showed strain: it couldn't start requests fast enough, so the offered load was lower than the profile",
			Clock(drop.Start), Count(*drop.Dropped), Pct(*drop.Dropped/(drop.Offered*WindowSeconds)), SrcDropped))
	}
	if failingDrop != nil {
		reasons = append(reasons, fmt.Sprintf("at %s the load generator dropped %s arrivals, %s of what the profile offered in that window (%s), while %s of requests failed: more than %s, so the missing load could flatter whatever absorbed the trouble",
			Clock(failingDrop.Start), Count(*failingDrop.Dropped), Pct(*failingDrop.Dropped/(failingDrop.Offered*WindowSeconds)), SrcDropped, Pct(*failingDrop.ErrorRate), Pct(DroppedFailingLimit)))
	}
	if failingNote != nil && failingDrop == nil {
		caveats = append(caveats, fmt.Sprintf("at %s the load generator dropped %s arrivals, %s of what the profile offered in that window (%s), while %s of requests failed: slow responses held the VUs new arrivals needed, so that window's load was lighter than the profile by that share",
			Clock(failingNote.Start), Count(*failingNote.Dropped), Pct(*failingNote.Dropped/(failingNote.Offered*WindowSeconds)), SrcDropped, Pct(*failingNote.ErrorRate)))
	}
	if load != nil {
		reasons = append(reasons, fmt.Sprintf("at %s the load generator used %s of its CPU budget (%s), so it may have been slowing requests down itself", Clock(load.Start), Pct(*load.LoadCPU), SrcLoadCPU))
	}
	if proxy != nil {
		reasons = append(reasons, fmt.Sprintf("at %s Toxiproxy, which carries the connections the scenario's faults act on, used %s of its CPU budget (%s), so it may have added latency of its own", Clock(proxy.Start), Pct(*proxy.ProxyCPU), SrcProxyCPU))
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
		if slices.Contains(b.Signals, "errors") {
			s += causeSentence(a.Windows[b.Window : b.Window+1])
		}
		if top, ok := MostSaturated(b.Saturation); ok {
			s += " Most saturated then: " + top.Detail + waitingOn(a, b.Window, top) + "."
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
		if ws := a.Windows; a.Plan != nil && len(ws) > 0 && float64(len(ws)*WindowSeconds) < a.Plan.Duration.Seconds()-WindowSeconds {
			// The load ended early (interrupted, timed out): only what ran.
			offered := 0.0
			for _, w := range ws {
				offered = math.Max(offered, w.Offered)
			}
			s = fmt.Sprintf("Held until the load ended at %s: up to %s rps offered", Clock(float64(len(ws)*WindowSeconds)), Rate(offered))
		}
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
	what, ok := map[string]string{"interrupted": "was interrupted", "timed_out": "timed out", "failed": "failed"}[a.Status]
	if !ok {
		what = strings.ReplaceAll(a.Status, "_", " ")
	}
	if a.Error != "" && a.Error != "interrupted" {
		what += " (" + a.Error + ")"
	}
	return fmt.Sprintf("The run %s, so this covers only the data collected. ", what)
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

// RecoverWindows is how many healthy windows in a row, to the end of the
// load, count as recovered after a break.
const RecoverWindows = 3

// recoveredAt is the start of the first window after the break from which
// every window to the end shows no break signal, if there are at least
// RecoverWindows of them.
func recoveredAt(ws []*Window, hard, sig [][]string, breakAt int) *float64 {
	healthy := func(i int) bool { return len(hard[i]) == 0 && len(sig[i]) == 0 && ws[i].RPS != nil }
	from := len(ws)
	for i := len(ws) - 1; i > breakAt && healthy(i); i-- {
		from = i
	}
	if from <= breakAt || len(ws)-from < RecoverWindows {
		return nil
	}
	at := ws[from].Start
	return &at
}

// splitStalls divides the analyzed windows' accept stalls at the end of
// the break's first window (all are before when nothing broke). A stall
// only adds failure, so one after the break began deepens a failure
// already under way.
func splitStalls(a *Analysis, breakAt int) (before, after []AcceptStall) {
	end := float64(len(a.Windows) * WindowSeconds)
	cut := end
	if breakAt >= 0 {
		cut = a.Windows[breakAt].Start + WindowSeconds
	}
	for _, s := range a.AcceptStalls {
		switch {
		case s.At < 0 || s.At >= end:
		case s.At < cut:
			before = append(before, s)
		default:
			after = append(after, s)
		}
	}
	return before, after
}

// SuspendSlack is how much longer than its profile the load may take on
// the wall clock, at least SuspendMinSeconds: k6 ends a little late, but
// minutes more mean the host stopped during it.
const (
	SuspendSlack      = 0.1
	SuspendMinSeconds = 30.0
)

// suspended says why a run whose load took far longer on the wall clock
// than its profile can't be judged: a host that sleeps (a laptop's idle
// sleep, a closed lid) or stalls freezes the VM, and the windows no longer
// hold the load the profile describes.
func suspended(a *Analysis) string {
	if a.Started == nil || a.Ended == nil || a.Plan == nil || a.Plan.Duration <= 0 {
		return ""
	}
	wall, planned := a.Ended.Sub(*a.Started), a.Plan.Duration
	if wall.Seconds() <= planned.Seconds()+math.Max(SuspendMinSeconds, SuspendSlack*planned.Seconds()) {
		return ""
	}
	return fmt.Sprintf("the %s load took %s on the wall clock: the host slept or stalled during it (on a laptop, idle sleep or a closed lid), so its windows don't hold the load the profile describes", durText(planned), durText(wall))
}

// labFailed says why a run didn't do what its design asked: a fault that
// couldn't be injected, or an autoscaler decision that couldn't be carried
// out. The run then measured a different design from the one it names.
func labFailed(tl []Event) string {
	for _, e := range tl {
		switch e.Kind {
		case "fault_error", "autoscale_error":
			return fmt.Sprintf("at %s Loadlore couldn't carry out the design (%s), so the run isn't the design it names", Clock(e.At), e.Detail)
		}
	}
	return ""
}

// durText is a duration in whole seconds, as 4m30s or 2h7m3s.
func durText(d time.Duration) string { return d.Round(time.Second).String() }
