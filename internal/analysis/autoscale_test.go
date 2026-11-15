package analysis

import (
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/plan"
)

// A break with api saturated and its autoscaler below max is a lag, with
// the CPU-metric advice; at max, a ceiling; with spare CPU, neither.
func TestAutoscaleLag(t *testing.T) {
	build := func(replicas int, cpu float64) *Analysis {
		a := &Analysis{Plan: &plan.Plan{Services: []*plan.Service{{Name: "api", Kind: "app", Autoscale: &plan.Autoscale{Start: 2, Min: 2, Max: 5, Metric: "cpu", Target: 0.7}}}}}
		for i := range 10 {
			c := 0.5
			if i >= 6 {
				c = cpu
			}
			a.Windows = append(a.Windows, &Window{Index: i, Start: float64(i * WindowSeconds), Services: map[string]*ServiceWindow{"api": {CPU: &c, Replicas: replicas}}})
		}
		a.Verdict.Break = &Break{At: 60}
		a.Timeline = []Event{
			{At: 45, Kind: "autoscale", Detail: "api 2 → 3 replicas (CPU 95% of its limit, target 70%)"},
			{At: 47, Kind: "replica_ready", Detail: "api-2 took traffic, 1.9 s after the decision"},
			{At: 75, Kind: "autoscale", Detail: "api 3 → 5 replicas (CPU 98% of its limit, target 70%)"},
		}
		return a
	}
	d := autoscaleLag(build(3, 0.97))
	if len(d) != 1 || d[0].Rule != "autoscale-lag" || !strings.Contains(d[0].Summary, "api ran 3 of up to 5 replicas") || !strings.Contains(d[0].Summary, "It reached 5 at 1:15") || !strings.Contains(d[0].Next, "at most 43% more replicas") {
		t.Fatalf("lag: %+v", d)
	}
	if d := autoscaleLag(build(5, 0.97)); len(d) != 1 || !strings.Contains(d[0].Title, "ceiling") {
		t.Errorf("ceiling: %+v", d)
	}
	if d := autoscaleLag(build(3, 0.6)); len(d) != 0 {
		t.Errorf("fired with CPU to spare: %+v", d)
	}
}
