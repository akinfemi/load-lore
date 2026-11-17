package plan

import (
	"fmt"
	"time"
)

// A quick run skips the part of the climb an earlier run already
// measured: a short warm-up to QuickShare of where that run broke, then
// the profile from there on, at its own slope, so the climb past the
// break is as fast as in a full run.
const (
	QuickShare  = 0.75
	QuickWarmup = 30 * time.Second
	// QuickMinSaving: a quick run that saves less than this isn't worth
	// what it gives up, so it runs the whole profile.
	QuickMinSaving = 30 * time.Second
	// QuickLowShare: a break in the warm-up, or this far into the rest of
	// the climb, may be the warm-up's, so the verdict says so.
	QuickLowShare = 0.1
)

// Quick says how a run's profile was shortened.
type Quick struct {
	FromRun  string  `json:"from_run"`
	BreakRPS float64 `json:"break_rps"` // where FromRun broke, at this run's scale
	StartRPS float64 `json:"start_rps"` // where the warm-up ends and the profile resumes
	WarmupS  float64 `json:"warmup_s"`
	SkippedS float64 `json:"skipped_s"` // load time skipped: fault times moved earlier by this
	// LowRPS: a break below this offered rate may be the warm-up's.
	LowRPS float64 `json:"low_rps"`
}

// QuickError is why a run can't be quick.
type QuickError struct{ Reason string }

func (e *QuickError) Error() string { return "--quick: " + e.Reason }

// Quicken shortens the plan's profile to start near breakRPS, where run
// from broke. It refuses designs whose result depends on the climb it
// skips: autoscaled services, and faults timed within it. It returns false,
// with the plan unchanged, when there is too little to skip.
func (p *Plan) Quicken(from string, breakRPS float64) (bool, error) {
	for _, s := range p.Services {
		if s.Autoscale != nil {
			return false, &QuickError{s.Name + " autoscales, and how the autoscaler keeps up with the climb is part of what the run measures: run the whole profile"}
		}
	}
	w := &p.Workload
	target := QuickShare * breakRPS
	if target <= w.StartRate {
		return false, nil
	}
	// The first rising stage that reaches the target.
	rate := w.StartRate
	var elapsed time.Duration
	idx := -1
	var frac float64
	for i, st := range w.Stages {
		d := st.Duration.D()
		if d > 0 && st.Target > rate && target >= rate && target < st.Target {
			idx, frac = i, (target-rate)/(st.Target-rate)
			break
		}
		elapsed += d
		rate = st.Target
	}
	if idx < 0 {
		return false, &QuickError{fmt.Sprintf("the profile never climbs through %s rps, %.0f%% of the %s rps where %s broke", fmtF(target), QuickShare*100, fmtF(breakRPS), from)}
	}
	st := w.Stages[idx]
	resumeAt := elapsed + time.Duration(frac*float64(st.Duration.D()))
	skipped := resumeAt - QuickWarmup
	if skipped < QuickMinSaving {
		return false, nil
	}
	for _, f := range p.Faults {
		if f.At.D() < resumeAt {
			return false, &QuickError{fmt.Sprintf("a %s fault at %s falls in the part of the climb a quick run skips (up to %s): run the whole profile", f.Inject, f.At.D(), resumeAt)}
		}
	}

	stages := []Stage{{Target: target, Duration: Dur(QuickWarmup)}}
	stages = append(stages, Stage{Target: st.Target, Duration: Dur(st.Duration.D() - (resumeAt - elapsed))})
	stages = append(stages, w.Stages[idx+1:]...)
	w.Stages = stages
	for _, f := range p.Faults {
		f.At = Dur(f.At.D() - skipped)
	}
	p.Duration = w.Duration()
	p.Quick = &Quick{FromRun: from, BreakRPS: breakRPS, StartRPS: target, WarmupS: QuickWarmup.Seconds(), SkippedS: skipped.Seconds(),
		LowRPS: target + QuickLowShare*(st.Target-target)}
	p.note("quick: the load climbs from %s to %s rps in %s, %.0f%% of the %s rps where %s broke, then follows the profile from there, at its own slope; %s of the climb is skipped, and fault times move %s earlier with it",
		fmtF(w.StartRate), fmtF(target), QuickWarmup, QuickShare*100, fmtF(breakRPS), from, skipped.Round(time.Second), skipped.Round(time.Second))
	return true, nil
}
