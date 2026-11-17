package analysis

import (
	"slices"
	"strings"
	"testing"
)

// A stall names the diagnoses that cover it, counting one that ends as it
// starts (a storm's latency shows in the next window), and says so when
// none does.
func TestNameStalls(t *testing.T) {
	a := &Analysis{
		Timeline: []Event{
			{At: 130, To: 140, Kind: "stall", Detail: "p99 reached 617 ms"},
			{At: 240, To: 260, Kind: "stall", Detail: "p99 reached 956 ms"},
			{At: 300, To: 310, Kind: "stall", Detail: "p99 reached 700 ms"},
			{At: 250, Kind: "container", Detail: "db restart"},
		},
		Diagnoses: []*Diagnosis{
			{Rule: "connection-storm", Title: "Connection storm (api → db)", From: 120, To: 130},
			{Rule: "connection-storm", Title: "Connection storm (api → db2)", From: 120, To: 140},
			{Rule: "queueing-knee", Title: "Queueing knee (db CPU)", From: 240, To: 290},
		},
	}
	nameStalls(a)
	tl := a.Timeline
	if !strings.HasSuffix(tl[0].Detail, "; diagnosed as Connection storm (api → db) and Connection storm (api → db2)") || !slices.Equal(tl[0].Rules, []string{"connection-storm"}) {
		t.Errorf("storm stall: %q %v", tl[0].Detail, tl[0].Rules)
	}
	if !strings.HasSuffix(tl[1].Detail, "; diagnosed as Queueing knee (db CPU)") || !slices.Equal(tl[1].Rules, []string{"queueing-knee"}) {
		t.Errorf("knee stall: %q %v", tl[1].Detail, tl[1].Rules)
	}
	if !strings.HasSuffix(tl[2].Detail, "; no diagnosis covers it") || tl[2].Rules != nil {
		t.Errorf("unexplained stall: %q %v", tl[2].Detail, tl[2].Rules)
	}
	if tl[3].Detail != "db restart" {
		t.Errorf("not a stall: %q", tl[3].Detail)
	}
}
