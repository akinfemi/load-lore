package analysis

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
)

// store reads the series a run exported to metrics/<name>.json: Prometheus
// range-query results at one-second steps. Files load on first use.
type store struct {
	dir   string
	mu    sync.Mutex
	cache map[string][]*series
}

type point struct {
	t, v float64
}

// hist is one native histogram sample: bucket counts keyed by bounds.
type hist struct {
	t       float64
	count   float64
	sum     float64
	buckets map[[2]float64]float64 // [lower, upper] -> count in that bucket
}

type series struct {
	labels map[string]string
	points []point
	hists  []hist
}

func newStore(dir string) *store {
	return &store{dir: dir, cache: map[string][]*series{}}
}

// has reports whether a metric was exported at all.
func (s *store) has(name string) bool { return len(s.load(name)) > 0 }

func (s *store) load(name string) []*series {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ss, ok := s.cache[name]; ok {
		return ss
	}
	ss, _ := readSeries(filepath.Join(s.dir, name+".json"))
	s.cache[name] = ss
	return ss
}

func readSeries(path string) ([]*series, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var raw struct {
		Result []struct {
			Metric     map[string]string `json:"metric"`
			Values     [][2]any          `json:"values"`
			Histograms [][2]any          `json:"histograms"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make([]*series, 0, len(raw.Result))
	for _, r := range raw.Result {
		s := &series{labels: r.Metric}
		for _, v := range r.Values {
			t, ok1 := v[0].(float64)
			f, ok2 := parseNum(v[1])
			if ok1 && ok2 {
				s.points = append(s.points, point{t, f})
			}
		}
		for _, h := range r.Histograms {
			t, ok := h[0].(float64)
			m, ok2 := h[1].(map[string]any)
			if !ok || !ok2 {
				continue
			}
			hs := hist{t: t, buckets: map[[2]float64]float64{}}
			hs.count, _ = parseNum(m["count"])
			hs.sum, _ = parseNum(m["sum"])
			bs, _ := m["buckets"].([]any)
			for _, b := range bs {
				row, ok := b.([]any)
				if !ok || len(row) != 4 {
					continue
				}
				lo, ok1 := parseNum(row[1])
				hi, ok2 := parseNum(row[2])
				c, ok3 := parseNum(row[3])
				if ok1 && ok2 && ok3 {
					hs.buckets[[2]float64{lo, hi}] = c
				}
			}
			s.hists = append(s.hists, hs)
		}
		out = append(out, s)
	}
	return out, nil
}

func parseNum(v any) (float64, bool) {
	switch x := v.(type) {
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil && !math.IsNaN(f)
	case float64:
		return x, true
	}
	return 0, false
}

// matcher selects series by label. A nil func matches every series.
type matcher func(labels map[string]string) bool

func eq(pairs ...string) matcher {
	return func(l map[string]string) bool {
		for i := 0; i+1 < len(pairs); i += 2 {
			if l[pairs[i]] != pairs[i+1] {
				return false
			}
		}
		return true
	}
}

func and(ms ...matcher) matcher {
	return func(l map[string]string) bool {
		for _, m := range ms {
			if m != nil && !m(l) {
				return false
			}
		}
		return true
	}
}

func not(key, value string) matcher {
	return func(l map[string]string) bool { return l[key] != value }
}

func (s *store) match(name string, m matcher) []*series {
	var out []*series
	for _, ss := range s.load(name) {
		if m == nil || m(ss.labels) {
			out = append(out, ss)
		}
	}
	return out
}

// increase is how much matching counters grew over (a, b], summed, with
// counter resets handled the way Prometheus does: a drop means the counter
// restarted from zero. A series that starts inside the window counts from
// zero, since it was born there (a process started or restarted).
func (s *store) increase(name string, m matcher, a, b float64) (float64, bool) {
	total, found := 0.0, false
	for _, ss := range s.match(name, m) {
		if inc, ok := s.increaseOne(ss, a, b); ok {
			total += inc
			found = true
		}
	}
	return total, found
}

// gaugeStale is how old a scrape may be and still count. Targets are
// scraped every second; beyond this the target has stopped (a replica
// died).
const gaugeStale = 2.5

// gaugeGrid samples the sum of matching gauges once a second over [a, b).
// Series from one scrape target share each scrape's timestamp, so they're
// summed per scrape: a series that only exists in some scrapes (one per
// connection state, say) is counted only in those. Targets are scraped at
// different moments, so each target's last scrape before a grid time is
// used, and the targets are added up.
func (s *store) gaugeGrid(name string, m matcher, a, b float64) []float64 {
	type scrape struct{ t, sum float64 }
	byTarget := map[string]map[float64]float64{}
	for i, x := range s.match(name, m) {
		target := x.labels["instance"]
		if target == "" {
			target = fmt.Sprint("series-", i)
		}
		if byTarget[target] == nil {
			byTarget[target] = map[float64]float64{}
		}
		for _, p := range x.points {
			if p.t >= a-gaugeStale && p.t < b {
				byTarget[target][p.t] += p.v
			}
		}
	}
	var targets [][]scrape
	for _, sums := range byTarget {
		sc := make([]scrape, 0, len(sums))
		for t, v := range sums {
			sc = append(sc, scrape{t, v})
		}
		sort.Slice(sc, func(i, j int) bool { return sc[i].t < sc[j].t })
		targets = append(targets, sc)
	}
	var out []float64
	for t := a; t < b; t++ {
		sum, any := 0.0, false
		for _, sc := range targets {
			i := sort.Search(len(sc), func(i int) bool { return sc[i].t > t })
			if i > 0 && t-sc[i-1].t <= gaugeStale {
				sum += sc[i-1].sum
				any = true
			}
		}
		if any {
			out = append(out, sum)
		}
	}
	return out
}

// gaugeSum is the max and mean over [a, b) of the sum of matching gauges.
func (s *store) gaugeSum(name string, m matcher, a, b float64) (mx, mean float64, ok bool) {
	sums := s.gaugeGrid(name, m, a, b)
	if len(sums) == 0 {
		return 0, 0, false
	}
	mx = math.Inf(-1)
	for _, v := range sums {
		mx = math.Max(mx, v)
		mean += v
	}
	return mx, mean / float64(len(sums)), true
}

// gaugeEach returns, per matching series, the max over [a, b), keyed by the
// value of label.
func (s *store) gaugeEach(name string, m matcher, label string, a, b float64) map[string]float64 {
	out := map[string]float64{}
	for _, ss := range s.match(name, m) {
		for _, p := range ss.points {
			if p.t >= a && p.t < b {
				k := ss.labels[label]
				if cur, ok := out[k]; !ok || p.v > cur {
					out[k] = p.v
				}
			}
		}
	}
	return out
}

// gaugeMin is the minimum over [a, b) of the sum of matching gauges.
func (s *store) gaugeMin(name string, m matcher, a, b float64) (float64, bool) {
	sums := s.gaugeGrid(name, m, a, b)
	if len(sums) == 0 {
		return 0, false
	}
	mn := math.Inf(1)
	for _, v := range sums {
		mn = math.Min(mn, v)
	}
	return mn, true
}

// bucket is one histogram bucket's count over a window.
type bucket struct {
	lower, upper, count float64
}

// classicBuckets turns the growth of matching _bucket counters over (a, b]
// into per-bucket counts.
func (s *store) classicBuckets(name string, m matcher, a, b float64) []bucket {
	byLE := map[float64]float64{}
	for _, ss := range s.match(name+"_bucket", m) {
		le, err := strconv.ParseFloat(ss.labels["le"], 64)
		if err != nil {
			continue
		}
		inc, ok := s.increaseOne(ss, a, b)
		if ok {
			byLE[le] += inc
		}
	}
	les := make([]float64, 0, len(byLE))
	for le := range byLE {
		les = append(les, le)
	}
	sort.Float64s(les)
	var out []bucket
	prevLE, prevC := 0.0, 0.0
	for _, le := range les {
		c := byLE[le]
		out = append(out, bucket{lower: prevLE, upper: le, count: math.Max(0, c-prevC)})
		prevLE, prevC = le, c
	}
	return out
}

// increaseOne is how much one counter grew over (a, b]. The counter's
// values at a and b are interpolated between the samples around them, as
// Prometheus's rate() does, so a window counts what happened inside it
// even though samples don't land on its edges.
func (s *store) increaseOne(ss *series, a, b float64) (float64, bool) {
	ps := ss.points
	if len(ps) == 0 || ps[0].t > b {
		return 0, false
	}
	prev, ok := counterAt(ps, a)
	if !ok {
		prev = 0 // born in the window
	}
	inc := 0.0
	step := func(v float64) {
		if v >= prev {
			inc += v - prev
		} else {
			inc += v // reset
		}
		prev = v
	}
	i := sort.Search(len(ps), func(i int) bool { return ps[i].t > a })
	for ; i < len(ps) && ps[i].t <= b; i++ {
		step(ps[i].v)
	}
	if end, ok := counterAt(ps, b); ok && (i >= len(ps) || ps[i-1].t < b) {
		step(end)
	}
	return inc, true
}

// maxInterpolationGap is the longest gap between samples that a value is
// interpolated across; beyond it (a target that stopped reporting) the
// last value holds.
const maxInterpolationGap = 5.0

// counterAt is a counter's value at t, interpolated between the samples
// around t. ok is false before the first sample.
func counterAt(ps []point, t float64) (float64, bool) {
	i := sort.Search(len(ps), func(i int) bool { return ps[i].t > t })
	if i == 0 {
		return 0, false
	}
	p := ps[i-1]
	if i == len(ps) || p.t == t {
		return p.v, true
	}
	n := ps[i]
	if n.v < p.v || n.t-p.t > maxInterpolationGap {
		return p.v, true // a reset, or a gap: don't invent values across it
	}
	return p.v + (n.v-p.v)*(t-p.t)/(n.t-p.t), true
}

// nativeBuckets turns the growth of matching native histograms over (a, b]
// into per-bucket counts, plus the growth of their sum and count.
func (s *store) nativeBuckets(name string, m matcher, a, b float64) (bs []bucket, sum, count float64) {
	acc := map[[2]float64]float64{}
	for _, ss := range s.match(name, m) {
		hs := ss.hists
		if len(hs) == 0 {
			continue
		}
		i := sort.Search(len(hs), func(i int) bool { return hs[i].t > a }) // first after a
		j := sort.Search(len(hs), func(i int) bool { return hs[i].t > b }) // first after b
		if j == 0 || hs[j-1].t <= a {
			continue // no sample in (a, b]
		}
		end := hs[j-1]
		var start *hist // nil: the series was born in the window
		if i > 0 {
			start = &hs[i-1]
		}
		if start != nil && end.count < start.count {
			start = nil // reset: count from zero
		}
		for k, c := range end.buckets {
			if start != nil {
				c -= start.buckets[k]
			}
			if c > 0 {
				acc[k] += c
			}
		}
		sum += end.sum
		count += end.count
		if start != nil {
			sum -= start.sum
			count -= start.count
		}
	}
	for k, c := range acc {
		bs = append(bs, bucket{lower: k[0], upper: k[1], count: c})
	}
	sort.Slice(bs, func(i, j int) bool { return bs[i].upper < bs[j].upper })
	return bs, sum, count
}

// quantile interpolates linearly within the bucket holding rank q. An
// answer in the open-ended last bucket is its lower bound, as Prometheus
// does.
func quantile(bs []bucket, q float64) (float64, bool) {
	total := 0.0
	for _, b := range bs {
		total += b.count
	}
	if total <= 0 {
		return 0, false
	}
	rank := q * total
	cum := 0.0
	for _, b := range bs {
		if b.count > 0 && cum+b.count >= rank {
			if math.IsInf(b.upper, 1) {
				return b.lower, true
			}
			return b.lower + (b.upper-b.lower)*(rank-cum)/b.count, true
		}
		cum += b.count
	}
	last := bs[len(bs)-1]
	if math.IsInf(last.upper, 1) {
		return last.lower, true
	}
	return last.upper, true
}
