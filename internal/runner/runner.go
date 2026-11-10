// Package runner runs one experiment end to end: design → plan → preflight →
// stack up → load with the fault timeline → collect → teardown. Teardown
// always runs: on success, failure, Ctrl-C and timeout, unless --keep.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/report"
	"github.com/akinfemi/load-lore/internal/rundir"
	"github.com/akinfemi/load-lore/internal/version"
)

// Options configure a run.
type Options struct {
	DesignPath     string
	Scale          float64 // 0 = the design's modes.local.scale
	Seed           int64
	Keep           bool
	Timeout        time.Duration // 0 = setup allowance + workload + margin
	SyntheticImage string
	ReuseData      bool      // skip the fresh dataset copy
	Root           string    // runs folder; default rundir.Root
	Progress       io.Writer // plain progress lines
}

// Run statuses.
const (
	StatusCompleted   = "completed"
	StatusFailed      = "failed"
	StatusInterrupted = "interrupted"
	StatusTimedOut    = "timed_out"
)

// Result is the run's outcome, also written to run.json.
type Result struct {
	RunID       string             `json:"run_id"`
	Dir         string             `json:"dir"`
	Design      string             `json:"design"`
	Status      string             `json:"status"`
	Error       string             `json:"error,omitempty"`
	Scale       float64            `json:"scale"`
	Seed        int64              `json:"seed"`
	ReuseData   bool               `json:"reuse_data"`
	Started     time.Time          `json:"started"`
	LoadStarted *time.Time         `json:"load_started,omitempty"`
	LoadEnded   *time.Time         `json:"load_ended,omitempty"`
	Finished    time.Time          `json:"finished"`
	Host        *driver.Host       `json:"host,omitempty"`
	Warnings    []string           `json:"warnings"`
	Notes       []string           `json:"notes"`
	Load        *driver.LoadResult `json:"load,omitempty"`
	Kept        bool               `json:"kept"`
	Loadl       string             `json:"loadl_version"`

	// From the report, once written: broke, held, inconclusive or no_data.
	Verdict        string `json:"verdict,omitempty"`
	VerdictSummary string `json:"verdict_summary,omitempty"`
	Report         string `json:"report,omitempty"`
}

// setupAllowance covers seeding, copying data and starting the stack.
const setupAllowance = 15 * time.Minute

// scrapeSettle is how long to wait after the load for the last scrapes.
var scrapeSettle = 3 * time.Second

// Run runs a design. Diagnostics from an invalid design come back as
// *InvalidDesignError.
func Run(parent context.Context, opt Options, newDriver func(context.Context) (driver.Driver, error)) (*Result, error) {
	out := opt.Progress
	if out == nil {
		out = io.Discard
	}
	raw, err := os.ReadFile(opt.DesignPath)
	if err != nil {
		return nil, err
	}
	d, diags := design.Load(raw)
	if diags.HasErrors() {
		return nil, &InvalidDesignError{Diagnostics: diags}
	}

	started := time.Now()
	root := opt.Root
	if root == "" {
		root = rundir.Root
	}
	id := rundir.NewID(d.Name, started)
	dir, err := rundir.Create(root, id)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	res := &Result{RunID: id, Dir: dir.Path, Design: d.Name, Started: started.UTC(), Seed: opt.Seed, ReuseData: opt.ReuseData, Loadl: version.Version, Warnings: []string{}}
	if err := dir.Write("design.yaml", raw); err != nil {
		return nil, err
	}

	p, err := plan.Build(d, plan.Options{RunID: id, Scale: opt.Scale, Seed: opt.Seed, SyntheticImage: opt.SyntheticImage, ReuseData: opt.ReuseData})
	if err != nil {
		return nil, err
	}
	res.Scale, res.Notes = p.Scale, p.Notes
	if err := dir.WriteJSON("plan.json", p); err != nil {
		return nil, err
	}
	fmt.Fprintf(out, "run %s\n  folder %s\n", id, dir.Path)
	for _, n := range p.Notes {
		fmt.Fprintf(out, "  note: %s\n", n)
	}

	drv, err := newDriver(parent)
	if err != nil {
		return nil, err
	}
	pf, err := drv.Preflight(parent, p)
	if err != nil {
		return nil, fmt.Errorf("preflight: %w", err)
	}
	res.Host = &pf.Host
	res.Warnings = append(res.Warnings, pf.Warnings...)
	if p.TotalCPU() > float64(pf.Host.CPUs) {
		if s := fittingScale(d, opt, pf.Host.CPUs); s > 0 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("--scale %s fits this host's %d CPUs", fmtF(s), pf.Host.CPUs))
		}
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(out, "  warning: %s\n", w)
	}
	_ = dir.Event("run_start", map[string]any{"design": d.Name, "scale": p.Scale, "seed": opt.Seed, "host": pf.Host})

	// Ctrl-C and SIGTERM cancel the run; teardown still happens. A second
	// Ctrl-C during teardown exits at once (run `loadl down` afterwards).
	ctx, stopSignals := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	timeout := opt.Timeout
	if timeout == 0 {
		timeout = setupAllowance + p.Duration + 5*time.Minute
	}
	ctx, cancelTimeout := context.WithTimeout(ctx, timeout)
	defer cancelTimeout()

	runErr := runStack(ctx, drv, p, dir, res, out)

	switch {
	case runErr == nil:
		res.Status = StatusCompleted
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.Status = StatusTimedOut
		res.Error = fmt.Sprintf("the run didn't finish within %s", timeout)
	case ctx.Err() != nil:
		res.Status = StatusInterrupted
		res.Error = "interrupted"
	default:
		res.Status = StatusFailed
		res.Error = runErr.Error()
	}

	stopSignals()
	if opt.Keep && res.Status != StatusFailed {
		res.Kept = true
		fmt.Fprintf(out, "keeping the stack up (--keep); remove it with `loadl down`\n")
	} else {
		fmt.Fprintf(out, "tearing down (press Ctrl-C again to skip; then run `loadl down`)\n")
		tctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		start := time.Now()
		if err := drv.Down(tctx); err != nil {
			res.Warnings = append(res.Warnings, "teardown: "+err.Error()+" (run `loadl down`)")
			fmt.Fprintf(out, "  warning: teardown: %v\n", err)
		}
		cancel()
		_ = dir.Event("teardown", map[string]any{"seconds": time.Since(start).Seconds()})
	}

	res.Finished = time.Now().UTC()
	_ = dir.Event("run_end", map[string]any{"status": res.Status, "error": res.Error})
	if err := dir.WriteJSON("run.json", res); err != nil {
		return res, err
	}
	// The report reads run.json, then the verdict goes back into it.
	if a, err := report.Write(dir.Path); err != nil {
		res.Warnings = append(res.Warnings, "report: "+err.Error())
		fmt.Fprintf(out, "  warning: report: %v\n", err)
	} else {
		res.Verdict, res.VerdictSummary, res.Report = string(a.Verdict.Kind), a.Verdict.Summary, dir.File(report.FileName)
		fmt.Fprintf(out, "%s\n", a.Verdict.Summary)
		for _, d := range a.Diagnoses {
			fmt.Fprintf(out, "  diagnosis: %s, from %s\n", d.Title, analysis.Clock(d.From))
		}
		fmt.Fprintf(out, "report: %s\n", res.Report)
	}
	if err := dir.WriteJSON("run.json", res); err != nil {
		return res, err
	}
	fmt.Fprintf(out, "%s: %s\n", res.Status, dir.Path)
	if res.Status != StatusCompleted {
		return res, fmt.Errorf("run %s: %s", res.Status, res.Error)
	}
	return res, nil
}

