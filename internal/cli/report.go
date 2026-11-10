package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/report"
	"github.com/akinfemi/load-lore/internal/rundir"
)

func newReportCmd(g *globalOpts) *cobra.Command {
	var print bool
	cmd := &cobra.Command{
		Use:   "report <run>",
		Short: "Write a run's report: verdict, diagram, charts, diagnosis and scored predictions",
		Long: `Analyze a run folder and write report.md and analysis.json into it.

<run> is a run id, a unique prefix of one, a path to the run folder, or
"latest". Everything in the report comes from the run folder, so a report
can be regenerated at any time, by any version of loadl that reads the
design. --json prints the analysis instead of a summary.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := rundir.Resolve(rundir.Root, args[0])
			if err != nil {
				return err
			}
			a, err := report.Write(dir)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch {
			case g.json:
				return writeJSON(out, a)
			case print:
				b, err := os.ReadFile(filepath.Join(dir, report.FileName))
				if err != nil {
					return err
				}
				_, err = out.Write(b)
				return err
			}
			printSummary(cmd, a, filepath.Join(dir, report.FileName))
			return nil
		},
	}
	cmd.Flags().BoolVar(&print, "print", false, "print the markdown report instead of a summary")
	return cmd
}

// printSummary prints the verdict, the diagnoses and the predictions in a
// few lines, and where the full report is.
func printSummary(cmd *cobra.Command, a *analysis.Analysis, path string) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, a.Verdict.Summary)
	for _, d := range a.Diagnoses {
		fmt.Fprintf(out, "  diagnosis: %s, from %s\n", d.Title, analysis.Clock(d.From))
	}
	for _, e := range a.Expect {
		switch e.Status {
		case analysis.ExpectHeld, analysis.ExpectBroken:
			fmt.Fprintf(out, "  prediction %s: %s (worst %s)\n", e.Status, e.Text, e.Shown)
		case analysis.ExpectUnscored:
			fmt.Fprintf(out, "  prediction not scored: %s (%s)\n", e.Text, e.Message)
		}
	}
	fmt.Fprintf(out, "report: %s\n", path)
}

func newCompareCmd(g *globalOpts) *cobra.Command {
	var outPath string
	cmd := &cobra.Command{
		Use:   "compare <run-a> <run-b>",
		Short: "Compare two runs side by side, such as before and after a design change",
		Long: `Write a markdown report comparing two runs: verdicts, throughput and
latency over time, diagnoses, predictions and the design diff. Run A is the
baseline. The report goes to stdout unless -o names a file. --json prints
both analyses.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var as [2]*analysis.Analysis
			for i, ref := range args {
				dir, err := rundir.Resolve(rundir.Root, ref)
				if err != nil {
					return err
				}
				if as[i], err = analysis.Analyze(dir); err != nil {
					return err
				}
			}
			if g.json {
				return writeJSON(cmd.OutOrStdout(), map[string]any{"a": as[0], "b": as[1]})
			}
			md := report.Compare(as[0], as[1])
			if outPath == "" {
				_, err := fmt.Fprint(cmd.OutOrStdout(), md)
				return err
			}
			if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", outPath)
			return nil
		},
	}
	cmd.Flags().StringVarP(&outPath, "out", "o", "", "write the comparison to this file")
	return cmd
}
