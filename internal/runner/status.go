package runner

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/rundir"
)

// Run phases, in order.
const (
	PhasePreflight  = "preflight"
	PhaseStarting   = "starting" // data, stack, monitoring
	PhaseLoad       = "load"
	PhaseCollecting = "collecting"
	PhaseTeardown   = "teardown"
	PhaseFinished   = "finished"
)

// Status is a run's progress. The run rewrites it in status.json as it
// goes, for loadl status and loadl watch, and a last time when it ends.
type Status struct {
	RunID  string `json:"run_id"`
	Design string `json:"design"`
	PID    int    `json:"pid"`           // the loadl process running it
	Log    string `json:"log,omitempty"` // its output, for a detached run
	Phase  string `json:"phase"`

	Started     time.Time  `json:"started"`
	LoadStarted *time.Time `json:"load_started,omitempty"`
	ProfileS    float64    `json:"profile_s"`      // the load profile's length
	ElapsedS    float64    `json:"load_elapsed_s"` // into the load, at the last sample
	// Sample is the latest progress sample: the last 10 seconds.
	Sample *driver.Sample `json:"sample,omitempty"`
	// BreakConfirmedS and StopsAtS are set once the live check confirms a
	// break: when, and when the load stops.
	BreakConfirmedS *float64 `json:"break_confirmed_at_s,omitempty"`
	BreakAtS        *float64 `json:"break_at_s,omitempty"`
	StopsAtS        *float64 `json:"stops_at_s,omitempty"`

	// Once finished: as in run.json.
	Status         string `json:"status,omitempty"`
	Error          string `json:"error,omitempty"`
	Verdict        string `json:"verdict,omitempty"`
	VerdictSummary string `json:"verdict_summary,omitempty"`
	Report         string `json:"report,omitempty"`

	Updated time.Time `json:"updated"`
}

// ProgressLine is one line of progress.jsonl: a sample and when in the
// load it was taken.
type ProgressLine struct {
	ElapsedS float64        `json:"elapsed_s"`
	Sample   *driver.Sample `json:"sample"`
}

// statusFile keeps status.json up to date. A nil one does nothing.
type statusFile struct {
	mu  sync.Mutex
	dir *rundir.Dir
	s   Status
}

func newStatusFile(dir *rundir.Dir, s Status) *statusFile {
	f := &statusFile{dir: dir, s: s}
	f.update(func(*Status) {})
	return f
}

// update changes the status and rewrites the file. Write errors are
// ignored: the status is a convenience, the run goes on without it.
func (f *statusFile) update(change func(*Status)) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	change(&f.s)
	f.s.Updated = time.Now().UTC()
	if b, err := json.MarshalIndent(f.s, "", "  "); err == nil {
		_ = f.dir.WriteAtomic(rundir.StatusFile, append(b, '\n'))
	}
}

func (f *statusFile) phase(p string) { f.update(func(s *Status) { s.Phase = p }) }

// LogEnv names the environment variable that tells a detached run where
// its output goes, so its status can say.
const LogEnv = "LOADL_DETACHED_LOG"

func detachedLog() string { return os.Getenv(LogEnv) }
