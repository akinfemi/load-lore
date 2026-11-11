package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/akinfemi/load-lore/internal/analysis"
	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/report"
	"github.com/akinfemi/load-lore/internal/rundir"
)

// SeriesRoot is where run --repeat keeps its series summaries, next to the
// runs folder.
const SeriesRoot = ".loadlore/series"

// SeriesResult is what Repeat did: the series summary and each run.
type SeriesResult struct {
	Dir    string           `json:"dir"`
	Series *analysis.Series `json:"series"`
	Runs   []*Result        `json:"runs"`
	Report string           `json:"report"`
}

// Repeat runs a design n times in a row with the same scale and seed, so
// what differs between runs is the host, and writes a series summary with
// the spread of break time, peak served and p99 at the break. Runs after
// the first wait for the host to cool back to the first run's speed. It
// stops at the first run that doesn't complete.
func Repeat(ctx context.Context, opt Options, n int, newDriver func(context.Context) (driver.Driver, error)) (*SeriesResult, error) {
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
	if opt.Seed == 0 {
		// Every run draws the same keys and mix, so the spread is the host's.
		opt.Seed = rand.Int64N(1<<31-1) + 1
	}
	root := SeriesRoot
	if opt.Root != "" {
		root = filepath.Join(filepath.Dir(opt.Root), "series")
	}
	id := rundir.NewID(d.Name+"-series", time.Now())
	sdir := filepath.Join(root, id)
	if err := os.MkdirAll(sdir, 0o755); err != nil {
		return nil, err
	}
	abs, _ := filepath.Abs(sdir)
	sr := &SeriesResult{Dir: abs, Runs: []*Result{}}
	fmt.Fprintf(out, "series %s: %d runs with seed %d\n  folder %s\n", id, n, opt.Seed, sdir)

	var as []*analysis.Analysis
	var runErr error
	for i := 1; i <= n; i++ {
		fmt.Fprintf(out, "\n== run %d of %d\n", i, n)
		o := opt
		o.Series = &analysis.SeriesRef{ID: id, Dir: abs, Index: i, Of: n}
		if i > 1 && len(sr.Runs) > 0 {
			if h := sr.Runs[0].Host; h != nil && h.Speed != nil {
				o.CoolTo = h.Speed.Index
			}
		}
		res, err := Run(ctx, o, newDriver)
		if res != nil {
			sr.Runs = append(sr.Runs, res)
			if a, aerr := analysis.Analyze(res.Dir); aerr == nil {
				as = append(as, a)
			} else {
				fmt.Fprintf(out, "  warning: analyzing %s: %v\n", res.RunID, aerr)
			}
		}
		sr.Series = analysis.Summarize(id, n, as)
		if werr := writeSeries(sdir, sr.Series); werr != nil {
			return sr, werr
		}
		if err != nil {
			runErr = err
			break
		}
	}
	sr.Report = filepath.Join(sdir, report.SeriesFileName)
	return sr, runErr
}

func writeSeries(dir string, s *analysis.Series) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, analysis.SeriesFile), append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, report.SeriesFileName), []byte(report.Series(s)), 0o644)
}
