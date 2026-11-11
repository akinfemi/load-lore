// Package local runs a plan on the local Docker engine: it generates a
// Docker Compose file per run and drives it with `docker compose`, and uses
// the Docker Engine API for container state, faults and events.
package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/hoststat"
	"github.com/akinfemi/load-lore/internal/load"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/rundir"
)

// Driver is the local Docker driver. One Driver runs one plan.
type Driver struct {
	api  *client.Client
	host string

	plan        *plan.Plan
	dir         *rundir.Dir
	runID       string
	project     string
	composePath string

	tox  *toxiproxyClient
	prom *promClient

	stopEvents func()
	eventsDone chan struct{}
	downOnce   sync.Once
	downErr    error
}

var _ driver.Driver = (*Driver)(nil)

// New connects to Docker.
func New(ctx context.Context) (*Driver, error) {
	api, host, err := NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &Driver{api: api, host: host}, nil
}

func (d *Driver) event(kind string, fields map[string]any) {
	if d.dir != nil {
		_ = d.dir.Event(kind, fields)
	}
}

func (d *Driver) bind(p *plan.Plan) {
	d.plan = p
	d.runID = p.RunID
	d.project = "loadlore-" + p.RunID
}

// --- Preflight ---

// minComposeMinor is the first Compose v2 release with inline config content.
const minComposeMinor = 23

// hostInfo reads Docker's view of the host and checks Compose. It fails
// only when Docker can't be reached; a missing or old Compose comes back
// as composeErr.
func (d *Driver) hostInfo(ctx context.Context) (h driver.Host, composeErr, err error) {
	info, err := d.api.Info(ctx, client.InfoOptions{})
	if err != nil {
		return h, nil, fmt.Errorf("docker isn't reachable at %q: %w", d.host, err)
	}
	h = driver.Host{
		Driver: "local", CPUs: info.Info.NCPU, MemoryBytes: info.Info.MemTotal, OS: info.Info.OperatingSystem,
		Kernel: info.Info.KernelVersion, Arch: info.Info.Architecture, EngineVersion: info.Info.ServerVersion,
		CgroupVersion: info.Info.CgroupVersion,
	}
	switch {
	case hoststat.SameKernel(info.Info.KernelVersion, info.Info.Name):
		h.HostCPU = driver.HostCPUSampled
	case localSocket(d.host) && (runtime.GOOS != "linux" || strings.Contains(info.Info.OperatingSystem, "Docker Desktop")):
		h.HostCPU = driver.HostCPUInVM
	}
	out, cerr := d.docker(ctx, "compose", "version", "--short")
	if cerr != nil {
		return h, fmt.Errorf("docker compose isn't available: %w", cerr), nil
	}
	h.ComposeVersion = strings.TrimSpace(string(out))
	if !composeNewEnough(h.ComposeVersion) {
		return h, fmt.Errorf("docker compose %s is too old: need 2.%d or later", h.ComposeVersion, minComposeMinor), nil
	}
	return h, nil, nil
}

// Inspect is what loadl doctor checks, without a plan: Docker, Compose,
// the synthetic image and, when the image is there, the host speed.
func (d *Driver) Inspect(ctx context.Context, image string) (*driver.Inspection, error) {
	h, composeErr, err := d.hostInfo(ctx)
	if err != nil {
		return nil, err
	}
	in := &driver.Inspection{Host: h, DockerHost: d.host, ComposeErr: composeErr, Image: image}
	if _, err := d.api.ImageInspect(ctx, image); err != nil {
		in.ImageErr = fmt.Errorf("image %s isn't available to Docker", image)
		return in, nil
	}
	in.Host.Speed, in.SpeedErr = d.speed(ctx, image)
	return in, nil
}

