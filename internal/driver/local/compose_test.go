package local

import (
	"flag"
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/plan"
)

var update = flag.Bool("update", false, "rewrite golden files")

func smokePlan(t *testing.T) *plan.Plan {
	t.Helper()
	d, diags, err := design.LoadFile("../../../examples/smoke.yaml")
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	p, err := plan.Build(d, plan.Options{RunID: "test-run", Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGoldenCompose(t *testing.T) {
	p := smokePlan(t)
	got, err := buildCompose(p, stackInput{Project: "loadlore-test-run", K6Script: []byte("// script\n"), DataVols: map[string]string{"db": "loadlore-test-run-db-data"}, Synthetic: "loadlore/synthetic:dev"})
	if err != nil {
		t.Fatal(err)
	}
	const path = "testdata/smoke-compose.yaml"
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update)", err)
	}
	if string(got) != string(want) {
		t.Errorf("%s differs; run go test -update after checking the change", path)
	}

	var cf composeFile
	if err := yaml.Unmarshal(got, &cf); err != nil {
		t.Fatal(err)
	}
	for name, s := range cf.Services {
		if s.Labels[LabelOwner] != "1" || s.Labels[LabelRun] != "test-run" {
			t.Errorf("%s isn't labeled for teardown: %v", name, s.Labels)
		}
		if s.CPUs <= 0 || s.MemLimit == "" {
			t.Errorf("%s has no resource limits", name)
		}
		if s.Restart != "no" {
			t.Errorf("%s restarts on its own", name)
		}
	}
	if cf.Services["lore-k6"].Profiles[0] != "load" {
		t.Error("k6 would start with the stack")
	}
	var prom map[string]any
	if err := yaml.Unmarshal([]byte(cf.Configs["prometheus"].Content), &prom); err != nil {
		t.Errorf("prometheus.yml doesn't parse: %v", err)
	}
	if !strings.Contains(cf.Configs["toxiproxy"].Content, `"upstream": "db:5432"`) {
		t.Errorf("toxiproxy config: %s", cf.Configs["toxiproxy"].Content)
	}

	// The lb checks both replicas through their proxies, starts after
	// them, and is scraped.
	hap := cf.Configs["haproxy-lb"].Content
	for _, want := range []string{"balance roundrobin", "option httpchk GET /healthz", "default-server check inter 1000ms fall 2 rise 2",
		"server api-0 lore-toxiproxy:20002", "server api-1 lore-toxiproxy:20003", "prometheus-exporter"} {
		if !strings.Contains(hap, want) {
			t.Errorf("haproxy.cfg lacks %q:\n%s", want, hap)
		}
	}
	lb := cf.Services["lb"]
	if lb.DependsOn["api-0"].Condition != "service_healthy" || lb.DependsOn["api-1"].Condition != "service_healthy" || lb.Labels[LabelRole] != "lb" {
		t.Errorf("lb service: %+v", lb)
	}
	if !strings.Contains(cf.Configs["prometheus"].Content, `"lb:8405"`) {
		t.Errorf("lb isn't scraped:\n%s", cf.Configs["prometheus"].Content)
	}
}

func TestComposeVersion(t *testing.T) {
	for v, want := range map[string]bool{"2.23.0": true, "2.22.9": false, "5.1.1": true, "v2.30.1-desktop.1": true, "1.29.2": false, "junk": false} {
		if got := composeNewEnough(v); got != want {
			t.Errorf("%s: %v", v, got)
		}
	}
}

func TestPGDataPath(t *testing.T) {
	if pgDataPath("17") != "/var/lib/postgresql/data" || pgDataPath("18") != "/var/lib/postgresql" {
		t.Error("wrong data path")
	}
}

// A design container with its own CFS period gets a period and quota in
// place of cpus; tooling keeps Docker's default.
func TestCPUPeriod(t *testing.T) {
	p := smokePlan(t)
	for _, s := range p.Services {
		if s.Name == "db" {
			s.CPUPeriod = 10
		}
	}
	got, err := buildCompose(p, stackInput{Project: "loadlore-test-run", K6Script: []byte("// script\n"), DataVols: map[string]string{"db": "loadlore-test-run-db-data"}, Synthetic: "loadlore/synthetic:dev"})
	if err != nil {
		t.Fatal(err)
	}
	var cf composeFile
	if err := yaml.Unmarshal(got, &cf); err != nil {
		t.Fatal(err)
	}
	if db := cf.Services["db"]; db.CPUs != 0 || db.CPUPeriod != 10000 || db.CPUQuota != 10000 {
		t.Errorf("db: cpus %v, period %d, quota %d", db.CPUs, db.CPUPeriod, db.CPUQuota)
	}
	for _, name := range []string{"api-0", "lore-k6", "lore-toxiproxy"} {
		if s := cf.Services[name]; s.CPUs == 0 || s.CPUPeriod != 0 {
			t.Errorf("%s: cpus %v, period %d", name, s.CPUs, s.CPUPeriod)
		}
	}
}
