package analysis

import (
	"fmt"

	"github.com/akinfemi/load-lore/internal/design"
)

// ExpectStatus is how a prediction came out.
type ExpectStatus string

// Expect statuses.
const (
	ExpectHeld     ExpectStatus = "held"
	ExpectBroken   ExpectStatus = "broken"
	ExpectUnscored ExpectStatus = "unscored"
	ExpectNote     ExpectStatus = "note"
)

// ExpectResult is one scored prediction.
type ExpectResult struct {
	Text      string       `json:"text,omitempty"`
	Note      string       `json:"note,omitempty"`
	Status    ExpectStatus `json:"status"`
	Metric    string       `json:"metric,omitempty"`
	Source    string       `json:"source,omitempty"`
	Range     string       `json:"range,omitempty"`
	Windows   int          `json:"windows"`
	Missing   int          `json:"windows_without_data"`
	Actual    *float64     `json:"actual,omitempty"` // the worst value in range, in canonical units
	Shown     string       `json:"actual_text,omitempty"`
	BrokeAt   *float64     `json:"broke_at_s,omitempty"`
	Threshold float64      `json:"threshold"`
	Message   string       `json:"message,omitempty"`
}

func scoreAll(a *Analysis, ws []*Window) []ExpectResult {
	out := []ExpectResult{}
	for _, e := range a.Design.Expect {
		if e == nil {
			continue
		}
		if e.Prediction == nil {
			out = append(out, ExpectResult{Note: e.Note, Text: e.Text, Status: ExpectNote})
			continue
		}
		if _, ok := design.RunMetrics[e.Prediction.Subject.Metric]; ok && e.Prediction.Subject.Service == "" {
			out = append(out, scoreRun(a, e, ws))
			continue
		}
		out = append(out, score(a, e, ws))
	}
	return out
}

type metricFn struct {
	get    func(w *Window) *float64
	kind   design.MetricKind
	source string
}

func metricFor(a *Analysis, m design.MetricRef) (metricFn, bool) {
	if m.Service == "" {
		switch m.Metric {
		case "p50_ms":
			return metricFn{func(w *Window) *float64 { return w.P50 }, design.MetricLatency, SrcK6Latency}, true
		case "p95_ms":
			return metricFn{func(w *Window) *float64 { return w.P95 }, design.MetricLatency, SrcK6Latency}, true
		case "p99_ms":
			return metricFn{func(w *Window) *float64 { return w.P99 }, design.MetricLatency, SrcK6Latency}, true
		case "error_rate":
			return metricFn{func(w *Window) *float64 { return w.ErrorRate }, design.MetricRatio, SrcErrors}, true
		case "rps":
			return metricFn{func(w *Window) *float64 { return w.RPS }, design.MetricRate, SrcRPS}, true
		case "load_rps":
			return metricFn{func(w *Window) *float64 { v := w.Offered; return &v }, design.MetricRate, SrcOffered}, true
		case "stale_read_rate":
			return metricFn{func(w *Window) *float64 { return w.StaleReadRate }, design.MetricRatio, SrcStaleReads}, true
		}
		return metricFn{}, false
	}
	svc := func(f func(*ServiceWindow) *float64) func(*Window) *float64 {
		return func(w *Window) *float64 {
			if sw := w.Services[m.Service]; sw != nil {
				return f(sw)
			}
			return nil
		}
	}
	switch m.Metric {
	case "cpu":
		return metricFn{svc(func(s *ServiceWindow) *float64 { return s.CPU }), design.MetricRatio, SrcCPU}, true
	case "memory":
		return metricFn{svc(func(s *ServiceWindow) *float64 { return s.Memory }), design.MetricRatio, SrcMemory}, true
	case "p99_ms":
		return metricFn{svc(func(s *ServiceWindow) *float64 { return s.P99 }), design.MetricLatency, SrcAppLatency}, true
	case "error_rate":
		return metricFn{svc(func(s *ServiceWindow) *float64 { return s.ErrorRate }), design.MetricRatio, SrcAppErrors}, true
	case "inflight":
		return metricFn{svc(func(s *ServiceWindow) *float64 { return s.Inflight }), design.MetricCount, SrcInflight}, true
	case "connections":
		return metricFn{svc(func(s *ServiceWindow) *float64 { return s.Connections }), design.MetricCount, connSource(a, m.Service)}, true
	case "hit_rate":
		return metricFn{svc(func(s *ServiceWindow) *float64 { return s.HitRate }), design.MetricRatio, SrcHitRate}, true
	case "stale_read_rate":
		return metricFn{svc(func(s *ServiceWindow) *float64 { return s.StaleReadRate }), design.MetricRatio, SrcStaleReads}, true
	case "depth":
		return metricFn{svc(func(s *ServiceWindow) *float64 { return s.Depth }), design.MetricCount, SrcQueueDepth}, true
	case "retrying":
		return metricFn{svc(func(s *ServiceWindow) *float64 { return s.Retrying }), design.MetricCount, SrcJobRetries}, true
	case "dead_jobs":
		return metricFn{svc(func(s *ServiceWindow) *float64 { return s.DeadTotal }), design.MetricCount, SrcJobRetries}, true
	case "oldest_job_ms":
		return metricFn{svc(func(s *ServiceWindow) *float64 {
			if s.OldestJobS == nil {
				return nil
			}
			v := *s.OldestJobS * 1000
			return &v
		}), design.MetricLatency, SrcOldestJob}, true
	case "job_wait_p99_ms":
		return metricFn{svc(func(s *ServiceWindow) *float64 { return s.JobWaitP99 }), design.MetricLatency, SrcJobWait}, true
	case "replication_lag_ms":
		return metricFn{svc(func(s *ServiceWindow) *float64 {
			if s.ReplicationLagS == nil {
				return nil
			}
			v := *s.ReplicationLagS * 1000
			return &v
		}), design.MetricLatency, SrcReplLagS}, true
	}
	return metricFn{}, false
}

