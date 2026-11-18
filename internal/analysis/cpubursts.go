package analysis

import (
	"fmt"
	"slices"
)

// CPU bursts: a stall that no other rule covers, and no fault made, in
// which a component was CPU-throttled for a long time while its average
// CPU stayed under its limit. Docker enforces a CPU limit as a quota per CFS period (100 ms
// unless the design sets resources.cpu_period_ms): work that arrives in a
// burst uses the period's quota early and waits for the next period, up
// to most of 100 ms, and while it waits more piles up. In the reference
// runs of When do you need to scale your database? (17-18 Nov) db, at 0.4
// CPU with 96 connections, was throttled for 5 to 16 s a window during a
// 20 to 30 s stall at about 1,600 rps, at 40% to 60% of its limit, against
// 0 to 0.3 s in other windows; three runs with cpu_period_ms: 10 had no
// such stall.
const (
	BurstThrottledMin = 3.0 // seconds throttled in a window, summed over threads
	BurstCPUMax       = 0.9 // the component's mean CPU stayed under this share of its limit
)

// cpuBursts names stalls the other rules left unexplained, when a
// component was throttled in bursts during them.
func cpuBursts(a *Analysis, have []*Diagnosis) []*Diagnosis {
	var out []*Diagnosis
	for _, e := range a.Timeline {
		if e.Kind != "stall" || slices.ContainsFunc(have, func(d *Diagnosis) bool { return d.From < e.To && e.At <= d.To }) {
			continue
		}
		// A fault in the stall made it: the design's own story.
		if slices.ContainsFunc(a.Timeline, func(f Event) bool {
			return (f.Kind == "fault_start" || f.Kind == "fault_end") && f.At >= e.At-WindowSeconds && f.At < e.To
		}) {
			continue
		}
		best, worst, cpu, at := "", 0.0, 0.0, 0.0
		for _, w := range a.Windows {
			if w.Start < e.At || w.Start >= e.To {
				continue
			}
			for name, sw := range w.Services {
				if sw.Throttled == nil || sw.CPU == nil || *sw.Throttled < BurstThrottledMin || *sw.CPU >= BurstCPUMax || *sw.Throttled <= worst {
					continue
				}
				best, worst, cpu, at = name, *sw.Throttled, *sw.CPU, w.Start
			}
		}
		s := a.Plan.Service(best)
		if best == "" || s == nil {
			continue
		}
		period := s.CPUPeriod
		if period == 0 {
			period = 100
		}
		d := &Diagnosis{
			Rule: "cpu-bursts", Title: fmt.Sprintf("CPU bursts at the limit (%s)", best), comp: best,
			From: e.At, To: e.To,
			Summary: fmt.Sprintf("At %s, %s was CPU-throttled for %.1f s (summed over its threads) in 10 seconds while its average CPU was %s of its %g-CPU limit. Docker enforces the limit as %g ms of CPU in every %d ms period: work that came in a burst used up a period's share early and waited for the next one, and requests piled up behind it until the burst cleared.",
				Clock(at), best, worst, Pct(cpu), s.CPU, s.CPU*float64(period), period),
			Next: fmt.Sprintf("This is how a container's CPU limit behaves (Kubernetes throttles the same way); a VM with a small CPU slows down evenly instead. A bigger CPU limit leaves each period more room; resources.cpu_period_ms on %s shortens the period, which spreads the wait but runs the CPU closer to its limit.", best),
			Evidence: []Evidence{
				{Text: fmt.Sprintf("%s throttled %.1f s in the window from %s, at %s of its CPU limit on average", best, worst, Clock(at), Pct(cpu)), Source: SrcThrottled + "; " + SrcCPU},
				{Text: fmt.Sprintf("%s: %s", Clock(e.At), e.Detail), Source: SrcK6Latency},
			},
		}
		out = append(out, d)
	}
	return out
}
