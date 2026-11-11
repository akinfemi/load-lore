package cli

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/doctor"
	"github.com/akinfemi/load-lore/internal/driver/local"
	"github.com/akinfemi/load-lore/internal/power"
	"github.com/akinfemi/load-lore/internal/runner"
	"github.com/akinfemi/load-lore/internal/scenario"
)

func newDoctorCmd(g *globalOpts) *cobra.Command {
	var image string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check that this machine can run the scenarios, and say how to fix what it can't",
		Long: `Check Docker (Colima, Docker Desktop or Linux), Compose, the CPUs and
memory available to Docker against what each scenario needs, the
synthetic image, the host speed index and, on macOS, power and Low Power
Mode. Each problem comes with its fix.

It takes about 10 seconds: the speed benchmark runs in a container. A
failed check means runs won't start (exit 1); a warning means they will,
but results may drift or differ from the scenario READMEs.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()
			f := doctor.Facts{GOOS: runtime.GOOS, Power: power.Read(ctx), SlowShare: runner.SlowHostShare}
			f.Scenarios, _ = scenario.Library()
			if d, err := local.New(ctx); err != nil {
				f.DockerErr = err
			} else {
				f.Inspection, f.DockerErr = d.Inspect(ctx, image)
			}
			cs := doctor.Checks(f)
			if g.json {
				if err := writeJSON(cmd.OutOrStdout(), map[string]any{"ok": !doctor.Failed(cs), "checks": cs}); err != nil {
					return err
				}
			} else {
				printDoctor(cmd.OutOrStdout(), cs)
			}
			if doctor.Failed(cs) {
				return &exitError{code: exitFailure}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&image, "synthetic-image", design.SyntheticImage+":dev", "image for loadlore/synthetic services")
	return cmd
}

func printDoctor(w io.Writer, cs []doctor.Check) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, c := range cs {
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", c.Status, c.Name, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(tw, "  \t\tfix: %s\n", c.Fix)
		}
	}
	_ = tw.Flush()
	var fails, warns []string
	for _, c := range cs {
		switch c.Status {
		case doctor.Fail:
			fails = append(fails, c.Name)
		case doctor.Warn:
			warns = append(warns, c.Name)
		}
	}
	switch {
	case len(fails) > 0:
		fmt.Fprintf(w, "\nruns won't start until %s %s fixed\n", strings.Join(fails, ", "), map[bool]string{true: "is", false: "are"}[len(fails) == 1])
	case len(warns) > 0:
		fmt.Fprintf(w, "\nready to run; the warnings can make results drift or differ from the scenario READMEs\n")
	default:
		fmt.Fprintln(w, "\nready to run")
	}
}
