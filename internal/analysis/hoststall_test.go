package analysis

import (
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/analysis/analysistest"
)

// A host disk stall: db reads nothing and commits only the exporter's
// transactions for a few seconds while queries wait on its disk. It is
// found, its windows flagged, the timeline and the verdict say it was the
// machine; one such second alone is not a stall.
func TestHostStalls(t *testing.T) {
	w := analysistest.Healthy()
	w.DiskStall = func(el float64) bool { return el > 62 && el <= 65 }
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	if len(a.HostStalls) != 1 {
		t.Fatalf("host stalls = %+v, want one", a.HostStalls)
	}
	h := a.HostStalls[0]
	if h.Kind != "disk" || h.At < 60 || h.At > 64 || h.Seconds < DiskStallSeconds || h.Services[0] != "db" || h.Waiting != 40 {
		t.Errorf("stall = %+v", h)
	}
	if a.Windows[6].HostStallS == 0 || a.Windows[5].HostStallS != 0 || a.Windows[7].HostStallS != 0 {
		t.Errorf("flagged windows: 5 %v, 6 %v, 7 %v", a.Windows[5].HostStallS, a.Windows[6].HostStallS, a.Windows[7].HostStallS)
	}
	found := false
	for _, e := range a.Timeline {
		found = found || (e.Kind == "host-stall" && strings.Contains(e.Detail, "the machine, not the design"))
	}
	if !found {
		t.Errorf("timeline has no host stall: %+v", a.Timeline)
	}
	if a.Verdict.Kind != VerdictHeld || len(a.Verdict.Caveats) != 1 || !strings.Contains(a.Verdict.Caveats[0], "Docker host's disk stopped") {
		t.Errorf("verdict %s, caveats %q", a.Verdict.Kind, a.Verdict.Caveats)
	}

	w.DiskStall = func(el float64) bool { return el == 63 }
	if a := analyze(t, analysistest.WriteRun(t, w, 1)); len(a.HostStalls) != 0 {
		t.Errorf("one second made a stall: %+v", a.HostStalls)
	}
}

// A stall nothing else explains, with db throttled in bursts below its
// limit, is named CPU bursts; the same stall under a fault is the fault's.
func TestCPUBursts(t *testing.T) {
	w := analysistest.Healthy()
	stall := func(el float64) bool { return el > 60 && el <= 70 }
	w.P99 = func(el float64) float64 {
		if stall(el) {
			return 0.6
		}
		return 0.005
	}
	w.DBCPU = func(float64) float64 { return 0.3 }
	w.DBThrottled = func(el float64) float64 {
		if stall(el) {
			return 1
		}
		return 0
	}
	a := analyze(t, analysistest.WriteRun(t, w, 1))
	var d *Diagnosis
	for _, x := range a.Diagnoses {
		if x.Rule == "cpu-bursts" {
			d = x
		}
	}
	if d == nil || d.comp != "db" || !strings.Contains(d.Summary, "throttled for 10.0 s") {
		t.Fatalf("diagnoses %+v", a.Diagnoses)
	}
	w.Events = []analysistest.Event{{At: 62, Kind: "fault_start", Fields: map[string]any{"detail": "pause db for 2s"}}}
	for _, x := range analyze(t, analysistest.WriteRun(t, w, 1)).Diagnoses {
		if x.Rule == "cpu-bursts" {
			t.Errorf("named a fault's stall: %s", x.Summary)
		}
	}
}
