package analysis

import (
	"cmp"
	"slices"
)

// ErrorCause is one cause of a service's failed requests over a span of
// windows, from the synthetic service's own counters.
type ErrorCause struct {
	Service string  `json:"service"`
	Cause   string  `json:"cause"`   // synthetic.Cause*, such as too_many_conns
	Count   float64 `json:"count"`   // failed requests
	Share   float64 `json:"share"`   // of the service's failed requests
	Target  string  `json:"target"`  // the call target with the most failed calls of this cause, if any
	First   float64 `json:"first_s"` // start of the first window it appears in
	Meaning string  `json:"meaning"` // what it means, in plain words
}

// SrcErrorCauses names the counters ErrorCauses come from.
const SrcErrorCauses = "loadlore_request_errors_total and loadlore_call_errors_total, by cause"

// ErrorCauses sums the causes of failed requests over ws, most failures
// first. Retries count as one failed request when the last attempt fails.
func ErrorCauses(ws []*Window) []ErrorCause {
	type key struct{ service, cause string }
	counts := map[key]float64{}
	first := map[key]float64{}
	calls := map[key]map[string]float64{}
	totals := map[string]float64{}
	for _, w := range ws {
		for name, sw := range w.Services {
			for c, n := range sw.ErrorCauses {
				k := key{name, c}
				if _, ok := counts[k]; !ok {
					first[k] = w.Start
				}
				counts[k] += n
				totals[name] += n
			}
		}
		for _, e := range w.Edges {
			for c, n := range e.ErrorCauses {
				k := key{e.From, c}
				if calls[k] == nil {
					calls[k] = map[string]float64{}
				}
				calls[k][e.To] += n
			}
		}
	}
	out := make([]ErrorCause, 0, len(counts))
	for k, n := range counts {
		ec := ErrorCause{Service: k.service, Cause: k.cause, Count: n, Share: n / totals[k.service], First: first[k]}
		best := 0.0
		for t, m := range calls[k] {
			if m > best || m == best && t < ec.Target {
				ec.Target, best = t, m
			}
		}
		ec.Meaning = causeMeaning(ec.Cause, ec.Target)
		out = append(out, ec)
	}
	slices.SortFunc(out, func(a, b ErrorCause) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Service, b.Service), cmp.Compare(a.Cause, b.Cause))
	})
	return out
}

// causeSentence names the main cause of failed requests in ws, for the
// verdict: " Of api's failed requests, 100% failed because db refused new
// connections (…)." It's empty when no service counted its failures.
func causeSentence(ws []*Window) string {
	ecs := ErrorCauses(ws)
	if len(ecs) == 0 {
		return ""
	}
	top := ecs[0] // the most failures of any single cause
	return " Of " + top.Service + "'s failed requests, " + Pct(top.Share) + " failed because " + top.Meaning + "."
}

// causeMeaning says what a cause means, naming the target when there is
// one. It reads after "because".
func causeMeaning(cause, target string) string {
	to, of := "", ""
	if target != "" {
		to, of = " to "+target, " in front of "+target
	}
	who := target
	if who == "" {
		who = "a dependency"
	}
	switch cause {
	case "too_many_conns":
		return who + " refused new connections (Postgres max_connections)"
	case "pool_timeout":
		return "they timed out waiting for a pool connection" + to
	case "timeout":
		return "calls" + to + " ran past their timeout"
	case "canceled":
		return "the caller went away first"
	case "conn_refused":
		return who + " refused connections: down, or nothing listening"
	case "conn_reset":
		return "connections" + to + " were reset or closed mid-call"
	case "db_error", "cache_error", "queue_error":
		return who + " returned errors"
	case "queue_full":
		return who + " was full and refused new jobs"
	case "upstream_error":
		return who + " answered with error statuses"
	case "circuit_open":
		return "the circuit breaker" + to + " was open"
	case "bulkhead_full":
		return "the bulkhead" + of + " was full"
	}
	return "of other errors"
}
