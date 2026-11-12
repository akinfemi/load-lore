// Package skilleval runs the loadlore-designer skill's eval set: each case
// is a user prompt given to Claude Code, headless, in a scratch workspace
// with the skill installed, followed by checks on what it did. Checks that
// code can decide (does the design validate, which services it has, which
// files exist, which commands ran) are decided here; the rest are asked of
// a judge model with a fixed question and a yes-or-no schema.
//
// Every case and every judged check is a paid model call. Each call has a
// budget cap, and the CI workflow runs only when an API key secret is set.
package skilleval

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/akinfemi/load-lore/internal/design"
)

// Set is the eval set file.
type Set struct {
	Cases []*Case `yaml:"cases"`
}

// Case is one prompt and its checks.
type Case struct {
	ID      string `yaml:"id"`
	Prompt  string `yaml:"prompt"`
	Fixture string `yaml:"fixture,omitempty"` // relative to the eval set's folder
	// Continues names an earlier case whose session and workspace this one
	// continues, for follow-up prompts such as "same as before but".
	Continues string `yaml:"continues,omitempty"`
	// UserReplicas is how many replicas the prompt asked for, for the
	// replicas_beyond_user check.
	UserReplicas int            `yaml:"user_replicas,omitempty"`
	Checks       map[string]any `yaml:"checks"`
}

// Load reads an eval set.
func Load(path string) (*Set, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Set
	if err := yaml.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	seen := map[string]bool{}
	for _, c := range s.Cases {
		if c.ID == "" || c.Prompt == "" {
			return nil, fmt.Errorf("%s: every case needs an id and a prompt", path)
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("%s: case %s twice", path, c.ID)
		}
		if c.Continues != "" && !seen[c.Continues] {
			return nil, fmt.Errorf("%s: case %s continues %s, which must come before it", path, c.ID, c.Continues)
		}
		seen[c.ID] = true
		for k := range c.Checks {
			if _, ok := judged[k]; !ok && !slices.Contains(decided, k) {
				return nil, fmt.Errorf("%s: case %s: unknown check %q", path, c.ID, k)
			}
		}
	}
	return &s, nil
}

// decided are the checks code decides.
var decided = []string{"validates", "services_include", "must_not_add", "field_absent", "files_present",
	"assumptions_present_for", "basis_for_calls", "scenario_check_passes", "runs_cloud_without_budget", "edits_design_without_choice"}

// judged are the checks the judge decides: the question it answers yes or
// no about the agent's reply and files. The case's expected value says
// which answer passes.
var judged = map[string]string{
	"asks_for_prediction":        "Does the reply ask the user to predict what will happen (for example where or when the system will break) before running it?",
	"gives_away_answer":          "Does the reply tell the user how to fix the design, or what will break it, before they have run it and predicted?",
	"explains_emergent_hit_rate": "Does the reply explain that a cache's hit rate isn't something you set: it comes out of the run, from the keys' distribution and the cache's size and TTL?",
	"offers_options":             "Does the reply offer the user a choice between distinct options, rather than one answer? If a range is given, is the number of options within it?",
	"includes_tradeoffs":         "Does the reply give a trade-off (a cost or downside) for each option it offers?",
	"detects":                    "Does the reply point out the named problem in the user's code?",
	"fixes_n_plus_one":           "Does the design the agent wrote fix the N+1 query pattern (batching the per-item queries), rather than modelling the code as it is?",
	"asks_for_budget":            "Does the reply ask the user for a spending budget, or for explicit consent to spend money, before running anything in the cloud?",
	"readme_contains_answer":     "Does the scenario's README.md reveal the answer: the fix, or exactly why the design breaks?",
	"hints_progressive":          "Do the scenario's hints in hints.md go from a nudge to the answer, each revealing more than the one before?",
}

// Runner runs Claude Code headless. Tests replace it.
type Runner interface {
	// Agent runs prompt in dir, continuing session if it isn't empty, and
	// returns the stream-json transcript.
	Agent(ctx context.Context, dir, prompt, session string) ([]byte, error)
	// Judge answers question about evidence, returning the judge's JSON.
	Judge(ctx context.Context, question, evidence string) ([]byte, error)
}

