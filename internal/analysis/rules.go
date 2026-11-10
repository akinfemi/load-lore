package analysis

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Diagnosis is a rule that fired: a pattern in the metrics, the mechanism
// it points to, the evidence, and an experiment to try next.
type Diagnosis struct {
	Rule     string     `json:"rule"`
	Title    string     `json:"title"`
	From     float64    `json:"from_s"`
	To       float64    `json:"to_s"` // end of the first sustained stretch
	Summary  string     `json:"summary"`
	Evidence []Evidence `json:"evidence"`
	Next     string     `json:"next"`
}

// Evidence is one fact behind a diagnosis and the metric it came from.
type Evidence struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

// RuleIDs are the diagnosis rules this version runs, by the id a
// Diagnosis carries in Rule.
var RuleIDs = []string{"pool-exhausted", "queueing-knee"}

// Rule thresholds. Rules are deterministic and versioned with the code.
const (
	PoolFullShare     = 0.95 // in use / pool size
	PoolTargetCPUMax  = 0.8  // the target isn't CPU-saturated below this
	PoolCallerCPUMax  = 0.9  // nor is the caller, whose CPU starvation also delays acquires
	KneeUtilization   = 0.7  // the spec's ~70–80%
	KneeLatencyFactor = 3.0  // p99 at least 3x the low-load p99 ...
	KneeLatencyMinMs  = 10.0 // ... and at least 10 ms above it
	KneeWindows       = 3    // for 30 seconds in a row: a knee doesn't recover by itself
	LittleMinInflight = 20   // below this, sampling noise swamps Little's law
)

// RuleVersion changes whenever a rule's logic or thresholds change.
const RuleVersion = "1"

func diagnose(a *Analysis) []*Diagnosis {
	out := []*Diagnosis{}
	out = append(out, poolExhausted(a)...)
	if d := queueingKnee(a); d != nil {
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out
}

// sustained finds the first stretch of flagged windows that lasts at least
// two windows (or reaches the end of the run).
func sustained(flags []bool) (from, to int, ok bool) {
	for i := range flags {
		if !flags[i] || (i+1 < len(flags) && !flags[i+1]) {
			continue
		}
		j := i
		for j+1 < len(flags) && flags[j+1] {
			j++
		}
		return i, j, true
	}
	return 0, 0, false
}

func poolAt(w *Window, svc, target string) *PoolWindow {
	for _, p := range w.Pools {
		if p.Service == svc && p.Target == target {
			return p
		}
	}
	return nil
}

func cpuOf(w *Window, svc string) *float64 {
	if sw := w.Services[svc]; sw != nil {
		return sw.CPU
	}
	return nil
}

// poolExhausted: connections at the pool limit, callers waiting for one,
// and the database (and the caller) not CPU-saturated. Requests are
// queueing for a connection, not for the database.
func poolExhausted(a *Analysis) []*Diagnosis {
	var out []*Diagnosis
	ws := a.Windows
	if len(ws) == 0 {
		return out
	}
	type key struct{ svc, target string }
	var keys []key
	for _, p := range ws[0].Pools {
		keys = append(keys, key{p.Service, p.Target})
	}
	for _, k := range keys {
		flags := make([]bool, len(ws))
		for i, w := range ws {
			p := poolAt(w, k.svc, k.target)
			if p == nil || p.InUse == nil || p.Max == nil || *p.Max <= 0 || *p.InUse < PoolFullShare**p.Max {
				continue
			}
			waiting := (p.Pending != nil && *p.Pending >= 1) || (p.WaitMs != nil && *p.WaitMs >= 1)
			tc, cc := cpuOf(w, k.target), cpuOf(w, k.svc)
			flags[i] = waiting && (tc == nil || *tc < PoolTargetCPUMax) && (cc == nil || *cc < PoolCallerCPUMax)
		}
		from, to, ok := sustained(flags)
		if !ok {
			continue
		}
		// The window with the most callers waiting shows it best.
		peak := from
		for i := from; i <= to; i++ {
			if pp := poolAt(ws[i], k.svc, k.target).Pending; pp != nil {
				if cur := poolAt(ws[peak], k.svc, k.target).Pending; cur == nil || *pp > *cur {
					peak = i
				}
			}
		}
		p, w := poolAt(ws[peak], k.svc, k.target), ws[peak]
		d := &Diagnosis{
			Rule: "pool-exhausted", Title: fmt.Sprintf("Connection pool exhausted (%s → %s)", k.svc, k.target),
			From: ws[from].Start, To: ws[to].Start + WindowSeconds,
			Summary: fmt.Sprintf("From %s, every connection in %s's pool to %s was in use and requests waited for one, while %s still had CPU to spare. Requests were queueing for a connection, not for the database.",
				Clock(ws[from].Start), k.svc, k.target, k.target),
		}
		ev := func(src, format string, args ...any) {
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf(format, args...), Source: src})
		}
		first := poolAt(ws[from], k.svc, k.target)
		ev(SrcPoolInUse, "%s of %s connections in use at %s (%s to %s)", Count(*first.InUse), Count(*first.Max), Clock(ws[from].Start), Clock(ws[from].Start), Clock(ws[to].Start+WindowSeconds))
		if p.Pending != nil {
			ev(SrcPoolPending, "up to %s requests waiting for a connection at %s", Count(*p.Pending), Clock(w.Start))
		}
		if p.WaitMs != nil {
			t := fmt.Sprintf("mean wait for a connection %s at %s", Ms(*p.WaitMs), Clock(w.Start))
			if from > 0 {
				if before := poolAt(ws[from-1], k.svc, k.target); before != nil && before.WaitMs != nil {
					t += fmt.Sprintf(", up from %s at %s", Ms(*before.WaitMs), Clock(ws[from-1].Start))
				}
			}
			ev(SrcPoolWait, "%s", t)
		}
		if c := cpuOf(w, k.target); c != nil {
			ev(SrcCPU, "%s CPU at %s of its limit at %s: not saturated", k.target, Pct(*c), Clock(w.Start))
		}
		if c := cpuOf(w, k.svc); c != nil {
			ev(SrcCPU, "%s CPU at %s of its limit at %s", k.svc, Pct(*c), Clock(w.Start))
		}
		if sw := w.Services[k.target]; sw != nil && sw.Connections != nil && sw.MaxConnections != nil {
			ev(SrcPGConns, "%s had %s connections open, max_connections %s", k.target, Count(*sw.Connections), Count(*sw.MaxConnections))
		}
		if e, ok := throttled(a, ws[from], k.target); ok {
			d.Evidence = append(d.Evidence, e)
		}
		d.Next = "Add PgBouncer, or resize the pools across replicas"
		if s := a.Plan.Service(k.svc); s != nil && len(s.Synthetic) > 0 {
			pool := s.Synthetic[0].Targets[k.target].Pool
			n := len(s.Containers)
			d.Next += fmt.Sprintf(": %d %s × %d connections = %d", n, plural(n, "replica", "replicas"), pool, n*pool)
			if t := a.Plan.Service(k.target); t != nil && t.Config["max_connections"] != "" {
				d.Next += ", against max_connections " + t.Config["max_connections"]
			}
		}
		d.Next += "."
		out = append(out, d)
	}
	return out
}

