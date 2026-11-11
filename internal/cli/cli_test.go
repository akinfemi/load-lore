package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/analysis/analysistest"
	"github.com/akinfemi/load-lore/internal/scenario"
)

// runCLI runs loadl with args and returns stdout, stderr and the exit code.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return out.String(), errOut.String(), code
}

func TestVersionJSON(t *testing.T) {
	out, _, code := runCLI(t, "version", "--json")
	if code != exitOK {
		t.Fatalf("exit code %d, want %d", code, exitOK)
	}
	var v map[string]string
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if v["version"] == "" {
		t.Errorf("version missing in %v", v)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	_, stderr, code := runCLI(t, "nope")
	if code != exitUsage {
		t.Fatalf("exit code %d, want %d", code, exitUsage)
	}
	if stderr == "" {
		t.Error("expected an error message on stderr")
	}
}

const example = "../../examples/single-postgres.yaml"

func writeDesign(t *testing.T, content string) string {
	t.Helper()
	p := t.TempDir() + "/design.yaml"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const broken = `version: 1
name: broken
services:
  api:
    image: loadlore/synthetic
    behavior:
      calls:
        - { target: dbb, op: read }
  db: { kind: postgres, dataset: { rows: 10 } }
workload:
  entry: api
  profile: [ { hold: { rps: 1, for: 1m } } ]
`

func TestValidateValid(t *testing.T) {
	out, _, code := runCLI(t, "validate", example)
	if code != exitOK || !strings.Contains(out, "valid (0 errors, 0 warnings)") {
		t.Fatalf("code %d, output %q", code, out)
	}
}

func TestValidateJSON(t *testing.T) {
	out, _, code := runCLI(t, "validate", "--json", writeDesign(t, broken))
	if code != exitFailure {
		t.Fatalf("exit code %d, want %d", code, exitFailure)
	}
	var res struct {
		Valid  bool `json:"valid"`
		Errors []struct {
			Path    string `json:"path"`
			Line    int    `json:"line"`
			Column  int    `json:"column"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Warnings []any `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if res.Valid || len(res.Errors) != 1 || res.Warnings == nil {
		t.Fatalf("unexpected result: %s", out)
	}
	e := res.Errors[0]
	if e.Path != "services.api.behavior.calls[0].target" || e.Code != "unknown-service" || e.Line != 8 || e.Column != 21 {
		t.Errorf("unexpected error: %+v", e)
	}
}

func TestValidateText(t *testing.T) {
	path := writeDesign(t, broken)
	out, _, code := runCLI(t, "validate", path)
	want := path + `:8:21: error: services.api.behavior.calls[0].target: unknown service "dbb" (did you mean "db"?)`
	if code != exitFailure || !strings.Contains(out, want) || !strings.Contains(out, "invalid (1 error, 0 warnings)") {
		t.Fatalf("code %d, output:\n%s", code, out)
	}
}

func TestValidateStdin(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetArgs([]string{"validate", "-", "--json"})
	root.SetIn(strings.NewReader(broken))
	root.SetOut(&out)
	if err := root.Execute(); err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(out.String(), `"file": "-"`) {
		t.Errorf("output: %s", out.String())
	}
}

func TestValidateMissingFile(t *testing.T) {
	_, stderr, code := runCLI(t, "validate", "nope.yaml")
	if code != exitUsage || !strings.Contains(stderr, "nope.yaml") {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

func TestSchemaJSON(t *testing.T) {
	out, _, code := runCLI(t, "schema", "--json")
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	var s map[string]any
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatal(err)
	}
	props, _ := s["properties"].(map[string]any)
	for _, k := range []string{"version", "name", "services", "workload", "scenario", "expect", "assumptions", "modes"} {
		if _, ok := props[k]; !ok {
			t.Errorf("schema has no %q", k)
		}
	}
}

func TestDiagram(t *testing.T) {
	out, _, code := runCLI(t, "diagram", example)
	if code != exitOK || !strings.HasPrefix(out, "flowchart LR\n") {
		t.Fatalf("code %d, output %q", code, out)
	}
	out, _, code = runCLI(t, "diagram", "--ascii", example)
	if code != exitOK || !strings.Contains(out, "┌") || !strings.Contains(out, "postgres 17") {
		t.Fatalf("code %d, output %q", code, out)
	}
	out, _, code = runCLI(t, "diagram", "--json", example)
	var res map[string]string
	if code != exitOK || json.Unmarshal([]byte(out), &res) != nil || res["mermaid"] == "" || res["ascii"] == "" {
		t.Fatalf("code %d, output %q", code, out)
	}
}

func TestDiagramInvalidDesign(t *testing.T) {
	out, stderr, code := runCLI(t, "diagram", writeDesign(t, broken))
	if code != exitFailure || out != "" || !strings.Contains(stderr, "unknown service") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out, stderr)
	}
}

func TestReportAndCompare(t *testing.T) {
	a := analysistest.WriteRun(t, analysistest.Healthy(), 1)
	b := analysistest.WriteRun(t, analysistest.Healthy(), 0.5)

	out, stderr, code := runCLI(t, "report", a)
	if code != exitOK || !strings.Contains(out, "Held the whole profile") || !strings.Contains(out, "report: ") {
		t.Fatalf("report: code %d\n%s%s", code, out, stderr)
	}
	if _, err := os.Stat(filepath.Join(a, "report.md")); err != nil {
		t.Error("report.md not written")
	}

	out, _, code = runCLI(t, "report", a, "--json")
	var an struct {
		Verdict struct{ Kind string } `json:"verdict"`
		Windows []any                 `json:"windows"`
	}
	if code != exitOK || json.Unmarshal([]byte(out), &an) != nil || an.Verdict.Kind != "held" || len(an.Windows) != 10 {
		t.Errorf("report --json: code %d\n%.300s", code, out)
	}

	out, _, code = runCLI(t, "compare", a, b)
	if code != exitOK || !strings.Contains(out, "# Compare: fixture") || !strings.Contains(out, "different scales (1 and 0.5)") {
		t.Errorf("compare: code %d\n%.500s", code, out)
	}

	_, stderr, code = runCLI(t, "report", filepath.Join(t.TempDir(), "nope"))
	if code != exitUsage || !strings.Contains(stderr, "no run") {
		t.Errorf("missing run: code %d, %s", code, stderr)
	}
}

func TestScenarios(t *testing.T) {
	out, _, code := runCLI(t, "scenarios")
	if code != exitOK || !strings.Contains(out, "when-to-scale-your-database") || !strings.Contains(out, "When do you need to scale your database?") {
		t.Errorf("list: code %d\n%s", code, out)
	}
	out, _, _ = runCLI(t, "scenarios", "QUEUEING knee", "--json")
	var list []struct {
		Name string `json:"name"`
		Meta struct {
			Level int `json:"level"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil || len(list) != 1 || list[0].Meta.Level != 1 {
		t.Errorf("search: %v\n%s", err, out)
	}
	out, _, _ = runCLI(t, "scenarios", "kafka")
	if !strings.Contains(out, `no scenario matches "kafka"`) {
		t.Errorf("no match: %s", out)
	}
}

func TestHint(t *testing.T) {
	repo, _ := filepath.Abs("../..")
	t.Chdir(t.TempDir())

	_, stderr, code := runCLI(t, "hint")
	if code != exitUsage || !strings.Contains(stderr, "no current scenario") {
		t.Errorf("no scenario: code %d %s", code, stderr)
	}

	out, _, code := runCLI(t, "hint", "when-to-scale-your-database")
	if code != exitOK || !strings.HasPrefix(out, "Hint 1 of 5 · Start with the verdict") || !strings.Contains(out, "Next: loadl hint (2 of 5)") {
		t.Errorf("first: code %d\n%s", code, out)
	}
	// The latest run's design picks the scenario: here, a solution's.
	run := filepath.Join(".loadlore", "runs", "x-20261110-000000")
	if err := os.MkdirAll(run, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := `{"design":"whatever","design_path":"` + filepath.Join(repo, "scenarios/when-to-scale-your-database/solutions/one-query-per-page/design.yaml") + `"}`
	if err := os.WriteFile(filepath.Join(run, "run.json"), []byte(rec), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, _ = runCLI(t, "hint", "--json")
	var h hintResult
	if err := json.Unmarshal([]byte(out), &h); err != nil || h.Number != 2 || h.Scenario != "when-to-scale-your-database" || h.Body == "" {
		t.Errorf("second: %v %+v", err, h)
	}
	for range 3 {
		runCLI(t, "hint")
	}
	out, _, _ = runCLI(t, "hint")
	if !strings.HasPrefix(out, "All 5 hints") {
		t.Errorf("after the last: %s", out)
	}
	out, _, _ = runCLI(t, "hint", "--show", "2")
	if !strings.HasPrefix(out, "Hint 2 of 5") {
		t.Errorf("show: %s", out)
	}
	runCLI(t, "hint", "--reset")
	_, stderr, code = runCLI(t, "hint", "--show", "1")
	if code != exitUsage || !strings.Contains(stderr, "isn't revealed yet") {
		t.Errorf("show after reset: %d %s", code, stderr)
	}
}

func TestScenarioCheckStatic(t *testing.T) {
	out, _, code := runCLI(t, "scenario", "check", "../../scenarios/when-to-scale-your-database", "--static")
	if code != exitOK || !strings.Contains(out, "ok (files only") {
		t.Errorf("folder: code %d\n%s", code, out)
	}
	// A library name works too, from the built-in copy.
	out, _, code = runCLI(t, "scenario", "check", "when-to-scale-your-database", "--static", "--json")
	var res struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
	}
	if code != exitOK || json.Unmarshal([]byte(out), &res) != nil || !res.OK || res.Status != scenario.StatusOK {
		t.Errorf("by name: code %d\n%s", code, out)
	}
	_, stderr, code := runCLI(t, "scenario", "check", "no-such-scenario", "--static")
	if code != exitUsage || !strings.Contains(stderr, `no scenario "no-such-scenario"`) {
		t.Errorf("unknown: %d %s", code, stderr)
	}
}

// A check whose solution is still too far from the starting design in host
// speed after a re-run ends INCONCLUSIVE and exits like a failed one.
func TestPrintCheckInconclusive(t *testing.T) {
	r := &scenario.CheckResult{
		Scenario: "demo", Lint: []scenario.Problem{}, Status: scenario.StatusInconclusive,
		Problem: &scenario.RunCheck{Name: "problem", RunDir: "/runs/demo-2", Status: scenario.StatusOK, OK: true,
			Claims:   []scenario.ClaimResult{{Claim: "verdict broke", OK: true, Detail: "broke at 1:10"}},
			Replaced: []scenario.Rerun{{RunDir: "/runs/demo-1", SpeedIndex: 874, Against: "bigger", AgainstSpeed: 1012, NewSpeed: 990}}},
		Solutions: []*scenario.RunCheck{{Name: "bigger", RunDir: "/runs/bigger-1", Status: scenario.StatusInconclusive,
			Claims:       []scenario.ClaimResult{{Claim: "helps", OK: true, Detail: "held through the profile"}},
			Inconclusive: "the host's speed index was 1200, against 990 in the starting design's run"}},
	}
	var b bytes.Buffer
	printCheck(&b, r, false)
	out := b.String()
	for _, want := range []string{
		"  starting design (run demo-2)\n    re-ran the starting design: speed index 874 against 1012 for solution bigger; now 990 (replaces run demo-1)\n",
		"    inconclusive: the host's speed index was 1200, against 990",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if !strings.HasSuffix(out, "\nINCONCLUSIVE\n") {
		t.Errorf("last line:\n%s", out)
	}
	var ee *exitError
	if err := checkExit(r); !errors.As(err, &ee) || ee.code != exitFailure {
		t.Errorf("exit: %v", err)
	}

	r.Status = scenario.StatusFailed
	b.Reset()
	printCheck(&b, r, false)
	if !strings.HasSuffix(b.String(), "\nFAILED\n") {
		t.Errorf("failed:\n%s", b.String())
	}
	r.Status, r.OK = scenario.StatusOK, true
	if err := checkExit(r); err != nil {
		t.Errorf("ok exit: %v", err)
	}
}

// Variants print after the solutions, labelled as variants.
func TestPrintCheckVariant(t *testing.T) {
	r := &scenario.CheckResult{
		Scenario: "demo", Lint: []scenario.Problem{}, Status: scenario.StatusFailed,
		Problem: &scenario.RunCheck{Name: "problem", RunDir: "/runs/demo-1", Status: scenario.StatusOK, OK: true,
			Claims: []scenario.ClaimResult{{Claim: "verdict broke", OK: true, Detail: "broke at 1:10"}}},
		Solutions: []*scenario.RunCheck{{Name: "bigger", RunDir: "/runs/bigger-1", Status: scenario.StatusOK, OK: true,
			Claims: []scenario.ClaimResult{{Claim: "helps", OK: true, Detail: "held through the profile"}}}},
		Variants: []*scenario.RunCheck{{Name: "bigger-pools", RunDir: "/runs/bigger-pools-1", Status: scenario.StatusFailed,
			Claims: []scenario.ClaimResult{{Claim: "verdict broke", OK: true, Detail: "broke at 1:10"}, {Claim: "break signal errors", OK: false, Detail: "at the break: latency"}}}},
	}
	var b bytes.Buffer
	printCheck(&b, r, false)
	want := "  solution bigger (run bigger-1)\n    ok   helps: held through the profile\n" +
		"  variant bigger-pools (run bigger-pools-1)\n    ok   verdict broke: broke at 1:10\n    FAIL break signal errors: at the break: latency\nFAILED\n"
	if out := b.String(); !strings.HasSuffix(out, want) {
		t.Errorf("got\n%s\nwant it to end\n%s", out, want)
	}
}
