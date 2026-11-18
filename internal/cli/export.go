package cli

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/akinfemi/load-lore/internal/export"
	"github.com/akinfemi/load-lore/internal/rundir"
)

func newExportCmd(g *globalOpts) *cobra.Command {
	var to string
	cmd := &cobra.Command{
		Use:   "export [run...]",
		Short: "Write runs to a corpus, one compact record each, for fitting the simulator",
		Long: `Write each run's record to a corpus folder: runs/<run-id>.json and an
index.json listing every record there. A record is the run's design, plan
and analysis, analyzed afresh with this loadl's rules, with the conditions
it stood on: the network profile (round trip asked for and measured),
how many connections went through Toxiproxy, each Postgres's disk
profile, each service's CPU period, a --quick start, the host and its
speed index. It says whether fitting may use the run: not when it was
inconclusive, failed, or came before the speed index was measured.
Windows a stall of the Docker host's disk touched are left out of the
analysis and listed as excluded.

With no runs named, every run under .loadlore/runs is exported. --to names
the corpus folder (default .loadlore/corpus), such as a checkout of the
corpus repository; export writes files and never commits or pushes.
Exporting a run again replaces its record.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var dirs []string
			if len(args) == 0 {
				var err error
				if dirs, err = export.RunDirs(rundir.Root); err != nil {
					return err
				}
			}
			for _, ref := range args {
				dir, err := rundir.Resolve(rundir.Root, ref)
				if err != nil {
					return err
				}
				dirs = append(dirs, dir)
			}
			res, err := export.Write(to, dirs)
			if err != nil {
				return err
			}
			if g.json {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			printExport(cmd.OutOrStdout(), res)
			return nil
		},
	}
	cmd.Flags().StringVar(&to, "to", export.DefaultDir, "the corpus folder to write to")
	return cmd
}

func printExport(w io.Writer, res *export.Result) {
	runs := "runs"
	if len(res.Written) == 1 {
		runs = "run"
	}
	fmt.Fprintf(w, "exported %d %s to %s: %d usable for fitting, %d not\n", len(res.Written), runs, res.Dir, res.Usable, res.Unusable)
	excl := 0
	for _, e := range res.Written {
		if e.Excluded > 0 {
			excl++
		}
	}
	if excl > 0 {
		fmt.Fprintf(w, "  %d with windows left out for a host disk stall\n", excl)
	}
	if len(res.Skipped) > 0 {
		fmt.Fprintf(w, "skipped %d:\n", len(res.Skipped))
		for _, id := range slices.Sorted(maps.Keys(res.Skipped)) {
			fmt.Fprintf(w, "  %s: %s\n", id, strings.TrimSpace(res.Skipped[id]))
		}
	}
}