// queueingKnee: p99 rising sharply as a resource's utilization passes about
// 70–80%. Near saturation requests wait behind each other, so latency grows
// non-linearly (Little's law).
func queueingKnee(a *Analysis) *Diagnosis {
	ws := a.Windows
	base := a.Verdict.BaselineP99
	if base == nil || len(ws) == 0 {
		return nil
	}
	high := func(w *Window) bool {
		return w.P99 != nil && *w.P99 >= KneeLatencyFactor**base && *w.P99 >= *base+KneeLatencyMinMs
	}
	for k := 0; k+KneeWindows <= len(ws); k++ {
		w := ws[k]
		ok := true
		for j := k; j < k+KneeWindows; j++ {
			ok = ok && high(ws[j])
		}
		if !ok {
			continue
		}
		var top *Saturation
		for _, s := range saturation(a, w) {
			if s.What != "connections" && s.Value >= KneeUtilization {
				top = &s
				break
			}
		}
		if top == nil {
			continue // latency rose with nothing near saturation: not this rule
		}
		return kneeDiagnosis(a, k, *top)
	}
	return nil
}

func kneeDiagnosis(a *Analysis, k int, r Saturation) *Diagnosis {
	ws := a.Windows
	base := *a.Verdict.BaselineP99
	util := func(w *Window) (float64, bool) {
		for _, s := range saturation(a, w) {
			if s.Component == r.Component && s.What == r.What {
				return s.Value, true
			}
		}
		return 0, false
	}
	w := ws[k]
	name := r.Component + " " + r.What
	d := &Diagnosis{
		Rule: "queueing-knee", Title: "Queueing knee (" + name + ")",
		From: w.Start, To: ws[k+KneeWindows-1].Start + WindowSeconds,
		Summary: fmt.Sprintf("From %s, p99 stayed above %s (%s at low load) with %s at %s. Near saturation requests wait behind each other, so latency grows non-linearly, not in proportion to load.",
			Clock(w.Start), Ms(math.Max(KneeLatencyFactor*base, base+KneeLatencyMinMs)), Ms(base), name, Pct(r.Value)),
		Next: "Add capacity or cut per-request work on " + r.Component + ", then re-run the ramp.",
	}

	// The knee's shape: the median p99 of the windows in each band of
	// utilization, over the whole run.
	bands := []struct {
		lo, hi float64
		label  string
		p99s   []float64
	}{{0, 0.5, "below 50%", nil}, {0.5, 0.7, "50–70%", nil}, {0.7, 0.8, "70–80%", nil}, {0.8, 0.9, "80–90%", nil}, {0.9, math.Inf(1), "90% and up", nil}}
	for _, x := range ws {
		u, ok := util(x)
		if !ok || x.P99 == nil {
			continue
		}
		for i := range bands {
			if u >= bands[i].lo && u < bands[i].hi {
				bands[i].p99s = append(bands[i].p99s, *x.P99)
			}
		}
	}
	var parts []string
	for _, b := range bands {
		if len(b.p99s) == 0 {
			continue
		}
		sort.Float64s(b.p99s)
		parts = append(parts, fmt.Sprintf("%s: %s (%d %s)", b.label, Ms(median(b.p99s)), len(b.p99s), plural(len(b.p99s), "window", "windows")))
	}
	d.Evidence = append(d.Evidence, Evidence{
		Text:   fmt.Sprintf("median end-to-end p99 by %s: %s", name, strings.Join(parts, "; ")),
		Source: SrcK6Latency + "; " + r.Source,
	})

	var us, ps []string
	for j := k; j < k+KneeWindows; j++ {
		u, _ := util(ws[j])
		us = append(us, Pct(u))
		ps = append(ps, Ms(*ws[j].P99))
	}
	d.Evidence = append(d.Evidence, Evidence{
		Text:   fmt.Sprintf("%s to %s: p99 %s with %s %s", Clock(w.Start), Clock(ws[k+KneeWindows-1].Start+WindowSeconds), strings.Join(ps, ", "), name, strings.Join(us, ", ")),
		Source: SrcK6Latency + "; " + r.Source,
	})

	// Throttling explains a knee that comes at a modest average: bursts hit
	// the CPU limit.
	comps := []string{r.Component}
	if target, ok := strings.CutPrefix(r.What, "pool to "); ok {
		comps = append(comps, target)
	}
	for _, c := range comps {
		if e, ok := throttled(a, w, c); ok {
			d.Evidence = append(d.Evidence, e)
		}
	}

	// Little's law on the first application service: requests in flight =
	// arrival rate × time in the system.
	if app := entryApp(a); app != "" {
		if sw := w.Services[app]; sw != nil && sw.RPS != nil && sw.MeanMs != nil && sw.InflightMean != nil && *sw.RPS**sw.MeanMs/1000 >= LittleMinInflight {
			lw := *sw.RPS * *sw.MeanMs / 1000
			d.Evidence = append(d.Evidence, Evidence{
				Text: fmt.Sprintf("Little's law on %s at %s: %s rps × %s mean time in the service = %s in flight; measured %s",
					app, Clock(w.Start), Rate(*sw.RPS), Ms(*sw.MeanMs), Count(lw), Count(*sw.InflightMean)),
				Source: SrcInflight + "; rate and mean from " + SrcAppLatency,
			})
		}
	}
	return d
}

