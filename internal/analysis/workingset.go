package analysis

import (
	"fmt"
	"sort"
	"strings"
)

// A working set that no longer fits: Postgres answers less than
// WorkingSetHitMax of its block reads from shared_buffers, reading
// WorkingSetMinReads blocks a second or more from outside them, for two
// windows in a row, while p99 is KneeLatencyFactor times the low-load p99
// or more. The rows requests touch no longer fit in memory, so reads wait
// on the OS cache or the disk.
//
// When Postgres times its reads (track_io_timing, on with a disk block),
// the reads must also be what slowed: a block read takes
// WorkingSetReadFactor times its low-load time or more, and
// WorkingSetReadMinMs more. A low hit rate alone doesn't say the reads
// waited: while a cache that fits warms up, or when the page cache answers
// the misses, the slowdown can come from somewhere else.
const (
	WorkingSetHitMax     = 0.9
	WorkingSetMinReads   = 100.0
	WorkingSetReadFactor = 3.0
	WorkingSetReadMinMs  = 1.0
)

// Sources of the working-set evidence.
const (
	SrcBufferHit   = "pg_stat_database_blks_hit ÷ (blks_hit + blks_read) (postgres_exporter)"
	SrcBlockReads  = "pg_stat_database_blks_read (postgres_exporter): blocks read from outside shared_buffers"
	SrcBlockReadMs = "pg_stat_database_blk_read_time ÷ pg_stat_database_blks_read (postgres_exporter, track_io_timing on)"
)

// blockReadBaseline is the median time a block read took at low load, if
// Postgres timed its reads.
func blockReadBaseline(a *Analysis, db string) (float64, bool) {
	var xs []float64
	for _, w := range healthy(a.Windows) {
		if w.Start < ToolWarmupSeconds || w.Offered > BaselineShare*a.PeakOffered {
			continue
		}
		if sw := w.Services[db]; sw != nil && sw.BlockReadMs != nil && sw.BlockReads != nil && *sw.BlockReads >= WorkingSetMinReads {
			xs = append(xs, *sw.BlockReadMs)
		}
	}
	if len(xs) == 0 {
		return 0, false
	}
	sort.Float64s(xs)
	return median(xs), true
}

func workingSet(a *Analysis) []*Diagnosis {
	var out []*Diagnosis
	ws, base := a.Windows, a.Verdict.BaselineP99
	if len(ws) == 0 || a.Plan == nil {
		return out
	}
	for _, s := range a.Plan.Services {
		if s.Kind != "postgres" {
			continue
		}
		readBase, timed := blockReadBaseline(a, s.Name)
		flags := make([]bool, len(ws))
		for i, w := range ws {
			sw := w.Services[s.Name]
			if sw == nil || sw.BufferHitRate == nil || sw.BlockReads == nil {
				continue
			}
			slow := base != nil && w.P99 != nil && *w.P99 >= KneeLatencyFactor**base && *w.P99 >= *base+KneeLatencyMinMs
			if timed {
				slow = slow && sw.BlockReadMs != nil && *sw.BlockReadMs >= WorkingSetReadFactor*readBase && *sw.BlockReadMs >= readBase+WorkingSetReadMinMs
			}
			flags[i] = *sw.BufferHitRate < WorkingSetHitMax && *sw.BlockReads >= WorkingSetMinReads && slow
		}
		from, to, ok := sustained(flags)
		if !ok {
			continue
		}
		// The window with the lowest hit rate shows it best.
		low := from
		for i := from; i <= to; i++ {
			if *ws[i].Services[s.Name].BufferHitRate < *ws[low].Services[s.Name].BufferHitRate {
				low = i
			}
		}
		w, sw := ws[low], ws[low].Services[s.Name]
		d := &Diagnosis{
			Rule:  "working-set",
			Title: fmt.Sprintf("Working set doesn't fit in memory (%s)", s.Name),
			From:  ws[from].Start,
			To:    ws[to].Start + WindowSeconds,
			Summary: fmt.Sprintf("From %s, %s answered only %s of its block reads from shared_buffers and read %s blocks a second from outside them. The rows requests touch no longer fit in its memory, so reads wait on the OS cache or the disk.",
				Clock(ws[from].Start), s.Name, Pct(*sw.BufferHitRate), Rate(*sw.BlockReads)),
		}
		d.Evidence = append(d.Evidence,
			Evidence{Text: fmt.Sprintf("%s of block reads from shared_buffers at %s", Pct(*sw.BufferHitRate), Clock(w.Start)), Source: SrcBufferHit},
			Evidence{Text: fmt.Sprintf("%s blocks read per second from outside them at %s", Rate(*sw.BlockReads), Clock(w.Start)), Source: SrcBlockReads},
		)
		if timed && sw.BlockReadMs != nil {
			d.Summary = fmt.Sprintf("From %s, %s answered only %s of its block reads from shared_buffers and read %s blocks a second from outside them, each taking %s against %s at low load. The rows requests touch no longer fit in its memory, so reads wait for the disk.",
				Clock(ws[from].Start), s.Name, Pct(*sw.BufferHitRate), Rate(*sw.BlockReads), Ms(*sw.BlockReadMs), Ms(readBase))
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s per block read at %s, against %s at low load", Ms(*sw.BlockReadMs), Clock(w.Start), Ms(readBase)), Source: SrcBlockReadMs})
		}
		if w.P99 != nil && base != nil {
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("p99 %s at %s, against %s at low load", Ms(*w.P99), Clock(w.Start), Ms(*base)), Source: SrcK6Latency})
		}
		if c := cpuOf(w, s.Name); c != nil {
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s CPU at %s of its limit at %s", s.Name, Pct(*c), Clock(w.Start)), Source: SrcCPU})
		}
		var size []string
		if ds := s.Dataset; ds != nil && ds.Rows > 0 && ds.RowBytes > 0 {
			size = append(size, fmt.Sprintf("a dataset of about %s", bytesText(float64(ds.Rows)*float64(ds.RowBytes))))
		}
		if sb := s.Config["shared_buffers"]; sb != "" {
			size = append(size, "shared_buffers "+sb)
		}
		if s.Memory > 0 {
			size = append(size, "a "+bytesText(float64(s.Memory))+" memory limit")
		}
		if len(size) > 0 {
			d.Evidence = append(d.Evidence, Evidence{Text: s.Name + " has " + strings.Join(size, ", "), Source: "plan.json"})
		}
		d.Next = fmt.Sprintf("Make the hot rows fit: give %s more memory (its limit and shared_buffers), read fewer rows per request (an index that covers the query, or narrower rows), or split the data so each node's working set fits.", s.Name)
		out = append(out, d)
	}
	return out
}

// bytesText is a size in MB or GB.
func bytesText(b float64) string {
	if b >= 1<<30 {
		return fmt.Sprintf("%.1f GB", b/(1<<30))
	}
	return fmt.Sprintf("%.0f MB", b/(1<<20))
}
