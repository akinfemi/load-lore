// Package plan turns a validated design into a run plan: every container to
// start, with resources scaled, every proxied connection, the synthetic
// service configs, the load profile and the fault timeline. A plan is
// independent of where it runs; drivers turn it into infrastructure.
package plan

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/akinfemi/load-lore/internal/dataset"
	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/synthetic"
)

// Fixed names and ports inside a stack.
const (
	AppPort       = 8080
	PostgresPort  = 5432
	RedisPort     = 6379
	ProxyHost     = "lore-toxiproxy"
	ProxyBasePort = 20000
	DBUser        = "loadlore"
	DBPassword    = "loadlore"
	DBName        = "lab"
)

// Plan is everything a driver needs to run one experiment.
type Plan struct {
	RunID    string        `json:"run_id"`
	Name     string        `json:"name"`
	Scale    float64       `json:"scale"`
	Seed     int64         `json:"seed"`
	Duration time.Duration `json:"duration"`

	Services []*Service `json:"services"`
	Proxies  []*Proxy   `json:"proxies"`
	Workload Workload   `json:"workload"`
	Faults   []*Fault   `json:"faults"`
	Infra    Infra      `json:"infra"`

	// Notes explain choices the plan made that a reader of the report
	// should know about, such as values changed by scaling.
	Notes []string `json:"notes"`
}

// Service is one design service, expanded to containers.
type Service struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"` // app, postgres, redis
	Image      string   `json:"image"`
	Containers []string `json:"containers"` // one per replica: api-0, api-1; or db
	CPU        float64  `json:"cpu"`        // per container, after scaling
	Memory     int64    `json:"memory"`     // bytes per container, after scaling
	Port       int      `json:"port"`

	// Application services: one config per replica (synthetic image only).
	Synthetic []*synthetic.Config `json:"synthetic,omitempty"`

	// Datastores.
	Version string            `json:"version,omitempty"`
	Config  map[string]string `json:"config,omitempty"` // after scaling
	Dataset *dataset.Spec     `json:"dataset,omitempty"`
}

// Proxy is one proxied connection: callers of From reach the To container
// through Listen, so faults can act on it.
type Proxy struct {
	Name     string `json:"name"`
	From     string `json:"from"` // service, or "load" for the load generator
	To       string `json:"to"`   // service
	Upstream string `json:"upstream"`
	Listen   int    `json:"listen"`
}

// Addr is the proxy's address inside the stack.
func (p *Proxy) Addr() string { return fmt.Sprintf("%s:%d", ProxyHost, p.Listen) }

// Workload is the load profile, scaled.
type Workload struct {
	Entry        string   `json:"entry"`
	URLs         []string `json:"urls"` // entry replicas, round-robined by the load generator
	StartRate    float64  `json:"start_rate"`
	Stages       []Stage  `json:"stages"`
	PeakRate     float64  `json:"peak_rate"`
	ReadShare    float64  `json:"read_share"`
	Keys         Keys     `json:"keys"`
	PayloadBytes int      `json:"payload_bytes"`
	MaxVUs       int      `json:"max_vus"`
	PreVUs       int      `json:"pre_vus"`
	Timeout      Dur      `json:"timeout"` // per request, in the load generator
}

// Stage moves the arrival rate linearly to Target over Duration. A zero
// duration is an instant jump.
type Stage struct {
	Target   float64 `json:"target"`
	Duration Dur     `json:"duration"`
}

// Keys says how request keys are drawn.
type Keys struct {
	Distribution string  `json:"distribution"`
	Count        int64   `json:"count"`
	Skew         float64 `json:"skew,omitempty"`
	Share        float64 `json:"share,omitempty"`
	Fraction     float64 `json:"fraction,omitempty"`
}

// Fault is one scheduled injection, resolved to containers or proxies.
type Fault struct {
	Index      int      `json:"index"`
	At         Dur      `json:"at"`
	For        *Dur     `json:"for,omitempty"`
	Inject     string   `json:"inject"`
	Target     string   `json:"target"`
	Containers []string `json:"containers,omitempty"` // kill, pause, restart
	Proxies    []string `json:"proxies,omitempty"`    // connection faults
	AddMs      int      `json:"add_ms,omitempty"`
	JitterMs   int      `json:"jitter_ms,omitempty"`
	Kbps       int      `json:"kbps,omitempty"`
}

