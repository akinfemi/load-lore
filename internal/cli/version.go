package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/akinfemi/loadlore/internal/version"
)

func newVersionCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the loadl version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if g.json {
				return writeJSON(cmd.OutOrStdout(), map[string]string{
					"version": version.Version,
					"commit":  version.Commit,
					"go":      runtime.Version(),
				})
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "loadl %s (%s, %s)\n", version.Version, version.Commit, runtime.Version())
			return err
		},
	}
}
