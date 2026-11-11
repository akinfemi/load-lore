// Package bench is the host speed benchmark pre-flight runs: a few seconds
// of fixed work in a container limited to one CPU, scored against the
// reference machine. CPU limits are fractions of whatever core the host
// has, so a design breaks at a lower load on slower cores; the speed index
// says how much slower, so results can be read in that light.
//
// It times three kinds of work, because a request's cost isn't only
// arithmetic:
//   - hash: SHA-256 over a buffer in cache, raw instruction throughput;
//   - memory: dependent random reads over 64 MiB, memory latency;
//   - syscalls: 1-byte round trips over loopback TCP, the kernel's network
//     path and scheduler wakeups, which dominate a small database query.
package bench

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"net"
	"time"
)

// ReferenceMachine is where Reference was measured.
const ReferenceMachine = "MacBook Air M4 (4 performance and 6 efficiency cores), Colima VM with 8 vCPUs"

// Reference is what the reference machine measured, per second. A part
// scoring 1000 matches it: medians of five runs on 10 Nov 2026 on a
// MacBook Air M4 (2025) on mains power, in a Colima VM with 8 vCPUs, in a
// container limited to 1 CPU. The same machine in Low Power Mode scored
// about half on every part.
var Reference = Result{HashMBps: 3125, MemReadsPerS: 10.8e6, RoundTripsPerS: 282e3}

// Result is one benchmark run.
type Result struct {
	HashMBps       float64 `json:"hash_mb_per_s"`
	MemReadsPerS   float64 `json:"memory_reads_per_s"`
	RoundTripsPerS float64 `json:"round_trips_per_s"`
}

// Score is a result against Reference: each part scaled so the reference
// is 1000, and the index their geometric mean.
type Score struct {
	Index    int `json:"index"`
	Hash     int `json:"hash"`
	Memory   int `json:"memory"`
	Syscalls int `json:"syscalls"`
}

// Speed is a scored result, as hosts and reports record it.
type Speed struct {
	Score
	Raw Result `json:"raw"`
}

// Scored scores r against Reference.
func (r Result) Scored() Score {
	part := func(v, ref float64) float64 { return 1000 * v / ref }
	h := part(r.HashMBps, Reference.HashMBps)
	m := part(r.MemReadsPerS, Reference.MemReadsPerS)
	s := part(r.RoundTripsPerS, Reference.RoundTripsPerS)
	return Score{
		Index: int(math.Round(math.Cbrt(h * m * s))),
		Hash:  int(math.Round(h)), Memory: int(math.Round(m)), Syscalls: int(math.Round(s)),
	}
}

// Run times each part for about d, after a short warm-up.
func Run(ctx context.Context, d time.Duration) (Result, error) {
	var r Result
	r.HashMBps = hash(d)
	r.MemReadsPerS = memory(d)
	rt, err := roundTrips(ctx, d)
	if err != nil {
		return r, err
	}
	r.RoundTripsPerS = rt
	return r, nil
}

func hash(d time.Duration) float64 {
	buf := make([]byte, 64<<10)
	for i := range buf {
		buf[i] = byte(i)
	}
	sum := sha256.Sum256(buf) // warm-up
	n, start := 0, time.Now()
	for time.Since(start) < d {
		for range 16 {
			sum = sha256.Sum256(append(buf[:len(buf)-len(sum)], sum[:]...))
			n++
		}
	}
	return float64(n*len(buf)) / (1 << 20) / time.Since(start).Seconds()
}

// memory chases a random cycle through 64 MiB, so each read depends on the
// last and the prefetcher can't help.
func memory(d time.Duration) float64 {
	const size = 16 << 20 // uint32s: 64 MiB
	// Sattolo's shuffle turns the identity into one cycle through every
	// slot, in place.
	next := make([]uint32, size)
	for i := range next {
		next[i] = uint32(i)
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for i := size - 1; i > 0; i-- {
		j := rng.IntN(i)
		next[i], next[j] = next[j], next[i]
	}
	p := uint32(0)
	for range 1 << 16 { // warm-up
		p = next[p]
	}
	n, start := 0, time.Now()
	for time.Since(start) < d {
		for range 1 << 14 {
			p = next[p]
		}
		n += 1 << 14
	}
	if p >= size { // never true; keeps the chase from being optimized away
		return 0
	}
	return float64(n) / time.Since(start).Seconds()
}

func roundTrips(ctx context.Context, d time.Duration) (float64, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		b := make([]byte, 1)
		for {
			if _, err := io.ReadFull(c, b); err != nil {
				return
			}
			if _, err := c.Write(b); err != nil {
				return
			}
		}
	}()
	var dialer net.Dialer
	c, err := dialer.DialContext(ctx, "tcp", ln.Addr().String())
	if err != nil {
		return 0, err
	}
	defer c.Close()
	b := []byte{1}
	trip := func() error {
		if _, err := c.Write(b); err != nil {
			return err
		}
		_, err := io.ReadFull(c, b)
		return err
	}
	for range 1000 { // warm-up
		if err := trip(); err != nil {
			return 0, err
		}
	}
	n, start := 0, time.Now()
	for time.Since(start) < d {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		for range 100 {
			if err := trip(); err != nil {
				return 0, err
			}
		}
		n += 100
	}
	if n == 0 {
		return 0, errors.New("no round trips")
	}
	return float64(n) / time.Since(start).Seconds(), nil
}

// Tolerance is how far apart two runs' speed indexes can be and still
// count as the same speed: a fanless laptop lost about 14% over four
// back-to-back runs as it heated up, and its break times moved with it.
const Tolerance = 0.10

// Gap is how far apart two speed indexes are, as a share of the faster.
func Gap(x, y int) float64 {
	hi := max(x, y)
	if hi <= 0 {
		return 0
	}
	return math.Abs(float64(x-y)) / float64(hi)
}
