package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/akinfemi/loadlore/internal/design"
)

type validateResult struct {
	File     string             `json:"file"`
	Valid    bool               `json:"valid"`
	Errors   design.Diagnostics `json:"errors"`
	Warnings design.Diagnostics `json:"warnings"`
}

func newValidateCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "validate <design.yaml>",
		Short: "Check a design file: schema, references and settings",
		Long: `Check a design file: schema, references between services, which fields apply
where, the expect lines and the assumption paths. Use - to read from stdin.

Errors carry a field path such as services.api.behavior.calls[1].target.
The exit code is 1 when there are errors; warnings alone don't fail.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			file := args[0]
			_, diags, err := loadDesign(cmd, file)
			if err != nil {
				return err
			}
			res := validateResult{File: file, Valid: !diags.HasErrors(), Errors: diags.Errors(), Warnings: diags.Warnings()}
			out := cmd.OutOrStdout()
			if g.json {
				if err := writeJSON(out, res); err != nil {
					return err
				}
			} else {
				printDiagnostics(out, file, diags)
				summary := "valid"
				if !res.Valid {
					summary = "invalid"
				}
				fmt.Fprintf(out, "%s: %s (%s, %s)\n", file, summary,
					count(len(res.Errors), "error"), count(len(res.Warnings), "warning"))
			}
			if !res.Valid {
				return &exitError{code: exitFailure}
			}
			return nil
		},
	}
}

// loadDesign reads a design from a file, or stdin for "-".
func loadDesign(cmd *cobra.Command, file string) (*design.Design, design.Diagnostics, error) {
	if file == "-" {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, nil, fmt.Errorf("reading stdin: %w", err)
		}
		d, diags := design.Load(data)
		return d, diags, nil
	}
	if _, err := os.Stat(file); err != nil {
		return nil, nil, err
	}
	return design.LoadFile(file)
}

func printDiagnostics(w io.Writer, file string, diags design.Diagnostics) {
	for _, d := range diags {
		loc := file
		if d.Line > 0 {
			loc = fmt.Sprintf("%s:%d:%d", file, d.Line, d.Column)
		}
		if d.Path != "" {
			fmt.Fprintf(w, "%s: %s: %s: %s\n", loc, d.Severity, d.Path, d.Message)
		} else {
			fmt.Fprintf(w, "%s: %s: %s\n", loc, d.Severity, d.Message)
		}
	}
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
