package local

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/akinfemi/load-lore/internal/plan"
)

// Pinned images for Loadlore's own containers.
const (
	ImageToxiproxy   = "ghcr.io/shopify/toxiproxy:2.13.0"
	ImagePrometheus  = "prom/prometheus:v3.16.0"
	ImageCAdvisor    = "ghcr.io/google/cadvisor:v0.60.6"
	ImagePGExporter  = "quay.io/prometheuscommunity/postgres-exporter:v0.20.1"
	ImageRedisExport = "oliver006/redis_exporter:v1.97.0"
	ImageK6          = "grafana/k6:2.3.0"
	ImageAlpine      = "alpine:3.20"
)

// Names of Loadlore's own services in the stack. The lore- prefix keeps
// them apart from design service names.
const (
	svcToxiproxy  = plan.ProxyHost
	svcPrometheus = "lore-prometheus"
	svcCAdvisor   = "lore-cadvisor"
	svcLoad       = "lore-k6"
	networkName   = "lab"
	toxiproxyAPI  = 8474
	promPort      = 9090
	k6ScriptPath  = "/scripts/load.js"
)

type composeFile struct {
	Name     string                     `yaml:"name"`
	Services map[string]*composeService `yaml:"services"`
	Networks map[string]*composeNetwork `yaml:"networks"`
	Volumes  map[string]*composeVolume  `yaml:"volumes,omitempty"`
	Configs  map[string]*composeConfig  `yaml:"configs,omitempty"`
}

type composeService struct {
	Image           string                   `yaml:"image"`
	PullPolicy      string                   `yaml:"pull_policy,omitempty"`
	Command         []string                 `yaml:"command,omitempty"`
	Environment     map[string]string        `yaml:"environment,omitempty"`
	CPUs            float64                  `yaml:"cpus,omitempty"`
	MemLimit        string                   `yaml:"mem_limit,omitempty"`
	MemswapLimit    string                   `yaml:"memswap_limit,omitempty"`
	ShmSize         string                   `yaml:"shm_size,omitempty"`
	Labels          map[string]string        `yaml:"labels"`
	Networks        []string                 `yaml:"networks"`
	Ports           []string                 `yaml:"ports,omitempty"`
	Volumes         []string                 `yaml:"volumes,omitempty"`
	Configs         []composeConfigRef       `yaml:"configs,omitempty"`
	Healthcheck     *composeHealth           `yaml:"healthcheck,omitempty"`
	DependsOn       map[string]composeDepend `yaml:"depends_on,omitempty"`
	Privileged      bool                     `yaml:"privileged,omitempty"`
	Devices         []string                 `yaml:"devices,omitempty"`
	Profiles        []string                 `yaml:"profiles,omitempty"`
	Restart         string                   `yaml:"restart"`
	StopGracePeriod string                   `yaml:"stop_grace_period,omitempty"`
}

type composeHealth struct {
	Test        []string `yaml:"test"`
	Interval    string   `yaml:"interval"`
	Timeout     string   `yaml:"timeout"`
	Retries     int      `yaml:"retries"`
	StartPeriod string   `yaml:"start_period,omitempty"`
}

type composeDepend struct {
	Condition string `yaml:"condition"`
}

type composeNetwork struct {
	Labels map[string]string `yaml:"labels"`
}

type composeVolume struct {
	External bool   `yaml:"external"`
	Name     string `yaml:"name"`
}

type composeConfig struct {
	Content string `yaml:"content"`
}

type composeConfigRef struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// stackInput is what compose generation needs besides the plan.
type stackInput struct {
	Project   string
	K6Script  []byte
	DataVols  map[string]string // postgres service -> run volume name
	Synthetic string            // image
}

