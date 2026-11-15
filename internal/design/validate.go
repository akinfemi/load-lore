package design

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// validator runs the semantic checks that struct tags can't express:
// references between services, which fields apply where, the call graph,
// the keyspace, scenario timing, expect metrics and assumption paths.
type validator struct {
	d      *Design
	nodes  map[string]nodePos
	failed map[string]bool // paths the decoder already rejected
	diags  Diagnostics
}

func (v *validator) report(sev Severity, path string, col func(*yaml.Node) int, code, format string, args ...any) {
	if v.failed[path] {
		return
	}
	diag := Diagnostic{Severity: sev, Path: path, Code: code, Message: fmt.Sprintf(format, args...)}
	if n, isVal := v.nodeFor(path); n != nil {
		diag.Line, diag.Column = n.Line, n.Column
		if col != nil && isVal {
			diag.Column = col(n)
		}
	}
	v.diags = append(v.diags, diag)
}

func (v *validator) errorf(path, code, format string, args ...any) {
	v.report(SevError, path, nil, code, format, args...)
}

func (v *validator) warnf(path, code, format string, args ...any) {
	v.report(SevWarning, path, nil, code, format, args...)
}

// nodeFor finds the YAML node for path, or for its nearest ancestor that is
// in the file. isVal is true when the node is path's own value node.
func (v *validator) nodeFor(path string) (n *yaml.Node, isVal bool) {
	for p := path; ; p = parentPath(p) {
		if np, ok := v.nodes[p]; ok {
			if p == path {
				return np.val, true
			}
			if np.key != nil {
				return np.key, false
			}
			return np.val, false
		}
		if p == "" {
			return nil, false
		}
	}
}

func parentPath(p string) string {
	if strings.HasSuffix(p, "]") {
		if i := strings.LastIndexByte(p, '['); i >= 0 {
			return p[:i]
		}
	}
	if i := strings.LastIndexByte(p, '.'); i >= 0 {
		return p[:i]
	}
	return ""
}

var serviceNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func (v *validator) validate() {
	names := v.d.ServiceNames()
	for _, name := range names {
		v.service(name, v.d.Services[name])
	}
	v.cycles()
	if v.d.Workload != nil {
		v.workload()
	}
	v.scenario()
	v.expect()
	v.assumptions()
	if m := v.d.Modes; m != nil && m.Local != nil && m.Local.Scale != nil && *m.Local.Scale > 1 {
		v.warnf("modes.local.scale", CodeScale, "scale %s grows resources and load beyond the design; scale is meant to shrink them", fmtNum(*m.Local.Scale))
	}
}

// --- services ---

