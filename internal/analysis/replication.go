package analysis

import (
	"fmt"
	"math"
)

// Replication lag: a read replica falls ReplicationLagMinS or more behind
// its primary, or ReplicationStaleMin or more of the reads after a write
// it serves miss the write, for two windows in a row. Reads from it can
// return data the primary changed in the last lag's worth of time.
const (
	ReplicationLagMinS  = 1.0
	ReplicationStaleMin = 0.05
	// A replica's block reads this many times slower than at low load say
	// its replay waited on the disk.
	ReplicationReadFactor = 3.0
)

func replicationLag(a *Analysis) []*Diagnosis {
	var out []*Diagnosis
	ws := a.Windows
	if len(ws) == 0 || a.Plan == nil {
		return out
	}
	for _, s := range a.Plan.Services {
		if s.Primary == "" {
			continue
		}
		flags := make([]bool, len(ws))
		for i, w := range ws {
			sw := w.Services[s.Name]
			if sw == nil {
				continue
			}
			lagged := sw.ReplicationLagS != nil && *sw.ReplicationLagS >= ReplicationLagMinS
			stale := sw.StaleReadRate != nil && sw.ReadsAfterWrite != nil && *sw.ReadsAfterWrite > 0 && *sw.StaleReadRate >= ReplicationStaleMin
			flags[i] = lagged || stale
		}
		from, to, ok := sustained(flags)
		if !ok {
			continue
		}
		// The window it was furthest behind shows it best.
		worst := from
		for i := from; i <= to; i++ {
			if lagOf(ws[i], s.Name) > lagOf(ws[worst], s.Name) {
				worst = i
			}
		}
		w, sw := ws[worst], ws[worst].Services[s.Name]
		d := &Diagnosis{
			Rule:  "replication-lag",
			Title: fmt.Sprintf("Replication lag (%s behind %s)", s.Name, s.Primary),
			From:  ws[from].Start,
			To:    ws[to].Start + WindowSeconds,
			comp:  s.Name,
		}
		howFar := ""
		if sw.ReplicationLagS != nil {
			howFar = fmt.Sprintf(", by up to %s at %s", secondsText(*sw.ReplicationLagS), Clock(w.Start))
			ev := fmt.Sprintf("%s %s behind %s at %s", s.Name, secondsText(*sw.ReplicationLagS), s.Primary, Clock(w.Start))
			if sw.ReplicationLagBytes != nil {
				howFar += fmt.Sprintf(" (%s of WAL it hadn't replayed)", bytesText(*sw.ReplicationLagBytes))
				ev += fmt.Sprintf(", %s of WAL it hadn't replayed", bytesText(*sw.ReplicationLagBytes))
			}
			d.Evidence = append(d.Evidence, Evidence{Text: ev, Source: SrcReplLagS})
		}
		stale := ""
		if sw.StaleReadRate != nil && sw.ReadsAfterWrite != nil && *sw.ReadsAfterWrite > 0 {
			stale = fmt.Sprintf(" %s of the reads after a write it served didn't see the write.", Pct(*sw.StaleReadRate))
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s of %s reads a second after a write stale at %s", Pct(*sw.StaleReadRate), Rate(*sw.ReadsAfterWrite), Clock(w.Start)), Source: SrcStaleReads})
		}
		why := ""
		if base, timed := blockReadBaseline(a, s.Name); timed && sw.BlockReadMs != nil && *sw.BlockReadMs >= ReplicationReadFactor*base {
			why = fmt.Sprintf(" Its block reads took %s each against %s at low load: a replica replays its primary's writes in one process, and each change to a page it doesn't hold in memory waits for the disk, which its own queries also queue for.", Ms(*sw.BlockReadMs), Ms(base))
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s per block read on %s at %s, against %s at low load", Ms(*sw.BlockReadMs), s.Name, Clock(w.Start), Ms(base)), Source: SrcBlockReadMs})
		} else if c := cpuOf(w, s.Name); c != nil && *c >= 0.9 {
			why = fmt.Sprintf(" Its CPU was at %s of its limit, shared between its queries and replaying the primary's writes.", Pct(*c))
		}
		if c := cpuOf(w, s.Name); c != nil {
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s CPU at %s of its limit at %s", s.Name, Pct(*c), Clock(w.Start)), Source: SrcCPU})
		}
		d.Summary = fmt.Sprintf("From %s, %s fell behind %s%s, so reads from it returned data from before the primary's latest writes.%s%s",
			Clock(ws[from].Start), s.Name, s.Primary, howFar, stale, why)
		d.Next = fmt.Sprintf("Send reads that must see a request's own writes, such as the page after a form post, to %s (read-your-writes), or let %s keep up: memory for the rows its writes touch, or a faster disk. More replicas don't help: each one replays every write.", s.Primary, s.Name)
		out = append(out, d)
	}
	return out
}

func lagOf(w *Window, svc string) float64 {
	if sw := w.Services[svc]; sw != nil && sw.ReplicationLagS != nil {
		return *sw.ReplicationLagS
	}
	return math.Inf(-1)
}

// secondsText is a duration in ms under a second, else seconds.
func secondsText(s float64) string {
	if s < 1 {
		return Ms(s * 1000)
	}
	return fmt.Sprintf("%.1f s", s)
}

// staleNote is the verdict's word on a replica that fell behind in a run
// that held: stale reads succeed, fast, so the verdict alone would miss
// them. Empty otherwise.
func staleNote(a *Analysis) string {
	if a.Verdict.Kind != VerdictHeld {
		return ""
	}
	for _, d := range a.Diagnoses {
		if d.Rule != "replication-lag" {
			continue
		}
		worst := -1.0
		for _, w := range a.Windows {
			if sw := w.Services[d.comp]; sw != nil && sw.StaleReadRate != nil && w.Start >= d.From && w.Start < d.To {
				worst = math.Max(worst, *sw.StaleReadRate)
			}
		}
		primary := ""
		if s := a.Plan.Service(d.comp); s != nil {
			primary = s.Primary
		}
		if worst >= 0 {
			return fmt.Sprintf(" But %s fell behind %s, and up to %s of the reads after a write it served were stale (see replication lag).", d.comp, primary, Pct(worst))
		}
		return fmt.Sprintf(" But %s fell behind %s, so reads from it returned old data (see replication lag).", d.comp, primary)
	}
	return ""
}
