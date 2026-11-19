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
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/hoststat"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/power"
	"github.com/akinfemi/load-lore/internal/report"
	"github.com/akinfemi/load-lore/internal/rundir"
	"github.com/akinfemi/load-lore/internal/scenario"
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

	// CoolTo is a speed index to wait for before starting (the first run's
	// in a series, or the run a scenario check compares this one with).
	// Pre-flight re-runs the benchmark until the host is within
	// CoolTolerance of it, for up to CoolLimit. 0 = don't wait.
	CoolTo int
	// Series places the run in a run --repeat series; nil for one run.
	Series *analysis.SeriesRef
	// Sweep places the run in a loadl sweep; nil otherwise.
	Sweep *SweepRef
	// Full runs the whole profile. Otherwise the load stops
	// EarlyStopAfter after the live check confirms a break.
	Full bool
	// Quick shortens the profile to start near an earlier run's break:
	// QuickFrom's, or the latest broken run's of the same scenario and
	// profile (see plan.Quicken).
	Quick     bool
	QuickFrom string
	// OnDir, when set, is called with the run folder once it exists and
	// has its status, as for a dashboard that follows the run.
	OnDir func(dir string)
}

// A machine slows as it heats up, so runs in a series wait for the host to
// get back within CoolTolerance of the first run's speed index, checking
// every coolEvery, for up to CoolLimit.
const (
	CoolTolerance = 0.05
	CoolLimit     = 10 * time.Minute
)

var coolEvery = 30 * time.Second

// Run statuses.
const (
	StatusCompleted   = "completed"
	StatusFailed      = "failed"
	StatusInterrupted = "interrupted"
	StatusTimedOut    = "timed_out"
)

// Result is the run's outcome, also written to run.json.
type Result struct {
	RunID      string  `json:"run_id"`
	Dir        string  `json:"dir"`
	Design     string  `json:"design"`
	DesignPath string  `json:"design_path"` // the file run, absolute
	Status     string  `json:"status"`
	Error      string  `json:"error,omitempty"`
	Scale      float64 `json:"scale"`
	Seed       int64   `json:"seed"`
	ReuseData  bool    `json:"reuse_data"`
	// Network is the network profile the run stood on, as the driver
	// applied and measured it, and Proxied how many connections went
	// through Toxiproxy for faults.
	Network *driver.NetworkResult `json:"network,omitempty"`
	Proxied int                   `json:"proxied_connections"`
	// Disks is each modelled disk's waits, asked and as measured on the
	// device before the stack started.
	Disks []driver.DiskResult `json:"disks,omitempty"`
	// Quick is set on a run whose profile was shortened (--quick).
	Quick       *plan.Quick         `json:"quick,omitempty"`
	Started     time.Time           `json:"started"`
	LoadStarted *time.Time          `json:"load_started,omitempty"`
	LoadEnded   *time.Time          `json:"load_ended,omitempty"`
	Finished    time.Time           `json:"finished"`
	Host        *driver.Host        `json:"host,omitempty"`
	Warnings    []string            `json:"warnings"`
	Notes       []string            `json:"notes"`
	Load        *driver.LoadResult  `json:"load,omitempty"`
	Kept        bool                `json:"kept"`
	Loadl       string              `json:"loadl_version"`
	Series      *analysis.SeriesRef `json:"series,omitempty"`
	Sweep       *SweepRef           `json:"sweep,omitempty"`
	// StoppedEarly is set when the load stopped after a break instead of
	// finishing the profile.
	StoppedEarly *analysis.EarlyStop `json:"stopped_early,omitempty"`

	// From the report, once written: broke, held, inconclusive or no_data.
	Verdict        string `json:"verdict,omitempty"`
	VerdictSummary string `json:"verdict_summary,omitempty"`
	Report         string `json:"report,omitempty"`
}

// SweepRef places a run in a loadl sweep: which case, at which CPU size
// of the component under test, and which of its repeats.
type SweepRef struct {
	ID     string  `json:"id"`
	Case   string  `json:"case"`
	Target string  `json:"target"` // the component under test
	CPU    float64 `json:"cpu"`
	Peak   float64 `json:"peak_rps"` // the top of the ramp
	Index  int     `json:"index"`    // the repeat, from 1
}

