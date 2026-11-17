package plan

import (
	"encoding/json"
	"flag"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/akinfemi/load-lore/internal/design"
)

var update = flag.Bool("update", false, "rewrite golden files")

func load(t *testing.T, src string) *design.Design {
	t.Helper()
	d, diags := design.Load([]byte(src))
	if diags.HasErrors() {
		t.Fatalf("design: %v", diags)
	}
	return d
}

func loadFile(t *testing.T, path string) *design.Design {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return load(t, string(b))
}

func TestGoldenPlans(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		scale      float64
	}{
		{"single-postgres", "../../examples/single-postgres.yaml", 1},
		{"single-postgres-half", "../../examples/single-postgres.yaml", 0.5},
		{"smoke", "../../examples/smoke.yaml", 1},
		{"pgbouncer", "../../examples/pgbouncer.yaml", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Build(loadFile(t, tc.path), Options{RunID: "test-run", Scale: tc.scale, Seed: 1})
			if err != nil {
				t.Fatal(err)
			}
			got, _ := json.MarshalIndent(p, "", "  ")
			path := filepath.Join("testdata", tc.name+".json")
			if *update {
				if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test -update)", err)
			}
			if string(append(got, '\n')) != string(want) {
				t.Errorf("%s differs; run go test -update after checking the change", path)
			}
		})
	}
}

