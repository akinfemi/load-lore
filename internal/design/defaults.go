package design

import (
	"math"
	"strings"
)

// Defaults. They mirror skills/loadlore-designer/references/schema.md and the
// `default=` values in the struct tags.
const (
	DefaultReplicas        = 1
	DefaultCPU             = 1.0
	DefaultMemory          = Size(512 << 20)
	DefaultPool            = 10
	DefaultCPUJitter       = 0.1
	DefaultTimeoutMs       = 5000
	DefaultBackoff         = "none"
	DefaultBackoffBaseMs   = 100
	DefaultRangeRows       = 10
	DefaultPostgresVersion = "17"
	DefaultRedisVersion    = "8"
	DefaultPoolMode        = "transaction"
	DefaultPgbPoolSize     = 20
	DefaultRowBytes        = 512
	DefaultReadShare       = 0.9
	DefaultZipfSkew        = 1.1
	DefaultHotspotShare    = 0.8
	DefaultHotspotFraction = 0.01
	DefaultPayloadKB       = 1.0
	DefaultClientTimeoutMs = 10000
	DefaultScale           = 1.0
	DefaultLBCPU           = 0.5
	DefaultLBMemory        = Size(256 << 20)
	DefaultAlgorithm       = "round-robin"
	DefaultCheckPath       = "/healthz"
	DefaultCheckIntervalMs = 2000
	DefaultCheckTimeoutMs  = 1000
	DefaultCheckFall       = 3
	DefaultCheckRise       = 2
)

// DefaultConcurrencyMax is the default max for each concurrency model.
var DefaultConcurrencyMax = map[string]int{"threads": 200, "event-loop": 0, "workers": 4}

// SyntheticImage is the image of the configurable synthetic service.
const SyntheticImage = "loadlore/synthetic"

// IsSynthetic reports whether the service runs loadlore/synthetic.
func (s *Service) IsSynthetic() bool {
	return s.Image == SyntheticImage || strings.HasPrefix(s.Image, SyntheticImage+":")
}

// IsApp reports whether the service is an application service.
func (s *Service) IsApp() bool { return s.Kind == "" && s.Image != "" }

func ptr[T any](v T) *T { return &v }

func setDefault[T any](p **T, v T) {
	if *p == nil {
		*p = ptr(v)
	}
}

// applyDefaults fills every unset field that has a default. It only fills
// fields that are valid for their context, so validation can run afterwards.
// The keyspace default needs the call graph and is set during validation.
func applyDefaults(d *Design) {
	for _, s := range d.Services {
		if s == nil {
			continue
		}
		setDefault(&s.Replicas, DefaultReplicas)
		if s.Resources == nil {
			s.Resources = &Resources{}
		}
		if s.Kind == "lb" {
			setDefault(&s.Resources.CPU, DefaultLBCPU)
			setDefault(&s.Resources.Memory, DefaultLBMemory)
		}
		setDefault(&s.Resources.CPU, DefaultCPU)
		setDefault(&s.Resources.Memory, DefaultMemory)

		switch {
		case s.IsApp():
			defaultApp(s, d.Services)
		case s.Kind == "postgres":
			if s.Version == "" {
				s.Version = DefaultPostgresVersion
			}
			if s.Dataset != nil {
				setDefault(&s.Dataset.RowBytes, DefaultRowBytes)
			}
		case s.Kind == "redis":
			if s.Version == "" {
				s.Version = DefaultRedisVersion
			}
		case s.Kind == "pgbouncer":
			if s.PoolMode == "" {
				s.PoolMode = DefaultPoolMode
			}
			setDefault(&s.DefaultPoolSize, DefaultPgbPoolSize)
		case s.Kind == "lb":
			if s.Algorithm == "" {
				s.Algorithm = DefaultAlgorithm
			}
			if s.HealthCheck == nil {
				s.HealthCheck = &HealthCheck{}
			}
			hc := s.HealthCheck
			if hc.Path == "" {
				hc.Path = DefaultCheckPath
			}
			setDefault(&hc.IntervalMs, DefaultCheckIntervalMs)
			setDefault(&hc.TimeoutMs, DefaultCheckTimeoutMs)
			setDefault(&hc.Fall, DefaultCheckFall)
			setDefault(&hc.Rise, DefaultCheckRise)
		}
	}

	if w := d.Workload; w != nil {
		defaultWorkload(w)
	}

	if d.Modes == nil {
		d.Modes = &Modes{}
	}
	if d.Modes.Local == nil {
		d.Modes.Local = &LocalMode{}
	}
	setDefault(&d.Modes.Local.Scale, DefaultScale)
}