// Preflight checks Docker, Compose, images and host capacity.
func (d *Driver) Preflight(ctx context.Context, p *plan.Plan) (*driver.Preflight, error) {
	d.bind(p)
	h, composeErr, err := d.hostInfo(ctx)
	if err != nil {
		return nil, err
	}
	if composeErr != nil {
		return nil, composeErr
	}
	pf := &driver.Preflight{Host: h}
	if pf.Host.CgroupVersion != "2" {
		pf.Warnings = append(pf.Warnings, fmt.Sprintf("Docker uses cgroup v%s; Loadlore is tested on cgroup v2, so CPU and memory limits may behave differently", pf.Host.CgroupVersion))
	}
	for _, s := range p.Services {
		if s.Kind == "app" && len(s.Synthetic) > 0 {
			if _, err := d.api.ImageInspect(ctx, p.Infra.SyntheticImg); err != nil {
				return nil, fmt.Errorf("image %s isn't available to Docker: build it with `make image`", p.Infra.SyntheticImg)
			}
			break
		}
	}
	if _, err := d.api.ImageInspect(ctx, p.Infra.SyntheticImg); err == nil {
		sp, err := d.speed(ctx, p.Infra.SyntheticImg)
		if err != nil {
			pf.Warnings = append(pf.Warnings, "the host speed benchmark failed, so the report has no speed index: "+err.Error())
		}
		pf.Host.Speed = sp
	}
	if need, have := p.TotalCPU(), float64(h.CPUs); need > have {
		pf.Warnings = append(pf.Warnings, fmt.Sprintf(
			"the run needs %s CPUs of container limits (Loadlore's own containers included) but Docker has %d; containers will compete for CPU, so host contention would mix into the results",
			strconv.FormatFloat(math.Round(need*100)/100, 'f', -1, 64), h.CPUs))
	}
	if need, have := p.TotalMemory(), h.MemoryBytes; need > have {
		pf.Warnings = append(pf.Warnings, fmt.Sprintf("the run's memory limits add up to %d MiB but Docker has %d MiB; the host may run out of memory before a container reaches its limit", need>>20, have>>20))
	}
	return pf, nil
}

func composeNewEnough(v string) bool {
	parts := strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return major > 2 || (major == 2 && minor >= minComposeMinor)
}

// --- Up ---

// Up seeds data, writes compose.yaml and starts the stack.
func (d *Driver) Up(ctx context.Context, p *plan.Plan, dir *rundir.Dir, progress func(string)) error {
	d.bind(p)
	d.dir = dir
	d.composePath = dir.File("compose.yaml")
	d.watchEvents()

	vols := map[string]string{}
	for _, s := range p.Services {
		if s.Kind == "postgres" {
			v, err := d.prepareData(ctx, s, progress)
			if err != nil {
				return err
			}
			vols[s.Name] = v
		}
	}

	script, err := load.Script(p)
	if err != nil {
		return err
	}
	if err := dir.Write("load.js", script); err != nil {
		return err
	}
	compose, err := buildCompose(p, stackInput{Project: d.project, K6Script: script, DataVols: vols, Synthetic: p.Infra.SyntheticImg})
	if err != nil {
		return err
	}
	if err := dir.Write("compose.yaml", compose); err != nil {
		return err
	}

	progress("pulling missing images")
	if _, err := d.compose(ctx, "--profile", "load", "pull", "--quiet", "--ignore-buildable", "--policy", "missing"); err != nil {
		return err
	}
	progress("starting the stack and waiting for it to be healthy")
	start := time.Now()
	if _, err := d.compose(ctx, "up", "-d", "--wait", "--wait-timeout", "300"); err != nil {
		return err
	}
	d.event("stack_up", map[string]any{"seconds": time.Since(start).Seconds()})

	promAddr, err := d.publishedPort(ctx, svcPrometheus, promPort)
	if err != nil {
		return err
	}
	toxAddr, err := d.publishedPort(ctx, svcToxiproxy, toxiproxyAPI)
	if err != nil {
		return err
	}
	d.prom, d.tox = newPromClient(promAddr), newToxiproxyClient(toxAddr)
	if err := d.tox.ping(ctx); err != nil {
		return fmt.Errorf("toxiproxy API: %w", err)
	}
	return nil
}

func (d *Driver) publishedPort(ctx context.Context, service string, port int) (string, error) {
	out, err := d.compose(ctx, "port", service, strconv.Itoa(port))
	if err != nil {
		return "", err
	}
	addr := strings.TrimSpace(string(out))
	if addr == "" || strings.HasSuffix(addr, ":0") {
		return "", fmt.Errorf("%s has no published port for %d", service, port)
	}
	return strings.Replace(addr, "0.0.0.0", "127.0.0.1", 1), nil
}

