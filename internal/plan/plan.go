// Package plan turns a validated design into a run plan: every container to
// start, with resources scaled, every proxied connection, the synthetic
// service configs, the load profile and the fault timeline. A plan is
// independent of where it runs; drivers turn it into infrastructure.
package plan

import (
	"fmt"
	"maps"
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
	LBStatsPort   = 8405
	PostgresPort  = 5432
	PgBouncerPort = 6432
	RedisPort     = 6379
	ProxyHost     = "lore-toxiproxy"
	ProxyBasePort = 20000
	DBUser        = "loadlore"
	DBPassword    = "loadlore"
	DBName        = "lab"
)

// DefaultCPUPeriodMs is the CFS period for the design's own containers
// when the design doesn't set resources.cpu_period_ms: Docker's default.
// Loadlore's tooling always keeps Docker's default.
const DefaultCPUPeriodMs = 100

// Plan is everything a driver needs to run one experiment.
type Plan struct {
	RunID    string        `json:"run_id"`
	Name     string        `json:"name"`
	Scale    float64       `json:"scale"`
	Seed     int64         `json:"seed"`
	Duration time.Duration `json:"duration"`
	// ReuseData skips the fresh copy of seeded datasets: the run uses a
	// scratch copy that earlier --reuse-data runs may have written to.
	ReuseData bool `json:"reuse_data"`

	Services []*Service `json:"services"`
	Proxies  []*Proxy   `json:"proxies"`
	Workload Workload   `json:"workload"`
	Faults   []*Fault   `json:"faults"`
	Infra    Infra      `json:"infra"`

	// Notes explain choices the plan made that a reader of the report
	// should know about, such as values changed by scaling.
	Notes []string `json:"notes"`

	// Assumptions are values Loadlore chose that the design didn't state
	// and that results depend on. Reports print them next to the design's
	// own assumptions.
	Assumptions []Assumption `json:"assumptions"`
}

// Assumption is a value Loadlore chose for the design.
type Assumption struct {
	Field string `json:"field"`
	Value string `json:"value"`
	Why   string `json:"why"`
}

// Disk is a postgres's data disk modelled like a cloud volume: every read
// that reaches it waits ReadMs, and reads and writes are each capped at
// IOPS a second (scaled with --scale).
type Disk struct {
	ReadMs int `json:"read_ms"`
	IOPS   int `json:"iops"`
}

// Default disk: about a gp3 volume's read latency and baseline IOPS.
const (
	DefaultDiskReadMs = 1
	DefaultDiskIOPS   = 3000
)

// Service is one design service, expanded to containers.
type Service struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"` // app, postgres, redis, pgbouncer, lb
	Image      string   `json:"image"`
	Containers []string `json:"containers"` // one per replica: api-0, api-1; or db
	// SlowDisk: a disk_slow fault targets this postgres, or it has a Disk,
	// so its data lives on a dm-delay device.
	SlowDisk bool `json:"slow_disk,omitempty"`
	// Disk models the data disk like a cloud volume (the design's disk).
	Disk      *Disk   `json:"disk,omitempty"`
	CPU       float64 `json:"cpu"`           // per container, after scaling
	CPUPeriod int     `json:"cpu_period_ms"` // CFS period the CPU limit is enforced over
	Memory    int64   `json:"memory"`        // bytes per container, after scaling
	Port      int     `json:"port"`

	// Application services: one config per replica (synthetic image only).
	Synthetic []*synthetic.Config `json:"synthetic,omitempty"`

	// Datastores.
	Version string            `json:"version,omitempty"`
	Config  map[string]string `json:"config,omitempty"` // after scaling
	Dataset *dataset.Spec     `json:"dataset,omitempty"`

	// Load balancers.
	LB *LB `json:"lb,omitempty"`

	// PgBouncer.
	PgBouncer *PgBouncer `json:"pgbouncer,omitempty"`

	// Postgres read replicas: the primary it streams from. A replica's
	// Kind is postgres, so it is measured like one; it starts from a copy
	// of the primary's seeded data, and its Dataset is the primary's.
	Primary string `json:"primary,omitempty"`
}

// Replicas lists the read replicas of the postgres service named primary.
func (p *Plan) Replicas(primary string) []*Service {
	var out []*Service
	for _, s := range p.Services {
		if s.Primary == primary && primary != "" {
			out = append(out, s)
		}
	}
	return out
}

// Replication settings Loadlore applies.
const (
	// DefaultWALKeepSize is the WAL a primary with replicas keeps for them
	// unless the design sets wal_keep_size: enough for a replica that falls
	// behind under a run's write load to catch up rather than lose its place.
	DefaultWALKeepSize = "1GB"
)

// PgBouncer is a PgBouncer's settings and the Postgres behind it.
type PgBouncer struct {
	Target          string `json:"target"`
	PoolMode        string `json:"pool_mode"`
	DefaultPoolSize int    `json:"default_pool_size"`
	MaxClientConn   int    `json:"max_client_conn"`
	Upstream        string `json:"upstream"` // the proxy in front of the target
}

