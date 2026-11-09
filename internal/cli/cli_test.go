package cli

import (
	"bytes"
	"encoding/json"
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