func TestScaling(t *testing.T) {
	d := loadFile(t, "../../examples/single-postgres.yaml")
	full, err := Build(d, Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	half, err := Build(d, Options{RunID: "r", Scale: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	api, db := half.Service("api"), half.Service("db")
	if api.CPU != 0.4 || api.Memory != 256<<20 || len(api.Containers) != 4 {
		t.Errorf("api at 0.5: cpu %v, memory %d, replicas %d", api.CPU, api.Memory, len(api.Containers))
	}
	if db.Dataset.Rows != 2_500_000 || db.Config["shared_buffers"] != "131072kB" || db.Config["max_connections"] != "100" {
		t.Errorf("db at 0.5: rows %d, config %v", db.Dataset.Rows, db.Config)
	}
	if half.Workload.PeakRate != full.Workload.PeakRate/2 || half.Workload.Keys.Count != 2_500_000 {
		t.Errorf("load at 0.5: peak %v, keys %d", half.Workload.PeakRate, half.Workload.Keys.Count)
	}
	if got := api.Synthetic[0].Targets["db"].Pool; got != 24 {
		t.Errorf("pool scaled to %d; pools must not scale", got)
	}
	if !strings.Contains(strings.Join(half.Notes, "\n"), "shared_buffers scaled from 256MB to 131072kB") {
		t.Errorf("notes: %v", half.Notes)
	}
}

func TestProxiesAndSyntheticTargets(t *testing.T) {
	p, err := Build(loadFile(t, "../../examples/smoke.yaml"), Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	// Only the latency fault's connection goes through Toxiproxy: loss and
	// partition act on the containers themselves.
	var names []string
	for _, x := range p.Proxies {
		names = append(names, x.Name+"→"+x.Addr())
	}
	if got := strings.Join(names, " "); got != "api--cache→cache:6379 api--db→lore-toxiproxy:20000 lb--api-0→api-0:8080 lb--api-1→api-1:8080" {
		t.Errorf("proxies: %s", got)
	}
	cfg := p.Service("api").Synthetic[1]
	if cfg.Replica != 1 || cfg.Targets["db"].Addrs[0] != "lore-toxiproxy:20000" || cfg.Targets["cache"].Addrs[0] != "cache:6379" || cfg.Targets["cache"].Kind != "redis" {
		t.Errorf("replica 1 config: %+v", cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("synthetic config invalid: %v", err)
	}
	if got := p.Faults[0].Proxies; len(got) != 1 || got[0] != "api--db" {
		t.Errorf("latency fault proxies: %v", got)
	}
	byInject := map[string]*Fault{}
	for _, f := range p.Faults {
		byInject[f.Inject] = f
	}
	if got := byInject["kill"].Containers; len(got) != 1 || got[0] != "api-1" {
		t.Errorf("kill fault containers: %v", got)
	}
	if c := byInject["partition"].Cuts; len(c) != 1 || c[0].Container != "api-0" || len(c[0].Hosts) != 1 || c[0].Hosts[0] != "cache" || len(c[0].Ports) != 0 {
		t.Errorf("partition cuts: %+v", c)
	}
	if f := p.Faults[1]; f.Inject != "loss" || f.Port != RedisPort || len(f.Containers) != 1 || f.Containers[0] != "cache" {
		t.Errorf("loss fault: %+v", f)
	}
	if got := strings.Join(p.Workload.URLs, " "); got != "http://lb:8080" {
		t.Errorf("load urls: %s", got)
	}
}

const profileDesign = `
version: 1
name: p
services:
  api: { image: loadlore/synthetic }
workload:
  entry: api
  profile:
    - ramp: { from: 100, to: 500, over: 1m }
    - spike: { to: 2000, for: 10s }
    - hold: { rps: 300, for: 30s }
`

func TestStagesAndOfferedRate(t *testing.T) {
	p, err := Build(load(t, profileDesign), Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	w := p.Workload
	var got []string
	for _, s := range w.Stages {
		got = append(got, fmtF(s.Target)+"/"+s.Duration.D().String())
	}
	if strings.Join(got, " ") != "500/1m0s 2000/0s 2000/10s 500/0s 300/0s 300/30s" || w.StartRate != 100 || w.PeakRate != 2000 {
		t.Errorf("start %v, stages %v, peak %v", w.StartRate, got, w.PeakRate)
	}
	for _, c := range []struct {
		t    time.Duration
		want float64
	}{{0, 100}, {30 * time.Second, 300}, {65 * time.Second, 2000}, {75 * time.Second, 300}, {100 * time.Second, 0}, {-time.Second, 0}} {
		if r := w.OfferedRate(c.t); r != c.want {
			t.Errorf("offered at %v: %v, want %v", c.t, r, c.want)
		}
	}
	if w.Duration() != 100*time.Second {
		t.Errorf("duration %v", w.Duration())
	}
}

func TestScaleSetting(t *testing.T) {
	for in, want := range map[string]string{
		"256MB": "131072kB", "1GB": "524288kB", "64kB": "64kB", "200mb": "102400kb", "1g": "524288kb",
	} {
		if got, ok := scaleSetting(in, 0.5); !ok || got != want {
			t.Errorf("scaleSetting(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := scaleSetting("16384", 0.5); ok {
		t.Error("a value without a unit was scaled")
	}
}

func TestInfraBudgets(t *testing.T) {
	p, err := Build(loadFile(t, "../../examples/single-postgres.yaml"), Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	// 3,500 rps at 2,500 rps per k6 CPU, rounded up to a half core (1.5,
	// the floor). No fault acts on a connection, so there is no Toxiproxy.
	if p.Infra.LoadCPU != 1.5 || p.Infra.ExpectedOps != 0 || p.Infra.ProxyCPU != 0 || p.UsesToxiproxy() {
		t.Errorf("infra %+v", p.Infra)
	}
	want := 0.0
	for _, s := range p.Services {
		want += s.CPU * float64(len(s.Containers))
	}
	want += 1.5 + 0.1 + 0.1 + 0.1 // k6, Prometheus, cAdvisor, postgres_exporter
	if got := p.TotalCPU(); math.Abs(got-want) > 1e-9 || got > 8 {
		t.Errorf("total CPU %v, want %v and at most 8 (the spec: it fits an 8-vCPU machine at scale 1)", got, want)
	}

	// A latency fault on db proxies api → db: 3,500 × (3 reads + 0.1
	// writes) = 10,850 calls a second, at 15,000 per Toxiproxy CPU and
	// sized for 65% of the budget: 1.11, rounded up to a half core.
	src, err := os.ReadFile("../../examples/single-postgres.yaml")
	if err != nil {
		t.Fatal(err)
	}
	p, err = Build(load(t, string(src)+"\nscenario:\n  - { at: 1m, inject: latency, target: db, add_ms: 20, for: 30s }\n"), Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Infra.ExpectedOps != 10850 || p.Infra.ProxyCPU != 1.5 || len(p.Proxied()) != 1 {
		t.Errorf("with a fault: infra %+v", p.Infra)
	}
}

const lbDesign = `
version: 1
name: lb
services:
  edge: { kind: lb, target: web, algorithm: least-conn, health_check: { interval_ms: 500, fall: 2 } }
  web: { image: loadlore/synthetic, replicas: 2, behavior: { calls: [ { target: inner, op: http, count: 2 } ] } }
  inner-lb: { kind: lb, target: worker }
  inner: { image: loadlore/synthetic, behavior: { calls: [ { target: inner-lb, op: http } ] } }
  worker: { image: loadlore/synthetic, replicas: 3 }
workload:
  entry: edge
  profile: [ { hold: { rps: 1000, for: 1m } } ]
`

func TestLoadBalancers(t *testing.T) {
	p, err := Build(load(t, lbDesign), Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	edge := p.Service("edge")
	if edge.Kind != "lb" || edge.Image != pinnedHAProxy || edge.CPU != design.DefaultLBCPU || edge.Memory != 256<<20 {
		t.Errorf("edge: %+v", edge)
	}
	lb := edge.LB
	var servers []string
	for _, s := range lb.Servers {
		servers = append(servers, s.Name+"@"+s.Addr)
	}
	if lb.Algorithm != "least-conn" || lb.IntervalMs != 500 || lb.Fall != 2 || lb.Rise != 2 || lb.CheckPath != "/healthz" || lb.ServerPort != AppPort {
		t.Errorf("lb settings: %+v", lb)
	}
	if got := strings.Join(servers, " "); got != "web-0@web-0:8080 web-1@web-1:8080" {
		t.Errorf("servers: %s", got)
	}
	if got := strings.Join(p.Workload.URLs, " "); got != "http://edge:8080" {
		t.Errorf("load urls: %s", got)
	}
	// inner calls inner-lb like any HTTP service.
	tgt := p.Service("inner").Synthetic[0].Targets["inner-lb"]
	if tgt.Kind != "http" || len(tgt.Addrs) != 1 {
		t.Errorf("inner → inner-lb: %+v", tgt)
	}
	if n := len(p.Service("inner-lb").LB.Servers); n != 3 {
		t.Errorf("inner-lb has %d servers", n)
	}
	if p.UsesToxiproxy() {
		t.Errorf("proxied with no fault: %+v", p.Proxied())
	}
	// A latency fault on worker[0] proxies inner-lb's connection to it: a
	// third of inner-lb's 2,000 calls a second.
	pf, err := Build(load(t, lbDesign+"scenario:\n  - { at: 10s, inject: latency, target: \"worker[0]\", add_ms: 20, for: 10s }\n"), Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if got := pf.Proxied(); len(got) != 1 || got[0].Name != "inner-lb--worker-0" || math.Abs(pf.Infra.ExpectedOps-2000.0/3) > 1e-9 {
		t.Errorf("proxied %+v, ops %v", got, pf.Infra.ExpectedOps)
	}
	for _, a := range p.Assumptions {
		if strings.Contains(a.Field, "replicas") {
			t.Errorf("replicas behind an lb reported as spread client-side: %+v", a)
		}
	}
}

func TestClientSideSpreadIsAnAssumption(t *testing.T) {
	src := strings.Replace(lbDesign, "entry: edge", "entry: web", 1)
	p, err := Build(load(t, src), Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range p.Assumptions {
		found = found || (a.Field == "services.web.replicas" && strings.Contains(a.Why, "no health checks"))
	}
	if !found {
		t.Errorf("assumptions: %+v", p.Assumptions)
	}
}

func TestRedisMemoryDefaults(t *testing.T) {
	d := loadFile(t, "../../examples/smoke.yaml") // cache: 128m
	p, err := Build(d, Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := p.Service("cache").Config
	if cfg["maxmemory"] != "98304kb" || cfg["maxmemory-policy"] != "allkeys-lru" {
		t.Errorf("defaults: %v", cfg)
	}
	var fields []string
	for _, a := range p.Assumptions {
		fields = append(fields, a.Field+"="+a.Value)
	}
	if got := strings.Join(fields, " "); !strings.Contains(got, "services.cache.config.maxmemory=96m") || !strings.Contains(got, "services.cache.config.maxmemory-policy=allkeys-lru") {
		t.Errorf("assumptions: %s", got)
	}

	// Scaled with the container.
	half, _ := Build(d, Options{RunID: "r", Scale: 0.5})
	if got := half.Service("cache").Config["maxmemory"]; got != "49152kb" {
		t.Errorf("at scale 0.5: %s", got)
	}

	// Set in the design: kept, and not an assumption. 0 means stock Redis.
	raw, _ := os.ReadFile("../../examples/smoke.yaml")
	src := strings.Replace(string(raw), "    kind: redis\n", "    kind: redis\n    config: { maxmemory: 0, maxmemory-policy: noeviction }\n", 1)
	p, _ = Build(load(t, src), Options{RunID: "r"})
	if cfg := p.Service("cache").Config; cfg["maxmemory"] != "0" || cfg["maxmemory-policy"] != "noeviction" {
		t.Errorf("explicit: %v", cfg)
	}
	for _, a := range p.Assumptions {
		if strings.Contains(a.Field, "cache") {
			t.Errorf("explicit setting reported as assumption: %+v", a)
		}
	}
}

func TestClientTimeout(t *testing.T) {
	raw, _ := os.ReadFile("../../examples/smoke.yaml")
	src := strings.Replace(string(raw), "workload:\n", "workload:\n  timeout_ms: 2500\n", 1)
	p, err := Build(load(t, src), Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Workload.Timeout.D(); got != 2500*time.Millisecond {
		t.Errorf("timeout %v", got)
	}
	for _, a := range p.Assumptions {
		if a.Field == "workload.timeout_ms" {
			if a.Value != "2.5s" || !strings.HasPrefix(a.Why, "set in the design") {
				t.Errorf("assumption %+v", a)
			}
			return
		}
	}
	t.Error("the client timeout isn't an assumption")
}

func TestCPUPeriod(t *testing.T) {
	raw, _ := os.ReadFile("../../examples/smoke.yaml")
	src := strings.Replace(string(raw), "    resources: { cpu: 1, memory: 512m }", "    resources: { cpu: 1, memory: 512m, cpu_period_ms: 10 }", 1)
	if src == string(raw) {
		t.Fatal("smoke.yaml changed; update the test")
	}
	p, err := Build(load(t, src), Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range p.Services {
		if want := map[bool]int{true: 10, false: DefaultCPUPeriodMs}[s.Name == "db"]; s.CPUPeriod != want {
			t.Errorf("%s: period %d, want %d", s.Name, s.CPUPeriod, want)
		}
	}
	for _, a := range p.Assumptions {
		if a.Field == "resources.cpu_period_ms" && a.Value != "100 ms for api, cache, lb" {
			t.Errorf("assumption %+v", a)
		}
	}
}

// Scaling shrinks memory but not connection counts, so memory stays above
// what the connections need, and never above the design's own value.
func TestConnectionMemory(t *testing.T) {
	d := loadFile(t, "../../examples/single-postgres.yaml") // db 1g, shared_buffers 256MB, 100 connections; lb 256m
	p, err := Build(d, Options{RunID: "r", Scale: 0.25})
	if err != nil {
		t.Fatal(err)
	}
	// 64MB of shared_buffers after scaling + 100 x 2 MiB + 64 MiB.
	if got := p.Service("db").Memory; got != 328<<20 {
		t.Errorf("db: %s", sizeStr(got))
	}
	// 1,750 VUs (peak 875 rps) x 40 KiB + 32 MiB, rounded up.
	if got := p.Service("lb").Memory; got != 101<<20 {
		t.Errorf("lb: %s", sizeStr(got))
	}
	if notes := strings.Join(p.Notes, "\n"); !strings.Contains(notes, "db: memory kept at 328m instead of 256m: shared_buffers 64m plus about 2m for each of 100 connections") {
		t.Errorf("notes:\n%s", notes)
	}
	// Tiny scales stop at the design's value.
	p, _ = Build(d, Options{RunID: "r", Scale: 0.05})
	if got := p.Service("lb").Memory; got > 256<<20 {
		t.Errorf("lb above the design: %s", sizeStr(got))
	}
}

func TestPgBouncerPlan(t *testing.T) {
	p, err := Build(loadFile(t, "../../examples/pgbouncer.yaml"), Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	b := p.Service("pgb")
	if b.Kind != "pgbouncer" || b.Port != PgBouncerPort || b.PgBouncer == nil {
		t.Fatalf("pgb %+v", b)
	}
	// PgBouncer reaches db through its own proxy, so db's faults reach it.
	if x := p.proxiesInto("pgb", "db"); len(x) != 1 || b.PgBouncer.Upstream != x[0].Addr() || b.PgBouncer.DefaultPoolSize != 20 || b.PgBouncer.MaxClientConn != 200 || b.PgBouncer.PoolMode != "transaction" {
		t.Errorf("pgbouncer %+v, proxies %v", b.PgBouncer, x)
	}
	tgt := p.Service("api").Synthetic[0].Targets["pgb"]
	if tgt.Kind != "postgres" || tgt.SimpleProtocol || tgt.Rows != 100000 || tgt.Pool != 30 || len(tgt.Addrs) != 1 {
		t.Errorf("api's pgb target %+v", tgt)
	}
	var fields []string
	for _, a := range p.Assumptions {
		fields = append(fields, a.Field)
	}
	for _, want := range []string{"services.pgb.pool_mode", "services.pgb.default_pool_size", "services.pgb.config.max_client_conn", "services.pgb.config.server_login_retry", "services.api.pools.pgb.min", "services.api.pools.pgb.hold"} {
		if !slices.Contains(fields, want) {
			t.Errorf("assumptions lack %s: %v", want, fields)
		}
	}
	if slices.ContainsFunc(p.Notes, func(n string) bool { return strings.Contains(n, "max_client_conn") }) {
		t.Errorf("121 client connections fit 200: %v", p.Notes)
	}

	// At PgBouncer's default of 100, 4 x 30 + 1 don't fit.
	d := loadFile(t, "../../examples/pgbouncer.yaml")
	delete(d.Services["pgb"].Config, "max_client_conn")
	p, err = Build(d, Options{RunID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(p.Notes, "pgb: its callers' pools can open 121 client connections, the exporter's included, against max_client_conn 100; PgBouncer refuses the ones beyond it") {
		t.Errorf("notes %v", p.Notes)
	}
}

// A scenario's measured prealloc_vus raises the VUs started up front when
// it's more than peak rate x SLO, scaled with the load.
func TestMeasuredPreallocVUs(t *testing.T) {
	d := loadFile(t, "../../examples/smoke.yaml") // peak 300 rps, SLO 500 ms: 150
	for _, tc := range []struct {
		prealloc int
		scale    float64
		pre, max int
	}{
		{0, 1, 150, 600},
		{100, 1, 150, 600},   // under peak x SLO: no change
		{900, 1, 900, 900},   // measured, and max follows
		{900, 0.5, 450, 450}, // scaled with the load
	} {
		p, err := Build(d, Options{RunID: "r", Scale: tc.scale, PreallocVUs: tc.prealloc})
		if err != nil {
			t.Fatal(err)
		}
		if p.Workload.PreVUs != tc.pre || p.Workload.MaxVUs != tc.max {
			t.Errorf("prealloc %d at scale %g: %d up front, max %d; want %d, %d", tc.prealloc, tc.scale, p.Workload.PreVUs, p.Workload.MaxVUs, tc.pre, tc.max)
		}
	}
}

// A loss fault acts on the target's containers, on replies from its
// service port.
func TestLossFault(t *testing.T) {
	d := loadFile(t, "../../examples/smoke.yaml")
	pct := 2.0
	d.Scenario = append(d.Scenario, &design.Fault{At: d.Scenario[0].At, Inject: "loss", Target: &design.TargetRef{Name: "db"}, Percent: &pct})
	p, err := Build(d, Options{RunID: "r", Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	f := p.Faults[len(p.Faults)-1]
	if f.Inject != "loss" || f.Percent != 2 || f.Port != PostgresPort || len(f.Containers) != 1 || len(f.Proxies) != 0 {
		t.Errorf("loss fault %+v", f)
	}
}

// A partition between one replica and a service cuts only that replica,
// from the proxies that lead to the other side.
func TestPartitionFault(t *testing.T) {
	d := loadFile(t, "../../examples/smoke.yaml")
	d.Scenario = append(d.Scenario, &design.Fault{At: d.Scenario[0].At, Inject: "partition", Between: []string{"api[1]", "db"}})
	p, err := Build(d, Options{RunID: "r", Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	f := p.Faults[len(p.Faults)-1]
	var want []int
	for _, x := range p.Proxies {
		if x.From == "api" && x.To == "db" {
			want = append(want, x.Listen)
		}
	}
	if f.Inject != "partition" || len(f.Cuts) != 1 || f.Cuts[0].Container != "api-1" || !slices.Equal(f.Cuts[0].Ports, want) {
		t.Errorf("partition cuts %+v, want api-1 to ports %v", f.Cuts, want)
	}
	// Two services that don't call each other can't be cut apart.
	d.Scenario[len(d.Scenario)-1].Between = []string{"lb", "db"}
	if _, err := Build(d, Options{RunID: "r", Scale: 1}); err == nil || !strings.Contains(err.Error(), "don't call each other") {
		t.Errorf("lb and db: %v", err)
	}
}

// disk_slow puts its postgres's data on a slow-disk device, and only a
// postgres's: redis here keeps nothing on disk.
func TestDiskSlowFault(t *testing.T) {
	d := loadFile(t, "../../examples/smoke.yaml")
	ms := 20
	d.Scenario = append(d.Scenario, &design.Fault{At: d.Scenario[0].At, Inject: "disk_slow", Target: &design.TargetRef{Name: "db"}, AddMs: &ms})
	p, err := Build(d, Options{RunID: "r", Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	if f := p.Faults[len(p.Faults)-1]; f.Inject != "disk_slow" || f.Target != "db" || f.AddMs != 20 || !p.Service("db").SlowDisk || p.Service("cache").SlowDisk {
		t.Errorf("disk_slow fault %+v, db slow disk %v", f, p.Service("db").SlowDisk)
	}
	d.Scenario[len(d.Scenario)-1].Target = &design.TargetRef{Name: "cache"}
	if _, err := Build(d, Options{RunID: "r", Scale: 1}); err == nil || !strings.Contains(err.Error(), "slow a postgres's disk only") {
		t.Errorf("disk_slow on redis: %v", err)
	}
}

// A disk block puts the postgres on a dm-delay device with its defaults,
// scales its IOPS (not its wait) and says so; without one, the assumption
// says reads that miss memory go to the Docker host's disk.
func TestDisk(t *testing.T) {
	d := loadFile(t, "../../examples/smoke.yaml")
	p, err := Build(d, Options{RunID: "r", Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	if db := p.Service("db"); db.Disk != nil {
		t.Errorf("no disk block: disk %+v", db.Disk)
	}
	if a := findAssumption(p, "services.db.disk"); a == nil || a.Value != "laptop: the Docker host's" {
		t.Errorf("no disk block: assumption %+v", a)
	}
	read := 4
	d.Services["db"].Disk = &design.Disk{ReadMs: &read}
	p, err = Build(d, Options{RunID: "r", Scale: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	db := p.Service("db")
	if db.Disk == nil || *db.Disk != (Disk{ReadMs: 4, IOPS: 1500}) || !db.SlowDisk {
		t.Fatalf("disk %+v, slow disk %v", db.Disk, db.SlowDisk)
	}
	if db.Config["track_io_timing"] != "on" {
		t.Errorf("track_io_timing %q with a disk block", db.Config["track_io_timing"])
	}
	a := findAssumption(p, "services.db.disk")
	if a == nil || a.Value != "no profile: 4 ms per read, 1500 IOPS" || !strings.Contains(a.Why, "default") || !strings.Contains(a.Why, "the wait not") {
		t.Errorf("disk assumption %+v", a)
	}
}

// A disk profile takes its waits, IOPS and throughput from its published
// numbers, scales the caps (not the waits), and the assumption names the
// profile and its source; laptop leaves the data on the host's disk.
func TestDiskProfiles(t *testing.T) {
	d := loadFile(t, "../../examples/smoke.yaml")
	prof := func(name string, iops *int) {
		d.Services["db"].Disk = &design.Disk{Profile: &name, IOPS: iops}
	}
	prof("gp3", nil)
	p, err := Build(d, Options{RunID: "r", Scale: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	db := p.Service("db")
	if db.Disk == nil || *db.Disk != (Disk{Profile: "gp3", ReadMs: 1, WriteMs: 1, IOPS: 1500, MiBps: 62.5}) || !db.SlowDisk || db.Config["track_io_timing"] != "on" {
		t.Fatalf("gp3 at 0.5: disk %+v, slow disk %v", db.Disk, db.SlowDisk)
	}
	a := findAssumption(p, "services.db.disk")
	if a == nil || a.Value != "gp3: 1 ms per read, 1 ms per write, 1500 IOPS, 62.5 MiB/s" ||
		!strings.Contains(a.Why, "Source: AWS, Amazon EBS General Purpose SSD volumes: https://docs.aws.amazon.com/") || !strings.Contains(a.Why, "the waits not") {
		t.Errorf("gp3 assumption %+v", a)
	}

	iops := 20000
	prof("io2", &iops)
	if p, err = Build(d, Options{RunID: "r", Scale: 1}); err != nil {
		t.Fatal(err)
	}
	if db := p.Service("db"); db.Disk == nil || *db.Disk != (Disk{Profile: "io2", IOPS: 20000, MiBps: 4000}) {
		t.Errorf("io2: disk %+v", db.Disk)
	}
	if a := findAssumption(p, "services.db.disk"); a == nil || !strings.Contains(a.Why, "provisioned in the design: 20000") {
		t.Errorf("io2 assumption %+v", a)
	}

	prof("laptop", nil)
	if p, err = Build(d, Options{RunID: "r", Scale: 1}); err != nil {
		t.Fatal(err)
	}
	if db := p.Service("db"); db.Disk != nil { // smoke's disk_slow fault still makes it a slow disk
		t.Errorf("laptop: disk %+v", db.Disk)
	}
	if a := findAssumption(p, "services.db.disk"); a == nil || a.Value != "laptop" || !strings.Contains(a.Why, "Source: none") {
		t.Errorf("laptop assumption %+v", a)
	}
}

func findAssumption(p *Plan, field string) *Assumption {
	for i := range p.Assumptions {
		if p.Assumptions[i].Field == field {
			return &p.Assumptions[i]
		}
	}
	return nil
}

// A read replica is planned as a postgres with its primary's image and
// dataset, pointed at the primary, which keeps WAL for it.
func TestReadReplica(t *testing.T) {
	d := loadFile(t, "../../examples/read-replica.yaml")
	p, err := Build(d, Options{RunID: "r", Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	db, ro := p.Service("db"), p.Service("ro")
	if ro.Kind != "postgres" || ro.Primary != "db" || ro.Image != db.Image || ro.Dataset == nil || ro.Dataset.Rows != db.Dataset.Rows || ro.Dataset == db.Dataset {
		t.Fatalf("replica %+v", ro)
	}
	if want := "host=db port=5432 user=loadlore password=loadlore"; ro.Config["primary_conninfo"] != want {
		t.Errorf("primary_conninfo %q", ro.Config["primary_conninfo"])
	}
	if db.Config["wal_keep_size"] != DefaultWALKeepSize || db.Primary != "" {
		t.Errorf("primary config %v, primary %q", db.Config, db.Primary)
	}
	if rs := p.Replicas("db"); len(rs) != 1 || rs[0] != ro {
		t.Errorf("replicas of db: %v", rs)
	}
	if a := findAssumption(p, "services.ro replication"); a == nil || a.Value != "asynchronous streaming from db" {
		t.Errorf("replication assumption %+v", a)
	}
	if a := findAssumption(p, "services.db.config.wal_keep_size"); a == nil || a.Value != "1GB" {
		t.Errorf("wal_keep_size assumption %+v", a)
	}
}

// Several replicas are numbered containers of one service, each with an
// exporter in the budget.
func TestReadReplicas(t *testing.T) {
	d := loadFile(t, "../../examples/read-replica.yaml")
	one, err := Build(d, Options{RunID: "r", Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	n := 3
	d.Services["ro"].Replicas = &n
	p, err := Build(d, Options{RunID: "r", Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Service("ro").Containers; !slices.Equal(got, []string{"ro-0", "ro-1", "ro-2"}) {
		t.Errorf("containers %v", got)
	}
	if got := len(p.proxiesInto("api", "ro")); got != 3 {
		t.Errorf("%d proxies from api to ro, want one per replica", got)
	}
	ro := p.Service("ro")
	want := one.TotalCPU() + 2*(ro.CPU+p.Infra.ExporterCPU)
	if got := p.TotalCPU(); math.Abs(got-want) > 0.011 {
		t.Errorf("total CPU %v, want %v", got, want)
	}
}

// A replica's disk block is its own: each of its containers gets a slow
// disk, and its reads are timed.
func TestReplicaDisk(t *testing.T) {
	d := loadFile(t, "../../examples/read-replica.yaml")
	iops, n := 1000, 2
	d.Services["ro"].Disk = &design.Disk{IOPS: &iops}
	d.Services["ro"].Replicas = &n
	p, err := Build(d, Options{RunID: "r", Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	ro, db := p.Service("ro"), p.Service("db")
	if ro.Disk == nil || *ro.Disk != (Disk{ReadMs: 1, IOPS: 1000}) || !ro.SlowDisk || ro.Config["track_io_timing"] != "on" {
		t.Errorf("replica disk %+v, slow %v, config %v", ro.Disk, ro.SlowDisk, ro.Config)
	}
	if db.Disk != nil || db.SlowDisk {
		t.Errorf("the primary got the replica's disk: %+v", db.Disk)
	}
}

// A queue runs like a Redis that never evicts; a worker's job runners
// each get a connection to it, and jobs carry the request's payload.
func TestQueue(t *testing.T) {
	d := loadFile(t, "../../examples/queue.yaml")
	p, err := Build(d, Options{RunID: "r", Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	q, w, api := p.Service("emails"), p.Service("sender"), p.Service("api")
	if q.Kind != "queue" || q.Port != RedisPort || q.Image != pinnedRedis8 || q.Config["maxmemory-policy"] != "noeviction" || q.Config["maxmemory"] == "" {
		t.Fatalf("queue %+v", q)
	}
	if a := findAssumption(p, "services.emails.config.maxmemory-policy"); a == nil || a.Value != "noeviction" {
		t.Errorf("policy assumption %+v", a)
	}
	if w.Role != "worker" || w.Consume == nil || *w.Consume != (Consume{Queue: "emails", Concurrency: 5}) || len(w.Synthetic) != 2 {
		t.Fatalf("worker %+v", w)
	}
	for i, cfg := range w.Synthetic {
		if cfg.Role != "worker" || cfg.Consume == nil || cfg.Consume.Target != "emails" || cfg.Consume.Concurrency != 5 || cfg.Consume.Consumer != w.Containers[i] {
			t.Errorf("worker config %d: role %q consume %+v", i, cfg.Role, cfg.Consume)
		}
		qt := cfg.Targets["emails"]
		if qt.Kind != "queue" || qt.Pool != 6 || qt.Stream != "emails" || qt.Group != QueueGroup || qt.RowBytes != 1024 || len(qt.Addrs) != 1 {
			t.Errorf("worker's queue target %+v", qt)
		}
		if err := cfg.Validate(); err != nil {
			t.Error(err)
		}
	}
	if len(p.proxiesInto("sender", "emails")) != 1 {
		t.Errorf("proxies from sender to emails: %v", p.proxiesInto("sender", "emails"))
	}
	at := api.Synthetic[0].Targets["emails"]
	if at.Kind != "queue" || at.Pool != 10 || at.Stream != "emails" || api.Synthetic[0].Role != "" || api.Role != "" {
		t.Errorf("api's queue target %+v, role %q", at, api.Synthetic[0].Role)
	}
	if a := findAssumption(p, "services.sender.consume.concurrency"); a == nil || a.Value != "5" || !strings.Contains(a.Why, "Sidekiq") {
		t.Errorf("concurrency assumption %+v", a)
	}
	if exporters(q) != 1 {
		t.Errorf("exporters for a queue: %d", exporters(q))
	}
}
