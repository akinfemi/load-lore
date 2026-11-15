package scenario

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"github.com/akinfemi/load-lore/internal/plan"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/bench"
)

// RunFunc runs one design and returns its run folder. A run that started
// but failed returns its folder along with the error. coolTo is a host
// speed index to wait for first, as runner.Options.CoolTo; 0 starts at once.
type RunFunc func(ctx context.Context, designPath string, coolTo int) (runDir string, err error)

// Check statuses. A check is inconclusive when no claim failed but a
// solution's run and the starting design's were still more than
// bench.Tolerance apart in host speed index after a re-run: the host's
// speed, not the design, could explain the difference. Or when a claim
// rests on a run not recovering and Toxiproxy stalled after its break.
const (
	StatusOK           = "ok"
	StatusFailed       = "failed"
	StatusInconclusive = "inconclusive"
)

// CheckResult is the outcome of `loadl scenario check`.
type CheckResult struct {
	Scenario  string      `json:"scenario"`
	Path      string      `json:"path"`
	OK        bool        `json:"ok"`     // Status is ok
	Status    string      `json:"status"` // ok, failed or inconclusive
	Lint      []Problem   `json:"lint"`
	Problem   *RunCheck   `json:"problem,omitempty"`
	Solutions []*RunCheck `json:"solutions"`
	Variants  []*RunCheck `json:"variants"`
	// Load is the load generator sizing these runs measured, against
	// meta.yaml's load block.
	Load *LoadCheck `json:"load,omitempty"`
}

// LoadCheck is the prealloc_vus the check's runs call for: the larger of
// the peak rate times slo.p99_ms and 1.5 times the most VUs busy at once
// before the break in any run, at --scale 1.
type LoadCheck struct {
	PeakVUs     int       `json:"peak_vus"`
	From        string    `json:"from"` // the run with that peak
	PreallocVUs int       `json:"prealloc_vus"`
	Meta        *LoadMeta `json:"meta,omitempty"` // what meta.yaml says now
	// WholeRun: measured over whole runs, not only before the break,
	// because a claim judges recovery after it.
	WholeRun bool `json:"whole_run,omitempty"`
}

func judgesRecovery(c Claims) bool {
	all := []Claim{c.Problem}
	for _, x := range c.Solutions {
		all = append(all, x)
	}
	for _, x := range c.Variants {
		all = append(all, x)
	}
	for _, x := range all {
		if x.Recovers != nil {
			return true
		}
	}
	return false
}

// LoadMargin is how far above the most VUs measured busy at once a
// scenario's prealloc_vus goes.
const LoadMargin = 1.5