func (v *validator) service(name string, s *Service) {
	p := pathKey("services", name)
	if !serviceNameRe.MatchString(name) || len(name) > 63 {
		v.errorf(p, CodeInvalidValue, "service name %q isn't valid: use lowercase letters, digits and '-'", name)
	}
	if v.failed[pathKey(p, "image")] || v.failed[pathKey(p, "kind")] {
		return // already reported; anything more would be noise
	}
	if s == nil {
		v.errorf(p, CodeRequired, "service %q needs either image (an application service) or kind (postgres, redis, pgbouncer, lb)", name)
		return
	}
	switch {
	case s.Image != "" && s.Kind != "":
		v.errorf(pathKey(p, "kind"), CodeConflict, "set either image or kind, not both")
		return
	case s.Image == "" && s.Kind == "":
		v.errorf(p, CodeRequired, "service %q needs either image (an application service) or kind (postgres, redis, pgbouncer, lb)", name)
		return
	}

	notFor := func(field string, set bool, why string) {
		if set {
			v.errorf(pathKey(p, field), CodeNotApplicable, "%s doesn't apply here: %s", field, why)
		}
	}
	if s.IsApp() {
		why := "it is for datastores and infrastructure (kind)"
		notFor("version", s.Version != "", why)
		notFor("config", s.Config != nil, why)
		notFor("dataset", s.Dataset != nil, why)
		notFor("disk", s.Disk != nil, "it is for postgres")
		notFor("primary", s.Primary != "", "it is for postgres-replica")
		notFor("target", s.Target != "", "it is for pgbouncer and lb")
		notFor("pool_mode", s.PoolMode != "", "it is for pgbouncer")
		notFor("default_pool_size", s.DefaultPoolSize != nil, "it is for pgbouncer")
		notFor("algorithm", s.Algorithm != "", "it is for lb")
		notFor("health_check", s.HealthCheck != nil, "it is for lb")
		if s.Behavior != nil && !s.IsSynthetic() {
			v.errorf(pathKey(p, "behavior"), CodeNotApplicable, "behavior only applies to %s; your own image does its own work", SyntheticImage)
		}
		if s.Behavior != nil {
			v.calls(name, pathKey(p, "behavior.calls"), s.Behavior.Calls)
			v.resilience(pathKey(p, "behavior.resilience"), s.Behavior.Resilience)
		}
		v.pools(name, p, s)
		if c := s.Concurrency; c != nil && c.Model == "workers" && c.Max != nil && *c.Max < 1 {
			v.errorf(pathKey(p, "concurrency.max"), CodeRange, "workers needs at least 1 worker")
		}
		return
	}

	why := "it is for application services (image)"
	notFor("role", s.Role != "", why)
	notFor("behavior", s.Behavior != nil, why)
	notFor("pools", s.Pools != nil, why)
	notFor("concurrency", s.Concurrency != nil, why)
	notFor("dataset", s.Dataset != nil && s.Kind == "postgres-replica", "a replica starts from a copy of its primary's data, so the dataset goes on the primary")
	notFor("dataset", s.Dataset != nil && !s.IsPostgres(), "datasets are seeded into postgres")
	notFor("primary", s.Primary != "" && s.Kind != "postgres-replica", "it is for postgres-replica")
	if s.Kind == "postgres-replica" {
		v.replica(name, p, s)
	}
	notFor("disk", s.Disk != nil && s.Kind != "postgres", "it is for postgres")
	if dk := s.Disk; dk != nil && s.Kind == "postgres" {
		if dk.ReadMs != nil && (*dk.ReadMs < 0 || *dk.ReadMs > 100) {
			v.errorf(pathKey(p, "disk.read_ms"), CodeRange, "read_ms must be 0 to 100 whole milliseconds, not %d", *dk.ReadMs)
		}
		if dk.IOPS != nil && *dk.IOPS < 100 {
			v.errorf(pathKey(p, "disk.iops"), CodeRange, "iops must be at least 100, not %d", *dk.IOPS)
		}
	}
	if s.Kind != "pgbouncer" && s.Kind != "lb" {
		notFor("target", s.Target != "", "it is for pgbouncer and lb")
	}
	if s.Kind != "pgbouncer" {
		notFor("pool_mode", s.PoolMode != "", "it is for pgbouncer")
		notFor("default_pool_size", s.DefaultPoolSize != nil, "it is for pgbouncer")
	}
	if s.Kind != "lb" {
		notFor("algorithm", s.Algorithm != "", "it is for lb")
		notFor("health_check", s.HealthCheck != nil, "it is for lb")
	} else {
		notFor("version", s.Version != "", "the lb's version is pinned by Loadlore")
		notFor("config", s.Config != nil, "set algorithm and health_check instead")
	}
	if s.Replicas != nil && *s.Replicas != 1 && s.Kind != "postgres-replica" {
		msg := fmt.Sprintf("%s runs exactly one replica", s.Kind)
		if s.Kind == "postgres" {
			msg += "; for read replicas, add a kind: postgres-replica service with primary: " + name
		}
		v.errorf(pathKey(p, "replicas"), CodeRange, "%s", msg)
	}
	if s.Kind == "lb" {
		switch t, ok := v.d.Services[s.Target]; {
		case s.Target == "":
			v.errorf(pathKey(p, "target"), CodeRequired, "lb needs a target: the application service whose replicas it spreads traffic across")
		case !ok:
			v.unknownService(pathKey(p, "target"), s.Target)
		case v.broken(s.Target):
		case t == nil || !t.IsApp():
			v.errorf(pathKey(p, "target"), CodeOpTarget, "an lb's target must be an application service; %q is %s", s.Target, describeService(t))
		}
	}
	if s.Kind == "pgbouncer" {
		switch t, ok := v.d.Services[s.Target]; {
		case s.Target == "":
			v.errorf(pathKey(p, "target"), CodeRequired, "pgbouncer needs a target: the postgres service it pools connections to")
		case !ok:
			v.unknownService(pathKey(p, "target"), s.Target)
		case t == nil || !t.IsPostgres():
			v.errorf(pathKey(p, "target"), CodeOpTarget, "pgbouncer's target must be a postgres or postgres-replica service; %q isn't", s.Target)
		case t.Replicas != nil && *t.Replicas > 1:
			v.errorf(pathKey(p, "target"), CodeOpTarget, "pgbouncer connects to one server, and %q has %d replicas; give each replica its own postgres-replica service, or call the replicas directly", s.Target, *t.Replicas)
		}
		for _, k := range sortedKeys(s.Config) {
			kp := pathKey(pathKey(p, "config"), k)
			if why, ok := pgbouncerOwnKeys[k]; ok {
				v.errorf(kp, CodeNotApplicable, "%s can't be set in config: %s", k, why)
			}
			if k == "max_client_conn" {
				if n, err := strconv.Atoi(s.Config[k].String()); err != nil || n < 1 {
					v.errorf(kp, CodeInvalidValue, "max_client_conn must be a whole number of 1 or more, not %q", s.Config[k].String())
				}
			}
		}
	}
	if ds := s.Dataset; ds != nil {
		for i, col := range ds.Indexes {
			if col != "id" && col != "user_id" {
				v.errorf(pathIndex(pathKey(p, "dataset.indexes"), i), CodeInvalidValue, "unknown column %q; the seeded table has id and user_id", col)
			}
		}
	}
}

// broken reports whether a service's image or kind is already in error, so
// checks that depend on what the service is would only add noise.
func (v *validator) broken(name string) bool {
	p := pathKey("services", name)
	s := v.d.Services[name]
	return v.failed[pathKey(p, "image")] || v.failed[pathKey(p, "kind")] || (s != nil && s.Image != "" && s.Kind != "")
}

