package cli

import (
	"github.com/spf13/cobra"

	"github.com/akinfemi/loadlore/internal/design"
)

func newSchemaCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Print the design file's JSON Schema for this version",
		Long: `Print the JSON Schema of the design file, generated from the types this
version of loadl accepts. The output is always JSON; --json is accepted for
consistency with the other commands.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = g
			return writeJSON(cmd.OutOrStdout(), design.JSONSchema())
		},
	}
}
