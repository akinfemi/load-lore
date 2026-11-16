package analysis

import (
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/plan"
)

// queueRun is twelve windows of a queue with a two-replica worker of five
// runners each. Until 0:50 the workers keep up; from then on, jobs arrive at
// publish a second and are finished at finish, so the queue grows. cpu is the
// worker's CPU from 0:50.
func queueRun(publish, finish, cpu float64) *Analysis {
	const lagFrom = 5
	a := &Analysis{Plan: &plan.Plan{Services: []*plan.Service{
		{Name: "jobs", Kind: "queue"},
		{Name: "sender", Kind: "app", Role: "worker", CPU: 0.5, Containers: []string{"sender-0", "sender-1"}, Consume: &plan.Consume{Queue: "jobs", Concurrency: 5}},
	}}}
	depth := 0.0
	for i := range 12 {
		pub, fin, c, mean, running := 50.0, 50.0, 0.4, 10.0, 1.0
		if i >= lagFrom {
			pub, fin, c, mean, running = publish, finish, cpu, 70, 10
		}
		depth += (pub - fin) * WindowSeconds
		oldest := 0.0
		if fin > 0 && depth > 0 {
			oldest = depth / fin
		}
		d, o := depth, oldest
		a.Windows = append(a.Windows, &Window{Index: i, Start: float64(i * WindowSeconds), Services: map[string]*ServiceWindow{
			"jobs":   {Depth: &d, DepthEnd: &d, OldestJobS: &o, Published: &pub, Finished: &fin},
			"sender": {CPU: &c, Replicas: 2, Jobs: &fin, JobMeanMs: &mean, RunningMean: &running},
		}})
	}
	return a
}

func TestConsumerLag(t *testing.T) {
	// CPU-bound workers fall behind: the advice is replicas, not runners.
	d := consumerLag(queueRun(190, 98, 1))
	if len(d) != 1 || d[0].Rule != "consumer-lag" || d[0].Title != "Consumer lag (jobs)" {
		t.Fatalf("lag: %+v", d)
	}
	s := d[0].Summary
	for _, want := range []string{"From 0:50, jobs went into jobs faster than sender finished them", "190 a second published against 98 finished", "CPU was at 100% of its limit", "70 ms against 10 ms before", "still queued"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary lacks %q: %s", want, s)
		}
	}
	if !strings.Contains(d[0].Next, "more replicas") {
		t.Errorf("next: %s", d[0].Next)
	}
	// Workers with spare CPU and every runner busy wait on their calls.
	d = consumerLag(queueRun(190, 98, 0.5))
	if len(d) != 1 || !strings.Contains(d[0].Summary, "All 10 of sender's job runners were busy") || !strings.Contains(d[0].Next, "consume.concurrency") {
		t.Fatalf("runner-bound: %+v", d)
	}
	// Keeping up, no lag.
	if d := consumerLag(queueRun(50, 50, 0.4)); len(d) != 0 {
		t.Errorf("fired while keeping up: %+v", d)
	}
	// One window behind, then caught up, isn't sustained.
	a := queueRun(190, 98, 1)
	for _, w := range a.Windows[6:] {
		q, one := w.Services["jobs"], 1.0
		q.DepthEnd, q.Depth, q.OldestJobS = a.Windows[5].Services["jobs"].DepthEnd, a.Windows[5].Services["jobs"].DepthEnd, &one
	}
	if d := consumerLag(a); len(d) != 0 {
		t.Errorf("fired on one window: %+v", d)
	}
}

// A queue that filled up is named full, with the refused publishes.
func TestQueueFull(t *testing.T) {
	a := queueRun(190, 98, 1)
	used, refused := 0.95, 40.0
	a.Windows[9].Services["jobs"].QueueUsed = &used
	a.Windows[9].Services["jobs"].QueueFull = &refused
	d := consumerLag(a)
	if len(d) != 1 || d[0].Title != "Queue full (jobs)" || !strings.Contains(d[0].Summary, "By 1:30 it was full (95% of its maxmemory), and it refused up to 40 new jobs a second") {
		t.Fatalf("full: %+v", d)
	}
}

// A held run with a backlog says so in its verdict.
func TestQueueNote(t *testing.T) {
	a := queueRun(190, 98, 1)
	a.Verdict.Kind = VerdictHeld
	a.Diagnoses = consumerLag(a)
	if n := queueNote(a); !strings.Contains(n, "But jobs backed up in jobs: the oldest waited") || !strings.Contains(n, "still waiting when the load ended") {
		t.Errorf("note %q", n)
	}
	a.Verdict.Kind = VerdictBroke
	if n := queueNote(a); n != "" {
		t.Errorf("note on a broken run: %q", n)
	}
}

// Failed jobs are dropped, not retried, and the verdict says so.
func TestDroppedNote(t *testing.T) {
	a := queueRun(50, 50, 0.4)
	if n := droppedNote(a); n != "" {
		t.Errorf("note without failures: %q", n)
	}
	rate, none := 0.2, 0.0
	for i, w := range a.Windows {
		if i >= 6 && i < 9 {
			w.Services["jobs"].JobErrorRate = &rate
		} else {
			w.Services["jobs"].JobErrorRate = &none
		}
	}
	// Three windows of 50 jobs a second, a fifth of them failing: 300.
	n := droppedNote(a)
	if !strings.Contains(n, "300 jobs from jobs failed (up to 20% of a window's jobs) and were dropped") || !strings.Contains(n, "doesn't retry failed jobs yet") {
		t.Errorf("note %q", n)
	}
	// A trickle under the bar stays quiet.
	small := 0.005
	for _, w := range a.Windows {
		w.Services["jobs"].JobErrorRate = &small
	}
	if n := droppedNote(a); n != "" {
		t.Errorf("note under the bar: %q", n)
	}
}