// runStack brings the stack up, runs the load with its fault timeline and
// progress lines, and collects the data.
func runStack(ctx context.Context, drv driver.Driver, p *plan.Plan, dir *rundir.Dir, res *Result, out io.Writer) error {
	progress := func(s string) { fmt.Fprintf(out, "  %s\n", s) }
	if err := drv.Up(ctx, p, dir, progress); err != nil {
		// Keep the containers' logs and states: they say why.
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		_ = drv.Collect(cctx, dir, time.Time{}, time.Time{})
		return fmt.Errorf("starting the stack: %w", err)
	}

	loadCtx, stopLoad := context.WithCancel(ctx)
	var wg sync.WaitGroup
	var t0 time.Time
	fmt.Fprintf(out, "load: %s, peak %s rps\n", p.Duration, fmtRate(p.Workload.PeakRate))
	lr, err := drv.RunLoad(ctx, func(t time.Time) {
		t0 = t
		ts := t.UTC()
		res.LoadStarted = &ts
		_ = dir.Event("load_start", map[string]any{"peak_rate": p.Workload.PeakRate, "duration": p.Duration.String()})
		wg.Go(func() { timeline(loadCtx, t, p.Faults, drv, dir, out) })
		wg.Go(func() { progressLines(loadCtx, t, drv, out) })
	})
	stopLoad()
	wg.Wait()
	if t0.IsZero() {
		return err
	}
	end := time.Now().UTC()
	res.LoadEnded = &end
	res.Load = lr
	fields := map[string]any{}
	if lr != nil {
		fields = map[string]any{"requests": lr.Requests, "dropped_iterations": lr.DroppedIterations, "failed_share": lr.FailedShare, "exit_code": lr.ExitCode}
	}
	_ = dir.Event("load_end", fields)

	// Collect even when the load failed: whatever was measured is evidence.
	// Wait for the last scrapes first.
	fmt.Fprintf(out, "collecting metrics and logs\n")
	time.Sleep(scrapeSettle)
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	if cerr := drv.Collect(cctx, dir, t0, end); cerr != nil {
		res.Warnings = append(res.Warnings, "collect: "+cerr.Error())
		fmt.Fprintf(out, "  warning: collect: %v\n", cerr)
	}
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}
	if lr.ExitCode != 0 && lr.ExitCode != 99 {
		return fmt.Errorf("the load generator exited with code %d", lr.ExitCode)
	}
	return nil
}

