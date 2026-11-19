//go:build e2e

package runner

// End-to-end: runs examples/smoke.yaml, its database on a gp3 disk, on the
// local Docker engine and checks the run folder and that nothing is left
// behind. Needs Docker and the
// loadlore/synthetic:dev image (make image). Run with make e2e.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/moby/client"

	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/driver/local"
)

func TestE2ESmoke(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "runs")
	src, err := os.ReadFile("../../examples/smoke.yaml")
	if err != nil {
		t.Fatal(err)
	}
	withDisk := strings.Replace(string(src), "    dataset: { rows: 20000, row_bytes: 256 }\n", "    dataset: { rows: 20000, row_bytes: 256 }\n    disk: { profile: gp3 }\n", 1)
	if withDisk == string(src) {
		t.Fatal("smoke.yaml's database changed; give it the disk another way")
	}
	designPath := filepath.Join(t.TempDir(), "smoke.yaml")
	if err := os.WriteFile(designPath, []byte(withDisk), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(ctx, Options{DesignPath: designPath, Seed: 7, Root: root, Progress: testWriter{t}},
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

	// The report was written from the folder, with real numbers in it.
	for _, name := range []string{"report.md", "analysis.json", "metrics/haproxy_backend_active_servers.json"} {
		if _, err := os.Stat(filepath.Join(res.Dir, name)); err != nil {
			t.Errorf("missing %s", name)
		}
	}
	if res.Verdict == "" || res.Verdict == "no_data" {
		t.Errorf("verdict %q: %s", res.Verdict, res.VerdictSummary)
	}
	var an struct {
		Windows []struct {
			RPS *float64 `json:"rps"`
		} `json:"windows"`
		Expect []struct {
			Status string `json:"status"`
		} `json:"expect"`
	}
	b, _ := os.ReadFile(filepath.Join(res.Dir, "analysis.json"))
	if err := json.Unmarshal(b, &an); err != nil || len(an.Windows) != 6 || an.Windows[2].RPS == nil {
		t.Errorf("analysis: %v, %d windows; load from %v to %v", err, len(an.Windows), res.LoadStarted, res.LoadEnded)
	}
	for i, e := range an.Expect {
		if e.Status != "held" && e.Status != "broken" {
			t.Errorf("expect[%d] not scored: %s", i, e.Status)
		}
	}

	// Every fault applied and came off cleanly, packet loss included.
	evs, _ := os.ReadFile(filepath.Join(res.Dir, "events.jsonl"))
	if !strings.Contains(string(evs), `"inject":"loss"`) || !strings.Contains(string(evs), `"inject":"partition"`) || !strings.Contains(string(evs), `"inject":"disk_slow"`) || strings.Contains(string(evs), `"kind":"fault_error"`) {
		t.Errorf("faults in events.jsonl:\n%s", evs)
	}

	// The stack got the network profile, and k6 too when it started,
	// without errors.
	var stack, k6 bool
	for _, line := range strings.Split(string(evs), "\n") {
		var ev struct {
			Kind   string         `json:"kind"`
			Fields map[string]any `json:"fields"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Kind != "network_profile" {
			continue
		}
		if ev.Fields["error"] != nil {
			t.Errorf("network profile: %v", ev.Fields)
		}
		if n, _ := ev.Fields["containers"].(float64); n > 0 {
			stack = true
		}
		if ev.Fields["role"] == "load" {
			k6 = true
		}
	}
	if !stack || !k6 {
		t.Errorf("network profile applied to the stack %v, to k6 %v:\n%s", stack, k6, evs)
	}

	// The gp3 disk was measured: dm-delay's 2 ms ticks come out at 1 to 2 ms.
	if len(res.Disks) != 1 || res.Disks[0].Unmeasured != "" || res.Disks[0].AddedReadMs < 0.8 || res.Disks[0].AddedReadMs > 2.2 {
		t.Errorf("disks: %+v", res.Disks)
	}
	if b, _ := os.ReadFile(filepath.Join(res.Dir, "report.md")); !strings.Contains(string(b), "measured +") {
		t.Error("the report doesn't give the disk's measured wait")
	}

	// The killed replica shows in container state.
	var states []struct {
		Name     string `json:"name"`
		Service  string `json:"service"`
		ExitCode int    `json:"exit_code"`
	}
	b, _ = os.ReadFile(filepath.Join(res.Dir, "containers.json"))
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
