package analysis

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
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

	pool string // "svc → target" for pool-exhausted, and for a knee named after a pool
}

// Evidence is one fact behind a diagnosis and the metric it came from.
type Evidence struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

// RuleIDs are the diagnosis rules this version runs, by the id a
// Diagnosis carries in Rule.
var RuleIDs = []string{"pool-exhausted", "queueing-knee", "connection-storm", "retry-storm", "cold-cache", "working-set", "hot-rows"}

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
	// A pool's target is busy, so a full pool is its symptom, when its CPU
	// is at PoolTargetCPUMax or more or its time per call with a connection
	// rose this much. Throttling alone doesn't count: a Postgres at 0.5 CPU
	// is throttled for a second or more in most windows from 30% CPU, with
	// a handful of connections in use, while it still answers at its usual
	// speed. When throttling does slow it, the time per call shows it.
	TargetSlowFactor = 3.0 // at least 3x its low-load value ...
	TargetSlowMinMs  = 1.0 // ... and at least 1 ms above it
	// A caller's CPU tops a knee, but its requests spent at least this
	// share of their time in calls to a busy target: the target is named.
	KneeDownstreamShare = 0.5
	// A connection held this many times longer than its calls took is
	// held across the request, mostly idle.
	HoldIdleFactor = 2.0
)

// RuleVersion changes whenever a rule's logic or thresholds change.
const RuleVersion = "7"

