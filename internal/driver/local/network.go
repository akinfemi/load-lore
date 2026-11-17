package local

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/netem"
	"github.com/moby/moby/client"
)

var _ driver.NetworkReporter = (*Driver)(nil)

// shapedRoles are the containers the network profile applies to: the
// stack and the load generator. Loadlore's own tooling isn't slowed, and
// Toxiproxy gets none so that a proxied connection has the same round
// trip as a direct one: the caller's and the target's halves.
var shapedRoles = map[string]bool{"app": true, "datastore": true, "pgbouncer": true, "lb": true, "load": true}

// netemParallel is how many netem helpers run at once.
const netemParallel = 6

// applyNetwork gives every running container of the stack the network
// profile's delay. Containers that start later (k6, an autoscaler's
// replicas, a restarted one, whose network namespace is new) get it as
// their start event arrives: see onStart.
//
// netem's delays don't come out the same on every host: on a Colima VM on
// Apple silicon they ran about 1.3x what was asked, idle or busy, over one
// connection or a hundred. So it first measures the round trip between two
// of the stack's containers without the delay and with it, scales the
// delay by what it found, and measures again once every container has it.
func (d *Driver) applyNetwork(ctx context.Context) error {
	us := d.plan.Network.DelayUs()
	res := &driver.NetworkResult{Profile: d.plan.Network.Profile, RTTMs: d.plan.Network.RTTMs, DelayUs: us}
	defer func() {
		d.netMu.Lock()
		d.net, d.netReady = res, true
		d.netMu.Unlock()
	}()
	if us == 0 {
		return nil
	}
	start := time.Now()
	list, err := d.api.ContainerList(ctx, client.ContainerListOptions{Filters: client.Filters{}.Add("label", LabelRun+"="+d.runID)})
	if err != nil {
		return err
	}
	type ctr struct{ id, name string }
	var cs []ctr
	for _, c := range list.Items {
		if shapedRoles[c.Labels[LabelRole]] && len(c.Names) > 0 {
			cs = append(cs, ctr{c.ID, strings.TrimPrefix(c.Names[0], "/")})
		}
	}
	slices.SortFunc(cs, func(a, b ctr) int { return strings.Compare(a.name, b.name) })

	var probe func() (time.Duration, error)
	var bridge time.Duration
	if len(cs) < 2 {
		res.Uncalibrated = "fewer than two containers to measure between"
	} else {
		var stop func()
		probe, stop, err = d.rttProbe(ctx, cs[0].id, cs[1].id)
		if err == nil {
			defer stop()
			res.DelayUs, bridge, err = d.calibrate(ctx, probe, cs[0].id, cs[1].id, us)
		}
		if err != nil {
			probe, res.DelayUs, res.Uncalibrated = nil, us, err.Error()
		}
		if res.DelayUs != us {
			res.Factor = float64(us) / float64(res.DelayUs)
		}
	}

	sem := make(chan struct{}, netemParallel)
	errs := make([]error, len(cs))
	var wg sync.WaitGroup
	for i, c := range cs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			errs[i] = d.netemBase(ctx, c.id, res.DelayUs)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("network profile %s: %w", d.plan.Network.Profile, err)
	}
	if probe != nil {
		if r, err := probe(); err == nil {
			res.MeasuredMs = round3(float64(r-bridge) / 1e6)
			res.BridgeMs = round3(float64(bridge) / 1e6)
		} else {
			res.Uncalibrated = "measuring after: " + err.Error()
		}
	}
	d.event("network_profile", map[string]any{"profile": res.Profile, "rtt_ms": res.RTTMs, "delay_us": res.DelayUs, "factor": res.Factor,
		"measured_rtt_ms": res.MeasuredMs, "bridge_rtt_ms": res.BridgeMs, "uncalibrated": res.Uncalibrated, "containers": len(cs), "seconds": time.Since(start).Seconds()})
	return nil
}

// calibrate measures the round trip between a and b with no delay, then
// with us on each side, and returns the delay that gets the profile's
// round trip on this host, and the bridge's own round trip.
func (d *Driver) calibrate(ctx context.Context, probe func() (time.Duration, error), a, b string, us int) (int, time.Duration, error) {
	r0, err := probe()
	if err != nil {
		return 0, 0, err
	}
	for _, id := range []string{a, b} {
		if err := d.netemBase(ctx, id, us); err != nil {
			return 0, 0, err
		}
	}
	r1, err := probe()
	if err != nil {
		return 0, 0, err
	}
	factor := float64(r1-r0) / float64(2*time.Duration(us)*time.Microsecond)
	switch {
	case factor < calibrateMin || factor > calibrateMax:
		return 0, 0, fmt.Errorf("asked %d µs a side, the round trip went from %v to %v: a factor of %.2f, outside %g to %g", us, r0, r1, factor, calibrateMin, calibrateMax)
	case math.Abs(factor-1) < calibrateSlack:
		return us, r0, nil
	}
	return max(1, int(math.Round(float64(us)/factor))), r0, nil
}

