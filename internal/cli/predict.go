package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/predict"
	"github.com/akinfemi/load-lore/internal/scenario"
)

type predictQuestion struct {
	N int `json:"n"`
	scenario.Question
	Choices []string `json:"choices,omitempty"` // for service answers
}

type predictResult struct {
	Scenario  string            `json:"scenario"`
	Design    string            `json:"design"`
	Questions []predictQuestion `json:"questions"`
	Lines     []string          `json:"lines,omitempty"` // written, once every question is answered
}

func newPredictCmd(g *globalOpts) *cobra.Command {
	var answers []string
	cmd := &cobra.Command{
		Use:   "predict <design.yaml>",
		Short: "Answer a scenario's questions before a run; the answers become the design's expect lines",
		Long: `Ask the questions of the scenario a design belongs to, turn each answer
into an expect line, and write them into the design's expect block,
replacing what's there. Nothing else in the file changes. The report
scores the lines after loadl run.

In a terminal, it asks each question in turn. --answer N=value answers
question N without asking, for scripts and AI agents; --json with no
answers lists the questions. Loads are requests per second (1000, 1.5k)
or never; a component is one of the design's services.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			sc, ok := scenario.ForDesign(path)
			if !ok {
				return fmt.Errorf("%s isn't part of a scenario, so there are no questions to ask; write expect lines yourself (loadl schema lists the metrics)", path)
			}
			if len(sc.Meta.Predict) == 0 {
				return fmt.Errorf("scenario %s has no questions for loadl predict", sc.Name)
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			d, diags := design.Load(src)
			if diags.HasErrors() {
				return fmt.Errorf("%s doesn't validate; run loadl validate %s", path, path)
			}
			services := slices.Sorted(maps.Keys(d.Services))
			res := predictResult{Scenario: sc.Name, Design: path}
			for i, q := range sc.Meta.Predict {
				pq := predictQuestion{N: i + 1, Question: q}
				if q.Answer == scenario.AnswerService {
					pq.Choices = services
				}
				res.Questions = append(res.Questions, pq)
			}

			given := map[int]string{}
			for _, a := range answers {
				k, v, ok := strings.Cut(a, "=")
				n, err := strconv.Atoi(strings.TrimSpace(k))
				if !ok || err != nil || n < 1 || n > len(res.Questions) {
					return fmt.Errorf("--answer %q: give N=value with N from 1 to %d", a, len(res.Questions))
				}
				given[n] = v
			}
			if g.json && len(given) == 0 {
				return writeJSON(cmd.OutOrStdout(), res)
			}

			out := cmd.OutOrStdout()
			interactive := !g.json
			in := bufio.NewReader(cmd.InOrStdin())
			lines := make([]string, len(res.Questions))
			var missing []string
			for i, q := range res.Questions {
				a, ok := given[q.N]
				if ok {
					l, err := predict.Line(q.Question, a, services)
					if err != nil {
						return fmt.Errorf("answer %d: %w", q.N, err)
					}
					lines[i] = l
					continue
				}
				if !interactive {
					missing = append(missing, strconv.Itoa(q.N))
					continue
				}
				if lines[i], err = ask(out, in, q, services); err != nil {
					return err
				}
			}
			if len(missing) > 0 {
				return fmt.Errorf("questions %s have no answer: give --answer N=value for each (loadl predict %s --json lists them)", strings.Join(missing, ", "), path)
			}
			updated, err := predict.Rewrite(src, lines)
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, updated, 0o644); err != nil {
				return err
			}
			res.Lines = lines
			if g.json {
				return writeJSON(out, res)
			}
			fmt.Fprintf(out, "\nwrote %d predictions to %s:\n", len(lines), path)
			for _, l := range lines {
				fmt.Fprintf(out, "  - %s\n", l)
			}
			fmt.Fprintf(out, "the report scores them after loadl run %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&answers, "answer", nil, "answer question N without asking, as N=value (repeatable)")
	return cmd
}

// ask asks q until it gets an answer that makes a line.
func ask(out io.Writer, in *bufio.Reader, q predictQuestion, services []string) (string, error) {
	fmt.Fprintf(out, "\n%d. %s\n", q.N, q.Ask)
	if len(q.Choices) > 0 {
		fmt.Fprintf(out, "   (%s)\n", strings.Join(q.Choices, ", "))
	}
	for {
		fmt.Fprint(out, "> ")
		a, err := in.ReadString('\n')
		if err != nil && (a == "" || !errors.Is(err, io.EOF)) {
			return "", fmt.Errorf("question %d wasn't answered: answer it here, or give --answer %d=value", q.N, q.N)
		}
		l, lerr := predict.Line(q.Question, a, services)
		if lerr == nil {
			return l, nil
		}
		fmt.Fprintf(out, "   %v\n", lerr)
		if err != nil { // EOF after a bad answer
			return "", fmt.Errorf("question %d wasn't answered: answer it here, or give --answer %d=value", q.N, q.N)
		}
	}
}
