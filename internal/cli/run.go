package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/driver/local"
	"github.com/akinfemi/load-lore/internal/runner"
)

func newRunCmd(g *globalOpts) *cobra.Command {
	var (
		mode    string
		scale   float64
		seed    int64
		keep    bool
		timeout time.Duration
		image   string
		reuse   bool
		repeat  int
	)
	cmd := &cobra.Command{
		Use:   "run <design.yaml>",
		Short: "Run a design on real containers under load and faults",
		Long: `Run a design: seed its datasets, start the stack on Docker, apply the load
profile and the scenario's faults, collect metrics and logs into
.loadlore/runs/<run-id>/, and tear everything down.

Teardown runs on success, failure, Ctrl-C and timeout. --keep leaves the
stack up to poke at; remove it later with loadl down.

--repeat N runs the design N times with the same seed and writes a series
summary to .loadlore/series/<id>/ with the spread of break time, peak
served and p99 at the break. Runs after the first wait for the host speed
index to get back near the first run's, since a machine slows as it heats
up; loadl compare uses the spread to say when a difference may be noise.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if mode != "local" {
				return fmt.Errorf("--mode %s isn't available yet; only local runs are", mode)
			}
			progress := cmd.ErrOrStderr()
			if !g.json {
				progress = cmd.OutOrStdout()
			}
			opt := runner.Options{DesignPath: args[0], Scale: scale, Seed: seed, Keep: keep, Timeout: timeout, SyntheticImage: image, ReuseData: reuse, Progress: progress}
			newDriver := func(ctx context.Context) (driver.Driver, error) { return local.New(ctx) }
			switch {
			case repeat < 1:
				return fmt.Errorf("--repeat %d: give 1 or more runs", repeat)
			case repeat > 1 && keep:
				return errors.New("--keep and --repeat don't go together: each run tears down before the next")
			case repeat > 1:
				sr, err := runner.Repeat(cmd.Context(), opt, repeat, newDriver)
				return finishRepeat(cmd, g, args[0], sr, err)
			}
			res, err := runner.Run(cmd.Context(), opt, newDriver)
			var inv *runner.InvalidDesignError
			if errors.As(err, &inv) {
				if g.json {
					_ = writeJSON(cmd.OutOrStdout(), validateResult{File: args[0], Errors: inv.Diagnostics.Errors(), Warnings: inv.Diagnostics.Warnings()})
				} else {
					printDiagnostics(cmd.ErrOrStderr(), args[0], inv.Diagnostics.Errors())
				}
				return &exitError{code: exitFailure}
			}
			if g.json && res != nil {
				if werr := writeJSON(cmd.OutOrStdout(), res); werr != nil {
					return werr
				}
			}
			if err != nil {
				if res != nil {
					if !g.json {
						fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
					}
					return &exitError{code: exitFailure}
				}
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "local", "where to run: local (cloud and sim come later)")
	cmd.Flags().Float64Var(&scale, "scale", 0, "shrink resources and load together, such as 0.5 (default: the design's modes.local.scale, else 1)")
	cmd.Flags().Int64Var(&seed, "seed", 0, "seed for the load generator's random keys and mix (0 = random)")
	cmd.Flags().BoolVar(&keep, "keep", false, "leave the stack running after the load ends")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "stop and tear down after this long (default: 15m setup + the profile + 5m)")
	cmd.Flags().BoolVar(&reuse, "reuse-data", false, "skip the fresh copy of seeded data and reuse a shared one; earlier runs' writes carry over, and the report says so")
	cmd.Flags().StringVar(&image, "synthetic-image", "loadlore/synthetic:dev", "image for loadlore/synthetic services")
	cmd.Flags().IntVar(&repeat, "repeat", 1, "run the design this many times with the same seed and report the spread")
	return cmd
}

func finishRepeat(cmd *cobra.Command, g *globalOpts, file string, sr *runner.SeriesResult, err error) error {
	var inv *runner.InvalidDesignError
	if errors.As(err, &inv) {
		if g.json {
			_ = writeJSON(cmd.OutOrStdout(), validateResult{File: file, Errors: inv.Diagnostics.Errors(), Warnings: inv.Diagnostics.Warnings()})
		} else {
			printDiagnostics(cmd.ErrOrStderr(), file, inv.Diagnostics.Errors())
		}
		return &exitError{code: exitFailure}
	}
	if sr == nil {
		return err
	}
	if g.json {
		if werr := writeJSON(cmd.OutOrStdout(), sr); werr != nil {
			return werr
		}
	} else if sr.Series != nil {
		printSeries(cmd.OutOrStdout(), sr)
	}
	if err != nil {
		if !g.json {
			fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
		}
		return &exitError{code: exitFailure}
	}
	return nil
}

func printSeries(w io.Writer, sr *runner.SeriesResult) {
	s := sr.Series
	fmt.Fprintf(w, "\nseries %s: %d of %d runs\n", s.ID, len(s.Runs), s.Planned)
	for i, r := range s.Runs {
		fmt.Fprintf(w, "  %d. %s: %s, speed index %d\n", i+1, r.RunID, r.Verdict, r.Speed)
	}
	line := func(name string, sp *analysis.Spread, f func(float64) string) {
		if sp != nil {
			fmt.Fprintf(w, "  %s: %s to %s (median %s) over %d runs\n", name, f(sp.Min), f(sp.Max), f(sp.Median), sp.N)
		}
	}
	rps := func(v float64) string { return analysis.Rate(v) + " rps" }
	line("break time", s.BreakAt, analysis.Clock)
	line("peak served", s.PeakServed, rps)
	line("p99 at the break", s.P99AtBreak, analysis.Ms)
	for _, x := range s.Warnings {
		fmt.Fprintf(w, "  warning: %s\n", x)
	}
	fmt.Fprintf(w, "summary: %s\n", sr.Report)
}

func newDownCmd(g *globalOpts) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Remove everything Loadlore left running",
		Long: `Remove every container, network and volume labeled as Loadlore's, from any
run. Cached seeded datasets stay unless --all is given.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Minute)
			defer cancel()
			counts, err := local.DownAll(ctx, all)
			if g.json {
				res := map[string]any{"removed": counts}
				if err != nil {
					res["error"] = err.Error()
				}
				if werr := writeJSON(cmd.OutOrStdout(), res); werr != nil {
					return werr
				}
			} else {
				printDown(cmd.OutOrStdout(), counts, all)
			}
			if err != nil {
				if !g.json {
					fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
				}
				return &exitError{code: exitFailure}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "also remove cached seeded datasets")
	return cmd
}

func printDown(w io.Writer, c local.Counts, all bool) {
	fmt.Fprintf(w, "removed %s, %s, %s", count(c.Containers, "container"), count(c.Networks, "network"), count(c.Volumes, "volume"))
	if !all {
		fmt.Fprint(w, " (dataset caches kept; --all removes them)")
	}
	fmt.Fprintln(w)
}
