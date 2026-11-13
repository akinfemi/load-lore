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
	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/driver"
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
		"xychart-beta", "Pink spikes mark 0:30 latency +20 ms on connections into db for 10s; 0:40 break",
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

// Failed requests get a table of their causes under the errors chart;
// the pool run has no failures, so its report has none.
func TestReportErrorCauses(t *testing.T) {
	w := analysistest.Healthy()
	w.Failed = func(t float64) float64 {
		if t > 70 {
			return 0.2
		}
		return 0
	}
	w.FailCause = "too_many_conns"
	dir := analysistest.WriteRun(t, w, 1)
	if _, err := Write(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	md := string(b)
	for _, want := range []string{
		"| Service | Failed requests | Share of its failures | Cause | From calls to | First seen | What it means |",
		"| `too_many_conns` | db | 1:10 | db refused new connections (Postgres max_connections) |",
		"Of api's failed requests, 100% failed because db refused new connections",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q", want)
		}
	}
	pool, err := os.ReadFile("testdata/pool.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(pool), "| Service | Failed requests |") {
		t.Error("a run without failures shouldn't get a causes table")
	}
}

func TestText(t *testing.T) {
	dir := analysistest.WriteRun(t, poolWorld(), 1)
	a, err := Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	txt := Text(a, "runs/x/report.md", 80)
	for _, want := range []string{
		"fixture · run fixture-run", "\nBROKE. Broke 0:40 into the load",
		"  offered ", "  served  ", "  p99     ", "^ break at 0:40",
		"Diagnosis\n  1. Connection pool exhausted (api → db), from 0:40", "Try next: Add PgBouncer",
		"Predictions\n", "  held      error_rate < 1%",
		"Full report, with the diagram and charts: runs/x/report.md",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("text lacks %q", want)
		}
	}
	for i, l := range strings.Split(txt, "\n") {
		if n := len([]rune(l)); n > 80 && !strings.HasPrefix(l, "Full report") {
			t.Errorf("line %d is %d columns: %q", i, n, l)
		}
	}
	if strings.Contains(txt, "```") {
		t.Error("markdown in the text report")
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

// Two runs that broke at the same time lead with how they differed, and
// hosts that differ only by a VM's memory drift aren't called different.
// A run that stopped its load early is flagged next to one that ran on:
// its most served may be cut short.
func TestCompareFlagsEarlyStop(t *testing.T) {
	a, err := analysis.Analyze(analysistest.WriteRun(t, poolWorld(), 1))
	if err != nil {
		t.Fatal(err)
	}
	b, err := analysis.Analyze(analysistest.WriteRun(t, analysistest.Healthy(), 1))
	if err != nil {
		t.Fatal(err)
	}
	if md := Compare(a, b); strings.Contains(md, "stopped its load") {
		t.Error("flagged two full runs")
	}
	a.StoppedEarly = &analysis.EarlyStop{BreakAt: 40, ConfirmedAt: 50, At: 110}
	want := "A stopped its load at 1:50 after its break, so the runs cover different parts of the profile."
	if md := Compare(a, b); !strings.Contains(md, want) || !strings.Contains(md, "loadl run --full") {
		t.Errorf("comparison lacks %q", want)
	}
}

func TestCompareTieAndHosts(t *testing.T) {
	a, err := analysis.Analyze(analysistest.WriteRun(t, poolWorld(), 1))
	if err != nil {
		t.Fatal(err)
	}
	b, err := analysis.Analyze(analysistest.WriteRun(t, poolWorld(), 1))
	if err != nil {
		t.Fatal(err)
	}
	ha, hb := *a.Environment.Host, *a.Environment.Host
	ha.MemoryBytes, hb.MemoryBytes = 10403422208, 10403430400
	a.Environment.Host, b.Environment.Host = &ha, &hb
	md := Compare(a, b)
	if !strings.Contains(md, "**Same verdict, different results.** Both broke at 0:40 (450 rps offered), but B served at most") || !strings.Contains(md, "from the break on, B served") {
		t.Errorf("no tie lead:\n%s", md[:min(len(md), 800)])
	}
	if strings.Contains(md, "different hosts") {
		t.Error("memory drift called a different host")
	}
	hb.CPUs = 4
	if md := Compare(a, b); !strings.Contains(md, "The runs ran on different hosts: 8 CPUs and 4 (A, then B)") {
		t.Error("CPU difference not named")
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

func TestCompareSpreadAndSpeed(t *testing.T) {
	run := func(id string, speed int, at, peak float64, s *analysis.Series) *analysis.Analysis {
		p99 := 900.0
		return &analysis.Analysis{RunID: id, Name: "d", Scale: 1, Seed: 1, Series: s,
			Verdict:     analysis.Verdict{Kind: analysis.VerdictBroke, MaxRPS: &peak, Break: &analysis.Break{At: at, P99: &p99}},
			Environment: analysis.Environment{Host: &driver.Host{CPUs: 8, MemoryBytes: 8 << 30, Speed: &bench.Speed{Score: bench.Score{Index: speed}}}}}
	}
	single := strings.Join(caveats(run("a", 1012, 180, 2950, nil), run("b", 874, 250, 4085, nil)), "\n")
	for _, want := range []string{"Both are single runs", "speed index 1012 and 874 (A, then B), 14% apart, more than 10%"} {
		if !strings.Contains(single, want) {
			t.Errorf("caveats lack %q:\n%s", want, single)
		}
	}
	if strings.Contains(single, "different hosts") {
		t.Errorf("a speed difference isn't a different host:\n%s", single)
	}

	s := &analysis.Series{BreakAt: &analysis.Spread{N: 3, Min: 160, Median: 180, Max: 220}, PeakServed: &analysis.Spread{N: 3, Min: 2800, Median: 2950, Max: 3100}}
	got := strings.Join(caveats(run("a", 1000, 180, 2950, s), run("b", 980, 210, 4085, nil)), "\n")
	if !strings.Contains(got, "The break time differs by 0:30, within the 1:00 spread of run A's series (3 runs, 2:40 to 3:40)") {
		t.Errorf("break time within spread:\n%s", got)
	}
	if strings.Contains(got, "peak served differs") || strings.Contains(got, "single runs") || strings.Contains(got, "different speeds") {
		t.Errorf("unexpected caveat:\n%s", got)
	}
}

func TestSeriesSummary(t *testing.T) {
	at1, at2, peak := 180.0, 220.0, 2950.0
	s := &analysis.Series{ID: "s", Design: "d", Scale: 1, Seed: 7, Planned: 2,
		Runs: []analysis.SeriesRun{
			{RunID: "r1", Verdict: analysis.VerdictBroke, Speed: 1000, BreakAt: &at1, PeakServed: &peak},
			{RunID: "r2", Verdict: analysis.VerdictBroke, Speed: 960, BreakAt: &at2, PeakServed: &peak,
				Cooled: &driver.Cooldown{Target: 1000, Tolerance: 0.05, First: 900, Waited: 150, Reached: true}},
		},
		BreakAt:    &analysis.Spread{N: 2, Min: 180, Median: 200, Max: 220},
		PeakServed: &analysis.Spread{N: 2, Min: 2950, Median: 2950, Max: 2950},
		Warnings:   []string{"the runs disagree"},
	}
	md := Series(s)
	for _, want := range []string{
		"| 2. `r2` | 960 | waited 2m30s (900 to 960) | broke | 3:40 |",
		"- Break time: 3:00 to 3:40 (median 3:20), 0:40 (20% of the median) apart over 2 runs that broke",
		"- Peak served: 2,950 rps in all 2 runs that were conclusive",
		"- p99 at the break: no conclusive run gave one",
		"- The runs disagree",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("lacks %q:\n%s", want, md)
		}
	}
}

// The note under the predictions: a broken one is framed as the lesson,
// and a load prediction gets the machine caveat. Held lines about other
// metrics need neither.
func TestPredictionNote(t *testing.T) {
	held := analysis.ExpectResult{Text: "error_rate < 1%", Status: analysis.ExpectHeld, Metric: "error_rate"}
	if n := predictionNote([]analysis.ExpectResult{held, {Note: "db gives out first", Status: analysis.ExpectNote}}); n != "" {
		t.Errorf("held, no load: %q", n)
	}
	load := analysis.ExpectResult{Text: "break_load_rps within 25% of 900", Status: analysis.ExpectBroken, Metric: "break_load_rps"}
	n := predictionNote([]analysis.ExpectResult{held, load})
	if !strings.Contains(n, "isn't a wrong answer") || !strings.Contains(n, "depend on this machine") {
		t.Errorf("broken load: %q", n)
	}
}
