package synthetic

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// These use the real clients against local addresses; no Docker needed.

func TestRealClientsReportRefusedConnections(t *testing.T) {
	cfg := baseConfig()
	cfg.Calls = []Call{call("db", "read")}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.handle(context.Background(), request{typ: "read", key: 1}); Cause(err) != CauseConnRefused {
		t.Errorf("postgres: cause %q (%v)", Cause(err), err)
	}

	cfg.Calls = []Call{call("cache", "get")}
	s2, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if err := s2.handle(context.Background(), request{typ: "read", key: 1}); Cause(err) != CauseConnRefused {
		t.Errorf("redis: cause %q (%v)", Cause(err), err)
	}
}

// An HTTP target with a pool of 1: the second concurrent call waits for the
// connection, and that wait shows in loadlore_pool_wait_seconds.
func TestHTTPTargetPoolWait(t *testing.T) {
	backCfg := &Config{Service: "back", Concurrency: Concurrency{Model: "threads"}, CPUMs: 80}
	back, err := New(backCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer back.Close()
	srv := httptest.NewServer(back.Handler())
	defer srv.Close()

	frontCfg := &Config{
		Service:     "front",
		Concurrency: Concurrency{Model: "threads"},
		Resilience:  Resilience{TimeoutMs: 5000},
		Targets:     map[string]Target{"back": {Kind: KindHTTP, Addr: strings.TrimPrefix(srv.URL, "http://"), Pool: 1}},
		Calls:       []Call{call("back", "http")},
	}
	front, err := New(frontCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer front.Close()

	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			if err := front.handle(context.Background(), request{typ: "write", key: int64(i + 1)}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got := metricValue(t, back, "loadlore_requests_total", map[string]string{"type": "write", "code": "200"}); got != 2 {
		t.Errorf("back saw %v write requests", got)
	}
	waited := histogramSum(t, front, "loadlore_pool_wait_seconds", map[string]string{"target": "back"})
	if waited < 0.05 {
		t.Errorf("total pool wait %.3fs; the second call should wait about 80ms for the only connection", waited)
	}
	if time.Duration(waited*float64(time.Second)) > 2*time.Second {
		t.Errorf("pool wait %.3fs is implausibly long", waited)
	}
}