func (v *validator) unknownService(path, name string) {
	msg := fmt.Sprintf("unknown service %q", name)
	if s := suggest(name, v.d.ServiceNames()); s != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", s)
	}
	v.errorf(path, CodeUnknownService, "%s", msg)
}

// opsFor lists the call ops a target accepts.
func opsFor(t *Service) []string {
	switch {
	case t.IsApp(), t.Kind == "lb":
		return []string{"http"}
	case t.Kind == "postgres", t.Kind == "pgbouncer":
		return []string{"read", "write"}
	case t.Kind == "postgres-replica":
		return []string{"read"}
	case t.Kind == "redis":
		return []string{"get", "set", "del"}
	}
	return nil
}

func (v *validator) calls(caller, path string, calls []*Call) {
	for i, c := range calls {
		cp := pathIndex(path, i)
		if c == nil {
			v.errorf(cp, CodeRequired, "empty call: needs target and op")
			continue
		}
		v.call(caller, cp, c)
	}
}

func (v *validator) call(caller, p string, c *Call) {
	if c.Target != "" {
		t, ok := v.d.Services[c.Target]
		switch {
		case c.Target == caller:
			v.errorf(pathKey(p, "target"), CodeCycle, "a service can't call itself")
		case !ok:
			v.unknownService(pathKey(p, "target"), c.Target)
		case t != nil && c.Op != "":
			if ops := opsFor(t); ops != nil && !slices.Contains(ops, c.Op) && !v.failed[pathKey(p, "op")] {
				if t.Kind == "postgres-replica" {
					v.errorf(pathKey(p, "op"), CodeOpTarget, "%s is a read replica and serves reads only; send writes to its primary, %s", c.Target, t.Primary)
				} else {
					v.errorf(pathKey(p, "op"), CodeOpTarget, "%s is %s; use op %s", c.Target, describeService(t), strings.Join(ops, ", "))
				}
			}
			if t.IsApp() && t.Replicas != nil && *t.Replicas > 1 {
				v.warnf(pathKey(p, "target"), CodeNoLB, "%s", noLBMessage(caller, c.Target, *t.Replicas))
			}
		}
	}
	if len(c.OnMiss) > 0 && c.Op != "get" {
		v.errorf(pathKey(p, "on_miss"), CodeNotApplicable, "on_miss only applies to op get")
	}
	if c.Coalesce && (c.Op != "get" || len(c.OnMiss) == 0) {
		v.errorf(pathKey(p, "coalesce"), CodeNotApplicable, "coalesce only applies to op get with on_miss calls")
	}
	if c.TTLS != nil && c.Op != "set" {
		v.errorf(pathKey(p, "ttl_s"), CodeNotApplicable, "ttl_s only applies to op set")
	}
	if c.Query != "" && c.Op != "read" {
		v.errorf(pathKey(p, "query"), CodeNotApplicable, "query only applies to op read")
	}
	if c.Rows != nil && c.Query != "range" && c.Query != "batch" {
		v.errorf(pathKey(p, "rows"), CodeNotApplicable, "rows only applies to query: range and query: batch")
	}
	v.calls(caller, pathKey(p, "on_miss"), c.OnMiss)
}

func describeService(s *Service) string {
	switch {
	case s == nil:
		return "empty"
	case s.IsApp():
		return "an application service"
	case s.Kind == "lb":
		return "a load balancer"
	}
	return "a " + s.Kind + " service"
}

// noLBMessage explains client-side spreading: without an lb, callers
// round-robin across replica addresses and never notice a dead replica.
func noLBMessage(caller, target string, replicas int) string {
	return fmt.Sprintf("%s has %d replicas and no load balancer in front, so %s spreads requests across them itself, with no health checks: a dead replica keeps getting its share. Put a kind: lb service in front to model a load balancer", target, replicas, caller)
}

func (v *validator) resilience(p string, r *Resilience) {
	if r == nil || r.Retries == nil {
		return
	}
	if r.Retries.BaseMs != nil && r.Retries.Backoff == "none" {
		v.errorf(pathKey(p, "retries.base_ms"), CodeNotApplicable, "base_ms only applies to backoff constant or exponential")
	}
	if r.Retries.Jitter && r.Retries.Backoff == "none" {
		v.errorf(pathKey(p, "retries.jitter"), CodeNotApplicable, "jitter only applies to backoff constant or exponential")
	}
}

