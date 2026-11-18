package analysis

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/plan"
)

// A host disk stall: the Docker host's disk stopped answering. In a run on
// the reference Mac (stale-orders-read-your-writes-20261117-220359, 2:23
// to 2:24) both Postgres servers, primary and replica, read no blocks and
// committed nothing but the exporter's own transactions for two seconds,
// while 41 and 67 queries waited on reads and 38 more on one WAL fsync,
// at about 20% CPU each. The exporter answered within its usual 17 ms
// throughout, since its queries don't touch the data. Every container's
// data lives on the VM's one virtual disk, so a stall there stops them all
// at once: this machine, not the design. A second counts when, for one
// Postgres, all of these hold:
//   - its exporter was scraped in that second (so the counts are fresh),
//   - at least DiskStallWaiters queries waited on its disk (IO waits, or
//     WAL and control-file locks queued behind one),
//   - it read no blocks from outside its buffers,
//   - and it committed at most DiskStallCommitShare of its median over the
//     DiskStallLookback seconds before, which had at least
//     DiskStallMinCommits a second.
//
// DiskStallSeconds such seconds in a row make a stall. A Postgres on a
// modelled disk (a profile with an IOPS cap) sits on the same virtual disk
// and is judged too, with one more condition: it had been reading at least
// DiskStallMinReads blocks a second (median over the lookback) and then read
// none. Its caps hold reads and writes apart and delay a throttled read by
// a slice of a second, so a backlog of writes can hold up its flushes by
// design but can't stop its reads for seconds. In a run on two such disks
// (replica-disk-probe-20261115-102302, 1:57) both servers stopped together,
// from 1,300 and 3,400 blocks a second. Seconds under a disk_slow fault on
// the server aren't judged: that slowness is the design's.
const (
	DiskStallWaiters     = 5
	DiskStallCommitShare = 0.1
	DiskStallLookback    = 10
	DiskStallMinCommits  = 50.0
	DiskStallSeconds     = 2
	DiskStallMinReads    = 20.0
	SrcHostStall         = "pg_stat_activity_count by wait_event, pg_stat_database_blks_read and pg_stat_database_xact_commit, per second (postgres_exporter)"
)

// HostStall is a stretch in which the Docker host's disk stopped answering
// the run's Postgres servers.
type HostStall struct {
	Kind     string   `json:"kind"` // "disk"
	At       float64  `json:"at_s"` // seconds from the start of the load
	Seconds  float64  `json:"seconds"`
	Services []string `json:"services"`
	Waiting  int      `json:"waiting"` // most queries waiting on the disk at once, all servers together
}

// To is where the stall ended.
func (h HostStall) To() float64 { return h.At + h.Seconds }

// diskWait is a wait on the data disk: an IO wait, or a lock queued
// behind one (WALWrite behind a WAL fsync, ControlFile behind a control
// file sync on a replica).
func diskWait(l map[string]string) bool {
	if l["state"] != "active" || l["datname"] != plan.DBName {
		return false
	}
	switch l["wait_event_type"] {
	case "IO":
		return true
	case "LWLock":
		return l["wait_event"] == "WALWrite" || l["wait_event"] == "ControlFile"
	}
	return false
}

