// Package cli wires the loadl commands together.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/akinfemi/load-lore/internal/version"
)

// Exit codes. Scripts and the AI skill rely on these.
const (
	exitOK      = 0
	exitFailure = 1 // the command ran and the answer is "no" (invalid design, failed or inconclusive check)
	exitUsage   = 2 // bad flags or arguments, or an unexpected error
)

// exitError carries an exit code out of a command without printing anything
// further; the command has already written its own output.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

type globalOpts struct {
	json bool
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	return run(os.Args[1:], os.Stdout, os.Stderr)
}

func run(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	err := root.Execute()
	if err == nil {
		return exitOK
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	fmt.Fprintln(stderr, "error:", err)
	return exitUsage
}

func newRootCmd() *cobra.Command {
	g := &globalOpts{}
	root := &cobra.Command{
		Use:           "loadl",
		Short:         "Run a system design on real containers under load and faults, and see where and why it breaks",
		Version:       version.Version + " (" + version.Commit + ")",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVar(&g.json, "json", false, "write machine-readable JSON to stdout")

	root.AddCommand(
		newValidateCmd(g),
		newSchemaCmd(g),
		newDiagramCmd(g),
		newRunCmd(g),
		newReportCmd(g),
		newCompareCmd(g),
		newScenariosCmd(g),
		newHintCmd(g),
		newScenarioCmd(g),
		newDownCmd(g),
		newDoctorCmd(g),
		newPredictCmd(g),
		newStatusCmd(g),
		newWatchCmd(g),
		newVersionCmd(g),
	)
	return root
}

// writeJSON writes v as indented JSON followed by a newline.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
