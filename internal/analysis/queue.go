package analysis

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/akinfemi/load-lore/internal/plan"
)

// Consumer lag: jobs wait in a queue because its workers finish them more
// slowly than they arrive. A window lags when the oldest unfinished job
// had waited ConsumerLagMinS or more, or when the queue held more jobs
// than its workers can run at once and grew, with jobs published
// ConsumerLagGrowth faster than they were finished. Two windows in a row
// fire it. When the queue filled up (QueueFullShare of its maxmemory, or
// publishes refused), it is named a full queue.
const (
	ConsumerLagMinS   = 5.0
	ConsumerLagGrowth = 1.05
	QueueFullShare    = 0.9
	// Workers whose CPU is at WorkerCPUSaturated or more are CPU-bound;
	// below it, with WorkerBusyShare of their job runners busy, they wait
	// on their calls.
	WorkerCPUSaturated = 0.9
	WorkerBusyShare    = 0.9
)

func consumerLag(a *Analysis) []*Diagnosis {
	var out []*Diagnosis
	ws := a.Windows
	if len(ws) == 0 || a.Plan == nil {
		return out
	}
	for _, q := range a.Plan.Services {
		if q.Kind != "queue" {
			continue
		}
		workers := workersOf(a.Plan, q.Name)
		runners := 0
		for _, w := range workers {
			runners += len(w.Running()) * w.Consume.Concurrency
		}
		flags := make([]bool, len(ws))
		for i, w := range ws {
			sw := w.Services[q.Name]
			if sw == nil {
				continue
			}
			waited := sw.OldestJobS != nil && *sw.OldestJobS >= ConsumerLagMinS
			growing := false
			if sw.DepthEnd != nil && *sw.DepthEnd > float64(runners) && sw.Published != nil && i > 0 {
				prev := ws[i-1].Services[q.Name]
				finished := 0.0
				if sw.Finished != nil {
					finished = *sw.Finished
				}
				growing = prev != nil && prev.DepthEnd != nil && *sw.DepthEnd > *prev.DepthEnd && *sw.Published >= ConsumerLagGrowth*finished
			}
			flags[i] = waited || growing
		}
		from, to, ok := sustained(flags)
		if !ok {
			continue
		}
		out = append(out, lagDiagnosis(a, q, workers, runners, from, to))
	}
	return out
}

// workersOf lists the workers that consume from queue.
func workersOf(p *plan.Plan, queue string) []*plan.Service {
	var out []*plan.Service
	for _, s := range p.Services {
		if s.Consume != nil && s.Consume.Queue == queue {
			out = append(out, s)
		}
	}
	return out
}