// RunCheck is one design's run and its claims.
type RunCheck struct {
	Name         string   `json:"name"` // "problem", or the solution's or variant's name
	Design       string   `json:"design"`
	RunDir       string   `json:"run_dir,omitempty"`
	Error        string   `json:"error,omitempty"`
	Verdict      string   `json:"verdict,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	BreakAt      *float64 `json:"break_at_s,omitempty"`
	BreakLoad    *float64 `json:"break_offered_rps,omitempty"`
	BreakSignals []string `json:"break_signals,omitempty"` // latency, errors, throughput at the break
	PeakRPS      *float64 `json:"peak_rps,omitempty"`      // most successful responses/s in any window
	// PeakVUs is the most VUs busy at once before the break, at --scale 1;
	// SLOVUs the peak rate times slo.p99_ms there.
	PeakVUs *float64 `json:"peak_vus,omitempty"`
	SLOVUs  float64  `json:"slo_vus,omitempty"`
	// RecoveredAt is when it was healthy again after its break, for good.
	RecoveredAt *float64 `json:"recovered_at_s,omitempty"`
	// RecoveryDoubt is why not recovering may be partly the lab's doing:
	// Toxiproxy stalled after the break (analysis.Verdict.RecoveryDoubt).
	RecoveryDoubt string `json:"recovery_doubt,omitempty"`
	// PeakVUsAll is the most VUs busy at once in the whole run, at --scale 1.
	PeakVUsAll   *float64      `json:"peak_vus_all,omitempty"`
	Diagnoses    []string      `json:"diagnoses"`
	Claims       []ClaimResult `json:"claims"`
	OK           bool          `json:"ok"`                     // Status is ok
	Status       string        `json:"status"`                 // ok, failed or inconclusive
	Inconclusive string        `json:"inconclusive,omitempty"` // why, when Status is inconclusive

	SpeedIndex int     `json:"speed_index,omitempty"` // the host speed index this run measured
	Replaced   []Rerun `json:"replaced"`              // earlier runs of this design that a re-run replaced

	rerun bool // this solution's one re-run, its own or the starting design's, is spent
}

// Rerun is a run replaced by a re-run of the same design: its host speed
// index was more than bench.Tolerance from the run it was judged with, and
// it was the slower of the two.
type Rerun struct {
	RunDir       string `json:"run_dir"`             // the replaced run
	SpeedIndex   int    `json:"speed_index"`         // its host speed index
	Against      string `json:"against"`             // the run it was judged with: "problem" or a solution's name
	AgainstSpeed int    `json:"against_speed_index"` // that run's speed index, which the re-run waited for
	NewSpeed     int    `json:"new_speed_index"`     // the re-run's speed index, 0 when it measured none
}

// ClaimResult is one claim and whether the run showed it.
type ClaimResult struct {
	Claim  string `json:"claim"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Check lints the folder, then runs the starting design, each solution and
// each variant in turn and checks their claims. With run nil it only lints.
func Check(ctx context.Context, s *Scenario, dir string, run RunFunc) *CheckResult {
	res := &CheckResult{Scenario: s.Name, Path: dir, Lint: s.Lint(), Solutions: []*RunCheck{}, Variants: []*RunCheck{}}
	if res.Lint == nil {
		res.Lint = []Problem{}
	}
	res.OK = !HasErrors(res.Lint)
	res.Status = StatusFailed
	if res.OK {
		res.Status = StatusOK
	}
	if run == nil || !res.OK {
		return res
	}
	res.Problem = runOne(ctx, run, "problem", filepath.Join(dir, DesignFile), 0)
	for _, name := range s.Solutions {
		if ctx.Err() != nil {
			break
		}
		// Each solution waits for the host to cool back to the starting
		// design's speed, so it isn't judged on a hotter machine.
		rc := runOne(ctx, run, name, filepath.Join(dir, filepath.FromSlash(SolutionDesign(name))), res.Problem.SpeedIndex)
		res.Solutions = append(res.Solutions, rc)
		rerunSlower(ctx, run, res)
	}
	for _, name := range s.Variants {
		if ctx.Err() != nil {
			break
		}
		// A variant's claims stand on their own, not against another
		// run, so it waits for the starting design's speed but never runs
		// again.
		res.Variants = append(res.Variants, runOne(ctx, run, name, filepath.Join(dir, filepath.FromSlash(VariantDesign(name))), res.Problem.SpeedIndex))
	}

	// The claims are judged once the runs are in, so every solution is
	// judged against the starting design's last run.
	ownClaims(res.Problem, s.Meta.Check.Problem)
	failed := !res.Problem.OK || len(res.Solutions) != len(s.Solutions) || len(res.Variants) != len(s.Variants)
	inconclusive := false
	for _, rc := range res.Solutions {
		solutionClaims(rc, s.Meta.Check.Solutions[rc.Name], res.Problem, s.Meta.Check.Problem.Diagnoses)
		failed = failed || rc.Status == StatusFailed
		inconclusive = inconclusive || rc.Status == StatusInconclusive
	}
	for _, rc := range res.Variants {
		ownClaims(rc, s.Meta.Check.Variants[rc.Name])
		failed = failed || rc.Status == StatusFailed
	}
	switch {
	case failed:
		res.Status = StatusFailed
	case inconclusive:
		res.Status = StatusInconclusive
	}
	res.OK = res.Status == StatusOK
	res.Load = loadCheck(s, res)
	return res
}

func loadCheck(s *Scenario, res *CheckResult) *LoadCheck {
	lc := &LoadCheck{Meta: s.Meta.Load, WholeRun: judgesRecovery(s.Meta.Check)}
	found := false
	for _, rc := range append(append([]*RunCheck{res.Problem}, res.Solutions...), res.Variants...) {
		if rc == nil {
			continue
		}
		lc.PreallocVUs = max(lc.PreallocVUs, int(math.Ceil(rc.SLOVUs)))
		// A recovery claim judges the windows after a break too, where
		// dropped arrivals would lighten the load it recovers from.
		peak := rc.PeakVUs
		if lc.WholeRun {
			peak = rc.PeakVUsAll
		}
		if peak != nil && (!found || int(math.Ceil(*peak)) > lc.PeakVUs) {
			lc.PeakVUs, lc.From, found = int(math.Ceil(*peak)), filepath.Base(rc.RunDir), true
		}
	}
	if !found {
		return nil
	}
	lc.PreallocVUs = min(plan.MaxVUsCap, max(lc.PreallocVUs, int(math.Ceil(LoadMargin*float64(lc.PeakVUs)))))
	return lc
}

// rerunSlower re-runs the slower of the starting design's run and a
// solution's when their host speed indexes are more than bench.Tolerance
// apart, after waiting for the host to get back to the faster one's speed.
// Each solution gets one re-run, its own or the starting design's. A new
// run of the starting design replaces the old one for every solution, so
// the solutions before it are compared with it again.
func rerunSlower(ctx context.Context, run RunFunc, res *CheckResult) {
	for ctx.Err() == nil {
		i := slices.IndexFunc(res.Solutions, func(rc *RunCheck) bool { return !rc.rerun && apart(res.Problem, rc) })
		if i < 0 {
			return
		}
		sol := res.Solutions[i]
		sol.rerun = true
		if sol.SpeedIndex < res.Problem.SpeedIndex {
			res.Solutions[i] = rerun(ctx, run, sol, res.Problem)
		} else {
			res.Problem = rerun(ctx, run, res.Problem, sol)
		}
	}
}

// rerun runs rc's design again once the host is back to other's speed and
// returns the new run, which records the one it replaces.
func rerun(ctx context.Context, run RunFunc, rc, other *RunCheck) *RunCheck {
	n := runOne(ctx, run, rc.Name, rc.Design, other.SpeedIndex)
	n.rerun = rc.rerun
	n.Replaced = slices.Concat(rc.Replaced, []Rerun{{RunDir: rc.RunDir, SpeedIndex: rc.SpeedIndex, Against: other.Name, AgainstSpeed: other.SpeedIndex, NewSpeed: n.SpeedIndex}})
	return n
}

// apart reports whether two runs' host speed indexes are more than
// bench.Tolerance apart. A run that measured none can't be compared.
func apart(a, b *RunCheck) bool {
	return a != nil && a.SpeedIndex > 0 && b.SpeedIndex > 0 && bench.Gap(a.SpeedIndex, b.SpeedIndex) > bench.Tolerance
}

// analyze reads a run folder; tests stand in for it.
var analyze = analysis.Analyze

func runOne(ctx context.Context, run RunFunc, name, designPath string, coolTo int) *RunCheck {
	rc := &RunCheck{Name: name, Design: designPath, Diagnoses: []string{}, Claims: []ClaimResult{}, Replaced: []Rerun{}}
	dir, err := run(ctx, designPath, coolTo)
	rc.RunDir = dir
	if err != nil {
		rc.Error = err.Error()
		return rc
	}
	a, err := analyze(dir)
	if err != nil {
		rc.Error = err.Error()
		return rc
	}
	rc.Verdict, rc.Summary = string(a.Verdict.Kind), a.Verdict.Summary
	if h := a.Environment.Host; h != nil && h.Speed != nil {
		rc.SpeedIndex = h.Speed.Index
	}
	rc.PeakRPS = a.Verdict.MaxRPS
	if scale := a.Scale; scale > 0 {
		if a.PeakVUs != nil {
			v := *a.PeakVUs / scale
			rc.PeakVUs = &v
		}
		if a.PeakVUsAll != nil {
			v := *a.PeakVUsAll / scale
			rc.PeakVUsAll = &v
		}
		rc.SLOVUs = a.PeakOffered / scale * a.Verdict.SLOP99Ms / 1000
	}
	rc.RecoveredAt = a.Verdict.RecoveredAt
	rc.RecoveryDoubt = a.Verdict.RecoveryDoubt
	if b := a.Verdict.Break; b != nil {
		at, load := b.At, b.Offered
		rc.BreakAt, rc.BreakLoad = &at, &load
		rc.BreakSignals = b.Signals
	}
	for _, d := range a.Diagnoses {
		if !slices.Contains(rc.Diagnoses, d.Rule) {
			rc.Diagnoses = append(rc.Diagnoses, d.Rule)
		}
	}
	return rc
}

func (rc *RunCheck) claim(text string, ok bool, detail string) {
	rc.Claims = append(rc.Claims, ClaimResult{Claim: text, OK: ok, Detail: detail})
}

// usable records a claim that the run finished with a verdict that can be
// judged, and reports whether it did.
func (rc *RunCheck) usable() bool {
	switch {
	case rc.Error != "":
		rc.claim("the run finishes", false, rc.Error)
		return false
	case rc.Verdict == string(analysis.VerdictInconclusive) || rc.Verdict == string(analysis.VerdictNoData):
		rc.claim("the run is conclusive", false, rc.Summary)
		return false
	}
	return true
}

// ownClaims judges the starting design's run or a variant's: their claims
// stand on their own, not against another run.
func ownClaims(rc *RunCheck, c Claim) {
	defer rc.settle()
	if !rc.usable() {
		return
	}
	rc.claim("verdict "+c.Verdict, rc.Verdict == c.Verdict, rc.Summary)
	rc.diagnosisClaims(c)
	rc.signalClaims(c)
	rc.recoveryClaim(c)
}

// recovered is whether the run was healthy at the end: it held, or it
// recovered from its break.
func (rc *RunCheck) recovered() bool {
	return rc.Verdict == string(analysis.VerdictHeld) || rc.RecoveredAt != nil
}

func (rc *RunCheck) recoveryClaim(c Claim) {
	if c.Recovers == nil {
		return
	}
	detail := "it hadn't recovered when the load ended"
	switch {
	case rc.Verdict == string(analysis.VerdictHeld):
		detail = "it held"
	case rc.RecoveredAt != nil:
		detail = "it recovered at " + analysis.Clock(*rc.RecoveredAt)
	}
	name := "recovers"
	if !*c.Recovers {
		name = "doesn't recover"
	}
	if !rc.recovered() && rc.RecoveryDoubt != "" {
		if rc.Inconclusive == "" {
			rc.Inconclusive = "whether it recovers can't be judged: " + rc.RecoveryDoubt
		}
		return
	}
	rc.claim(name, rc.recovered() == *c.Recovers, detail)
}

// solutionClaims judges a solution against the starting design's run. When
// the starting design held, the problem is the diagnoses claimed for it
// (stale reads from a replica that fell behind, say): the solution helps
// when it holds and none of them fire.
func solutionClaims(rc *RunCheck, c Claim, problem *RunCheck, problemDiagnoses []string) {
	defer rc.settle()
	// Still apart after a re-run, the two runs can't be compared: whatever
	// the claims say, the host's speed could be the difference.
	if apart(problem, rc) {
		after := ""
		if rc.rerun {
			after = " even after a re-run"
		}
		rc.Inconclusive = fmt.Sprintf("the host's speed index was %d, against %d in the starting design's run, %s apart, more than %s%s: the host's speed, not the design, could explain the difference",
			rc.SpeedIndex, problem.SpeedIndex, analysis.Pct(bench.Gap(problem.SpeedIndex, rc.SpeedIndex)), analysis.Pct(bench.Tolerance), after)
	}
	if !rc.usable() {
		return
	}
	if c.Verdict != "" {
		rc.claim("verdict "+c.Verdict, rc.Verdict == c.Verdict, rc.Summary)
	}
	rc.diagnosisClaims(c)
	rc.signalClaims(c)
	rc.recoveryClaim(c)

	gain := DefaultMinGain
	if c.MinGain != nil {
		gain = *c.MinGain
	}
	held := rc.Verdict == string(analysis.VerdictHeld)
	switch {
	case problem != nil && problem.Verdict == string(analysis.VerdictHeld) && len(problemDiagnoses) > 0:
		var still []string
		for _, d := range problemDiagnoses {
			if slices.Contains(rc.Diagnoses, d) {
				still = append(still, d)
			}
		}
		detail := fmt.Sprintf("held, and none of the starting design's %s fired", strings.Join(problemDiagnoses, ", "))
		switch {
		case !held:
			detail = "it didn't hold: " + rc.Summary
		case len(still) > 0:
			detail = "still fired: " + strings.Join(still, ", ")
		}
		rc.claim("helps (clears "+strings.Join(problemDiagnoses, ", ")+")", held && len(still) == 0, detail)
	case problem == nil || problem.BreakLoad == nil || problem.PeakRPS == nil:
		rc.claim("helps", false, "the starting design didn't break in its own run, so there's nothing to improve on")
	case held:
		rc.claim("helps", true, fmt.Sprintf("held through the profile; the starting design broke at %s rps offered", analysis.Rate(*problem.BreakLoad)))
	case c.Recovers != nil && *c.Recovers && rc.RecoveredAt != nil && rc.BreakAt != nil && !problem.recovered() && problem.RecoveryDoubt != "":
		if rc.Inconclusive == "" {
			rc.Inconclusive = "it recovered, but the starting design's not recovering can't be compared with: " + problem.RecoveryDoubt
		}
	case c.Recovers != nil && *c.Recovers && rc.RecoveredAt != nil && rc.BreakAt != nil && !problem.recovered():
		rc.claim("helps (recovers where the starting design didn't)", true, fmt.Sprintf("recovered at %s, %s after its break; the starting design hadn't recovered when the load ended", analysis.Clock(*rc.RecoveredAt), analysis.Clock(*rc.RecoveredAt-*rc.BreakAt)))
	case rc.PeakRPS == nil:
		rc.claim("helps", false, "no throughput data")
	default:
		ratio := *rc.PeakRPS / *problem.PeakRPS
		detail := fmt.Sprintf("served up to %s rps, %.2fx the starting design's %s rps", analysis.Rate(*rc.PeakRPS), ratio, analysis.Rate(*problem.PeakRPS))
		if rc.BreakLoad != nil {
			detail += fmt.Sprintf("; it broke at %s rps offered, the starting design at %s", analysis.Rate(*rc.BreakLoad), analysis.Rate(*problem.BreakLoad))
		}
		rc.claim(fmt.Sprintf("helps (serves %gx the peak throughput or more)", gain), ratio >= gain, detail)
	}
}

func (rc *RunCheck) diagnosisClaims(c Claim) {
	for _, want := range c.Diagnoses {
		got := "fired: none"
		if len(rc.Diagnoses) > 0 {
			got = "fired: " + strings.Join(rc.Diagnoses, ", ")
		}
		rc.claim("diagnosis "+want, slices.Contains(rc.Diagnoses, want), got)
	}
}

func (rc *RunCheck) signalClaims(c Claim) {
	for _, want := range c.Signals {
		got := "it didn't break"
		if rc.BreakAt != nil {
			got = "at the break: " + strings.Join(rc.BreakSignals, ", ")
		}
		rc.claim("break signal "+want, slices.Contains(rc.BreakSignals, want), got)
	}
}

// settle sets the run's status: inconclusive when its claims can't be
// judged, else ok when every claim held.
func (rc *RunCheck) settle() {
	switch {
	case rc.Inconclusive != "":
		rc.Status = StatusInconclusive
	case len(rc.Claims) > 0 && !slices.ContainsFunc(rc.Claims, func(c ClaimResult) bool { return !c.OK }):
		rc.Status = StatusOK
	default:
		rc.Status = StatusFailed
	}
	rc.OK = rc.Status == StatusOK
}
