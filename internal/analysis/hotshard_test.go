package analysis

import (
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/synthetic"
)

// shardRun is api reading from four shards over a ramp to 1,000 rps:
// from 0:50 db0 takes hot of the calls at hotCPU, the others idle at 30%.
func shardRun(hot, hotCPU float64) *Analysis {
	shards := []string{"db0", "db1", "db2", "db3"}
	a := &Analysis{Plan: &plan.Plan{}, PeakOffered: 1000}
	a.Plan.Services = []*plan.Service{{Name: "api", Kind: "app", Synthetic: []*synthetic.Config{{Calls: []synthetic.Call{
		{Shards: shards, ShardBy: "key", Op: "read"},
		{Shards: shards, ShardBy: "all", Op: "read"},
	}}}}}
	for i := range 10 {
		offered := 100 + 100*float64(i)
		w := &Window{Start: float64(i * WindowSeconds), Offered: offered, Services: map[string]*ServiceWindow{}}
		share, cpu := 0.25, 0.2
		if i >= 5 {
			share, cpu = hot, hotCPU
		}
		for j, sh := range shards {
			rps, c, ms := offered*(1-share)/3, 0.3, 0.4
			if j == 0 {
				rps, c, ms = offered*share, cpu, 0.4
			}
			w.Edges = append(w.Edges, &EdgeWindow{From: "api", To: sh, RPS: &rps, OpMeanMs: map[string]float64{"read": ms}})
			w.Services[sh] = &ServiceWindow{CPU: &c}
		}
		a.Windows = append(a.Windows, w)
	}
	return a
}

func TestHotShard(t *testing.T) {
	d := hotShard(shardRun(0.6, 0.95))
	if len(d) != 1 || d[0].Rule != "hot-shard" || d[0].comp != "db0" || d[0].From != 50 {
		t.Fatalf("hot shard: %+v", d)
	}
	for _, want := range []string{"db0 took 60% of api's calls to its 4 shards (db0, db1, db2, db3), against an even share of 25%, and ran at 95% CPU, while db1 was at 30% CPU", "a key stays on one shard however many shards there are"} {
		if !strings.Contains(d[0].Summary, want) {
			t.Errorf("summary lacks %q: %s", want, d[0].Summary)
		}
	}
	if !strings.Contains(d[0].Next, "More shards won't move a hot key") {
		t.Errorf("next: %s", d[0].Next)
	}

	// Busy but evenly loaded, or uneven but not busy: no hot shard.
	if d := hotShard(shardRun(0.3, 0.95)); len(d) != 0 {
		t.Errorf("a near-even share: %+v", d)
	}
	if d := hotShard(shardRun(0.6, 0.5)); len(d) != 0 {
		t.Errorf("an uneven but idle shard: %+v", d)
	}
}
