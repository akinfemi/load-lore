package runner

import (
	"testing"
	"time"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/plan"
)

func watchPlan(t *testing.T) *plan.Plan {
	t.Helper()
	d, diags := design.Load([]byte(longDesignSrc))
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	p, err := plan.Build(d, plan.Options{RunID: "watch", Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func sample(served, errs, p99 float64) *driver.Sample {
	return &driver.Sample{Offered: 10, Served: &served, Errors: &errs, P99Ms: &p99}
}

// feed adds s at each 10 s from from to to, and reports whether the last
// add stops the load.
func feed(b *breakWatch, from, to int, s *driver.Sample) bool {
	var ok bool
	for i := from; i <= to; i++ {
		_, ok = b.add(time.Duration(i)*10*time.Second, s)
	}
	return ok
}

func TestBreakWatch(t *testing.T) {
	p := watchPlan(t) // 10 rps for 10 minutes, 500 ms SLO

	b := newBreakWatch(p)
	if feed(b, 1, 30, sample(10, 0, 5)) {
		t.Error("a healthy run stopped")
	}

	// Errors in 2 windows in a row break it; the stop comes EarlyStopAfter
	// after the second.
	b = newBreakWatch(p)
	feed(b, 1, 10, sample(10, 0, 5))
	if _, ok := b.add(110*time.Second, sample(8, 0.2, 5)); ok {
		t.Error("one window of errors is a stall, not a break")
	}
	at, ok := b.add(120*time.Second, sample(8, 0.2, 5))
	if !ok || at != 120*time.Second+EarlyStopAfter || b.stop == nil || b.stop.BreakAt != 100 || b.stop.ConfirmedAt != 120 {
		t.Errorf("stop at %s, %v, %+v", at, ok, b.stop)
	}

	// p99 alone needs 3 windows at 10x the low-load p99 and the SLO.
	b = newBreakWatch(p)
	feed(b, 1, 10, sample(10, 0, 5))
	if feed(b, 11, 12, sample(10, 0, 600)) {
		t.Error("two slow windows stopped the run")
	}
	if _, ok := b.add(130*time.Second, sample(10, 0, 600)); !ok {
		t.Error("three slow windows didn't")
	}

	// A break the profile ends about as soon as the stop would: no stop.
	b = newBreakWatch(p)
	feed(b, 1, 56, sample(10, 0, 5))
	if feed(b, 57, 58, sample(8, 0.2, 5)) {
		t.Error("stopped a run that ends anyway")
	}
}
