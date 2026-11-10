// Package hoststat samples the CPU time of the kernel Docker runs on, from
// /proc/stat, when that kernel is this machine's. The analysis uses it to
// name the cause when tooling was short of CPU: other virtual machines on
// the same physical host (steal), or other programs on this machine.
//
// Inside Colima or Docker Desktop, Docker's kernel runs in a VM and loadl
// on the host can't read it, and the VM itself can't see what the host's
// other programs take from it, so there is nothing to sample.
package hoststat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Metric is the file name the samples are written under, in the run's
// metrics folder, shaped like a Prometheus range query.
const Metric = "loadlore_host_cpu_seconds_total"

// Modes are /proc/stat's first columns, in order.
var Modes = []string{"user", "nice", "system", "idle", "iowait", "irq", "softirq", "steal"}

// userHZ is the unit of /proc/stat: USER_HZ, 100 on every Linux build
// Docker supports.
const userHZ = 100

// SameKernel reports whether Docker's kernel is this machine's: loadl runs
// on Linux and its kernel release and hostname match Docker's.
func SameKernel(dockerKernel, dockerName string) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	rel, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil || strings.TrimSpace(string(rel)) != dockerKernel {
		return false
	}
	name, err := os.Hostname()
	return err == nil && name == dockerName
}

// Sampler reads /proc/stat at a fixed interval until its context ends.
type Sampler struct {
	path string
	mu   sync.Mutex
	ts   []float64
	vals [][]float64 // per sample, seconds per mode
	done chan struct{}
}

// Start samples every interval until ctx ends; Wait returns after the last.
func Start(ctx context.Context, interval time.Duration) *Sampler {
	return start(ctx, "/proc/stat", interval)
}

func start(ctx context.Context, path string, interval time.Duration) *Sampler {
	s := &Sampler{path: path, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			s.sample()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return s
}

// Wait blocks until sampling has stopped.
func (s *Sampler) Wait() { <-s.done }

func (s *Sampler) sample() {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	v, ok := parse(b)
	if !ok {
		return
	}
	now := float64(time.Now().UnixMilli()) / 1000
	s.mu.Lock()
	s.ts, s.vals = append(s.ts, now), append(s.vals, v)
	s.mu.Unlock()
}

// parse reads the aggregate "cpu" line, in seconds per mode.
func parse(b []byte) ([]float64, bool) {
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 || f[0] != "cpu" {
			continue
		}
		if len(f) < 1+len(Modes) {
			return nil, false
		}
		v := make([]float64, len(Modes))
		for i := range Modes {
			n, err := strconv.ParseFloat(f[1+i], 64)
			if err != nil {
				return nil, false
			}
			v[i] = n / userHZ
		}
		return v, true
	}
	return nil, false
}

// JSON returns the samples as a Prometheus range-query result, one series
// per mode, so the analysis reads them like any exported metric. It
// returns nil when nothing was sampled.
func (s *Sampler) JSON() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ts) == 0 {
		return nil, nil
	}
	type res struct {
		Metric map[string]string `json:"metric"`
		Values [][2]any          `json:"values"`
	}
	out := struct {
		ResultType string `json:"resultType"`
		Result     []res  `json:"result"`
	}{ResultType: "matrix"}
	for m, mode := range Modes {
		r := res{Metric: map[string]string{"__name__": Metric, "mode": mode}}
		for i, t := range s.ts {
			r.Values = append(r.Values, [2]any{t, fmt.Sprintf("%g", s.vals[i][m])})
		}
		out.Result = append(out.Result, r)
	}
	return json.Marshal(out)
}
