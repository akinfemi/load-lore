package design

import (
	"slices"
	"strings"
)

// MetricKind decides how an expect value is written and normalized.
type MetricKind int

// Metric kinds.
const (
	MetricLatency MetricKind = iota + 1 // milliseconds; "ms", "s" or no unit
	MetricRatio                         // 0–1; "%" or a fraction
	MetricRate                          // requests per second; no unit
	MetricCount                         // a plain count; no unit
)

// EndToEndMetrics can be predicted without a service prefix. load_rps is the
// offered load (what the load generator sends) and may only appear in until
// conditions; rps is achieved throughput.
var EndToEndMetrics = map[string]MetricKind{
	"p50_ms":     MetricLatency,
	"p95_ms":     MetricLatency,
	"p99_ms":     MetricLatency,
	"error_rate": MetricRatio,
	"rps":        MetricRate,
	"load_rps":   MetricRate,
}

// ComponentMetrics lists the per-component metrics, written
// <service>.<metric>, by component class. Each must map to a series the run
// collects. cpu and memory are utilization of the container's limit.
var ComponentMetrics = map[string]map[string]MetricKind{
	"postgres": {"connections": MetricCount, "cpu": MetricRatio, "memory": MetricRatio},
	"redis":    {"hit_rate": MetricRatio, "memory": MetricRatio, "cpu": MetricRatio},
	"service":  {"cpu": MetricRatio, "memory": MetricRatio, "p99_ms": MetricLatency, "error_rate": MetricRatio, "inflight": MetricCount},
}

// metricClass is the ComponentMetrics key for a service.
func (s *Service) metricClass() string {
	if s.Kind == "" {
		return "service"
	}
	return s.Kind
}

func metricNames(m map[string]MetricKind) string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}
