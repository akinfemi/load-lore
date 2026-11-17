package local

import (
	"encoding/json"
	"fmt"
	"math"
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
	ImagePgbExporter = "prometheuscommunity/pgbouncer-exporter:v0.12.1"
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
	Entrypoint      []string                 `yaml:"entrypoint,omitempty"`
	Command         []string                 `yaml:"command,omitempty"`
	Environment     map[string]string        `yaml:"environment,omitempty"`
	CPUs            float64                  `yaml:"cpus,omitempty"`
	CPUPeriod       int64                    `yaml:"cpu_period,omitempty"` // microseconds
	CPUQuota        int64                    `yaml:"cpu_quota,omitempty"`
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
	BlkioConfig     *composeBlkio            `yaml:"blkio_config,omitempty"`
}

// composeBlkio caps a container's I/O on a device.
type composeBlkio struct {
	DeviceReadIOPS  []composeDeviceRate `yaml:"device_read_iops,omitempty"`
	DeviceWriteIOPS []composeDeviceRate `yaml:"device_write_iops,omitempty"`
	DeviceReadBps   []composeDeviceRate `yaml:"device_read_bps,omitempty"`
	DeviceWriteBps  []composeDeviceRate `yaml:"device_write_bps,omitempty"`
}

type composeDeviceRate struct {
	Path string `yaml:"path"`
	Rate int    `yaml:"rate"`
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

// pgExporterQueries adds pg_lock_waits_waiting: the backends of the run's
// database waiting on a heavyweight lock, such as a row lock, at the scrape.
const (
	pgExporterQueriesPath = "/etc/loadlore/pg-queries.yaml"
	pgExporterQueries     = `pg_lock_waits:
  query: "SELECT count(*) AS waiting FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()"
  metrics:
    - waiting:
        usage: "GAUGE"
        description: "Backends waiting on a heavyweight lock"
`
)

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
	DiskDevs  map[string]string // postgres service with a disk -> its dm-delay device
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

	// A design container whose CFS period isn't Docker's default gets an
	// explicit period and quota in place of cpus; Compose rejects both.
	setPeriod := func(c *composeService, s *plan.Service) {
		if s.CPUPeriod > 0 && s.CPUPeriod != plan.DefaultCPUPeriodMs && c.CPUs > 0 {
			period := int64(s.CPUPeriod) * 1000
			c.CPUs, c.CPUPeriod, c.CPUQuota = 0, period, int64(math.Round(c.CPUs*float64(period)))
		}
	}

	// Services wait for Toxiproxy only when it carries connections.
	toxDep := func(m map[string]composeDepend) map[string]composeDepend {
		if p.UsesToxiproxy() {
			m[svcToxiproxy] = composeDepend{Condition: "service_started"}
		}
		return m
	}
	healthyDeps := toxDep(map[string]composeDepend{})
	for _, s := range p.Services {
		switch s.Kind {
		case "postgres":
			// One container per replica: a primary has one, a postgres-replica
			// service one or several, each with its own data and exporter.
			for i, name := range s.Containers {
				replica := -1
				if len(s.Containers) > 1 {
					replica = i
				}
				c := base(s.Image, s.Name, "datastore", replica, s.CPU, s.Memory)
				setPeriod(c, s)
				c.PullPolicy = "missing"
				c.Environment = map[string]string{"POSTGRES_USER": plan.DBUser, "POSTGRES_PASSWORD": plan.DBPassword, "POSTGRES_DB": plan.DBName}
				c.Command = []string{"postgres"}
				for _, k := range sortedKeys(s.Config) {
					v := s.Config[k]
					if k == "primary_conninfo" {
						// The primary tells its replicas apart by this.
						v += " application_name=" + name
					}
					c.Command = append(c.Command, "-c", k+"="+v)
				}
				// Docker's 64 MB /dev/shm is too small for parallel queries.
				c.ShmSize = memStr(max(64<<20, s.Memory/4))
				vol := "data-" + name
				cf.Volumes[vol] = &composeVolume{External: true, Name: in.DataVols[name]}
				c.Volumes = []string{vol + ":" + pgDataPath(s.Version)}
				if dev := in.DiskDevs[name]; s.Disk != nil && dev != "" {
					rate := []composeDeviceRate{{Path: dev, Rate: s.Disk.IOPS}}
					c.BlkioConfig = &composeBlkio{DeviceReadIOPS: rate, DeviceWriteIOPS: rate}
					if s.Disk.MiBps > 0 {
						bps := []composeDeviceRate{{Path: dev, Rate: int(s.Disk.MiBps * (1 << 20))}}
						c.BlkioConfig.DeviceReadBps, c.BlkioConfig.DeviceWriteBps = bps, bps
					}
				}
				c.Healthcheck = &composeHealth{Test: []string{"CMD-SHELL", fmt.Sprintf("pg_isready -U %s -d %s -h 127.0.0.1", plan.DBUser, plan.DBName)}, Interval: "1s", Timeout: "3s", Retries: 180}
				c.StopGracePeriod = "10s"
				if s.Primary != "" {
					// It streams from the primary from the moment it starts.
					c.DependsOn = map[string]composeDepend{s.Primary: {Condition: "service_healthy"}}
				}
				cf.Services[name] = c
				healthyDeps[name] = composeDepend{Condition: "service_healthy"}

				exp := base(ImagePGExporter, s.Name, "exporter", replica, p.Infra.ExporterCPU, p.Infra.ExporterMem)
				exp.Environment = map[string]string{
					"DATA_SOURCE_URI":  fmt.Sprintf("%s:%d/%s?sslmode=disable", name, plan.PostgresPort, plan.DBName),
					"DATA_SOURCE_USER": plan.DBUser, "DATA_SOURCE_PASS": plan.DBPassword,
				}
				// The exporter has no lock-wait metric of its own; this query adds one.
				exp.Environment["PG_EXPORTER_EXTEND_QUERY_PATH"] = pgExporterQueriesPath
				cf.Configs["pgexp-queries"] = &composeConfig{Content: pgExporterQueries}
				exp.Configs = []composeConfigRef{{Source: "pgexp-queries", Target: pgExporterQueriesPath}}
				exp.DependsOn = map[string]composeDepend{name: {Condition: "service_healthy"}}
				cf.Services["lore-pgexp-"+name] = exp
			}
		case "redis", "queue":
			c := base(s.Image, s.Name, "datastore", -1, s.CPU, s.Memory)
			setPeriod(c, s)
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
			if s.Kind == "queue" {
				// The stream's length and its consumer group: jobs waiting
				// and running.
				exp.Environment["REDIS_EXPORTER_CHECK_SINGLE_STREAMS"] = s.Name
			}
			exp.DependsOn = map[string]composeDepend{s.Name: {Condition: "service_healthy"}}
			cf.Services["lore-redisexp-"+s.Name] = exp
		}
	}

	for _, s := range p.Services {
		if s.Kind != "pgbouncer" {
			continue
		}
		c := base(s.Image, s.Name, "pgbouncer", -1, s.CPU, s.Memory)
		setPeriod(c, s)
		c.PullPolicy = "missing"
		ini, users := "pgbouncer-ini-"+s.Name, "pgbouncer-userlist-"+s.Name
		cf.Configs[ini] = &composeConfig{Content: pgbouncerConfig(s)}
		cf.Configs[users] = &composeConfig{Content: fmt.Sprintf("%q %q\n", plan.DBUser, plan.DBPassword)}
		c.Configs = []composeConfigRef{{Source: ini, Target: pgbouncerINI}, {Source: users, Target: pgbouncerUsers}}
		// The image's entrypoint writes a config from environment variables
		// when none is mounted; run PgBouncer on the generated one directly.
		c.Entrypoint = []string{"/usr/bin/pgbouncer"}
		c.Command = []string{pgbouncerINI}
		c.Healthcheck = &composeHealth{Test: []string{"CMD", "pg_isready", "-h", "127.0.0.1", "-p", strconv.Itoa(plan.PgBouncerPort), "-U", plan.DBUser, "-d", plan.DBName}, Interval: "1s", Timeout: "3s", Retries: 60}
		c.DependsOn = toxDep(map[string]composeDepend{s.PgBouncer.Target: {Condition: "service_healthy"}})
		cf.Services[s.Name] = c
		healthyDeps[s.Name] = composeDepend{Condition: "service_healthy"}

		exp := base(ImagePgbExporter, s.Name, "exporter", -1, p.Infra.ExporterCPU, p.Infra.ExporterMem)
		exp.Command = []string{fmt.Sprintf("--pgBouncer.connectionString=postgres://%s:%s@%s:%d/pgbouncer?sslmode=disable", plan.DBUser, plan.DBPassword, s.Name, plan.PgBouncerPort)}
		exp.DependsOn = map[string]composeDepend{s.Name: {Condition: "service_healthy"}}
		cf.Services["lore-pgbexp-"+s.Name] = exp
	}

	for _, s := range p.Services {
		if s.Kind != "lb" {
			continue
		}
		c := base(s.Image, s.Name, "lb", -1, s.CPU, s.Memory)
		setPeriod(c, s)
		c.PullPolicy = "missing"
		cfgName := "haproxy-" + s.Name
		cf.Configs[cfgName] = &composeConfig{Content: haproxyConfig(s.LB)}
		c.Configs = []composeConfigRef{{Source: cfgName, Target: "/usr/local/etc/haproxy/haproxy.cfg"}}
		// The image has no HTTP client; bash can open a TCP connection.
		c.Healthcheck = &composeHealth{Test: []string{"CMD", "bash", "-c", fmt.Sprintf("exec 3<>/dev/tcp/127.0.0.1/%d", plan.LBStatsPort)}, Interval: "1s", Timeout: "3s", Retries: 60}
		// The replicas start first so the lb's first checks find them up;
		// an autoscaler's later replicas aren't started yet.
		deps := toxDep(map[string]composeDepend{})
		for _, srv := range s.LB.Servers {
			if !srv.Disabled {
				deps[srv.Name] = composeDepend{Condition: "service_healthy"}
			}
		}
		if s.LB.Autoscaled {
			c.Ports = []string{fmt.Sprintf("127.0.0.1::%d", plan.LBAdminPort)}
		}
		c.DependsOn = deps
		cf.Services[s.Name] = c
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
			setPeriod(c, s)
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
			if !slices.Contains(s.Running(), name) {
				// An autoscaler's replica: started during the run, by name.
				c.Profiles = []string{profileScale}
			}
			cf.Services[name] = c
		}
	}

	// Toxiproxy: one listener per proxied connection, and none at all when
	// no fault acts on a connection.
	if p.UsesToxiproxy() {
		tox := base(ImageToxiproxy, svcToxiproxy, "infra", -1, p.Infra.ProxyCPU, p.Infra.ProxyMemory)
		tox.Command = []string{"-host=0.0.0.0", "-config=/config/toxiproxy.json", "-proxy-metrics", "-runtime-metrics"}
		tox.Environment = map[string]string{"GOMEMLIMIT": goMemLimit(p.Infra.ProxyMemory)}
		toxCfg, err := toxiproxyConfig(p)
		if err != nil {
			return nil, err
		}
		cf.Configs["toxiproxy"] = &composeConfig{Content: string(toxCfg)}
		tox.Configs = []composeConfigRef{{Source: "toxiproxy", Target: "/config/toxiproxy.json"}}
		tox.Ports = []string{fmt.Sprintf("127.0.0.1::%d", toxiproxyAPI)}
		cf.Services[svcToxiproxy] = tox
	}

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
		"GOMEMLIMIT": goMemLimit(p.Infra.LoadMemory),
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

