package skilleval

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// AgentTools are what the agent under test may use: files, and loadl's
// commands that don't start containers. A run takes minutes and Docker,
// and none of the checks needs one.
var AgentTools = []string{
	"Read", "Write", "Edit", "Glob", "Grep",
	"Bash(loadl validate *)", "Bash(loadl diagram *)", "Bash(loadl schema *)", "Bash(loadl scenarios *)",
	"Bash(loadl scenario check --static *)", "Bash(loadl predict *)", "Bash(loadl hint *)", "Bash(ls *)", "Bash(cat *)",
}

// Claude runs Claude Code from PATH.
type Claude struct {
	AgentModel string  // empty: Claude Code's default
	JudgeModel string  // the judge's model
	AgentUSD   float64 // budget per case
	JudgeUSD   float64 // budget per judged check
}

// Agent runs one prompt headless in dir.
func (c Claude) Agent(ctx context.Context, dir, prompt, session string) ([]byte, error) {
	args := []string{"-p", prompt, "--output-format", "stream-json", "--verbose",
		"--allowedTools", strings.Join(AgentTools, ","),
		"--max-budget-usd", fmt.Sprint(c.AgentUSD)}
	if c.AgentModel != "" {
		args = append(args, "--model", c.AgentModel)
	}
	if session != "" {
		args = append(args, "--resume", session)
	}
	return c.run(ctx, dir, args)
}

// Judge answers a yes-or-no question about the evidence, as JSON.
func (c Claude) Judge(ctx context.Context, question, evidence string) ([]byte, error) {
	prompt := evidence + "\n\nAnswer this question about the agent's reply and files, strictly from what is above: " + question +
		"\nGive yes or no, and one sentence saying what in the reply or files decided it."
	args := []string{"-p", prompt, "--bare", "--output-format", "json", "--json-schema", JudgeSchema,
		"--model", c.JudgeModel, "--max-budget-usd", fmt.Sprint(c.JudgeUSD), "--disallowedTools", "Bash,Write,Edit"}
	dir, err := os.MkdirTemp("", "skilleval-judge-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	return c.run(ctx, dir, args)
}

func (c Claude) run(ctx context.Context, dir string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("claude: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// LoadlOnPath runs loadl from PATH in dir.
func LoadlOnPath(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "loadl", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// Markdown is a report of the results.
func Markdown(rs []*Result) string {
	var b strings.Builder
	pass, cost := 0, 0.0
	for _, r := range rs {
		if r.OK {
			pass++
		}
		cost += r.CostUSD
	}
	fmt.Fprintf(&b, "# loadlore-designer skill evals\n\n%d of %d cases passed. The agent's runs cost $%.2f (judging not included).\n\n", pass, len(rs), cost)
	for _, r := range rs {
		mark := "pass"
		if !r.OK {
			mark = "FAIL"
		}
		fmt.Fprintf(&b, "## %s: %s\n\n", r.ID, mark)
		if r.Error != "" {
			fmt.Fprintf(&b, "Error: %s\n\n", r.Error)
		}
		for _, c := range r.Checks {
			m := "ok  "
			if !c.OK {
				m = "FAIL"
			}
			how := "checked"
			if c.Judged {
				how = "judged"
			}
			fmt.Fprintf(&b, "- %s %s (%s): %s\n", m, c.Name, how, c.Detail)
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}
