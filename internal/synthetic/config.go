// Package synthetic is the configurable service behind loadlore/synthetic. It
// stands in for an application service: it burns real CPU, allocates real
// memory, and calls Postgres, Redis and other services through real client
// libraries with real pools, timeouts and retries. Nothing sleeps in place of
// I/O, so saturation, queueing and pool exhaustion happen for real.
package synthetic

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
)

// Env vars the container reads its config from. ConfigEnv holds the JSON
// itself; ConfigFileEnv points at a mounted file. ConfigEnv wins.
const (
	ConfigEnv     = "LOADLORE_SYNTHETIC_CONFIG"
	ConfigFileEnv = "LOADLORE_SYNTHETIC_CONFIG_FILE"
)

// Config is everything one replica needs. The CLI builds it from the design
// file with every default already applied.
type Config struct {
	Service string `json:"service"`
	Replica int    `json:"replica"`
	Listen  string `json:"listen,omitempty"` // default ":8080"

	Concurrency Concurrency `json:"concurrency"`
	CPUMs       float64     `json:"cpu_ms"`
	CPUJitter   float64     `json:"cpu_jitter"`
	AllocKB     int         `json:"alloc_kb"`
	Calls       []Call      `json:"calls"`
	Resilience  Resilience  `json:"resilience"`

	// Targets are the services this one calls, by design name.
	Targets map[string]Target `json:"targets"`
}

// Concurrency models the service runtime.
type Concurrency struct {
	Model string `json:"model"` // threads | event-loop | workers
	Max   int    `json:"max"`   // 0 = unlimited (threads, event-loop)
}

// Call is one outbound call, as in the design file.
type Call struct {
	Target string `json:"target"`
	Op     string `json:"op"` // read | write | get | set | del | http
	Count  int    `json:"count"`
	When   string `json:"when"` // always | read | write
	Mode   string `json:"mode"` // sync | async
	OnMiss []Call `json:"on_miss,omitempty"`
	TTLS   int    `json:"ttl_s,omitempty"`
	Query  string `json:"query,omitempty"` // point | range | scan
	Rows   int    `json:"rows,omitempty"`
}

// Resilience settings for every outbound call.
type Resilience struct {
	TimeoutMs      int             `json:"timeout_ms"` // 0 = none
	Retries        *Retries        `json:"retries,omitempty"`
	CircuitBreaker *CircuitBreaker `json:"circuit_breaker,omitempty"`
	Bulkhead       *Bulkhead       `json:"bulkhead,omitempty"`
}

// Retries on a failed call.
type Retries struct {
	Max     int    `json:"max"`
	Backoff string `json:"backoff"` // none | constant | exponential
	BaseMs  int    `json:"base_ms"`
	Jitter  bool   `json:"jitter"`
}

// CircuitBreaker per target.
type CircuitBreaker struct {
	ErrorRate float64 `json:"error_rate"`
	WindowS   int     `json:"window_s"`
	OpenS     int     `json:"open_s"`
}

// Bulkhead caps concurrent calls per target.
type Bulkhead struct {
	MaxConcurrent int `json:"max_concurrent"`
}

// Target kinds as the synthetic service sees them. pgbouncer is a postgres
// target with SimpleProtocol set.
const (
	KindPostgres = "postgres"
	KindRedis    = "redis"
	KindHTTP     = "http"
)

// Target is how to reach one dependency. Addrs are usually Toxiproxy
// listeners: one for a datastore, one per replica for an application
// service, which the HTTP client round-robins across.
type Target struct {
	Kind  string   `json:"kind"`
	Addrs []string `json:"addrs"` // host:port
	Pool  int      `json:"pool"`  // connections per replica

	// Postgres.
	Database       string `json:"database,omitempty"`
	User           string `json:"user,omitempty"`
	Password       string `json:"password,omitempty"`
	SimpleProtocol bool   `json:"simple_protocol,omitempty"` // for pgbouncer in transaction mode

	// Dataset shape, for payload sizes and user_id scans (postgres), and
	// cache value sizes (redis).
	Rows     int64 `json:"rows,omitempty"`
	RowBytes int   `json:"row_bytes,omitempty"`
}

