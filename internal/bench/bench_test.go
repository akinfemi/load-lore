package bench

import (
	"context"
	"testing"
	"time"
)

func TestScored(t *testing.T) {
	r := Result{HashMBps: Reference.HashMBps, MemReadsPerS: Reference.MemReadsPerS / 8, RoundTripsPerS: Reference.RoundTripsPerS}
	if s := r.Scored(); s.Hash != 1000 || s.Memory != 125 || s.Syscalls != 1000 || s.Index != 500 {
		t.Errorf("%+v", s)
	}
}

func TestRun(t *testing.T) {
	if testing.Short() {
		t.Skip("times real work")
	}
	r, err := Run(context.Background(), 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if r.HashMBps <= 0 || r.MemReadsPerS <= 0 || r.RoundTripsPerS <= 0 {
		t.Errorf("%+v", r)
	}
}

func TestMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates 64 MiB")
	}
	if memory(10*time.Millisecond) <= 0 {
		t.Error("no reads")
	}
}
