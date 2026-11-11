package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/driver/local"
	"github.com/akinfemi/load-lore/internal/rundir"
	"github.com/akinfemi/load-lore/internal/runner"
	"github.com/akinfemi/load-lore/internal/scenario"
)

func newScenariosCmd(g *globalOpts) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "scenarios [search]",
		Short: "List and search the scenario library",
		Long: `List the scenarios built into loadl, by level, or the scenario folders in
--dir. A search matches every word against the name, title and concepts.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var all []*scenario.Scenario
			var err error
			if dir != "" {
				all, err = scenariosIn(dir)
			} else {
				all, err = scenario.Library()
			}
			if err != nil {
				return err
			}
			q := ""
			if len(args) == 1 {
				q = args[0]
			}
			found := []*scenario.Scenario{}
			for _, s := range all {
				if s.Matches(q) {
					found = append(found, s)
				}
			}
			if g.json {
				return writeJSON(cmd.OutOrStdout(), found)
			}
			if len(found) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no scenario matches %q\n", q)
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tLEVEL\tMINUTES\tTITLE\tCONCEPTS")
			for _, s := range found {
				fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%s\n", s.Name, s.Meta.Level, s.Meta.Minutes, s.Meta.Title, strings.Join(s.Meta.Concepts, ", "))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "list the scenario folders in this directory instead of the built-in library")
	return cmd
}

func scenariosIn(dir string) ([]*scenario.Scenario, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []*scenario.Scenario
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(p, scenario.MetaFile)); err != nil {
			continue
		}
		s, err := scenario.Open(p)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// hintState is .loadlore/hints.json: how many hints each scenario has
// revealed, and the scenario last asked about.
type hintState struct {
	Current  string         `json:"current,omitempty"`
	Revealed map[string]int `json:"revealed"`
}

const hintsFile = ".loadlore/hints.json"

func readHints() hintState {
	st := hintState{Revealed: map[string]int{}}
	if b, err := os.ReadFile(hintsFile); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if st.Revealed == nil {
		st.Revealed = map[string]int{}
	}
	return st
}

func (st hintState) save() error {
	if err := os.MkdirAll(filepath.Dir(hintsFile), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(hintsFile, append(b, '\n'), 0o644)
}

type hintResult struct {
	Scenario string `json:"scenario"`
	Number   int    `json:"number"`
	Of       int    `json:"of"`
	Title    string `json:"title,omitempty"`
	Body     string `json:"body,omitempty"`
	Done     bool   `json:"done"` // every hint has been revealed
	Reset    bool   `json:"reset,omitempty"`
}

func newHintCmd(g *globalOpts) *cobra.Command {
	var (
		reset bool
		show  int
	)
	cmd := &cobra.Command{
		Use:   "hint [scenario]",
		Short: "Reveal the next hint for the current scenario",
		Long: `Reveal the next hint for a scenario, one at a time. Without an argument, the
scenario is the one the latest run's design belongs to, else the one you
last asked about. The argument is a library name or a scenario folder.
Progress is kept in .loadlore/hints.json.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st := readHints()
			s, err := currentScenario(args, st)
			if err != nil {
				return err
			}
			st.Current = s.Name
			n := len(s.Hints)
			res := hintResult{Scenario: s.Name, Of: n}
			switch {
			case n == 0:
				return fmt.Errorf("scenario %s has no hints", s.Name)
			case reset:
				delete(st.Revealed, s.Name)
				res.Reset = true
			case show > 0:
				if show > st.Revealed[s.Name] {
					return fmt.Errorf("hint %d isn't revealed yet; hints come in order (%d of %d so far)", show, st.Revealed[s.Name], n)
				}
				res.Number = show
			case st.Revealed[s.Name] >= n:
				res.Done = true
			default:
				st.Revealed[s.Name]++
				res.Number = st.Revealed[s.Name]
			}
			if res.Number > 0 {
				h := s.Hints[res.Number-1]
				res.Title, res.Body = h.Title, h.Body
			}
			res.Done = res.Done || st.Revealed[s.Name] >= n
			if err := st.save(); err != nil {
				return err
			}
			if g.json {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			printHint(cmd.OutOrStdout(), res)
			return nil
		},
	}
	cmd.Flags().BoolVar(&reset, "reset", false, "hide this scenario's hints again")
	cmd.Flags().IntVar(&show, "show", 0, "show an already revealed hint again, by number")
	return cmd
}

func printHint(w io.Writer, r hintResult) {
	switch {
	case r.Reset:
		fmt.Fprintf(w, "hints for %s are hidden again; loadl hint shows the first\n", r.Scenario)
		return
	case r.Number == 0:
		fmt.Fprintf(w, "All %d hints for %s are revealed. loadl hint --show <n> shows one again; one worked answer is in the scenario's solutions/ folder.\n", r.Of, r.Scenario)
		return
	}
	fmt.Fprintf(w, "Hint %d of %d · %s\n\n%s\n\n", r.Number, r.Of, r.Title, r.Body)
	if r.Done {
		fmt.Fprintln(w, "That was the last hint. One worked answer is in the scenario's solutions/ folder.")
	} else {
		fmt.Fprintf(w, "Next: loadl hint (%d of %d)\n", r.Number+1, r.Of)
	}
}

