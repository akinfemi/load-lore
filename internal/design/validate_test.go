package design

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// base is a small valid design. Each case patches it at field paths.
const base = `
version: 1
name: base
services:
  api:
    image: loadlore/synthetic
    replicas: 2
    behavior:
      cpu_ms: 1
      calls:
        - { target: db, op: read }
  db:
    kind: postgres
    dataset: { rows: 1000 }
  lb:
    kind: lb
    target: api
workload:
  entry: lb
  profile:
    - hold: { rps: 10, for: 1m }
`

const del = "<delete>"

// patch sets each path (services.api.behavior.calls[1]) in base to the YAML
// value given, creating maps and extending lists as needed, or deletes it.
func patch(t *testing.T, edits map[string]string) []byte {
	t.Helper()
	var doc any
	if err := yaml.Unmarshal([]byte(base), &doc); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(edits))
	for p := range edits {
		paths = append(paths, p)
	}
	slices.Sort(paths) // calls[0] before calls[1]
	for _, p := range paths {
		var val any
		if edits[p] != del {
			if err := yaml.Unmarshal([]byte(edits[p]), &val); err != nil {
				t.Fatalf("bad value for %s: %v", p, err)
			}
		}
		segs, err := splitPath(p)
		if err != nil {
			t.Fatal(err)
		}
		doc = setPath(doc, segs, val, edits[p] == del)
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func setPath(cur any, segs []pathSeg, val any, remove bool) any {
	if len(segs) == 0 {
		return val
	}
	s := segs[0]
	if s.index >= 0 {
		list, _ := cur.([]any)
		for len(list) <= s.index {
			list = append(list, nil)
		}
		if remove && len(segs) == 1 {
			return slices.Delete(list, s.index, s.index+1)
		}
		list[s.index] = setPath(list[s.index], segs[1:], val, remove)
		return list
	}
	m, _ := cur.(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	if remove && len(segs) == 1 {
		delete(m, s.name)
		return m
	}
	m[s.name] = setPath(m[s.name], segs[1:], val, remove)
	return m
}

func diagKeys(ds Diagnostics) []string {
	out := []string{}
	for _, d := range ds {
		out = append(out, fmt.Sprintf("%s %s %s", d.Severity, d.Path, d.Code))
	}
	slices.Sort(out)
	return out
}

func TestBaseIsValid(t *testing.T) {
	_, diags := Load(patch(t, nil))
	if len(diags) != 0 {
		t.Fatalf("base design has diagnostics: %v", diags)
	}
}

func TestValidationDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		set  map[string]string
		want []string // "severity path code", in any order; exactly these
	}{
		// Top level and decoding.
		{"bad version", map[string]string{"version": "2"}, []string{"error version invalid-value"}},
		{"name not kebab", map[string]string{"name": "My_Design"}, []string{"error name invalid-value"}},
		{"missing workload", map[string]string{"workload": del}, []string{"error workload required"}},
		{"missing services", map[string]string{"services": del, "workload": del}, []string{"error services required", "error workload required"}},
		{"unknown top-level field", map[string]string{"edges": "[]"}, []string{"error edges unknown-field"}},
		{"typo suggests field", map[string]string{"services.api.behavoir": "{}"}, []string{"error services.api.behavoir unknown-field"}},
		{"wrong type", map[string]string{"services.api.replicas": "two"}, []string{"error services.api.replicas type"}},
		{"float for int", map[string]string{"services.api.replicas": "1.5"}, []string{"error services.api.replicas type"}},
		{"mapping for scalar", map[string]string{"services.api.image": "{a: 1}"}, []string{"error services.api.image type"}},
		{"replicas below minimum", map[string]string{"services.api.replicas": "0"}, []string{"error services.api.replicas range"}},
		{"cpu must be positive", map[string]string{"services.api.resources": "{cpu: 0}"}, []string{"error services.api.resources.cpu range"}},
		{"bad memory size", map[string]string{"services.api.resources": "{memory: lots}"}, []string{"error services.api.resources.memory invalid-value"}},
		{"duration needs unit", map[string]string{"workload.profile[0]": "{hold: {rps: 10, for: 60}}"}, []string{"error workload.profile[0].hold.for invalid-value"}},

		// Outcomes and planned features.
		{"hit_rate is an outcome", map[string]string{"services.cache": "{kind: redis, hit_rate: 0.9}"}, []string{"error services.cache.hit_rate outcome-field"}},
		{"max_rps is an outcome", map[string]string{"services.api.max_rps": "1000"}, []string{"error services.api.max_rps outcome-field"}},
		{"kafka is planned", map[string]string{"services.q": "{kind: kafka}"}, []string{"error services.q.kind planned"}},
		{"worker is planned", map[string]string{"services.api.role": "worker"}, []string{"error services.api.role planned"}},
		{"publish is planned", map[string]string{"services.api.behavior.calls[1]": "{target: db, op: publish}"}, []string{"error services.api.behavior.calls[1].op planned"}},
		{"cloud mode is planned", map[string]string{"modes": "{cloud: {provider: aws}}"}, []string{"error modes.cloud planned"}},

		// Services.
		{"bad service name", map[string]string{"services.Bad_Name": "{kind: redis}"}, []string{"error services.Bad_Name invalid-value"}},
		{"image and kind", map[string]string{"services.api.kind": "redis"}, []string{"error services.api.kind conflict"}},
		{"neither image nor kind", map[string]string{"services.x": "{replicas: 1}"}, []string{"error services.x required"}},
		{"empty service", map[string]string{"services.x": "~"}, []string{"error services.x required"}},
		{"dataset on app", map[string]string{"services.api.dataset": "{rows: 10}"}, []string{"error services.api.dataset not-applicable"}},
		{"behavior on datastore", map[string]string{"services.db.behavior": "{cpu_ms: 1}"}, []string{"error services.db.behavior not-applicable"}},
		{"behavior on own image", map[string]string{"services.api.image": "ghcr.io/me/api:1"}, []string{"error services.api.behavior not-applicable"}},
		{"dataset on redis", map[string]string{"services.cache": "{kind: redis, dataset: {rows: 5}}"}, []string{"error services.cache.dataset not-applicable"}},
		{"postgres replicas", map[string]string{"services.db.replicas": "2"}, []string{"error services.db.replicas range"}},
		{"unknown index column", map[string]string{"services.db.dataset.indexes": "[id, email]"}, []string{"error services.db.dataset.indexes[1] invalid-value"}},
		{"pgbouncer needs target", map[string]string{"services.pgb": "{kind: pgbouncer}"}, []string{"error services.pgb.target required"}},
		{"pgbouncer target not postgres", map[string]string{"services.pgb": "{kind: pgbouncer, target: api}"}, []string{"error services.pgb.target op-target"}},
		{"pool_mode on postgres", map[string]string{"services.db.pool_mode": "session"}, []string{"error services.db.pool_mode not-applicable"}},
		{"workers need one", map[string]string{"services.api.concurrency": "{model: workers, max: 0}"}, []string{"error services.api.concurrency.max range"}},

		// Calls.
		{"unknown target", map[string]string{"services.api.behavior.calls[1]": "{target: dbb, op: read}"}, []string{"error services.api.behavior.calls[1].target unknown-service"}},
		{"call missing op", map[string]string{"services.api.behavior.calls[1]": "{target: db}"}, []string{"error services.api.behavior.calls[1].op required"}},
		{"bad op value", map[string]string{"services.api.behavior.calls[1]": "{target: db, op: query}"}, []string{"error services.api.behavior.calls[1].op invalid-value"}},
		{"op wrong for kind", map[string]string{"services.api.behavior.calls[1]": "{target: db, op: get}"}, []string{"error services.api.behavior.calls[1].op op-target"}},
		{"calls itself", map[string]string{"services.api.behavior.calls[1]": "{target: api, op: http}"}, []string{"error services.api.behavior.calls[1].target cycle"}},
		{"partition replica out of range", map[string]string{"scenario": "[{at: 10s, inject: partition, between: [\"api[9]\", db]}]"}, []string{"error scenario[0].between[0] range"}},
		{"partition bad side", map[string]string{"scenario": "[{at: 10s, inject: partition, between: [\"API!\", db]}]"}, []string{"error scenario[0].between[0] invalid-value"}},
		{"coalesce needs on_miss", map[string]string{"services.cache": "{kind: redis}", "services.api.behavior.calls[1]": "{target: cache, op: get, coalesce: true}"}, []string{"error services.api.behavior.calls[1].coalesce not-applicable"}},
		{"coalesce on read", map[string]string{"services.api.behavior.calls[0]": "{target: db, op: read, coalesce: true}"}, []string{"error services.api.behavior.calls[0].coalesce not-applicable"}},
		{"on_miss on read", map[string]string{"services.api.behavior.calls[0]": "{target: db, op: read, on_miss: [{target: db, op: read}]}"}, []string{"error services.api.behavior.calls[0].on_miss not-applicable"}},
		{"ttl on get", map[string]string{"services.cache": "{kind: redis}", "services.api.behavior.calls[1]": "{target: cache, op: get, ttl_s: 5}"}, []string{"error services.api.behavior.calls[1].ttl_s not-applicable"}},
		{"query on write", map[string]string{"services.api.behavior.calls[1]": "{target: db, op: write, query: scan}"}, []string{"error services.api.behavior.calls[1].query not-applicable"}},
		{"rows without range", map[string]string{"services.api.behavior.calls[1]": "{target: db, op: read, rows: 5}"}, []string{"error services.api.behavior.calls[1].rows not-applicable"}},
		{"nested on_miss error path", map[string]string{"services.cache": "{kind: redis}", "services.api.behavior.calls[1]": "{target: cache, op: get, on_miss: [{target: db, op: read}, {target: nope, op: set}]}"}, []string{"error services.api.behavior.calls[1].on_miss[1].target unknown-service"}},
		{"count zero", map[string]string{"services.api.behavior.calls[0]": "{target: db, op: read, count: 0}"}, []string{"error services.api.behavior.calls[0].count range"}},
		{"http loop", map[string]string{
			"services.api.behavior.calls[1]": "{target: b, op: http}",
			"services.b":                     "{image: loadlore/synthetic, behavior: {calls: [{target: lb, op: http}]}}",
		}, []string{"error services.b.behavior.calls[0].target cycle"}},

		// Load balancers.
		{"lb without target", map[string]string{"services.lb.target": del}, []string{"error services.lb.target required"}},
		{"lb unknown target", map[string]string{"services.lb.target": "apu"}, []string{"error services.lb.target unknown-service"}},
		{"lb in front of a datastore", map[string]string{"services.lb.target": "db"}, []string{"error services.lb.target op-target"}},
		{"lb replicas", map[string]string{"services.lb.replicas": "2"}, []string{"error services.lb.replicas range"}},
		{"lb config", map[string]string{"services.lb.config": "{maxconn: 10}"}, []string{"error services.lb.config not-applicable"}},
		{"lb algorithm", map[string]string{"services.lb.algorithm": "fastest"}, []string{"error services.lb.algorithm invalid-value"}},
		{"lb check interval", map[string]string{"services.lb.health_check": "{interval_ms: 10}"}, []string{"error services.lb.health_check.interval_ms range"}},
		{"lb check path", map[string]string{"services.lb.health_check": "{path: healthz}"}, []string{"error services.lb.health_check.path invalid-value"}},
		{"algorithm on an app", map[string]string{"services.api.algorithm": "least-conn"}, []string{"error services.api.algorithm not-applicable"}},
		{"health check on postgres", map[string]string{"services.db.health_check": "{fall: 2}"}, []string{"error services.db.health_check not-applicable"}},
		{"op on an lb", map[string]string{
			"services.b": "{image: loadlore/synthetic, behavior: {calls: [{target: lb, op: read}]}}",
		}, []string{"error services.b.behavior.calls[0].op op-target"}},
		{"entry without lb", map[string]string{"workload.entry": "api"}, []string{"warning workload.entry no-lb"}},
		{"entry with one replica", map[string]string{"workload.entry": "api", "services.api.replicas": "1"}, []string{}},
		{"call without lb", map[string]string{
			"services.b": "{image: loadlore/synthetic, behavior: {calls: [{target: api, op: http}]}}",
		}, []string{"warning services.b.behavior.calls[0].target no-lb"}},
		{"base_ms with no backoff", map[string]string{"services.api.behavior.resilience": "{retries: {max: 2, base_ms: 10}}"}, []string{"error services.api.behavior.resilience.retries.base_ms not-applicable"}},
		{"retries need max", map[string]string{"services.api.behavior.resilience": "{retries: {backoff: exponential}}"}, []string{"error services.api.behavior.resilience.retries.max required"}},
		{"breaker error rate range", map[string]string{"services.api.behavior.resilience": "{circuit_breaker: {error_rate: 2, window_s: 1, open_s: 1}}"}, []string{"error services.api.behavior.resilience.circuit_breaker.error_rate range"}},

		// Pools.
		{"pool for unknown service", map[string]string{"services.api.pools": "{nope: 3}"}, []string{"error services.api.pools.nope unknown-service"}},
		{"pool never used", map[string]string{"services.cache": "{kind: redis}", "services.api.pools": "{cache: 3}"}, []string{"warning services.api.pools.cache unused-pool"}},
		{"pool size below 1", map[string]string{"services.api.pools": "{db: 0}"}, []string{"error services.api.pools.db range"}},
		{"pool object form", map[string]string{"services.api.pools": "{db: {size: 24, min: 24}}"}, nil},
		{"pool min alone", map[string]string{"services.api.pools": "{db: {min: 5}}"}, nil},
		{"pool min over size", map[string]string{"services.api.pools": "{db: {size: 4, min: 5}}"}, []string{"error services.api.pools.db.min range"}},
		{"pool min over default size", map[string]string{"services.api.pools": "{db: {min: 11}}"}, []string{"error services.api.pools.db.min range"}},
		{"pool min not for http", map[string]string{"services.web": "{image: loadlore/synthetic, behavior: {calls: [{target: api, op: http}]}, pools: {api: {min: 2}}}"}, []string{"error services.web.pools.api.min not-applicable", "warning services.web.behavior.calls[0].target no-lb"}},
		{"pgbouncer owns listen_port", map[string]string{"services.pgb": "{kind: pgbouncer, target: db, config: {listen_port: 7000}}"}, []string{"error services.pgb.config.listen_port not-applicable"}},
		{"pgbouncer pool_mode is a field", map[string]string{"services.pgb": "{kind: pgbouncer, target: db, config: {pool_mode: session}}"}, []string{"error services.pgb.config.pool_mode not-applicable"}},
		{"pgbouncer max_client_conn number", map[string]string{"services.pgb": "{kind: pgbouncer, target: db, config: {max_client_conn: lots}}"}, []string{"error services.pgb.config.max_client_conn invalid-value"}},
		{"pgbouncer other keys pass", map[string]string{"services.pgb": "{kind: pgbouncer, target: db, config: {max_client_conn: 500, server_idle_timeout: 60}}"}, nil},
		{"hold request", map[string]string{"services.api.pools": "{db: {size: 4, hold: request}}"}, nil},
		{"hold not for http", map[string]string{"services.web": "{image: loadlore/synthetic, behavior: {calls: [{target: api, op: http}]}, pools: {api: {hold: request}}}"}, []string{"error services.web.pools.api.hold not-applicable", "warning services.web.behavior.calls[0].target no-lb"}},
		{"hold bad value", map[string]string{"services.api.pools": "{db: {hold: forever}}"}, []string{"error services.api.pools.db.hold invalid-value"}},
		{"pool unknown field", map[string]string{"services.api.pools": "{db: {size: 4, warm: true}}"}, []string{"error services.api.pools.db.warm unknown-field"}},

		// Workload.
		{"unknown entry", map[string]string{"workload.entry": "apo"}, []string{"error workload.entry unknown-service"}},
		{"entry is datastore", map[string]string{"workload.entry": "db"}, []string{"error workload.entry op-target"}},
		{"empty profile", map[string]string{"workload.profile": "[]"}, []string{"error workload.profile range"}},
		{"step with two kinds", map[string]string{"workload.profile[0]": "{hold: {rps: 1, for: 1m}, spike: {to: 5, for: 1s}}"}, []string{"error workload.profile[0] conflict"}},
		{"step with none", map[string]string{"workload.profile[0]": "{}"}, []string{"error workload.profile[0] required"}},
		{"zero-length ramp", map[string]string{"workload.profile[1]": "{ramp: {from: 1, to: 5, over: 0s}}"}, []string{"error workload.profile[1].ramp.over range"}},
		{"mix sums above 1", map[string]string{"workload.mix": "{read: 0.9, write: 0.2}"}, []string{"error workload.mix range"}},
		{"mix out of range", map[string]string{"workload.mix": "{read: 1.5}"}, []string{"error workload.mix.read range"}},
		{"skew without zipf", map[string]string{"workload.keys": "{skew: 1.2}"}, []string{"error workload.keys.skew not-applicable"}},
		{"share without hotspot", map[string]string{"workload.keys": "{distribution: zipf, share: 0.5}"}, []string{"error workload.keys.share not-applicable"}},
		{"keyspace too big", map[string]string{"workload.keys": "{count: 5000}"}, []string{"error workload.keys.count keyspace"}},
		{"keyspace needs dataset", map[string]string{"services.db.dataset": del}, []string{"error workload.keys.count keyspace"}},
		{"keyspace ambiguous", map[string]string{
			"services.db2":                   "{kind: postgres, dataset: {rows: 50}}",
			"services.api.behavior.calls[1]": "{target: db2, op: read}",
		}, []string{"error workload.keys.count keyspace"}},

		// Scenario.
		{"fault after the end", map[string]string{"scenario": "[{at: 2m, inject: kill, target: api}]"}, []string{"error scenario[0].at timing"}},
		{"fault runs past end", map[string]string{"scenario": "[{at: 50s, inject: kill, target: api, for: 30s}]"}, []string{"warning scenario[0].for timing"}},
		{"unknown inject", map[string]string{"scenario": "[{at: 1s, inject: explode, target: api}]"}, []string{"error scenario[0].inject invalid-value"}},
		{"latency needs add_ms", map[string]string{"scenario": "[{at: 1s, inject: latency, target: db}]"}, []string{"error scenario[0].add_ms required"}},
		{"field not for inject", map[string]string{"scenario": "[{at: 1s, inject: kill, target: db, add_ms: 5}]"}, []string{"error scenario[0].add_ms not-applicable"}},
		{"pause needs for", map[string]string{"scenario": "[{at: 1s, inject: pause, target: db}]"}, []string{"error scenario[0].for required"}},
		{"missing target", map[string]string{"scenario": "[{at: 1s, inject: kill}]"}, []string{"error scenario[0].target required"}},
		{"replica out of range", map[string]string{"scenario": "[{at: 1s, inject: kill, target: \"api[2]\"}]"}, []string{"error scenario[0].target range"}},
		{"bad target syntax", map[string]string{"scenario": "[{at: 1s, inject: kill, target: \"api[x]\"}]"}, []string{"error scenario[0].target invalid-value"}},
		{"unknown fault target", map[string]string{"scenario": "[{at: 1s, inject: restart, target: cache}]"}, []string{"error scenario[0].target unknown-service"}},
		{"edge fault on uncalled service", map[string]string{"services.cache": "{kind: redis}", "scenario": "[{at: 1s, inject: latency, target: cache, add_ms: 5}]"}, []string{"error scenario[0].target op-target"}},
		{"disk_slow on app", map[string]string{"scenario": "[{at: 1s, inject: disk_slow, target: api, add_ms: 5}]"}, []string{"error scenario[0].target op-target"}},
		{"partition with target", map[string]string{"scenario": "[{at: 1s, inject: partition, target: db, between: [api, db]}]"}, []string{"error scenario[0].target not-applicable"}},
		{"partition same service", map[string]string{"scenario": "[{at: 1s, inject: partition, between: [db, db]}]"}, []string{"error scenario[0].between invalid-value"}},
		{"partition needs two", map[string]string{"scenario": "[{at: 1s, inject: partition, between: [db]}]"}, []string{"error scenario[0].between range"}},
		{"loss percent range", map[string]string{"scenario": "[{at: 1s, inject: loss, target: db, percent: 150}]"}, []string{"error scenario[0].percent range"}},

		// Expect.
		{"expect syntax", map[string]string{"expect": "[\"p99_ms 300\"]"}, []string{"error expect[0] expect-syntax"}},
		{"expect equals", map[string]string{"expect": "[\"p99_ms = 300\"]"}, []string{"error expect[0] expect-syntax"}},
		{"expect unknown metric", map[string]string{"expect": "[\"p98_ms < 300\"]"}, []string{"error expect[0] expect-metric"}},
		{"expect load_rps as subject", map[string]string{"expect": "[\"load_rps > 5\"]"}, []string{"error expect[0] expect-metric"}},
		{"expect unknown service", map[string]string{"expect": "[\"cache.hit_rate > 80%\"]"}, []string{"error expect[0] unknown-service"}},
		{"expect metric not for kind", map[string]string{"expect": "[\"db.hit_rate > 80%\"]"}, []string{"error expect[0] expect-metric"}},
		{"expect ratio as whole number", map[string]string{"expect": "[\"error_rate < 5\"]"}, []string{"error expect[0] expect-value"}},
		{"expect unit on count", map[string]string{"expect": "[\"db.connections < 5ms\"]"}, []string{"error expect[0] expect-value"}},
		{"expect after past end", map[string]string{"expect": "[\"p99_ms < 5 after 5m\"]"}, []string{"error expect[0] timing"}},
		{"expect bad note", map[string]string{"expect": "[{note: x, extra: y}]"}, []string{"error expect[0] invalid-value"}},
		{"expect until metric checked", map[string]string{"expect": "[\"p99_ms < 5 until loadrps >= 5\"]"}, []string{"error expect[0] expect-metric"}},
		{"expect run metric compared", map[string]string{"expect": "[\"break_load_rps < 1000\"]"}, []string{"error expect[0] expect-metric"}},
		{"expect within on a window metric", map[string]string{"expect": "[\"p99_ms within 25% of 300\"]"}, []string{"error expect[0] expect-metric"}},
		{"expect tolerance not a percentage", map[string]string{"expect": "[\"break_load_rps within 25 of 1000\"]"}, []string{"error expect[0] expect-value"}},
		{"expect tolerance too big", map[string]string{"expect": "[\"break_load_rps within 150% of 1000\"]"}, []string{"error expect[0] expect-value"}},

		// Assumptions and modes.
		{"assumption bad basis", map[string]string{"assumptions": "[{field: workload.mix, basis: hunch}]"}, []string{"error assumptions[0].basis invalid-value"}},
		{"assumption missing basis", map[string]string{"assumptions": "[{field: workload.mix}]"}, []string{"error assumptions[0].basis required"}},
		{"assumption unknown field", map[string]string{"assumptions": "[{field: services.api.behavior.cpu_msec, basis: guessed}]"}, []string{"warning assumptions[0].field assumption-path"}},
		{"assumption unknown service", map[string]string{"assumptions": "[{field: services.web.replicas, basis: guessed}]"}, []string{"warning assumptions[0].field assumption-path"}},
		{"assumption index out of range", map[string]string{"assumptions": "[{field: \"services.api.behavior.calls[3].count\", basis: guessed}]"}, []string{"warning assumptions[0].field assumption-path"}},
		{"scale above 1", map[string]string{"modes": "{local: {scale: 2}}"}, []string{"warning modes.local.scale scale"}},
		{"scale zero", map[string]string{"modes": "{local: {scale: 0}}"}, []string{"error modes.local.scale range"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := patch(t, tt.set)
			_, diags := Load(src)
			got := diagKeys(diags)
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("diagnostics:\n got  %q\n want %q\nmessages:\n%s\nyaml:\n%s", got, want, diags, src)
			}
			for _, d := range diags {
				if d.Message == "" {
					t.Errorf("empty message for %s", d.Path)
				}
				if d.Line == 0 && d.Code != CodeSyntax {
					t.Errorf("no position for %s %s", d.Path, d.Code)
				}
			}
		})
	}
}