// watchEvents records container lifecycle events (deaths, OOM kills,
// restarts) for this run in events.jsonl.
func (d *Driver) watchEvents() {
	ctx, cancel := context.WithCancel(context.Background())
	d.stopEvents = cancel
	d.eventsDone = make(chan struct{})
	res := d.api.Events(ctx, client.EventsListOptions{
		Filters: client.Filters{}.Add("label", LabelRun+"="+d.runID).Add("type", "container"),
	})
	go func() {
		defer close(d.eventsDone)
		for {
			select {
			case m := <-res.Messages:
				action := string(m.Action)
				switch {
				case action == "start", action == "die", action == "kill", action == "oom", action == "restart",
					action == "pause", action == "unpause", strings.HasPrefix(action, "health_status: unhealthy"):
					f := map[string]any{"action": action, "container": m.Actor.Attributes["name"], "service": m.Actor.Attributes[LabelService]}
					if code, ok := m.Actor.Attributes["exitCode"]; ok {
						f["exit_code"] = code
					}
					d.event("docker", f)
				}
			case err := <-res.Err:
				if err != nil && ctx.Err() == nil {
					d.event("docker_events_error", map[string]any{"error": err.Error()})
				}
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// --- Load ---

// RunLoad starts k6 and waits for it.
func (d *Driver) RunLoad(ctx context.Context, started func(time.Time)) (*driver.LoadResult, error) {
	if _, err := d.compose(ctx, "--profile", "load", "up", "-d", "--no-deps", svcLoad); err != nil {
		return nil, err
	}
	started(time.Now())
	ids, err := d.containerIDs(ctx, svcLoad)
	if err != nil {
		return nil, err
	}
	id := ids[0]
	wait := d.api.ContainerWait(ctx, id, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	res := &driver.LoadResult{}
	select {
	case r := <-wait.Result:
		res.ExitCode = int(r.StatusCode)
	case err := <-wait.Error:
		if ctx.Err() == nil {
			return nil, fmt.Errorf("waiting for the load generator: %w", err)
		}
	case <-ctx.Done():
	}
	if ctx.Err() != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, _ = d.api.ContainerKill(stopCtx, id, client.ContainerKillOptions{Signal: "SIGINT"})
		return nil, ctx.Err()
	}
	summary, err := d.readFileFromContainer(ctx, id, load.SummaryPath)
	if err != nil {
		return res, fmt.Errorf("k6 exited with %d and wrote no summary: %w", res.ExitCode, err)
	}
	if err := d.dir.Write("k6-summary.json", summary); err != nil {
		return res, err
	}
	parseSummary(summary, res)
	return res, nil
}

// StopLoad sends k6 SIGINT, on which it stops starting iterations, waits
// for the running ones and writes its summary.
func (d *Driver) StopLoad(ctx context.Context) error {
	ids, err := d.containerIDs(ctx, svcLoad)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		_, err := d.api.ContainerKill(ctx, id, client.ContainerKillOptions{Signal: "SIGINT"})
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func parseSummary(b []byte, res *driver.LoadResult) {
	var s struct {
		Metrics map[string]struct {
			Values map[string]float64 `json:"values"`
		} `json:"metrics"`
	}
	if json.Unmarshal(b, &s) != nil {
		return
	}
	res.Requests = s.Metrics["http_reqs"].Values["count"]
	res.DroppedIterations = s.Metrics["dropped_iterations"].Values["count"]
	res.FailedShare = s.Metrics["http_req_failed"].Values["rate"]
}

// --- Faults ---

// Inject applies a fault through Toxiproxy or the Docker API.
func (d *Driver) Inject(ctx context.Context, f *plan.Fault) (func(context.Context) error, error) {
	switch f.Inject {
	case "latency", "bandwidth", "timeout", "reset":
		x := toxic{Name: fmt.Sprintf("fault-%d", f.Index), Stream: "downstream", Toxicity: 1, Attributes: map[string]int{}}
		switch f.Inject {
		case "latency":
			x.Type = "latency"
			x.Attributes["latency"], x.Attributes["jitter"] = f.AddMs, f.JitterMs
		case "bandwidth":
			x.Type = "bandwidth"
			x.Attributes["rate"] = max(1, f.Kbps/8) // Toxiproxy takes KB/s
		case "timeout":
			x.Type = "timeout"
			x.Attributes["timeout"] = 0 // hold data until removed
		case "reset":
			x.Type = "reset_peer"
			x.Attributes["timeout"] = 0
		}
		var applied []string
		for _, proxy := range f.Proxies {
			if err := d.tox.addToxic(ctx, proxy, x); err != nil {
				return d.removeToxics(applied, x.Name), err
			}
			applied = append(applied, proxy)
		}
		return d.removeToxics(applied, x.Name), nil
	case "kill", "restart", "pause":
		var ids []string
		for _, c := range f.Containers {
			found, err := d.containerIDs(ctx, c)
			if err != nil {
				return nil, err
			}
			ids = append(ids, found...)
		}
		var errs []error
		for _, id := range ids {
			var err error
			switch f.Inject {
			case "kill":
				_, err = d.api.ContainerKill(ctx, id, client.ContainerKillOptions{Signal: "SIGKILL"})
			case "restart":
				t := 10
				_, err = d.api.ContainerRestart(ctx, id, client.ContainerRestartOptions{Timeout: &t})
			case "pause":
				_, err = d.api.ContainerPause(ctx, id, client.ContainerPauseOptions{})
			}
			errs = append(errs, err)
		}
		if f.Inject != "pause" {
			return nil, errors.Join(errs...)
		}
		return func(ctx context.Context) error {
			var errs []error
			for _, id := range ids {
				_, err := d.api.ContainerUnpause(ctx, id, client.ContainerUnpauseOptions{})
				errs = append(errs, err)
			}
			return errors.Join(errs...)
		}, errors.Join(errs...)
	}
	return nil, fmt.Errorf("%s isn't supported by local runs yet", f.Inject)
}

func (d *Driver) removeToxics(proxies []string, name string) func(context.Context) error {
	return func(ctx context.Context) error {
		var errs []error
		for _, p := range proxies {
			errs = append(errs, d.tox.removeToxic(ctx, p, name))
		}
		return errors.Join(errs...)
	}
}

// --- Progress ---

// Sample reads current numbers from Prometheus for the progress line. Served,
// errors and p99 are end to end, from the load generator.
func (d *Driver) Sample(ctx context.Context, elapsed time.Duration) (*driver.Sample, error) {
	s := &driver.Sample{Offered: d.plan.Workload.OfferedRate(elapsed)}
	if d.prom == nil {
		return s, nil
	}
	if v, ok, err := d.prom.scalar(ctx, `sum(rate(k6_http_reqs_total[10s]))`); err == nil && ok {
		if good, ok, err := d.prom.scalar(ctx, `sum(rate(k6_http_reqs_total{expected_response="true"}[10s])) or vector(0)`); err == nil && ok {
			s.Served = &good
			if v > 0 {
				share := max(0, (v-good)/v)
				s.Errors = &share
			}
		}
	}
	if v, ok, err := d.prom.scalar(ctx, `histogram_quantile(0.99, sum(rate(k6_http_req_duration_seconds[10s]))) * 1000`); err == nil && ok && !math.IsNaN(v) {
		s.P99Ms = &v
	}
	cpu, err := d.prom.vector(ctx, `sum by (service) (rate(container_cpu_usage_seconds_total{role=~"app|datastore|pgbouncer"}[10s])) / sum by (service) (container_spec_cpu_quota{role=~"app|datastore|pgbouncer"} / container_spec_cpu_period{role=~"app|datastore|pgbouncer"})`, "service")
	if err == nil {
		s.CPU = cpu
	}
	d.sampleMore(ctx, s)
	return s, nil
}

// sampleMore adds what the live dashboard shows besides the progress
// line: p50 and p95, memory, pools, Postgres connections and Redis hit
// rates. A query that fails leaves its field empty.
func (d *Driver) sampleMore(ctx context.Context, s *driver.Sample) {
	for q, dst := range map[float64]**float64{0.5: &s.P50Ms, 0.95: &s.P95Ms} {
		query := fmt.Sprintf(`histogram_quantile(%g, sum(rate(k6_http_req_duration_seconds[10s]))) * 1000`, q)
		if v, ok, err := d.prom.scalar(ctx, query); err == nil && ok && !math.IsNaN(v) {
			*dst = &v
		}
	}
	vec := func(query, label string) map[string]float64 {
		v, err := d.prom.vector(ctx, query, label)
		if err != nil || len(v) == 0 {
			return nil
		}
		return v
	}
	const roles = `role=~"app|datastore|pgbouncer|lb"`
	s.Memory = vec(`max by (service) (container_memory_working_set_bytes{`+roles+`} / (container_spec_memory_limit_bytes{`+roles+`} > 0))`, "service")
	edge := func(m string) string {
		return `label_join(sum by (service, target) (` + m + `), "edge", ">", "service", "target")`
	}
	inUse := vec(edge(`loadlore_pool_connections{state="in_use"}`), "edge")
	maxes := vec(edge(`loadlore_pool_max_connections`), "edge")
	pending := vec(edge(`loadlore_pool_pending`), "edge")
	for k, m := range maxes {
		if s.Pools == nil {
			s.Pools = map[string]driver.PoolSample{}
		}
		s.Pools[k] = driver.PoolSample{InUse: inUse[k], Max: m, Pending: pending[k]}
	}
	s.Connections = vec(`sum by (service) (pg_stat_activity_count)`, "service")
	s.MaxConnections = vec(`max by (service) (pg_settings_max_connections)`, "service")
	s.HitRate = vec(`sum by (service) (rate(redis_keyspace_hits_total[10s])) / (sum by (service) (rate(redis_keyspace_hits_total[10s])) + sum by (service) (rate(redis_keyspace_misses_total[10s])) > 0)`, "service")
}

// --- Collect ---

// Collect exports metrics, container state and logs into the run folder.
// With zero times (the stack failed to start) it skips metrics.
func (d *Driver) Collect(ctx context.Context, dir *rundir.Dir, from, to time.Time) error {
	var errs []error
	if d.prom != nil && !from.IsZero() {
		n, err := d.prom.export(ctx, from.Add(-15*time.Second), to.Add(5*time.Second), func(name string, data []byte) error {
			return dir.Write("metrics/"+name, data)
		})
		d.event("metrics_exported", map[string]any{"metrics": n})
		errs = append(errs, err)
	}

	res, err := d.api.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{}.Add("label", LabelRun+"="+d.runID)})
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	type state struct {
		Name       string `json:"name"`
		Service    string `json:"service"`
		Role       string `json:"role"`
		Image      string `json:"image"`
		Status     string `json:"status"`
		ExitCode   int    `json:"exit_code"`
		OOMKilled  bool   `json:"oom_killed"`
		Restarts   int    `json:"restarts"`
		StartedAt  string `json:"started_at"`
		FinishedAt string `json:"finished_at"`
	}
	var states []state
	for _, c := range res.Items {
		in, err := d.api.ContainerInspect(ctx, c.ID, client.ContainerInspectOptions{})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		ci := in.Container
		name := strings.TrimPrefix(ci.Name, "/")
		st := state{Name: name, Service: c.Labels[LabelService], Role: c.Labels[LabelRole], Image: c.Image, Restarts: ci.RestartCount}
		if ci.State != nil {
			st.Status, st.ExitCode, st.OOMKilled = string(ci.State.Status), ci.State.ExitCode, ci.State.OOMKilled
			st.StartedAt, st.FinishedAt = ci.State.StartedAt, ci.State.FinishedAt
		}
		states = append(states, st)
		if logs, err := d.docker(ctx, "logs", "--timestamps", c.ID); err == nil {
			errs = append(errs, dir.Write("logs/"+name+".log", logs))
		}
	}
	errs = append(errs, dir.WriteJSON("containers.json", states))
	return errors.Join(errs...)
}

// --- Down ---

// Down removes the run's stack, volumes and any leftover containers.
func (d *Driver) Down(ctx context.Context) error {
	d.downOnce.Do(func() {
		if d.stopEvents != nil {
			d.stopEvents()
			<-d.eventsDone
		}
		var errs []error
		if d.composePath != "" {
			if _, err := os.Stat(d.composePath); err == nil {
				if _, err := d.compose(ctx, "--profile", "load", "down", "-v", "--remove-orphans", "-t", "5"); err != nil {
					errs = append(errs, err)
				}
			}
		}
		if d.runID != "" {
			errs = append(errs, removeByLabel(ctx, d.api, LabelRun+"="+d.runID, true))
		}
		d.downErr = errors.Join(errs...)
	})
	return d.downErr
}

// removeByLabel force-removes containers, then networks and volumes, that
// carry a label. Volumes marked as caches stay unless withCaches is set.
func removeByLabel(ctx context.Context, api *client.Client, label string, withCaches bool) error {
	var errs []error
	f := client.Filters{}.Add("label", label)
	cs, err := api.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: f})
	errs = append(errs, err)
	for _, c := range cs.Items {
		_, err := api.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
		errs = append(errs, ignoreNotFound(err))
	}
	ns, err := api.NetworkList(ctx, client.NetworkListOptions{Filters: f})
	errs = append(errs, err)
	for _, n := range ns.Items {
		_, err := api.NetworkRemove(ctx, n.ID, client.NetworkRemoveOptions{})
		errs = append(errs, ignoreNotFound(err))
	}
	vs, err := api.VolumeList(ctx, client.VolumeListOptions{Filters: f})
	errs = append(errs, err)
	for _, v := range vs.Items {
		if _, isCache := v.Labels[LabelCache]; isCache && !withCaches {
			continue
		}
		_, err := api.VolumeRemove(ctx, v.Name, client.VolumeRemoveOptions{Force: true})
		errs = append(errs, ignoreNotFound(err))
	}
	return errors.Join(errs...)
}

func ignoreNotFound(err error) error {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no such") {
		return nil
	}
	return err
}

