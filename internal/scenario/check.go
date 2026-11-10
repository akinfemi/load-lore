package scenario

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/akinfemi/load-lore/internal/analysis"
)

// RunFunc runs one design and returns its run folder. A run that started
// but failed returns its folder along with the error.
type RunFunc func(ctx context.Context, designPath string) (runDir string, err error)

// CheckResult is the outcome of `loadl scenario check`.
type CheckResult struct {
	Scenario  string      `json:"scenario"`
	Path      string      `json:"path"`
	OK        bool        `json:"ok"`
	Lint      []Problem   `json:"lint"`
	Problem   *RunCheck   `json:"problem,omitempty"`
	Solutions []*RunCheck `json:"solutions"`
}

// RunCheck is one design's run and its claims.
type RunCheck struct {
	Name      string        `json:"name"` // "problem" or the solution's name
	Design    string        `json:"design"`
	RunDir    string        `json:"run_dir,omitempty"`
	Error     string        `json:"error,omitempty"`
	Verdict   string        `json:"verdict,omitempty"`
	Summary   string        `json:"summary,omitempty"`
	BreakAt   *float64      `json:"break_at_s,omitempty"`
	BreakLoad *float64      `json:"break_offered_rps,omitempty"`
	PeakRPS   *float64      `json:"peak_rps,omitempty"` // most successful responses/s in any window
	Diagnoses []string      `json:"diagnoses"`
	Claims    []ClaimResult `json:"claims"`
	OK        bool          `json:"ok"`
}

// ClaimResult is one claim and whether the run showed it.
type ClaimResult struct {
	Claim  string `json:"claim"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// Check lints the folder, then runs the starting design and each solution
// in turn and checks their claims. With run nil it only lints.
func Check(ctx context.Context, s *Scenario, dir string, run RunFunc) *CheckResult {
	res := &CheckResult{Scenario: s.Name, Path: dir, Lint: s.Lint(), Solutions: []*RunCheck{}}
	if res.Lint == nil {
		res.Lint = []Problem{}
	}
	res.OK = !HasErrors(res.Lint)
	if run == nil || !res.OK {
		return res
	}
	res.Problem = runOne(ctx, run, "problem", filepath.Join(dir, DesignFile))
	problemClaims(res.Problem, s.Meta.Check.Problem)
	for _, name := range s.Solutions {
		if ctx.Err() != nil {
			break
		}
		rc := runOne(ctx, run, name, filepath.Join(dir, filepath.FromSlash(SolutionDesign(name))))
		solutionClaims(rc, s.Meta.Check.Solutions[name], res.Problem)
		res.Solutions = append(res.Solutions, rc)
	}
	res.OK = res.Problem.OK && len(res.Solutions) == len(s.Solutions)
	for _, rc := range res.Solutions {
		res.OK = res.OK && rc.OK
	}
	return res
}

func runOne(ctx context.Context, run RunFunc, name, designPath string) *RunCheck {
	rc := &RunCheck{Name: name, Design: designPath, Diagnoses: []string{}, Claims: []ClaimResult{}}
	dir, err := run(ctx, designPath)
	rc.RunDir = dir
	if err != nil {
		rc.Error = err.Error()
		return rc
	}
	a, err := analysis.Analyze(dir)
	if err != nil {
		rc.Error = err.Error()
		return rc
	}
	rc.Verdict, rc.Summary = string(a.Verdict.Kind), a.Verdict.Summary
	rc.PeakRPS = a.Verdict.MaxRPS
	if b := a.Verdict.Break; b != nil {
		at, load := b.At, b.Offered
		rc.BreakAt, rc.BreakLoad = &at, &load
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

func problemClaims(rc *RunCheck, c Claim) {
	defer rc.settle()
	if !rc.usable() {
		return
	}
	rc.claim("verdict "+c.Verdict, rc.Verdict == c.Verdict, rc.Summary)
	rc.diagnosisClaims(c)
}

func solutionClaims(rc *RunCheck, c Claim, problem *RunCheck) {
	defer rc.settle()
	if !rc.usable() {
		return
	}
	if c.Verdict != "" {
		rc.claim("verdict "+c.Verdict, rc.Verdict == c.Verdict, rc.Summary)
	}
	rc.diagnosisClaims(c)

	gain := DefaultMinGain
	if c.MinGain != nil {
		gain = *c.MinGain
	}
	held := rc.Verdict == string(analysis.VerdictHeld)
	switch {
	case problem == nil || problem.BreakLoad == nil || problem.PeakRPS == nil:
		rc.claim("helps", false, "the starting design didn't break in its own run, so there's nothing to improve on")
	case held:
		rc.claim("helps", true, fmt.Sprintf("held through the profile; the starting design broke at %s rps offered", analysis.Rate(*problem.BreakLoad)))
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

func (rc *RunCheck) settle() {
	rc.OK = len(rc.Claims) > 0 && !slices.ContainsFunc(rc.Claims, func(c ClaimResult) bool { return !c.OK })
}