// lbAlgorithms maps design algorithm names to HAProxy balance modes.
var lbAlgorithms = map[string]string{"round-robin": "roundrobin", "least-conn": "leastconn", "random": "random"}

// Where the PgBouncer image reads its config and users.
const (
	pgbouncerINI   = "/etc/pgbouncer/pgbouncer.ini"
	pgbouncerUsers = "/etc/pgbouncer/userlist.txt"
)

// pgbouncerConfig is a PgBouncer's pgbouncer.ini: one database, the run's,
// reached through the proxy in front of its Postgres, then the design's
// own pgbouncer.ini keys, which win.
func pgbouncerConfig(s *plan.Service) string {
	b := s.PgBouncer
	host, port, _ := strings.Cut(b.Upstream, ":")
	var w strings.Builder
	fmt.Fprintf(&w, "[databases]\n%s = host=%s port=%s dbname=%s\n\n[pgbouncer]\n", plan.DBName, host, port, plan.DBName)
	settings := map[string]string{
		"listen_addr": "0.0.0.0", "listen_port": strconv.Itoa(plan.PgBouncerPort), "unix_socket_dir": "",
		"auth_type": "scram-sha-256", "auth_file": pgbouncerUsers,
		// The exporter reads SHOW POOLS and SHOW STATS as this user.
		"admin_users": plan.DBUser,
		"pool_mode":   b.PoolMode, "default_pool_size": strconv.Itoa(b.DefaultPoolSize),
		"max_client_conn": strconv.Itoa(b.MaxClientConn),
		// Some drivers send it; PgBouncer refuses unknown startup parameters.
		"ignore_startup_parameters": "extra_float_digits",
	}
	for k, v := range s.Config {
		settings[k] = v
	}
	for _, k := range sortedKeys(settings) {
		fmt.Fprintf(&w, "%s = %s\n", k, settings[k])
	}
	return w.String()
}