func lagDiagnosis(a *Analysis, q *plan.Service, workers []*plan.Service, runners, from, to int) *Diagnosis {
	ws := a.Windows
	at := func(i int) *ServiceWindow { return ws[i].Services[q.Name] }
	// The deepest window from the lag on shows it best.
	worst := from
	for i := from; i < len(ws); i++ {
		if depthOf(at(i)) > depthOf(at(worst)) {
			worst = i
		}
	}
	w, sw := ws[worst], at(worst)
	// The rates are clearest where published ran furthest ahead of
	// finished; once a queue is full, it takes only what its workers free.
	gap := from
	for i := from; i < len(ws); i++ {
		if gapOf(at(i)) > gapOf(at(gap)) {
			gap = i
		}
	}
	gw, gsw := ws[gap], at(gap)
	names := make([]string, 0, len(workers))
	for _, wk := range workers {
		names = append(names, wk.Name)
	}
	who := joinAnd(names)
	if who == "" {
		who = "its workers"
	}
	d := &Diagnosis{Rule: "consumer-lag", Title: fmt.Sprintf("Consumer lag (%s)", q.Name), From: ws[from].Start, To: ws[to].Start + WindowSeconds, comp: q.Name}

	rates := ""
	if gsw.Published != nil && gsw.Finished != nil {
		rates = fmt.Sprintf(": at %s, %s a second published against %s finished", Clock(gw.Start), Rate(*gsw.Published), Rate(*gsw.Finished))
		d.Evidence = append(d.Evidence,
			Evidence{Text: fmt.Sprintf("%s jobs a second published to %s at %s", Rate(*gsw.Published), q.Name, Clock(gw.Start)), Source: SrcPublished},
			Evidence{Text: fmt.Sprintf("%s jobs a second finished by %s at %s", Rate(*gsw.Finished), who, Clock(gw.Start)), Source: SrcJobs})
		// Published is new jobs; retries put back are on top of them, and
		// a failed run that goes to retry isn't done.
		if gsw.Retried != nil && *gsw.Retried >= 0.5 {
			rates += fmt.Sprintf(", %s of them failed runs sent to wait for a retry", Rate(*gsw.Retried))
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s failed runs a second sent to %s's retry set at %s, and %s retries a second put back", Rate(*gsw.Retried), q.Name, Clock(gw.Start), Rate(deref0(gsw.Requeued))), Source: SrcJobRetries})
		}
	}
	grew := ""
	if sw.Depth != nil {
		grew = fmt.Sprintf(" The queue grew to %s jobs", Count(*sw.Depth))
		d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s jobs in %s at %s, against %d job %s across %s", Count(*sw.Depth), q.Name, Clock(w.Start), runners, plural(runners, "runner", "runners"), who), Source: SrcQueueDepth})
		if old := maxOldest(ws, q.Name, from); old > 0 {
			grew += fmt.Sprintf(", and the oldest had waited %s", secondsText(old))
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("the oldest unfinished job had waited %s", secondsText(old)), Source: SrcOldestJob})
		}
		if sw.Retrying != nil && *sw.Retrying >= 1 {
			grew += fmt.Sprintf(", with %s more waiting to retry", Count(*sw.Retrying))
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s jobs in %s's retry set at %s", Count(*sw.Retrying), q.Name, Clock(w.Start)), Source: SrcJobRetries})
		}
		grew += "."
	}
	if sw.JobWaitP99 != nil {
		d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("p99 wait of the jobs taken at %s: %s", Clock(w.Start), Ms(*sw.JobWaitP99)), Source: SrcJobWait})
	}

	why, next := "", ""
	if len(workers) > 0 {
		wk := workers[0]
		ww := w.Services[wk.Name]
		per := wk.Consume.Concurrency
		base := jobBaseline(ws, wk.Name, from)
		if ww != nil && ww.CPU != nil {
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s CPU at %s of its limit at %s, over %d %s", wk.Name, Pct(*ww.CPU), Clock(w.Start), ww.Replicas, plural(ww.Replicas, "replica", "replicas")), Source: SrcCPU})
		}
		if ww != nil && ww.JobMeanMs != nil {
			ev := fmt.Sprintf("%s per job on %s at %s", Ms(*ww.JobMeanMs), wk.Name, Clock(w.Start))
			if base > 0 {
				ev += fmt.Sprintf(", against %s before the lag", Ms(base))
			}
			d.Evidence = append(d.Evidence, Evidence{Text: ev, Source: SrcJobDuration})
		}
		if ww != nil && ww.RunningMean != nil {
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s of %s's %d job runners busy on average at %s", Count(*ww.RunningMean), wk.Name, runners, Clock(w.Start)), Source: SrcJobsRunning})
		}
		peak := ""
		if sw.Finished != nil && sw.Published != nil {
			peak = fmt.Sprintf(" Its workers finished at most about %s jobs a second, against %s published.", Rate(maxFinished(ws, q.Name)), Rate(maxPublished(ws, q.Name)))
		}
		hot := hotJobs(a, wk, worst, from)
		if hot != nil {
			d.Evidence = append(d.Evidence,
				Evidence{Text: fmt.Sprintf("%s's writes to %s took %s at %s, against %s before the lag", wk.Name, hot.db, Ms(hot.write), Clock(w.Start), Ms(hot.base)), Source: SrcOpTime},
				Evidence{Text: fmt.Sprintf("%s backends of %s waiting on locks at %s, on average", lockText(hot.locks), hot.db, Clock(w.Start)), Source: SrcLockWaits})
		}
		// The lag window where the most of the jobs failed.
		fail, fi := 0.0, from
		for i := from; i <= to && i < len(ws); i++ {
			if qs := at(i); qs != nil && qs.JobErrorRate != nil && *qs.JobErrorRate > fail {
				fail, fi = *qs.JobErrorRate, i
			}
		}
		switch {
		case fail >= JobFailingShare && hot == nil:
			took := ""
			if fw := ws[fi].Services[wk.Name]; fw != nil && fw.JobMeanMs != nil && base > 0 {
				took = fmt.Sprintf(" (a job took %s against %s before)", Ms(*fw.JobMeanMs), Ms(base))
			}
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s of the jobs %s finished at %s failed", Pct(fail), wk.Name, Clock(ws[fi].Start)), Source: SrcJobs})
			why = fmt.Sprintf(" %s of %s's jobs failed at %s, each holding its runner until its calls gave up%s: the runners spent the lag on calls that failed, not on finishing jobs.", Pct(fail), wk.Name, Clock(ws[fi].Start), took)
			next = fmt.Sprintf("The backlog came from the failing calls, not from too few runners: fix what %s's jobs call, or fail those calls sooner (a shorter timeout, or a circuit breaker) so the runners move on.%s", wk.Name, peak)
			if wk.Consume.RetryMax > 0 {
				next += " Retries bring the failed jobs back once the calls answer: check the oldest job's wait, and that none went dead."
			} else {
				next += " Without consume.retries the failed jobs were dropped; real workers would retry them, and the backlog would have grown more."
			}
		case hot != nil:
			why = fmt.Sprintf(" %s's writes to %s took %s against %s before the lag, with %s backends of %s waiting on locks: its jobs update the same rows, and a row takes one write at a time, each keeping its lock until it commits. More job runners would only wait in line for the same rows.", wk.Name, hot.db, Ms(hot.write), Ms(hot.base), lockText(hot.locks), hot.db)
			next = fmt.Sprintf("Don't add job runners or replicas to %s: the hot rows set its pace. Make the writes to them fewer or cheaper: apply many jobs in one write (a batch), or spread them over more rows.%s If that pace catches up after the peak, the queue is doing its job: check that the oldest job's wait is one your users accept.", wk.Name, peak)
		case ww != nil && ww.CPU != nil && *ww.CPU >= WorkerCPUSaturated:
			slower := ""
			if ww.JobMeanMs != nil && base > 0 && *ww.JobMeanMs >= 1.5*base {
				slower = fmt.Sprintf(": each job took %s against %s before, as its %d job runners per replica shared %s", Ms(*ww.JobMeanMs), Ms(base), per, cpuText(wk.CPU))
			}
			why = fmt.Sprintf(" %s's CPU was at %s of its limit%s. More job runners per replica wouldn't finish more jobs; more replicas, or less work per job, would.", wk.Name, Pct(*ww.CPU), slower)
			next = fmt.Sprintf("Give %s the capacity for the peak: more replicas, or less CPU per job.%s A queue smooths a burst only when its workers can catch up afterwards; it adds no capacity.", wk.Name, peak)
		case ww != nil && ww.RunningMean != nil && runners > 0 && *ww.RunningMean >= WorkerBusyShare*float64(runners):
			why = fmt.Sprintf(" All %d of %s's job runners were busy, while its CPU stayed below its limit: each job spent most of its time waiting on its calls.", runners, wk.Name)
			next = fmt.Sprintf("Raise %s's consume.concurrency, or add replicas, if what its jobs call has room for more at once; otherwise speed up those calls.%s", wk.Name, peak)
			// A runner's cycle is taking the job, running it and
			// acknowledging it: when running it was under half of that,
			// the queue's own round trips set the pace.
			if ww.Jobs != nil && *ww.Jobs > 0 && ww.JobMeanMs != nil {
				cycle := *ww.RunningMean / *ww.Jobs * 1000
				if *ww.JobMeanMs < 0.5*cycle {
					why = fmt.Sprintf(" All %d of %s's job runners were busy, while its CPU stayed below its limit. A runner took %s per job, but ran each for only %s: the rest went on the queue's round trips, taking each job and acknowledging it, one job at a time per runner.", runners, wk.Name, Ms(cycle), Ms(*ww.JobMeanMs))
					next = fmt.Sprintf("Raise %s's consume.concurrency, or add replicas: more runners make more round trips at once.%s", wk.Name, peak)
				}
			}
		default:
			next = fmt.Sprintf("Find what limits %s: its CPU, its job runners (consume.concurrency) or the calls its jobs make, and give it room for the peak.%s", wk.Name, peak)
		}
	} else {
		why = " Nothing consumes from it."
		next = fmt.Sprintf("Add a worker: a service with role: worker and consume: {from: %s}.", q.Name)
	}

	full := ""
	if fi := firstFull(ws, q.Name); fi >= 0 {
		d.Title = fmt.Sprintf("Queue full (%s)", q.Name)
		fw := ws[fi].Services[q.Name]
		full = fmt.Sprintf(" By %s it was full", Clock(ws[fi].Start))
		if fw.QueueUsed != nil {
			full += fmt.Sprintf(" (%s of its maxmemory)", Pct(*fw.QueueUsed))
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s's memory at %s of its maxmemory at %s", q.Name, Pct(*fw.QueueUsed), Clock(ws[fi].Start)), Source: SrcQueueMemory})
		}
		if ri, refused := mostRefused(ws, q.Name, fi); refused > 0 {
			full += fmt.Sprintf(", and it refused up to %s new jobs a second: the requests that published them failed", Rate(refused))
			d.Evidence = append(d.Evidence, Evidence{Text: fmt.Sprintf("%s publishes a second refused with queue_full at %s", Rate(refused), Clock(ws[ri].Start)), Source: SrcPublishErrors})
		}
		full += "."
	}

	left := ""
	if last := at(len(ws) - 1); last != nil && last.DepthEnd != nil && *last.DepthEnd > float64(runners) {
		left = fmt.Sprintf(" When the load ended, %s jobs were still queued", Count(*last.DepthEnd))
		if f := maxFinished(ws, q.Name); f > 0 {
			left += fmt.Sprintf(": about %s of work at the most %s finished, if no more arrived", roughDuration(*last.DepthEnd/f), who)
		}
		left += "."
	}
	d.Summary = fmt.Sprintf("From %s, jobs went into %s faster than %s finished them%s.%s%s%s%s",
		Clock(ws[from].Start), q.Name, who, rates, grew, why, full, left)
	d.Next = next
	return d
}

