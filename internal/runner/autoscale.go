package runner

import (
	"context"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/rundir"
)

// autoscaleTolerance: a metric within 10% of its target changes nothing,
// as in Kubernetes' HorizontalPodAutoscaler.
const autoscaleTolerance = 0.1

// desiredReplicas is how many replicas an autoscaler wants: running x
// value / target, rounded up, within min and max, or running when the
// ratio is within the tolerance.
func desiredReplicas(running int, value float64, a *plan.Autoscale) int {
	ratio := value / a.Target
	if math.Abs(ratio-1) <= autoscaleTolerance {
		return running
	}
	return min(max(int(math.Ceil(float64(running)*ratio)), a.Min), a.Max)
}

// autoscale runs each autoscaled service's control loop for the load:
// every interval it reads the metric over the running replicas and scales
// toward the desired count, waiting each direction's cooldown after a
// change. A scale-up runs in the background, so the loop keeps reading;
// the replicas it starts count as running from the decision, as a
// Deployment's or an autoscaling group's desired count does.
func autoscale(ctx context.Context, t0 time.Time, p *plan.Plan, drv driver.Driver, dir *rundir.Dir, out io.Writer) {
	var wg sync.WaitGroup
	for _, s := range p.Services {
		if s.Autoscale == nil {
			continue
		}
		wg.Go(func() { autoscaleOne(ctx, t0, s.Name, s.Autoscale, drv, dir, out) })
	}
	wg.Wait()
}

func autoscaleOne(ctx context.Context, t0 time.Time, service string, a *plan.Autoscale, drv driver.Driver, dir *rundir.Dir, out io.Writer) {
	interval := time.Duration(a.IntervalS) * time.Second
	running := a.Start
	var last time.Time // the last change
	var scaling sync.WaitGroup
	busy := false
	var mu sync.Mutex
	tick := time.NewTicker(interval)
	defer tick.Stop()
	defer scaling.Wait()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		mu.Lock()
		inProgress := busy
		mu.Unlock()
		if inProgress {
			continue
		}
		v, ok, err := drv.Metric(ctx, service, a.Metric, interval)
		if err != nil || !ok {
			continue
		}
		want := desiredReplicas(running, v, a)
		since := time.Since(last)
		switch {
		case want > running && (last.IsZero() || since >= time.Duration(a.UpCooldownS)*time.Second):
		case want < running && (last.IsZero() || since >= time.Duration(a.DownCooldownS)*time.Second):
		default:
			continue
		}
		at := time.Since(t0).Seconds()
		_ = dir.Event("autoscale", map[string]any{"service": service, "at_s": at, "from": running, "to": want, "metric": a.Metric, "value": v, "target": a.Target})
		fmt.Fprintf(out, "[%s] autoscale: %s %d → %d replicas (%s %s, target %s)\n", clock(time.Since(t0)), service, running, want, a.Metric, metricText(a.Metric, v), metricText(a.Metric, a.Target))
		from := running
		running, last = want, time.Now()
		mu.Lock()
		busy = true
		mu.Unlock()
		scaling.Go(func() {
			defer func() { mu.Lock(); busy = false; mu.Unlock() }()
			err := drv.Scale(ctx, service, want, func(c string, took time.Duration) {
				fmt.Fprintf(out, "[%s] autoscale: %s took traffic %s after the decision\n", clock(time.Since(t0)), c, took.Round(100*time.Millisecond))
			})
			if err != nil && ctx.Err() == nil {
				_ = dir.Event("autoscale_error", map[string]any{"service": service, "from": from, "to": want, "error": err.Error()})
				fmt.Fprintf(out, "autoscale: %s %d → %d failed: %v\n", service, from, want, err)
			}
		})
	}
}

func metricText(metric string, v float64) string {
	switch metric {
	case "cpu":
		return fmt.Sprintf("%.0f%%", v*100)
	case "p99":
		return fmt.Sprintf("%.0f ms", v)
	}
	return fmt.Sprintf("%.1f in flight", v)
}