// DownAll removes everything labeled as Loadlore's: every run's containers,
// networks and volumes, and the dataset caches when withCaches is set.
func DownAll(ctx context.Context, withCaches bool) (removed Counts, err error) {
	api, _, err := NewClient(ctx)
	if err != nil {
		return removed, err
	}
	f := client.Filters{}.Add("label", LabelOwner+"=1")
	if cs, err := api.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: f}); err == nil {
		removed.Containers = len(cs.Items)
	}
	if vs, err := api.VolumeList(ctx, client.VolumeListOptions{Filters: f}); err == nil {
		for _, v := range vs.Items {
			if _, isCache := v.Labels[LabelCache]; !isCache || withCaches {
				removed.Volumes++
			}
		}
	}
	if ns, err := api.NetworkList(ctx, client.NetworkListOptions{Filters: f}); err == nil {
		removed.Networks = len(ns.Items)
	}
	return removed, removeByLabel(ctx, api, LabelOwner+"=1", withCaches)
}

// Counts is what DownAll removed.
type Counts struct {
	Containers int `json:"containers"`
	Networks   int `json:"networks"`
	Volumes    int `json:"volumes"`
}

// localSocket reports whether Docker is reached through a socket on this
// machine rather than over the network.
func localSocket(host string) bool {
	return host == "" || strings.HasPrefix(host, "unix://") || strings.HasPrefix(host, "npipe://")
}

// Speed runs the host speed benchmark again.
func (d *Driver) Speed(ctx context.Context, p *plan.Plan) (*bench.Speed, error) {
	return d.speed(ctx, p.Infra.SyntheticImg)
}

// speed runs the host speed benchmark in a container limited to one CPU,
// the way the design's own containers are limited.
func (d *Driver) speed(ctx context.Context, image string) (*bench.Speed, error) {
	out, err := d.docker(ctx, "run", "--rm", "--network", "none", "--cpus", "1", "--memory", "256m",
		"--label", LabelOwner+"=1", image, "bench")
	if err != nil {
		return nil, err
	}
	// Docker may print warnings around the result, which is one JSON line.
	line := out
	if i := bytes.LastIndex(out, []byte("\n{")); i >= 0 {
		line = out[i+1:]
	}
	var r bench.Result
	if err := json.Unmarshal(bytes.TrimSpace(line), &r); err != nil {
		return nil, fmt.Errorf("reading its result: %w", err)
	}
	return &bench.Speed{Score: r.Scored(), Raw: r}, nil
}
