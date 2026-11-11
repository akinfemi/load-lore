package scenario

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/analysis/analysistest"
	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/diagram"
)

// Every scenario in the library passes its static checks, and its design
// is the same as the spec's example where one exists.
func TestLibraryLints(t *testing.T) {
	lib, err := Library()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib) == 0 {
		t.Fatal("empty library")
	}
	for _, s := range lib {
		disk, err := Open(filepath.Join("../../scenarios", s.Name))
		if err != nil {
			t.Fatal(err)
		}
		for _, sc := range []*Scenario{s, disk} {
			for _, p := range sc.Lint() {
				if !p.Warning {
					t.Errorf("%s: %s", s.Name, p)
				}
			}
		}
		if len(s.Hints) < 3 {
			t.Errorf("%s: %d hints", s.Name, len(s.Hints))
		}
	}
}

// The first scenario starts from the spec's example: same design, other
// comments.
func TestBreakingPointIsTheSpecExample(t *testing.T) {
	load := func(p string) string {
		d, diags, err := design.LoadFile(p)
		if err != nil || diags.HasErrors() {
			t.Fatal(err, diags)
		}
		d.Expect = nil // the scenario's predictions are the learner's
		b, _ := yamlJSON(d)
		return b
	}
	if a, b := load("../../examples/single-postgres.yaml"), load("../../scenarios/single-postgres-breaking-point/design.yaml"); a != b {
		t.Errorf("the scenario's design drifted from examples/single-postgres.yaml:\n%s\n%s", a, b)
	}
}

func writeScenario(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "demo")
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const demoDesign = `version: 1
name: demo
services:
  api:
    image: loadlore/synthetic
    behavior: { calls: [ { target: db, op: read } ] }
  db:
    kind: postgres
    dataset: { rows: 1000 }
workload:
  entry: api
  profile: [ { hold: { rps: 10, for: 30s } } ]
`

func demoFiles() map[string]string {
	d, _ := design.Load([]byte(demoDesign))
	readme := "# Demo\n\n```mermaid\n" + flowchart(d) + "```\n"
	return map[string]string{
		"meta.yaml": `title: Demo
level: 1
concepts: [pools]
minutes: 10
host: { cpus: 2, memory: 2g, reference: test machine, speed: 1000 }
check:
  problem: { verdict: broke, diagnoses: [pool-exhausted] }
  solutions:
    bigger: { min_gain: 1.5 }
`,
		"design.yaml":                  demoDesign,
		"README.md":                    readme,
		"hints.md":                     "# Hints\n\nIntro.\n\n## One\n\nFirst.\n\n```sh\n## not a hint\n```\n\n## Two\n\nSecond.\n",
		"solutions/bigger/design.yaml": strings.Replace(demoDesign, "name: demo", "name: demo-bigger", 1),
		"solutions/bigger/README.md":   "# Bigger\n",
	}
}

