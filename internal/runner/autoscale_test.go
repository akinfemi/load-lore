package runner

import (
	"testing"

	"github.com/akinfemi/load-lore/internal/plan"
)

// The HPA formula: running x value / target, rounded up, within min and
// max; within 10% of the target, no change.
func TestDesiredReplicas(t *testing.T) {
	a := &plan.Autoscale{Min: 2, Max: 6, Metric: "cpu", Target: 0.7}
	for _, tc := range []struct {
		running int
		value   float64
		want    int
	}{
		{2, 0.75, 2}, // within 10%
		{2, 0.95, 3}, // 2 x 0.95/0.7 = 2.7
		{2, 1.0, 3},
		{3, 1.0, 5},  // 4.3
		{4, 1.0, 6},  // 5.7
		{5, 1.0, 6},  // capped at max
		{4, 0.3, 2},  // 1.7, held at min
		{6, 0.35, 3}, // halves
	} {
		if got := desiredReplicas(tc.running, tc.value, a); got != tc.want {
			t.Errorf("%d replicas at %.2f: %d, want %d", tc.running, tc.value, got, tc.want)
		}
	}
}
