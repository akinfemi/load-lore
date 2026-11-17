package analysis

import (
	"strings"
	"testing"
)

// A run whose fault or scale-up failed measured a different design.
func TestLabFailed(t *testing.T) {
	if got := labFailed([]Event{{At: 30, Kind: "fault_start", Detail: "kill on api"}, {At: 90, Kind: "autoscale", Detail: "api 2 → 3"}}); got != "" {
		t.Errorf("healthy timeline: %q", got)
	}
	got := labFailed([]Event{{At: 90, Kind: "autoscale", Detail: "api 2 → 3"}, {At: 122, Kind: "autoscale_error", Detail: "api: scaling failed: lb: no such server"}})
	if !strings.Contains(got, "at 2:02 Loadlore couldn't carry out the design (api: scaling failed: lb: no such server)") {
		t.Errorf("autoscale error: %q", got)
	}
	if got := labFailed([]Event{{At: 60, Kind: "fault_error", Detail: "loss on db: no such container"}}); !strings.Contains(got, "at 1:00") {
		t.Errorf("fault error: %q", got)
	}
}
