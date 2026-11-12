package analysis

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// A cold cache: a cache's share of lookups that miss rises to ColdMissFactor
// times its level before or more, and to ColdMissMin of lookups at least,
// and at the same time the reads that miss it reach the database at
// ColdReadFactor times their earlier rate or more. After a cache restart
// every key misses at once, so the database takes the reads the cache used
// to answer, all together: a thundering herd. The misses are counted, not
// the hit rate halved: a warm cache refills its hot keys in seconds, so
// over a 10-second window the hit rate can stay high while the misses
// multiply.
const (
	ColdMissFactor = 5.0
	ColdMissMin    = 0.05
	ColdReadFactor = 2.0
	// A warm cache can send the database no reads at all; its reads then
	// count as this many a second, so a herd still has a level to beat.
	ColdReadFloor = 1.0
	// The level before is the median of up to this many windows before the
	// drop, past the warm-up, with the hit rate at ColdWarmHitMin or more.
	ColdBaseWindows = 6
	ColdWarmHitMin  = 0.3
)

// Sources of the cold-cache evidence.
const (
	SrcDBReads = "loadlore_calls_total to the database (the caller's own counts)"
)

// coldCache finds a cache that went cold while its callers kept reading
// through it to a database.
func coldCache(a *Analysis) []*Diagnosis {
	var out []*Diagnosis
	ws := a.Windows
	if len(ws) == 0 || a.Plan == nil {
		return out
	}
	for _, s := range a.Plan.Services {
		if len(s.Synthetic) == 0 {
			continue
		}
		targets := s.Synthetic[0].Targets
		var caches, dbs []string
		for name := range targets {
			switch t := a.Plan.Service(name); {
			case t == nil:
			case t.Kind == "redis":
				caches = append(caches, name)
			case t.Kind == "postgres" || t.Kind == "pgbouncer":
				dbs = append(dbs, name)
			}
		}
		sort.Strings(caches)
		sort.Strings(dbs)
		for _, cache := range caches {
			for _, db := range dbs {
				if d := coldCacheEdge(a, ws, s.Name, cache, db); d != nil {
					out = append(out, d)
				}
			}
		}
	}
	return out
}

func coldCacheEdge(a *Analysis, ws []*Window, svc, cache, db string) *Diagnosis {
	hit := func(i int) *float64 {
		if sw := ws[i].Services[cache]; sw != nil {
			return sw.HitRate
		}
		return nil
	}
	reads := func(i int) *float64 {
		if e := edgeAt(ws[i], svc, db); e != nil {
			return e.RPS
		}
		return nil
	}
	for i := range ws {
		// The level before the drop.
		var hits, rs []float64
		for j := i - 1; j >= 0 && len(hits) < ColdBaseWindows; j-- {
			h, r := hit(j), reads(j)
			if ws[j].Start < ToolWarmupSeconds || h == nil || r == nil || *h < ColdWarmHitMin {
				continue
			}
			hits, rs = append(hits, *h), append(rs, *r)
		}
		if len(hits) < 2 {
			continue
		}
		sort.Float64s(hits)
		sort.Float64s(rs)
		hb, rb := median(hits), median(rs)
		h, r := hit(i), reads(i)
		if h == nil || r == nil || 1-*h < ColdMissMin || 1-*h < ColdMissFactor*(1-hb) || *r < ColdReadFactor*math.Max(rb, ColdReadFloor) {
			continue
		}
		w := ws[i]
		d := &Diagnosis{
			Rule:  "cold-cache",
			Title: fmt.Sprintf("Cold cache (%s, %s → %s)", cache, svc, db),
			From:  w.Start,
			To:    w.Start + WindowSeconds,
			Summary: fmt.Sprintf("At %s, %s's hit rate fell to %s from %s, %s of lookups missing, and %s's reads from %s reached %s rps, %s. The keys the cache used to answer now miss together, so their reads all go to the database at once: a thundering herd.",
				Clock(w.Start), cache, Pct(*h), Pct(hb), missText(1-*h, 1-hb), svc, db, Rate(*r), readsBefore(*r, rb)),
		}
		d.Evidence = append(d.Evidence,
			Evidence{Text: fmt.Sprintf("%s hit rate %s at %s, against %s before", cache, Pct(*h), Clock(w.Start), Pct(hb)), Source: SrcHitRate},
			Evidence{Text: fmt.Sprintf("%s rps from %s to %s at %s, %s", Rate(*r), svc, db, Clock(w.Start), readsBefore(*r, rb)), Source: SrcDBReads},
		)
		if c := cpuOf(w, db); c != nil {
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s CPU at %s of its limit at %s", db, Pct(*c), Clock(w.Start)), Source: SrcCPU})
		}
		for _, ev := range a.Timeline {
			near := ev.At >= w.Start-2*WindowSeconds && ev.At < w.Start+WindowSeconds
			if near && strings.Contains(ev.Detail, cache) && (ev.Kind == "fault_start" || ev.Kind == "container") {
				d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s at %s", ev.Detail, Clock(ev.At)), Source: "events.jsonl"})
				break
			}
		}
		d.Next = fmt.Sprintf("Keep a cold %s from sending every miss to %s at once: coalesce concurrent misses for the same key (single-flight), so one read fills it for everyone, or warm %s before it takes traffic.", cache, db, cache)
		return d
	}
	return nil
}

// missText is the share of lookups missing, against before.
func missText(m, before float64) string {
	if before <= 0 {
		return Pct(m)
	}
	return fmt.Sprintf("%s (%.0fx the %s before)", Pct(m), m/before, Pct(before))
}

// readsBefore compares a read rate with the rate before.
func readsBefore(r, before float64) string {
	if before <= 0 {
		return "against none before"
	}
	return fmt.Sprintf("%.1fx the %s rps before", r/before, Rate(before))
}