// The delay is corrected when netem's ran off by more than calibrateSlack,
// and the measurement is trusted between calibrateMin and calibrateMax.
const (
	calibrateSlack = 0.05
	calibrateMin   = 0.5
	calibrateMax   = 3.0
	probePings     = 500
)

// rttProbe starts an echo in b's network namespace and returns a function
// that measures the median round trip to it from a's, and one that stops
// the echo.
func (d *Driver) rttProbe(ctx context.Context, a, b string) (func() (time.Duration, error), func(), error) {
	ip, err := d.ipOf(ctx, b)
	if err != nil {
		return nil, nil, err
	}
	out, err := d.docker(ctx, "run", "-d", "--rm", "--network", "container:"+b, "--label", LabelOwner+"=1", "--label", LabelRun+"="+d.runID,
		d.plan.Infra.SyntheticImg, "netprobe", "echo")
	if err != nil {
		return nil, nil, err
	}
	echo := strings.TrimSpace(string(out))
	stop := func() { _, _ = d.docker(context.Background(), "rm", "-f", echo) }
	addr := fmt.Sprintf("%s:%d", ip, netem.ProbePort)
	probe := func() (time.Duration, error) {
		out, err := d.docker(ctx, "run", "--rm", "--network", "container:"+a, "--label", LabelOwner+"=1", "--label", LabelRun+"="+d.runID,
			d.plan.Infra.SyntheticImg, "netprobe", "rtt", addr, strconv.Itoa(probePings))
		if err != nil {
			return 0, err
		}
		v, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("netprobe printed %q", out)
		}
		return time.Duration(v) * time.Microsecond, nil
	}
	return probe, stop, nil
}

// NetworkApplied says how Up applied the network profile.
func (d *Driver) NetworkApplied() *driver.NetworkResult {
	d.netMu.Lock()
	defer d.netMu.Unlock()
	return d.net
}

// delayUs is the delay each container's packets get: the profile's, as
// calibrated for this host.
func (d *Driver) delayUs() int {
	d.netMu.Lock()
	defer d.netMu.Unlock()
	if d.net != nil {
		return d.net.DelayUs
	}
	return d.plan.Network.DelayUs()
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// onStart gives a container that started after the stack came up the
// network profile, in the background: until it lands, about a second,
// the container's packets go out without the delay.
func (d *Driver) onStart(id, role, name string) {
	d.netMu.Lock()
	ready := d.netReady
	d.netMu.Unlock()
	if !ready || !shapedRoles[role] || d.delayUs() == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		f := map[string]any{"container": name, "role": role, "profile": d.plan.Network.Profile}
		if err := d.netemBase(ctx, id, d.delayUs()); err != nil {
			f["error"] = err.Error()
		}
		d.event("network_profile", f)
	}()
}

// netemBase sets the container's packets to a delay of us alone, ending
// any loss or partition there.
func (d *Driver) netemBase(ctx context.Context, id string, us int) error {
	return d.netem(ctx, id, "base", strconv.Itoa(us))
}

// ipOf is a container's IPv4 address on the stack's network.
func (d *Driver) ipOf(ctx context.Context, id string) (string, error) {
	in, err := d.api.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return "", err
	}
	if ns := in.Container.NetworkSettings; ns != nil {
		for _, ep := range ns.Networks {
			if ep != nil && ep.IPAddress.IsValid() && ep.IPAddress.Is4() {
				return ep.IPAddress.String(), nil
			}
		}
	}
	return "", fmt.Errorf("container %s has no IPv4 address", id)
}

// containerIPs are the stack network addresses of a service's containers.
func (d *Driver) containerIPs(ctx context.Context, service string) ([]string, error) {
	ids, err := d.containerIDs(ctx, service)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, id := range ids {
		ip, err := d.ipOf(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, ip)
	}
	return out, nil
}

func joinOrDash(xs []string) string {
	if len(xs) == 0 {
		return "-"
	}
	return strings.Join(xs, ",")
}
