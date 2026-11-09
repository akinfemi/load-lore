package plan

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
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
	if api.CPU != 0.5 || api.Memory != 256<<20 || len(api.Containers) != 4 {
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
	if got := strings.Join(names, " "); got != "api--cache→cache:6379 api--db→db:5432" {
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
	if got := strings.Join(p.Workload.URLs, " "); got != "http://api-0:8080 http://api-1:8080" {
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
	// 5,000 rps at 2,500 rps per k6 CPU; 5,000 × (3 reads + 0.1 writes)
	// = 15,500 proxied calls at 12,000 per Toxiproxy CPU, rounded up.
	if p.Infra.LoadCPU != 2 || p.Infra.ExpectedOps != 15500 || p.Infra.ProxyCPU != 1.5 {
		t.Errorf("infra %+v", p.Infra)
	}
	if got := p.TotalCPU(); got != 10.25 {
		t.Errorf("total CPU %v", got)
	}
}