func TestValidAssumptionPaths(t *testing.T) {
	for _, field := range []string{
		"services.api.behavior.cpu_ms",
		"services.api.behavior.calls[0].count",
		"services.api.resources.cpu",  // defaulted, not in the file
		"services.db.config.work_mem", // any native config key
		"services.api.pools.db",       // defaulted pool
		"workload.mix",                // a whole block
		"workload.keys.count",         // set by the keyspace default
		"services.api.behavior.resilience.retries.max",
	} {
		src := patch(t, map[string]string{"assumptions": fmt.Sprintf("[{field: %q, basis: guessed}]", field)})
		if _, diags := Load(src); len(diags) != 0 {
			t.Errorf("%s: unexpected diagnostics %v", field, diags)
		}
	}
}

func TestDiagnosticPositions(t *testing.T) {
	src := `version: 1
name: pos
services:
  api:
    image: loadlore/synthetic
    behavior:
      calls:
        - { target: db, op: read }
        - { target: dbb, op: read }
  db: { kind: postgres, dataset: { rows: 10 } }
workload:
  entry: api
  profile: [ { hold: { rps: 1, for: 1m } } ]
expect:
  - p99_ms < 300 until loadrps >= 3
  - "error_rate < 5 throughout"
`
	_, diags := Load([]byte(src))
	want := []Diagnostic{
		{Severity: SevError, Path: "services.api.behavior.calls[1].target", Line: 9, Column: 21, Code: CodeUnknownService},
		{Severity: SevError, Path: "expect[0]", Line: 15, Column: 24, Code: CodeExpectMetric},
		{Severity: SevError, Path: "expect[1]", Line: 16, Column: 19, Code: CodeExpectValue},
	}
	if len(diags) != len(want) {
		t.Fatalf("got %d diagnostics, want %d:\n%s", len(diags), len(want), diags)
	}
	for i, w := range want {
		g := diags[i]
		if g.Path != w.Path || g.Line != w.Line || g.Column != w.Column || g.Code != w.Code {
			t.Errorf("diagnostic %d: got %s %d:%d %s, want %s %d:%d %s", i, g.Path, g.Line, g.Column, g.Code, w.Path, w.Line, w.Column, w.Code)
		}
	}
	if !strings.Contains(diags[0].Message, `did you mean "db"?`) {
		t.Errorf("no suggestion in %q", diags[0].Message)
	}
}