func median(sorted []float64) float64 {
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// ThrottledMin is the CFS throttling in a window worth pointing out.
const ThrottledMin = 1.0 // seconds, summed over the container's threads

// throttled is evidence that a component hit its CPU limit in bursts.
func throttled(a *Analysis, w *Window, name string) (Evidence, bool) {
	sw, s := w.Services[name], a.Plan.Service(name)
	if sw == nil || s == nil || sw.Throttled == nil || *sw.Throttled < ThrottledMin {
		return Evidence{}, false
	}
	t := fmt.Sprintf("%s was CPU-throttled for %.1f s (summed over its threads) in the window from %s", name, *sw.Throttled, Clock(w.Start))
	if sw.CPU != nil {
		t += fmt.Sprintf(", while its average CPU was %s of its limit: bursts of work hit the %g-CPU limit and waited for the next period", Pct(*sw.CPU), s.CPU)
	}
	return Evidence{Text: t, Source: SrcThrottled}, true
}

// entryApp is the application service that receives the load, looking
// through an lb.
func entryApp(a *Analysis) string {
	name := a.Plan.Workload.Entry
	if s := a.Plan.Service(name); s != nil && s.LB != nil {
		name = s.LB.Target
	}
	if s := a.Plan.Service(name); s != nil && s.Kind == "app" && len(s.Synthetic) > 0 {
		return name
	}
	return ""
}