// setupAllowance covers seeding, copying data and starting the stack.
const setupAllowance = 15 * time.Minute

// scrapeSettle is how long to wait after the load for the last scrapes.
var scrapeSettle = 3 * time.Second

// coolDown waits until the host speed index is within CoolTolerance of
// target, re-running the benchmark every coolEvery, for up to CoolLimit or
// until ctx ends. It returns the last measurement. Only a slower host
// waits: a faster one has nothing to cool from.
func coolDown(ctx context.Context, drv driver.Driver, p *plan.Plan, target int, sp *bench.Speed, out io.Writer) (*bench.Speed, *driver.Cooldown) {
	c := &driver.Cooldown{Target: target, Tolerance: CoolTolerance, First: sp.Index}
	floor := (1 - CoolTolerance) * float64(target)
	if float64(sp.Index) >= floor {
		c.Reached = true
		return sp, c
	}
	start := time.Now()
	for float64(sp.Index) < floor {
		if time.Since(start) >= CoolLimit {
			c.Waited = time.Since(start).Seconds()
			return sp, c
		}
		fmt.Fprintf(out, "  cooling down: speed index %d, waiting for %d or more (the run it's compared with measured %d; within %s counts)\n", sp.Index, int(math.Ceil(floor)), target, fmtF(CoolTolerance*100)+"%")
		select {
		case <-ctx.Done():
			c.Waited = time.Since(start).Seconds()
			return sp, c
		case <-time.After(coolEvery):
		}
		next, err := drv.Speed(ctx, p)
		if err != nil {
			fmt.Fprintf(out, "  warning: the speed benchmark failed while cooling down: %v\n", err)
			continue
		}
		sp = next
	}
	c.Waited, c.Reached = time.Since(start).Seconds(), true
	return sp, c
}