// Infra is the budget for Loadlore's own containers. They get explicit
// limits so they don't compete unseen with the system under test.
type Infra struct {
	LoadCPU      float64 `json:"load_cpu"`
	LoadMemory   int64   `json:"load_memory"`
	ProxyCPU     float64 `json:"proxy_cpu"`
	ProxyMemory  int64   `json:"proxy_memory"`
	PromCPU      float64 `json:"prometheus_cpu"`
	PromMemory   int64   `json:"prometheus_memory"`
	CAdvisorCPU  float64 `json:"cadvisor_cpu"`
	CAdvisorMem  int64   `json:"cadvisor_memory"`
	ExporterCPU  float64 `json:"exporter_cpu"`
	ExporterMem  int64   `json:"exporter_memory"`
	ExpectedOps  float64 `json:"expected_proxied_ops"` // peak proxied calls per second
	SyntheticImg string  `json:"synthetic_image"`
}

// Dur is a duration that marshals as a string like "5m0s".
type Dur time.Duration

// D returns the time.Duration.
func (d Dur) D() time.Duration { return time.Duration(d) }

// MarshalText implements encoding.TextMarshaler.
func (d Dur) MarshalText() ([]byte, error) { return []byte(time.Duration(d).String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Dur) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	*d = Dur(v)
	return err
}

// Options control how a design becomes a plan.
type Options struct {
	RunID          string
	Scale          float64 // 0 means the design's modes.local.scale
	Seed           int64
	SyntheticImage string
}

// Measured budgets for Loadlore's own containers (see the early checks in
// the spec doc: k6 at 1 CPU held 5,000 rps against a fast target with
// <0.06% dropped, at 2 CPUs none; one Toxiproxy core relays about 15k
// evenly paced queries per second before it saturates).
const (
	k6RatePerCPU     = 2500.0
	proxyOpsPerCPU   = 12000.0
	minLoadCPU       = 1.0
	maxLoadCPU       = 4.0
	minProxyCPU      = 0.5
	maxProxyCPU      = 4.0
	requestTimeout   = 10 * time.Second
	maxVUsCap        = 4000
	minMemoryApp     = 32 << 20
	minMemoryPG      = 128 << 20
	minMemoryRedis   = 32 << 20
	defaultPGImage   = "postgres:%s"
	defaultRedisImg  = "redis:%s"
	pinnedPG17       = "postgres:17.11"
	pinnedRedis8     = "redis:8.2"
	loadGeneratorKey = "load"
)

// Build makes a plan from a design that passed validation.
func Build(d *design.Design, opt Options) (*Plan, error) {
	scale := opt.Scale
	if scale == 0 {
		scale = *d.Modes.Local.Scale
	}
	if scale <= 0 {
		return nil, fmt.Errorf("scale must be greater than 0")
	}
	p := &Plan{RunID: opt.RunID, Name: d.Name, Scale: scale, Seed: opt.Seed, Duration: d.TotalDuration()}
	p.Infra.SyntheticImg = opt.SyntheticImage
	if p.Infra.SyntheticImg == "" {
		p.Infra.SyntheticImg = design.SyntheticImage + ":dev"
	}
	if scale != 1 {
		p.note("scale %s: CPU, memory, memory settings, dataset rows, the keyspace and request rates are multiplied by %s; replicas, pool sizes, connection limits and per-request work are not", fmtF(scale), fmtF(scale))
	}

	names := d.ServiceNames()
	byName := map[string]*Service{}
	for _, name := range names {
		s, err := p.service(name, d.Services[name], scale)
		if err != nil {
			return nil, err
		}
		p.Services = append(p.Services, s)
		byName[name] = s
	}

	p.proxies(d, byName)
	for _, name := range names {
		if s := byName[name]; s.Kind == "app" && d.Services[name].IsSynthetic() {
			p.syntheticConfigs(d, name, s, byName)
		}
	}
	p.workload(d, byName, scale)
	if err := p.faults(d, byName); err != nil {
		return nil, err
	}
	p.infra()
	return p, nil
}

func (p *Plan) note(format string, args ...any) {
	p.Notes = append(p.Notes, fmt.Sprintf(format, args...))
}

