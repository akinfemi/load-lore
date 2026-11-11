//go:build unix

package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/akinfemi/load-lore/internal/rundir"
	"github.com/akinfemi/load-lore/internal/runner"
)

// detachWait is how long run --detach waits for the run to get its id:
// the design is checked and its folder made before pre-flight.
var detachWait = 60 * time.Second

type detached struct {
	RunID string `json:"run_id"`
	Dir   string `json:"dir"`
	PID   int    `json:"pid"`
	Log   string `json:"log"`
}

// runDetached starts the same loadl run in the background, in its own
// session so the terminal's Ctrl-C doesn't reach it, and returns once the
// run has its id. Its output goes to a log under .loadlore/logs.
func runDetached(cmd *cobra.Command, g *globalOpts, args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logDir := filepath.Join(filepath.Dir(rundir.Root), "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	logPath, _ := filepath.Abs(filepath.Join(logDir, "run-"+time.Now().UTC().Format("20060102-150405.000")+".log"))
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer logf.Close()

	child := exec.Command(exe, childRunArgs(cmd, args)...)
	child.Stdout, child.Stderr = logf, logf
	child.Env = append(os.Environ(), runner.LogEnv+"="+logPath)
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()

	deadline := time.After(detachWait)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		if id := runIDFromLog(logPath); id != "" {
			d := detached{RunID: id, Dir: filepath.Join(rundir.Root, id), PID: child.Process.Pid, Log: logPath}
			if g.json {
				return writeJSON(cmd.OutOrStdout(), d)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "started run %s in the background (pid %d)\n", d.RunID, d.PID)
			fmt.Fprintf(out, "  folder %s\n  log    %s\n", d.Dir, d.Log)
			fmt.Fprintf(out, "loadl status %s shows its progress; kill -INT %d stops it, with teardown\n", d.RunID, d.PID)
			return nil
		}
		select {
		case err := <-exited:
			b, _ := os.ReadFile(logPath)
			return fmt.Errorf("the run ended before it started (%v):\n%s", err, strings.TrimSpace(string(b)))
		case <-deadline:
			return fmt.Errorf("the run hasn't printed its id after %s; see %s", detachWait, logPath)
		case <-tick.C:
		}
	}
}

// childRunArgs are the run command's arguments for the background
// process: the flags that were set, without --detach, and without --json
// so the log reads as a terminal would, then the design.
func childRunArgs(cmd *cobra.Command, args []string) []string {
	out := []string{"run"}
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if f.Name == "detach" || f.Name == "json" {
			return
		}
		out = append(out, "--"+f.Name+"="+f.Value.String())
	})
	return append(out, args...)
}

// runIDFromLog finds the "run <id>" line a run prints first.
func runIDFromLog(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if id, ok := strings.CutPrefix(sc.Text(), "run "); ok && !strings.Contains(id, " ") {
			return id
		}
	}
	return ""
}
