package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/akinfemi/load-lore/internal/analysis/analysistest"
	"github.com/akinfemi/load-lore/internal/bench"
)

// A run from before network profiles and the speed index: all-proxied,
// no proxied count, unusable for want of a speed index. One with them:
// faulted-proxied and usable, its host-stalled window left out.
func TestBuild(t *testing.T) {
	old := analysistest.WriteRun(t, analysistest.Healthy(), 1)
	r, err := Build(old)
	if err != nil {
		t.Fatal(err)
	}
	c := r.Conditions
	if r.Format != Format || c.Era != EraAllProxied || c.Proxied != nil || c.Network != nil || c.SpeedIndex != nil {
		t.Errorf("old run: format %d, conditions %+v", r.Format, c)
	}
	if r.Fit.Usable || len(r.Fit.Why) != 1 {
		t.Errorf("old run fit = %+v", r.Fit)
	}
	if d := c.Disks["db"]; d.Profile != "host" {
		t.Errorf("db disk = %+v", d)
	}
	if c.CPUPeriods["db"] != 100 {
		t.Errorf("cpu periods = %v", c.CPUPeriods)
	}

	w := analysistest.Healthy()
	w.DiskStall = func(el float64) bool { return el > 62 && el <= 65 }
	w.RunJSON = map[string]any{
		"network":             map[string]any{"profile": "same-zone", "rtt_ms": 0.3, "measured_rtt_ms": 0.31},
		"proxied_connections": 0,
		"host":                map[string]any{"cpus": 8, "speed": bench.Speed{Index: 1000}},
	}
	r, err = Build(analysistest.WriteRun(t, w, 1))
	if err != nil {
		t.Fatal(err)
	}
	c = r.Conditions
	if c.Era != EraFaultedProxied || c.Proxied == nil || *c.Proxied != 0 || c.Network.Profile != "same-zone" || c.SpeedIndex == nil || *c.SpeedIndex != 1000 {
		t.Errorf("new run conditions %+v", c)
	}
	if !r.Fit.Usable {
		t.Errorf("new run fit = %+v", r.Fit)
	}
	if len(r.Excluded) != 1 || r.Excluded[0].Index != 6 {
		t.Fatalf("excluded = %+v", r.Excluded)
	}
	for _, x := range r.Analysis.Windows {
		if x.Index == 6 {
			t.Error("the stalled window is still in the analysis")
		}
	}
}

// Write puts each record under runs/ and keeps one index line per run,
// merged with what the corpus already had.
func TestWrite(t *testing.T) {
	out := t.TempDir()
	a := analysistest.WriteRun(t, analysistest.Healthy(), 1)
	b := analysistest.WriteRun(t, analysistest.Healthy(), 1)
	if _, err := Write(out, []string{a}); err != nil {
		t.Fatal(err)
	}
	res, err := Write(out, []string{a, b, t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 2 || len(res.Skipped) != 1 {
		t.Errorf("written %d, skipped %v", len(res.Written), res.Skipped)
	}
	raw, err := os.ReadFile(filepath.Join(out, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var es []Entry
	if err := json.Unmarshal(raw, &es); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, e := range es {
		ids[e.RunID] = true
		if _, err := os.Stat(filepath.Join(out, e.Path)); err != nil {
			t.Errorf("%s: %v", e.RunID, err)
		}
	}
	if len(es) != len(ids) || len(ids) < 1 {
		t.Errorf("index has %d entries for %d runs", len(es), len(ids))
	}
}

// A CPU-burst stall's windows stay in the record, labelled with the rule.
func TestBurstsLabelled(t *testing.T) {
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
	r, err := Build(analysistest.WriteRun(t, w, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Excluded) != 0 {
		t.Errorf("excluded %+v", r.Excluded)
	}
	found := false
	for _, l := range r.Labels {
		found = found || (l.Index == 6 && slices.Contains(l.Rules, "cpu-bursts"))
	}
	if !found {
		t.Errorf("labels %+v", r.Labels)
	}
}