func (p *Plan) service(name string, ds *design.Service, scale float64) (*Service, error) {
	s := &Service{Name: name}
	cpu := math.Max(0.05, math.Round(*ds.Resources.CPU*scale*100)/100)
	mem := int64(float64(ds.Resources.Memory.Bytes()) * scale)
	mem = mem >> 20 << 20
	var minMem int64
	switch {
	case ds.IsApp():
		s.Kind, s.Port, minMem = "app", AppPort, minMemoryApp
		s.Image = ds.Image
		if ds.IsSynthetic() {
			s.Image = "" // filled from Infra.SyntheticImg by drivers
		}
		for i := range *ds.Replicas {
			s.Containers = append(s.Containers, fmt.Sprintf("%s-%d", name, i))
		}
	case ds.Kind == "postgres":
		s.Kind, s.Port, minMem = "postgres", PostgresPort, minMemoryPG
		s.Version = ds.Version
		s.Image = fmt.Sprintf(defaultPGImage, ds.Version)
		if ds.Version == "17" {
			s.Image = pinnedPG17
		}
		s.Containers = []string{name}
		spec := dataset.Spec{RowBytes: design.DefaultRowBytes}
		if ds.Dataset != nil {
			spec = dataset.Spec{Rows: ds.Dataset.Rows, RowBytes: *ds.Dataset.RowBytes, Indexes: slices.Clone(ds.Dataset.Indexes)}
		} else {
			p.note("%s has no dataset: its items table is created empty", name)
		}
		if scale != 1 && spec.Rows > 0 {
			spec.Rows = max(1, int64(math.Round(float64(spec.Rows)*scale)))
		}
		s.Dataset = &spec
	case ds.Kind == "redis":
		s.Kind, s.Port, minMem = "redis", RedisPort, minMemoryRedis
		s.Version = ds.Version
		s.Image = fmt.Sprintf(defaultRedisImg, ds.Version)
		if ds.Version == "8" {
			s.Image = pinnedRedis8
		}
		s.Containers = []string{name}
	default:
		return nil, fmt.Errorf("%s: %s isn't supported by local runs yet", name, ds.Kind)
	}
	if mem < minMem {
		p.note("%s: memory raised from %s to the %s minimum for %s", name, sizeStr(mem), sizeStr(minMem), s.Kind)
		mem = minMem
	}
	s.CPU, s.Memory = cpu, mem

	if len(ds.Config) > 0 {
		s.Config = map[string]string{}
		for _, k := range sortedKeys(ds.Config) {
			v := ds.Config[k].String()
			if scale != 1 && memorySetting[k] {
				if scaled, ok := scaleSetting(v, scale); ok {
					p.note("%s: %s scaled from %s to %s", name, k, v, scaled)
					v = scaled
				} else {
					p.note("%s: %s=%s is kept as written; it has no unit to scale", name, k, v)
				}
			}
			s.Config[k] = v
		}
	}
	return s, nil
}

// memorySetting lists native config keys that size memory and so scale.
var memorySetting = map[string]bool{
	"shared_buffers": true, "work_mem": true, "maintenance_work_mem": true,
	"effective_cache_size": true, "wal_buffers": true, "temp_buffers": true,
	"maxmemory": true,
}

// proxies creates one proxy per caller service and target container.
func (p *Plan) proxies(d *design.Design, byName map[string]*Service) {
	port := ProxyBasePort
	add := func(from, to, container string, upPort int) {
		p.Proxies = append(p.Proxies, &Proxy{
			Name: from + "--" + container, From: from, To: to,
			Upstream: fmt.Sprintf("%s:%d", container, upPort), Listen: port,
		})
		port++
	}
	for _, name := range d.ServiceNames() {
		ds := d.Services[name]
		if ds.Behavior == nil {
			continue
		}
		for _, target := range callTargets(ds.Behavior.Calls) {
			t := byName[target]
			for _, c := range t.Containers {
				add(name, target, c, t.Port)
			}
		}
	}
	// The load generator talks to the entry directly, unless a connection
	// fault targets the entry; then it goes through proxies too.
	entry := d.Workload.Entry
	for _, f := range d.Scenario {
		if f.Target != nil && f.Target.Name == entry && connectionFault[f.Inject] {
			for _, c := range byName[entry].Containers {
				add(loadGeneratorKey, entry, c, AppPort)
			}
			break
		}
	}
}

func callTargets(calls []*design.Call) []string {
	var out []string
	var walk func([]*design.Call)
	walk = func(cs []*design.Call) {
		for _, c := range cs {
			if !slices.Contains(out, c.Target) {
				out = append(out, c.Target)
			}
			walk(c.OnMiss)
		}
	}
	walk(calls)
	return out
}

func (p *Plan) proxiesInto(from, to string) []*Proxy {
	var out []*Proxy
	for _, x := range p.Proxies {
		if x.From == from && x.To == to {
			out = append(out, x)
		}
	}
	return out
}

