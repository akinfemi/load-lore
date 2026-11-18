// Package export writes runs to a corpus: one compact record per run, for
// the simulator to be fitted and tested on. Each record is the run's
// design, plan and analysis, re-analyzed with the current rules, with the
// conditions it stood on (network and disk profiles, connections through
// Toxiproxy, a quick start, the host's speed) and whether fitting may use
// it. Windows a host stall touched are left out: the machine, not the
// design.
package export

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/runner"
	"github.com/akinfemi/load-lore/internal/version"
)

// Format is the record layout's version, raised when a field changes
// meaning or goes.
const Format = 1

// DefaultDir is where export writes when no corpus is named.
const DefaultDir = ".loadlore/corpus"

// Era names how a run's connections reached their targets, since fitting
// keeps the two apart: before rules version 18 every connection went
// through Toxiproxy, with no network delay; since, only faulted ones do,
// over a network profile.
const (
	EraAllProxied     = "all-proxied"
	EraFaultedProxied = "faulted-proxied"
)

// Record is one run in the corpus.
type Record struct {
	Format     int        `json:"format"`
	RunID      string     `json:"run_id"`
	Design     string     `json:"design"`
	Status     string     `json:"status"`
	ExportedAt time.Time  `json:"exported_at"`
	Loadl      string     `json:"loadl_version"`     // the loadl that exported it
	RunLoadl   string     `json:"run_loadl_version"` // the loadl that ran it
	Rules      string     `json:"rules_version"`
	Conditions Conditions `json:"conditions"`
	Fit        Fit        `json:"fit"`
	Excluded   []Excluded `json:"excluded_windows"`
	// Labels name the diagnoses that cover each window, such as
	// cpu-bursts: the container's own behaviour, kept and labelled, where
	// a host stall's windows are left out.
	Labels     []Label             `json:"window_labels"`
	Series     *analysis.SeriesRef `json:"series,omitempty"`
	Sweep      *runner.SweepRef    `json:"sweep,omitempty"`
	DesignYAML string              `json:"design_yaml"`
	Plan       *plan.Plan          `json:"plan"`
	Analysis   *analysis.Analysis  `json:"analysis"`
}

// Conditions are what the run stood on besides its design.
type Conditions struct {
	Era     string                `json:"era"`
	Network *driver.NetworkResult `json:"network"` // nil before network profiles
	// Proxied is how many connections went through Toxiproxy; nil for a
	// run from before it was counted.
	Proxied *int        `json:"proxied_connections"`
	Quick   *plan.Quick `json:"quick"` // nil for a run of the whole profile
	// Disks is each Postgres's disk: a profile's name (laptop is the
	// Docker host's own disk, named), "host" for that disk with no profile
	// named, or "custom" for waits and caps set by hand.
	Disks map[string]Disk `json:"disks"`
	// CPUPeriods is each service's CFS period, in ms.
	CPUPeriods map[string]int `json:"cpu_period_ms"`
	Scale      float64        `json:"scale"`
	Seed       int64          `json:"seed"`
	Host       *driver.Host   `json:"host"`
	SpeedIndex *int           `json:"speed_index"`
}

// Disk is one Postgres's data disk.
type Disk struct {
	Profile string     `json:"profile"`
	Model   *plan.Disk `json:"model,omitempty"` // waits and caps, when modelled
}

// Fit says whether fitting may use the run, and why not.
type Fit struct {
	Usable bool     `json:"usable"`
	Why    []string `json:"why,omitempty"`
}

// Excluded is a window left out of the record's analysis.
type Excluded struct {
	Index int     `json:"index"`
	Start float64 `json:"start_s"`
	Why   string  `json:"why"`
}

// Label is the rules of the diagnoses that cover one window.
type Label struct {
	Index int      `json:"index"`
	Start float64  `json:"start_s"`
	Rules []string `json:"rules"`
}

