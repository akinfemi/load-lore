package analysis

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Toxiproxy accepts a client and dials its upstream in the same loop, with
// no timeout, one loop per proxy (one caller-to-target edge). A dial that
// waits, on a SYN retransmit to an upstream whose accept queue is full or
// that drops handshake packets, holds up every new connection on that
// edge, which a real network wouldn't. Its log shows it: no "Accepted
// client" on a busy edge for a second or more, then the connections that
// queued meanwhile accepted back to back, far faster than callers open
// them. A caller that paused and then opened many connections at once
// spaces them by a millisecond or more in the recorded runs; a queue
// draining, by a few tenths of one.
const (
	StallGapSeconds   = 1.0  // no accepts on an edge for at least this long
	StallRateMin      = 10.0 // on an edge accepting at least this many a second over the StallLookback seconds before
	StallLookback     = 5.0
	StallDrainMin     = 10     // then at least this many accepts
	StallDrainSeconds = 0.1    // within this long of the first
	StallDrainSpacing = 0.5e-3 // with a median spacing below this (seconds)
	proxyLogGlob      = "*-lore-toxiproxy-*.log"
	SrcAcceptStall    = `Toxiproxy's log, its "Accepted client" lines per proxy`
)

// AcceptStall is a stretch in which Toxiproxy accepted no new connections
// on one edge while callers kept opening them.
type AcceptStall struct {
	Edge    string  `json:"edge"` // Toxiproxy's proxy: caller--target
	At      float64 `json:"at_s"` // seconds from the start of the load
	Seconds float64 `json:"seconds"`
	Queued  int     `json:"queued"` // accepted back to back right after
}

// acceptStalls reads the run's Toxiproxy log. A run without one (the
// logs weren't collected) has none to report.
func acceptStalls(dir string, t0 float64) ([]AcceptStall, error) {
	paths, _ := filepath.Glob(filepath.Join(dir, "logs", proxyLogGlob))
	if len(paths) == 0 {
		return nil, nil
	}
	f, err := os.Open(paths[0])
	if err != nil {
		return nil, err
	}
	defer f.Close()
	accepts := map[string][]float64{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, `"Accepted client"`) {
			continue
		}
		stamp, body, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			continue
		}
		var e struct {
			Name string `json:"name"`
		}
		if json.Unmarshal([]byte(body), &e) != nil || e.Name == "" {
			continue
		}
		accepts[e.Name] = append(accepts[e.Name], float64(t.UnixNano())/1e9-t0)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	var out []AcceptStall
	for edge, ts := range accepts {
		sort.Float64s(ts)
		out = append(out, stallsOn(edge, ts)...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out, nil
}

// stallsOn finds the stalls in one edge's sorted accept times.
func stallsOn(edge string, ts []float64) []AcceptStall {
	var out []AcceptStall
	lo := 0
	for i := 0; i+1 < len(ts); i++ {
		gap := ts[i+1] - ts[i]
		if gap < StallGapSeconds {
			continue
		}
		for ts[lo] < ts[i]-StallLookback {
			lo++
		}
		if float64(i+1-lo)/StallLookback < StallRateMin {
			continue
		}
		hi := i + 1
		for hi < len(ts) && ts[hi] < ts[i+1]+StallDrainSeconds {
			hi++
		}
		drain := ts[i+1 : hi]
		if len(drain) < StallDrainMin {
			continue
		}
		spacing := make([]float64, len(drain)-1)
		for j := range spacing {
			spacing[j] = drain[j+1] - drain[j]
		}
		sort.Float64s(spacing)
		if spacing[len(spacing)/2] >= StallDrainSpacing {
			continue
		}
		out = append(out, AcceptStall{Edge: edge, At: ts[i], Seconds: gap, Queued: len(drain)})
	}
	return out
}

// stallText describes stalls for a reason or caveat: the first, and how
// many more.
func stallText(ss []AcceptStall) string {
	s := ss[0]
	t := fmt.Sprintf("at %s Toxiproxy accepted no new connections on %s for %.1f s, then accepted %d that had queued meanwhile back to back (%s)",
		Clock(s.At), strings.Replace(s.Edge, "--", " → ", 1), s.Seconds, s.Queued, SrcAcceptStall)
	if len(ss) > 1 {
		t += fmt.Sprintf(", and stalled %d more %s", len(ss)-1, plural(len(ss)-1, "time", "times"))
	}
	return t
}