func (p *Plan) syntheticConfigs(d *design.Design, name string, s *Service, byName map[string]*Service) {
	ds := d.Services[name]
	b := ds.Behavior
	targets := map[string]synthetic.Target{}
	for _, target := range callTargets(b.Calls) {
		t := byName[target]
		var addrs []string
		for _, x := range p.proxiesInto(name, target) {
			addrs = append(addrs, x.Addr())
		}
		st := synthetic.Target{Addrs: addrs, Pool: ds.Pools[target]}
		switch t.Kind {
		case "app":
			st.Kind = synthetic.KindHTTP
		case "postgres":
			st.Kind = synthetic.KindPostgres
			st.Database, st.User, st.Password = DBName, DBUser, DBPassword
			st.Rows, st.RowBytes = max(t.Dataset.Rows, 1), t.Dataset.RowBytes
		case "redis":
			st.Kind = synthetic.KindRedis
			st.RowBytes = rowBytesBehind(byName)
		}
		targets[target] = st
	}
	r := b.Resilience
	res := synthetic.Resilience{TimeoutMs: *r.TimeoutMs}
	if x := r.Retries; x != nil {
		res.Retries = &synthetic.Retries{Max: x.Max, Backoff: x.Backoff, Jitter: x.Jitter}
		if x.BaseMs != nil {
			res.Retries.BaseMs = *x.BaseMs
		}
	}
	if x := r.CircuitBreaker; x != nil {
		res.CircuitBreaker = &synthetic.CircuitBreaker{ErrorRate: x.ErrorRate, WindowS: x.WindowS, OpenS: x.OpenS}
	}
	if x := r.Bulkhead; x != nil {
		res.Bulkhead = &synthetic.Bulkhead{MaxConcurrent: x.MaxConcurrent}
	}
	calls := convertCalls(b.Calls)
	for i := range s.Containers {
		s.Synthetic = append(s.Synthetic, &synthetic.Config{
			Service: name, Replica: i, Listen: fmt.Sprintf(":%d", AppPort),
			Concurrency: synthetic.Concurrency{Model: ds.Concurrency.Model, Max: *ds.Concurrency.Max},
			CPUMs:       b.CPUMs, CPUJitter: *b.CPUJitter, AllocKB: b.AllocKB,
			Calls: calls, Resilience: res, Targets: targets,
		})
	}
}

// rowBytesBehind sizes cache values like the rows they cache: the largest
// dataset row among the postgres services in the design.
func rowBytesBehind(byName map[string]*Service) int {
	size := 0
	for _, s := range byName {
		if s.Kind == "postgres" && s.Dataset != nil {
			size = max(size, s.Dataset.RowBytes)
		}
	}
	if size == 0 {
		size = design.DefaultRowBytes
	}
	return size
}

func convertCalls(calls []*design.Call) []synthetic.Call {
	out := make([]synthetic.Call, 0, len(calls))
	for _, c := range calls {
		sc := synthetic.Call{Target: c.Target, Op: c.Op, Count: *c.Count, When: c.When, Mode: c.Mode, Query: c.Query, OnMiss: convertCalls(c.OnMiss)}
		if c.TTLS != nil {
			sc.TTLS = *c.TTLS
		}
		if c.Rows != nil {
			sc.Rows = *c.Rows
		}
		if len(sc.OnMiss) == 0 {
			sc.OnMiss = nil
		}
		out = append(out, sc)
	}
	return out
}

