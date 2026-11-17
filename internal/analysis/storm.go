package analysis

import (
	"fmt"
	"math"
)

// A connection storm: in one window a service's pools to a Postgres open at
// least StormOpenedShare of their combined size, and no fewer than
// StormMinOpened connections, while the entry p99 (in that window or the
// next) is at least StormLatencyFactor times the low-load p99 and
// StormLatencyMinMs above it, up to the break. Pools fill gradually as load rises; in the
// local runs of the first scenario a cold pool filled in one window,
// opening 59 to 105 of 96 connections where other windows opened 0 to 11,
// with p99 at 173 ms to 1.26 s against about 3 ms at low load.
const (
	StormOpenedShare   = 0.5
	StormMinOpened     = 10
	StormLatencyFactor = 10.0
	StormLatencyMinMs  = 10.0
)

// opened is how many connections a pool opened in window i: counted by the
// connect histogram, or for runs from before it, how far the open count
// rose from the end of the window before.
func opened(ws []*Window, i int, svc, target string) (n float64, counted, ok bool) {
	p := poolAt(ws[i], svc, target)
	if p == nil {
		return 0, false, false
	}
	if p.Opened != nil {
		return *p.Opened, true, true
	}
	if i == 0 || p.Open == nil {
		return 0, false, false
	}
	prev := poolAt(ws[i-1], svc, target)
	if prev == nil || prev.OpenEnd == nil {
		return 0, false, false
	}
	return math.Max(0, *p.Open-*prev.OpenEnd), false, true
}

// connectionStorm: a pool to Postgres opening many connections at once as
// latency jumps. pgxpool opens connections only when callers need them, so
// when requests pile up every replica opens more together, and each new
// Postgres connection starts a server process and authenticates on the
// database's CPU just when it is short of it.
func connectionStorm(a *Analysis) []*Diagnosis {
	var out []*Diagnosis
	ws, base := a.Windows, a.Verdict.BaselineP99
	if len(ws) == 0 || base == nil || a.Plan == nil {
		return out
	}
	slow := func(w *Window) bool {
		return w.P99 != nil && *w.P99 >= StormLatencyFactor**base && *w.P99 >= *base+StormLatencyMinMs
	}
	for _, p := range ws[0].Pools {
		svc, target := p.Service, p.Target
		// PgBouncer's own server connections aren't counted as they open,
		// and a rise in them is usually slow transactions, not a burst.
		if t, s := a.Plan.Service(target), a.Plan.Service(svc); t == nil || t.Kind != "postgres" || s == nil || s.Kind != "app" {
			continue
		}
		storm := make([]bool, len(ws))
		for i, w := range ws {
			// After the break, timed-out calls close connections and the
			// pools reopen them: churn from a broken system, not a cause.
			if b := a.Verdict.Break; b != nil && w.Start > b.At {
				break
			}
			n, _, ok := opened(ws, i, svc, target)
			pw := poolAt(w, svc, target)
			if !ok || pw == nil || pw.Max == nil {
				continue
			}
			hit := slow(w) || (i+1 < len(ws) && slow(ws[i+1]))
			storm[i] = hit && n >= StormMinOpened && n >= StormOpenedShare**pw.Max
		}
		first, episodes := -1, 0
		for i := range storm {
			if storm[i] && (i == 0 || !storm[i-1]) {
				episodes++
				if first < 0 {
					first = i
				}
			}
		}
		if first < 0 {
			continue
		}
		last := first
		for last+1 < len(ws) && storm[last+1] {
			last++
		}
		out = append(out, stormDiagnosis(a, svc, target, first, last, episodes))
	}
	return out
}