func TestSyntaxError(t *testing.T) {
	_, diags := Load([]byte("version: 1\nservices:\n  api: [\n"))
	if len(diags) != 1 || diags[0].Code != CodeSyntax || diags[0].Line == 0 {
		t.Fatalf("got %v", diags)
	}
	_, diags = Load(nil)
	if len(diags) != 1 || diags[0].Code != CodeSyntax {
		t.Fatalf("empty file: got %v", diags)
	}
}

func TestDefaults(t *testing.T) {
	d, diags := Load(patch(t, map[string]string{
		"services.cache":                   "{kind: redis}",
		"services.api.behavior.calls[1]":   "{target: db, op: read, query: range}",
		"services.api.behavior.calls[2]":   "{target: cache, op: get}",
		"services.pgb":                     "{kind: pgbouncer, target: db}",
		"services.api.behavior.resilience": "{retries: {max: 2, backoff: exponential}}",
		"workload.mix":                     "{write: 0.25}",
		"workload.keys":                    "{distribution: hotspot}",
	}))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	api, db, cache, pgb, lb := d.Services["api"], d.Services["db"], d.Services["cache"], d.Services["pgb"], d.Services["lb"]
	checks := []struct {
		name      string
		got, want any
	}{
		{"api cpu", *api.Resources.CPU, DefaultCPU},
		{"api memory", api.Resources.Memory.String(), "512m"},
		{"api role", api.Role, "server"},
		{"concurrency", api.Concurrency.Model + "/" + strconv.Itoa(*api.Concurrency.Max), "threads/200"},
		{"cpu jitter", *api.Behavior.CPUJitter, DefaultCPUJitter},
		{"timeout", *api.Behavior.Resilience.TimeoutMs, DefaultTimeoutMs},
		{"retry base", *api.Behavior.Resilience.Retries.BaseMs, DefaultBackoffBaseMs},
		{"call count", *api.Behavior.Calls[0].Count, 1},
		{"call when", api.Behavior.Calls[0].When, "always"},
		{"call mode", api.Behavior.Calls[0].Mode, "sync"},
		{"read query", api.Behavior.Calls[0].Query, "point"},
		{"range rows", *api.Behavior.Calls[1].Rows, DefaultRangeRows},
		{"get has no query", api.Behavior.Calls[2].Query, ""},
		{"pool db", *api.Pools["db"].Size, DefaultPool},
		{"pool db min", *api.Pools["db"].Min, 0},
		{"pool cache", *api.Pools["cache"].Size, DefaultPool},
		{"postgres version", db.Version, "17"},
		{"row bytes", *db.Dataset.RowBytes, DefaultRowBytes},
		{"redis version", cache.Version, "8"},
		{"pgbouncer mode", pgb.PoolMode, "transaction"},
		{"pgbouncer pool", *pgb.DefaultPoolSize, DefaultPgbPoolSize},
		{"lb algorithm", lb.Algorithm, "round-robin"},
		{"lb check", fmt.Sprintf("%s %d %d %d %d", lb.HealthCheck.Path, *lb.HealthCheck.IntervalMs, *lb.HealthCheck.TimeoutMs, *lb.HealthCheck.Fall, *lb.HealthCheck.Rise), "/healthz 2000 1000 3 2"},
		{"lb resources", fmt.Sprintf("%v %s", *lb.Resources.CPU, lb.Resources.Memory), "0.5 256m"},
		{"mix read", *d.Workload.Mix.Read, 0.75},
		{"hotspot share", *d.Workload.Keys.Share, DefaultHotspotShare},
		{"hotspot fraction", *d.Workload.Keys.Fraction, DefaultHotspotFraction},
		{"keyspace", *d.Workload.Keys.Count, int64(1000)},
		{"payload", *d.Workload.PayloadKB, DefaultPayloadKB},
		{"client timeout", *d.Workload.TimeoutMs, DefaultClientTimeoutMs},
		{"scale", *d.Modes.Local.Scale, DefaultScale},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
	if d.Workload.Keys.Skew != nil {
		t.Error("skew set for hotspot")
	}
}

func TestDefaultMixPrintsCleanly(t *testing.T) {
	d, _ := Load(patch(t, nil))
	if got := strconv.FormatFloat(*d.Workload.Mix.Write, 'f', -1, 64); got != "0.1" {
		t.Errorf("write share %s, want 0.1", got)
	}
}

func TestSpecExampleIsValid(t *testing.T) {
	d, diags, err := LoadFile("../../examples/single-postgres.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got := d.TotalDuration().String(); got != "7m0s" {
		t.Errorf("total duration %s", got)
	}
	if p := d.Expect[0].Prediction; p.Op != OpWithin || p.Value.Canonical != 3000 || p.Tolerance.Canonical != 0.25 {
		t.Errorf("expect[0] parsed as %+v", p)
	}
	if p := d.Expect[1].Prediction; p.Op != OpNone || p.Subject.Metric != "errors_load_rps" {
		t.Errorf("expect[1] parsed as %+v", p)
	}
}
