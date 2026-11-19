package diagram

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/design"
)

var update = flag.Bool("update", false, "rewrite golden files")

var goldenDesigns = map[string]string{
	"single-postgres": "../../examples/single-postgres.yaml",
	"cache-aside":     "testdata/cache-aside.yaml",
	"services":        "testdata/services.yaml",
}

func TestGoldenDiagrams(t *testing.T) {
	for name, path := range goldenDesigns {
		t.Run(name, func(t *testing.T) {
			d, diags, err := design.LoadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if diags.HasErrors() {
				t.Fatalf("design has errors: %v", diags)
			}
			f := Flowchart(d)
			art, err := ASCII(f)
			if err != nil {
				t.Fatal(err)
			}
			golden(t, filepath.Join("testdata", "golden", name+".mmd"), f.String())
			golden(t, filepath.Join("testdata", "golden", name+".txt"), art)
		})
	}
}

func golden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs (run go test -update after checking the change):\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}

func TestASCIIHasNoNonBreakingSpaces(t *testing.T) {
	d, _, err := design.LoadFile("testdata/cache-aside.yaml")
	if err != nil {
		t.Fatal(err)
	}
	art, err := ASCII(Flowchart(d))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(art, " ") {
		t.Error("ASCII output contains non-breaking spaces")
	}
}

func TestRPSFormatting(t *testing.T) {
	for in, want := range map[float64]string{5000: "5,000 rps", 100: "100 rps", 1234567: "1,234,567 rps", 2.5: "2.5 rps", 0: "0 rps"} {
		if got := rps(in); got != want {
			t.Errorf("rps(%v) = %q, want %q", in, got, want)
		}
	}
}

// A postgres's disk block shows on its node, defaults filled in.
func TestDiskLabel(t *testing.T) {
	iops := 10000
	if got := diskLabel(&design.Disk{}); got != "disk 1 ms · 3,000 IOPS" {
		t.Errorf("defaults: %q", got)
	}
	if got := diskLabel(&design.Disk{IOPS: &iops}); got != "disk 1 ms · 10,000 IOPS" {
		t.Errorf("iops set: %q", got)
	}
}
