package report

import (
	"flag"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/analysis/analysistest"
)

var update = flag.Bool("update", false, "rewrite golden files")

// poolWorld fills the pool from 40 s while the database idles, and kills
// nothing but injects a latency fault at 30 s.
func poolWorld() analysistest.World {
	w := analysistest.Healthy()
	w.InUse = func(t float64) float64 { return math.Min(10, 2+t/5) }
	w.Pending = func(t float64) float64 { return math.Max(0, (t-40)*3) }
	w.WaitS = func(t float64) float64 { return math.Max(0.0001, (t-40)/200) }
	w.P99 = func(t float64) float64 { return math.Max(0.005, (t-40)/50) }
	w.Events = []analysistest.Event{
		{At: 30, Kind: "fault_start", Fields: map[string]any{"inject": "latency", "target": "db", "detail": "latency +20 ms on connections into db for 10s"}},
		{At: 40, Kind: "fault_end", Fields: map[string]any{"inject": "latency", "target": "db", "detail": "latency +20 ms on connections into db for 10s"}},
	}
	return w
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs; run go test -update after checking the change", path)
	}
}

func TestReport(t *testing.T) {
	dir := analysistest.WriteRun(t, poolWorld(), 1)
	a, err := Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	md := string(b)
	golden(t, "pool.md", md)
	if _, err := os.Stat(filepath.Join(dir, AnalysisFile)); err != nil {
		t.Error("no analysis.json")
	}
	for _, want := range []string{
		"## Verdict", "**Broke.**", "## Diagram at 0:40", "```mermaid\nflowchart LR", "class api hot", "class db ok",
		"xychart-beta", "Shaded bars mark 0:30 latency +20 ms on connections into db for 10s; 0:40 break",
		"### 1. Connection pool exhausted (api → db), from 0:40", "**Try next:** Add PgBouncer",
		"| `error_rate < 1%` | **held** |", "## Thresholds", "## Per-window data", "## Design file",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q", want)
		}
	}
	if a.Verdict.Kind != analysis.VerdictBroke {
		t.Errorf("verdict %s", a.Verdict.Kind)
	}
	// Every mermaid block is closed.
	if strings.Count(md, "```mermaid") == 0 || strings.Count(md, "```")%2 != 0 {
		t.Error("unbalanced code fences")
	}
}

func TestCompare(t *testing.T) {
	a, err := analysis.Analyze(analysistest.WriteRun(t, analysistest.Healthy(), 1))
	if err != nil {
		t.Fatal(err)
	}
	b, err := analysis.Analyze(analysistest.WriteRun(t, poolWorld(), 1))
	if err != nil {
		t.Fatal(err)
	}
	md := Compare(a, b)
	golden(t, "compare.md", md)
	for _, want := range []string{"| Verdict | held | broke at 0:40 (450 rps offered) |", "Both runs used the same design file.", "### B", "Connection pool exhausted"} {
		if !strings.Contains(md, want) {
			t.Errorf("comparison lacks %q", want)
		}
	}
	b.DesignYAML = strings.Replace(b.DesignYAML, "pools: { db: 10 }", "pools: { db: 40 }", 1)
	if md := Compare(a, b); !strings.Contains(md, "-    pools: { db: 10 }\n+    pools: { db: 40 }") {
		_, diff, _ := strings.Cut(md, "## Design changes")
		t.Errorf("design diff missing:\n%s", diff)
	}
}

func TestUnifiedDiff(t *testing.T) {
	x := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n"
	y := "a\nb\nC\nd\ne\nf\ng\nh\ni\nj\nk\n"
	got := unifiedDiff(x, y, "x", "y")
	want := "--- x\n+++ y\n@@ -1,6 +1,6 @@\n a\n b\n-c\n+C\n d\n e\n f\n@@ -8,3 +8,4 @@\n h\n i\n j\n+k\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if unifiedDiff(x, x, "x", "y") != "" {
		t.Error("diff of equal texts")
	}
}

func TestNiceCeil(t *testing.T) {
	for in, want := range map[float64]float64{0: 1, 0.7: 0.8, 3: 3, 12: 12, 13: 15, 4100: 5000, 5250: 6000, 100: 100} {
		if got := niceCeil(in); got != want {
			t.Errorf("niceCeil(%v) = %v, want %v", in, got, want)
		}
	}
}