func diagnose(a *Analysis) []*Diagnosis {
	out := []*Diagnosis{}
	exhausted := poolExhausted(a)
	out = append(out, exhausted...)
	out = append(out, connectionStorm(a)...)
	out = append(out, retryStorm(a)...)
	out = append(out, coldCache(a)...)
	out = append(out, workingSet(a)...)
	out = append(out, hotRows(a)...)
	// A knee named after a pool whose target kept up is pool-exhausted's
	// case; when that rule fired for the pool, the knee would only repeat it.
	if d := queueingKnee(a); d != nil && (d.pool == "" || !slices.ContainsFunc(exhausted, func(x *Diagnosis) bool { return x.pool == d.pool })) {
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
			cc := cpuOf(w, k.svc)
			flags[i] = waiting && len(targetBusy(a, w, k.svc, k.target)) == 0 && (cc == nil || *cc < PoolCallerCPUMax)
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
			Rule: "pool-exhausted", Title: fmt.Sprintf("Connection pool exhausted (%s → %s)", k.svc, k.target), pool: k.svc + " → " + k.target,
			From: ws[from].Start, To: ws[to].Start + WindowSeconds,
			Summary: fmt.Sprintf("From %s, every connection in %s's pool to %s was in use and requests waited for one, while %s still had CPU to spare. Requests were queueing for a connection, not for the database.",
				Clock(ws[from].Start), k.svc, k.target, k.target),
		}
		ev := func(src, format string, args ...any) {
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf(format, args...), Source: src})
		}
		src := poolSources(a, k.svc)
		first := poolAt(ws[from], k.svc, k.target)
		ev(src.inUse, "%s of %s %s in use at %s (%s to %s)", Count(*first.InUse), Count(*first.Max), src.conns, Clock(ws[from].Start), Clock(ws[from].Start), Clock(ws[to].Start+WindowSeconds))
		if p.Pending != nil {
			ev(src.pending, "up to %s %s waiting for a connection at %s", Count(*p.Pending), src.waiters, Clock(w.Start))
		}
		if p.WaitMs != nil {
			t := fmt.Sprintf("mean wait for a connection %s at %s", Ms(*p.WaitMs), Clock(w.Start))
			if from > 0 {
				if before := poolAt(ws[from-1], k.svc, k.target); before != nil && before.WaitMs != nil {
					t += fmt.Sprintf(", up from %s at %s", Ms(*before.WaitMs), Clock(ws[from-1].Start))
				}
			}
			ev(src.wait, "%s", t)
		}
		if c := cpuOf(w, k.target); c != nil {
			ev(SrcCPU, "%s CPU at %s of its limit at %s: not saturated", k.target, Pct(*c), Clock(w.Start))
		}
		if c := cpuOf(w, k.svc); c != nil {
			ev(SrcCPU, "%s CPU at %s of its limit at %s", k.svc, Pct(*c), Clock(w.Start))
		}
		if sw := w.Services[k.target]; sw != nil && sw.Connections != nil && sw.MaxConnections != nil {
			ev(connSource(a, k.target), "%s had %s connections open, %s %s", k.target, Count(*sw.Connections), connLimit(a, k.target), Count(*sw.MaxConnections))
		}
		if base, ok := heldBaseline(a, k.svc, k.target); ok && p.HeldMs != nil {
			ev(src.held, "%s per %s once a connection was free at %s, against %s at low load: %s answered at its usual speed", Ms(*p.HeldMs), src.unit, Clock(w.Start), Ms(base), k.target)
		}
		// Held across a whole request, a connection is busy far longer
		// than its queries take: that, not the target, fills the pool.
		if p.HoldMs != nil && p.HeldMs != nil && *p.HoldMs >= HoldIdleFactor**p.HeldMs {
			ev(SrcPoolHold, "each connection was held %s at %s while its calls took %s with it: held across the request, it sat idle most of that time", Ms(*p.HoldMs), Clock(w.Start), Ms(*p.HeldMs))
		}
		if b := a.Plan.Service(k.svc); b != nil && b.PgBouncer != nil {
			d.Summary = fmt.Sprintf("From %s, every server connection %s had to %s was in use and clients waited in %s for one, while %s still had CPU to spare. Requests were queueing for a connection, not for the database.",
				Clock(ws[from].Start), k.svc, k.target, k.svc, k.target)
			d.Next = fmt.Sprintf("Raise %s's default_pool_size (%d now), keeping it and every other connection under %s's max_connections", k.svc, b.PgBouncer.DefaultPoolSize, k.target)
			if t := a.Plan.Service(k.target); t != nil && t.Config["max_connections"] != "" {
				d.Next += " (" + t.Config["max_connections"] + ")"
			}
			if b.PgBouncer.PoolMode == "session" {
				d.Next += ", or switch pool_mode to transaction: in session mode a client holds a server connection for as long as it stays connected."
			} else {
				d.Next += ", or cut the time each transaction holds one."
			}
			out = append(out, d)
			continue
		}
		if t := a.Plan.Service(k.target); t != nil && t.PgBouncer != nil {
			d.Next = fmt.Sprintf("Resize the pools to %s across replicas: client connections to PgBouncer are cheap, and it queues the ones beyond its %d server connections", k.target, t.PgBouncer.DefaultPoolSize)
			if s := a.Plan.Service(k.svc); s != nil && len(s.Synthetic) > 0 {
				pool, n := s.Synthetic[0].Targets[k.target].Pool, len(s.Containers)
				d.Next += fmt.Sprintf(": %d %s × %d connections = %d, against max_client_conn %d", n, plural(n, "replica", "replicas"), pool, n*pool, t.PgBouncer.MaxClientConn)
			}
			d.Next += "."
			out = append(out, d)
			continue
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
		// A full pool in front of a busy target is the target's queue:
		// connections are held longer because the target is slow. Name
		// the target. The pool alone is to blame only when the target is
		// idle, which is pool-exhausted's case.
		if target, ok := strings.CutPrefix(top.What, "pool to "); ok {
			for j := k; j < k+KneeWindows; j++ {
				why := targetBusy(a, ws[j], top.Component, target)
				if len(why) == 0 {
					continue
				}
				pool := *top
				next, ok := slowTarget(a, ws[k:k+KneeWindows], target)
				if !ok {
					break
				}
				symptom := Evidence{Text: fmt.Sprintf("%s at %s: a symptom, since connections stayed in use longer while %s was slow to answer", pool.Detail, Clock(w.Start), target), Source: pool.Source}
				if r, more, ok := behindPgBouncer(a, ws[k:k+KneeWindows], target); ok {
					next, why = &r, append(why, more...)
				} else if a.Plan.Service(target).PgBouncer != nil {
					break // a PgBouncer that kept up: not where requests queued
				}
				return kneeDiagnosis(a, k, *next, &symptom, why)
			}
		}
		// A caller's CPU near its limit isn't where requests queue when
		// most of their time went to calls to a busy target: that target
		// is the slow part, and the caller is waiting on it.
		if top.What == "CPU" {
			for j := k; j < k+KneeWindows; j++ {
				target, why := downstreamBusy(a, ws[j], top.Component)
				if target == "" {
					continue
				}
				next, ok := slowTarget(a, ws[k:k+KneeWindows], target)
				if !ok {
					continue
				}
				symptom := Evidence{Text: fmt.Sprintf("%s at %s: not where requests queued, since most of their time went to calls to %s", top.Detail, Clock(w.Start), target), Source: top.Source}
				if r, more, ok := behindPgBouncer(a, ws[k:k+KneeWindows], target); ok {
					next, why = &r, append(why, more...)
				} else if a.Plan.Service(target).PgBouncer != nil {
					continue
				}
				return kneeDiagnosis(a, k, *next, &symptom, why)
			}
		}
		return kneeDiagnosis(a, k, *top, nil, nil)
	}
	return nil
}

