package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/driver"
)

// SeriesFile is the series summary in a series folder.
const SeriesFile = "series.json"

// SeriesRef places a run in a series, as run.json records it.
type SeriesRef struct {
	ID    string `json:"id"`
	Dir   string `json:"dir"`   // the series folder, absolute
	Index int    `json:"index"` // 1-based
	Of    int    `json:"of"`    // runs planned
}

// Series is a set of runs of one design with the same scale and seed, from
// run --repeat: what each run found and how far apart the runs were.
type Series struct {
	ID      string      `json:"id"`
	Design  string      `json:"design"`
	Scale   float64     `json:"scale"`
	Seed    int64       `json:"seed"`
	Planned int         `json:"planned"`
	Runs    []SeriesRun `json:"runs"`

	// Spreads over the conclusive runs: break time, offered and p99 at the
	// break over the runs that broke, peak served over all of them.
	BreakAt      *Spread  `json:"break_at_s,omitempty"`
	BreakOffered *Spread  `json:"break_offered_rps,omitempty"`
	PeakServed   *Spread  `json:"peak_rps,omitempty"`
	P99AtBreak   *Spread  `json:"p99_at_break_ms,omitempty"`
	Warnings     []string `json:"warnings"`
}

// SeriesRun is one run in a series.
type SeriesRun struct {
	RunID        string           `json:"run_id"`
	Verdict      VerdictKind      `json:"verdict"`
	Summary      string           `json:"summary"`
	Speed        int              `json:"speed_index,omitempty"` // 0 = not measured
	Cooled       *driver.Cooldown `json:"cooled,omitempty"`
	BreakAt      *float64         `json:"break_at_s,omitempty"`
	BreakOffered *float64         `json:"break_offered_rps,omitempty"`
	PeakServed   *float64         `json:"peak_rps,omitempty"`
	P99AtBreak   *float64         `json:"p99_at_break_ms,omitempty"`
}

// Spread is the range of one number over a series' runs.
type Spread struct {
	N      int     `json:"n"`
	Min    float64 `json:"min"`
	Median float64 `json:"median"`
	Max    float64 `json:"max"`
}

// Width is how far apart the extremes are.
func (s *Spread) Width() float64 { return s.Max - s.Min }

func spreadOf(vs []float64) *Spread {
	if len(vs) == 0 {
		return nil
	}
	vs = slices.Clone(vs)
	slices.Sort(vs)
	med := vs[len(vs)/2]
	if len(vs)%2 == 0 {
		med = (vs[len(vs)/2-1] + vs[len(vs)/2]) / 2
	}
	return &Spread{N: len(vs), Min: vs[0], Median: med, Max: vs[len(vs)-1]}
}

// Summarize builds a series' summary from its runs' analyses, in run order.
func Summarize(id string, planned int, runs []*Analysis) *Series {
	s := &Series{ID: id, Planned: planned, Runs: []SeriesRun{}, Warnings: []string{}}
	if len(runs) > 0 {
		s.Design, s.Scale, s.Seed = runs[0].Name, runs[0].Scale, runs[0].Seed
	}
	var at, offered, peak, p99 []float64
	var inconclusive []string
	kinds := map[VerdictKind]int{}
	for _, a := range runs {
		v := a.Verdict
		r := SeriesRun{RunID: a.RunID, Verdict: v.Kind, Summary: v.Summary, PeakServed: v.MaxRPS}
		if h := a.Environment.Host; h != nil {
			r.Cooled = h.Cooled
			if h.Speed != nil {
				r.Speed = h.Speed.Index
			}
		}
		if b := v.Break; b != nil {
			r.BreakAt, r.BreakOffered, r.P99AtBreak = &b.At, &b.Offered, b.P99
		}
		s.Runs = append(s.Runs, r)
		kinds[v.Kind]++
		if v.Kind != VerdictBroke && v.Kind != VerdictHeld {
			inconclusive = append(inconclusive, a.RunID)
			continue
		}
		if r.PeakServed != nil {
			peak = append(peak, *r.PeakServed)
		}
		if v.Kind == VerdictBroke {
			at, offered = append(at, *r.BreakAt), append(offered, *r.BreakOffered)
			if r.P99AtBreak != nil {
				p99 = append(p99, *r.P99AtBreak)
			}
		}
	}
	s.BreakAt, s.BreakOffered, s.PeakServed, s.P99AtBreak = spreadOf(at), spreadOf(offered), spreadOf(peak), spreadOf(p99)

	if len(runs) < planned {
		s.Warnings = append(s.Warnings, fmt.Sprintf("only %d of the %d planned runs finished", len(runs), planned))
	}
	if len(inconclusive) > 0 {
		s.Warnings = append(s.Warnings, fmt.Sprintf("%s left out of the spreads, since a run that isn't conclusive says nothing about the design: %s",
			fmt.Sprintf("%d %s", len(inconclusive), plural(len(inconclusive), "run is", "runs are")), strings.Join(inconclusive, ", ")))
	}
	if kinds[VerdictBroke] > 0 && kinds[VerdictHeld] > 0 {
		s.Warnings = append(s.Warnings, fmt.Sprintf("the runs disagree: %d broke and %d held, so the design is near its limit at this profile's peak", kinds[VerdictBroke], kinds[VerdictHeld]))
	}
	if len(s.Runs) > 0 && s.Runs[0].Speed > 0 {
		first := s.Runs[0].Speed
		for _, r := range s.Runs[1:] {
			if r.Speed > 0 && bench.Gap(first, r.Speed) > bench.Tolerance {
				s.Warnings = append(s.Warnings, fmt.Sprintf("run %s's host speed index was %d, against %d for the first run, %s apart, more than %s: it may have broken earlier or later because of the host, not the design",
					r.RunID, r.Speed, first, Pct(bench.Gap(first, r.Speed)), Pct(bench.Tolerance)))
			}
		}
	}
	return s
}

// loadSeries reads the series a run belongs to, as written so far.
func loadSeries(ref *SeriesRef) (*Series, error) {
	if ref == nil {
		return nil, nil
	}
	var s Series
	if err := readJSON(filepath.Join(ref.Dir, SeriesFile), &s); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}