func (v *validator) pools(name, p string, s *Service) {
	targets := map[string]bool{}
	if s.Behavior != nil {
		walkCalls(s.Behavior.Calls, func(c *Call) { targets[c.Target] = true })
	}
	for _, t := range sortedKeys(s.Pools) {
		pp := pathKey(pathKey(p, "pools"), t)
		pool := s.Pools[t]
		if pool.Size != nil && *pool.Size < 1 {
			v.errorf(pp, CodeRange, "pool size must be at least 1")
		}
		ts, ok := v.d.Services[t]
		if !ok {
			v.unknownService(pp, t)
			continue
		}
		if pool.Hold == "request" && !ts.IsPostgres() && ts.Kind != "pgbouncer" {
			v.errorf(pathKey(pp, "hold"), CodeNotApplicable, "hold: request only applies to postgres and pgbouncer targets; %s is %s", t, describeKind(ts))
		}
		if pool.Min != nil && *pool.Min > 0 {
			switch {
			case !ts.IsPostgres() && ts.Kind != "pgbouncer":
				v.errorf(pathKey(pp, "min"), CodeNotApplicable, "a pool minimum only applies to postgres and pgbouncer targets; %s is %s", t, describeKind(ts))
			case pool.Size != nil && *pool.Min > *pool.Size:
				v.errorf(pathKey(pp, "min"), CodeRange, "min %d is more than the pool's size, %d", *pool.Min, *pool.Size)
			}
		}
		if !targets[t] {
			v.warnf(pp, CodeUnusedPool, "%s has a pool for %s but never calls it", name, t)
		}
	}
}