// slowTarget names a target found busy over a knee's windows: its CPU,
// as of the knee's first window, when it reached KneeUtilization or was
// throttled in any of them; else its slowdown, since it answered more
// slowly for some other reason. ok is false without CPU data.
func slowTarget(a *Analysis, ws []*Window, target string) (*Saturation, bool) {
	first := cpuOf(ws[0], target)
	if first == nil {
		return nil, false
	}
	peak, throttledAny := 0.0, false
	for _, w := range ws {
		if v := cpuOf(w, target); v != nil {
			peak = math.Max(peak, *v)
		}
		_, t := throttled(a, w, target)
		throttledAny = throttledAny || t
	}
	if peak >= KneeUtilization || throttledAny {
		return &Saturation{Component: target, What: "CPU", Value: *first, Detail: fmt.Sprintf("%s CPU at %s of its limit", target, Pct(*first)), Source: SrcCPU}, true
	}
	return &Saturation{Component: target, What: slowerWhat, Value: peak, Detail: fmt.Sprintf("%s answering more slowly with its CPU at most %s of its limit", target, Pct(peak)), Source: SrcCPU}, true
}

// slowerWhat marks a knee named after a target that slowed down without
// running out of CPU.
const slowerWhat = "slower"

// behindPgBouncer follows a slow PgBouncer to what made it slow over the
// knee's windows: its Postgres, when that was busy; else its server pool,
// when full, since clients then queue in PgBouncer; else its own CPU, when
// at PoolTargetCPUMax or more. ok is false for a target that isn't a
// PgBouncer, or one that kept up.
func behindPgBouncer(a *Analysis, ws []*Window, target string) (Saturation, []Evidence, bool) {
	t := a.Plan.Service(target)
	if t == nil || t.PgBouncer == nil {
		return Saturation{}, nil, false
	}
	db := t.PgBouncer.Target
	full := func(w *Window) (*PoolWindow, bool) {
		p := poolAt(w, target, db)
		return p, p != nil && p.InUse != nil && p.Max != nil && *p.Max > 0 && *p.InUse >= PoolFullShare**p.Max
	}
	for _, w := range ws {
		why := targetBusy(a, w, target, db)
		r, ok := slowTarget(a, ws, db)
		if len(why) == 0 || !ok {
			continue
		}
		ev := []Evidence{}
		if p, ok := full(w); ok {
			ev = append(ev, Evidence{Text: fmt.Sprintf("%s's %s of %s server connections to %s were in use at %s, so its clients waited for %s", target, Count(*p.InUse), Count(*p.Max), db, Clock(w.Start), db), Source: SrcPgbPool})
		}
		return *r, append(ev, why...), true
	}
	for _, w := range ws {
		if p, ok := full(w); ok {
			u := *p.InUse / *p.Max
			return Saturation{Component: target, What: "pool to " + db, Value: u,
				Detail: fmt.Sprintf("%s → %s pool %s of %s server connections in use", target, db, Count(*p.InUse), Count(*p.Max)), Source: SrcPgbPool}, nil, true
		}
	}
	for _, w := range ws {
		if c := cpuOf(w, target); c != nil && *c >= PoolTargetCPUMax {
			return Saturation{Component: target, What: "CPU", Value: *c, Detail: fmt.Sprintf("%s CPU at %s of its limit", target, Pct(*c)), Source: SrcCPU}, nil, true
		}
	}
	return Saturation{}, nil, false
}

// connLimit and connSource name a service's connection limit and where its
// connection count comes from: max_connections on a Postgres,
// max_client_conn on a PgBouncer.
func connLimit(a *Analysis, svc string) string {
	if s := a.Plan.Service(svc); s != nil && s.PgBouncer != nil {
		return "max_client_conn"
	}
	return "max_connections"
}