func depthOf(sw *ServiceWindow) float64 {
	if sw == nil || sw.Depth == nil {
		return math.Inf(-1)
	}
	return *sw.Depth
}

// gapOf is how far published ran ahead of finished in a window.
func gapOf(sw *ServiceWindow) float64 {
	if sw == nil || sw.Published == nil {
		return math.Inf(-1)
	}
	f := 0.0
	if sw.Finished != nil {
		f = *sw.Finished
	}
	return *sw.Published - f
}

// mostRefused is the window from start with the most publishes a second
// refused because the queue was full, and that rate.
func mostRefused(ws []*Window, queue string, start int) (int, float64) {
	at, mx := start, 0.0
	for i := start; i < len(ws); i++ {
		if sw := ws[i].Services[queue]; sw != nil && sw.QueueFull != nil && *sw.QueueFull > mx {
			at, mx = i, *sw.QueueFull
		}
	}
	return at, mx
}

func maxOldest(ws []*Window, queue string, from int) float64 {
	mx := 0.0
	for _, w := range ws[from:] {
		if sw := w.Services[queue]; sw != nil && sw.OldestJobS != nil {
			mx = math.Max(mx, *sw.OldestJobS)
		}
	}
	return mx
}

func maxFinished(ws []*Window, queue string) float64 {
	mx := 0.0
	for _, w := range ws {
		if sw := w.Services[queue]; sw != nil && sw.Finished != nil {
			mx = math.Max(mx, *sw.Finished)
		}
	}
	return mx
}