func (p *Plan) workload(d *design.Design, byName map[string]*Service, scale float64) {
	w := d.Workload
	entry := byName[w.Entry]
	pw := Workload{Entry: w.Entry, ReadShare: *w.Mix.Read, PayloadBytes: int(*w.PayloadKB * 1024), Timeout: Dur(requestTimeout)}
	if xs := p.proxiesInto(loadGeneratorKey, w.Entry); len(xs) > 0 {
		for _, x := range xs {
			pw.URLs = append(pw.URLs, "http://"+x.Addr())
		}
	} else {
		for _, c := range entry.Containers {
			pw.URLs = append(pw.URLs, fmt.Sprintf("http://%s:%d", c, AppPort))
		}
	}

	rate := 0.0
	first := true
	for _, st := range w.Profile {
		switch {
		case st.Ramp != nil:
			from, to := st.Ramp.From*scale, st.Ramp.To*scale
			if first {
				pw.StartRate = from
			} else if from != rate {
				pw.Stages = append(pw.Stages, Stage{Target: from})
			}
			pw.Stages = append(pw.Stages, Stage{Target: to, Duration: Dur(st.Ramp.Over.D())})
			rate = to
			pw.PeakRate = max(pw.PeakRate, from, to)
		case st.Hold != nil:
			r := st.Hold.RPS * scale
			if first {
				pw.StartRate = r
			} else if r != rate {
				pw.Stages = append(pw.Stages, Stage{Target: r})
			}
			pw.Stages = append(pw.Stages, Stage{Target: r, Duration: Dur(st.Hold.For.D())})
			rate = r
			pw.PeakRate = max(pw.PeakRate, r)
		case st.Spike != nil:
			r := st.Spike.To * scale
			if first {
				pw.StartRate = r
			} else {
				pw.Stages = append(pw.Stages, Stage{Target: r})
			}
			pw.Stages = append(pw.Stages, Stage{Target: r, Duration: Dur(st.Spike.For.D())})
			pw.Stages = append(pw.Stages, Stage{Target: rate}) // back to the rate before
			pw.PeakRate = max(pw.PeakRate, r)
		}
		first = false
	}

	k := w.Keys
	pw.Keys = Keys{Distribution: k.Distribution, Count: 1}
	if k.Count != nil {
		pw.Keys.Count = max(1, int64(math.Round(float64(*k.Count)*scale)))
	}
	if k.Skew != nil {
		pw.Keys.Skew = *k.Skew
	}
	if k.Share != nil {
		pw.Keys.Share, pw.Keys.Fraction = *k.Share, *k.Fraction
	}

	// Enough VUs to keep arrivals open while requests take up to about
	// two seconds; past that, k6 reports dropped iterations, which the
	// report shows as the load generator unable to keep the rate. VUs
	// started mid-run take time to initialize and drop arrivals meanwhile,
	// so enough for 250 ms of latency at peak are allocated up front.
	pw.MaxVUs = int(math.Min(maxVUsCap, math.Max(100, math.Ceil(pw.PeakRate*2))))
	pw.PreVUs = int(math.Min(float64(pw.MaxVUs), math.Max(20, math.Ceil(pw.PeakRate/4))))
	p.Workload = pw
}

// connectionFault lists faults that act on proxied connections.
var connectionFault = map[string]bool{"latency": true, "bandwidth": true, "timeout": true, "reset": true}

func (p *Plan) faults(d *design.Design, byName map[string]*Service) error {
	for i, f := range d.Scenario {
		pf := &Fault{Index: i, At: Dur(f.At.D()), Inject: f.Inject}
		if f.For != nil {
			v := Dur(f.For.D())
			pf.For = &v
		}
		switch f.Inject {
		case "loss", "partition", "disk_slow":
			return fmt.Errorf("scenario[%d]: %s isn't supported by local runs yet", i, f.Inject)
		}
		pf.Target = f.Target.Name
		t := byName[f.Target.Name]
		containers := t.Containers
		if f.Target.Index != nil {
			containers = []string{t.Containers[*f.Target.Index]}
		}
		if connectionFault[f.Inject] {
			for _, x := range p.Proxies {
				if x.To == pf.Target && slices.ContainsFunc(containers, func(c string) bool { return strings.HasPrefix(x.Upstream, c+":") }) {
					pf.Proxies = append(pf.Proxies, x.Name)
				}
			}
		} else {
			pf.Containers = containers
		}
		if f.AddMs != nil {
			pf.AddMs = *f.AddMs
		}
		if f.JitterMs != nil {
			pf.JitterMs = *f.JitterMs
		}
		if f.Kbps != nil {
			pf.Kbps = *f.Kbps
		}
		p.Faults = append(p.Faults, pf)
	}
	return nil
}