// haproxyConfig is an lb's HAProxy configuration: HTTP mode, active health
// checks on every replica, and the built-in Prometheus exporter. Everything
// else is HAProxy's own default.
func haproxyConfig(lb *plan.LB) string {
	var b strings.Builder
	admin := ""
	if lb.Autoscaled {
		// The autoscaler enables and drains servers through it.
		admin = fmt.Sprintf("    stats socket ipv4@0.0.0.0:%d level admin\n", plan.LBAdminPort)
	}
	fmt.Fprintf(&b, `global
    maxconn 50000
%s
defaults
    mode http
    timeout connect 5s
    timeout client 60s
    timeout server 60s
    timeout http-keep-alive 60s
    timeout check %dms

frontend web
    bind :%d
    default_backend replicas

frontend stats
    bind :%d
    http-request use-service prometheus-exporter if { path /metrics }

backend replicas
    balance %s
    option httpchk GET %s
    default-server check inter %dms fall %d rise %d
`, admin, lb.TimeoutMs, plan.AppPort, lb.StatsListen, lbAlgorithms[lb.Algorithm], lb.CheckPath, lb.IntervalMs, lb.Fall, lb.Rise)
	for _, srv := range lb.Servers {
		off := ""
		if srv.Disabled {
			off = " disabled"
		}
		fmt.Fprintf(&b, "    server %s %s%s\n", srv.Name, srv.Addr, off)
	}
	return b.String()
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
	for _, x := range p.Proxied() {
		proxies = append(proxies, toxiProxy{Name: x.Name, Listen: fmt.Sprintf("0.0.0.0:%d", x.Listen), Upstream: x.Upstream, Enabled: true})
	}
	return json.MarshalIndent(proxies, "", "  ")
}

