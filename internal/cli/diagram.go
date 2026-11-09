package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/akinfemi/loadlore/internal/diagram"
)

func newDiagramCmd(g *globalOpts) *cobra.Command {
	var ascii bool
	cmd := &cobra.Command{
		Use:   "diagram <design.yaml>",
		Short: "Print the design's topology as Mermaid, or as ASCII with --ascii",
		Long: `Print the design's topology, generated from the design file: the load
generator, each service, and the calls between them with their pools.
Mermaid by default (renders on GitHub); --ascii draws it for the terminal.
With --json, both forms are returned. Use - to read from stdin.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			file := args[0]
			d, diags, err := loadDesign(cmd, file)
			if err != nil {
				return err
			}
			if diags.HasErrors() {
				if g.json {
					_ = writeJSON(cmd.OutOrStdout(), validateResult{File: file, Errors: diags.Errors(), Warnings: diags.Warnings()})
				} else {
					printDiagnostics(cmd.ErrOrStderr(), file, diags.Errors())
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: invalid; run loadl validate for details\n", file)
				}
				return &exitError{code: exitFailure}
			}

			f := diagram.Flowchart(d)
			out := cmd.OutOrStdout()
			if g.json {
				art, err := diagram.ASCII(f)
				if err != nil {
					return err
				}
				return writeJSON(out, map[string]string{"mermaid": f.String(), "ascii": art})
			}
			if ascii {
				art, err := diagram.ASCII(f)
				if err != nil {
					return err
				}
				_, err = fmt.Fprint(out, art)
				return err
			}
			_, err = fmt.Fprint(out, f.String())
			return err
		},
	}
	cmd.Flags().BoolVar(&ascii, "ascii", false, "draw the diagram for the terminal instead of Mermaid")
	return cmd
}
