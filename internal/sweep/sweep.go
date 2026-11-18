// Package sweep runs the component sweeps: small designs that each stress
// one operation on one component, at several CPU sizes, a few times each,
// on short ramps. Their breaks are the simulator's per-operation costs.
package sweep

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/rundir"
	"github.com/akinfemi/load-lore/internal/runner"
)

// Root is where sweeps keep their designs, state and summaries.
const Root = ".loadlore/sweeps"

// DefaultCPUs are the target's sizes: about five, from a quarter of a CPU
// to two, which fit next to the callers and the tooling on an 8-CPU
// Docker host. Bigger sizes need a bigger host (--cpus).
var DefaultCPUs = []float64{0.25, 0.5, 1, 1.5, 2}

// DefaultRepeat is how many times each point runs.
const DefaultRepeat = 3

// MaxDoublings is how many times a point's ramp top doubles when a run
// holds, before the point is left as held.
const MaxDoublings = 2

// Find returns the named cases, every case for none.
func Find(names []string) ([]Case, error) {
	if len(names) == 0 {
		return slices.Clone(Cases), nil
	}
	var out []Case
	for _, n := range names {
		i := slices.IndexFunc(Cases, func(c Case) bool { return c.Name == n })
		if i < 0 {
			var all []string
			for _, c := range Cases {
				all = append(all, c.Name)
			}
			return nil, fmt.Errorf("no sweep case %q; the cases are %s", n, strings.Join(all, ", "))
		}
		out = append(out, Cases[i])
	}
	return out, nil
}

// Render fills a case's design at a CPU size and ramp top.
func Render(c Case, cpu float64, peak int) ([]byte, error) {
	t, err := template.New(c.Name).Parse(c.Design)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	low := max(10, peak/20)
	err = t.Execute(&b, Params{Name: fmt.Sprintf("sweep-%s-cpu%s", c.Name, cpuName(cpu)), CPU: cpu, Low: low, Peak: peak})
	return b.Bytes(), err
}

// Peak is the first ramp top for a case at a CPU size: twice the guess.
func Peak(c Case, cpu float64) int {
	return int(math.Round(2*c.PerCPU*cpu/10) * 10)
}

func cpuLabel(cpu float64) string { return strconv.FormatFloat(cpu, 'f', -1, 64) }

// cpuName is a CPU size as design names allow: 0.25 is 0p25.
func cpuName(cpu float64) string { return strings.ReplaceAll(cpuLabel(cpu), ".", "p") }

// Point is one case at one CPU size.
type Point struct {
	Case   string  `json:"case"`
	Target string  `json:"target"`
	CPU    float64 `json:"cpu"`
	Calls  int     `json:"calls_per_request"`
	Peak   int     `json:"peak_rps"` // the ramp top the repeats use
	Runs   []*Run  `json:"runs"`
	Held   bool    `json:"held,omitempty"` // every ramp top tried held
}

// Run is one run of a point.
type Run struct {
	RunID   string   `json:"run_id"`
	Peak    int      `json:"peak_rps"`
	Verdict string   `json:"verdict"`
	BreakAt *float64 `json:"break_at_s,omitempty"`
	// BreakRPS is the offered rate at the break, and PerCall the calls a
	// second to the target per CPU it stood for.
	BreakRPS *float64 `json:"break_offered_rps,omitempty"`
	MaxRPS   *float64 `json:"max_rps,omitempty"`
	PerCall  *float64 `json:"calls_per_s_per_cpu,omitempty"`
	// LimitedBy is the most saturated component at the break; a run whose
	// break wasn't the target's doesn't measure it.
	LimitedBy string `json:"limited_by,omitempty"`
	Counts    bool   `json:"counts"` // broke, conclusively, at the target
	Speed     *int   `json:"speed_index,omitempty"`
	Error     string `json:"error,omitempty"`
}

// State is a sweep's progress, saved after every run so it can resume.
type State struct {
	ID      string    `json:"id"`
	Started time.Time `json:"started"`
	Seed    int64     `json:"seed"`
	Repeat  int       `json:"repeat"`
	CPUs    []float64 `json:"cpus"`
	Points  []*Point  `json:"points"`
	Done    bool      `json:"done"`
}

