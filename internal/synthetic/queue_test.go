package synthetic

import (
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestJobRequest(t *testing.T) {
	r := jobRequest(&redis.XMessage{ID: "1-0", Values: map[string]any{jobType: "read", jobKey: "42"}})
	if r != (request{typ: "read", key: 42}) {
		t.Errorf("job request %+v", r)
	}
	// A job without fields runs as a write of key 1.
	if r := jobRequest(&redis.XMessage{ID: "1-0", Values: map[string]any{}}); r != (request{typ: "write", key: 1}) {
		t.Errorf("empty job %+v", r)
	}
}

func TestEnqueuedAt(t *testing.T) {
	at, ok := enqueuedAt("1700000000123-4")
	if !ok || !at.Equal(time.UnixMilli(1700000000123)) {
		t.Errorf("enqueued at %v %v", at, ok)
	}
	if _, ok := enqueuedAt("nonsense"); ok {
		t.Error("parsed a bad id")
	}
}

// serverErr is an error Redis sent back, as go-redis reports one.
type serverErr string

func (e serverErr) Error() string { return string(e) }
func (serverErr) RedisError()     {}

func TestQueueErr(t *testing.T) {
	if Cause(queueErr(serverErr("OOM command not allowed when used memory > 'maxmemory'."))) != CauseQueueFull {
		t.Error("OOM isn't queue_full")
	}
	if Cause(queueErr(serverErr("ERR something"))) != CauseQueueError {
		t.Error("a server error isn't queue_error")
	}
	if err := errors.New("dial tcp: connection refused"); !errors.Is(queueErr(err), err) || Cause(queueErr(err)) == CauseQueueError {
		t.Error("a network error lost its own cause")
	}
}

func TestWorkerConfig(t *testing.T) {
	base := func() *Config {
		return &Config{
			Service: "w", Role: RoleWorker, Concurrency: Concurrency{Model: "threads"},
			Consume: &Consume{Target: "jobs", Concurrency: 2, Consumer: "w-0"},
			Targets: map[string]Target{"jobs": {Kind: KindQueue, Addrs: []string{"q:6379"}, Pool: 3, Stream: "jobs", Group: "workers"}},
		}
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("valid worker: %v", err)
	}
	for name, mod := range map[string]func(*Config){
		"no consume":     func(c *Config) { c.Consume = nil },
		"server consume": func(c *Config) { c.Role = "" },
		"bad role":       func(c *Config) { c.Role = "cron" },
		"not a queue":    func(c *Config) { c.Targets["jobs"] = Target{Kind: KindRedis, Addrs: []string{"q:6379"}, Pool: 3} },
		"no stream":      func(c *Config) { t := c.Targets["jobs"]; t.Stream = ""; c.Targets["jobs"] = t },
		"no runners":     func(c *Config) { c.Consume.Concurrency = 0 },
		"get on a queue": func(c *Config) { c.Calls = []Call{call("jobs", "get")} },
	} {
		c := base()
		mod(c)
		if c.Validate() == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
