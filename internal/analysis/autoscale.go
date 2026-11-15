package analysis

import (
	"fmt"
	"strings"
)

// Autoscale lag: the run broke while an autoscaled service's replicas were
// saturated, CPU at AutoscaleSaturated of their limit or more in the
// break's window or the AutoscaleWindows after it (a spike can start
// partway through a window), and its autoscaler hadn't caught up: it was
// still below max (lag), or at max (ceiling).
const (
	AutoscaleSaturated = 0.9
	AutoscaleWindows   = 2
)

func autoscaleLag(a *Analysis) []*Diagnosis {
	var out []*Diagnosis
	b := a.Verdict.Break
	if b == nil || a.Plan == nil {
		return out
	}
	k := int(b.At / WindowSeconds)
	if k < 0 || k >= len(a.Windows) {
		return out
	}
	for _, s := range a.Plan.Services {
		as := s.Autoscale
		if as == nil {
			continue
		}
		// The most saturated window from the break on.
		var w *Window
		var sw *ServiceWindow
		for i := k; i <= min(k+AutoscaleWindows, len(a.Windows)-1); i++ {
			x := a.Windows[i].Services[s.Name]
			if x != nil && x.CPU != nil && (sw == nil || *x.CPU > *sw.CPU) {
				w, sw = a.Windows[i], x
			}
		}
		if sw == nil || *sw.CPU < AutoscaleSaturated {
			continue
		}
		running := sw.Replicas
		if bw := a.Windows[k].Services[s.Name]; bw != nil && bw.Replicas > 0 {
			running = bw.Replicas // as the break began
		}
		var steps, ready []string
		reached, reachedAt := as.Start, -1.0
		for _, e := range a.Timeline {
			switch {
			case e.Kind == "autoscale" && strings.HasPrefix(e.Detail, s.Name+" "):
				steps = append(steps, Clock(e.At)+" "+e.Detail)
				var from, to int
				if _, err := fmt.Sscanf(strings.TrimPrefix(e.Detail, s.Name+" "), "%d → %d", &from, &to); err == nil && to > reached {
					reached, reachedAt = to, e.At
				}
			case e.Kind == "replica_ready" && strings.HasPrefix(e.Detail, s.Name+"-"):
				ready = append(ready, Clock(e.At)+" "+e.Detail)
			}
		}
		d := &Diagnosis{Rule: "autoscale-lag", From: b.At, To: b.At + WindowSeconds, comp: s.Name}
		d.Evidence = append(d.Evidence,
			Evidence{Text: fmt.Sprintf("%s CPU at %s of its limit at %s, over %d %s", s.Name, Pct(*sw.CPU), Clock(w.Start), sw.Replicas, plural(sw.Replicas, "replica", "replicas")), Source: SrcCPU})
		for _, st := range steps {
			d.Evidence = append(d.Evidence, Evidence{Text: "autoscale " + st, Source: "events.jsonl (Loadlore's autoscaler)"})
		}
		for _, r := range ready {
			d.Evidence = append(d.Evidence, Evidence{Text: r, Source: "events.jsonl (Loadlore's autoscaler)"})
		}
		if running >= as.Max {
			d.Title = fmt.Sprintf("Autoscale ceiling (%s at its max)", s.Name)
			d.Summary = fmt.Sprintf("At the break, %s, %s ran its most replicas, %d, with their CPU at %s of the limit: the autoscaler had nothing left to add.", Clock(b.At), s.Name, as.Max, Pct(*sw.CPU))
			d.Next = fmt.Sprintf("Raise %s's max if the budget allows, make each replica cheaper per request, or shed load beyond what %d replicas can serve.", s.Name, as.Max)
			out = append(out, d)
			continue
		}
		d.Title = fmt.Sprintf("Autoscale lag (%s)", s.Name)
		later := ""
		if reached > running {
			later = fmt.Sprintf(" It reached %d at %s.", reached, Clock(reachedAt))
		}
		d.Summary = fmt.Sprintf("At the break, %s, %s ran %d of up to %d replicas, their CPU at %s of the limit: the load grew faster than the autoscaler added capacity.%s",
			Clock(b.At), s.Name, running, as.Max, Pct(*sw.CPU), later)
		d.Next = fmt.Sprintf("Let %s's autoscaler react sooner and in bigger steps: decide more often (interval_s), and keep more replicas running before a spike you expect (min).", s.Name)
		if as.Metric == "cpu" {
			d.Next = fmt.Sprintf("Scale %s on a metric that keeps growing with the backlog, inflight or p99: CPU stops at 100%%, so with a target of %s one decision can add at most %.0f%% more replicas, however far the load has run ahead. Also decide more often (interval_s), and keep more replicas running before a spike you expect (min).", s.Name, Pct(as.Target), (1/as.Target-1)*100)
		}
		out = append(out, d)
	}
	return out
}