// timeline injects each fault at its time and ends it after its duration.
// Faults still active when the load ends are ended then.
func timeline(ctx context.Context, t0 time.Time, faults []*plan.Fault, drv driver.Driver, dir *rundir.Dir, out io.Writer) {
	sorted := slices.Clone(faults)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].At < sorted[j].At })
	var wg sync.WaitGroup
	for _, f := range sorted {
		wg.Go(func() {
			if !sleepUntil(ctx, t0.Add(f.At.D())) {
				return
			}
			desc := describeFault(f)
			undo, err := drv.Inject(ctx, f)
			if err != nil {
				_ = dir.Event("fault_error", map[string]any{"index": f.Index, "inject": f.Inject, "target": f.Target, "error": err.Error()})
				fmt.Fprintf(out, "[%s] fault failed: %s: %v\n", clock(time.Since(t0)), desc, err)
			} else {
				_ = dir.Event("fault_start", map[string]any{"index": f.Index, "inject": f.Inject, "target": f.Target, "detail": desc})
				fmt.Fprintf(out, "[%s] fault: %s\n", clock(time.Since(t0)), desc)
			}
			if undo == nil {
				return
			}
			if f.For != nil {
				sleepUntil(ctx, t0.Add(f.At.D()+f.For.D()))
			} else {
				<-ctx.Done()
			}
			uctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			if err := undo(uctx); err != nil {
				_ = dir.Event("fault_end_error", map[string]any{"index": f.Index, "error": err.Error()})
				return
			}
			_ = dir.Event("fault_end", map[string]any{"index": f.Index, "inject": f.Inject, "target": f.Target, "detail": desc})
			if ctx.Err() == nil {
				fmt.Fprintf(out, "[%s] fault ended: %s\n", clock(time.Since(t0)), desc)
			}
		})
	}
	wg.Wait()
}

func describeFault(f *plan.Fault) string {
	s := f.Inject + " on " + f.Target
	switch f.Inject {
	case "latency":
		s = fmt.Sprintf("latency +%d ms on connections into %s", f.AddMs, f.Target)
		if f.JitterMs > 0 {
			s += fmt.Sprintf(" (±%d ms)", f.JitterMs)
		}
	case "kill", "restart", "pause":
		s = fmt.Sprintf("%s %s", f.Inject, strings.Join(f.Containers, ", "))
	}
	if f.For != nil {
		s += " for " + f.For.D().String()
	}
	return s
}

// progressLines prints a progress line every 10 seconds. Every number comes from
// the run's Prometheus, except offered load, which is the profile.
func progressLines(ctx context.Context, t0 time.Time, drv driver.Driver, out io.Writer) {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		elapsed := time.Since(t0)
		qctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		s, err := drv.Sample(qctx, elapsed)
		cancel()
		if err != nil || s == nil {
			continue
		}
		parts := []string{"offered " + fmtRate(s.Offered) + " rps"}
		parts = append(parts, "served "+optional(s.Served, func(v float64) string { return fmtRate(v) + " rps" }))
		parts = append(parts, "p99 "+optional(s.P99Ms, fmtMs))
		parts = append(parts, "errors "+optional(s.Errors, func(v float64) string { return strconv.FormatFloat(v*100, 'f', 1, 64) + "%" }))
		if len(s.CPU) > 0 {
			var cpus []string
			for _, svc := range sortedKeys(s.CPU) {
				cpus = append(cpus, fmt.Sprintf("%s %.0f%%", svc, s.CPU[svc]*100))
			}
			parts = append(parts, "cpu "+strings.Join(cpus, " "))
		}
		fmt.Fprintf(out, "[%s] %s\n", clock(elapsed), strings.Join(parts, " · "))
	}
}

func optional(v *float64, f func(float64) string) string {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return "n/a"
	}
	return f(*v)
}

func sleepUntil(ctx context.Context, t time.Time) bool {
	d := time.Until(t)
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// fittingScale finds the largest of a few scales whose plan fits the host's
// CPUs, or 0 if none does.
func fittingScale(d *design.Design, opt Options, cpus int) float64 {
	for _, s := range []float64{0.75, 0.5, 0.25, 0.1} {
		p, err := plan.Build(d, plan.Options{RunID: "x", Scale: s, SyntheticImage: opt.SyntheticImage})
		if err == nil && p.TotalCPU() <= float64(cpus) {
			return s
		}
	}
	return 0
}

// InvalidDesignError carries a design's diagnostics.
type InvalidDesignError struct{ Diagnostics design.Diagnostics }

func (e *InvalidDesignError) Error() string {
	return fmt.Sprintf("the design has %d error(s); run loadl validate", len(e.Diagnostics.Errors()))
}

func clock(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%02d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

func fmtRate(f float64) string {
	s := strconv.FormatFloat(math.Round(f), 'f', 0, 64)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func fmtMs(v float64) string {
	if v >= 1000 {
		return strconv.FormatFloat(v/1000, 'f', 2, 64) + " s"
	}
	return strconv.FormatFloat(v, 'f', 0, 64) + " ms"
}

func fmtF(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