// Options are a sweep's settings.
type Options struct {
	Cases  []Case
	CPUs   []float64
	Repeat int
	Seed   int64
	// Resume continues the sweep with this id, skipping finished runs.
	Resume string
	// Run is the base for every run: image, timeout, progress.
	Run       runner.Options
	NewDriver func(context.Context) (driver.Driver, error)
	Progress  io.Writer
	// RunOne runs a design; runner.Run unless a test sets it.
	RunOne func(context.Context, runner.Options, func(context.Context) (driver.Driver, error)) (*runner.Result, error)
	// Analyze reads a finished run; analysis.Analyze unless a test sets it.
	Analyze func(dir string) (*analysis.Analysis, error)
}

// Do runs the sweep, or the rest of one, and writes sweep.json and
// summary.md to its folder as it goes.
func Do(ctx context.Context, o Options) (*State, string, error) {
	out := o.Progress
	if out == nil {
		out = io.Discard
	}
	if o.RunOne == nil {
		o.RunOne = runner.Run
	}
	if o.Analyze == nil {
		o.Analyze = analysis.Analyze
	}
	st, dir, err := open(o)
	if err != nil {
		return nil, "", err
	}
	save := func() error {
		b, err := json.MarshalIndent(st, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "sweep.json"), append(b, '\n'), 0o644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "summary.md"), []byte(Summary(st)), 0o644)
	}
	if err := save(); err != nil {
		return nil, "", err
	}
	total, done := 0, 0
	for _, p := range st.Points {
		total += st.Repeat
		done += counted(p)
	}
	fmt.Fprintf(out, "sweep %s: %d points, %d runs (%d done)\n  folder %s\n", st.ID, len(st.Points), total, done, dir)
	coolTo := 0
	for _, p := range st.Points {
		for _, r := range p.Runs {
			if r.Speed != nil && coolTo == 0 {
				coolTo = *r.Speed
			}
		}
	}
	for _, p := range st.Points {
		c := caseNamed(o.Cases, p.Case)
		for counted(p) < st.Repeat && !p.Held {
			if err := ctx.Err(); err != nil {
				return st, dir, err
			}
			idx := counted(p) + 1
			fmt.Fprintf(out, "\n== %s at %s CPU, ramp to %d rps, run %d of %d\n", p.Case, cpuLabel(p.CPU), p.Peak, idx, st.Repeat)
			raw, err := Render(c, p.CPU, p.Peak)
			if err != nil {
				return st, dir, err
			}
			path := filepath.Join(dir, "designs", fmt.Sprintf("%s-cpu%s-peak%d.yaml", p.Case, cpuName(p.CPU), p.Peak))
			if err := os.WriteFile(path, raw, 0o644); err != nil {
				return st, dir, err
			}
			ro := o.Run
			ro.DesignPath, ro.Seed, ro.Progress, ro.CoolTo = path, st.Seed, out, coolTo
			ro.Sweep = &runner.SweepRef{ID: st.ID, Case: p.Case, Target: p.Target, CPU: p.CPU, Peak: float64(p.Peak), Index: idx}
			res, err := o.RunOne(ctx, ro, o.NewDriver)
			r := &Run{Peak: p.Peak}
			if res != nil {
				r.RunID = res.RunID
				if res.Host != nil && res.Host.Speed != nil {
					s := res.Host.Speed.Index
					r.Speed = &s
					if coolTo == 0 {
						coolTo = s
					}
				}
			}
			switch {
			case err != nil:
				r.Error = err.Error()
			case res == nil:
				r.Error = "no result"
			default:
				judge(o, res.Dir, p, r)
			}
			p.Runs = append(p.Runs, r)
			// A run that held climbed too low: double the top and run the
			// point again, up to MaxDoublings times.
			if r.Verdict == string(analysis.VerdictHeld) {
				if doublings(p) < MaxDoublings {
					p.Peak *= 2
					fmt.Fprintf(out, "  held: the ramp's top doubles to %d rps\n", p.Peak)
				} else {
					p.Held = true
					fmt.Fprintf(out, "  held at every top tried; the point is left as held\n")
				}
			}
			if err := save(); err != nil {
				return st, dir, err
			}
			if r.Error != "" && ctx.Err() != nil {
				return st, dir, ctx.Err()
			}
			if r.Error != "" && failedTwice(p) {
				fmt.Fprintf(out, "  two runs of this point failed; going on to the next\n")
				break
			}
		}
	}
	st.Done = true
	return st, dir, save()
}