// Entry is one line of the corpus index.
type Entry struct {
	RunID      string   `json:"run_id"`
	Design     string   `json:"design"`
	Verdict    string   `json:"verdict"`
	BreakAt    *float64 `json:"break_at_s,omitempty"`
	BreakRPS   *float64 `json:"break_offered_rps,omitempty"`
	MaxRPS     *float64 `json:"max_rps,omitempty"`
	Rules      string   `json:"rules_version"`
	Era        string   `json:"era"`
	Network    string   `json:"network,omitempty"`
	Quick      bool     `json:"quick,omitempty"`
	SpeedIndex *int     `json:"speed_index,omitempty"`
	Usable     bool     `json:"usable"`
	Sweep      string   `json:"sweep,omitempty"`
	Excluded   int      `json:"excluded_windows,omitempty"`
	Path       string   `json:"path"`
}

// Build makes the record of the run in dir, analyzing it afresh.
func Build(dir string) (*Record, error) {
	var res runner.Result
	b, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		return nil, fmt.Errorf("%s isn't a run folder: %w", dir, err)
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("%s/run.json: %w", dir, err)
	}
	// Older runs: proxied_connections came with network profiles.
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(b, &raw)

	a, err := analysis.Analyze(dir)
	if err != nil {
		return nil, err
	}
	if a.Verdict.Kind == analysis.VerdictNoData {
		return nil, ErrNoData
	}
	r := &Record{
		Format: Format, RunID: a.RunID, Design: a.Name, Status: a.Status,
		ExportedAt: time.Now().UTC(), Loadl: version.Version, RunLoadl: res.Loadl,
		Rules: analysis.RuleVersion, Series: res.Series, Sweep: res.Sweep,
		DesignYAML: a.DesignYAML, Plan: a.Plan, Excluded: []Excluded{}, Labels: []Label{},
	}
	c := Conditions{Era: EraAllProxied, Network: res.Network, Quick: res.Quick, Scale: res.Scale, Seed: res.Seed, Host: res.Host,
		Disks: map[string]Disk{}, CPUPeriods: map[string]int{}}
	if res.Network != nil {
		c.Era = EraFaultedProxied
	}
	if _, ok := raw["proxied_connections"]; ok && res.Network != nil {
		c.Proxied = &res.Proxied
	}
	if res.Host != nil && res.Host.Speed != nil {
		c.SpeedIndex = &res.Host.Speed.Index
	}
	for _, s := range a.Plan.Services {
		if s.CPUPeriod > 0 {
			c.CPUPeriods[s.Name] = s.CPUPeriod
		}
		if s.Kind != "postgres" {
			continue
		}
		named := ""
		if ds, ok := a.Design.Services[s.Name]; ok && ds.Disk != nil && ds.Disk.Profile != nil {
			named = *ds.Disk.Profile
		}
		switch {
		case s.Disk == nil && named != "":
			c.Disks[s.Name] = Disk{Profile: named}
		case s.Disk == nil:
			c.Disks[s.Name] = Disk{Profile: "host"}
		case s.Disk.Profile != "":
			c.Disks[s.Name] = Disk{Profile: s.Disk.Profile, Model: s.Disk}
		default:
			c.Disks[s.Name] = Disk{Profile: "custom", Model: s.Disk}
		}
	}
	r.Conditions = c

	// Leave out the windows a host stall touched.
	var kept []*analysis.Window
	for _, w := range a.Windows {
		if w.HostStallS > 0 {
			r.Excluded = append(r.Excluded, Excluded{Index: w.Index, Start: w.Start, Why: fmt.Sprintf("a host disk stall covered %.0f s of it", w.HostStallS)})
			continue
		}
		kept = append(kept, w)
	}
	a.Windows = kept
	r.Analysis = a
	for _, w := range kept {
		var rules []string
		for _, d := range a.Diagnoses {
			if d.From < w.Start+analysis.WindowSeconds && w.Start < d.To && !slices.Contains(rules, d.Rule) {
				rules = append(rules, d.Rule)
			}
		}
		if len(rules) > 0 {
			r.Labels = append(r.Labels, Label{Index: w.Index, Start: w.Start, Rules: rules})
		}
	}

	r.Fit = Fit{Usable: true}
	if a.Status != runner.StatusCompleted {
		r.Fit.Why = append(r.Fit.Why, "the run "+a.Status)
	}
	if a.Verdict.Kind == analysis.VerdictInconclusive {
		for _, why := range a.Verdict.Inconclusive {
			r.Fit.Why = append(r.Fit.Why, "inconclusive: "+why)
		}
	}
	if c.SpeedIndex == nil {
		r.Fit.Why = append(r.Fit.Why, "no host speed index: from before it was measured")
	}
	r.Fit.Usable = len(r.Fit.Why) == 0
	return r, nil
}