// prometheusConfig scrapes every container of the run once a second. The
// cAdvisor job keeps only this run's containers.
func prometheusConfig(p *plan.Plan) string {
	type pgTarget struct{ service, container, replica string }
	var app, rd, pgb, lbs []string
	var pg []pgTarget
	for _, s := range p.Services {
		switch s.Kind {
		case "lb":
			lbs = append(lbs, s.Name)
		case "app":
			if len(s.Synthetic) == 0 {
				continue
			}
			for _, c := range s.Containers {
				app = append(app, fmt.Sprintf("%s:%d", c, plan.AppPort))
			}
		case "postgres":
			for i, c := range s.Containers {
				t := pgTarget{service: s.Name, container: c}
				if len(s.Containers) > 1 {
					t.replica = strconv.Itoa(i)
				}
				pg = append(pg, t)
			}
		case "redis", "queue":
			rd = append(rd, s.Name)
		case "pgbouncer":
			pgb = append(pgb, s.Name)
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
	for _, t := range pg {
		labels := fmt.Sprintf("        labels: {service: %q}\n", t.service)
		if t.replica != "" {
			labels = fmt.Sprintf("        labels: {service: %q, replica: %q}\n", t.service, t.replica)
		}
		job("postgres-"+t.container, []string{fmt.Sprintf("lore-pgexp-%s:9187", t.container)}, labels)
	}
	for _, name := range rd {
		job("redis-"+name, []string{fmt.Sprintf("lore-redisexp-%s:9121", name)}, fmt.Sprintf("        labels: {service: %q}\n", name))
	}
	for _, name := range pgb {
		job("pgbouncer-"+name, []string{fmt.Sprintf("lore-pgbexp-%s:9127", name)}, fmt.Sprintf("        labels: {service: %q}\n", name))
	}
	for _, name := range lbs {
		job("lb-"+name, []string{fmt.Sprintf("%s:%d", name, plan.LBStatsPort)}, fmt.Sprintf("        labels: {service: %q}\n", name))
	}
	if p.UsesToxiproxy() {
		job("toxiproxy", []string{fmt.Sprintf("%s:%d", svcToxiproxy, toxiproxyAPI)}, "")
	}
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

// goMemLimit makes a Go tool's garbage collector work harder as it nears
// its container's memory limit, instead of being OOM-killed at it.
func goMemLimit(limit int64) string { return strconv.FormatInt(limit*9/10>>20, 10) + "MiB" }

func memStr(b int64) string { return strconv.FormatInt(b>>20, 10) + "m" }

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// profileScale holds an autoscaler's replicas that don't run when the load
// starts: compose up leaves them, and the autoscaler starts them by name.
const profileScale = "scale"
