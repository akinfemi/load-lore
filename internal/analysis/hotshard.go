package analysis

import (
	"fmt"
	"slices"
	"strings"

	"github.com/akinfemi/load-lore/internal/synthetic"
)

// Hot shard: a caller sends calls to shards by key, and one shard takes
// HotShardShareFactor times its even share of the caller's calls to them
// or more while it is busy (CPU at HotShardCPU or more, or its calls
// TargetSlowFactor times their low-load time and TargetSlowMinMs more),
// and another shard's CPU stays under HotShardIdleCPU, for two windows in
// a row. The keys are spread by hash, but the traffic isn't: the popular
// keys live on that shard, and more shards leave them there.
const (
	HotShardShareFactor = 1.5
	HotShardCPU         = 0.8
	HotShardIdleCPU     = 0.5
)

// SrcShardCalls is where a caller's calls to each shard are counted.
const SrcShardCalls = "loadlore_calls_total by target (the caller's own count of calls to each shard)"

func hotShard(a *Analysis) []*Diagnosis {
	var out []*Diagnosis
	if len(a.Windows) == 0 || a.Plan == nil {
		return out
	}
	for _, s := range a.Plan.Services {
		if len(s.Synthetic) == 0 {
			continue
		}
		for _, group := range shardGroups(s.Synthetic[0].Calls) {
			if d := hotShardGroup(a, s.Name, group); d != nil {
				out = append(out, d)
			}
		}
	}
	return out
}

// shardGroups lists the shard sets a service's calls route by key, each
// once, in call order.
func shardGroups(calls []synthetic.Call) [][]string {
	var out [][]string
	var walk func([]synthetic.Call)
	walk = func(cs []synthetic.Call) {
		for _, c := range cs {
			if len(c.Shards) > 1 && c.ShardBy != "all" && !slices.ContainsFunc(out, func(g []string) bool { return slices.Equal(g, c.Shards) }) {
				out = append(out, c.Shards)
			}
			walk(c.OnMiss)
		}
	}
	walk(calls)
	return out
}

func hotShardGroup(a *Analysis, svc string, shards []string) *Diagnosis {
	ws := a.Windows
	n := float64(len(shards))
	// Low-load mean call time per shard, for the slow test.
	base := map[string]float64{}
	for _, sh := range shards {
		var v []float64
		for _, w := range ws {
			if w.Start < ToolWarmupSeconds || w.Offered > BaselineShare*a.PeakOffered {
				continue
			}
			if m := callMean(w, svc, sh); m != nil {
				v = append(v, *m)
			}
		}
		if len(v) > 0 {
			slices.Sort(v)
			base[sh] = median(v)
		}
	}
	type pick struct {
		hot, idle      string
		share, hotCPU  float64
		idleCPU, rps   float64
		slow, slowBase float64
	}
	picks := make([]*pick, len(ws))
	flags := make([]bool, len(ws))
	for i, w := range ws {
		total, best := 0.0, -1.0
		var p pick
		for _, sh := range shards {
			e := edgeAt(w, svc, sh)
			if e == nil || e.RPS == nil {
				continue
			}
			total += *e.RPS
			if *e.RPS > best {
				best, p.hot = *e.RPS, sh
			}
		}
		if total <= 0 || p.hot == "" {
			continue
		}
		p.share, p.rps = best/total, best
		busy := false
		if c := cpuOf(w, p.hot); c != nil {
			p.hotCPU = *c
			busy = *c >= HotShardCPU
		}
		if m, b := callMean(w, svc, p.hot), base[p.hot]; m != nil && b > 0 && *m >= TargetSlowFactor*b && *m-b >= TargetSlowMinMs {
			busy, p.slow, p.slowBase = true, *m, b
		}
		p.idleCPU = 2
		for _, sh := range shards {
			if c := cpuOf(w, sh); sh != p.hot && c != nil && *c < p.idleCPU {
				p.idle, p.idleCPU = sh, *c
			}
		}
		picks[i] = &p
		flags[i] = busy && p.share >= HotShardShareFactor/n && p.idle != "" && p.idleCPU < HotShardIdleCPU
	}
	from, to, ok := sustained(flags)
	if !ok {
		return nil
	}
	// The window where the hot shard's share was highest describes it.
	k := from
	for i := from; i <= to; i++ {
		if picks[i].share > picks[k].share {
			k = i
		}
	}
	p, w := picks[k], ws[k]
	busyText := fmt.Sprintf("ran at %s CPU", Pct(p.hotCPU))
	if p.slow > 0 && p.hotCPU < HotShardCPU {
		busyText = fmt.Sprintf("answered in %s a call against %s at low load (CPU %s)", Ms(p.slow), Ms(p.slowBase), Pct(p.hotCPU))
	}
	d := &Diagnosis{
		Rule:   "hot-shard",
		Title:  fmt.Sprintf("Hot shard (%s → %s)", svc, p.hot),
		From:   ws[from].Start,
		To:     ws[to].Start + WindowSeconds,
		comp:   p.hot,
		caller: svc,
		Summary: fmt.Sprintf("From %s, %s took %s of %s's calls to its %d shards (%s), against an even share of %s, and %s, while %s was at %s CPU. The keys are spread by hash, but the traffic isn't: the most popular keys live on %s, and a key stays on one shard however many shards there are.",
			Clock(ws[from].Start), p.hot, Pct(p.share), svc, len(shards), strings.Join(shards, ", "), Pct(1/n), busyText, p.idle, Pct(p.idleCPU), p.hot),
	}
	d.Evidence = append(d.Evidence,
		Evidence{Text: fmt.Sprintf("%s calls a second from %s to %s at %s, %s of its calls to the shards", Rate(p.rps), svc, p.hot, Clock(w.Start), Pct(p.share)), Source: SrcShardCalls},
		Evidence{Text: fmt.Sprintf("%s CPU at %s of its limit at %s", p.hot, Pct(p.hotCPU), Clock(w.Start)), Source: SrcCPU},
		Evidence{Text: fmt.Sprintf("%s CPU at %s of its limit at %s", p.idle, Pct(p.idleCPU), Clock(w.Start)), Source: SrcCPU},
	)
	if p.slow > 0 {
		d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("a call to %s took %s at %s, against %s at low load", p.hot, Ms(p.slow), Clock(w.Start), Ms(p.slowBase)), Source: SrcOpTime})
	}
	d.Next = fmt.Sprintf("More shards won't move a hot key off its shard: they spread the other keys thinner and leave the popular ones where they are, and every fan-out read then costs one more query. Take the load off the hot keys instead: cache their reads (popular keys are what a cache keeps best), or spread a hot key's writes over several rows. More CPU for %s alone is a stopgap.", p.hot)
	return d
}

// callMean is the mean time of svc's calls to target in w, over ops,
// weighted equally (the ops a shard takes don't change with the load).
func callMean(w *Window, svc, target string) *float64 {
	e := edgeAt(w, svc, target)
	if e == nil || len(e.OpMeanMs) == 0 {
		return nil
	}
	sum := 0.0
	for _, v := range e.OpMeanMs {
		sum += v
	}
	m := sum / float64(len(e.OpMeanMs))
	return &m
}
