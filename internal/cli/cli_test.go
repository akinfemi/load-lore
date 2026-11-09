package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
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
