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

// autoscale runs each autoscaled service's control loop for the load.
// Every interval it reads the metric over the replicas taking traffic and
// wants ceil(ready x value / target) of them, as Kubernetes' HPA does with
// its ready pods. More than it has asked for: it asks for the difference,
// and those start in the background while it goes on deciding. Fewer: it
// scales down once nothing is still starting. Each direction waits its
// cooldown after the last change.
func autoscale(ctx context.Context, t0 time.Time, p *plan.Plan, drv driver.Driver, dir *rundir.Dir, out io.Writer) {
	var wg sync.WaitGroup
	for _, s := range p.Services {
		if s.Autoscale == nil {
			continue
		}
		took := "took traffic"
		if s.Role == "worker" {
			took = "started taking jobs"
		}
		wg.Go(func() { autoscaleOne(ctx, t0, s.Name, took, s.Autoscale, drv, dir, out) })
	}
	wg.Wait()
}

func autoscaleOne(ctx context.Context, t0 time.Time, service, took string, a *plan.Autoscale, drv driver.Driver, dir *rundir.Dir, out io.Writer) {
	interval := time.Duration(a.IntervalS) * time.Second
	var mu sync.Mutex
	asked, ready := a.Start, a.Start // replicas asked for, and taking traffic
	scalingDown := false
	var last time.Time // the last change
	var bg sync.WaitGroup
	defer bg.Wait()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		v, ok, err := drv.Metric(ctx, service, a.Metric, interval)
		if err != nil || !ok {
			continue
		}
		mu.Lock()
		cur, have, down := asked, ready, scalingDown
		mu.Unlock()
		if a.Metric == "queue_depth" {
			v /= float64(max(1, have)) // the queue's jobs, per replica taking them
		}
		want := desiredReplicas(have, v, a)
		since := time.Since(last)
		switch {
		case want > cur && (last.IsZero() || since >= time.Duration(a.UpCooldownS)*time.Second):
		case want < cur && have == cur && !down && (last.IsZero() || since >= time.Duration(a.DownCooldownS)*time.Second):
		default:
			continue
		}
		_ = dir.Event("autoscale", map[string]any{"service": service, "at_s": time.Since(t0).Seconds(), "from": cur, "to": want, "ready": have, "metric": a.Metric, "value": v, "target": a.Target})
		fmt.Fprintf(out, "[%s] autoscale: %s %d → %d replicas (%s %s over %d running, target %s)\n", clock(time.Since(t0)), service, cur, want, a.Metric, metricText(a.Metric, v), have, metricText(a.Metric, a.Target))
		last = time.Now()
		mu.Lock()
		asked = want
		if want < cur {
			scalingDown = true
		}
		mu.Unlock()
		onReady := func(c string, after time.Duration) {
			mu.Lock()
			ready++
			mu.Unlock()
			fmt.Fprintf(out, "[%s] autoscale: %s %s %s after the decision\n", clock(time.Since(t0)), c, took, after.Round(100*time.Millisecond))
		}
		bg.Go(func() {
			err := drv.Scale(ctx, service, want, onReady)
			mu.Lock()
			if want < cur {
				scalingDown = false
				if err == nil {
					ready = want
				}
			}
			if err != nil {
				asked = cur
			}
			mu.Unlock()
			if err != nil && ctx.Err() == nil {
				_ = dir.Event("autoscale_error", map[string]any{"service": service, "from": cur, "to": want, "error": err.Error()})
				fmt.Fprintf(out, "autoscale: %s %d → %d failed: %v\n", service, cur, want, err)
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
	case "queue_depth":
		return fmt.Sprintf("%.0f jobs a replica", v)
	}
	return fmt.Sprintf("%.1f in flight", v)
}