func connSource(a *Analysis, svc string) string {
	if s := a.Plan.Service(svc); s != nil && s.PgBouncer != nil {
		return SrcPgbClients
	}
	return SrcPGConns
}

// poolSrc names a pool's metrics and its words: an application service's
// own pools, or a PgBouncer's server connections.
type poolSrc struct {
	inUse, pending, wait, held string
	conns, waiters, unit       string
}

func poolSources(a *Analysis, svc string) poolSrc {
	if s := a.Plan.Service(svc); s != nil && s.PgBouncer != nil {
		return poolSrc{inUse: SrcPgbPool, pending: SrcPgbWaiting, wait: SrcPgbWait, held: SrcPgbHeld,
			conns: "server connections", waiters: "clients", unit: "transaction"}
	}
	return poolSrc{inUse: SrcPoolInUse, pending: SrcPoolPending, wait: SrcPoolWait, held: SrcPoolHeld,
		conns: "connections", waiters: "requests", unit: "call"}
}

// downstreamBusy finds a target svc's requests spent most of their time
// calling in w, at least KneeDownstreamShare of it, while the target was
// busy. It returns the target and the evidence, or "" when there is none.
func downstreamBusy(a *Analysis, w *Window, svc string) (string, []Evidence) {
	sw := w.Services[svc]
	if sw == nil || sw.RPS == nil || sw.MeanMs == nil || *sw.RPS <= 0 || *sw.MeanMs <= 0 {
		return "", nil
	}
	for _, p := range w.Pools {
		if p.Service != svc || p.HeldMs == nil || p.WaitMs == nil {
			continue
		}
		var calls *float64
		for _, e := range w.Edges {
			if e.From == svc && e.To == p.Target {
				calls = e.RPS
			}
		}
		if calls == nil {
			continue
		}
		perReq, perCall := *calls / *sw.RPS, *p.HeldMs+*p.WaitMs
		share := perReq * perCall / *sw.MeanMs
		if share < KneeDownstreamShare {
			continue
		}
		busy := targetBusy(a, w, svc, p.Target)
		if len(busy) == 0 {
			continue
		}
		e := Evidence{Text: fmt.Sprintf("%s's requests spent %s of their time in calls to %s at %s: %s calls per request × %s each, against %s per request",
			svc, Pct(math.Min(share, 1)), p.Target, Clock(w.Start), strconv.FormatFloat(math.Round(perReq*10)/10, 'f', -1, 64), Ms(perCall), Ms(*sw.MeanMs)),
			Source: SrcCalls + "; " + SrcCallLatency + "; " + SrcAppLatency}
		return p.Target, append([]Evidence{e}, busy...)
	}
	return "", nil
}

// targetBusy is the evidence that a pool's target was the slow part in a
// window: its CPU near its limit, or calls that took longer once they had
// a connection. Nil means the target kept up. CPU throttling in the window
// is added as the likely reason, but isn't evidence alone.
func targetBusy(a *Analysis, w *Window, svc, target string) []Evidence {
	var out []Evidence
	if c := cpuOf(w, target); c != nil && *c >= PoolTargetCPUMax {
		out = append(out, Evidence{Text: fmt.Sprintf("%s CPU at %s of its limit at %s", target, Pct(*c), Clock(w.Start)), Source: SrcCPU})
	}
	p := poolAt(w, svc, target)
	if base, ok := heldBaseline(a, svc, target); ok && p != nil && p.HeldMs != nil && *p.HeldMs >= TargetSlowFactor*base && *p.HeldMs >= base+TargetSlowMinMs {
		src := poolSources(a, svc)
		out = append(out, Evidence{Text: fmt.Sprintf("%s's %ss to %s took %s each once they had a connection at %s, against %s at low load", svc, src.unit, target, Ms(*p.HeldMs), Clock(w.Start), Ms(base)), Source: src.held})
	}
	if e, ok := throttled(a, w, target); ok && len(out) > 0 {
		out = append(out, e)
	}
	return out
}

// heldBaseline is the median time per call with a connection over the
// windows the low-load p99 comes from.
func heldBaseline(a *Analysis, svc, target string) (float64, bool) {
	var xs []float64
	for _, w := range healthy(a.Windows) {
		if p := poolAt(w, svc, target); p != nil && p.HeldMs != nil && w.Offered <= BaselineShare*a.PeakOffered {
			xs = append(xs, *p.HeldMs)
		}
	}
	if len(xs) == 0 {
		return 0, false
	}
	sort.Float64s(xs)
	return median(xs), true
}