// hostStalls finds disk stalls, one per stretch, naming every Postgres
// they stopped.
func hostStalls(st *store, p *plan.Plan, faults []*design.Fault, t0, end float64) []HostStall {
	type sec struct {
		t       int
		waiting int
	}
	var per [][]sec // per Postgres service, its stalled seconds
	var names []string
	for _, s := range p.Services {
		if s.Kind != "postgres" {
			continue
		}
		modelled := s.Disk != nil && s.Disk.IOPS > 0
		// When a disk_slow fault on this server is on, from its start to
		// its end (or the end of the run).
		var slow [][2]float64
		for _, f := range faults {
			if f.Inject != "disk_slow" || f.Target == nil || f.Target.Name != s.Name {
				continue
			}
			from, to := time.Duration(f.At).Seconds(), end
			if f.For != nil {
				to = from + time.Duration(*f.For).Seconds()
			}
			slow = append(slow, [2]float64{from, to})
		}
		svc := eq("service", s.Name)
		lab := and(svc, eq("datname", plan.DBName))
		waits := and(svc, diskWait)
		// Step from one of its exporter's samples to the next, so counts
		// and waits line up with what one scrape saw.
		var ts []float64
		for _, ss := range st.match("pg_stat_database_xact_commit", lab) {
			for _, pt := range ss.points {
				if pt.t >= t0 && pt.t <= t0+end {
					ts = append(ts, pt.t)
				}
			}
			break
		}
		commits := make([]float64, 0, len(ts))
		reads := make([]float64, 0, len(ts))
		var stalled []sec
		for i := 1; i < len(ts); i++ {
			a, b := ts[i-1], ts[i]
			c, ok := st.increase("pg_stat_database_xact_commit", lab, a, b)
			r, rok := st.increase("pg_stat_database_blks_read", lab, a, b)
			if !ok {
				c = -1
			}
			if !rok {
				r = -1
			}
			commits, reads = append(commits, c), append(reads, r)
			if !ok || len(commits) <= DiskStallLookback {
				continue
			}
			if n, ok := st.increase("pg_exporter_scrapes_total", svc, a, b); !ok || n < 1 {
				continue
			}
			if slices.ContainsFunc(slow, func(w [2]float64) bool { return a-t0 < w[1] && b-t0 > w[0] }) {
				continue
			}
			lo := len(commits) - 1 - DiskStallLookback
			med, n := lookbackMedian(commits[lo : len(commits)-1])
			if n < DiskStallLookback/2 || med < DiskStallMinCommits || c > DiskStallCommitShare*med {
				continue
			}
			if !rok || r > 0 {
				continue
			}
			if rmed, _ := lookbackMedian(reads[lo : len(reads)-1]); modelled && rmed < DiskStallMinReads {
				continue
			}
			g := st.gaugeGrid("pg_stat_activity_count", waits, b, b+0.5)
			if len(g) == 0 || g[0] < DiskStallWaiters {
				continue
			}
			stalled = append(stalled, sec{int(math.Round(a - t0)), int(g[0])})
		}
		if len(stalled) > 0 {
			per, names = append(per, stalled), append(names, s.Name)
		}
	}
	// Stretches of DiskStallSeconds or more, per service, then merged
	// across services where they overlap.
	type run struct {
		from, to, waiting int
		svcs              []string
	}
	var runs []run
	for i, ss := range per {
		for j := 0; j < len(ss); {
			k := j
			for k+1 < len(ss) && ss[k+1].t == ss[k].t+1 {
				k++
			}
			if k-j+1 >= DiskStallSeconds {
				w := 0
				for _, x := range ss[j : k+1] {
					w = max(w, x.waiting)
				}
				runs = append(runs, run{ss[j].t, ss[k].t + 1, w, []string{names[i]}})
			}
			j = k + 1
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].from < runs[j].from })
	var merged []run
	for _, r := range runs {
		if n := len(merged); n > 0 && r.from <= merged[n-1].to {
			m := &merged[n-1]
			m.to, m.waiting = max(m.to, r.to), m.waiting+r.waiting
			for _, s := range r.svcs {
				if !slices.Contains(m.svcs, s) {
					m.svcs = append(m.svcs, s)
				}
			}
			continue
		}
		merged = append(merged, r)
	}
	var out []HostStall
	for _, m := range merged {
		slices.Sort(m.svcs)
		out = append(out, HostStall{Kind: "disk", At: float64(m.from), Seconds: float64(m.to - m.from), Services: m.svcs, Waiting: m.waiting})
	}
	return out
}

// lookbackMedian is the median of the known values (-1 is unknown), and
// how many there were.
func lookbackMedian(xs []float64) (float64, int) {
	var v []float64
	for _, x := range xs {
		if x >= 0 {
			v = append(v, x)
		}
	}
	if len(v) == 0 {
		return 0, 0
	}
	sort.Float64s(v)
	return median(v), len(v)
}

// hostStallText says what stopped, for the timeline and the verdict.
func hostStallText(h HostStall) string {
	who := "the database"
	if len(h.Services) > 1 {
		who = "every database"
	}
	return fmt.Sprintf("at %s the Docker host's disk stopped for %.0f s: %s (%s) read no blocks and committed almost nothing while up to %d queries waited on it. Every container's data is on the one virtual disk of the VM Docker runs in, so this is the machine, not the design",
		Clock(h.At), h.Seconds, who, strings.Join(h.Services, ", "), h.Waiting)
}

// markHostStalls flags the windows a host stall touched and adds it to
// the timeline.
func markHostStalls(a *Analysis) {
	for _, h := range a.HostStalls {
		for _, w := range a.Windows {
			if over := min(h.To(), w.Start+WindowSeconds) - max(h.At, w.Start); over > 0 {
				w.HostStallS += over
			}
		}
		a.Timeline = append(a.Timeline, Event{At: h.At, To: h.To(), Kind: "host-stall", Detail: "host disk stall: " + hostStallText(h)})
	}
	sort.SliceStable(a.Timeline, func(i, j int) bool { return a.Timeline[i].At < a.Timeline[j].At })
}

// hostStallVerdict says in the verdict what each host stall may have
// done: one just before the break or in its first window may have made
// it, so the run is inconclusive; any other is a caveat.
func hostStallVerdict(a *Analysis, v *Verdict, breakAt int) {
	for _, h := range a.HostStalls {
		if breakAt >= 0 && h.At < a.Windows[breakAt].Start+WindowSeconds && h.To() > a.Windows[breakAt].Start-WindowSeconds {
			v.Inconclusive = append(v.Inconclusive, hostStallText(h)+", just before the break or in its first window, so it may have made the break")
			continue
		}
		v.Caveats = append(v.Caveats, hostStallText(h)+"; its windows are flagged (host_stall_s in analysis.json) and left out of exports")
	}
}
