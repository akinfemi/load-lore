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
	var names []string
	for _, x := range p.Proxies {
		names = append(names, x.Name+"→"+x.Upstream)
	}
	if got := strings.Join(names, " "); got != "api--cache→cache:6379 api--db→db:5432 lb--api-0→api-0:8080 lb--api-1→api-1:8080" {
		t.Errorf("proxies: %s", got)
	}
	cfg := p.Service("api").Synthetic[1]
	if cfg.Replica != 1 || cfg.Targets["db"].Addrs[0] != "lore-toxiproxy:20001" || cfg.Targets["cache"].Kind != "redis" {
		t.Errorf("replica 1 config: %+v", cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("synthetic config invalid: %v", err)
	}
	if got := p.Faults[0].Proxies; len(got) != 1 || got[0] != "api--db" {
		t.Errorf("latency fault proxies: %v", got)
	}
	if got := p.Faults[1].Containers; len(got) != 1 || got[0] != "api-1" {
		t.Errorf("kill fault containers: %v", got)
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

func TestUnsupportedFaults(t *testing.T) {
	src := strings.Replace(profileDesign, "workload:", `  db: { kind: postgres, dataset: { rows: 10 } }
scenario:
  - { at: 1s, inject: partition, between: [api, db] }
workload:`, 1)
	if _, err := Build(load(t, src), Options{RunID: "r"}); err == nil || !strings.Contains(err.Error(), "partition isn't supported by local runs yet") {
		t.Errorf("got %v", err)
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
	// 5,000 rps at 2,500 rps per k6 CPU, rounded up to a half core.
	// Proxied calls: 5,000 lb → api, plus 5,000 × (3 reads + 0.1 writes)
	// api → db = 20,500, at 15,000 per Toxiproxy CPU, rounded up.
	if p.Infra.LoadCPU != 2 || p.Infra.ExpectedOps != 20500 || p.Infra.ProxyCPU != 1.5 {
		t.Errorf("infra %+v", p.Infra)
	}
	want := 0.0
	for _, s := range p.Services {
		want += s.CPU * float64(len(s.Containers))
	}
	want += 2 + 1.5 + 0.1 + 0.1 + 0.1 // k6, Toxiproxy, Prometheus, cAdvisor, postgres_exporter
	if got := p.TotalCPU(); math.Abs(got-want) > 1e-9 || got > 8 {
		t.Errorf("total CPU %v, want %v and at most 8 (the spec: it fits an 8-vCPU machine at scale 1)", got, want)
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
	if got := strings.Join(servers, " "); got != "web-0@lore-toxiproxy:20000 web-1@lore-toxiproxy:20001" {
		t.Errorf("servers: %s", got)
	}
	if got := strings.Join(p.Workload.URLs, " "); got != "http://edge:8080" {
		t.Errorf("load urls: %s", got)
	}
	// inner calls inner-lb like any HTTP service, through its proxy.
	tgt := p.Service("inner").Synthetic[0].Targets["inner-lb"]
	if tgt.Kind != "http" || len(tgt.Addrs) != 1 {
		t.Errorf("inner → inner-lb: %+v", tgt)
	}
	if n := len(p.Service("inner-lb").LB.Servers); n != 3 {
		t.Errorf("inner-lb has %d servers", n)
	}
	// 1,000 edge → web, 2,000 web → inner, 2,000 inner → inner-lb, 2,000
	// inner-lb → worker.
	if p.Infra.ExpectedOps != 7000 {
		t.Errorf("proxied ops %v", p.Infra.ExpectedOps)
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
	// 2,500 VUs (peak 1,250 rps) x 40 KiB + 32 MiB, rounded up.
	if got := p.Service("lb").Memory; got != 130<<20 {
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
