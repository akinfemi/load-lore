package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/scenario"
)

// Every combination the wizard offers writes a design that validates.
func TestInitDesignsValidate(t *testing.T) {
	for _, store := range []string{"postgres", "cache", "none"} {
		for _, fault := range []string{"none", "kill", "slow-db", "cache-restart"} {
			for _, replicas := range []int{1, 3} {
				a := defaultInitAnswers()
				a.Store, a.Fault, a.Replicas = store, fault, replicas
				src, err := initDesign(a)
				fits := fault == "none" || fault == "kill" || fault == "slow-db" && store != "none" || fault == "cache-restart" && store == "cache"
				if !fits {
					if err == nil {
						t.Errorf("%s/%s: no error for a fault that doesn't fit", store, fault)
					}
					continue
				}
				if err != nil {
					t.Fatalf("%s/%s/%d: %v", store, fault, replicas, err)
				}
				d, diags := design.Load([]byte(src))
				if diags.HasErrors() || d.Name != a.Name {
					t.Errorf("%s/%s/%d: %v", store, fault, replicas, diags.Errors())
				}
				if hasLB := strings.Contains(src, "kind: lb"); hasLB != (replicas > 1) {
					t.Errorf("%s/%s/%d: lb %v", store, fault, replicas, hasLB)
				}
			}
		}
	}
}

func TestInitFlagsAndScenario(t *testing.T) {
	t.Chdir(t.TempDir())
	out, errOut, code := runCLI(t, "init", "--name", "checkout", "--replicas", "3", "--store", "cache", "--rps", "800", "--fault", "cache-restart")
	if code != exitOK {
		t.Fatalf("init: %d %s", code, errOut)
	}
	raw, err := os.ReadFile("design.yaml")
	if err != nil || !strings.Contains(string(raw), "name: checkout") || !strings.Contains(string(raw), "inject: restart, target: cache") || !strings.Contains(out, "loadl run design.yaml") {
		t.Fatalf("design.yaml: %v\n%s\n%s", err, raw, out)
	}
	// It doesn't overwrite without --force.
	if _, errOut, code := runCLI(t, "init", "--blank"); code == exitOK || !strings.Contains(errOut, "exists") {
		t.Errorf("second init: %d %s", code, errOut)
	}
	if _, errOut, code := runCLI(t, "init", "--blank", "--force"); code != exitOK {
		t.Errorf("init --force: %d %s", code, errOut)
	}

	// A library scenario is copied into a folder of its own, where
	// predict and hint find it.
	if _, errOut, code := runCLI(t, "init", "slow-dependency"); code != exitOK {
		t.Fatalf("init slow-dependency: %d %s", code, errOut)
	}
	for _, f := range []string{"design.yaml", "README.md", "hints.md", "meta.yaml", "solutions/fewer-retries/design.yaml"} {
		if _, err := os.Stat(filepath.Join("slow-dependency", f)); err != nil {
			t.Errorf("copied scenario: %v", err)
		}
	}
	if s, ok := scenario.ForDesign(filepath.Join("slow-dependency", "design.yaml")); !ok || s.Name != "slow-dependency" {
		t.Errorf("ForDesign on the copy: %v %v", s, ok)
	}
	if _, _, code := runCLI(t, "init", "no-such-scenario"); code == exitOK {
		t.Error("init of an unknown scenario succeeded")
	}
}

// The wizard keeps defaults on Enter and asks again after a bad answer.
func TestInitWizard(t *testing.T) {
	a := defaultInitAnswers()
	in := bufio.NewReader(strings.NewReader("orders\n\nlots\n1\ncache\n2000\n\ncache-restart\n"))
	var out strings.Builder
	if err := askInit(&out, in, &a); err != nil {
		t.Fatal(err)
	}
	if a.Name != "orders" || a.Replicas != 2 || a.CPU != 1 || a.Store != "cache" || a.RPS != 2000 || a.Minutes != 3 || a.Fault != "cache-restart" {
		t.Errorf("answers %+v", a)
	}
	if !strings.Contains(out.String(), "a number of cores") {
		t.Errorf("no retry after a bad answer:\n%s", out.String())
	}
}
