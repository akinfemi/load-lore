//go:build e2e

package runner

// End-to-end: runs examples/smoke.yaml on the local Docker engine and checks
// the run folder and that nothing is left behind. Needs Docker and the
// loadlore/synthetic:dev image (make image). Run with make e2e.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/moby/moby/client"

	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/driver/local"
)

func TestE2ESmoke(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "runs")
	res, err := Run(ctx, Options{DesignPath: "../../examples/smoke.yaml", Seed: 7, Root: root, Progress: testWriter{t}},
		func(ctx context.Context) (driver.Driver, error) { return local.New(ctx) })
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"design.yaml", "plan.json", "compose.yaml", "load.js", "events.jsonl", "k6-summary.json", "containers.json", "run.json",
		"metrics/loadlore_requests_total.json", "metrics/k6_http_reqs_total.json", "metrics/container_cpu_usage_seconds_total.json", "metrics/pg_stat_activity_count.json"} {
		if _, err := os.Stat(filepath.Join(res.Dir, name)); err != nil {
			t.Errorf("missing %s", name)
		}
	}
	if res.Load == nil || res.Load.Requests == 0 {
		t.Errorf("no requests recorded: %+v", res.Load)
	}

	// The killed replica shows in container state.
	var states []struct {
		Name     string `json:"name"`
		Service  string `json:"service"`
		ExitCode int    `json:"exit_code"`
	}
	b, _ := os.ReadFile(filepath.Join(res.Dir, "containers.json"))
	if err := json.Unmarshal(b, &states); err != nil {
		t.Fatal(err)
	}
	killed := false
	for _, s := range states {
		killed = killed || (s.Service == "api" && s.ExitCode == 137)
	}
	if !killed {
		t.Error("no api replica exited 137 after the kill fault")
	}

	// Nothing of the run is left.
	api, _, err := local.NewClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f := client.Filters{}.Add("label", local.LabelRun+"="+res.RunID)
	cs, _ := api.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: f})
	vs, _ := api.VolumeList(ctx, client.VolumeListOptions{Filters: f})
	ns, _ := api.NetworkList(ctx, client.NetworkListOptions{Filters: f})
	if len(cs.Items)+len(vs.Items)+len(ns.Items) != 0 {
		t.Errorf("left behind: %d containers, %d volumes, %d networks", len(cs.Items), len(vs.Items), len(ns.Items))
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { w.t.Log(string(p)); return len(p), nil }
