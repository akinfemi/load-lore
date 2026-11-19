package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/driver/local"
	"github.com/akinfemi/load-lore/internal/runner"
	"github.com/akinfemi/load-lore/internal/sweep"
	"github.com/akinfemi/load-lore/internal/version"
)

func newSweepCmd(g *globalOpts) *cobra.Command {
	var cpus, resume, image string
	var repeat int
	var seed int64
	var list bool
	cmd := &cobra.Command{
		Use:   "sweep [case...]",
		Short: "Run the component sweeps: one operation on one component, at several CPU sizes",
		Long: `Run small designs that each stress one operation on one component, the
target: Postgres point, range and batch reads and flushed writes, Redis
gets and sets, a service-to-service hop, a load balancer hop, reads from
a replica, queue publishes and a fan-out read over two shards. Each runs
at several CPU sizes of the target, a few times each, on a 3-minute ramp
that stops a minute after the break. A point whose run holds doubles its
ramp's top, up to twice. A ramp that would need the load generator past
10,000 requests a second makes more calls to the target per request
instead; a point that can't (a load balancer hop is one call a request)
is listed in the summary as not run here. Everything but the target is
sized to give out later; a run whose break names another component is
kept but not counted.

The sweep's designs, state and summary.md go to .loadlore/sweeps/<id>/;
the runs to .loadlore/runs, each tagged with its sweep, case and CPU size,
for loadl export. --resume <id> continues a stopped sweep. Runs after the
first wait for the host speed index to get back near the first's.

The defaults (11 cases, 5 sizes, 3 runs; Redis and queues up to 1 CPU, the lb
0.25 on an 8-CPU host) make about 130 runs, overnight on an 8-CPU Docker host.
--list prints the cases.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cases, err := sweep.Find(args)
			if err != nil {
				return err
			}
			if list {
				printCases(cmd.OutOrStdout(), cases)
				return nil
			}
			sizes, err := parseCPUs(cpus)
			if err != nil {
				return err
			}
			progress := cmd.OutOrStdout()
			if g.json {
				progress = cmd.ErrOrStderr()
			}
			ctx, stop := untilInterrupted(cmd.Context())
			defer stop()
			st, dir, err := sweep.Do(ctx, sweep.Options{
				Cases: cases, CPUs: sizes, Repeat: repeat, Seed: seed, Resume: resume,
				Run:       runner.Options{SyntheticImage: image},
				NewDriver: func(ctx context.Context) (driver.Driver, error) { return local.New(ctx) },
				Progress:  progress,
			})
			if st != nil {
				if g.json {
					if werr := writeJSON(cmd.OutOrStdout(), st); werr != nil {
						return werr
					}
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "\nsweep %s: summary %s/summary.md\n", st.ID, dir)
					if !st.Done {
						fmt.Fprintf(cmd.OutOrStdout(), "not finished: loadl sweep --resume %s runs the rest\n", st.ID)
					}
				}
			}
			return err
		},
	}
	cmd.Flags().StringVar(&cpus, "cpus", joinCPUs(sweep.DefaultCPUs), "the target's CPU sizes, comma-separated")
	cmd.Flags().IntVar(&repeat, "repeat", sweep.DefaultRepeat, "runs per case and size")
	cmd.Flags().Int64Var(&seed, "seed", 1, "seed for every run's keys and mix")
	cmd.Flags().StringVar(&resume, "resume", "", "continue the sweep with this id")
	cmd.Flags().StringVar(&image, "synthetic-image", version.SyntheticImage(), "image for loadlore/synthetic services")
	cmd.Flags().BoolVar(&list, "list", false, "print the cases and exit")
	return cmd
}

func parseCPUs(s string) ([]float64, error) {
	var out []float64
	for _, f := range strings.Split(s, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("--cpus %q: give CPU sizes above 0, comma-separated, such as 0.5,1,2", s)
		}
		out = append(out, v)
	}
	return out, nil
}

func joinCPUs(cs []float64) string {
	var s []string
	for _, c := range cs {
		s = append(s, strconv.FormatFloat(c, 'f', -1, 64))
	}
	return strings.Join(s, ",")
}

func printCases(w io.Writer, cs []sweep.Case) {
	for _, c := range cs {
		extra := ""
		if c.MaxCPU > 0 {
			extra = fmt.Sprintf(", up to %g CPU", c.MaxCPU)
		}
		calls := fmt.Sprintf("%d calls", c.Calls)
		if c.Calls == 1 {
			calls = "1 call"
		}
		fmt.Fprintf(w, "%-16s %s (target %s, %s a request%s)\n", c.Name, c.What, c.Target, calls, extra)
	}
}
