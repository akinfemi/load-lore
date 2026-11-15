package scenario

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/akinfemi/load-lore/internal/plan"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/diagram"
)

// Problem is something wrong with a scenario folder.
type Problem struct {
	File    string `json:"file"`
	Message string `json:"message"`
	Warning bool   `json:"warning,omitempty"`
}

func (p Problem) String() string {
	kind := "error"
	if p.Warning {
		kind = "warning"
	}
	return fmt.Sprintf("%s: %s: %s", p.File, kind, p.Message)
}

// breakSignals are the signals a break can show, as analysis.Break.Signals.
var breakSignals = []string{"latency", "errors", "throughput"}

// Lint checks a scenario folder without running anything: every file is
// there, every design validates, the README's diagram matches its design,
// and the claims name real solutions, variants, rules and signals.
func (s *Scenario) Lint() []Problem {
	var ps []Problem
	bad := func(file, format string, args ...any) {
		ps = append(ps, Problem{File: file, Message: fmt.Sprintf(format, args...)})
	}
	warn := func(file, format string, args ...any) {
		ps = append(ps, Problem{File: file, Message: fmt.Sprintf(format, args...), Warning: true})
	}

	m := s.Meta
	if strings.TrimSpace(m.Title) == "" {
		bad(MetaFile, "title is required")
	}
	if m.Level < 1 || m.Level > 3 {
		bad(MetaFile, "level must be 1, 2 or 3, not %d", m.Level)
	}
	if len(m.Concepts) == 0 {
		bad(MetaFile, "concepts must list at least one concept the scenario teaches")
	}
	if m.Minutes <= 0 {
		bad(MetaFile, "minutes (a rough time to work through it) is required")
	}
	if m.Host.CPUs <= 0 || m.Host.Memory <= 0 {
		bad(MetaFile, "host.cpus and host.memory are required: what Docker needs available at --scale 1, Loadlore's own containers included")
	}
	if m.Host.Reference == "" || m.Host.Speed <= 0 {
		// They come from the full check's runs, like the reference
		// reports, so a draft without them is only warned.
		warn(MetaFile, "no host.reference and host.speed yet: after the full loadl scenario check passes, add the machine it ran on and its host speed index, as loadl run prints it")
	}
	for i, q := range m.Predict {
		if p := q.Problem(); p != "" {
			bad(MetaFile, "predict[%d]: %s", i, p)
		}
	}
	if len(m.Predict) == 0 {
		warn(MetaFile, "predict lists no questions, so loadl predict has nothing to ask; add the README's questions with the expect line each answer becomes")
	}
	if v := m.Check.Problem.Verdict; v != "broke" && v != "held" {
		bad(MetaFile, "check.problem.verdict must be broke or held, not %q", v)
	}
	claims := map[string]Claim{"check.problem": m.Check.Problem}
	for name, c := range m.Check.Solutions {
		claims["check.solutions."+name] = c
		if !slices.Contains(s.Solutions, name) {
			bad(MetaFile, "check.solutions.%s: there's no solutions/%s folder", name, name)
		}
	}
	for name, c := range m.Check.Variants {
		claims["check.variants."+name] = c
		if !slices.Contains(s.Variants, name) {
			bad(MetaFile, "check.variants.%s: there's no variants/%s folder", name, name)
		}
		if c.Verdict == "" {
			bad(MetaFile, "check.variants.%s.verdict is required: broke or held, what the variant must do", name)
		}
	}
	for _, name := range s.Variants {
		if _, ok := m.Check.Variants[name]; !ok {
			bad(MetaFile, "check.variants.%s is required: what a run of variants/%s must show, such as { verdict: broke, signals: [errors] }", name, name)
		}
	}
	for at, c := range claims {
		for _, r := range c.Diagnoses {
			if !slices.Contains(analysis.RuleIDs, r) {
				bad(MetaFile, "%s.diagnoses: no rule %q; this version has %s", at, r, strings.Join(analysis.RuleIDs, ", "))
			}
		}
		if c.Verdict != "" && c.Verdict != "broke" && c.Verdict != "held" {
			bad(MetaFile, "%s.verdict must be broke or held, not %q", at, c.Verdict)
		}
		for _, sg := range c.Signals {
			if !slices.Contains(breakSignals, sg) {
				bad(MetaFile, "%s.signals: no signal %q; a break's signals are %s", at, sg, strings.Join(breakSignals, ", "))
			}
		}
		if len(c.Signals) > 0 && c.Verdict != "broke" {
			bad(MetaFile, "%s.signals need verdict broke: only a run that broke has break signals", at)
		}
		if c.MinGain != nil && *c.MinGain <= 1 {
			bad(MetaFile, "%s.min_gain must be above 1", at)
		}
	}

	if l := s.Meta.Load; l != nil {
		switch {
		case l.PreallocVUs < 1 || l.PeakVUs < 0:
			bad(MetaFile, "load.prealloc_vus must be at least 1 and load.peak_vus at least 0")
		case float64(l.PreallocVUs) < math.Min(LoadMargin*float64(l.PeakVUs), plan.MaxVUsCap):
			bad(MetaFile, "load.prealloc_vus %d is under %g x load.peak_vus %d (or the load generator's cap of %d)", l.PreallocVUs, LoadMargin, l.PeakVUs, plan.MaxVUsCap)
		case l.PreallocVUs > plan.MaxVUsCap:
			bad(MetaFile, "load.prealloc_vus %d is over the load generator's cap of %d", l.PreallocVUs, plan.MaxVUsCap)
		case l.From == "":
			bad(MetaFile, "load.from: name the run load.peak_vus was measured in")
		}
	}

	d := s.lintDesign(DesignFile, s.Name, "a scenario's design is named after its folder", &ps)
	readme, err := s.ReadFile(ReadmeFile)
	switch {
	case err != nil:
		bad(ReadmeFile, "missing")
	case d != nil:
		if want := diagram.Flowchart(d).String(); !strings.Contains(string(readme), "```mermaid\n"+want+"```") {
			bad(ReadmeFile, "its Mermaid diagram doesn't match design.yaml; paste the output of `loadl diagram %s` into a ```mermaid block", DesignFile)
		}
	}
	if _, err := s.ReadFile(HintsFile); err != nil {
		bad(HintsFile, "missing")
	} else if len(s.Hints) == 0 {
		bad(HintsFile, `no hints: start each one with a "## " heading`)
	}

	if len(s.Solutions) == 0 {
		bad(SolutionsDir, "at least one solution is required, as solutions/<name>/design.yaml")
	}
	for _, name := range s.Solutions {
		s.lintDesign(SolutionDesign(name), "", "", &ps)
		if _, err := s.ReadFile(SolutionsDir + "/" + name + "/" + ReadmeFile); err != nil {
			bad(SolutionsDir+"/"+name+"/"+ReadmeFile, "missing: say what changed and why it helps")
		}
		if _, err := s.ReadFile(SolutionsDir + "/" + name + "/" + ReportFile); err != nil {
			warn(SolutionsDir+"/"+name+"/"+ReportFile, "no reference report; copy one from a run of this design")
		}
	}
	for _, name := range s.Variants {
		s.lintDesign(VariantDesign(name), s.Name+"-"+name, "a variant's design is named <scenario>-<variant>", &ps)
		if _, err := s.ReadFile(VariantsDir + "/" + name + "/" + ReadmeFile); err != nil {
			bad(VariantsDir+"/"+name+"/"+ReadmeFile, "missing: say what the variant is and what it shows")
		}
	}
	slices.SortStableFunc(ps, func(a, b Problem) int { return strings.Compare(a.File, b.File) })
	return ps
}