func stormDiagnosis(a *Analysis, svc, target string, first, last, episodes int) *Diagnosis {
	ws, base := a.Windows, *a.Verdict.BaselineP99
	w := ws[first]
	total, counted := 0.0, false
	for i := first; i <= last; i++ {
		n, c, _ := opened(ws, i, svc, target)
		total, counted = total+n, c
	}
	pw := poolAt(w, svc, target)
	// The worst p99 of the storm's windows and the one after.
	worst := first
	for i := first; i <= min(last+1, len(ws)-1); i++ {
		if ws[i].P99 != nil && (ws[worst].P99 == nil || *ws[i].P99 > *ws[worst].P99) {
			worst = i
		}
	}
	size := int(*pw.Max)
	if s := a.Plan.Service(svc); s != nil && len(s.Synthetic) > 0 {
		size = s.Synthetic[0].Targets[target].Pool
	}
	span := "within 10 seconds"
	if last > first {
		span = fmt.Sprintf("in %d seconds", (last-first+1)*WindowSeconds)
	}
	d := &Diagnosis{
		Rule: "connection-storm", Title: fmt.Sprintf("Connection storm (%s → %s)", svc, target),
		From: w.Start, To: ws[last].Start + WindowSeconds,
		Summary: fmt.Sprintf("At %s, %s's pools opened %s connections to %s %s, at %s rps offered, and p99 reached %s (%s at low load). The pools open connections only when callers need them, so when a short slowdown leaves requests waiting, every replica opens more at once. Each new Postgres connection starts a server process and authenticates, which takes %s's CPU from queries just when it is short of it.",
			Clock(w.Start), svc, Count(total), target, span, Rate(w.Offered), Ms(*ws[worst].P99), Ms(base), target),
		Next: fmt.Sprintf("Open the connections before the load arrives: give the pool a minimum (pools: { %s: { size: %d, min: %d } }) and re-run to compare a warm pool with this cold one. PgBouncer in front of %s also keeps server connections open.", target, size, size, target),
	}
	ev := func(src, format string, args ...any) {
		d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf(format, args...), Source: src})
	}
	src := SrcPoolConnect
	if !counted {
		src = SrcPoolOpen
	}
	t := fmt.Sprintf("%s new connections to %s from %s to %s", Count(total), target, Clock(w.Start), Clock(ws[last].Start+WindowSeconds))
	// The open gauge can lag the connect histogram by a scrape, so the
	// count reached is the most open in the storm's windows or the next.
	if first > 0 {
		prev := poolAt(ws[first-1], svc, target)
		var reached, of *float64
		for i := first; i <= min(last+1, len(ws)-1); i++ {
			if p := poolAt(ws[i], svc, target); p != nil {
				reached, of = maxPtr(reached, p.Open), maxPtr(of, p.Max)
			}
		}
		if prev != nil && prev.OpenEnd != nil && reached != nil && of != nil && *reached > *prev.OpenEnd {
			t += fmt.Sprintf(": from %s open to %s of %s", Count(*prev.OpenEnd), Count(*reached), Count(*of))
		}
	}
	ev(src, "%s", t)
	ev(SrcK6Latency, "p99 %s at %s, against %s at low load", Ms(*ws[worst].P99), Clock(ws[worst].Start), Ms(base))
	if pw.ConnectMs != nil {
		t := fmt.Sprintf("each new connection took %s to open at %s", Ms(*pw.ConnectMs), Clock(w.Start))
		if b, ok := connectBaseline(a, svc, target, first); ok {
			t += fmt.Sprintf(", against %s for the ones opened before", Ms(b))
		}
		ev(SrcPoolConnect, "%s", t)
	}
	psrc := poolSources(a, svc)
	if pw.Pending != nil && *pw.Pending >= 1 {
		ev(psrc.pending, "up to %s %s waiting for a connection at %s", Count(*pw.Pending), psrc.waiters, Clock(w.Start))
	}
	if c := cpuOf(w, target); c != nil {
		ev(SrcCPU, "%s CPU at %s of its limit at %s", target, Pct(*c), Clock(w.Start))
	}
	if e, ok := throttled(a, w, target); ok {
		d.Evidence = append(d.Evidence, e)
	}
	if b, ok := heldBaseline(a, svc, target); ok && pw.HeldMs != nil && *pw.HeldMs >= TargetSlowFactor*b {
		ev(psrc.held, "%s's %ss to %s took %s each once they had a connection at %s, against %s at low load", svc, psrc.unit, target, Ms(*pw.HeldMs), Clock(w.Start), Ms(b))
	}
	if episodes > 1 {
		ev(src, "%d bursts like it in the run; this is the first", episodes)
	}
	return d
}

// connectBaseline is the mean time to open a connection over the windows
// before the storm.
func connectBaseline(a *Analysis, svc, target string, before int) (float64, bool) {
	var secs, n float64
	for _, w := range a.Windows[:before] {
		if p := poolAt(w, svc, target); p != nil && p.ConnectMs != nil && p.Opened != nil && *p.Opened > 0 {
			secs += *p.ConnectMs * *p.Opened
			n += *p.Opened
		}
	}
	return secs / n, n > 0
}