// ErrNoData is a run whose load never started: nothing to export.
var ErrNoData = errors.New("the load never started")

// Result is what an export did.
type Result struct {
	Dir      string            `json:"dir"`
	Written  []Entry           `json:"written"`
	Skipped  map[string]string `json:"skipped"` // run id -> why
	Usable   int               `json:"usable"`
	Unusable int               `json:"unusable"`
}

// Write exports the runs in dirs to the corpus at out: runs/<id>.json
// each, and index.json listing every record in the corpus, these and the
// ones already there.
func Write(out string, dirs []string) (*Result, error) {
	if err := os.MkdirAll(filepath.Join(out, "runs"), 0o755); err != nil {
		return nil, err
	}
	res := &Result{Dir: out, Written: []Entry{}, Skipped: map[string]string{}}
	index := map[string]Entry{}
	if b, err := os.ReadFile(filepath.Join(out, "index.json")); err == nil {
		var es []Entry
		if err := json.Unmarshal(b, &es); err != nil {
			return nil, fmt.Errorf("%s/index.json: %w", out, err)
		}
		for _, e := range es {
			index[e.RunID] = e
		}
	}
	for _, dir := range dirs {
		r, err := Build(dir)
		if err != nil {
			res.Skipped[filepath.Base(dir)] = err.Error()
			continue
		}
		b, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		rel := filepath.Join("runs", r.RunID+".json")
		if err := writeAtomic(filepath.Join(out, rel), b); err != nil {
			return nil, err
		}
		e := entry(r, rel)
		index[r.RunID] = e
		res.Written = append(res.Written, e)
		if r.Fit.Usable {
			res.Usable++
		} else {
			res.Unusable++
		}
	}
	es := make([]Entry, 0, len(index))
	for _, e := range index {
		es = append(es, e)
	}
	sort.Slice(es, func(i, j int) bool { return es[i].RunID < es[j].RunID })
	b, err := json.MarshalIndent(es, "", "  ")
	if err != nil {
		return nil, err
	}
	return res, writeAtomic(filepath.Join(out, "index.json"), append(b, '\n'))
}

func entry(r *Record, rel string) Entry {
	a := r.Analysis
	e := Entry{RunID: r.RunID, Design: r.Design, Verdict: string(a.Verdict.Kind), MaxRPS: a.Verdict.MaxRPS, Rules: r.Rules,
		Era: r.Conditions.Era, Quick: r.Conditions.Quick != nil, SpeedIndex: r.Conditions.SpeedIndex, Usable: r.Fit.Usable,
		Excluded: len(r.Excluded), Path: rel}
	if b := a.Verdict.Break; b != nil {
		at, off := b.At, b.Offered
		e.BreakAt, e.BreakRPS = &at, &off
	}
	if n := r.Conditions.Network; n != nil {
		e.Network = n.Profile
	}
	if s := r.Sweep; s != nil {
		e.Sweep = s.ID
	}
	return e
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// RunDirs lists the run folders under root, sorted by name.
func RunDirs(root string) ([]string, error) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(root, e.Name(), "run.json")); err == nil {
				out = append(out, filepath.Join(root, e.Name()))
			}
		}
	}
	slices.Sort(out)
	return out, nil
}
