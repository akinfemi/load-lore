package analysis

import (
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/synthetic"
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
	if !strings.Contains(n, "300 jobs from jobs failed (up to 20% of a window's jobs) and were dropped") || !strings.Contains(n, "don't retry failed jobs (consume.retries)") {
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

	// With retries: the failed runs went to the retry set, some jobs ran
	// out of tries, and some still waited at the end.
	a.Plan.Services[1].Consume.RetryMax = 3
	again, dead := 2.0, 0.0
	for i, w := range a.Windows {
		sw := w.Services["jobs"]
		sw.Retried = &again
		if i >= 8 {
			d := float64(i-7) * 4
			dead = d
		}
		dt, waiting := dead, 9.0
		sw.DeadTotal, sw.Retrying = &dt, &waiting
	}
	n = droppedNote(a)
	if want := " Retries: 240 failed runs of jobs from jobs went to the retry set, to go back on the queue after their wait; 16 jobs failed all 4 tries and are dead; 9 were still waiting to retry when the load ended."; n != want {
		t.Errorf("with retries %q\nwant %q", n, want)
	}
}

// Workers whose writes queue for the same rows: the advice is the rows,
// not more runners.
func TestConsumerLagHotRows(t *testing.T) {
	a := queueRun(190, 98, 0.5)
	a.Plan.Services = append(a.Plan.Services, &plan.Service{Name: "db", Kind: "postgres"})
	a.Plan.Services[1].Synthetic = []*synthetic.Config{{Targets: map[string]synthetic.Target{"db": {}}}}
	for i, w := range a.Windows {
		wr, locks := 2.2, 0.0
		if i >= 5 {
			wr, locks = 15.8, 6.6
		}
		w.Edges = append(w.Edges, &EdgeWindow{From: "sender", To: "db", OpMeanMs: map[string]float64{"write": wr}})
		w.Services["db"] = &ServiceWindow{LockWaits: &locks}
	}
	d := consumerLag(a)
	if len(d) != 1 {
		t.Fatalf("lag: %+v", d)
	}
	for _, want := range []string{"sender's writes to db took 16 ms against 2.2 ms before the lag, with 6.6 backends of db waiting on locks", "More job runners would only wait in line"} {
		if !strings.Contains(d[0].Summary, want) {
			t.Errorf("summary lacks %q: %s", want, d[0].Summary)
		}
	}
	if !strings.Contains(d[0].Next, "Don't add job runners") || strings.Contains(d[0].Next, "consume.concurrency") {
		t.Errorf("next: %s", d[0].Next)
	}
	// Writes as fast as before: the runner advice stands.
	for _, w := range a.Windows {
		w.Edges[len(w.Edges)-1].OpMeanMs["write"] = 2.2
	}
	if d := consumerLag(a); len(d) != 1 || !strings.Contains(d[0].Next, "consume.concurrency") {
		t.Errorf("without hot rows: %+v", d)
	}
}

// Runners all busy but running each job for under half their cycle: the
// queue's round trips set the pace, and the summary says so.
func TestConsumerLagRoundTrips(t *testing.T) {
	a := queueRun(190, 98, 0.5)
	for _, w := range a.Windows[5:] {
		mean := 30.0 // 10 runners finishing 98 jobs/s: about 102 ms a cycle
		w.Services["sender"].JobMeanMs = &mean
	}
	d := consumerLag(a)
	if len(d) != 1 || !strings.Contains(d[0].Summary, "A runner took 102 ms per job, but ran each for only 30 ms: the rest went on the queue's round trips") || !strings.Contains(d[0].Next, "more runners make more round trips at once") {
		t.Fatalf("round trips: %+v", d)
	}
}

// Most jobs failing in the lag: it's put on the failing calls, and the
// advice depends on whether the worker retries.
func TestConsumerLagFailingJobs(t *testing.T) {
	a := queueRun(190, 98, 0.5)
	fail := 0.9
	for _, w := range a.Windows[5:] {
		w.Services["jobs"].JobErrorRate = &fail
	}
	d := consumerLag(a)
	if len(d) != 1 || !strings.Contains(d[0].Summary, "90% of sender's jobs failed at 0:50") || !strings.Contains(d[0].Next, "Without consume.retries the failed jobs were dropped") {
		t.Fatalf("failing jobs: %+v", d)
	}
	a.Plan.Services[1].Consume.RetryMax = 3
	if d := consumerLag(a); len(d) != 1 || !strings.Contains(d[0].Next, "Retries bring the failed jobs back") {
		t.Errorf("with retries: %+v", d)
	}
}

// Jobs a killed worker left, picked up again: the verdict says so.
func TestReclaimedNote(t *testing.T) {
	a := queueRun(50, 50, 0.4)
	r := 0.5
	a.Windows[7].Services["jobs"].Reclaimed = &r
	if n := droppedNote(a); !strings.Contains(n, "5 jobs from jobs that a worker took and never finished") || !strings.Contains(n, "after 30 s idle") {
		t.Errorf("note %q", n)
	}
}
