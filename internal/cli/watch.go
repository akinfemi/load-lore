package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/akinfemi/load-lore/internal/dashboard"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/rundir"
	"github.com/akinfemi/load-lore/internal/runner"
)

func newWatchCmd(_ *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "watch [run]",
		Short: "Follow a run on the live dashboard, such as one started with run --detach",
		Long: `Open the live dashboard for a run in progress: the topology colored by
saturation, offered and served load, p50/p95/p99 and errors over the
profile with faults and the break marked, and each component's numbers.
q or Ctrl-C stops watching; the run goes on. [run] is a run id, a prefix
of one, a folder, or latest (the default). For a finished run, it prints
the last frame.`,
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
			m := dashboard.New(dir)
			out := cmd.OutOrStdout()
			if s, err := dashboard.Load(dir); err == nil && s.Status.Phase == runner.PhaseFinished {
				fmt.Fprint(out, dashboard.Render(s, termWidth(), ""))
				return nil
			}
			if _, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(out), tea.WithInput(cmd.InOrStdin())).Run(); err != nil {
				return err
			}
			fmt.Fprint(out, m.Frame())
			if !m.Finished() {
				fmt.Fprintf(out, "\nstopped watching; the run goes on (loadl status %s)\n", filepath.Base(dir))
			}
			return nil
		},
	}
}

// runWatched runs a design with the live dashboard in front: the run
// goes on in this process, its progress lines go to a log, and Ctrl-C on
// the dashboard stops it with teardown. It falls back to plain waiting if
// the dashboard can't start.
func runWatched(cmd *cobra.Command, opt runner.Options, newDriver func(context.Context) (driver.Driver, error)) (*runner.Result, error) {
	logDir := filepath.Join(filepath.Dir(rundir.Root), "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, "run-"+time.Now().UTC().Format("20060102-150405.000")+".log")
	logf, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	defer logf.Close()
	opt.Progress = logf
	dirs := make(chan string, 1)
	opt.OnDir = func(dir string) { dirs <- dir }

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()
	type outcome struct {
		res *runner.Result
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := runner.Run(ctx, opt, newDriver)
		done <- outcome{res, err}
	}()

	var dir string
	select {
	case dir = <-dirs:
	case o := <-done: // invalid design, or no folder
		return o.res, o.err
	}
	out := cmd.OutOrStdout()
	m := dashboard.New(dir)
	m.Interrupt = cancel
	if _, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(out), tea.WithInput(cmd.InOrStdin())).Run(); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "the dashboard couldn't start (%v); waiting for the run, whose progress is in %s\n", err, logPath)
	}
	o := <-done
	fmt.Fprint(out, m.Frame())
	fmt.Fprintf(out, "\nprogress log: %s\n", logPath)
	return o.res, o.err
}

// isTerminalOut reports whether w is an interactive terminal.
func isTerminalOut(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