// judge fills a run's numbers from its analysis.
func judge(o Options, dir string, p *Point, r *Run) {
	a, err := o.Analyze(dir)
	if err != nil {
		r.Error = err.Error()
		return
	}
	v := a.Verdict
	r.Verdict, r.MaxRPS = string(v.Kind), v.MaxRPS
	if b := v.Break; b != nil {
		at, off := b.At, b.Offered
		r.BreakAt, r.BreakRPS = &at, &off
		r.LimitedBy = limitedBy(b.Saturation)
		per := off * float64(p.Calls) / p.CPU
		r.PerCall = &per
		r.Counts = v.Kind == analysis.VerdictBroke && r.LimitedBy == p.Target
	}
}

// limitedBy is the component that gave out: the most saturated, where a
// full pool stands for its target (requests queue for connections
// because the target is slow) and connection counts don't count.
func limitedBy(ss []analysis.Saturation) string {
	for _, s := range ss {
		if t, ok := strings.CutPrefix(s.What, "pool to "); ok {
			return t
		}
		if s.What != "connections" {
			return s.Component
		}
	}
	return ""
}

// counted is how many of a point's runs count toward its repeats: those
// at its current ramp top that didn't fail to run.
func counted(p *Point) int {
	n := 0
	for _, r := range p.Runs {
		if r.Peak == p.Peak && r.Error == "" && r.Verdict != string(analysis.VerdictHeld) {
			n++
		}
	}
	return n
}

func doublings(p *Point) int {
	n := 0
	for _, r := range p.Runs {
		if r.Verdict == string(analysis.VerdictHeld) {
			n++
		}
	}
	return n
}

func failedTwice(p *Point) bool {
	n := 0
	for _, r := range p.Runs {
		if r.Error != "" {
			n++
		}
	}
	return n >= 2
}

func caseNamed(cs []Case, name string) Case {
	for _, c := range cs {
		if c.Name == name {
			return c
		}
	}
	for _, c := range Cases {
		if c.Name == name {
			return c
		}
	}
	return Case{}
}

// open starts a sweep, or loads the one to resume.
func open(o Options) (*State, string, error) {
	if o.Resume != "" {
		dir := filepath.Join(Root, o.Resume)
		b, err := os.ReadFile(filepath.Join(dir, "sweep.json"))
		if err != nil {
			return nil, "", fmt.Errorf("no sweep %s to resume: %w", o.Resume, err)
		}
		var st State
		if err := json.Unmarshal(b, &st); err != nil {
			return nil, "", err
		}
		st.Done = false
		return &st, dir, nil
	}
	if o.Repeat < 1 {
		return nil, "", fmt.Errorf("--repeat %d: give 1 or more runs", o.Repeat)
	}
	seed := o.Seed
	if seed == 0 {
		seed = 1
	}
	st := &State{ID: rundir.NewID("sweep", time.Now()), Started: time.Now().UTC(), Seed: seed, Repeat: o.Repeat, CPUs: o.CPUs, Points: []*Point{}}
	for _, c := range o.Cases {
		for _, cpu := range o.CPUs {
			if c.MaxCPU > 0 && cpu > c.MaxCPU {
				continue
			}
			raw, err := Render(c, cpu, Peak(c, cpu))
			if err != nil {
				return nil, "", err
			}
			if _, diags := design.Load(raw); diags.HasErrors() {
				return nil, "", fmt.Errorf("sweep case %s at %s CPU doesn't validate: %v", c.Name, cpuLabel(cpu), diags.Errors())
			}
			st.Points = append(st.Points, &Point{Case: c.Name, Target: c.Target, CPU: cpu, Calls: c.Calls, Peak: Peak(c, cpu), Runs: []*Run{}})
		}
	}
	dir := filepath.Join(Root, st.ID)
	if err := os.MkdirAll(filepath.Join(dir, "designs"), 0o755); err != nil {
		return nil, "", err
	}
	return st, dir, nil
}
