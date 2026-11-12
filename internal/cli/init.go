package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/scenario"
)

// initAnswers are the wizard's questions, with their defaults.
type initAnswers struct {
	Name     string  `json:"name"`
	Replicas int     `json:"replicas"`
	CPU      float64 `json:"cpu"`
	Store    string  `json:"store"` // postgres, cache, none
	RPS      int     `json:"rps"`
	Minutes  int     `json:"minutes"`
	Fault    string  `json:"fault"` // none, kill, slow-db, cache-restart
}

func defaultInitAnswers() initAnswers {
	return initAnswers{Name: "my-service", Replicas: 2, CPU: 0.5, Store: "postgres", RPS: 500, Minutes: 3, Fault: "none"}
}

type initResult struct {
	Path     string       `json:"path"`
	Scenario string       `json:"scenario,omitempty"`
	Answers  *initAnswers `json:"answers,omitempty"`
}

var kebab = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func newInitCmd(g *globalOpts) *cobra.Command {
	var out string
	var blank, force bool
	a := defaultInitAnswers()
	cmd := &cobra.Command{
		Use:   "init [scenario]",
		Short: "Start a design: copy a library scenario, or answer a few questions for a new design file",
		Long: `With a scenario name, copy that scenario from the library into a folder
of its own (./<name>, or --out), so you can work through it outside a
checkout: loadl predict, run and hint all know it.

Without one, ask a few questions in a terminal (an api's replicas and CPU,
its datastore, the peak load and how long to ramp to it, and a fault) and
write a design file that validates, design.yaml or --out. The answers can
also be given as flags, for scripts and AI agents; without a terminal, or
with --json, the flags and their defaults are used without asking.
--blank writes the defaults with no questions.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return initScenario(cmd, g, args[0], out, force)
			}
			if out == "" {
				out = "design.yaml"
			}
			if _, err := os.Stat(out); err == nil && !force {
				return fmt.Errorf("%s exists; pick another --out, or --force to overwrite it", out)
			}
			interactive := !blank && !g.json && isTerminal(cmd.InOrStdin())
			for _, f := range []string{"name", "replicas", "cpu", "store", "rps", "minutes", "fault"} {
				if cmd.Flags().Changed(f) {
					interactive = false // answers given: don't ask
				}
			}
			if interactive {
				if err := askInit(cmd.OutOrStdout(), bufio.NewReader(cmd.InOrStdin()), &a); err != nil {
					return err
				}
			}
			src, err := initDesign(a)
			if err != nil {
				return err
			}
			if err := os.WriteFile(out, []byte(src), 0o644); err != nil {
				return err
			}
			if g.json {
				return writeJSON(cmd.OutOrStdout(), initResult{Path: out, Answers: &a})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "wrote %s\n\nNext:\n  loadl validate %s\n  loadl diagram %s --ascii\n  loadl run %s\n\nThe file explains each part; the design file reference is in skills/loadlore-designer/references/schema.md.\n", out, out, out, out)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&out, "out", "o", "", "where to write: the design file (default design.yaml), or the folder a scenario goes into (default ./<scenario>)")
	f.BoolVar(&blank, "blank", false, "write the defaults without asking")
	f.BoolVar(&force, "force", false, "overwrite what's there")
	f.StringVar(&a.Name, "name", a.Name, "the design's name, lowercase with dashes")
	f.IntVar(&a.Replicas, "replicas", a.Replicas, "api replicas; more than one puts a load balancer in front")
	f.Float64Var(&a.CPU, "cpu", a.CPU, "CPU cores per api replica")
	f.StringVar(&a.Store, "store", a.Store, "datastore: postgres, cache (Redis in front of Postgres) or none")
	f.IntVar(&a.RPS, "rps", a.RPS, "peak load in requests per second")
	f.IntVar(&a.Minutes, "minutes", a.Minutes, "minutes to ramp to the peak; it then holds for a minute")
	f.StringVar(&a.Fault, "fault", a.Fault, "a fault halfway through the ramp: none, kill (one api replica), slow-db or cache-restart")
	return cmd
}

func initScenario(cmd *cobra.Command, g *globalOpts, name, out string, force bool) error {
	s, err := scenario.Find(name)
	if err != nil {
		return err
	}
	parent := "."
	if out != "" {
		parent = filepath.Dir(filepath.Clean(out))
	}
	dst := filepath.Join(parent, s.Name)
	if out != "" {
		dst = filepath.Clean(out)
	}
	if _, err := os.Stat(dst); err == nil && !force {
		return fmt.Errorf("%s exists; pick another --out, or --force to overwrite it", dst)
	}
	if s.Path != "" {
		return fmt.Errorf("%s is already a folder on disk at %s: work in it there", name, s.Path)
	}
	tmp, err := os.MkdirTemp(parent, ".loadl-init-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	got, err := s.Materialize(tmp)
	if err != nil {
		return err
	}
	if force {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	}
	if err := os.Rename(got, dst); err != nil {
		return err
	}
	if g.json {
		return writeJSON(cmd.OutOrStdout(), initResult{Path: dst, Scenario: s.Name})
	}
	design := filepath.Join(dst, scenario.DesignFile)
	fmt.Fprintf(cmd.OutOrStdout(), "copied %s to %s\n\nStart with its README:\n  less %s\n  loadl predict %s\n  loadl run %s\n  loadl hint %s\n", s.Meta.Title, dst, filepath.Join(dst, scenario.ReadmeFile), design, design, dst)
	return nil
}

// askInit asks each question, keeping the default on an empty answer.
func askInit(w io.Writer, in *bufio.Reader, a *initAnswers) error {
	fmt.Fprintln(w, "A few questions for a new design file. Press Enter to keep the default in [brackets].")
	ask := func(q, def string, ok func(string) error) (string, error) {
		for {
			fmt.Fprintf(w, "\n%s [%s] ", q, def)
			line, err := in.ReadString('\n')
			line = strings.TrimSpace(line)
			switch {
			case err == nil:
			case !errors.Is(err, io.EOF):
				return "", err
			case line == "":
				return def, nil // input ended: keep the default
			}
			if line == "" {
				line = def
			}
			if e := ok(line); e != nil {
				fmt.Fprintf(w, "  %v\n", e)
				continue
			}
			return line, nil
		}
	}
	intIn := func(lo, hi int) func(string) error {
		return func(s string) error {
			n, err := strconv.Atoi(s)
			if err != nil || n < lo || n > hi {
				return fmt.Errorf("a whole number from %d to %d", lo, hi)
			}
			return nil
		}
	}
	oneOf := func(opts ...string) func(string) error {
		return func(s string) error {
			for _, o := range opts {
				if s == o {
					return nil
				}
			}
			return fmt.Errorf("one of %s", strings.Join(opts, ", "))
		}
	}
	var err error
	var s string
	if a.Name, err = ask("The design's name, lowercase with dashes:", a.Name, func(s string) error {
		if !kebab.MatchString(s) {
			return errors.New("lowercase letters, digits and dashes, such as checkout-api")
		}
		return nil
	}); err != nil {
		return err
	}
	if s, err = ask("How many api replicas? More than one gets a load balancer in front.", strconv.Itoa(a.Replicas), intIn(1, 20)); err != nil {
		return err
	}
	a.Replicas, _ = strconv.Atoi(s)
	if s, err = ask("CPU cores per api replica:", strconv.FormatFloat(a.CPU, 'g', -1, 64), func(s string) error {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || f <= 0 || f > 16 {
			return errors.New("a number of cores, such as 0.5 or 2")
		}
		return nil
	}); err != nil {
		return err
	}
	a.CPU, _ = strconv.ParseFloat(s, 64)
	if a.Store, err = ask("Its datastore: postgres, cache (Redis in front of Postgres) or none?", a.Store, oneOf("postgres", "cache", "none")); err != nil {
		return err
	}
	if s, err = ask("Peak load, in requests per second:", strconv.Itoa(a.RPS), intIn(1, 50000)); err != nil {
		return err
	}
	a.RPS, _ = strconv.Atoi(s)
	if s, err = ask("Minutes to ramp up to it? It then holds for a minute.", strconv.Itoa(a.Minutes), intIn(1, 30)); err != nil {
		return err
	}
	a.Minutes, _ = strconv.Atoi(s)
	faults := []string{"none", "kill"}
	if a.Store != "none" {
		faults = append(faults, "slow-db")
	}
	if a.Store == "cache" {
		faults = append(faults, "cache-restart")
	}
	if a.Fault, err = ask(fmt.Sprintf("A fault halfway through the ramp: %s?", strings.Join(faults, ", ")), a.Fault, oneOf(faults...)); err != nil {
		return err
	}
	return nil
}

// initDesign writes a commented design file from the answers, and checks
// that it validates.
func initDesign(a initAnswers) (string, error) {
	if !kebab.MatchString(a.Name) {
		return "", fmt.Errorf("--name %q: lowercase letters, digits and dashes, such as checkout-api", a.Name)
	}
	switch {
	case a.Replicas < 1:
		return "", errors.New("--replicas must be at least 1")
	case a.CPU <= 0:
		return "", errors.New("--cpu must be more than 0")
	case a.RPS < 1:
		return "", errors.New("--rps must be at least 1")
	case a.Minutes < 1:
		return "", errors.New("--minutes must be at least 1")
	}
	switch a.Store {
	case "postgres", "cache", "none":
	default:
		return "", fmt.Errorf("--store %q: postgres, cache or none", a.Store)
	}
	switch {
	case a.Fault == "none", a.Fault == "kill":
	case a.Fault == "slow-db" && a.Store != "none":
	case a.Fault == "cache-restart" && a.Store == "cache":
	default:
		return "", fmt.Errorf("--fault %q doesn't fit --store %s: none, kill, slow-db (with a datastore) or cache-restart (with --store cache)", a.Fault, a.Store)
	}

	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	p("version: 1")
	p("name: %s", a.Name)
	p("")
	p("# Written by loadl init. Every service runs as a container with the CPU and")
	p("# memory limits below; change them, the calls, the load and the faults to")
	p("# match your system. The design file reference is in")
	p("# skills/loadlore-designer/references/schema.md.")
	p("")
	p("services:")
	entry := "api"
	if a.Replicas > 1 {
		entry = "lb"
		p("  lb:")
		p("    kind: lb               # HAProxy, round-robin across api's replicas")
		p("    target: api")
		p("    health_check: { interval_ms: 2000, fall: 3, rise: 2 }")
		p("")
	}
	p("  api:")
	p("    image: loadlore/synthetic   # does what behavior says, so you can model any service")
	if a.Replicas > 1 {
		p("    replicas: %d", a.Replicas)
	}
	p("    resources: { cpu: %s, memory: 256m }", strconv.FormatFloat(a.CPU, 'g', -1, 64))
	p("    behavior:")
	p("      cpu_ms: 1              # CPU time per request: your handler's own work")
	switch a.Store {
	case "none":
		p("      # calls: what each request does, in order: reads and writes to a")
		p("      # datastore, or http calls to another service.")
	case "postgres":
		p("      calls:")
		p("        - { target: db, op: read }")
		p("        - { target: db, op: write, when: write }")
	case "cache":
		p("      calls:")
		p("        - target: cache")
		p("          op: get")
		p("          on_miss:          # on a miss, read the database and fill the cache")
		p("            - { target: db, op: read }")
		p("            - { target: cache, op: set, ttl_s: 300 }")
		p("        - { target: db, op: write, when: write }")
	}
	if a.Store != "none" {
		p("      resilience:")
		p("        timeout_ms: 1000     # per call; copy your client's real timeout")
		p("    pools:")
		if a.Store == "cache" {
			p("      cache: 10")
		}
		p("      db: 10               # connections per replica")
	}
	if a.Store == "cache" {
		p("")
		p("  cache:")
		p("    kind: redis")
		p("    resources: { cpu: 0.5, memory: 256m }")
	}
	if a.Store != "none" {
		p("")
		p("  db:")
		p("    kind: postgres")
		p("    version: \"17\"")
		p("    resources: { cpu: 1, memory: 1g }")
		p("    dataset: { rows: 100000, row_bytes: 256 }   # seeded before the run")
	}
	p("")
	p("workload:")
	p("  entry: %s", entry)
	p("  profile:")
	start := max(1, a.RPS/10)
	p("    - ramp: { from: %d, to: %d, over: %dm }", start, a.RPS, a.Minutes)
	p("    - hold: { rps: %d, for: 1m }", a.RPS)
	if a.Store == "none" {
		p("  mix: { read: 1.0 }")
	} else {
		p("  mix: { read: 0.9, write: 0.1 }")
		p("  keys: { distribution: zipf, skew: 1.1 }   # a few keys get most of the traffic")
	}
	half := a.Minutes * 30 // seconds: halfway through the ramp
	switch a.Fault {
	case "kill":
		p("")
		p("scenario:")
		p("  - { at: %ds, inject: kill, target: \"api[0]\" }", half)
	case "slow-db":
		p("")
		p("scenario:")
		p("  - { at: %ds, inject: latency, target: db, add_ms: 100, for: 30s }", half)
	case "cache-restart":
		p("")
		p("scenario:")
		p("  - { at: %ds, inject: restart, target: cache }", half)
	default:
		p("")
		p("# scenario: faults at set times, such as")
		p("#   - { at: 2m, inject: kill, target: \"api[0]\" }")
	}
	p("")
	p("# Your predictions, scored in the report after the run, such as")
	p("#   - break_load_rps within 25%% of %d", a.RPS/2)
	p("#   - p99_ms < 300 until load_rps >= %d", a.RPS/2)
	p("expect:")
	p("  - error_rate < 1%%")

	src := b.String()
	if _, diags := design.Load([]byte(src)); diags.HasErrors() {
		return "", fmt.Errorf("the generated design doesn't validate, which is a loadl bug: %v", diags.Errors())
	}
	return src, nil
}

// isTerminal is whether r is an interactive terminal.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