// LB is a load balancer's settings and the replicas behind it.
type LB struct {
	Target      string     `json:"target"`
	Algorithm   string     `json:"algorithm"`
	CheckPath   string     `json:"check_path"`
	IntervalMs  int        `json:"interval_ms"`
	TimeoutMs   int        `json:"timeout_ms"`
	Fall        int        `json:"fall"`
	Rise        int        `json:"rise"`
	Servers     []LBServer `json:"servers"`
	ServerPort  int        `json:"server_port"`
	StatsListen int        `json:"stats_listen"`
}

// LBServer is one replica behind an lb, reached through its proxy.
type LBServer struct {
	Name string `json:"name"` // the replica's container: api-0
	Addr string `json:"addr"` // the proxy in front of it
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
	SLOP99Ms     float64  `json:"slo_p99_ms"`
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
	// Loss: the share of packets dropped, and the target's service port,
	// whose replies lose them.
	Percent float64 `json:"percent,omitempty"`
	Port    int     `json:"port,omitempty"`
	// Partition: the two sides, and for each container on either side the
	// proxy ports leading to the other, whose packets it drops.
	Between []string `json:"between,omitempty"`
	Cuts    []Cut    `json:"cuts,omitempty"`
}

// Cut is one container's share of a partition: every packet it sends to
// these Toxiproxy ports is dropped, as on a network that silently stopped
// carrying them.
type Cut struct {
	Container string `json:"container"`
	Ports     []int  `json:"ports"`
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

// ProxyShareLimit is how much of Toxiproxy's CPU budget a design's
// estimated peak may use: above it, Toxiproxy was CPU-throttled in bursts
// in the pool trap's runs, so pre-flight warns.
const ProxyShareLimit = 0.65

// ProxyShare is the share of Toxiproxy's CPU budget its estimated peak
// traffic uses, by the cost model (proxyOpsPerCPU calls per CPU).
func (in Infra) ProxyShare() float64 {
	if in.ProxyCPU <= 0 {
		return 0
	}
	return in.ExpectedOps / proxyOpsPerCPU / in.ProxyCPU
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
	ReuseData      bool
	// PreallocVUs, at scale 1, are started before the load, when more
	// than the peak rate times the SLO: a scenario's measured sizing.
	PreallocVUs int
}

// Measured budgets for Loadlore's own containers. Early checks: k6 at 1 CPU
// held 5,000 rps against a fast target with <0.06% dropped; one Toxiproxy
// core relays about 15k evenly paced queries per second before it
// saturates. In the first full run of the spec example (scale 0.75), the
// peak 10-second CPU use was k6 0.53 at 3,750 rps (about 7,000 rps per
// core), Toxiproxy 0.65 at about 11,600 proxied calls/s (about 18,000 per
// core), Prometheus 0.02, cAdvisor 0.03 and postgres_exporter 0.01. At scale
// 1 with an lb, Toxiproxy relayed about 20,000 calls/s per core. The budgets
// below leave Toxiproxy about 1.2x headroom over those rates, and
// Prometheus and cAdvisor about 2x over theirs (peaks of 0.045 and 0.06 at
// scale 1); reports mark a run inconclusive if a tool still reaches 90% of
// its budget before the system breaks.
//
// k6's average use misleads: on the first scenario's solution at scale 1
// (5,000 rps peak), k6 at 1.5 CPU averaged 35% of its budget but was
// stalled at its CPU limit in 2.5-4% of its scheduling periods as the
// system neared its break and 10-12.5% once slow responses piled up VUs,
// which inflates the p99 it measures. At 2 CPU it wasn't stalled at all
// (2.5 and 3 CPU the same), so k6 gets one CPU per 2,500 rps of peak.
//
// Toxiproxy's memory grows with open connections: when the spec example
// broke, it went from 17 MB to over 245 MB as requests queued, and was
// OOM-killed at a 256 MB limit. It gets 1 GiB.
const (
	k6RatePerCPU   = 2500.0
	proxyOpsPerCPU = 15000.0
	proxyMemory    = 1 << 30
	minLoadCPU     = 1.5 // at 1 CPU, k6 stalled in bursts at about a third of it (pool trap runs, 1,400 rps)
	maxLoadCPU     = 4.0
	minProxyCPU    = 1.0 // at 0.5 CPU, Toxiproxy was throttled in bursts at 58-65% of it (pool trap, mains power)
	maxProxyCPU    = 4.0
	promCPU        = 0.1
	cadvisorCPU    = 0.1
	exporterCPU    = 0.1
	MaxVUsCap      = 4000 // the most VUs the load generator gets
	minMemoryApp   = 32 << 20
	minMemoryPG    = 128 << 20
	minMemoryRedis = 32 << 20
	// Memory each connection holds, which --scale doesn't shrink because
	// connection counts don't scale. A Postgres backend measured about
	// 1.5 MiB (97 backends, 145 MiB RSS, in the first scenario at scale 1);
	// an HAProxy connection holds two 16 KiB buffers by default.
	pgBackendMemory  = 2 << 20
	pgBaseMemory     = 64 << 20
	pgDefaultConns   = 100
	pgDefaultBuffers = 128 << 20
	lbConnMemory     = 40 << 10
	lbBaseMemory     = 32 << 20
	defaultPGImage   = "postgres:%s"
	defaultRedisImg  = "redis:%s"
	pinnedPG17       = "postgres:17.11"
	pinnedRedis8     = "redis:8.2"
	pinnedHAProxy    = "haproxy:3.2.26"
	pinnedPgBouncer  = "edoburu/pgbouncer:v1.26.1-p0" // PgBouncer 1.26.1
	minMemoryPgb     = 32 << 20
	// PgBouncer's own default for max_client_conn.
	pgbDefaultMaxClientConn = 100
	redisMemoryShare        = 0.75
	loadGeneratorKey        = "load"
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
	p := &Plan{RunID: opt.RunID, Name: d.Name, Scale: scale, Seed: opt.Seed, Duration: d.TotalDuration(), ReuseData: opt.ReuseData}
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
	p.replicas(d, byName)

	p.proxies(d, byName)
	for _, name := range names {
		if s := byName[name]; s.Kind == "app" && d.Services[name].IsSynthetic() {
			p.syntheticConfigs(d, name, s, byName)
		}
	}
	p.lbs(d, byName)
	p.workload(d, byName, scale, opt)
	if scale < 1 {
		p.connectionMemory(d)
	}
	if err := p.faults(d, byName); err != nil {
		return nil, err
	}
	p.infra(d)
	p.assume(d)
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
	case ds.Kind == "postgres-replica":
		// The rest, the primary's image and dataset, is filled once every
		// service is built (replicas).
		s.Kind, s.Port, minMem = "postgres", PostgresPort, minMemoryPG
		s.Version, s.Primary = ds.Version, ds.Primary
		s.Containers = []string{name}
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
		if dk := ds.Disk; dk != nil {
			disk := Disk{ReadMs: DefaultDiskReadMs, IOPS: DefaultDiskIOPS}
			if dk.ReadMs != nil {
				disk.ReadMs = *dk.ReadMs
			}
			if dk.IOPS != nil {
				disk.IOPS = *dk.IOPS
			}
			disk.IOPS = max(1, int(math.Round(float64(disk.IOPS)*scale)))
			s.Disk, s.SlowDisk = &disk, true
		}
	case ds.Kind == "redis":
		s.Kind, s.Port, minMem = "redis", RedisPort, minMemoryRedis
		s.Version = ds.Version
		s.Image = fmt.Sprintf(defaultRedisImg, ds.Version)
		if ds.Version == "8" {
			s.Image = pinnedRedis8
		}
		s.Containers = []string{name}
	case ds.Kind == "pgbouncer":
		s.Kind, s.Port, minMem = "pgbouncer", PgBouncerPort, minMemoryPgb
		s.Image = pinnedPgBouncer
		s.Containers = []string{name}
		s.PgBouncer = &PgBouncer{Target: ds.Target, PoolMode: ds.PoolMode, DefaultPoolSize: *ds.DefaultPoolSize, MaxClientConn: pgbDefaultMaxClientConn}
		if v, ok := ds.Config["max_client_conn"]; ok {
			if n, err := strconv.Atoi(v.String()); err == nil {
				s.PgBouncer.MaxClientConn = n
			}
		}
	case ds.Kind == "lb":
		s.Kind, s.Port, minMem = "lb", AppPort, minMemoryApp
		s.Image = pinnedHAProxy
		s.Containers = []string{name}
		hc := ds.HealthCheck
		s.LB = &LB{
			Target: ds.Target, Algorithm: ds.Algorithm, CheckPath: hc.Path,
			IntervalMs: *hc.IntervalMs, TimeoutMs: *hc.TimeoutMs, Fall: *hc.Fall, Rise: *hc.Rise,
			StatsListen: LBStatsPort, Servers: []LBServer{},
		}
	default:
		return nil, fmt.Errorf("%s: %s isn't supported by local runs yet", name, ds.Kind)
	}
	if mem < minMem {
		p.note("%s: memory raised from %s to the %s minimum for %s", name, sizeStr(mem), sizeStr(minMem), s.Kind)
		mem = minMem
	}
	s.CPU, s.Memory, s.CPUPeriod = cpu, mem, DefaultCPUPeriodMs
	if ds.Resources.CPUPeriodMs != nil {
		s.CPUPeriod = *ds.Resources.CPUPeriodMs
	}

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
	if s.Disk != nil {
		// Times each block read, so the analysis can tell reads that wait
		// for the disk from ones the page cache answers.
		if _, set := s.Config["track_io_timing"]; !set {
			if s.Config == nil {
				s.Config = map[string]string{}
			}
			s.Config["track_io_timing"] = "on"
		}
	}
	if s.Kind == "redis" {
		p.redisMemory(name, s)
	}
	return s, nil
}

