package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/rundir"
	"github.com/akinfemi/load-lore/internal/runner"
)

// runStatus is what loadl status reports: the run's status.json, and
// whether the process running it is still there.
type runStatus struct {
	runner.Status
	Dir string `json:"dir"`
	// Alive is false when a run that hasn't finished has lost its loadl
	// process: it was killed, or the machine slept through it.
	Alive bool `json:"alive"`
}

func newStatusCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "status [run]",
		Short: "Show a run's progress, or its verdict once it has finished",
		Long: `Show where a run is: its phase, how far into the load, the last 10
seconds' numbers, and whether the live check has confirmed a break. Once
it has finished, its verdict and report. [run] is a run id, a prefix of
one, a folder, or latest (the default): the run written to most
recently, finished or not. --json is for scripts and AI agents polling a
run started with loadl run --detach.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := "latest"
			if len(args) == 1 {
				ref = args[0]
			}
			dir, err := rundir.ResolveAny(rundir.Root, ref)
			if err != nil {
				return err
			}
			rs, err := readRunStatus(dir)
			if err != nil {
				return err
			}
			if g.json {
				return writeJSON(cmd.OutOrStdout(), rs)
			}
			printStatus(cmd.OutOrStdout(), rs)
			return nil
		},
	}
}

func readRunStatus(dir string) (*runStatus, error) {
	rs := &runStatus{Dir: dir}
	b, err := os.ReadFile(filepath.Join(dir, rundir.StatusFile))
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &rs.Status); err != nil {
			return nil, fmt.Errorf("%s: %w", rundir.StatusFile, err)
		}
	case errors.Is(err, os.ErrNotExist):
		// A run from before status.json: its run.json says how it ended.
		var res runner.Result
		b, err := os.ReadFile(filepath.Join(dir, "run.json"))
		if err != nil || json.Unmarshal(b, &res) != nil {
			return nil, fmt.Errorf("%s has neither %s nor a readable run.json", dir, rundir.StatusFile)
		}
		rs.Status = runner.Status{RunID: res.RunID, Design: res.Design, Phase: runner.PhaseFinished, Started: res.Started, LoadStarted: res.LoadStarted,
			Status: res.Status, Error: res.Error, Verdict: res.Verdict, VerdictSummary: res.VerdictSummary, Report: res.Report}
	default:
		return nil, err
	}
	rs.Alive = rs.Phase == runner.PhaseFinished || processAlive(rs.PID)
	return rs, nil
}

// processAlive reports whether a process with this pid exists.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func printStatus(w io.Writer, rs *runStatus) {
	s := rs.Status
	fmt.Fprintf(w, "run %s (%s)\n", s.RunID, s.Design)
	switch {
	case s.Phase == runner.PhaseFinished:
		fmt.Fprintf(w, "  %s", s.Status)
		if s.Error != "" && s.Error != s.Status {
			fmt.Fprintf(w, ": %s", s.Error)
		}
		fmt.Fprintln(w)
		if s.VerdictSummary != "" {
			fmt.Fprintf(w, "  %s\n", s.VerdictSummary)
		}
		if s.Report != "" {
			fmt.Fprintf(w, "  report: %s (loadl report %s --print shows it here)\n", s.Report, s.RunID)
		}
		return
	case !rs.Alive:
		fmt.Fprintf(w, "  stopped in %s without finishing: its loadl process (pid %d) is gone. loadl down removes what it left running.\n", s.Phase, s.PID)
		return
	case s.Phase != runner.PhaseLoad:
		fmt.Fprintf(w, "  %s, since %s\n", map[string]string{
			runner.PhasePreflight: "checking the host", runner.PhaseStarting: "starting the stack",
			runner.PhaseCollecting: "collecting metrics and logs", runner.PhaseTeardown: "tearing down",
		}[s.Phase], s.Updated.Local().Format(time.TimeOnly))
	default:
		fmt.Fprintf(w, "  load %s of %s", analysis.Clock(s.ElapsedS), analysis.Clock(s.ProfileS))
		if x := s.Sample; x != nil {
			fmt.Fprintf(w, " · offered %s rps", analysis.Rate(x.Offered))
			if x.Served != nil {
				fmt.Fprintf(w, " · served %s rps", analysis.Rate(*x.Served))
			}
			if x.P99Ms != nil {
				fmt.Fprintf(w, " · p99 %s", analysis.Ms(*x.P99Ms))
			}
			if x.Errors != nil {
				fmt.Fprintf(w, " · errors %s", analysis.Pct(*x.Errors))
			}
		}
		fmt.Fprintln(w)
		if s.BreakConfirmedS != nil && s.StopsAtS != nil && s.BreakAtS != nil {
			fmt.Fprintf(w, "  break confirmed at %s (from %s); the load stops at %s\n", analysis.Clock(*s.BreakConfirmedS), analysis.Clock(*s.BreakAtS), analysis.Clock(*s.StopsAtS))
		}
	}
	fmt.Fprintf(w, "  pid %d: kill -INT %d stops it, with teardown\n", s.PID, s.PID)
	if s.Log != "" {
		fmt.Fprintf(w, "  log: %s\n", s.Log)
	}
}