func defaultApp(s *Service, services map[string]*Service) {
	if s.Role == "" {
		s.Role = "server"
	}
	if s.Concurrency == nil {
		s.Concurrency = &Concurrency{}
	}
	if s.Concurrency.Model == "" {
		s.Concurrency.Model = "threads"
	}
	if max, ok := DefaultConcurrencyMax[s.Concurrency.Model]; ok {
		setDefault(&s.Concurrency.Max, max)
	}
	if !s.IsSynthetic() {
		return
	}
	if s.Behavior == nil {
		s.Behavior = &Behavior{}
	}
	b := s.Behavior
	setDefault(&b.CPUJitter, DefaultCPUJitter)
	if b.Resilience == nil {
		b.Resilience = &Resilience{}
	}
	setDefault(&b.Resilience.TimeoutMs, DefaultTimeoutMs)
	if r := b.Resilience.Retries; r != nil {
		if r.Backoff == "" {
			r.Backoff = DefaultBackoff
		}
		if r.Backoff != "none" {
			setDefault(&r.BaseMs, DefaultBackoffBaseMs)
		}
	}
	walkCalls(b.Calls, func(c *Call) {
		setDefault(&c.Count, 1)
		if c.When == "" {
			c.When = "always"
		}
		if c.Mode == "" {
			c.Mode = "sync"
		}
		if c.Op == "read" && c.Query == "" {
			c.Query = "point"
		}
		if c.Query == "range" || c.Query == "batch" {
			setDefault(&c.Rows, DefaultRangeRows)
		}
		if _, known := services[c.Target]; known {
			if s.Pools == nil {
				s.Pools = map[string]int{}
			}
			if _, ok := s.Pools[c.Target]; !ok {
				s.Pools[c.Target] = DefaultPool
			}
		}
	})
}

func defaultWorkload(w *Workload) {
	if w.Mix == nil {
		w.Mix = &Mix{}
	}
	switch {
	case w.Mix.Read == nil && w.Mix.Write == nil:
		w.Mix.Read, w.Mix.Write = ptr(DefaultReadShare), ptr(complement(DefaultReadShare))
	case w.Mix.Read == nil:
		w.Mix.Read = ptr(complement(*w.Mix.Write))
	case w.Mix.Write == nil:
		w.Mix.Write = ptr(complement(*w.Mix.Read))
	}
	if w.Keys == nil {
		w.Keys = &Keys{}
	}
	switch w.Keys.Distribution {
	case "":
		w.Keys.Distribution = "uniform"
	case "zipf":
		setDefault(&w.Keys.Skew, DefaultZipfSkew)
	case "hotspot":
		setDefault(&w.Keys.Share, DefaultHotspotShare)
		setDefault(&w.Keys.Fraction, DefaultHotspotFraction)
	}
	setDefault(&w.PayloadKB, DefaultPayloadKB)
	setDefault(&w.TimeoutMs, DefaultClientTimeoutMs)
}

// walkCalls visits every call, including on_miss calls, depth first.
func walkCalls(calls []*Call, fn func(*Call)) {
	for _, c := range calls {
		if c == nil {
			continue
		}
		fn(c)
		walkCalls(c.OnMiss, fn)
	}
}

// complement returns 1-f rounded to 9 decimals, so 1-0.9 prints as 0.1.
func complement(f float64) float64 { return math.Round((1-f)*1e9) / 1e9 }
