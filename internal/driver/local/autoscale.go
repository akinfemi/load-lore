package local

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/akinfemi/load-lore/internal/plan"
)

// An autoscaled service's replicas all exist in the compose file; the ones
// it doesn't start with are in the scale profile, and the lb in front
// lists them disabled. Scaling up starts containers by name and, once
// their health checks pass, enables them on the lb. Scaling down drains
// them on the lb, waits for their requests to finish, then stops them.

// drainWait is the longest a leaving replica gets to finish its requests.
const drainWait = 30 * time.Second

// Metric is an autoscaled service's metric over its running replicas,
// averaged over the last window: cpu as a fraction of the limit, p99 in
// ms, inflight as requests per replica. ok is false while there's no data.
func (d *Driver) Metric(ctx context.Context, service, metric string, window time.Duration) (float64, bool, error) {
	if d.prom == nil {
		return 0, false, errors.New("the stack isn't up")
	}
	w := fmt.Sprintf("%ds", max(5, int(window.Seconds())))
	var q string
	switch metric {
	case "cpu":
		q = fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{service=%q,role="app"}[%s])) / sum(container_spec_cpu_quota{service=%q,role="app"} / container_spec_cpu_period{service=%q,role="app"})`, service, w, service, service)
	case "p99":
		q = fmt.Sprintf(`histogram_quantile(0.99, sum by (le) (rate(loadlore_request_duration_seconds_bucket{service=%q}[%s]))) * 1000`, service, w)
	case "inflight":
		q = fmt.Sprintf(`avg(avg_over_time(loadlore_requests_inflight{service=%q}[%s]))`, service, w)
	case "queue_depth":
		// The whole queue's jobs, waiting or running; the autoscaler divides
		// by the replicas taking them.
		s := d.plan.Service(service)
		if s == nil || s.Consume == nil {
			return 0, false, fmt.Errorf("%s isn't a worker", service)
		}
		q = fmt.Sprintf(`max(avg_over_time(redis_stream_length{service=%q,stream=%q}[%s]))`, s.Consume.Queue, s.Consume.Queue, w)
	default:
		return 0, false, fmt.Errorf("no autoscale metric %q", metric)
	}
	v, ok, err := d.prom.scalar(ctx, q)
	if err != nil || !ok || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false, err
	}
	return v, true, nil
}

// Scale runs service with n replicas. Scaling up starts the next ones in
// order, in the background of the call that wanted them: each waits the
// autoscaler's startup_s, starts, passes its health check and is enabled
// on the lb, then ready is called; Scale returns once they're under way, so
// the autoscaler goes on deciding meanwhile. Scaling down drains and stops
// the last ones, and refuses while replicas are still starting.
func (d *Driver) Scale(ctx context.Context, service string, n int, ready func(container string, took time.Duration)) error {
	s := d.plan.Service(service)
	if s == nil || s.Autoscale == nil {
		return fmt.Errorf("%s isn't autoscaled", service)
	}
	n = min(max(n, 1), len(s.Containers))
	d.scaleMu.Lock()
	if d.running == nil {
		d.running, d.starting = map[string]int{}, map[string]int{}
	}
	cur, ok := d.running[service]
	if !ok {
		cur = s.Autoscale.Start
	}
	switch {
	case n > cur:
		add := s.Containers[cur:n]
		d.running[service] = n
		d.starting[service] += len(add)
		d.scaleMu.Unlock()
		go d.startReplicas(ctx, service, add, time.Duration(s.Autoscale.StartupS)*time.Second, ready)
		return nil
	case n < cur:
		if d.starting[service] > 0 {
			d.scaleMu.Unlock()
			return fmt.Errorf("%d of %s's replicas are still starting", d.starting[service], service)
		}
		leave := s.Containers[n:cur]
		d.running[service] = n
		d.scaleMu.Unlock()
		for _, c := range leave {
			if err := d.lbCommand(ctx, service, "set server replicas/"+c+" state drain"); err != nil {
				return err
			}
		}
		d.awaitDrained(ctx, service, leave)
		for _, c := range leave {
			if err := d.lbCommand(ctx, service, "set server replicas/"+c+" state maint"); err != nil {
				return err
			}
		}
		if _, err := d.compose(ctx, append([]string{"stop", "-t", "10"}, leave...)...); err != nil {
			return fmt.Errorf("stopping %s: %w", strings.Join(leave, ", "), err)
		}
		d.event("replicas_stopped", map[string]any{"service": service, "containers": leave})
		return nil
	}
	d.scaleMu.Unlock()
	return nil
}

// startReplicas brings up containers for an autoscaled service: after the
// startup wait, compose starts them and waits for their health checks, and
// each is enabled on the lb.
func (d *Driver) startReplicas(ctx context.Context, service string, add []string, wait time.Duration, ready func(string, time.Duration)) {
	defer func() {
		d.scaleMu.Lock()
		d.starting[service] -= len(add)
		d.scaleMu.Unlock()
	}()
	start := time.Now()
	// Finding capacity and pulling the image, which this host has already
	// done.
	if wait > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
	fail := func(err error) {
		if ctx.Err() == nil {
			d.event("autoscale_error", map[string]any{"service": service, "containers": add, "error": err.Error()})
		}
	}
	args := append([]string{"--profile", profileScale, "up", "-d", "--no-deps", "--wait", "--wait-timeout", "180"}, add...)
	if _, err := d.compose(ctx, args...); err != nil {
		fail(fmt.Errorf("starting %s: %w", strings.Join(add, ", "), err))
		return
	}
	for _, c := range add {
		if err := d.lbCommand(ctx, service, "enable server replicas/"+c); err != nil {
			fail(err)
			return
		}
		d.event("replica_ready", map[string]any{"service": service, "container": c, "seconds": time.Since(start).Seconds()})
		if ready != nil {
			ready(c, time.Since(start))
		}
	}
}

// awaitDrained waits until the lb has no requests in progress on any of
// the leaving replicas, or drainWait.
func (d *Driver) awaitDrained(ctx context.Context, service string, leave []string) {
	deadline := time.Now().Add(drainWait)
	for time.Now().Before(deadline) {
		busy := false
		for _, lb := range d.lbsFor(service) {
			out, err := d.haproxy(ctx, lb, "show stat")
			if err != nil {
				return
			}
			for c, cur := range serverSessions(out) {
				if slices.Contains(leave, c) && cur > 0 {
					busy = true
				}
			}
		}
		if !busy {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// serverSessions reads each server's current sessions from show stat's CSV.
func serverSessions(stat string) map[string]int {
	out := map[string]int{}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(stat, "# ")))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil || len(rows) == 0 {
		return out
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[h] = i
	}
	sv, ok1 := col["svname"]
	sc, ok2 := col["scur"]
	if !ok1 || !ok2 {
		return out
	}
	for _, row := range rows[1:] {
		if len(row) > max(sv, sc) && row[sv] != "FRONTEND" && row[sv] != "BACKEND" {
			if n, err := strconv.Atoi(row[sc]); err == nil {
				out[row[sv]] += n
			}
		}
	}
	return out
}

// lbsFor lists the lbs in front of service.
func (d *Driver) lbsFor(service string) []string {
	var out []string
	for _, s := range d.plan.Services {
		if s.LB != nil && s.LB.Target == service {
			out = append(out, s.Name)
		}
	}
	return out
}

// lbCommand sends a runtime API command to every lb in front of service.
func (d *Driver) lbCommand(ctx context.Context, service, cmd string) error {
	for _, lb := range d.lbsFor(service) {
		out, err := d.haproxy(ctx, lb, cmd)
		if err != nil {
			return fmt.Errorf("%s: %s: %w", lb, cmd, err)
		}
		if t := strings.TrimSpace(out); t != "" {
			return fmt.Errorf("%s: %s: %s", lb, cmd, t)
		}
	}
	return nil
}

// haproxy sends one command to an lb's runtime API and returns its answer.
func (d *Driver) haproxy(ctx context.Context, lb, cmd string) (string, error) {
	addr, err := d.publishedPort(ctx, lb, plan.LBAdminPort)
	if err != nil {
		return "", err
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := fmt.Fprintf(conn, "%s\n", cmd); err != nil {
		return "", err
	}
	var b strings.Builder
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		b.WriteString(sc.Text())
		b.WriteByte('\n')
	}
	return b.String(), sc.Err()
}