// Run runs a design. Diagnostics from an invalid design come back as
// *InvalidDesignError.
func Run(parent context.Context, opt Options, newDriver func(context.Context) (driver.Driver, error)) (_ *Result, runErr error) {
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

	// A laptop that idle-sleeps mid-run freezes the stack; keep it awake.
	if stop, ok := power.KeepAwake(); ok {
		defer stop()
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
	designPath, _ := filepath.Abs(opt.DesignPath)
	res := &Result{RunID: id, Dir: dir.Path, Design: d.Name, DesignPath: designPath, Started: started.UTC(), Seed: opt.Seed, ReuseData: opt.ReuseData, Loadl: version.Version, Warnings: []string{}, Series: opt.Series, Sweep: opt.Sweep}
	if err := dir.Write("design.yaml", raw); err != nil {
		return nil, err
	}

	sc, inScenario := scenario.ForDesign(opt.DesignPath)
	popt := plan.Options{RunID: id, Scale: opt.Scale, Seed: opt.Seed, SyntheticImage: opt.SyntheticImage, ReuseData: opt.ReuseData}
	if inScenario && sc.Meta.Load != nil {
		popt.PreallocVUs = sc.Meta.Load.PreallocVUs
	}
	p, err := plan.Build(d, popt)
	if err != nil {
		return nil, err
	}
	if opt.Quick {
		src, err := findQuickSource(root, opt.QuickFrom, designPath, d.Name, p)
		if err != nil {
			return nil, err
		}
		ok, err := p.Quicken(src.id, src.breakRPS)
		if err != nil {
			return nil, err
		}
		if !ok {
			fmt.Fprintf(out, "quick: %s broke at %s rps, too early in the profile to skip much; running the whole profile\n", src.id, fmtF(src.breakRPS))
		}
	}
	res.Scale, res.Notes = p.Scale, p.Notes
	res.Quick = p.Quick
	res.Network = &driver.NetworkResult{Profile: p.Network.Profile, RTTMs: p.Network.RTTMs, DelayUs: p.Network.DelayUs()}
	res.Proxied = len(p.Proxied())
	if err := dir.WriteJSON("plan.json", p); err != nil {
		return nil, err
	}
	st := newStatusFile(dir, Status{RunID: id, Design: d.Name, PID: os.Getpid(), Log: detachedLog(), Phase: PhasePreflight, Started: started.UTC(), ProfileS: p.Duration.Seconds()})
	if opt.OnDir != nil {
		opt.OnDir(dir.Path)
	}
	defer func() {
		// A run that ends before its report (pre-flight failed, say) is
		// finished too, with what went wrong.
		st.update(func(s *Status) {
			if s.Phase == PhaseFinished {
				return
			}
			s.Phase, s.Status = PhaseFinished, StatusFailed
			if runErr != nil {
				s.Error = runErr.Error()
			}
		})
	}()
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
	if opt.CoolTo > 0 && pf.Host.Speed != nil {
		pf.Host.Speed, pf.Host.Cooled = coolDown(parent, drv, p, opt.CoolTo, pf.Host.Speed, out)
		if c := pf.Host.Cooled; !c.Reached {
			pf.Warnings = append(pf.Warnings, fmt.Sprintf("the host's speed index was still %d after waiting %s, against %d for the run this one is compared with: it may break at a lower load because the host is slower, not the design",
				pf.Host.Speed.Index, CoolLimit, c.Target))
		}
	}
	res.Host = &pf.Host
	if sp := pf.Host.Speed; sp != nil {
		fmt.Fprintf(out, "  host: %d CPUs, %d MiB, speed index %d (reference machine 1000)\n", pf.Host.CPUs, pf.Host.MemoryBytes>>20, sp.Index)
	}
	res.Warnings = append(res.Warnings, pf.Warnings...)
	if share := p.Infra.ProxyShare(); share > plan.ProxyShareLimit {
		res.Warnings = append(res.Warnings, fmt.Sprintf("Toxiproxy's estimated peak (about %s proxied calls/s) is %.0f%% of its %s CPU budget, above the %.0f%% it's sized for: it may be CPU-throttled in bursts and make the run inconclusive; a lower peak load keeps it under",
			fmtRate(p.Infra.ExpectedOps), share*100, fmtF(p.Infra.ProxyCPU), plan.ProxyShareLimit*100))
	}
	if p.TotalCPU() > float64(pf.Host.CPUs) {
		if s := fittingScale(d, opt, pf.Host.CPUs); s > 0 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("--scale %s fits this host's %d CPUs", fmtF(s), pf.Host.CPUs))
		}
	}
	if inScenario {
		res.Warnings = append(res.Warnings, scenarioHostWarnings(sc, p.Scale, pf.Host)...)
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

	st.phase(PhaseStarting)
	stackErr := runStack(ctx, drv, p, dir, res, out, !opt.Full, st)

	switch {
	case stackErr == nil:
		res.Status = StatusCompleted
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.Status = StatusTimedOut
		res.Error = fmt.Sprintf("the run didn't finish within %s", timeout)
	case ctx.Err() != nil:
		res.Status = StatusInterrupted
		res.Error = "interrupted"
	default:
		res.Status = StatusFailed
		res.Error = stackErr.Error()
	}

	stopSignals()
	if opt.Keep && res.Status != StatusFailed {
		res.Kept = true
		fmt.Fprintf(out, "keeping the stack up (--keep); remove it with `loadl down`\n")
	} else {
		st.phase(PhaseTeardown)
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
	st.update(func(s *Status) {
		s.Phase, s.Status, s.Error = PhaseFinished, res.Status, res.Error
		s.Verdict, s.VerdictSummary, s.Report = res.Verdict, res.VerdictSummary, res.Report
	})
	fmt.Fprintf(out, "%s: %s\n", res.Status, dir.Path)
	if res.Status != StatusCompleted {
		return res, fmt.Errorf("run %s: %s", res.Status, res.Error)
	}
	return res, nil
}

// runStack brings the stack up, runs the load with its fault timeline and
// progress lines, and collects the data.
func runStack(ctx context.Context, drv driver.Driver, p *plan.Plan, dir *rundir.Dir, res *Result, out io.Writer, earlyStop bool, st *statusFile) error {
	progress := func(s string) { fmt.Fprintf(out, "  %s\n", s) }
	if err := drv.Up(ctx, p, dir, progress); err != nil {
		// Keep the containers' logs and states: they say why.
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		_ = drv.Collect(cctx, dir, time.Time{}, time.Time{})
		return fmt.Errorf("starting the stack: %w", err)
	}
	if dr, ok := drv.(driver.DiskReporter); ok {
		res.Disks = dr.DisksApplied()
		for _, dk := range res.Disks {
			switch {
			case dk.Unmeasured != "":
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s's disk: its waits couldn't be measured (%s), so they are dm-delay's own, which come out shorter than asked", dk.Container, dk.Unmeasured))
			default:
				progress(fmt.Sprintf("%s's disk: %d ms per read and %d per write asked, reads %s ms and flushed writes %s ms slower measured", dk.Container, dk.ReadMs, dk.WriteMs, fmtF(dk.AddedReadMs), fmtF(dk.AddedSyncMs)))
			}
		}
	}
	if nr, ok := drv.(driver.NetworkReporter); ok {
		if n := nr.NetworkApplied(); n != nil {
			res.Network = n
			switch {
			case n.Uncalibrated != "" && n.RTTMs > 0:
				res.Warnings = append(res.Warnings, fmt.Sprintf("network profile %s: the round trip couldn't be measured (%s), so it is netem's own, which on some hosts runs long", n.Profile, n.Uncalibrated))
			case n.RTTMs > 0:
				progress(fmt.Sprintf("network profile %s: %s ms round trip asked, %s ms measured between two containers", n.Profile, fmtF(n.RTTMs), fmtF(n.MeasuredMs)))
			}
		}
	}

	loadCtx, stopLoad := context.WithCancel(ctx)
	var wg sync.WaitGroup
	var t0 time.Time
	var host *hoststat.Sampler
	var stopped *analysis.EarlyStop // written by the progress goroutine, read after wg.Wait
	fmt.Fprintf(out, "load: %s, peak %s rps\n", p.Duration, fmtRate(p.Workload.PeakRate))
	lr, err := drv.RunLoad(ctx, func(t time.Time) {
		t0 = t
		ts := t.UTC()
		res.LoadStarted = &ts
		st.update(func(s *Status) { s.Phase, s.LoadStarted = PhaseLoad, &ts })
		_ = dir.Event("load_start", map[string]any{"peak_rate": p.Workload.PeakRate, "duration": p.Duration.String()})
		wg.Go(func() { timeline(loadCtx, t, p.Faults, drv, dir, out) })
		wg.Go(func() { autoscale(loadCtx, t, p, drv, dir, out) })
		var watch *breakWatch
		if earlyStop {
			watch = newBreakWatch(p)
		}
		wg.Go(func() { stopped = progressLines(loadCtx, t, drv, out, watch, dir, st) })
		if res.Host != nil && res.Host.HostCPU == driver.HostCPUSampled {
			host = hoststat.Start(loadCtx, time.Second)
		}
	})
	stopLoad()
	wg.Wait()
	if t0.IsZero() {
		return err
	}
	end := time.Now().UTC()
	res.LoadEnded = &end
	res.Load = lr
	res.StoppedEarly = stopped
	fields := map[string]any{}
	if lr != nil {
		fields = map[string]any{"requests": lr.Requests, "dropped_iterations": lr.DroppedIterations, "failed_share": lr.FailedShare, "exit_code": lr.ExitCode}
	}
	_ = dir.Event("load_end", fields)

	st.phase(PhaseCollecting)
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
	if host != nil {
		host.Wait()
		if b, herr := host.JSON(); herr == nil && b != nil {
			herr = dir.Write(filepath.Join("metrics", hoststat.Metric+".json"), b)
			if herr != nil {
				res.Warnings = append(res.Warnings, "host CPU samples: "+herr.Error())
			}
		}
	}
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}
	if lr.ExitCode != 0 && lr.ExitCode != 99 && (stopped == nil || lr.ExitCode != driver.K6Stopped) {
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
	case "loss":
		s = fmt.Sprintf("loss %g%% of the packets %s sends from port %d (its replies)", f.Percent, f.Target, f.Port)
	case "partition":
		s = fmt.Sprintf("partition between %s and %s", f.Between[0], f.Between[1])
	case "disk_slow":
		s = fmt.Sprintf("disk_slow +%d ms on every write and fsync of %s's disk", f.AddMs, f.Target)
	case "kill", "restart", "pause":
		s = fmt.Sprintf("%s %s", f.Inject, strings.Join(f.Containers, ", "))
	}
	if f.For != nil {
		s += " for " + f.For.D().String()
	}
	return s
}