// connectionMemory keeps scaled-down memory above what the unscaled
// connections need, never above the design's own value. Without it a
// scaled Postgres with a full set of connections, or an lb holding every
// client connection, is OOM-killed: a failure the full-size design
// doesn't have.
func (p *Plan) connectionMemory(d *design.Design) {
	for _, s := range p.Services {
		var need int64
		var why string
		switch s.Kind {
		case "postgres":
			conns := int64(pgDefaultConns)
			if v, ok := s.Config["max_connections"]; ok {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					conns = n
				}
			}
			buffers := int64(pgDefaultBuffers)
			if v, ok := s.Config["shared_buffers"]; ok {
				if b, ok := pgBytes(v); ok {
					buffers = b
				}
			}
			need = buffers + conns*pgBackendMemory + pgBaseMemory
			why = fmt.Sprintf("shared_buffers %s plus about %s for each of %d connections, which don't scale", sizeStr(buffers), sizeStr(pgBackendMemory), conns)
		case "lb":
			conns := int64(p.Workload.MaxVUs)
			need = conns*lbConnMemory + lbBaseMemory
			why = fmt.Sprintf("about %s for each of up to %d client connections, which don't scale", sizeStr(lbConnMemory), conns)
		default:
			continue
		}
		need = (need + 1<<20 - 1) >> 20 << 20
		full := d.Services[s.Name].Resources.Memory.Bytes()
		if need = min(need, full); s.Memory < need {
			p.note("%s: memory kept at %s instead of %s: %s", s.Name, sizeStr(need), sizeStr(s.Memory), why)
			s.Memory = need
		}
	}
}