func TestLint(t *testing.T) {
	s, err := Open(writeScenario(t, demoFiles()))
	if err != nil {
		t.Fatal(err)
	}
	ps := s.Lint()
	if HasErrors(ps) || len(ps) != 1 || !strings.Contains(ps[0].String(), "report.md: warning") {
		t.Errorf("clean folder: %v", ps)
	}
	if len(s.Hints) != 2 || s.Hints[0].Title != "One" || !strings.Contains(s.Hints[0].Body, "## not a hint") || s.Hints[1].Body != "Second." {
		t.Errorf("hints: %+v", s.Hints)
	}

	files := demoFiles()
	files["README.md"] = "# Demo\n\nno diagram\n"
	files["design.yaml"] = strings.Replace(demoDesign, "name: demo", "name: other", 1)
	files["meta.yaml"] = strings.Replace(strings.Replace(files["meta.yaml"], "pool-exhausted", "made-up", 1), "bigger:", "missing:", 1)
	files["hints.md"] = "no sections\n"
	s, err = Open(writeScenario(t, files))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range s.Lint() {
		got = append(got, p.String())
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{
		`README.md: error: its Mermaid diagram doesn't match design.yaml`,
		`design.yaml: error: name is "other"`,
		`hints.md: error: no hints`,
		`meta.yaml: error: check.problem.diagnoses: no rule "made-up"`,
		`meta.yaml: error: check.solutions.missing: there's no solutions/missing folder`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}

	if _, err := Open(writeScenario(t, map[string]string{"meta.yaml": "title: x\nsurprise: 1\n"})); err == nil || !strings.Contains(err.Error(), "surprise") {
		t.Errorf("unknown meta field: %v", err)
	}
}

func TestLintVariants(t *testing.T) {
	s, err := Open(writeScenario(t, variantFiles("{ verdict: broke, signals: [latency] }", "{ verdict: broke, signals: [errors] }")))
	if err != nil {
		t.Fatal(err)
	}
	if ps := s.Lint(); HasErrors(ps) || len(s.Variants) != 1 || s.Variants[0] != "bigger-pools" {
		t.Errorf("clean folder: variants %v, %v", s.Variants, ps)
	}

	// orphan has no claim, ghost has no folder, and bigger-pools is
	// misnamed and has no README.
	files := variantFiles("{ verdict: broke, signals: [slow] }", "{ signals: [errors] }")
	files["meta.yaml"] += "    ghost: { verdict: held, signals: [errors] }\n"
	files["variants/orphan/design.yaml"] = strings.Replace(demoDesign, "name: demo", "name: demo-orphan", 1)
	files["variants/orphan/README.md"] = "# Orphan\n"
	files["variants/bigger-pools/design.yaml"] = demoDesign
	delete(files, "variants/bigger-pools/README.md")
	s, err = Open(writeScenario(t, files))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range s.Lint() {
		got = append(got, p.String())
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{
		`meta.yaml: error: check.problem.signals: no signal "slow"; a break's signals are latency, errors, throughput`,
		`meta.yaml: error: check.variants.bigger-pools.verdict is required`,
		`meta.yaml: error: check.variants.bigger-pools.signals need verdict broke`,
		`meta.yaml: error: check.variants.ghost: there's no variants/ghost folder`,
		`meta.yaml: error: check.variants.ghost.signals need verdict broke`,
		`meta.yaml: error: check.variants.orphan is required: what a run of variants/orphan must show`,
		`variants/bigger-pools/README.md: error: missing: say what the variant is and what it shows`,
		`variants/bigger-pools/design.yaml: error: name is "demo"; a variant's design is named <scenario>-<variant>, "demo-bigger-pools"`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	if strings.Contains(all, "variants/orphan/") {
		t.Errorf("orphan's own files are fine:\n%s", all)
	}
}

func TestForDesign(t *testing.T) {
	dir := writeScenario(t, variantFiles("{ verdict: broke }", "{ verdict: broke }"))
	for _, p := range []string{filepath.Join(dir, "design.yaml"), filepath.Join(dir, "solutions", "bigger", "design.yaml"), filepath.Join(dir, "variants", "bigger-pools", "design.yaml")} {
		if s, ok := ForDesign(p); !ok || s.Name != "demo" {
			t.Errorf("%s: %v", p, ok)
		}
	}
	if _, ok := ForDesign("../../examples/smoke.yaml"); ok {
		t.Error("an example isn't a scenario")
	}
}

func TestCheck(t *testing.T) {
	dir := writeScenario(t, demoFiles())
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Stand-in runs: the fixture's system, healthy or breaking at 70 s.
	healthy := analysistest.WriteRun(t, analysistest.Healthy(), 1)
	brokeW := analysistest.Healthy()
	brokeW.P99 = func(t float64) float64 { return 0.005 + 2*boolF(t > 70) }
	broke := analysistest.WriteRun(t, brokeW, 1)
	runs := map[string]string{}
	run := func(_ context.Context, p string, _ int) (string, error) {
		if d, ok := runs[filepath.Base(filepath.Dir(p))]; ok {
			if d == "" {
				return "", errors.New("docker isn't reachable")
			}
			return d, nil
		}
		return "", errors.New("unexpected design " + p)
	}

	// The fixture breaks on latency with no rule firing, so the
	// problem's diagnosis claim fails; the solution held, so it helps.
	runs["demo"], runs["bigger"] = broke, healthy
	res := Check(context.Background(), s, dir, run)
	if res.OK || res.Status != StatusFailed || res.Problem.Verdict != "broke" || res.Problem.OK || res.Problem.Status != StatusFailed {
		t.Errorf("problem: %+v", res.Problem)
	}
	if c := res.Problem.Claims; len(c) != 2 || !c[0].OK || c[1].OK || c[1].Detail != "fired: none" {
		t.Errorf("problem claims: %+v", c)
	}
	if sol := res.Solutions[0]; !sol.OK || sol.Claims[0].Claim != "helps" || !strings.HasPrefix(sol.Claims[0].Detail, "held through the profile") {
		t.Errorf("solution: %+v", sol)
	}

	// A solution that serves no more doesn't help.
	runs["bigger"] = broke
	res = Check(context.Background(), s, dir, run)
	if c := res.Solutions[0].Claims[0]; c.OK || !strings.Contains(c.Detail, "served up to 950 rps, 1.00x the starting design's 950 rps; it broke at 750 rps offered") {
		t.Errorf("same load: %+v", c)
	}

	// A run that fails is a failed claim, not a crash.
	runs["bigger"] = ""
	res = Check(context.Background(), s, dir, run)
	if c := res.Solutions[0].Claims[0]; c.OK || c.Claim != "the run finishes" {
		t.Errorf("failed run: %+v", c)
	}

	// Static: no runs.
	res = Check(context.Background(), s, dir, nil)
	if !res.OK || res.Problem != nil {
		t.Errorf("static: %+v", res)
	}
}

func boolF(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func flowchart(d *design.Design) string { return diagram.Flowchart(d).String() }

func yamlJSON(d *design.Design) (string, error) {
	b, err := json.Marshal(d)
	return string(b), err
}