func (p *Plan) infra() {
	in := &p.Infra
	peak := p.Workload.PeakRate
	in.LoadCPU = clampHalf(peak/k6RatePerCPU, minLoadCPU, maxLoadCPU)
	in.LoadMemory = 1 << 30
	if p.Workload.MaxVUs > 1000 {
		in.LoadMemory = 2 << 30
	}

	// Proxied calls per request, from the design, times the peak rate.
	var perRequest float64
	for _, s := range p.Services {
		if s.Kind != "app" || len(s.Synthetic) == 0 {
			continue
		}
		perRequest += callsPerRequest(s.Synthetic[0].Calls, p.Workload.ReadShare)
	}
	in.ExpectedOps = peak * perRequest
	in.ProxyCPU = clampHalf(in.ExpectedOps/proxyOpsPerCPU, minProxyCPU, maxProxyCPU)
	in.ProxyMemory = 256 << 20
	in.PromCPU, in.PromMemory = 1, 1<<30
	in.CAdvisorCPU, in.CAdvisorMem = 0.5, 256<<20
	in.ExporterCPU, in.ExporterMem = 0.25, 128<<20
	p.note("Loadlore's own containers get fixed budgets: load generator %s CPU (peak %s rps), Toxiproxy %s CPU (about %s proxied calls/s at peak), Prometheus %s, cAdvisor %s, exporters %s each",
		fmtF(in.LoadCPU), fmtF(math.Round(peak)), fmtF(in.ProxyCPU), fmtF(math.Round(in.ExpectedOps)), fmtF(in.PromCPU), fmtF(in.CAdvisorCPU), fmtF(in.ExporterCPU))
}

// callsPerRequest is the expected number of sync and async calls one request
// makes, counting on_miss calls as if every get missed (an upper bound).
func callsPerRequest(calls []synthetic.Call, readShare float64) float64 {
	var n float64
	for _, c := range calls {
		share := 1.0
		switch c.When {
		case "read":
			share = readShare
		case "write":
			share = 1 - readShare
		}
		n += share * float64(c.Count) * (1 + callsPerRequest(c.OnMiss, readShare))
	}
	return n
}

// clampHalf rounds up to the next half core and clamps.
func clampHalf(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, math.Ceil(v*2)/2))
}

// TotalCPU and TotalMemory sum every container's limits, Loadlore's included.
func (p *Plan) TotalCPU() float64 {
	t := p.Infra.LoadCPU + p.Infra.ProxyCPU + p.Infra.PromCPU + p.Infra.CAdvisorCPU
	for _, s := range p.Services {
		t += s.CPU * float64(len(s.Containers))
		if s.Kind == "postgres" || s.Kind == "redis" {
			t += p.Infra.ExporterCPU
		}
	}
	return t
}

// TotalMemory sums every container's memory limit.
func (p *Plan) TotalMemory() int64 {
	t := p.Infra.LoadMemory + p.Infra.ProxyMemory + p.Infra.PromMemory + p.Infra.CAdvisorMem
	for _, s := range p.Services {
		t += s.Memory * int64(len(s.Containers))
		if s.Kind == "postgres" || s.Kind == "redis" {
			t += p.Infra.ExporterMem
		}
	}
	return t
}

// Service returns the service with the given name.
func (p *Plan) Service(name string) *Service {
	for _, s := range p.Services {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// OfferedRate is the load generator's target arrival rate at t into the
// load: 0 before the start and after the profile ends.
func (w *Workload) OfferedRate(t time.Duration) float64 {
	if t < 0 || t >= w.Duration() {
		return 0
	}
	rate := w.StartRate
	var elapsed time.Duration
	for _, st := range w.Stages {
		d := st.Duration.D()
		if d == 0 {
			rate = st.Target
			continue
		}
		if t < elapsed+d {
			return rate + (st.Target-rate)*float64(t-elapsed)/float64(d)
		}
		elapsed += d
		rate = st.Target
	}
	return rate
}

// Duration is the length of the profile.
func (w *Workload) Duration() time.Duration {
	var d time.Duration
	for _, st := range w.Stages {
		d += st.Duration.D()
	}
	return d
}

// --- helpers ---

var pgUnits = []struct {
	suffix string
	mult   float64
}{{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"kB", 1 << 10}, {"tb", 1 << 40}, {"gb", 1 << 30}, {"mb", 1 << 20}, {"kb", 1 << 10}, {"g", 1 << 30}, {"m", 1 << 20}, {"k", 1 << 10}}

// scaleSetting scales a memory setting written with a unit (256MB, 200mb,
// 1g) and writes it back in the same unit family, in kB so nothing rounds
// to zero.
func scaleSetting(v string, scale float64) (string, bool) {
	for _, u := range pgUnits {
		num, ok := strings.CutSuffix(v, u.suffix)
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
		if err != nil {
			return "", false
		}
		kb := int64(math.Max(64, math.Round(f*u.mult*scale/1024)))
		if u.suffix == strings.ToLower(u.suffix) && u.suffix != "kB" { // redis style
			return strconv.FormatInt(kb, 10) + "kb", true
		}
		return strconv.FormatInt(kb, 10) + "kB", true
	}
	return "", false
}

func sizeStr(b int64) string { return design.Size(b).String() }

func fmtF(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