// buildCompose generates the compose file for a plan.
func buildCompose(p *plan.Plan, in stackInput) ([]byte, error) {
	cf := &composeFile{
		Name:     in.Project,
		Services: map[string]*composeService{},
		Networks: map[string]*composeNetwork{networkName: {Labels: map[string]string{LabelOwner: "1", LabelRun: p.RunID}}},
		Volumes:  map[string]*composeVolume{},
		Configs:  map[string]*composeConfig{},
	}
	labels := func(service, role string, replica int) map[string]string {
		l := map[string]string{LabelOwner: "1", LabelRun: p.RunID, LabelService: service, LabelRole: role}
		if replica >= 0 {
			l[LabelReplica] = strconv.Itoa(replica)
		}
		return l
	}
	base := func(image, service, role string, replica int, cpu float64, mem int64) *composeService {
		s := &composeService{Image: image, Labels: labels(service, role, replica), Networks: []string{networkName}, Restart: "no", CPUs: cpu}
		if mem > 0 {
			s.MemLimit = memStr(mem)
			s.MemswapLimit = memStr(mem) // no swap: memory pressure shows as it would without swap
		}
		return s
	}

	healthyDeps := map[string]composeDepend{svcToxiproxy: {Condition: "service_started"}}
	for _, s := range p.Services {
		switch s.Kind {
		case "postgres":
			c := base(s.Image, s.Name, "datastore", -1, s.CPU, s.Memory)
			c.PullPolicy = "missing"
			c.Environment = map[string]string{"POSTGRES_USER": plan.DBUser, "POSTGRES_PASSWORD": plan.DBPassword, "POSTGRES_DB": plan.DBName}
			c.Command = []string{"postgres"}
			for _, k := range sortedKeys(s.Config) {
				c.Command = append(c.Command, "-c", k+"="+s.Config[k])
			}
			// Docker's 64 MB /dev/shm is too small for parallel queries.
			c.ShmSize = memStr(max(64<<20, s.Memory/4))
			vol := "data-" + s.Name
			cf.Volumes[vol] = &composeVolume{External: true, Name: in.DataVols[s.Name]}
			c.Volumes = []string{vol + ":" + pgDataPath(s.Version)}
			c.Healthcheck = &composeHealth{Test: []string{"CMD-SHELL", fmt.Sprintf("pg_isready -U %s -d %s -h 127.0.0.1", plan.DBUser, plan.DBName)}, Interval: "1s", Timeout: "3s", Retries: 180}
			c.StopGracePeriod = "10s"
			cf.Services[s.Name] = c
			healthyDeps[s.Name] = composeDepend{Condition: "service_healthy"}

			exp := base(ImagePGExporter, s.Name, "exporter", -1, p.Infra.ExporterCPU, p.Infra.ExporterMem)
			exp.Environment = map[string]string{
				"DATA_SOURCE_URI":  fmt.Sprintf("%s:%d/%s?sslmode=disable", s.Name, plan.PostgresPort, plan.DBName),
				"DATA_SOURCE_USER": plan.DBUser, "DATA_SOURCE_PASS": plan.DBPassword,
			}
			exp.DependsOn = map[string]composeDepend{s.Name: {Condition: "service_healthy"}}
			cf.Services["lore-pgexp-"+s.Name] = exp
		case "redis":
			c := base(s.Image, s.Name, "datastore", -1, s.CPU, s.Memory)
			c.PullPolicy = "missing"
			c.Command = []string{"redis-server", "--save", "", "--appendonly", "no"}
			for _, k := range sortedKeys(s.Config) {
				c.Command = append(c.Command, "--"+k, s.Config[k])
			}
			c.Healthcheck = &composeHealth{Test: []string{"CMD", "redis-cli", "ping"}, Interval: "1s", Timeout: "3s", Retries: 60}
			cf.Services[s.Name] = c
			healthyDeps[s.Name] = composeDepend{Condition: "service_healthy"}

			exp := base(ImageRedisExport, s.Name, "exporter", -1, p.Infra.ExporterCPU, p.Infra.ExporterMem)
			exp.Environment = map[string]string{"REDIS_ADDR": fmt.Sprintf("redis://%s:%d", s.Name, plan.RedisPort)}
			exp.DependsOn = map[string]composeDepend{s.Name: {Condition: "service_healthy"}}
			cf.Services["lore-redisexp-"+s.Name] = exp
		}
	}

	for _, s := range p.Services {
		if s.Kind != "app" {
			continue
		}
		for i, name := range s.Containers {
			image := s.Image
			if image == "" {
				image = in.Synthetic
			}
			c := base(image, s.Name, "app", i, s.CPU, s.Memory)
			if i < len(s.Synthetic) {
				c.PullPolicy = "never" // built locally: make image
				cfg, err := json.Marshal(s.Synthetic[i])
				if err != nil {
					return nil, err
				}
				c.Environment = map[string]string{"LOADLORE_SYNTHETIC_CONFIG": string(cfg)}
				c.Healthcheck = &composeHealth{Test: []string{"CMD", "/synthetic", "healthcheck"}, Interval: "1s", Timeout: "3s", Retries: 120}
			}
			c.DependsOn = healthyDeps
			cf.Services[name] = c
		}
	}

	// Toxiproxy: one listener per proxied connection.
	tox := base(ImageToxiproxy, svcToxiproxy, "infra", -1, p.Infra.ProxyCPU, p.Infra.ProxyMemory)
	tox.Command = []string{"-host=0.0.0.0", "-config=/config/toxiproxy.json", "-proxy-metrics", "-runtime-metrics"}
	toxCfg, err := toxiproxyConfig(p)
	if err != nil {
		return nil, err
	}
	cf.Configs["toxiproxy"] = &composeConfig{Content: string(toxCfg)}
	tox.Configs = []composeConfigRef{{Source: "toxiproxy", Target: "/config/toxiproxy.json"}}
	tox.Ports = []string{fmt.Sprintf("127.0.0.1::%d", toxiproxyAPI)}
	cf.Services[svcToxiproxy] = tox

	// Monitoring.
	prom := base(ImagePrometheus, svcPrometheus, "infra", -1, p.Infra.PromCPU, p.Infra.PromMemory)
	prom.Command = []string{
		"--config.file=/etc/prometheus/prometheus.yml", "--storage.tsdb.path=/prometheus",
		"--storage.tsdb.retention.time=1d", "--web.enable-remote-write-receiver",
		"--enable-feature=native-histograms",
	}
	cf.Configs["prometheus"] = &composeConfig{Content: prometheusConfig(p)}
	prom.Configs = []composeConfigRef{{Source: "prometheus", Target: "/etc/prometheus/prometheus.yml"}}
	prom.Ports = []string{fmt.Sprintf("127.0.0.1::%d", promPort)}
	prom.Healthcheck = &composeHealth{Test: []string{"CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:9090/-/ready"}, Interval: "1s", Timeout: "3s", Retries: 60}
	cf.Services[svcPrometheus] = prom

	cad := base(ImageCAdvisor, svcCAdvisor, "infra", -1, p.Infra.CAdvisorCPU, p.Infra.CAdvisorMem)
	cad.Privileged = true
	cad.Devices = []string{"/dev/kmsg"}
	cad.Volumes = []string{"/:/rootfs:ro", "/var/run:/var/run:ro", "/sys:/sys:ro", "/var/lib/docker/:/var/lib/docker:ro"}
	cad.Command = []string{
		"--docker_only=true", "--housekeeping_interval=1s", "--store_container_labels=false",
		"--whitelisted_container_labels=" + strings.Join([]string{LabelRun, LabelService, LabelReplica, LabelRole}, ","),
		"--disable_metrics=advtcp,cpu_topology,cpuset,hugetlb,memory_numa,percpu,process,referenced_memory,resctrl,sched,tcp,udp",
	}
	cf.Services[svcCAdvisor] = cad

	// The load generator starts only when the run asks for it.
	k6 := base(ImageK6, svcLoad, "load", -1, p.Infra.LoadCPU, p.Infra.LoadMemory)
	k6.Profiles = []string{"load"}
	k6.Command = []string{"run", "--quiet", "--no-usage-report", "-o", "experimental-prometheus-rw", k6ScriptPath}
	k6.Environment = map[string]string{
		"K6_PROMETHEUS_RW_SERVER_URL":                fmt.Sprintf("http://%s:%d/api/v1/write", svcPrometheus, promPort),
		"K6_PROMETHEUS_RW_PUSH_INTERVAL":             "1s",
		"K6_PROMETHEUS_RW_TREND_AS_NATIVE_HISTOGRAM": "true",
	}
	cf.Configs["k6"] = &composeConfig{Content: string(in.K6Script)}
	k6.Configs = []composeConfigRef{{Source: "k6", Target: k6ScriptPath}}
	cf.Services[svcLoad] = k6

	if len(cf.Volumes) == 0 {
		cf.Volumes = nil
	}
	var b strings.Builder
	b.WriteString("# Generated by loadl for run " + p.RunID + ". Inspect it, or rerun the stack with\n")
	b.WriteString("# docker compose -f compose.yaml up -d (the data volume must exist).\n")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(cf); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

// pgDataPath is where the postgres image keeps its data. From 18 the image
// mounts the parent directory.
func pgDataPath(version string) string {
	if major, err := strconv.Atoi(strings.SplitN(version, ".", 2)[0]); err == nil && major >= 18 {
		return "/var/lib/postgresql"
	}
	return "/var/lib/postgresql/data"
}

type toxiProxy struct {
	Name     string `json:"name"`
	Listen   string `json:"listen"`
	Upstream string `json:"upstream"`
	Enabled  bool   `json:"enabled"`
}

func toxiproxyConfig(p *plan.Plan) ([]byte, error) {
	proxies := []toxiProxy{}
	for _, x := range p.Proxies {
		proxies = append(proxies, toxiProxy{Name: x.Name, Listen: fmt.Sprintf("0.0.0.0:%d", x.Listen), Upstream: x.Upstream, Enabled: true})
	}
	return json.MarshalIndent(proxies, "", "  ")
}

// prometheusConfig scrapes every container of the run once a second. The
// cAdvisor job keeps only this run's containers.
func prometheusConfig(p *plan.Plan) string {
	var app, pg, rd []string
	for _, s := range p.Services {
		switch s.Kind {
		case "app":
			if len(s.Synthetic) == 0 {
				continue
			}
			for _, c := range s.Containers {
				app = append(app, fmt.Sprintf("%s:%d", c, plan.AppPort))
			}
		case "postgres":
			pg = append(pg, s.Name)
		case "redis":
			rd = append(rd, s.Name)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "global:\n  scrape_interval: 1s\n  scrape_timeout: 1s\n  evaluation_interval: 5s\n  external_labels:\n    run: %q\nscrape_configs:\n", p.RunID)
	job := func(name string, targets []string, extra string) {
		fmt.Fprintf(&b, "  - job_name: %s\n    static_configs:\n      - targets: [%s]\n%s", name, quoteJoin(targets), extra)
	}
	if len(app) > 0 {
		job("synthetic", app, "")
	}
	for _, name := range pg {
		job("postgres-"+name, []string{fmt.Sprintf("lore-pgexp-%s:9187", name)}, fmt.Sprintf("        labels: {service: %q}\n", name))
	}
	for _, name := range rd {
		job("redis-"+name, []string{fmt.Sprintf("lore-redisexp-%s:9121", name)}, fmt.Sprintf("        labels: {service: %q}\n", name))
	}
	job("toxiproxy", []string{fmt.Sprintf("%s:%d", svcToxiproxy, toxiproxyAPI)}, "")
	job("cadvisor", []string{svcCAdvisor + ":8080"}, fmt.Sprintf(`    metric_relabel_configs:
      - source_labels: [container_label_io_loadlore_run]
        regex: %q
        action: keep
      - source_labels: [container_label_io_loadlore_service]
        target_label: service
      - source_labels: [container_label_io_loadlore_replica]
        target_label: replica
      - source_labels: [container_label_io_loadlore_role]
        target_label: role
      - regex: "container_label_.*|id"
        action: labeldrop
`, p.RunID))
	return b.String()
}

func quoteJoin(xs []string) string {
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = strconv.Quote(x)
	}
	return strings.Join(q, ", ")
}

func memStr(b int64) string { return strconv.FormatInt(b>>20, 10) + "m" }

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