// lintDesign loads one design and records its errors. name, when set, is
// the name the design must have, and why says why.
func (s *Scenario) lintDesign(file, name, why string, ps *[]Problem) *design.Design {
	raw, err := s.ReadFile(file)
	if err != nil {
		*ps = append(*ps, Problem{File: file, Message: "missing"})
		return nil
	}
	d, diags := design.Load(raw)
	for _, e := range diags.Errors() {
		*ps = append(*ps, Problem{File: file, Message: diagText(e)})
	}
	if diags.HasErrors() {
		return nil
	}
	for _, w := range diags.Warnings() {
		*ps = append(*ps, Problem{File: file, Message: diagText(w), Warning: true})
	}
	if name != "" && d.Name != name {
		*ps = append(*ps, Problem{File: file, Message: fmt.Sprintf("name is %q; %s, %q", d.Name, why, name)})
	}
	for _, m := range postgresFloor(d) {
		*ps = append(*ps, Problem{File: file, Message: m, Warning: true})
	}
	return d
}

// MinPostgresCPU is the smallest CPU limit a Postgres has been measured to
// behave at in a scenario, at --scale 1. At 0.1 CPU (the first scenario at
// --scale 0.25) it stalled under CPU throttling from the start of the
// ramp, which is a different lesson; between the two is unmeasured.
const MinPostgresCPU = 0.4

// postgresFloor warns about each Postgres below MinPostgresCPU. Splitting a
// fixed budget across replicas or shards stops here.
func postgresFloor(d *design.Design) []string {
	var out []string
	for _, name := range slices.Sorted(maps.Keys(d.Services)) {
		svc := d.Services[name]
		if !svc.IsPostgres() || svc.Resources == nil || svc.Resources.CPU == nil || *svc.Resources.CPU >= MinPostgresCPU {
			continue
		}
		out = append(out, fmt.Sprintf("services.%s has %g CPU, under %g, the smallest a Postgres has been measured to behave at: below it, CPU throttling can stall it from the start of the load and the run teaches something else. Give it %g or more", name, *svc.Resources.CPU, MinPostgresCPU, MinPostgresCPU))
	}
	return out
}

// HasErrors reports whether any problem isn't a warning.
func HasErrors(ps []Problem) bool {
	return slices.ContainsFunc(ps, func(p Problem) bool { return !p.Warning })
}

func diagText(d design.Diagnostic) string {
	loc := ""
	if d.Line > 0 {
		loc = fmt.Sprintf("line %d: ", d.Line)
	}
	if d.Path != "" {
		loc += d.Path + ": "
	}
	return loc + d.Message
}
