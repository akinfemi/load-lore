package analysis

import (
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/diskprobe"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/plan"
)

// A modelled disk's assumption gains what the run measured on it; other
// assumptions, and a disk the probe couldn't measure, say so or stay put.
func TestWithMeasuredDisks(t *testing.T) {
	as := []plan.Assumption{
		{Field: "services.db.disk", Value: "gp3: 2 ms per read, 2 ms per write, 3000 IOPS, 125 MiB/s", Why: "a gp3 volume"},
		{Field: "services.ro.disk", Value: "gp3", Why: "a gp3 volume"},
		{Field: "services.api.cpu", Value: "1", Why: "set"},
	}
	m := &diskprobe.Result{Read: diskprobe.Stats{N: 400, MeanUs: 1790, P5Us: 1100, P95Us: 2400}, Sync: diskprobe.Stats{N: 150, MeanUs: 5600, P5Us: 4100, P95Us: 6900}}
	b := &diskprobe.Result{Read: diskprobe.Stats{N: 400, MeanUs: 400}, Sync: diskprobe.Stats{N: 150, MeanUs: 2700}}
	out := withMeasuredDisks(as, []driver.DiskResult{
		{Service: "db", Container: "db", ReadMs: 2, WriteMs: 2, Base: b, Measured: m, AddedReadMs: 1.39, AddedSyncMs: 2.9},
		{Service: "ro", Container: "ro-1", Unmeasured: "no image"},
	})
	if got := out[0].Value; got != "gp3: 2 ms per read, 2 ms per write, 3000 IOPS, 125 MiB/s; measured +1.39 ms per read, +2.9 ms per flushed write" {
		t.Errorf("db value %q", got)
	}
	if !strings.Contains(out[0].Why, "a read took 1.79 ms on average (1.1 to 2.4 ms, 5th to 95th percentile) against 0.4 ms with no wait") {
		t.Errorf("db why %q", out[0].Why)
	}
	if out[1].Value != "gp3; ro-1: not measured" || out[2] != as[2] {
		t.Errorf("got %+v", out[1:])
	}
	if as[0].Value != "gp3: 2 ms per read, 2 ms per write, 3000 IOPS, 125 MiB/s" {
		t.Error("the input changed")
	}
}