func maxPublished(ws []*Window, queue string) float64 {
	mx := 0.0
	for _, w := range ws {
		if sw := w.Services[queue]; sw != nil && sw.Published != nil {
			mx = math.Max(mx, *sw.Published)
		}
	}
	return mx
}

// jobBaseline is a worker's quickest mean job time before the lag.
func jobBaseline(ws []*Window, worker string, before int) float64 {
	best := 0.0
	for _, w := range ws[:before] {
		if sw := w.Services[worker]; sw != nil && sw.JobMeanMs != nil && (best == 0 || *sw.JobMeanMs < best) {
			best = *sw.JobMeanMs
		}
	}
	return best
}

// firstFull is the first window in which the queue was full: its memory
// at QueueFullShare of maxmemory, or publishes refused. -1 if never.
func firstFull(ws []*Window, queue string) int {
	for i, w := range ws {
		sw := w.Services[queue]
		if sw != nil && (sw.QueueUsed != nil && *sw.QueueUsed >= QueueFullShare || sw.QueueFull != nil && *sw.QueueFull > 0) {
			return i
		}
	}
	return -1
}

// roughDuration is a duration to a sensible precision: seconds under two
// minutes, then minutes.
func roughDuration(s float64) string {
	switch {
	case s < 10:
		return fmt.Sprintf("%.1f s", s)
	case s < 120:
		return fmt.Sprintf("%.0f s", s)
	}
	return fmt.Sprintf("%.0f minutes", s/60)
}

