package diskprobe

import (
	"testing"
	"time"
)

func TestSummarize(t *testing.T) {
	var ds []time.Duration
	for i := 1; i <= 100; i++ {
		ds = append(ds, time.Duration(i)*time.Microsecond)
	}
	s := Summarize(ds)
	if s.N != 100 || s.MeanUs != 50.5 || s.P5Us != 5 || s.P50Us != 50 || s.P95Us != 95 || s.P99Us != 99 {
		t.Fatalf("got %+v", s)
	}
	if (Summarize(nil) != Stats{}) {
		t.Fatal("empty input should give zero stats")
	}
}
