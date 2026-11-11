package predict

import (
	"os"
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/scenario"
)

var (
	load = scenario.Question{Ask: "When does p99 pass 500 ms?", Answer: scenario.AnswerRPS,
		Expect: "p99_ms < 500 until load_rps >= {answer}", Never: "p99_ms < 500 throughout"}
	first = scenario.Question{Ask: "Which component gives out first?", Answer: scenario.AnswerService,
		Expect: `note: "{answer} gives out first"`}
	open = scenario.Question{Ask: "What would bigger pools do?", Answer: scenario.AnswerText,
		Expect: `note: "bigger pools: {answer}"`}
)

func TestLine(t *testing.T) {
	services := []string{"lb", "api", "db"}
	for _, tc := range []struct {
		q       scenario.Question
		answer  string
		want    string
		wantErr string
	}{
		{load, "1000", "p99_ms < 500 until load_rps >= 1000", ""},
		{load, " 1,200 rps", "p99_ms < 500 until load_rps >= 1200", ""},
		{load, "1.5k", "p99_ms < 500 until load_rps >= 1500", ""},
		{load, "Never", "p99_ms < 500 throughout", ""},
		{load, "soon", "", "isn't a load"},
		{load, "", "", "an answer is needed"},
		{first, "DB", `note: "db gives out first"`, ""},
		{first, "cache", "", "isn't one of the design's services: lb, api, db"},
		{open, `they'd "hit"   max_connections`, `note: "bigger pools: they'd 'hit' max_connections"`, ""},
	} {
		got, err := Line(tc.q, tc.answer, services)
		switch {
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("Line(%q) error %v, want %q", tc.answer, err, tc.wantErr)
		case tc.wantErr == "" && (err != nil || got != tc.want):
			t.Errorf("Line(%q) = %q, %v; want %q", tc.answer, got, err, tc.want)
		}
	}
}

func TestRewriteKeepsTheRest(t *testing.T) {
	src, err := os.ReadFile("../../scenarios/connection-pool-trap/design.yaml")
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{"p99_ms < 500 until load_rps >= 600", `note: "db gives out first"`}
	out, err := Rewrite(src, lines)
	if err != nil {
		t.Fatal(err)
	}
	before, _, _ := strings.Cut(string(src), "\nexpect:\n")
	gotBefore, block, _ := strings.Cut(string(out), "\nexpect:\n")
	if before != gotBefore {
		t.Error("the design before the expect block changed")
	}
	if want := "  - p99_ms < 500 until load_rps >= 600\n  - note: \"db gives out first\"\n"; block != want {
		t.Errorf("expect block %q, want %q", block, want)
	}

	// A block in the middle, with a blank line and a comment in it: what
	// follows it stays.
	const mid = `version: 1
name: mid
services:
  api: { image: loadlore/synthetic, behavior: { calls: [ { target: db, op: read } ] } }
  db: { kind: postgres, dataset: { rows: 100 } }
workload:
  entry: api
  profile: [ { hold: { rps: 10, for: 1m } } ]
expect:
  - error_rate < 1%

  # a comment in the block
  - rps >= 5

# the faults
scenario:
  - { at: 0s, inject: latency, target: db, add_ms: 5 }
`
	out, err = Rewrite([]byte(mid), []string{"rps >= 8"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "expect:\n  - rps >= 8\n\n# the faults\nscenario:\n  - { at: 0s, inject: latency, target: db, add_ms: 5 }\n"; !strings.HasSuffix(string(out), want) {
		t.Errorf("got\n%s", out)
	}
}

func TestRewriteAddsABlock(t *testing.T) {
	src, err := os.ReadFile("../../scenarios/connection-pool-trap/design.yaml")
	if err != nil {
		t.Fatal(err)
	}
	before, _, _ := strings.Cut(string(src), "# Your predictions")
	out, err := Rewrite([]byte(before), []string{"error_rate < 1% throughout"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(out), "\n# Your predictions, scored in the report.\nexpect:\n  - error_rate < 1% throughout\n") {
		t.Errorf("tail %q", string(out)[len(out)-120:])
	}
	if _, err := Rewrite([]byte(before), []string{"p99_ms <<< 3"}); err == nil {
		t.Error("an invalid line was written")
	}
}

func TestCheckQuestion(t *testing.T) {
	for _, q := range []scenario.Question{load, first, open} {
		if p := q.Problem(); p != "" {
			t.Errorf("%q: %s", q.Ask, p)
		}
	}
	bad := load
	bad.Never = ""
	if p := bad.Problem(); !strings.Contains(p, "never is required") {
		t.Errorf("got %q", p)
	}
	bad = load
	bad.Expect = "p99_ms << {answer}"
	if p := bad.Problem(); !strings.Contains(p, "isn't a valid expect line") {
		t.Errorf("got %q", p)
	}
}