func cpuText(cpu float64) string {
	switch cpu {
	case 0.5:
		return "half a CPU"
	case 1:
		return "one CPU"
	}
	return strings.TrimSuffix(fmt.Sprintf("%.2f", cpu), "0") + " CPU"
}

// queueNote is the verdict's word on a backlog in a run that held: the
// requests that published the jobs were answered fast, so the verdict
// alone would miss jobs left waiting. Empty otherwise.
func queueNote(a *Analysis) string {
	if a.Verdict.Kind != VerdictHeld {
		return ""
	}
	for _, d := range a.Diagnoses {
		if d.Rule != "consumer-lag" {
			continue
		}
		ws := a.Windows
		old := maxOldest(ws, d.comp, 0)
		end := ""
		if last := ws[len(ws)-1].Services[d.comp]; last != nil && last.DepthEnd != nil && *last.DepthEnd > 0 {
			end = fmt.Sprintf(", %s still waiting when the load ended", Count(*last.DepthEnd))
		}
		return fmt.Sprintf(" But jobs backed up in %s: the oldest waited %s%s (see consumer lag).", d.comp, secondsText(old), end)
	}
	return ""
}

// DroppedJobShare is the share of a window's finished jobs that failed
// from which droppedNote speaks up.
const DroppedJobShare = 0.01

// JobFailingShare is the share of a window's finished jobs that failed
// for consumer lag to put the lag on the failing calls.
const JobFailingShare = 0.2