// Options configure a run.
type Options struct {
	SetPath  string // prompts.yaml
	SkillDir string // the skill folder to install
	WorkRoot string // where each case's workspace goes
	Only     []string
	Runner   Runner
	// Validate runs `loadl validate --json` and `loadl scenario check
	// --static` style checks; tests replace it.
	Loadl func(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// Result is one case's outcome.
type Result struct {
	ID      string        `json:"id"`
	OK      bool          `json:"ok"`
	Error   string        `json:"error,omitempty"`
	CostUSD float64       `json:"cost_usd"`
	Checks  []CheckResult `json:"checks"`
	Reply   string        `json:"reply"`
	Dir     string        `json:"workspace"`
}

// CheckResult is one check.
type CheckResult struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Judged bool   `json:"judged"`
	Detail string `json:"detail"`
}

// Run runs the eval set and returns each case's result.
func Run(ctx context.Context, o Options) ([]*Result, error) {
	set, err := Load(o.SetPath)
	if err != nil {
		return nil, err
	}
	sessions, dirs := map[string]string{}, map[string]string{}
	var out []*Result
	for _, c := range set.Cases {
		if len(o.Only) > 0 && !slices.Contains(o.Only, c.ID) && !continuedBy(set, c.ID, o.Only) {
			continue
		}
		r := &Result{ID: c.ID}
		out = append(out, r)
		dir, session := dirs[c.Continues], sessions[c.Continues]
		if c.Continues == "" {
			if dir, err = workspace(o, c, filepath.Dir(o.SetPath)); err != nil {
				r.Error = err.Error()
				continue
			}
		}
		r.Dir = dir
		raw, err := o.Runner.Agent(ctx, dir, c.Prompt, session)
		t := parseTranscript(raw)
		r.Reply, r.CostUSD = t.Result, t.CostUSD
		if err != nil {
			r.Error = err.Error()
			continue
		}
		sessions[c.ID], dirs[c.ID] = t.Session, dir
		r.Checks, err = check(ctx, o, c, dir, t)
		if err != nil {
			r.Error = err.Error()
			continue
		}
		r.OK = !slices.ContainsFunc(r.Checks, func(x CheckResult) bool { return !x.OK })
	}
	return out, nil
}

func continuedBy(set *Set, id string, only []string) bool {
	for _, c := range set.Cases {
		if c.Continues == id && slices.Contains(only, c.ID) {
			return true
		}
	}
	return false
}

// workspace makes a scratch folder with the skill installed where Claude
// Code finds project skills, and the case's fixture copied in.
func workspace(o Options, c *Case, setDir string) (string, error) {
	dir := filepath.Join(o.WorkRoot, c.ID)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := copyTree(o.SkillDir, filepath.Join(dir, ".claude", "skills", filepath.Base(o.SkillDir)), "evals"); err != nil {
		return "", err
	}
	if c.Fixture != "" {
		if err := copyTree(filepath.Join(setDir, c.Fixture), dir, ""); err != nil {
			return "", fmt.Errorf("fixture %s: %w", c.Fixture, err)
		}
	}
	return dir, nil
}

// copyTree copies src into dst, leaving out a top-level folder named skip.
func copyTree(src, dst, skip string) error {
	return filepath.WalkDir(src, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if skip != "" && (rel == skip || strings.HasPrefix(rel, skip+string(filepath.Separator))) {
			return filepath.SkipDir
		}
		out := filepath.Join(dst, rel)
		if e.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
}

// transcript is what the stream-json output says.
type transcript struct {
	Session  string
	Result   string
	CostUSD  float64
	Commands []string // Bash commands the agent ran
	Edited   []string // files it wrote or edited
}

func parseTranscript(raw []byte) transcript {
	var t transcript
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var line struct {
			Type      string  `json:"type"`
			SessionID string  `json:"session_id"`
			Result    string  `json:"result"`
			Cost      float64 `json:"total_cost_usd"`
			Message   struct {
				Content []struct {
					Type  string          `json:"type"`
					Name  string          `json:"name"`
					Input json.RawMessage `json:"input"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		if line.SessionID != "" {
			t.Session = line.SessionID
		}
		switch line.Type {
		case "result":
			t.Result, t.CostUSD = line.Result, line.Cost
		case "assistant":
			for _, b := range line.Message.Content {
				if b.Type != "tool_use" {
					continue
				}
				var in struct {
					Command  string `json:"command"`
					FilePath string `json:"file_path"`
				}
				_ = json.Unmarshal(b.Input, &in)
				switch b.Name {
				case "Bash":
					t.Commands = append(t.Commands, in.Command)
				case "Write", "Edit", "MultiEdit":
					t.Edited = append(t.Edited, in.FilePath)
				}
			}
		}
	}
	return t
}

// designFiles are the workspace's design files: YAML with version: 1 and
// services, outside the installed skill.
func designFiles(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if e.IsDir() && e.Name() == ".claude" {
			return filepath.SkipDir
		}
		if e.IsDir() || !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".yml") {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err == nil && bytes.Contains(raw, []byte("version: 1")) && bytes.Contains(raw, []byte("services:")) {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// mainDesign is the design to check: design.yaml at the top if there is
// one, else the first design file found.
func mainDesign(dir string) string {
	all := designFiles(dir)
	for _, p := range all {
		if filepath.Base(p) == "design.yaml" && filepath.Dir(p) == dir {
			return p
		}
	}
	if len(all) > 0 {
		return all[0]
	}
	return ""
}

func check(ctx context.Context, o Options, c *Case, dir string, t transcript) ([]CheckResult, error) {
	var out []CheckResult
	path := mainDesign(dir)
	var d *design.Design
	if path != "" {
		raw, _ := os.ReadFile(path)
		d, _ = design.Load(raw)
	}
	names := sortedKeys(c.Checks)
	for _, name := range names {
		want := c.Checks[name]
		if q, ok := judged[name]; ok {
			cr, err := judge(ctx, o, c, dir, t, name, q, want)
			if err != nil {
				return out, err
			}
			out = append(out, cr)
			continue
		}
		out = append(out, decide(ctx, o, c, dir, path, d, t, name, want))
	}
	return out, nil
}

func decide(ctx context.Context, o Options, c *Case, dir, path string, d *design.Design, t transcript, name string, want any) CheckResult {
	cr := CheckResult{Name: name}
	noDesign := func() CheckResult {
		cr.Detail = "no design file in the workspace"
		return cr
	}
	switch name {
	case "validates":
		if path == "" {
			return noDesign()
		}
		_, err := o.Loadl(ctx, dir, "validate", "--json", path)
		cr.OK = (err == nil) == truthy(want)
		cr.Detail = fmt.Sprintf("loadl validate %s: %v", rel(dir, path), errText(err))
	case "services_include":
		if d == nil {
			return noDesign()
		}
		var missing []string
		for _, s := range stringList(want) {
			if _, ok := d.Services[s]; !ok {
				missing = append(missing, s)
			}
		}
		cr.OK = len(missing) == 0
		cr.Detail = "missing: " + strings.Join(missing, ", ")
		if cr.OK {
			cr.Detail = "all present"
		}
	case "must_not_add":
		if d == nil {
			return noDesign()
		}
		var added []string
		for _, what := range stringList(want) {
			if s := addedWhat(d, what, c.UserReplicas); s != "" {
				added = append(added, s)
			}
		}
		cr.OK = len(added) == 0
		cr.Detail = strings.Join(added, "; ")
		if cr.OK {
			cr.Detail = "nothing unrequested"
		}
	case "field_absent":
		if path == "" {
			return noDesign()
		}
		raw, _ := os.ReadFile(path)
		field := fmt.Sprint(want)
		cr.OK = !bytes.Contains(raw, []byte(field+":"))
		cr.Detail = fmt.Sprintf("%s in %s: %v", field, rel(dir, path), !cr.OK)
	case "files_present":
		root := scenarioRoot(dir)
		var missing []string
		for _, f := range stringList(want) {
			if _, err := os.Stat(filepath.Join(root, f)); err != nil {
				missing = append(missing, f)
			}
		}
		cr.OK = len(missing) == 0
		cr.Detail = fmt.Sprintf("in %s, missing: %s", rel(dir, root), strings.Join(missing, ", "))
	case "assumptions_present_for":
		if d == nil {
			return noDesign()
		}
		var missing []string
		for _, f := range stringList(want) {
			if !slices.ContainsFunc(d.Assumptions, func(a *design.Assumption) bool { return a != nil && strings.HasPrefix(a.Field, f) }) {
				missing = append(missing, f)
			}
		}
		cr.OK = len(missing) == 0
		cr.Detail = "no assumption for: " + strings.Join(missing, ", ")
		if cr.OK {
			cr.Detail = "all recorded"
		}
	case "basis_for_calls":
		if d == nil {
			return noDesign()
		}
		cr.OK = slices.ContainsFunc(d.Assumptions, func(a *design.Assumption) bool {
			return a != nil && strings.Contains(a.Field, "calls") && a.Basis == fmt.Sprint(want)
		})
		cr.Detail = fmt.Sprintf("an assumption about calls with basis %v: %v", want, cr.OK)
	case "scenario_check_passes":
		root := scenarioRoot(dir)
		// Static only: the full check runs every design on Docker for
		// many minutes, which a CI eval can't afford.
		_, err := o.Loadl(ctx, dir, "scenario", "check", "--static", root)
		cr.OK = (err == nil) == truthy(want)
		cr.Detail = fmt.Sprintf("loadl scenario check --static %s: %v", rel(dir, root), errText(err))
	case "runs_cloud_without_budget":
		ran := slices.ContainsFunc(t.Commands, func(cmd string) bool {
			return strings.Contains(cmd, "loadl run") && strings.Contains(cmd, "cloud")
		})
		cr.OK = ran == truthy(want)
		cr.Detail = fmt.Sprintf("ran a cloud run: %v", ran)
	case "edits_design_without_choice":
		edited := slices.ContainsFunc(t.Edited, func(p string) bool { return strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml") })
		cr.OK = edited == truthy(want)
		cr.Detail = fmt.Sprintf("edited a design file: %v", edited)
	}
	return cr
}

// addedWhat says how the design adds one of the things a case says not
// to, or "" if it doesn't.
func addedWhat(d *design.Design, what string, userReplicas int) string {
	for name, s := range d.Services {
		if s == nil {
			continue
		}
		switch what {
		case "cache":
			if s.Kind == "redis" {
				return "a redis service, " + name
			}
		case "pgbouncer":
			if s.Kind == "pgbouncer" {
				return "a pgbouncer, " + name
			}
		case "queue":
			if s.Kind == "kafka" || s.Kind == "rabbitmq" {
				return "a queue, " + name
			}
		case "resilience":
			if b := s.Behavior; b != nil && b.Resilience != nil {
				r := b.Resilience
				if r.Retries != nil || r.CircuitBreaker != nil || r.Bulkhead != nil {
					return "retries, a breaker or a bulkhead on " + name
				}
			}
		case "replicas_beyond_user":
			if userReplicas > 0 && s.Replicas != nil && *s.Replicas > userReplicas {
				return fmt.Sprintf("%d replicas of %s, against the user's %d", *s.Replicas, name, userReplicas)
			}
		}
	}
	return ""
}

// scenarioRoot is the folder with a meta.yaml, or the workspace.
func scenarioRoot(dir string) string {
	found := dir
	_ = filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if e.IsDir() && e.Name() == ".claude" {
			return filepath.SkipDir
		}
		if !e.IsDir() && e.Name() == "meta.yaml" {
			found = filepath.Dir(p)
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// judge asks the judge question about the agent's reply and files.
func judge(ctx context.Context, o Options, c *Case, dir string, t transcript, name, q string, want any) (CheckResult, error) {
	cr := CheckResult{Name: name, Judged: true}
	var ev strings.Builder
	fmt.Fprintf(&ev, "The user asked an AI agent, which uses a skill for writing Loadlore system-design files:\n\n<prompt>\n%s\n</prompt>\n\n", c.Prompt)
	fmt.Fprintf(&ev, "The agent's final reply:\n\n<reply>\n%s\n</reply>\n\n", t.Result)
	files := designFiles(dir)
	if root := scenarioRoot(dir); root != dir {
		for _, f := range []string{"README.md", "hints.md"} {
			files = append(files, filepath.Join(root, f))
		}
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		fmt.Fprintf(&ev, "<file path=%q>\n%s\n</file>\n\n", rel(dir, f), raw)
	}
	question := q
	if m, ok := want.(map[string]any); ok {
		question += fmt.Sprintf(" (Range: %v to %v options.)", m["min"], m["max"])
	} else if name == "detects" {
		question += fmt.Sprintf(" The named problem: %v.", want)
	}
	raw, err := o.Runner.Judge(ctx, question, ev.String())
	if err != nil {
		return cr, fmt.Errorf("judging %s: %w", name, err)
	}
	var v struct {
		Yes    bool   `json:"yes"`
		Reason string `json:"reason"`
	}
	if err := decodeJudge(raw, &v); err != nil {
		return cr, fmt.Errorf("judging %s: %w", name, err)
	}
	expect := true
	if b, ok := want.(bool); ok {
		expect = b
	}
	cr.OK = v.Yes == expect
	cr.Detail = fmt.Sprintf("judge: %v (want %v): %s", yesNo(v.Yes), yesNo(expect), v.Reason)
	return cr, nil
}

// decodeJudge reads the judge's answer from Claude Code's JSON output:
// its structured_output when present, else its result text.
func decodeJudge(raw []byte, v any) error {
	var out struct {
		Structured json.RawMessage `json:"structured_output"`
		Result     string          `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	if len(out.Structured) > 0 && string(out.Structured) != "null" {
		return json.Unmarshal(out.Structured, v)
	}
	s := strings.TrimSpace(out.Result)
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		s = s[i : j+1]
	}
	if err := json.Unmarshal([]byte(s), v); err != nil {
		return fmt.Errorf("no answer in the judge's output: %w", err)
	}
	return nil
}

// JudgeSchema is the judge's answer.
const JudgeSchema = `{"type":"object","properties":{"yes":{"type":"boolean"},"reason":{"type":"string"}},"required":["yes","reason"],"additionalProperties":false}`

func truthy(v any) bool {
	b, ok := v.(bool)
	return !ok || b
}

func stringList(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, fmt.Sprint(e))
		}
		return out
	case nil:
		return nil
	}
	return []string{fmt.Sprint(v)}
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func rel(dir, p string) string {
	if r, err := filepath.Rel(dir, p); err == nil {
		return r
	}
	return p
}

func errText(err error) string {
	if err == nil {
		return "ok"
	}
	var ee interface{ ExitCode() int }
	if errors.As(err, &ee) {
		return fmt.Sprintf("exit %d", ee.ExitCode())
	}
	return err.Error()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
