// Package diskprobe measures what a disk makes a request wait: reads that
// bypass the page cache, and small writes each followed by fdatasync, the
// way Postgres flushes its WAL. loadl runs it on a modelled disk before
// the stack starts, with the disk's wait off and then on, so each run
// records the wait it really stood on.
package diskprobe

import (
	"math"
	"slices"
	"time"
)

// Stats are the latencies of one kind of request, in microseconds.
type Stats struct {
	N      int     `json:"n"`
	MeanUs float64 `json:"mean_us"`
	P5Us   float64 `json:"p5_us"`
	P50Us  float64 `json:"p50_us"`
	P95Us  float64 `json:"p95_us"`
	P99Us  float64 `json:"p99_us"`
}

// Result is one probe: reads, and writes with their fdatasync.
type Result struct {
	Read Stats `json:"read"`
	Sync Stats `json:"sync"`
}

// File names the probe uses in the directory, removed by Clean.
const (
	ReadFile = ".loadlore-diskprobe-read"
	SyncFile = ".loadlore-diskprobe-sync"
)

// ReadFileBytes is the read file's size: well past any cache the device
// stack keeps, so random reads land on the device.
const ReadFileBytes = 64 << 20

// Summarize turns latencies into Stats.
func Summarize(ds []time.Duration) Stats {
	if len(ds) == 0 {
		return Stats{}
	}
	us := make([]float64, len(ds))
	sum := 0.0
	for i, d := range ds {
		us[i] = float64(d) / float64(time.Microsecond)
		sum += us[i]
	}
	slices.Sort(us)
	q := func(p float64) float64 {
		i := int(math.Ceil(p*float64(len(us)))) - 1
		return round1(us[max(0, min(i, len(us)-1))])
	}
	return Stats{N: len(us), MeanUs: round1(sum / float64(len(us))), P5Us: q(0.05), P50Us: q(0.5), P95Us: q(0.95), P99Us: q(0.99)}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