// droppedNote is the verdict's word on failed jobs. Without retries a
// worker drops a job whose calls fail, where real job systems retry it
// (Sidekiq up to 25 times), so the jobs would have come back and the queue
// would have held more than this run shows; it says so when a window had
// DroppedJobShare of its jobs fail. With retries it says how many failed
// runs went to the retry set, how many jobs ran out of retries and are
// dead, and how many still waited to retry when the load ended.
func droppedNote(a *Analysis) string {
	if a.Plan == nil {
		return ""
	}
	var dropped, retried []string
	for _, q := range a.Plan.Services {
		if q.Kind != "queue" {
			continue
		}
		maxRetries := 0
		for _, w := range a.Plan.Services {
			if w.Consume != nil && w.Consume.Queue == q.Name {
				maxRetries = max(maxRetries, w.Consume.RetryMax)
			}
		}
		failed, worst, again, dead, waiting := 0.0, 0.0, 0.0, 0.0, 0.0
		for _, w := range a.Windows {
			sw := w.Services[q.Name]
			if sw == nil {
				continue
			}
			if sw.Finished != nil && sw.JobErrorRate != nil {
				failed += *sw.Finished * *sw.JobErrorRate * WindowSeconds
				worst = math.Max(worst, *sw.JobErrorRate)
			}
			if sw.Retried != nil {
				again += *sw.Retried * WindowSeconds
			}
			if sw.DeadTotal != nil {
				dead = *sw.DeadTotal
			}
			if sw.Retrying != nil {
				waiting = *sw.Retrying
			}
		}
		if maxRetries == 0 {
			if worst >= DroppedJobShare && failed >= 1 {
				dropped = append(dropped, fmt.Sprintf("%s jobs from %s failed (up to %s of a window's jobs) and were dropped", Count(failed), q.Name, Pct(worst)))
			}
			continue
		}
		if again < 1 && dead < 1 {
			continue
		}
		n := fmt.Sprintf("%s failed runs of jobs from %s went to the retry set, to go back on the queue after their wait", Count(again), q.Name)
		if dead >= 1 {
			n += fmt.Sprintf("; %s jobs failed all %d tries and are dead", Count(dead), maxRetries+1)
		}
		if waiting >= 1 {
			n += fmt.Sprintf("; %s were still waiting to retry when the load ended", Count(waiting))
		}
		retried = append(retried, n)
	}
	out := ""
	if len(retried) > 0 {
		out += fmt.Sprintf(" Retries: %s.", strings.Join(retried, ". "))
	}
	if len(dropped) > 0 {
		out += fmt.Sprintf(" %s: this design's workers don't retry failed jobs (consume.retries), so real workers retrying them would have had a bigger backlog than this run shows.", strings.Join(dropped, "; "))
	}
	return out
}

// hotWrites is a worker's writes to a Postgres slowed by row locks.
type hotWrites struct {
	db                 string
	write, base, locks float64
}

// hotJobs finds a Postgres the worker's jobs write to whose writes, in
// window i, took HotWriteFactor times their time before the lag (and
// HotMinMs more) while HotLockWaitsMin connections waited on locks: the
// jobs queue for the same rows. Nil if none.
func hotJobs(a *Analysis, wk *plan.Service, i, from int) *hotWrites {
	if len(wk.Synthetic) == 0 {
		return nil
	}
	ws := a.Windows
	dbs := make([]string, 0, len(wk.Synthetic[0].Targets))
	for t := range wk.Synthetic[0].Targets {
		if svc := a.Plan.Service(t); svc != nil && svc.Kind == "postgres" {
			dbs = append(dbs, t)
		}
	}
	sort.Strings(dbs)
	for _, db := range dbs {
		write := func(w *Window) (float64, bool) {
			if e := edgeAt(w, wk.Name, db); e != nil {
				v, ok := e.OpMeanMs["write"]
				return v, ok
			}
			return 0, false
		}
		base := 0.0
		for _, w := range ws[:from] {
			if v, ok := write(w); ok && v > 0 && (base == 0 || v < base) {
				base = v
			}
		}
		wr, ok := write(ws[i])
		sw := ws[i].Services[db]
		if base == 0 || !ok || sw == nil || sw.LockWaits == nil {
			continue
		}
		if *sw.LockWaits >= HotLockWaitsMin && wr >= HotWriteFactor*base && wr >= base+HotMinMs {
			return &hotWrites{db: db, write: wr, base: base, locks: *sw.LockWaits}
		}
	}
	return nil
}

// deref0 is *p, or 0 for nil.
func deref0(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