// progressEvery is how often progressLines samples the run.
var progressEvery = 10 * time.Second

// progressLines prints a progress line every 10 seconds. Every number comes
// from the run's Prometheus, except offered load, which is the profile.
// With a watch, each sample also goes to the live break check, and once
// it confirms a break the load stops EarlyStopAfter later. It returns the
// early stop, if there was one.
func progressLines(ctx context.Context, t0 time.Time, drv driver.Driver, out io.Writer, watch *breakWatch, dir *rundir.Dir, st *statusFile) *analysis.EarlyStop {
	tick := time.NewTicker(progressEvery)
	defer tick.Stop()
	var stopTimer <-chan time.Time
	announced := false
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-stopTimer:
			es := watch.stop
			_ = dir.Event("load_stop", map[string]any{"break_at_s": es.BreakAt, "confirmed_at_s": es.ConfirmedAt, "at_s": es.At})
			fmt.Fprintf(out, "[%s] stopping the load, %s after the break was confirmed (--full runs the whole profile)\n", clock(time.Since(t0)), EarlyStopAfter)
			sctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			err := drv.StopLoad(sctx)
			cancel()
			if err != nil {
				fmt.Fprintf(out, "  warning: stopping the load: %v; it runs to the end of the profile\n", err)
				stopTimer = nil
				continue
			}
			return es
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
		if len(s.QueueJobs) > 0 {
			var queues []string
			for _, q := range sortedKeys(s.QueueJobs) {
				queues = append(queues, fmt.Sprintf("%s %.0f", q, s.QueueJobs[q]))
			}
			parts = append(parts, "jobs queued "+strings.Join(queues, " "))
		}
		fmt.Fprintf(out, "[%s] %s\n", clock(elapsed), strings.Join(parts, " · "))
		st.update(func(x *Status) { x.ElapsedS, x.Sample = elapsed.Seconds(), s })
		_ = dir.AppendJSON(rundir.ProgressFile, ProgressLine{ElapsedS: elapsed.Seconds(), Sample: s})
		if watch == nil || announced {
			continue
		}
		if at, ok := watch.add(elapsed, s); ok {
			announced = true
			es := watch.stop
			st.update(func(x *Status) { x.BreakAtS, x.BreakConfirmedS, x.StopsAtS = &es.BreakAt, &es.ConfirmedAt, &es.At })
			fmt.Fprintf(out, "[%s] break confirmed (from %s); the load stops at %s\n", clock(elapsed), clock(time.Duration(es.BreakAt*float64(time.Second))), clock(at))
			stopTimer = time.After(time.Until(t0.Add(at)))
		}
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

// scenarioHostWarnings compares the host with what the scenario's meta.yaml
// says it needs at --scale 1, scaled. Without it the scenario's numbers may
// not match its README even when the design fits.
func scenarioHostWarnings(sc *scenario.Scenario, scale float64, h driver.Host) []string {
	need := sc.Meta.Host
	var out []string
	if cpus := need.CPUs * scale; cpus > float64(h.CPUs) {
		fit := math.Floor(float64(h.CPUs)/need.CPUs*20) / 20
		out = append(out, fmt.Sprintf("scenario %s needs %s CPUs available to Docker at --scale %s (%s CPUs at --scale 1, from its meta.yaml), but Docker has %d; try --scale %s",
			sc.Name, fmtF(cpus), fmtF(scale), fmtF(need.CPUs), h.CPUs, fmtF(fit)))
	}
	if mem := int64(float64(need.Memory) * scale); mem > h.MemoryBytes {
		out = append(out, fmt.Sprintf("scenario %s needs %d MiB of memory available to Docker at --scale %s (from its meta.yaml), but Docker has %d MiB",
			sc.Name, mem>>20, fmtF(scale), h.MemoryBytes>>20))
	}
	if sp := h.Speed; sp != nil && need.Speed > 0 && float64(sp.Index) < SlowHostShare*float64(need.Speed) {
		out = append(out, fmt.Sprintf("this host's speed index is %d, against %d on the machine scenario %s was checked on (%s): CPU limits are fractions of this host's cores, so expect it to break at a lower load than its README describes; the lessons are in the relative results",
			sp.Index, need.Speed, sc.Name, need.Reference))
	}
	return out
}

// SlowHostShare is how much slower than a scenario's reference machine a
// host can be before pre-flight warns.
const SlowHostShare = 0.75

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