// kneeDiagnosis names r. pool, when set, is the full pool in front of r
// and why says r was busy.
func kneeDiagnosis(a *Analysis, k int, r Saturation, symptom *Evidence, why []Evidence) *Diagnosis {
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
	if r.What == slowerWhat {
		return slowerKnee(a, k, r, symptom, why)
	}
	d := &Diagnosis{
		Rule: "queueing-knee", Title: "Queueing knee (" + name + ")",
		From: w.Start, To: ws[k+KneeWindows-1].Start + WindowSeconds,
		Summary: fmt.Sprintf("From %s, p99 stayed above %s (%s at low load) with %s at %s. Near saturation requests wait behind each other, so latency grows non-linearly, not in proportion to load.",
			Clock(w.Start), Ms(math.Max(KneeLatencyFactor*base, base+KneeLatencyMinMs)), Ms(base), name, Pct(r.Value)),
		Next: "Add capacity or cut per-request work on " + r.Component + ", then re-run the ramp.",
	}
	if t, ok := strings.CutPrefix(r.What, "pool to "); ok {
		d.pool = r.Component + " → " + t
		if b := a.Plan.Service(r.Component); b != nil && b.PgBouncer != nil {
			d.Next = fmt.Sprintf("Give %s more server connections (default_pool_size %d) if %s has room for them, or cut the time each transaction holds one, then re-run the ramp.", r.Component, b.PgBouncer.DefaultPoolSize, t)
		}
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
	if symptom != nil {
		d.Evidence = append(d.Evidence, *symptom)
		d.Evidence = append(d.Evidence, why...)
	}

	// Throttling explains a knee that comes at a modest average: bursts hit
	// the CPU limit.
	comps := []string{r.Component}
	if target, ok := strings.CutPrefix(r.What, "pool to "); ok {
		comps = append(comps, target)
	}
	for _, c := range comps {
		if e, ok := throttled(a, w, c); ok && !slices.Contains(d.Evidence, e) {
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
	if a.Plan == nil {
		return ""
	}
	name := a.Plan.Workload.Entry
	if s := a.Plan.Service(name); s != nil && s.LB != nil {
		name = s.LB.Target
	}
	if s := a.Plan.Service(name); s != nil && s.Kind == "app" && len(s.Synthetic) > 0 {
		return name
	}
	return ""
}

// slowerKnee is a knee whose target answered more slowly without running
// out of CPU: callers held their connections longer and queued for them.
func slowerKnee(a *Analysis, k int, r Saturation, symptom *Evidence, why []Evidence) *Diagnosis {
	ws := a.Windows
	base, w := *a.Verdict.BaselineP99, ws[k]
	var ps []string
	for j := k; j < k+KneeWindows; j++ {
		ps = append(ps, Ms(*ws[j].P99))
	}
	d := &Diagnosis{
		Rule: "queueing-knee", Title: "Queueing knee (" + r.Component + ", slower without running out of CPU)",
		From: w.Start, To: ws[k+KneeWindows-1].Start + WindowSeconds,
		Summary: fmt.Sprintf("From %s, p99 stayed above %s (%s at low load): %s answered more slowly while its CPU stayed at most %s of its limit, so callers held their connections longer and queued for them. The slowdown wasn't its CPU.",
			Clock(w.Start), Ms(math.Max(KneeLatencyFactor*base, base+KneeLatencyMinMs)), Ms(base), r.Component, Pct(r.Value)),
		Next: fmt.Sprintf("Find what %s waited on besides CPU, such as disk writes or locks, in its own logs and metrics, then re-run the ramp.", r.Component),
	}
	d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s to %s: p99 %s", Clock(w.Start), Clock(ws[k+KneeWindows-1].Start+WindowSeconds), strings.Join(ps, ", ")), Source: SrcK6Latency})
	if symptom != nil {
		d.Evidence = append(d.Evidence, *symptom)
	}
	d.Evidence = append(d.Evidence, why...)
	d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s CPU at most %s of its limit from %s to %s", r.Component, Pct(r.Value), Clock(w.Start), Clock(ws[k+KneeWindows-1].Start+WindowSeconds)), Source: SrcCPU})
	return d
}