// currentScenario picks the scenario a hint is for.
func currentScenario(args []string, st hintState) (*scenario.Scenario, error) {
	if len(args) == 1 {
		return scenario.Find(args[0])
	}
	if dir, err := rundir.Resolve(rundir.Root, "latest"); err == nil {
		var rec struct {
			Design     string `json:"design"`
			DesignPath string `json:"design_path"`
		}
		if b, err := os.ReadFile(filepath.Join(dir, "run.json")); err == nil && json.Unmarshal(b, &rec) == nil {
			if rec.DesignPath != "" {
				if s, ok := scenario.ForDesign(rec.DesignPath); ok {
					return s, nil
				}
			}
			if s := libraryByDesignName(rec.Design); s != nil {
				return s, nil
			}
		}
	}
	if st.Current != "" {
		return scenario.Find(st.Current)
	}
	return nil, errors.New("no current scenario: the latest run isn't from one; name it, as in loadl hint single-postgres-breaking-point (loadl scenarios lists them)")
}

// libraryByDesignName matches a design copied out of a scenario: its name
// is the scenario's, or the scenario's plus a suffix, as solutions are.
func libraryByDesignName(name string) *scenario.Scenario {
	lib, err := scenario.Library()
	if err != nil {
		return nil
	}
	var best *scenario.Scenario
	for _, s := range lib {
		if (name == s.Name || strings.HasPrefix(name, s.Name+"-")) && (best == nil || len(s.Name) > len(best.Name)) {
			best = s
		}
	}
	return best
}

func newScenarioCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scenario",
		Short: "Work with scenario folders",
	}
	cmd.AddCommand(newScenarioCheckCmd(g))
	return cmd
}

func newScenarioCheckCmd(g *globalOpts) *cobra.Command {
	var (
		scale  float64
		seed   int64
		static bool
		image  string
	)
	cmd := &cobra.Command{
		Use:   "check <folder | name>",
		Short: "Run a scenario and its solutions to confirm it shows the problem and each solution helps",
		Long: `Check a scenario folder: every file is there and every design validates,
the README's diagram matches the design, then the starting design and each
solution run in turn and must show what meta.yaml's check block claims.

A solution helps when it holds, or its peak throughput (the most successful
responses per second in any window) is at least min_gain times the starting
design's (default 1.2). --static skips the runs. Runs are
stored like any other, so each has its report. Runs after the first wait
for the host speed index to get back near the first run's, since a machine
slows as it heats up, and a solution whose run was more than 10% faster or
slower than the starting design's gets a warning.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := scenario.Find(args[0])
			if err != nil {
				return err
			}
			tmp := ""
			if s.Path == "" {
				if tmp, err = os.MkdirTemp("", "loadl-scenario-"); err != nil {
					return err
				}
				defer os.RemoveAll(tmp)
			}
			dir, err := s.Materialize(tmp)
			if err != nil {
				return err
			}
			progress := cmd.ErrOrStderr()
			if !g.json {
				progress = cmd.OutOrStdout()
			}
			var run scenario.RunFunc
			if !static {
				// Later runs wait for the host to cool back to the first
				// run's speed, so a solution isn't judged on a hotter machine.
				coolTo := 0
				run = func(ctx context.Context, designPath string) (string, error) {
					rel, _ := filepath.Rel(dir, designPath)
					fmt.Fprintf(progress, "\n== %s: %s\n", s.Name, rel)
					res, err := runner.Run(ctx, runner.Options{DesignPath: designPath, Scale: scale, Seed: seed, SyntheticImage: image, Progress: progress, CoolTo: coolTo},
						func(ctx context.Context) (driver.Driver, error) { return local.New(ctx) })
					if res == nil {
						return "", err
					}
					if coolTo == 0 && res.Host != nil && res.Host.Speed != nil {
						coolTo = res.Host.Speed.Index
					}
					return res.Dir, err
				}
			}
			res := scenario.Check(cmd.Context(), s, dir, run)
			if g.json {
				if err := writeJSON(cmd.OutOrStdout(), res); err != nil {
					return err
				}
			} else {
				printCheck(cmd.OutOrStdout(), res, static)
			}
			if !res.OK {
				return &exitError{code: exitFailure}
			}
			return nil
		},
	}
	cmd.Flags().Float64Var(&scale, "scale", 0, "run every design at this scale, such as 0.25 (default: each design's modes.local.scale, else 1)")
	cmd.Flags().Int64Var(&seed, "seed", 1, "seed for the load generator, the same for every design so runs compare")
	cmd.Flags().BoolVar(&static, "static", false, "check the folder without running anything")
	cmd.Flags().StringVar(&image, "synthetic-image", "loadlore/synthetic:dev", "image for loadlore/synthetic services")
	return cmd
}

func printCheck(w io.Writer, r *scenario.CheckResult, static bool) {
	fmt.Fprintf(w, "\nscenario %s\n", r.Scenario)
	if len(r.Lint) == 0 {
		fmt.Fprintln(w, "  files: ok")
	}
	for _, p := range r.Lint {
		fmt.Fprintf(w, "  %s\n", p)
	}
	for _, rc := range append([]*scenario.RunCheck{r.Problem}, r.Solutions...) {
		if rc == nil {
			continue
		}
		label := "starting design"
		if rc.Name != "problem" {
			label = "solution " + rc.Name
		}
		fmt.Fprintf(w, "  %s", label)
		if rc.RunDir != "" {
			fmt.Fprintf(w, " (run %s)", filepath.Base(rc.RunDir))
		}
		fmt.Fprintln(w)
		for _, c := range rc.Claims {
			mark := "ok  "
			if !c.OK {
				mark = "FAIL"
			}
			fmt.Fprintf(w, "    %s %s: %s\n", mark, c.Claim, c.Detail)
		}
		for _, x := range rc.Warnings {
			fmt.Fprintf(w, "    warning: %s\n", x)
		}
	}
	switch {
	case !r.OK:
		fmt.Fprintln(w, "FAILED")
	case static:
		fmt.Fprintln(w, "ok (files only; without --static the designs run too)")
	default:
		fmt.Fprintln(w, "ok")
	}
}