// cycles reports loops in the http call graph between application services.
func (v *validator) cycles() {
	type edge struct{ to, path string }
	graph := map[string][]edge{}
	for _, name := range v.d.ServiceNames() {
		s := v.d.Services[name]
		if s == nil || s.Behavior == nil {
			continue
		}
		var visit func(path string, calls []*Call)
		visit = func(path string, calls []*Call) {
			for i, c := range calls {
				if c == nil {
					continue
				}
				cp := pathIndex(path, i)
				t, ok := v.d.Services[c.Target]
				to := c.Target
				if ok && t != nil && t.Kind == "lb" {
					to = t.Target
					t, ok = v.d.Services[to]
				}
				if ok && t != nil && t.IsApp() && to != name {
					graph[name] = append(graph[name], edge{to, pathKey(cp, "target")})
				}
				visit(pathKey(cp, "on_miss"), c.OnMiss)
			}
		}
		visit(pathKey(pathKey("services", name), "behavior.calls"), s.Behavior.Calls)
	}

	const (
		unseen = iota
		active
		done
	)
	state := map[string]int{}
	var stack []string
	reported := map[string]bool{}
	var dfs func(n string)
	dfs = func(n string) {
		state[n] = active
		stack = append(stack, n)
		for _, e := range graph[n] {
			switch state[e.to] {
			case unseen:
				dfs(e.to)
			case active:
				i := slices.Index(stack, e.to)
				loop := append(slices.Clone(stack[i:]), e.to)
				if !reported[e.path] {
					reported[e.path] = true
					v.errorf(e.path, CodeCycle, "calls form a loop (%s), so every request would recurse forever", strings.Join(loop, " → "))
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[n] = done
	}
	for _, name := range v.d.ServiceNames() {
		if state[name] == unseen {
			dfs(name)
		}
	}
}

// --- workload ---

func (v *validator) workload() {
	w := v.d.Workload
	if w.Entry != "" {
		switch s, ok := v.d.Services[w.Entry]; {
		case !ok:
			v.unknownService("workload.entry", w.Entry)
		case s != nil && s.Kind != "" && s.Kind != "lb" && s.Image == "":
			v.errorf("workload.entry", CodeOpTarget, "the entry must be an application service (image) or an lb; %s is %s", w.Entry, describeService(s))
		case s != nil && s.IsApp() && s.Replicas != nil && *s.Replicas > 1:
			v.warnf("workload.entry", CodeNoLB, "%s", noLBMessage("the load generator", w.Entry, *s.Replicas))
		}
	}
	for i, st := range w.Profile {
		p := pathIndex("workload.profile", i)
		if st == nil {
			v.errorf(p, CodeRequired, "each step needs one of ramp, hold or spike")
			continue
		}
		n := 0
		for _, set := range []bool{st.Ramp != nil, st.Hold != nil, st.Spike != nil} {
			if set {
				n++
			}
		}
		switch {
		case n == 0:
			v.errorf(p, CodeRequired, "each step needs one of ramp, hold or spike")
		case n > 1:
			v.errorf(p, CodeConflict, "each step takes exactly one of ramp, hold or spike; put them in separate steps")
		}
		if st.Ramp != nil && st.Ramp.Over <= 0 {
			v.errorf(pathKey(p, "ramp.over"), CodeRange, "must be longer than 0s")
		}
		if st.Hold != nil && st.Hold.For <= 0 {
			v.errorf(pathKey(p, "hold.for"), CodeRange, "must be longer than 0s")
		}
		if st.Spike != nil && st.Spike.For <= 0 {
			v.errorf(pathKey(p, "spike.for"), CodeRange, "must be longer than 0s")
		}
	}
	if m := w.Mix; m != nil && m.Read != nil && m.Write != nil {
		if sum := *m.Read + *m.Write; math.Abs(sum-1) > 1e-6 {
			v.errorf("workload.mix", CodeRange, "read and write must add up to 1, found %s", fmtNum(math.Round(sum*1e6)/1e6))
		}
	}
	if k := w.Keys; k != nil {
		if k.Skew != nil && k.Distribution != "zipf" {
			v.errorf("workload.keys.skew", CodeNotApplicable, "skew only applies to distribution zipf")
		}
		if k.Share != nil && k.Distribution != "hotspot" {
			v.errorf("workload.keys.share", CodeNotApplicable, "share only applies to distribution hotspot")
		}
		if k.Fraction != nil && k.Distribution != "hotspot" {
			v.errorf("workload.keys.fraction", CodeNotApplicable, "fraction only applies to distribution hotspot")
		}
		v.keyspace(k)
	}
}

// keyspace resolves workload.keys.count: it defaults to the dataset rows of
// the datastore the entry path reads, is required when that's ambiguous, and
// can't exceed that row count.
func (v *validator) keyspace(k *Keys) {
	path := v.d.EntryPath()
	if !path.Keyed {
		return
	}
	rows := map[int64][]string{}
	for _, name := range path.Read {
		if ds := v.d.Services[name].Dataset; ds != nil {
			rows[ds.Rows] = append(rows[ds.Rows], name)
		}
	}
	var noData []string
	for _, name := range path.Read {
		if v.d.Services[name].Dataset == nil {
			noData = append(noData, name)
		}
	}

	if k.Count == nil {
		switch {
		case len(rows) == 1:
			for r := range rows {
				k.Count = ptr(r)
			}
		case len(rows) > 1:
			var parts []string
			for _, r := range sortedKeys(rows) {
				parts = append(parts, fmt.Sprintf("%s (%d rows)", strings.Join(rows[r], ", "), r))
			}
			v.errorf("workload.keys.count", CodeKeyspace, "set workload.keys.count: the entry path reads datasets of different sizes: %s", strings.Join(parts, "; "))
		default:
			why := "nothing on the entry path has a dataset"
			if len(noData) > 0 {
				why = strings.Join(noData, ", ") + " has no dataset"
			}
			v.errorf("workload.keys.count", CodeKeyspace, "set workload.keys.count, or give the datastore a dataset: %s", why)
		}
		return
	}
	for _, r := range sortedKeys(rows) {
		if *k.Count > r {
			v.errorf("workload.keys.count", CodeKeyspace, "count %d is more than the %d rows in %s's dataset", *k.Count, r, strings.Join(rows[r], ", "))
			return
		}
	}
}

// --- scenario ---

var faultFields = map[string][]string{
	"latency":   {"add_ms", "jitter_ms"},
	"loss":      {"percent"},
	"bandwidth": {"kbps"},
	"timeout":   {},
	"reset":     {},
	"kill":      {},
	"pause":     {},
	"restart":   {},
	"partition": {"between"},
	"disk_slow": {"add_ms"},
}

var faultRequired = map[string]string{
	"latency": "add_ms", "loss": "percent", "bandwidth": "kbps", "partition": "between", "disk_slow": "add_ms",
}

// edgeFaults act on connections into the target, through Toxiproxy or netem.
var edgeFaults = map[string]bool{"latency": true, "loss": true, "bandwidth": true, "timeout": true, "reset": true}

func (v *validator) scenario() {
	total := v.d.TotalDuration()
	for i, f := range v.d.Scenario {
		p := pathIndex("scenario", i)
		if f == nil {
			v.errorf(p, CodeRequired, "empty fault: needs at, inject and target")
			continue
		}
		if f.Inject == "" {
			continue
		}
		set := map[string]bool{
			"add_ms": f.AddMs != nil, "jitter_ms": f.JitterMs != nil, "percent": f.Percent != nil,
			"kbps": f.Kbps != nil, "between": f.Between != nil,
		}
		allowed := faultFields[f.Inject]
		for _, field := range []string{"add_ms", "jitter_ms", "percent", "kbps", "between"} {
			if set[field] && !slices.Contains(allowed, field) {
				v.errorf(pathKey(p, field), CodeNotApplicable, "%s doesn't apply to inject %s", field, f.Inject)
			}
		}
		if req, ok := faultRequired[f.Inject]; ok && !set[req] {
			v.errorf(pathKey(p, req), CodeRequired, "inject %s needs %s", f.Inject, req)
		}
		if f.Inject == "pause" && f.For == nil {
			v.errorf(pathKey(p, "for"), CodeRequired, "inject pause needs for: a paused container must resume")
		}

		if f.Inject == "partition" {
			if f.Target != nil {
				v.errorf(pathKey(p, "target"), CodeNotApplicable, "partition uses between: [a, b] instead of target")
			}
			var names []string
			for j, side := range f.Between {
				at := pathIndex(pathKey(p, "between"), j)
				ref, err := ParseTargetRef(side)
				if err != nil {
					v.errorf(at, CodeInvalidValue, "%v", err)
					continue
				}
				names = append(names, ref.Name)
				t, ok := v.d.Services[ref.Name]
				if !ok {
					v.unknownService(at, ref.Name)
					continue
				}
				if t != nil && ref.Index != nil && t.Replicas != nil && *ref.Index >= *t.Replicas {
					v.errorf(at, CodeRange, "%s has %d replica%s, so the index must be below %d", ref.Name, *t.Replicas, plural(*t.Replicas, "", "s"), *t.Replicas)
				}
			}
			if len(names) == 2 && names[0] == names[1] {
				v.errorf(pathKey(p, "between"), CodeInvalidValue, "a partition needs two different services")
			}
		} else {
			v.faultTarget(p, f)
		}

		if total > 0 && f.At.D() >= total {
			v.errorf(pathKey(p, "at"), CodeTiming, "starts at %s, but the workload ends at %s, so it would never fire", f.At, Duration(total))
		}
		if f.For != nil {
			if *f.For <= 0 {
				v.errorf(pathKey(p, "for"), CodeRange, "must be longer than 0s")
			} else if total > 0 && f.At.D() < total && f.At.D()+f.For.D() > total {
				v.warnf(pathKey(p, "for"), CodeTiming, "runs past the end of the workload at %s; it ends with the run", Duration(total))
			}
		}
	}
}

func (v *validator) faultTarget(p string, f *Fault) {
	if f.Target == nil {
		v.errorf(pathKey(p, "target"), CodeRequired, "inject %s needs a target", f.Inject)
		return
	}
	t, ok := v.d.Services[f.Target.Name]
	if !ok {
		v.unknownService(pathKey(p, "target"), f.Target.Name)
		return
	}
	if t == nil {
		return
	}
	if f.Target.Index != nil && t.Replicas != nil && *f.Target.Index >= *t.Replicas {
		v.errorf(pathKey(p, "target"), CodeRange, "%s has %d replica%s, so the index must be below %d",
			f.Target.Name, *t.Replicas, plural(*t.Replicas, "", "s"), *t.Replicas)
	}
	if f.Inject == "disk_slow" && t.IsApp() {
		v.errorf(pathKey(p, "target"), CodeOpTarget, "disk_slow targets a datastore; %s is an application service", f.Target.Name)
	}
	if f.Inject == "disk_slow" && t.Kind == "postgres-replica" {
		v.errorf(pathKey(p, "target"), CodeOpTarget, "disk_slow slows a primary's disk; slowing a read replica's isn't supported yet")
	}
	if edgeFaults[f.Inject] && f.Target.Name != v.d.entry() && !v.d.isCalled(f.Target.Name) {
		v.errorf(pathKey(p, "target"), CodeOpTarget, "nothing calls %s, so there is no connection for %s to act on", f.Target.Name, f.Inject)
	}
}

// --- expect ---

func (v *validator) expect() {
	total := v.d.TotalDuration()
	for i, e := range v.d.Expect {
		p := pathIndex("expect", i)
		if e == nil || e.Prediction == nil {
			continue
		}
		pr := e.Prediction
		_, runMetric := RunMetrics[pr.Subject.Metric]
		runMetric = runMetric && pr.Subject.Service == ""
		switch {
		case runMetric && pr.Op != OpWithin && pr.Op != OpNone:
			v.exprErr(p, pr.Subject.Offset, CodeExpectMetric, "%s is the whole run's: predict it as %s within 25%% of <rps>, or %s none", pr.Subject.Metric, pr.Subject.Metric, pr.Subject.Metric)
			continue
		case runMetric:
			if pr.Op == OpWithin {
				v.quantity(p, MetricRate, pr.Subject, &pr.Value)
				if t := &pr.Tolerance; t.Unit != "%" || t.Number <= 0 || t.Number > 100 {
					v.exprErr(p, t.Offset, CodeExpectValue, "the tolerance is a percentage above 0 and at most 100, such as 25%%, not %q", t.Text)
				} else {
					t.Canonical = t.Number / 100
				}
			}
			continue
		case pr.Op == OpWithin || pr.Op == OpNone:
			v.exprErr(p, pr.Subject.Offset, CodeExpectMetric, "%q is for the whole run's metrics (%s), not %s", pr.Op, metricNames(RunMetrics), pr.Subject)
			continue
		}
		if pr.Subject.Metric == "load_rps" && pr.Subject.Service == "" {
			v.exprErr(p, pr.Subject.Offset, CodeExpectMetric, "load_rps is the offered load you set in workload; predict rps (achieved throughput) instead")
		} else if kind, ok := v.metric(p, pr.Subject); ok {
			v.quantity(p, kind, pr.Subject, &pr.Value)
		}
		switch pr.Qualifier.Kind {
		case Until:
			c := pr.Qualifier.Cond
			if kind, ok := v.metric(p, c.Metric); ok {
				v.quantity(p, kind, c.Metric, &c.Value)
			}
		case After:
			if total > 0 && pr.Qualifier.After.D() >= total {
				v.errorf(p, CodeTiming, "after %s is past the end of the workload at %s", pr.Qualifier.After, Duration(total))
			}
		}
	}
}

func (v *validator) exprErr(path string, offset int, code, format string, args ...any) {
	v.report(SevError, path, func(n *yaml.Node) int { return n.Column + offset + quoteWidth(n) }, code, format, args...)
}

func (v *validator) metric(path string, m MetricRef) (MetricKind, bool) {
	if m.Service == "" {
		if k, ok := EndToEndMetrics[m.Metric]; ok {
			return k, true
		}
		msg := fmt.Sprintf("unknown metric %q; end-to-end metrics are %s, and per-component ones are written service.metric", m.Metric, metricNames(EndToEndMetrics))
		if s := suggest(m.Metric, mapKeys(EndToEndMetrics)); s != "" {
			msg = fmt.Sprintf("unknown metric %q (did you mean %q?)", m.Metric, s)
		}
		v.exprErr(path, m.Offset, CodeExpectMetric, "%s", msg)
		return 0, false
	}
	s, ok := v.d.Services[m.Service]
	if !ok || s == nil {
		msg := fmt.Sprintf("unknown service %q", m.Service)
		if sg := suggest(m.Service, v.d.ServiceNames()); sg != "" {
			msg += fmt.Sprintf(" (did you mean %q?)", sg)
		}
		v.exprErr(path, m.Offset, CodeUnknownService, "%s", msg)
		return 0, false
	}
	metrics := ComponentMetrics[s.metricClass()]
	if k, ok := metrics[m.Metric]; ok {
		return k, true
	}
	if len(metrics) == 0 {
		v.exprErr(path, m.Offset, CodeExpectMetric, "%s (%s) has no per-component metrics to predict", m.Service, s.Kind)
		return 0, false
	}
	v.exprErr(path, m.Offset, CodeExpectMetric, "%s has no metric %q; its metrics are %s", m.Service, m.Metric, metricNames(metrics))
	return 0, false
}

func (v *validator) quantity(path string, kind MetricKind, m MetricRef, q *Quantity) {
	bad := func(format string, args ...any) {
		v.exprErr(path, q.Offset, CodeExpectValue, format, args...)
	}
	switch kind {
	case MetricLatency:
		switch q.Unit {
		case "", "ms":
			q.Canonical = q.Number
		case "s":
			q.Canonical = q.Number * 1000
		default:
			bad("%s is a latency: write milliseconds, such as 300 or 300ms", m)
		}
	case MetricRatio:
		switch q.Unit {
		case "%":
			if q.Number > 100 {
				bad("%s can't be more than 100%%", m)
			}
			q.Canonical = q.Number / 100
		case "":
			if q.Number > 1 {
				bad("%s is a fraction: write %s%% or %s", m, fmtNum(q.Number), fmtNum(q.Number/100))
			}
			q.Canonical = q.Number
		default:
			bad("%s is a fraction: write a percentage such as 5%% or a fraction such as 0.05", m)
		}
	case MetricRate, MetricCount:
		if q.Unit != "" {
			bad("%s takes a plain number, without %q", m, q.Unit)
		}
		q.Canonical = q.Number
	}
}

// --- assumptions ---

func (v *validator) assumptions() {
	for i, a := range v.d.Assumptions {
		if a == nil || a.Field == "" {
			continue
		}
		if err := resolveFieldPath(v.d, a.Field); err != nil {
			v.warnf(pathKey(pathIndex("assumptions", i), "field"), CodeAssumptionPath, "%q doesn't point at a field in this design: %v", a.Field, err)
		}
	}
}

// --- helpers on Design ---

// ServiceNames returns the service names in sorted order.
func (d *Design) ServiceNames() []string { return sortedKeys(d.Services) }

// TotalDuration is the length of the workload profile.
func (d *Design) TotalDuration() time.Duration {
	if d.Workload == nil {
		return 0
	}
	var t time.Duration
	for _, s := range d.Workload.Profile {
		switch {
		case s == nil:
		case s.Ramp != nil:
			t += s.Ramp.Over.D()
		case s.Hold != nil:
			t += s.Hold.For.D()
		case s.Spike != nil:
			t += s.Spike.For.D()
		}
	}
	return t
}

func (d *Design) entry() string {
	if d.Workload == nil {
		return ""
	}
	return d.Workload.Entry
}

func (d *Design) isCalled(name string) bool {
	for _, s := range d.Services {
		if s == nil {
			continue
		}
		if (s.Kind == "pgbouncer" || s.Kind == "lb") && s.Target == name {
			return true
		}
		if s.Behavior == nil {
			continue
		}
		found := false
		walkCalls(s.Behavior.Calls, func(c *Call) { found = found || c.Target == name })
		if found {
			return true
		}
	}
	return false
}

// EntryPathInfo describes what requests from the workload entry reach.
type EntryPathInfo struct {
	Read  []string // postgres services read on the entry path, sorted
	Keyed bool     // whether any call on the path uses a key
}

// EntryPath follows calls from the workload entry through http calls, load
// balancers and pgbouncer, and reports which postgres services it reads.
func (d *Design) EntryPath() EntryPathInfo {
	var info EntryPathInfo
	read := map[string]bool{}
	seen := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		s, ok := d.Services[name]
		if !ok || s == nil || seen[name] {
			return
		}
		if s.Kind == "lb" {
			seen[name] = true
			visit(s.Target)
			return
		}
		if s.Behavior == nil {
			return
		}
		seen[name] = true
		walkCalls(s.Behavior.Calls, func(c *Call) {
			t, ok := d.Services[c.Target]
			if !ok || t == nil {
				return
			}
			switch {
			case t.IsApp(), t.Kind == "lb":
				visit(c.Target)
				return
			case t.Kind == "pgbouncer":
				if pg, ok := d.Services[t.Target]; ok && pg != nil && c.Op == "read" {
					if db := d.datasetOf(t.Target); db != "" {
						read[db] = true
					}
				}
			case t.IsPostgres() && c.Op == "read":
				if db := d.datasetOf(c.Target); db != "" {
					read[db] = true
				}
			}
			if c.Query != "scan" {
				info.Keyed = true
			}
		})
	}
	visit(d.entry())
	info.Read = sortedKeys(read)
	return info
}

// resolveFieldPath checks that a path such as
// services.api.behavior.calls[0].count names a field of the schema, and that
// the services and list items it names exist in the design.
func resolveFieldPath(d *Design, path string) error {
	segs, err := splitPath(path)
	if err != nil {
		return err
	}
	cur := rootCursor(d)
	for _, seg := range segs {
		if cur.free {
			return nil
		}
		if cur, err = cur.step(seg); err != nil {
			return err
		}
	}
	return nil
}

type pathSeg struct {
	name  string
	index int // -1 when the segment is a name
}

func splitPath(path string) ([]pathSeg, error) {
	var segs []pathSeg
	for part := range strings.SplitSeq(path, ".") {
		name := part
		var idx []int
		for strings.HasSuffix(name, "]") {
			open := strings.LastIndexByte(name, '[')
			if open < 0 {
				return nil, fmt.Errorf("unbalanced brackets in %q", part)
			}
			n, err := strconv.Atoi(name[open+1 : len(name)-1])
			if err != nil {
				return nil, fmt.Errorf("bad index in %q", part)
			}
			idx = append([]int{n}, idx...)
			name = name[:open]
		}
		if name == "" {
			return nil, fmt.Errorf("empty segment in %q", path)
		}
		segs = append(segs, pathSeg{name: name, index: -1})
		for _, n := range idx {
			segs = append(segs, pathSeg{index: n})
		}
	}
	return segs, nil
}

// describeKind names what a service is, for messages.
func describeKind(s *Service) string {
	if s.Kind != "" {
		return "a " + s.Kind
	}
	return "an application service"
}

// pgbouncerOwnKeys are pgbouncer.ini keys Loadlore sets itself, so the
// stack can reach PgBouncer and read its metrics, or that have their own
// field, which the report and the client follow.
var pgbouncerOwnKeys = map[string]string{
	"pool_mode":                 "use the pool_mode field",
	"default_pool_size":         "use the default_pool_size field",
	"listen_addr":               "Loadlore sets it so the other containers can reach PgBouncer",
	"listen_port":               "Loadlore sets it so the other containers can reach PgBouncer",
	"unix_socket_dir":           "Loadlore sets it; PgBouncer is reached over TCP",
	"auth_type":                 "Loadlore sets it to match the run's database user",
	"auth_file":                 "Loadlore sets it to match the run's database user",
	"admin_users":               "Loadlore's exporter reads PgBouncer's statistics as this user",
	"stats_users":               "Loadlore's exporter reads PgBouncer's statistics",
	"ignore_startup_parameters": "Loadlore sets it so the exporter and clients can connect",
}

// datasetOf is the postgres whose dataset a postgres service serves: itself,
// or a replica's primary. Empty when name is neither.
func (d *Design) datasetOf(name string) string {
	s := d.Services[name]
	switch {
	case s == nil:
		return ""
	case s.Kind == "postgres":
		return name
	case s.Kind == "postgres-replica":
		if pr := d.Services[s.Primary]; pr != nil && pr.Kind == "postgres" {
			return s.Primary
		}
	}
	return ""
}

// replicaOwnKeys are postgresql.conf settings Loadlore sets on a replica to
// stream from its primary.
var replicaOwnKeys = map[string]string{
	"primary_conninfo":  "Loadlore points the replica at its primary",
	"primary_slot_name": "Loadlore manages replication itself",
	"hot_standby":       "a replica serves reads, so hot_standby stays on",
}

// replica checks a postgres-replica: a primary that is a postgres, the
// primary's version, and no settings that would stop it streaming.
func (v *validator) replica(name, p string, s *Service) {
	switch pr, ok := v.d.Services[s.Primary]; {
	case s.Primary == "":
		v.errorf(pathKey(p, "primary"), CodeRequired, "postgres-replica needs a primary: the postgres service it streams from")
	case !ok:
		v.unknownService(pathKey(p, "primary"), s.Primary)
	case v.broken(s.Primary):
	case pr == nil || pr.Kind != "postgres":
		v.errorf(pathKey(p, "primary"), CodeOpTarget, "a replica's primary must be a postgres service; %q is %s", s.Primary, describeService(pr))
	case pr.Version != "" && s.Version != "" && pr.Version != s.Version:
		v.errorf(pathKey(p, "version"), CodeConflict, "a replica runs its primary's version: %s is %s, %s is %s", s.Primary, pr.Version, name, s.Version)
	}
	for _, k := range sortedKeys(s.Config) {
		if why, ok := replicaOwnKeys[k]; ok {
			v.errorf(pathKey(pathKey(p, "config"), k), CodeNotApplicable, "%s can't be set in config: %s", k, why)
		}
	}
}