// pgBytes reads a Postgres memory setting such as 256MB or 65536kB.
func pgBytes(v string) (int64, bool) {
	for _, u := range pgUnits {
		if num, ok := strings.CutSuffix(v, u.suffix); ok {
			f, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
			return int64(f * u.mult), err == nil
		}
	}
	return 0, false
}

// redisMemory caps Redis below its container limit and evicts least
// recently used keys, unless the design sets either: stock Redis has no
// cap and would be OOM-killed when the cache outgrows its container.
func (p *Plan) redisMemory(name string, s *Service) {
	if s.Config == nil {
		s.Config = map[string]string{}
	}
	if _, set := s.Config["maxmemory"]; !set {
		capBytes := int64(float64(s.Memory)*redisMemoryShare) >> 10 << 10
		s.Config["maxmemory"] = strconv.FormatInt(capBytes>>10, 10) + "kb"
		p.Assumptions = append(p.Assumptions, Assumption{
			Field: "services." + name + ".config.maxmemory", Value: sizeStr(capBytes),
			Why: fmt.Sprintf("75%% of the container's %s memory limit, as managed Redis usually does; the rest is for fragmentation and client buffers. Set maxmemory: 0 for stock Redis (no cap)", sizeStr(s.Memory)),
		})
	}
	if _, set := s.Config["maxmemory-policy"]; !set {
		s.Config["maxmemory-policy"] = "allkeys-lru"
		p.Assumptions = append(p.Assumptions, Assumption{
			Field: "services." + name + ".config.maxmemory-policy", Value: "allkeys-lru",
			Why: "evict the least recently used keys when the cache is full, as managed Redis usually does",
		})
	}
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
		if ds.Kind == "lb" || ds.Kind == "pgbouncer" {
			t := byName[ds.Target]
			for _, c := range t.Containers {
				add(name, ds.Target, c, t.Port)
			}
			continue
		}
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

// lbs points each load balancer at the proxies in front of its target's
// replicas.
func (p *Plan) lbs(d *design.Design, byName map[string]*Service) {
	for _, name := range d.ServiceNames() {
		s := byName[name]
		if s.LB == nil {
			continue
		}
		for _, x := range p.proxiesInto(name, s.LB.Target) {
			s.LB.Servers = append(s.LB.Servers, LBServer{Name: strings.SplitN(x.Upstream, ":", 2)[0], Addr: x.Addr()})
		}
		s.LB.ServerPort = byName[s.LB.Target].Port
	}
	for _, name := range d.ServiceNames() {
		s := byName[name]
		if s.PgBouncer == nil {
			continue
		}
		s.PgBouncer.Upstream = p.proxiesInto(name, s.PgBouncer.Target)[0].Addr()
		clients := 1 // the exporter's
		for _, caller := range d.ServiceNames() {
			if pool, ok := d.Services[caller].Pools[name]; ok && byName[caller].Kind == "app" {
				clients += *pool.Size * len(byName[caller].Containers)
			}
		}
		if clients > s.PgBouncer.MaxClientConn {
			p.note("%s: its callers' pools can open %d client connections, the exporter's included, against max_client_conn %d; PgBouncer refuses the ones beyond it", name, clients, s.PgBouncer.MaxClientConn)
		}
	}
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
		pool := ds.Pools[target]
		st := synthetic.Target{Addrs: addrs, Pool: *pool.Size, PoolMin: *pool.Min}
		if pool.Hold == synthetic.HoldRequest {
			st.Hold = synthetic.HoldRequest
		}
		switch t.Kind {
		case "app", "lb":
			st.Kind = synthetic.KindHTTP
		case "postgres":
			st.Kind = synthetic.KindPostgres
			st.Database, st.User, st.Password = DBName, DBUser, DBPassword
			st.Rows, st.RowBytes = max(t.Dataset.Rows, 1), t.Dataset.RowBytes
			st.Data = p.replicatedData(t)
		case "pgbouncer":
			// Through PgBouncer to its Postgres, whose dataset it reads.
			db := byName[t.PgBouncer.Target]
			st.Kind = synthetic.KindPostgres
			st.Database, st.User, st.Password = DBName, DBUser, DBPassword
			st.Rows, st.RowBytes = max(db.Dataset.Rows, 1), db.Dataset.RowBytes
			st.Data = p.replicatedData(db)
			// The client prepares statements as it does against Postgres:
			// PgBouncer tracks them across server connections in every
			// pool mode (max_prepared_statements).
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
		sc := synthetic.Call{Target: c.Target, Op: c.Op, Count: *c.Count, When: c.When, Mode: c.Mode, Query: c.Query, OnMiss: convertCalls(c.OnMiss), Coalesce: c.Coalesce}
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

func (p *Plan) workload(d *design.Design, byName map[string]*Service, scale float64, opt Options) {
	w := d.Workload
	entry := byName[w.Entry]
	pw := Workload{Entry: w.Entry, ReadShare: *w.Mix.Read, PayloadBytes: int(*w.PayloadKB * 1024), Timeout: Dur(time.Duration(*w.TimeoutMs) * time.Millisecond), SLOP99Ms: *w.SLO.P99Ms}
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
	// which during a stall lightens the load just as the system struggles,
	// so enough to ride out latency up to the SLO at peak are allocated up
	// front.
	// A scenario's measured sizing raises the VUs up front (see
	// scenario.LoadMeta); in-flight requests scale with the rate.
	pre := math.Max(20, math.Ceil(pw.PeakRate*pw.SLOP99Ms/1000))
	measured := math.Ceil(float64(opt.PreallocVUs) * scale)
	pw.MaxVUs = int(math.Min(MaxVUsCap, math.Max(math.Max(100, math.Ceil(pw.PeakRate*2)), measured)))
	pw.PreVUs = int(math.Min(float64(pw.MaxVUs), math.Max(pre, measured)))
	if measured > pre {
		p.note("load generator: %d VUs started before the load, the scenario's measured prealloc_vus %d at --scale 1, more than %s rps × the SLO's %g ms", pw.PreVUs, opt.PreallocVUs, fmtF(math.Round(pw.PeakRate)), pw.SLOP99Ms)
	}
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
		case "disk_slow":
			t := byName[f.Target.Name]
			if t.Kind != "postgres" {
				return fmt.Errorf("scenario[%d]: disk_slow on %s: local runs slow a postgres's disk only", i, f.Target.Name)
			}
			t.SlowDisk = true
			pf.Target, pf.AddMs, pf.Containers = t.Name, *f.AddMs, t.Containers
			p.Faults = append(p.Faults, pf)
			continue
		case "partition":
			if err := p.partition(pf, f, byName); err != nil {
				return fmt.Errorf("scenario[%d]: %w", i, err)
			}
			p.Faults = append(p.Faults, pf)
			continue
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
		if f.Inject == "loss" {
			pf.Percent, pf.Port = *f.Percent, t.Port
		}
		p.Faults = append(p.Faults, pf)
	}
	return nil
}

func (p *Plan) infra(d *design.Design) {
	in := &p.Infra
	peak := p.Workload.PeakRate
	in.LoadCPU = clampHalf(peak/k6RatePerCPU, minLoadCPU, maxLoadCPU)
	in.LoadMemory = 1 << 30
	if p.Workload.MaxVUs > 1000 {
		in.LoadMemory = 2 << 30
	}

	in.ExpectedOps = p.proxiedOps(d, peak)
	// Sized so the estimated peak uses at most ProxyShareLimit of it: above
	// that, Toxiproxy was CPU-throttled in bursts.
	in.ProxyCPU = clampHalf(in.ExpectedOps/proxyOpsPerCPU/ProxyShareLimit, minProxyCPU, maxProxyCPU)
	in.ProxyMemory = proxyMemory
	in.PromCPU, in.PromMemory = promCPU, 1<<30
	in.CAdvisorCPU, in.CAdvisorMem = cadvisorCPU, 256<<20
	in.ExporterCPU, in.ExporterMem = exporterCPU, 128<<20
	p.note("Loadlore's own containers get fixed budgets: load generator %s CPU (peak %s rps), Toxiproxy %s CPU (about %s proxied calls/s at peak), Prometheus %s, cAdvisor %s, exporters %s each",
		fmtF(in.LoadCPU), fmtF(math.Round(peak)), fmtF(in.ProxyCPU), fmtF(math.Round(in.ExpectedOps)), fmtF(in.PromCPU), fmtF(in.CAdvisorCPU), fmtF(in.ExporterCPU))
}

// proxiedOps estimates the proxied calls per second at a given entry rate,
// following the call graph: each lb forwards what reaches it, and each
// synthetic service makes its calls for every request that reaches it.
// on_miss calls count as if every get missed, so it is an upper bound.
func (p *Plan) proxiedOps(d *design.Design, rate float64) float64 {
	readShare := p.Workload.ReadShare
	var ops float64
	if len(p.proxiesInto(loadGeneratorKey, d.Workload.Entry)) > 0 {
		ops += rate
	}
	var visit func(name string, r float64)
	visit = func(name string, r float64) {
		ds := d.Services[name]
		switch {
		case ds.Kind == "lb":
			ops += r
			visit(ds.Target, r)
		case ds.IsSynthetic() && ds.Behavior != nil:
			var walk func(calls []*design.Call, r float64)
			walk = func(calls []*design.Call, r float64) {
				for _, c := range calls {
					share := 1.0
					switch c.When {
					case "read":
						share = readShare
					case "write":
						share = 1 - readShare
					}
					n := r * share * float64(*c.Count)
					ops += n
					switch t := d.Services[c.Target]; {
					case t.IsApp() || t.Kind == "lb":
						visit(c.Target, n)
					case t.Kind == "pgbouncer":
						ops += n // and on from PgBouncer to its Postgres
					}
					walk(c.OnMiss, n)
				}
			}
			walk(ds.Behavior.Calls, r)
		}
	}
	visit(d.Workload.Entry, rate)
	return ops
}

// assume records the values Loadlore chose for the run that the design
// didn't state and that results depend on.
func (p *Plan) assume(d *design.Design) {
	why := "the default; k6 gives up on a request after this long and counts it as failed"
	if d.Workload.TimeoutMs != nil && *d.Workload.TimeoutMs != design.DefaultClientTimeoutMs {
		why = "set in the design; k6 gives up on a request after this long and counts it as failed"
	}
	p.Assumptions = append(p.Assumptions, Assumption{
		Field: "workload.timeout_ms", Value: p.Workload.Timeout.D().String(), Why: why,
	})
	why = "the default; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99, so latency users would still accept isn't called a break"
	if d.Workload.SLO != nil && d.Workload.SLO.P99Ms != nil && *d.Workload.SLO.P99Ms != design.DefaultSLOP99Ms {
		why = "set in the design; a break on p99 alone needs p99 at or above this as well as 10x the low-load p99"
	}
	p.Assumptions = append(p.Assumptions, Assumption{
		Field: "workload.slo.p99_ms", Value: fmtF(p.Workload.SLOP99Ms) + " ms", Why: why,
	})
	p.assumePeriods(d)
	for _, name := range d.ServiceNames() {
		if s := d.Services[name]; s.Kind == "pgbouncer" {
			p.assumePgBouncer(name, s)
		}
		if s := d.Services[name]; s.IsPostgres() {
			p.assumeDisk(name, s)
		}
		if s := d.Services[name]; s.Kind == "postgres-replica" {
			p.assumeReplica(name, s)
		}
		if s := d.Services[name]; s.Kind == "lb" {
			p.Assumptions = append(p.Assumptions, Assumption{
				Field: "services." + name + " timeouts", Value: "connect 5s, server 60s",
				Why: "HAProxy's own timeouts, with its defaults otherwise. A request already sent to a replica that dies waits for the server timeout (or the load generator's); health checks only stop new requests going there",
			})
		}
	}
	for _, name := range d.ServiceNames() {
		s := d.Services[name]
		for _, t := range slices.Sorted(maps.Keys(s.Pools)) {
			ts, pool := d.Services[t], s.Pools[t]
			if ts == nil || (ts.Kind != "postgres" && ts.Kind != "pgbouncer") || pool.Min == nil {
				continue
			}
			a := Assumption{Field: "services." + name + ".pools." + t + ".min", Value: strconv.Itoa(*pool.Min),
				Why: "the default, as pgxpool's: each replica opens connections only when callers need them, so a pool fills as load rises, and a burst opens many at once"}
			if *pool.Min > 0 {
				a.Why = fmt.Sprintf("set in the design: each replica opens %d connections before the load starts and keeps them open", *pool.Min)
			}
			p.Assumptions = append(p.Assumptions, a)
			h := Assumption{Field: "services." + name + ".pools." + t + ".hold", Value: pool.Hold,
				Why: "the default, as pgxpool's: a request takes a connection for each call and hands it back straight away"}
			if pool.Hold == synthetic.HoldRequest {
				h.Why = "set in the design: a request keeps the connection from its first call to " + t + " until it responds, through any slower calls in between, as Rails and Django check out a connection per request"
			}
			p.Assumptions = append(p.Assumptions, h)
		}
	}
	for _, name := range d.ServiceNames() {
		s := d.Services[name]
		if !s.IsApp() || *s.Replicas < 2 {
			continue
		}
		direct := d.Workload.Entry == name
		for _, o := range d.Services {
			if o.Behavior != nil && slices.Contains(callTargets(o.Behavior.Calls), name) {
				direct = true
			}
		}
		if direct {
			p.Assumptions = append(p.Assumptions, Assumption{
				Field: "services." + name + ".replicas", Value: strconv.Itoa(*s.Replicas),
				Why: "traffic reaches it without an lb: callers spread requests across the replicas themselves (round-robin), with no health checks",
			})
		}
	}
	if p.Assumptions == nil {
		p.Assumptions = []Assumption{}
	}
}

// assumeDisk says what a postgres's data disk is: the Docker host's, which
// on a laptop answers reads that miss memory from the host's own memory,
// or one modelled like a cloud volume.
func (p *Plan) assumeDisk(name string, ds *design.Service) {
	s := p.Service(name)
	if s == nil {
		return
	}
	a := Assumption{Field: "services." + name + ".disk", Value: "the Docker host's",
		Why: "no disk block: a read that misses " + name + "'s memory goes to the Docker host's disk, which on a laptop mostly answers from the host's own memory, fast and uncapped. Add disk: {} for a cloud volume's wait and IOPS cap"}
	if s.Disk != nil {
		a.Value = fmt.Sprintf("%d ms per read, %d IOPS", s.Disk.ReadMs, s.Disk.IOPS)
		a.Why = "set in the design: every read that misses " + name + "'s memory waits this long, and reads and writes are each capped at this rate, as on a cloud volume"
		if ds.Disk.ReadMs == nil || ds.Disk.IOPS == nil {
			a.Why += fmt.Sprintf(" (unset values default to about a gp3 volume's: %d ms, %d IOPS)", DefaultDiskReadMs, DefaultDiskIOPS)
		}
		if p.Scale != 1 {
			a.Why += "; IOPS scaled with --scale, the wait not"
		}
	}
	p.Assumptions = append(p.Assumptions, a)
}

// assumePeriods lists the CFS periods of the services that don't set one,
// grouped by value.
func (p *Plan) assumePeriods(d *design.Design) {
	var names []string
	for _, s := range p.Services {
		if r := d.Services[s.Name].Resources; r == nil || r.CPUPeriodMs == nil {
			names = append(names, s.Name)
		}
	}
	if len(names) == 0 {
		return
	}
	p.Assumptions = append(p.Assumptions, Assumption{
		Field: "resources.cpu_period_ms",
		Value: fmt.Sprintf("%d ms for %s", DefaultCPUPeriodMs, strings.Join(names, ", ")),
		Why:   "Docker's default. A container that uses its CPU share early in a period stalls until the next one, which can show up in p99 when a service has a fraction of a CPU",
	})
}

// assumePgBouncer prints the PgBouncer settings results depend on.
func (p *Plan) assumePgBouncer(name string, ds *design.Service) {
	b := p.Service(name).PgBouncer
	field := "services." + name + "."
	_, setMax := ds.Config["max_client_conn"]
	why := "PgBouncer's own default: clients beyond it are refused"
	if setMax {
		why = "set in the design: clients beyond it are refused"
	}
	p.Assumptions = append(p.Assumptions,
		Assumption{Field: field + "pool_mode", Value: b.PoolMode,
			Why: "a server connection serves one client's transaction, then the next client's (transaction); for the whole client connection (session); or for one statement (statement). Loadlore's default is transaction, PgBouncer's is session"},
		Assumption{Field: field + "default_pool_size", Value: strconv.Itoa(b.DefaultPoolSize),
			Why: fmt.Sprintf("server connections to %s for the database and user; clients beyond them wait in PgBouncer for one", b.Target)},
		Assumption{Field: field + "config.max_client_conn", Value: strconv.Itoa(b.MaxClientConn), Why: why},
	)
	retry, why := "15s", "PgBouncer's own default"
	if v, ok := ds.Config["server_login_retry"]; ok {
		retry, why = v.String()+"s", "set in the design"
	}
	p.Assumptions = append(p.Assumptions, Assumption{Field: field + "config.server_login_retry", Value: retry,
		Why: why + ": after a failed connection to " + b.Target + ", PgBouncer waits this long before trying again, so it can recover from a restart of " + b.Target + " later than a direct client would"})
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
		if s.Kind == "postgres" || s.Kind == "redis" || s.Kind == "pgbouncer" {
			t += p.Infra.ExporterCPU
		}
	}
	return math.Round(t*100) / 100
}

// TotalMemory sums every container's memory limit.
func (p *Plan) TotalMemory() int64 {
	t := p.Infra.LoadMemory + p.Infra.ProxyMemory + p.Infra.PromMemory + p.Infra.CAdvisorMem
	for _, s := range p.Services {
		t += s.Memory * int64(len(s.Containers))
		if s.Kind == "postgres" || s.Kind == "redis" || s.Kind == "pgbouncer" {
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

// partition cuts each side's containers off from the proxies that lead to
// the other side's (or, for name[i], to that one replica).
func (p *Plan) partition(pf *Fault, f *design.Fault, byName map[string]*Service) error {
	var refs [2]design.TargetRef
	for i, side := range f.Between {
		ref, err := design.ParseTargetRef(side)
		if err != nil {
			return err
		}
		refs[i] = ref
	}
	pf.Between = f.Between
	containers := func(r design.TargetRef) []string {
		cs := byName[r.Name].Containers
		if r.Index != nil {
			return []string{cs[*r.Index]}
		}
		return cs
	}
	for _, pair := range [][2]design.TargetRef{{refs[0], refs[1]}, {refs[1], refs[0]}} {
		from, to := pair[0], pair[1]
		var ports []int
		for _, x := range p.proxiesInto(from.Name, to.Name) {
			if slices.ContainsFunc(containers(to), func(c string) bool { return strings.HasPrefix(x.Upstream, c+":") }) {
				ports = append(ports, x.Listen)
			}
		}
		if len(ports) == 0 {
			continue
		}
		for _, c := range containers(from) {
			pf.Cuts = append(pf.Cuts, Cut{Container: c, Ports: ports})
		}
	}
	if len(pf.Cuts) == 0 {
		return fmt.Errorf("partition: %s and %s don't call each other, so there is no connection to cut", f.Between[0], f.Between[1])
	}
	return nil
}

// replicas gives each read replica its primary's image and dataset, points
// it at the primary, and has each primary with replicas keep WAL for them.
func (p *Plan) replicas(d *design.Design, byName map[string]*Service) {
	for _, s := range p.Services {
		pr := byName[s.Primary]
		if s.Primary == "" || pr == nil {
			continue
		}
		s.Image = pr.Image
		spec := *pr.Dataset
		spec.Indexes = slices.Clone(pr.Dataset.Indexes)
		s.Dataset = &spec
		if s.Config == nil {
			s.Config = map[string]string{}
		}
		s.Config["primary_conninfo"] = fmt.Sprintf("host=%s port=%d user=%s password=%s application_name=%s", pr.Name, PostgresPort, DBUser, DBPassword, s.Name)
		if pr.Config == nil {
			pr.Config = map[string]string{}
		}
		if _, set := pr.Config["wal_keep_size"]; !set {
			pr.Config["wal_keep_size"] = DefaultWALKeepSize
		}
	}
}

// assumeReplica says how a read replica replicates: asynchronous streaming
// from a copy of its primary's data, and the WAL the primary keeps for it.
func (p *Plan) assumeReplica(name string, ds *design.Service) {
	pr := p.Service(ds.Primary)
	if pr == nil {
		return
	}
	keep := pr.Config["wal_keep_size"]
	why := "Loadlore's default: enough for a replica that falls behind under write load to catch up"
	if keep != DefaultWALKeepSize {
		why = "set in the design"
	}
	p.Assumptions = append(p.Assumptions,
		Assumption{Field: "services." + name + " replication", Value: "asynchronous streaming from " + ds.Primary,
			Why: "the replica starts from a copy of " + ds.Primary + "'s seeded data and replays its WAL as it arrives, directly, not through Toxiproxy: a commit on " + ds.Primary + " doesn't wait for it, so a read on " + name + " can miss a write that just committed"})
	field := "services." + ds.Primary + ".config.wal_keep_size"
	if !slices.ContainsFunc(p.Assumptions, func(a Assumption) bool { return a.Field == field }) {
		p.Assumptions = append(p.Assumptions, Assumption{Field: field, Value: keep, Why: why})
	}
}

// replicatedData is the primary whose data postgres service s serves, if
// that primary has read replicas: then the synthetic service checks reads
// after writes there. Empty otherwise.
func (p *Plan) replicatedData(s *Service) string {
	switch {
	case s.Primary != "":
		return s.Primary
	case len(p.Replicas(s.Name)) > 0:
		return s.Name
	}
	return ""
}