// LoadConfig reads the config from the environment.
func LoadConfig() (*Config, error) {
	data := []byte(os.Getenv(ConfigEnv))
	if len(data) == 0 {
		path := os.Getenv(ConfigFileEnv)
		if path == "" {
			return nil, fmt.Errorf("no config: set %s or %s", ConfigEnv, ConfigFileEnv)
		}
		var err error
		if data, err = os.ReadFile(path); err != nil {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}
	return ParseConfig(data)
}

// ParseConfig decodes and checks a config.
func ParseConfig(data []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("decoding config: %w", err)
	}
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate checks the config is complete. The CLI has already validated the
// design, so these errors mean a bug in the CLI, not a user mistake.
func (c *Config) Validate() error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	if c.Service == "" {
		bad("service is empty")
	}
	if !slices.Contains([]string{"threads", "event-loop", "workers"}, c.Concurrency.Model) {
		bad("concurrency.model %q", c.Concurrency.Model)
	}
	if c.Concurrency.Max < 0 || (c.Concurrency.Model == "workers" && c.Concurrency.Max < 1) {
		bad("concurrency.max %d for %s", c.Concurrency.Max, c.Concurrency.Model)
	}
	if c.CPUMs < 0 || c.CPUJitter < 0 || c.AllocKB < 0 || c.Resilience.TimeoutMs < 0 {
		bad("negative cpu_ms, cpu_jitter, alloc_kb or timeout_ms")
	}
	for name, t := range c.Targets {
		switch {
		case !slices.Contains([]string{KindPostgres, KindRedis, KindHTTP}, t.Kind):
			bad("target %s: kind %q", name, t.Kind)
		case len(t.Addrs) == 0:
			bad("target %s: no addrs", name)
		case t.Kind != KindHTTP && len(t.Addrs) != 1:
			bad("target %s: a %s target has exactly one addr", name, t.Kind)
		case t.Pool < 1:
			bad("target %s: pool %d", name, t.Pool)
		case t.Kind == KindPostgres && (t.Rows < 1 || t.RowBytes < 1):
			bad("target %s: postgres needs rows and row_bytes", name)
		}
	}
	var checkCalls func(path string, calls []Call)
	checkCalls = func(path string, calls []Call) {
		for i, call := range calls {
			p := fmt.Sprintf("%s[%d]", path, i)
			t, ok := c.Targets[call.Target]
			if !ok {
				bad("%s: no target %q", p, call.Target)
				continue
			}
			want := map[string][]string{
				KindPostgres: {"read", "write"},
				KindRedis:    {"get", "set", "del"},
				KindHTTP:     {"http"},
			}[t.Kind]
			if !slices.Contains(want, call.Op) {
				bad("%s: op %q on a %s target", p, call.Op, t.Kind)
			}
			if call.Count < 1 {
				bad("%s: count %d", p, call.Count)
			}
			if !slices.Contains([]string{"always", "read", "write"}, call.When) {
				bad("%s: when %q", p, call.When)
			}
			if call.Mode != "sync" && call.Mode != "async" {
				bad("%s: mode %q", p, call.Mode)
			}
			if call.Op == "read" && !slices.Contains([]string{"point", "range", "batch", "scan"}, call.Query) {
				bad("%s: query %q", p, call.Query)
			}
			if (call.Query == "range" || call.Query == "batch") && call.Rows < 1 {
				bad("%s: %s needs rows", p, call.Query)
			}
			checkCalls(p+".on_miss", call.OnMiss)
		}
	}
	checkCalls("calls", c.Calls)
	if r := c.Resilience.Retries; r != nil {
		if r.Max < 0 || !slices.Contains([]string{"none", "constant", "exponential"}, r.Backoff) || (r.Backoff != "none" && r.BaseMs < 1) {
			bad("retries %+v", *r)
		}
	}
	if b := c.Resilience.CircuitBreaker; b != nil && (b.ErrorRate <= 0 || b.ErrorRate > 1 || b.WindowS < 1 || b.OpenS < 1) {
		bad("circuit_breaker %+v", *b)
	}
	if b := c.Resilience.Bulkhead; b != nil && b.MaxConcurrent < 1 {
		bad("bulkhead %+v", *b)
	}
	return errors.Join(errs...)
}
