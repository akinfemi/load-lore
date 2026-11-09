package load

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/plan"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestGoldenScript(t *testing.T) {
	d, diags, err := design.LoadFile("../../examples/smoke.yaml")
	if err != nil || diags.HasErrors() {
		t.Fatal(err, diags)
	}
	p, err := plan.Build(d, plan.Options{RunID: "test-run", Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Script(p)
	if err != nil {
		t.Fatal(err)
	}
	const path = "testdata/smoke.js"
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update)", err)
	}
	if string(got) != string(want) {
		t.Errorf("%s differs; run go test -update after checking the change", path)
	}
	for _, must := range []string{"ramping-arrival-rate", `"seed": 7`, `"distribution": "zipf"`, "handleSummary", "systemTags", "name: 'read'"} {
		if !strings.Contains(string(got), must) {
			t.Errorf("script lacks %s", must)
		}
	}
	if strings.Contains(string(got), "randomSeed") || strings.Contains(string(got), "Math.random()") {
		t.Error("script uses k6's randomSeed or Math.random directly; seeded runs must use rand()")
	}
	if strings.Contains(string(got), "constant-vus") || strings.Contains(string(got), "ramping-vus") {
		t.Error("closed-model executor in the script")
	}
}

func TestScatterIsCoprime(t *testing.T) {
	for _, n := range []int64{1, 2, 1000, 1000003, 5_000_000, 1000003 * 1000033} {
		if m := scatterFor(n); gcd(m, n) != 1 {
			t.Errorf("scatterFor(%d) = %d shares a factor", n, m)
		}
	}
}