// Format renders a value of a metric kind.
func Format(kind design.MetricKind, v float64) string {
	switch kind {
	case design.MetricLatency:
		return Ms(v)
	case design.MetricRatio:
		return Pct(v)
	case design.MetricRate:
		return Rate(v) + " rps"
	}
	return Count(v)
}

// threshold is a prediction's value in canonical units. Rates scale with
// --scale, since the load does; latencies, ratios and counts don't.
func threshold(a *Analysis, kind design.MetricKind, q design.Quantity) (float64, string) {
	v := q.Canonical
	if kind == design.MetricRate && a.Scale != 1 && a.Scale > 0 {
		scaled := v * a.Scale
		return scaled, fmt.Sprintf("%s rps scaled to %s rps by --scale %g", Rate(v), Rate(scaled), a.Scale)
	}
	return v, ""
}

func score(a *Analysis, e *design.Expectation, ws []*Window) ExpectResult {
	pr := e.Prediction
	r := ExpectResult{Text: e.Text, Metric: pr.Subject.String(), Status: ExpectUnscored}
	m, ok := metricFor(a, pr.Subject)
	if !ok {
		r.Message = "this version of loadl can't measure " + r.Metric
		return r
	}
	r.Source = m.source
	var notes []string
	thr, note := threshold(a, m.kind, pr.Value)
	r.Threshold = thr
	if note != "" {
		notes = append(notes, note)
	}
	if len(ws) == 0 {
		r.Message = "no data: the load never ran"
		return r
	}

	// The windows the prediction covers.
	from, to := 0, len(ws)
	switch pr.Qualifier.Kind {
	case design.After:
		after := pr.Qualifier.After.D().Seconds()
		for from < len(ws) && ws[from].Start < after {
			from++
		}
		r.Range = "from " + Clock(after) + " to the end"
	case design.Until:
		c := pr.Qualifier.Cond
		cm, ok := metricFor(a, c.Metric)
		if !ok {
			r.Message = "this version of loadl can't measure " + c.Metric.String()
			return r
		}
		ct, cnote := threshold(a, cm.kind, c.Value)
		if cnote != "" {
			notes = append(notes, "condition "+cnote)
		}
		to = len(ws)
		for i, w := range ws {
			if v := cm.get(w); v != nil && c.Op.Holds(*v, ct) {
				to = i
				break
			}
		}
		cond := fmt.Sprintf("%s %s %s", c.Metric, c.Op, Format(cm.kind, ct))
		if to == len(ws) {
			r.Range = "the whole run (" + cond + " never held)"
		} else {
			r.Range = fmt.Sprintf("0:00 to %s, when %s first held", Clock(ws[to].Start), cond)
		}
	default:
		r.Range = "the whole run"
	}
	if from >= to {
		r.Message = "no windows in range"
		return r
	}

	worse := func(x, y float64) bool { // is x worse than y for this prediction
		if pr.Op == design.OpLT || pr.Op == design.OpLE {
			return x > y
		}
		return x < y
	}
	for i := from; i < to; i++ {
		r.Windows++
		v := m.get(ws[i])
		if v == nil {
			r.Missing++
			continue
		}
		if r.Actual == nil || worse(*v, *r.Actual) {
			r.Actual = v
		}
		if !pr.Op.Holds(*v, thr) && r.BrokeAt == nil {
			at := ws[i].Start
			r.BrokeAt = &at
		}
	}
	switch {
	case r.Actual == nil:
		r.Message = "no data for " + r.Metric + " (" + m.source + ")"
		return r
	case r.BrokeAt != nil:
		r.Status = ExpectBroken
	default:
		r.Status = ExpectHeld
	}
	if es := a.StoppedEarly; es != nil && r.Status == ExpectHeld && to == len(ws) {
		// Held, but the range went on past the stop: say what it covers.
		notes = append(notes, fmt.Sprintf("held until the load stopped at %s; the rest of the profile didn't run (`loadl run --full` runs it)", Clock(es.At)))
	}
	r.Shown = Format(m.kind, *r.Actual)
	if r.Missing > 0 {
		notes = append(notes, fmt.Sprintf("%d of %d windows had no data", r.Missing, r.Windows))
	}
	for i, n := range notes {
		if i > 0 {
			r.Message += "; "
		}
		r.Message += n
	}
	return r
}

// ScoreLine scores one expect line, which needn't be in the run's design,
// against the run: scenario check uses it for claims about a run.
func ScoreLine(a *Analysis, line string) (ExpectResult, error) {
	if a.Design == nil {
		return ExpectResult{}, fmt.Errorf("the run has no design to check %q against", line)
	}
	e, err := a.Design.Expectation(line)
	if err != nil {
		return ExpectResult{}, err
	}
	p := e.Prediction
	ws := a.Windows[:min(a.Analyzed, len(a.Windows))]
	if _, ok := design.RunMetrics[p.Subject.Metric]; ok && p.Subject.Service == "" {
		return scoreRun(a, e, ws), nil
	}
	return score(a, e, ws), nil
}
