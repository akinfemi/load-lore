// Command skilleval runs the loadlore-designer skill's eval set with Claude
// Code (claude on PATH, authenticated) and loadl on PATH, and writes a
// report. Every case and every judged check is a paid model call; each
// has a budget cap (--agent-usd, --judge-usd).
//
//	go run ./cmd/skilleval [-only id,id] [-out dir]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akinfemi/load-lore/internal/skilleval"
)

func main() {
	set := flag.String("set", "skills/loadlore-designer/evals/prompts.yaml", "the eval set")
	skill := flag.String("skill", "skills/loadlore-designer", "the skill folder to install in each workspace")
	out := flag.String("out", "skilleval-out", "where workspaces, results.json and report.md go")
	only := flag.String("only", "", "comma-separated case ids to run (a case that continues another brings it along)")
	agentModel := flag.String("agent-model", "", "the model the agent runs on (default: Claude Code's)")
	judgeModel := flag.String("judge-model", "claude-opus-5-5", "the judge's model")
	agentUSD := flag.Float64("agent-usd", 2, "budget per case, in dollars")
	judgeUSD := flag.Float64("judge-usd", 0.5, "budget per judged check, in dollars")
	flag.Parse()

	work, err := filepath.Abs(filepath.Join(*out, "workspaces"))
	if err == nil {
		err = os.MkdirAll(work, 0o755)
	}
	if err != nil {
		fail(err)
	}
	skillDir, _ := filepath.Abs(*skill)
	setPath, _ := filepath.Abs(*set)
	var ids []string
	if *only != "" {
		ids = strings.Split(*only, ",")
	}
	rs, err := skilleval.Run(context.Background(), skilleval.Options{
		SetPath: setPath, SkillDir: skillDir, WorkRoot: work, Only: ids,
		Runner: skilleval.Claude{AgentModel: *agentModel, JudgeModel: *judgeModel, AgentUSD: *agentUSD, JudgeUSD: *judgeUSD},
		Loadl:  skilleval.LoadlOnPath, TranscriptDir: filepath.Join(*out, "transcripts"),
	})
	if err != nil {
		fail(err)
	}
	raw, _ := json.MarshalIndent(rs, "", "  ")
	report := skilleval.Markdown(rs)
	if err := os.WriteFile(filepath.Join(*out, "results.json"), raw, 0o644); err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(*out, "report.md"), []byte(report), 0o644); err != nil {
		fail(err)
	}
	fmt.Print(report)
	for _, r := range rs {
		if !r.OK {
			os.Exit(1)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "skilleval:", err)
	os.Exit(2)
}
