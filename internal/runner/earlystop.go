package runner

import (
	"math"
	"time"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/plan"
)

// EarlyStopAfter is how long the load goes on once the live check has
// confirmed a break, so the report has the break's aftermath to diagnose:
// the pool and knee rules need a few windows past it.
var EarlyStopAfter = 60 * time.Second

// earlyStopMargin: a stop that would come less than this before the
// profile ends isn't worth it.
const earlyStopMargin = 20 * time.Second

// breakWatch applies the verdict's break rule to the progress samples as
// they come, one window per sample. The report's own verdict comes later,
// from the exported metrics; this only decides when to stop the load.
type breakWatch struct {
	p      *plan.Plan
	ws     []*analysis.Window
	stopAt time.Duration // 0: no break confirmed yet; -1: confirmed too late to stop
	stop   *analysis.EarlyStop
}

func newBreakWatch(p *plan.Plan) *breakWatch { return &breakWatch{p: p} }

// add takes the sample at elapsed (the end of the window it covers). It
// returns when to stop the load, once a break is confirmed and the stop
// comes well before the profile ends.
func (b *breakWatch) add(elapsed time.Duration, s *driver.Sample) (time.Duration, bool) {
	if b.stopAt != 0 {
		return b.stopAt, b.stopAt > 0
	}
	if s == nil || s.Served == nil {
		return 0, false
	}
	win := float64(analysis.WindowSeconds)
	start := math.Max(0, math.Round(elapsed.Seconds()/win)*win-win)
	mid := time.Duration((start + win/2) * float64(time.Second))
	b.ws = append(b.ws, &analysis.Window{Index: len(b.ws), Start: start, Offered: b.p.Workload.OfferedRate(mid), RPS: s.Served, ErrorRate: s.Errors, P99: s.P99Ms})
	slo := b.p.Workload.SLOP99Ms
	if slo <= 0 {
		slo = design.DefaultSLOP99Ms
	}
	i := analysis.BreakAt(b.ws, b.p.Workload.PeakRate, slo)
	if i < 0 {
		return 0, false
	}
	stopAt := elapsed + EarlyStopAfter
	if stopAt > b.p.Duration-earlyStopMargin {
		b.stopAt = -1 // the profile ends about as soon; let it
		return 0, false
	}
	b.stopAt = stopAt
	b.stop = &analysis.EarlyStop{BreakAt: b.ws[i].Start, ConfirmedAt: elapsed.Seconds(), At: stopAt.Seconds()}
	return stopAt, true
}
