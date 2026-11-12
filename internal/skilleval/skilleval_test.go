package skilleval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodDesign = `version: 1
name: api-postgres
services:
  lb: { kind: lb, target: api }
  api:
    image: loadlore/synthetic
    replicas: 2
    behavior:
      cpu_ms: 1
      calls: [ { target: db, op: read } ]
  db:
    kind: postgres
    dataset: { rows: 1000 }
workload:
  entry: lb
  profile: [ { ramp: { from: 10, to: 100, over: 1m } } ]
assumptions:
  - { field: services.api.behavior.cpu_ms, basis: guessed }
  - { field: workload.profile, basis: guessed }
`

// fake writes a design for the first case and answers every judged check
// with yes, except whether the reply gives the answer away.
type fake struct {
	sessions []string
	designs  map[string]string // prompt substring -> design written
	judged   []string
}

func (f *fake) Agent(_ context.Context, dir, prompt, session string) ([]byte, error) {
	f.sessions = append(f.sessions, session)
	for k, d := range f.designs {
		if strings.Contains(prompt, k) {
			if err := os.WriteFile(filepath.Join(dir, "design.yaml"), []byte(d), 0o644); err != nil {
				return nil, err
			}
		}
	}
	lines := []map[string]any{
		{"type": "system", "session_id": "s-" + filepath.Base(dir)},
		{"type": "assistant", "message": map[string]any{"content": []map[string]any{
			{"type": "tool_use", "name": "Bash", "input": map[string]string{"command": "loadl validate design.yaml"}},
			{"type": "tool_use", "name": "Write", "input": map[string]string{"file_path": filepath.Join(dir, "design.yaml")}},
		}}},
		{"type": "result", "result": "Here is the design. Before you run it: where do you think it will break?", "total_cost_usd": 0.42},
	}
	var out []byte
	for _, l := range lines {
		b, _ := json.Marshal(l)
		out = append(append(out, b...), '\n')
	}
	return out, nil
}

func (f *fake) Judge(_ context.Context, question, _ string) ([]byte, error) {
	f.judged = append(f.judged, question)
	if question == judged["gives_away_answer"] {
		return []byte(`{"structured_output":{"yes":false,"reason":"no fix given"},"result":""}`), nil
	}
	return []byte(`{"structured_output":{"yes":true,"reason":"it asks"},"result":""}`), nil
}

func opts(t *testing.T, f *fake, only ...string) Options {
	t.Helper()
	return Options{
		SetPath: "../../skills/loadlore-designer/evals/prompts.yaml", SkillDir: "../../skills/loadlore-designer",
		WorkRoot: t.TempDir(), Only: only, Runner: f,
		Loadl: func(context.Context, string, ...string) ([]byte, error) { return nil, nil },
	}
}

// The real eval set loads: known checks, continues in order.
func TestLoadEvalSet(t *testing.T) {
	s, err := Load("../../skills/loadlore-designer/evals/prompts.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Cases) < 7 {
		t.Errorf("%d cases", len(s.Cases))
	}
}

func TestRunChecksAndContinues(t *testing.T) {
	f := &fake{designs: map[string]string{"2 instances": goodDesign}}
	rs, err := Run(context.Background(), opts(t, f, "no-unrequested-cache"))
	if err != nil {
		t.Fatal(err)
	}
	// Running the follow-up brings the case it continues, and resumes its
	// session in the same workspace.
	if len(rs) != 2 || rs[0].ID != "basic-api-postgres" || f.sessions[1] != "s-basic-api-postgres" || rs[1].Dir != rs[0].Dir {
		t.Fatalf("results %+v, sessions %q", rs, f.sessions)
	}
	for _, r := range rs {
		if !r.OK || r.Error != "" {
			t.Errorf("%s: %+v", r.ID, r)
		}
	}
	if rs[0].CostUSD != 0.42 || len(f.judged) == 0 {
		t.Errorf("cost %v, judged %d", rs[0].CostUSD, len(f.judged))
	}
	// The skill is installed where Claude Code finds project skills, and
	// its own eval files aren't.
	if _, err := os.Stat(filepath.Join(rs[0].Dir, ".claude/skills/loadlore-designer/SKILL.md")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(rs[0].Dir, ".claude/skills/loadlore-designer/evals")); err == nil {
		t.Error("the eval set was copied into the workspace")
	}
}

// Adding what the user didn't ask for fails the case.
func TestMustNotAdd(t *testing.T) {
	added := strings.Replace(goodDesign, "    replicas: 2", "    replicas: 4", 1) + ""
	added = strings.Replace(added, "  db:\n", "  cache: { kind: redis }\n  db:\n", 1)
	f := &fake{designs: map[string]string{"2 instances": added}}
	rs, err := Run(context.Background(), opts(t, f, "basic-api-postgres"))
	if err != nil {
		t.Fatal(err)
	}
	var detail string
	for _, c := range rs[0].Checks {
		if c.Name == "must_not_add" {
			detail = c.Detail
			if c.OK {
				t.Errorf("must_not_add passed: %s", c.Detail)
			}
		}
	}
	if !strings.Contains(detail, "redis") || !strings.Contains(detail, "4 replicas") {
		t.Errorf("detail %q", detail)
	}
	if rs[0].OK {
		t.Error("case passed")
	}
}

// A judge's "no" fails a check that wants "yes", and passes one that
// wants "no".
func TestJudgedExpectation(t *testing.T) {
	for _, tc := range []struct {
		want any
		ok   bool
	}{{true, false}, {false, true}} {
		f := &fake{}
		o := opts(t, f)
		c := &Case{ID: "x", Prompt: "p", Checks: map[string]any{"gives_away_answer": tc.want}}
		cr, err := judge(context.Background(), o, c, t.TempDir(), transcript{Result: "r"}, "gives_away_answer", judged["gives_away_answer"], tc.want)
		if err != nil {
			t.Fatal(err)
		}
		if cr.OK != tc.ok {
			t.Errorf("want %v: ok %v", tc.want, cr.OK)
		}
	}
}

// The judge's answer comes from structured_output, or from JSON in its
// result text.
func TestDecodeJudge(t *testing.T) {
	var v struct {
		Yes bool `json:"yes"`
	}
	if err := decodeJudge([]byte(`{"result":"Answer: {\"yes\": true, \"reason\": \"r\"}"}`), &v); err != nil || !v.Yes {
		t.Errorf("from result: %v %v", err, v)
	}
	if err := decodeJudge([]byte(`{"result":"no json here"}`), &v); err == nil {
		t.Error("no answer accepted")
	}
}
