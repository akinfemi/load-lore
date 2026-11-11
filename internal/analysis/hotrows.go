package analysis

import (
	"fmt"
	"math"
	"sort"
)

// Hot rows: a Postgres has HotLockWaitsMin backends or more waiting on
// locks, on average, while a caller's writes to it take HotWriteFactor
// times their low-load time or more (and HotMinMs more), for two windows in
// a row; its reads stay within HotReadFactor of theirs and the database
// isn't CPU-saturated. Writes to the same popular rows wait for each
// other's row locks; reads don't take them, so only writes slow down.
//
// The lock waits are what make it hot rows. Writes slowing while reads
// don't fits a slow disk too (writes must reach it, cached reads needn't),
// so without them the rule stays quiet, and it skips windows with a
// disk_slow fault on the database.
const (
	HotWriteFactor  = 3.0
	HotMinMs        = 2.0
	HotReadFactor   = 1.5
	HotLockWaitsMin = 1.0
)

// Sources of the hot-rows evidence.
const (
	SrcOpTime    = "loadlore_call_duration_seconds by op (the caller's own timings, pool wait included)"
	SrcLockWaits = "pg_lock_waits_waiting: pg_stat_activity backends with wait_event_type Lock (postgres_exporter, Loadlore's query)"
)

func hotRows(a *Analysis) []*Diagnosis {
	var out []*Diagnosis
	ws := a.Windows
	if len(ws) == 0 || a.Plan == nil {
		return out
	}
	for _, s := range a.Plan.Services {
		if len(s.Synthetic) == 0 {
			continue
		}
		var dbs []string
		for t := range s.Synthetic[0].Targets {
			if svc := a.Plan.Service(t); svc != nil && svc.Kind == "postgres" {
				dbs = append(dbs, t)
			}
		}
		sort.Strings(dbs)
		for _, db := range dbs {
			if d := hotRowsEdge(a, ws, s.Name, db); d != nil {
				out = append(out, d)
			}
		}
	}
	return out
}

func hotRowsEdge(a *Analysis, ws []*Window, svc, db string) *Diagnosis {
	op := func(i int, name string) *float64 {
		if e := edgeAt(ws[i], svc, db); e != nil {
			if v, ok := e.OpMeanMs[name]; ok {
				return &v
			}
		}
		return nil
	}
	// The low-load times: windows past the warm-up offered at most
	// BaselineShare of the peak.
	var wsBase, rsBase []float64
	for i, w := range ws {
		if w.Start < ToolWarmupSeconds || w.Offered > BaselineShare*a.PeakOffered {
			continue
		}
		if wr, rd := op(i, "write"), op(i, "read"); wr != nil && rd != nil {
			wsBase, rsBase = append(wsBase, *wr), append(rsBase, *rd)
		}
	}
	if len(wsBase) < 2 {
		return nil
	}
	sort.Float64s(wsBase)
	sort.Float64s(rsBase)
	wb, rb := median(wsBase), median(rsBase)
	locks := func(i int) *float64 {
		if sw := ws[i].Services[db]; sw != nil {
			return sw.LockWaits
		}
		return nil
	}
	flags := make([]bool, len(ws))
	for i, w := range ws {
		wr, rd, lw := op(i, "write"), op(i, "read"), locks(i)
		if wr == nil || rd == nil || lw == nil || *lw < HotLockWaitsMin || diskFault(a, db, w.Start) {
			continue
		}
		c := cpuOf(w, db)
		flags[i] = *wr >= HotWriteFactor*wb && *wr >= wb+HotMinMs && *rd <= HotReadFactor*rb && (c == nil || *c < PoolTargetCPUMax)
	}
	from, to, ok := sustained(flags)
	if !ok {
		return nil
	}
	peak := from
	for i := from; i <= to; i++ {
		if *op(i, "write") > *op(peak, "write") {
			peak = i
		}
	}
	w := ws[peak]
	wr, rd := *op(peak, "write"), *op(peak, "read")
	d := &Diagnosis{
		Rule:  "hot-rows",
		Title: fmt.Sprintf("Hot rows (%s → %s)", svc, db),
		From:  ws[from].Start,
		To:    ws[to].Start + WindowSeconds,
		Summary: fmt.Sprintf("From %s, %s's writes to %s took %s each, %.1fx their %s at low load, while its reads stayed at %s and %s backends of %s waited on locks. Writes to the same popular rows wait for each other's row locks; reads don't take those locks, so only writes slow down.",
			Clock(ws[from].Start), svc, db, Ms(wr), wr/wb, Ms(wb), Ms(rd), lockText(*locks(peak)), db),
	}
	d.Evidence = append(d.Evidence,
		Evidence{Text: fmt.Sprintf("%s backends waiting on locks at %s, on average", lockText(*locks(peak)), Clock(w.Start)), Source: SrcLockWaits},
		Evidence{Text: fmt.Sprintf("a write took %s at %s, against %s at low load", Ms(wr), Clock(w.Start), Ms(wb)), Source: SrcOpTime},
		Evidence{Text: fmt.Sprintf("a read took %s at %s, against %s at low load", Ms(rd), Clock(w.Start), Ms(rb)), Source: SrcOpTime},
	)
	if c := cpuOf(w, db); c != nil {
		d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s CPU at %s of its limit at %s: not what writes wait for", db, Pct(*c), Clock(w.Start)), Source: SrcCPU})
	}
	d.Next = "Spread the writes over more rows: change the key design so popular items don't share a row, batch updates to the same row, or shard a hot counter into several rows summed on read."
	return d
}

func lockText(n float64) string { return fmt.Sprintf("%.1f", n) }

// diskFault is whether a disk_slow fault on target is on during the window
// starting at start.
func diskFault(a *Analysis, target string, start float64) bool {
	for _, f := range a.Plan.Faults {
		if f.Inject != "disk_slow" || f.Target != target {
			continue
		}
		from := f.At.D().Seconds()
		to := math.Inf(1)
		if f.For != nil {
			to = from + f.For.D().Seconds()
		}
		if from < start+WindowSeconds && to > start {
			return true
		}
	}
	return false
}
