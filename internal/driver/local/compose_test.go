package local

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
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

	// The lb checks both replicas directly (no fault acts on them),
	// starts after them, and is scraped.
	hap := cf.Configs["haproxy-lb"].Content
	for _, want := range []string{"balance roundrobin", "option httpchk GET /healthz", "default-server check inter 1000ms fall 2 rise 2",
		"server api-0 api-0:8080", "server api-1 api-1:8080", "prometheus-exporter"} {
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

func TestPgBouncerCompose(t *testing.T) {
	d, diags, err := design.LoadFile("../../../examples/pgbouncer.yaml")
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	p, err := plan.Build(d, plan.Options{RunID: "test-run", Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	got, err := buildCompose(p, stackInput{Project: "loadlore-test-run", K6Script: []byte("// script\n"), DataVols: map[string]string{"db": "v"}, Synthetic: "loadlore/synthetic:dev"})
	if err != nil {
		t.Fatal(err)
	}
	var cf composeFile
	if err := yaml.Unmarshal(got, &cf); err != nil {
		t.Fatal(err)
	}
	if _, ok := cf.Services[svcToxiproxy]; ok || cf.Configs["toxiproxy"] != nil || strings.Contains(cf.Configs["prometheus"].Content, "toxiproxy") {
		t.Errorf("Toxiproxy in a run whose faults act on no connection")
	}
	pgb := cf.Services["pgb"]
	if pgb == nil || pgb.Entrypoint[0] != "/usr/bin/pgbouncer" || pgb.DependsOn["db"].Condition != "service_healthy" || pgb.Labels[LabelRole] != "pgbouncer" {
		t.Fatalf("pgb service: %+v", pgb)
	}
	// It reaches db directly: no fault acts on that connection.
	ini := cf.Configs["pgbouncer-ini-pgb"].Content
	for _, want := range []string{"[databases]\nlab = host=db port=5432 dbname=lab\n",
		"pool_mode = transaction\n", "default_pool_size = 20\n", "max_client_conn = 200\n", "admin_users = loadlore\n", "auth_type = scram-sha-256\n"} {
		if !strings.Contains(ini, want) {
			t.Errorf("pgbouncer.ini lacks %q:\n%s", want, ini)
		}
	}
	if users := cf.Configs["pgbouncer-userlist-pgb"].Content; users != "\"loadlore\" \"loadlore\"\n" {
		t.Errorf("userlist.txt %q", users)
	}
	if exp := cf.Services["lore-pgbexp-pgb"]; exp == nil || !strings.Contains(exp.Command[0], "@pgb:6432/pgbouncer") {
		t.Errorf("exporter %+v", exp)
	}
	if !strings.Contains(cf.Configs["prometheus"].Content, `"lore-pgbexp-pgb:9127"`) {
		t.Errorf("pgbouncer isn't scraped:\n%s", cf.Configs["prometheus"].Content)
	}
	if cf.Services["api-0"].DependsOn["pgb"].Condition != "service_healthy" {
		t.Errorf("api starts before pgb: %+v", cf.Services["api-0"].DependsOn)
	}
}

// A postgres with a disk block gets its IOPS cap on its dm-delay device.
func TestDiskCompose(t *testing.T) {
	d, diags, err := design.LoadFile("../../../examples/smoke.yaml")
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	d.Services["db"].Disk = &design.Disk{}
	p, err := plan.Build(d, plan.Options{RunID: "test-run", Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	got, err := buildCompose(p, stackInput{Project: "loadlore-test-run", K6Script: []byte("// script\n"), DataVols: map[string]string{"db": "v"},
		DiskDevs: map[string]string{"db": "/dev/dm-3"}, Synthetic: "loadlore/synthetic:dev"})
	if err != nil {
		t.Fatal(err)
	}
	var cf composeFile
	if err := yaml.Unmarshal(got, &cf); err != nil {
		t.Fatal(err)
	}
	want := []composeDeviceRate{{Path: "/dev/dm-3", Rate: 3000}}
	b := cf.Services["db"].BlkioConfig
	if b == nil || !slices.Equal(b.DeviceReadIOPS, want) || !slices.Equal(b.DeviceWriteIOPS, want) {
		t.Errorf("db blkio_config %+v", b)
	}
	if cf.Services["cache"].BlkioConfig != nil {
		t.Errorf("cache has a blkio_config")
	}
}

// A read replica waits for its primary and streams from it; the data copy
// marks it a standby and opens the primary to replication.
func TestReplicaCompose(t *testing.T) {
	d, diags, err := design.LoadFile("../../../examples/read-replica.yaml")
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	p, err := plan.Build(d, plan.Options{RunID: "test-run", Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	got, err := buildCompose(p, stackInput{Project: "loadlore-test-run", K6Script: []byte("// script\n"), DataVols: map[string]string{"db": "v1", "ro": "v2"}, Synthetic: "loadlore/synthetic:dev"})
	if err != nil {
		t.Fatal(err)
	}
	var cf composeFile
	if err := yaml.Unmarshal(got, &cf); err != nil {
		t.Fatal(err)
	}
	ro := cf.Services["ro"]
	if ro == nil || ro.DependsOn["db"].Condition != "service_healthy" || !slices.Contains(ro.Command, "primary_conninfo=host=db port=5432 user=loadlore password=loadlore application_name=ro") {
		t.Fatalf("ro service: %+v", ro)
	}
	if cf.Services["lore-pgexp-ro"] == nil {
		t.Error("the replica has no exporter")
	}
	if !slices.Contains(cf.Services["db"].Command, "wal_keep_size=1GB") {
		t.Errorf("db command %v", cf.Services["db"].Command)
	}
	if s := replicationSetup(p, p.Service("ro")); !strings.Contains(s, "touch /to/standby.signal") {
		t.Errorf("replica setup %q", s)
	}
	if s := replicationSetup(p, p.Service("db")); !strings.Contains(s, "host replication all all scram-sha-256") {
		t.Errorf("primary setup %q", s)
	}
	if s := replicationSetup(p, p.Service("api")); s != "" {
		t.Errorf("app setup %q", s)
	}
}

// Several read replicas are containers of one service: each has its own
// data, exporter and scrape job, and names itself to the primary.
func TestReplicasCompose(t *testing.T) {
	d, diags, err := design.LoadFile("../../../examples/read-replica.yaml")
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	n := 2
	d.Services["ro"].Replicas = &n
	p, err := plan.Build(d, plan.Options{RunID: "test-run", Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	got, err := buildCompose(p, stackInput{Project: "loadlore-test-run", K6Script: []byte("// script\n"), DataVols: map[string]string{"db": "v1", "ro-0": "v2", "ro-1": "v3"}, Synthetic: "loadlore/synthetic:dev"})
	if err != nil {
		t.Fatal(err)
	}
	var cf composeFile
	if err := yaml.Unmarshal(got, &cf); err != nil {
		t.Fatal(err)
	}
	if cf.Services["ro"] != nil {
		t.Error("a container named after the service")
	}
	for i, c := range []string{"ro-0", "ro-1"} {
		s := cf.Services[c]
		if s == nil || s.Labels[LabelService] != "ro" || s.Labels[LabelReplica] != strconv.Itoa(i) || s.DependsOn["db"].Condition != "service_healthy" {
			t.Fatalf("%s: %+v", c, s)
		}
		if !slices.Contains(s.Command, "primary_conninfo=host=db port=5432 user=loadlore password=loadlore application_name="+c) {
			t.Errorf("%s command %v", c, s.Command)
		}
		if vol := cf.Volumes["data-"+c]; vol == nil || vol.Name != map[string]string{"ro-0": "v2", "ro-1": "v3"}[c] {
			t.Errorf("%s volume %+v", c, vol)
		}
		if exp := cf.Services["lore-pgexp-"+c]; exp == nil || !strings.HasPrefix(exp.Environment["DATA_SOURCE_URI"], c+":5432/") {
			t.Errorf("%s exporter %+v", c, exp)
		}
		job := fmt.Sprintf("job_name: postgres-%s\n    static_configs:\n      - targets: [\"lore-pgexp-%s:9187\"]\n        labels: {service: \"ro\", replica: \"%d\"}", c, c, i)
		if !strings.Contains(cf.Configs["prometheus"].Content, job) {
			t.Errorf("no scrape job for %s:\n%s", c, cf.Configs["prometheus"].Content)
		}
	}
}

// An autoscaled service has a container per replica it may run; the ones it
// doesn't start with are in the scale profile and start disabled on the
// lb, whose runtime API is published.
func TestAutoscaleCompose(t *testing.T) {
	d, diags, err := design.LoadFile("../../../examples/autoscale.yaml")
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	p, err := plan.Build(d, plan.Options{RunID: "test-run", Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	got, err := buildCompose(p, stackInput{Project: "loadlore-test-run", K6Script: []byte("// script\n"), DataVols: map[string]string{"db": "v"}, Synthetic: "loadlore/synthetic:dev"})
	if err != nil {
		t.Fatal(err)
	}
	var cf composeFile
	if err := yaml.Unmarshal(got, &cf); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		c := cf.Services[fmt.Sprintf("api-%d", i)]
		if c == nil {
			t.Fatalf("no api-%d", i)
		}
		if scaled := slices.Contains(c.Profiles, profileScale); scaled != (i >= 2) {
			t.Errorf("api-%d profiles %v", i, c.Profiles)
		}
	}
	lb := cf.Services["lb"]
	if _, ok := lb.DependsOn["api-2"]; ok || lb.DependsOn["api-1"].Condition != "service_healthy" {
		t.Errorf("lb depends on %v", lb.DependsOn)
	}
	if !slices.Contains(lb.Ports, "127.0.0.1::9999") {
		t.Errorf("lb ports %v", lb.Ports)
	}
	cfg := cf.Configs["haproxy-lb"].Content
	for _, want := range []string{"stats socket ipv4@0.0.0.0:9999 level admin", "server api-1 api-1:8080", "server api-4 api-4:8080"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("haproxy.cfg lacks %q:\n%s", want, cfg)
		}
	}
	// A replica that isn't running has no address yet: HAProxy has to start
	// without resolving it.
	if !regexp.MustCompile(`server api-4 \S+ disabled init-addr last,libc,127\.0\.0\.1\n`).MatchString(cfg) || regexp.MustCompile(`server api-1 \S+ disabled`).MatchString(cfg) {
		t.Errorf("disabled servers wrong:\n%s", cfg)
	}
}

// A queue is a Redis that keeps every job, with an exporter watching its
// stream; workers are synthetic containers like any app service.
func TestQueueCompose(t *testing.T) {
	d, diags, err := design.LoadFile("../../../examples/queue.yaml")
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	p, err := plan.Build(d, plan.Options{RunID: "test-run", Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	got, err := buildCompose(p, stackInput{Project: "loadlore-test-run", K6Script: []byte("// script\n"), DataVols: map[string]string{"db": "v1"}, Synthetic: "loadlore/synthetic:dev"})
	if err != nil {
		t.Fatal(err)
	}
	var cf composeFile
	if err := yaml.Unmarshal(got, &cf); err != nil {
		t.Fatal(err)
	}
	q := cf.Services["emails"]
	if q == nil || q.Image != "redis:8.2" || !slices.Contains(q.Command, "noeviction") || q.Healthcheck == nil {
		t.Fatalf("queue service: %+v", q)
	}
	exp := cf.Services["lore-redisexp-emails"]
	if exp == nil || exp.Environment["REDIS_EXPORTER_CHECK_SINGLE_STREAMS"] != "emails" {
		t.Fatalf("queue exporter: %+v", exp)
	}
	if cf.Services["sender-0"] == nil || cf.Services["sender-1"] == nil {
		t.Error("worker containers missing")
	}
	if !strings.Contains(prometheusConfig(p), "lore-redisexp-emails:9121") {
		t.Error("the queue's exporter isn't scraped")
	}
}
