package analysis

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/akinfemi/load-lore/internal/plan"
)

// A retry storm: a caller's call attempts to a target, per request it
// handles, reach RetryAmplification times their low-load value, with
// retries at least RetryShareMin of those attempts, for two windows in a
// row, while the target is failing or slow. Retrying a dependency that is
// already struggling sends it more traffic when it can least take it, so
// the slowdown feeds itself.
const (
	RetryAmplification = 1.5
	RetryShareMin      = 0.2
	// The target counts as slow when its calls' p99 is this many times the
	// low-load value, or as failing at BreakErrorRate.
	RetryTargetSlowFactor = 3.0
)

// Sources of the retry storm's evidence.
const (
	SrcRetries  = "loadlore_call_retries_total ÷ loadlore_calls_total (the caller's own counts)"
	SrcPerReq   = "loadlore_calls_total ÷ loadlore_requests_total (the caller's own counts)"
	SrcCallP99  = "loadlore_call_duration_seconds (the caller's view of the target)"
	SrcCallErrs = "loadlore_calls_total{outcome=\"error\"} ÷ loadlore_calls_total"
)

type edgeKey struct{ svc, target string }

// retryEdges are the calls that may be retried: from services whose
// resilience settings retry.
func retryEdges(p *plan.Plan) []edgeKey {
	var out []edgeKey
	for _, s := range p.Services {
		if len(s.Synthetic) == 0 {
			continue
		}
		cfg := s.Synthetic[0]
		if r := cfg.Resilience.Retries; r == nil || r.Max <= 0 {
			continue
		}
		var ts []string
		for t := range cfg.Targets {
			ts = append(ts, t)
		}
		sort.Strings(ts)
		for _, t := range ts {
			out = append(out, edgeKey{s.Name, t})
		}
	}
	return out
}

func edgeAt(w *Window, from, to string) *EdgeWindow {
	for _, e := range w.Edges {
		if e.From == from && e.To == to {
			return e
		}
	}
	return nil
}

// retryStorm finds callers whose retries multiplied their traffic to a
// failing or slow target.
func retryStorm(a *Analysis) []*Diagnosis {
	var out []*Diagnosis
	ws := a.Windows
	if len(ws) == 0 || a.Plan == nil {
		return out
	}
	for _, k := range retryEdges(a.Plan) {
		perReq := make([]*float64, len(ws))    // attempts per request
		retryShare := make([]float64, len(ws)) // retries / attempts
		for i, w := range ws {
			e, sw := edgeAt(w, k.svc, k.target), w.Services[k.svc]
			if e == nil || e.RPS == nil || sw == nil || sw.RPS == nil || *sw.RPS <= 0 {
				continue
			}
			v := *e.RPS / *sw.RPS
			perReq[i] = &v
			if e.Retries != nil && *e.RPS > 0 {
				retryShare[i] = *e.Retries / *e.RPS
			}
		}
		// The low-load value: windows past the warm-up with almost no
		// retries, before anything went wrong.
		var base, baseP99 []float64
		for i, w := range ws {
			if w.Start < ToolWarmupSeconds || perReq[i] == nil || retryShare[i] >= 0.05 {
				continue
			}
			if e := edgeAt(w, k.svc, k.target); e != nil && e.ErrorRate != nil && *e.ErrorRate >= BreakErrorRate {
				continue
			}
			base = append(base, *perReq[i])
			if e := edgeAt(w, k.svc, k.target); e != nil && e.P99 != nil {
				baseP99 = append(baseP99, *e.P99)
			}
			if len(base) >= 6 {
				break
			}
		}
		if len(base) < 2 {
			continue
		}
		sort.Float64s(base)
		sort.Float64s(baseP99)
		b := median(base)
		var bp99 *float64
		if len(baseP99) > 0 {
			m := median(baseP99)
			bp99 = &m
		}
		trouble := func(w *Window) bool {
			e := edgeAt(w, k.svc, k.target)
			if e == nil {
				return false
			}
			return e.ErrorRate != nil && *e.ErrorRate >= BreakErrorRate ||
				bp99 != nil && e.P99 != nil && *e.P99 >= RetryTargetSlowFactor**bp99 && *e.P99 >= *bp99+KneeLatencyMinMs
		}
		flags := make([]bool, len(ws))
		for i, w := range ws {
			flags[i] = perReq[i] != nil && *perReq[i] >= RetryAmplification*b && retryShare[i] >= RetryShareMin && trouble(w)
		}
		from, to, ok := sustained(flags)
		if !ok {
			continue
		}
		// The window with the most amplification shows it best.
		peak := from
		for i := from; i <= to; i++ {
			if *perReq[i] > *perReq[peak] {
				peak = i
			}
		}
		w := ws[peak]
		e := edgeAt(w, k.svc, k.target)
		r := a.Plan.Service(k.svc).Synthetic[0].Resilience.Retries
		d := &Diagnosis{
			comp: k.target, caller: k.svc, amp: *perReq[peak] / b,
			Rule:  "retry-storm",
			Title: fmt.Sprintf("Retry storm (%s → %s)", k.svc, k.target),
			From:  ws[from].Start,
			To:    ws[to].Start + WindowSeconds,
			Summary: fmt.Sprintf("From %s, %s sent %.1f calls to %s for each request it handled, against %.1f at low load: %s of them were retries of calls that had failed. Retrying a dependency that is already failing sends it more traffic when it can least take it, so the slowdown feeds itself.",
				Clock(ws[from].Start), k.svc, *perReq[peak], k.target, b, Pct(retryShare[peak])),
		}
		d.Evidence = append(d.Evidence,
			Evidence{Text: fmt.Sprintf("%.1f calls to %s per request at %s, %.1fx the low-load %.1f", *perReq[peak], k.target, Clock(w.Start), *perReq[peak]/b, b), Source: SrcPerReq},
			Evidence{Text: fmt.Sprintf("%s of the calls were retries at %s (%s retries/s)", Pct(retryShare[peak]), Clock(w.Start), Rate(retryShare[peak]**e.RPS)), Source: SrcRetries},
		)
		if e.ErrorRate != nil {
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s of the calls to %s failed at %s", Pct(*e.ErrorRate), k.target, Clock(w.Start)), Source: SrcCallErrs})
		}
		if e.P99 != nil && bp99 != nil {
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("p99 of a call to %s %s at %s, against %s at low load", k.target, Ms(*e.P99), Clock(w.Start), Ms(*bp99)), Source: SrcCallP99})
		}
		for _, ev := range a.Timeline {
			if ev.Kind == "fault_start" && ev.At <= ws[from].Start+WindowSeconds && strings.Contains(ev.Detail, k.target) {
				d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("it started with the fault at %s: %s", Clock(ev.At), ev.Detail), Source: "events.jsonl"})
				break
			}
		}
		d.Next = fmt.Sprintf("Retry less when %s is failing: exponential backoff with jitter, a retry budget (retries as a share of calls), or a circuit breaker that stops calling it for a while. Today %s retries a failed call up to %d times, %s.", k.target, k.svc, r.Max, backoffText(r.Backoff, r.BaseMs, r.Jitter))
		if math.IsNaN(*perReq[peak]) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// backoffText says how a service waits between retries.
func backoffText(kind string, baseMs int, jitter bool) string {
	var t string
	switch kind {
	case "constant":
		t = fmt.Sprintf("waiting %d ms between tries", baseMs)
	case "exponential":
		t = fmt.Sprintf("waiting from %d ms, doubling each time", baseMs)
	default:
		return "with no wait between tries"
	}
	if jitter {
		t += ", with jitter"
	}
	return t
}
