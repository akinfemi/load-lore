package hoststat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const stat = `cpu  1000 20 300 50000 40 5 6 70 0 0
cpu0 500 10 150 25000 20 2 3 35 0 0
intr 12345
`

func TestParse(t *testing.T) {
	v, ok := parse([]byte(stat))
	if !ok {
		t.Fatal("not parsed")
	}
	want := []float64{10, 0.2, 3, 500, 0.4, 0.05, 0.06, 0.7}
	for i := range want {
		if v[i] != want[i] {
			t.Errorf("%s = %v, want %v", Modes[i], v[i], want[i])
		}
	}
	if _, ok := parse([]byte("cpu 1 2 3\n")); ok {
		t.Error("a short line parsed")
	}
}

func TestSamplerJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stat")
	if err := os.WriteFile(path, []byte(stat), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := start(ctx, path, 10*time.Millisecond)
	time.Sleep(35 * time.Millisecond)
	cancel()
	s.Wait()
	b, err := s.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Values [][2]any          `json:"values"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Result) != len(Modes) || out.Result[7].Metric["mode"] != "steal" || out.Result[7].Metric["__name__"] != Metric {
		t.Fatalf("%s", b)
	}
	if n := len(out.Result[7].Values); n < 2 || out.Result[7].Values[0][1] != "0.7" {
		t.Errorf("steal values %v", out.Result[7].Values)
	}
}

func TestEmptySamplerWritesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := start(ctx, filepath.Join(t.TempDir(), "missing"), time.Hour)
	s.Wait()
	if b, err := s.JSON(); b != nil || err != nil {
		t.Errorf("%s %v", b, err)
	}
}
