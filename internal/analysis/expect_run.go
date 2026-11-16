package analysis

import (
	"fmt"
	"math"

	"github.com/akinfemi/load-lore/internal/design"
)

// Sources of the whole run's metrics.
const (
	SrcBreakLoad  = "the verdict's break: offered load in its first window"
	SrcErrorsLoad = "offered load in the first of 2 windows in a row with 1% or more of requests failing (k6)"
)

// runMetric is a whole-run metric's measured value: the offered load at
// the moment it happened, and when; ok is false if it never did.
func runMetric(a *Analysis, ws []*Window, name string) (rps, at float64, ok bool, source string) {
	switch name {
	case "break_load_rps":
		if b := a.Verdict.Break; b != nil {
			return b.Offered, b.At, true, SrcBreakLoad
		}
		return 0, 0, false, SrcBreakLoad
	case "errors_load_rps":
		failing := func(w *Window) bool { return w.ErrorRate != nil && *w.ErrorRate >= BreakErrorRate }
		for i := 0; i+BreakWindows <= len(ws); i++ {
			run := true
			for _, w := range ws[i : i+BreakWindows] {
				run = run && failing(w)
			}
			if run {
				return ws[i].Offered, ws[i].Start, true, SrcErrorsLoad
			}
		}
		return 0, 0, false, SrcErrorsLoad
	}
	return 0, 0, false, ""
}

// scoreRun scores a prediction of a whole-run metric: "within N% of X",
// which a guess too high or too low breaks, "none" or "any".
func scoreRun(a *Analysis, e *design.Expectation, ws []*Window) ExpectResult {
	pr := e.Prediction
	r := ExpectResult{Text: e.Text, Metric: pr.Subject.String(), Status: ExpectUnscored, Range: "the whole run"}
	rps, at, happened, src := runMetric(a, ws, pr.Subject.Metric)
	r.Source = src
	if len(ws) == 0 {
		r.Message = "no data: the load never ran"
		return r
	}
	what := map[string]string{"break_load_rps": "it broke", "errors_load_rps": "requests started failing"}[pr.Subject.Metric]
	never := map[string]string{"break_load_rps": "it never broke", "errors_load_rps": "requests never failed"}[pr.Subject.Metric]

	// How much of the profile ran: the rest can't say a thing never
	// happened, unless the run stopped early on purpose (a break).
	maxOffered := 0.0
	for _, w := range ws {
		maxOffered = math.Max(maxOffered, w.Offered)
	}
	end := float64(len(ws) * WindowSeconds)
	whole := a.Plan == nil || end >= a.Plan.Duration.Seconds()-WindowSeconds
	var notes []string
	if a.Verdict.Kind == VerdictInconclusive && happened {
		notes = append(notes, "the run is inconclusive, so this may be the tooling's limit, not the system's")
	}
	if !happened && !whole && a.StoppedEarly != nil {
		notes = append(notes, fmt.Sprintf("held until the load stopped at %s; the rest of the profile didn't run (`loadl run --full` runs it)", Clock(a.StoppedEarly.At)))
	}

	if happened {
		v := rps
		r.Actual, r.Shown = &v, Rate(rps)+" rps at "+Clock(at)
	} else {
		r.Shown = map[string]string{"break_load_rps": "no break", "errors_load_rps": "no failures"}[pr.Subject.Metric]
	}
	switch pr.Op {
	case design.OpNone:
		switch {
		case happened:
			r.Status = ExpectBroken
			notes = append([]string{fmt.Sprintf("%s at %s rps offered, at %s", what, Rate(rps), Clock(at))}, notes...)
		case !whole && a.StoppedEarly == nil:
			r.Message = fmt.Sprintf("the load ended at %s, before the profile did, so it can't say %s", Clock(end), never)
			return r
		default:
			r.Status = ExpectHeld
		}
	case design.OpAny:
		switch {
		case happened:
			r.Status = ExpectHeld
			notes = append([]string{fmt.Sprintf("%s at %s rps offered, at %s", what, Rate(rps), Clock(at))}, notes...)
		case !whole && a.StoppedEarly == nil:
			r.Message = fmt.Sprintf("the load ended at %s, before the profile did, so it can't say %s", Clock(end), never)
			return r
		default:
			r.Status = ExpectBroken
			notes = append([]string{never}, notes...)
		}
	case design.OpWithin:
		target, scaleNote := threshold(a, design.MetricRate, pr.Value)
		r.Threshold = target
		if scaleNote != "" {
			notes = append(notes, scaleNote)
		}
		tol := pr.Tolerance.Canonical
		switch {
		case happened:
			diff := (rps - target) / target
			side := "above"
			if diff < 0 {
				side = "below"
			}
			gap := fmt.Sprintf("%s rps, %s %s the guess of %s", Rate(rps), Pct(math.Abs(diff)), side, Rate(target))
			switch {
			case math.Abs(diff) <= tol+1e-9:
				r.Status = ExpectHeld
				notes = append([]string{what + " at " + gap}, notes...)
			case diff < 0:
				r.Status = ExpectBroken
				notes = append([]string{"too high: " + what + " at " + gap}, notes...)
			default:
				r.Status = ExpectBroken
				notes = append([]string{"too low: " + what + " at " + gap}, notes...)
			}
		case maxOffered < target*(1-tol) && !whole:
			r.Message = fmt.Sprintf("the load ended at %s rps, below the guess's range, so it can't say", Rate(maxOffered))
			return r
		default:
			r.Status = ExpectBroken
			notes = append([]string{fmt.Sprintf("too low: %s up to %s rps offered", never, Rate(maxOffered))}, notes...)
		}
	}
	for i, n := range notes {
		if i > 0 {
			r.Message += "; "
		}
		r.Message += n
	}
	return r
}
