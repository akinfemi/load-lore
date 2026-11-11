// Package dashboard is the live terminal view of a run. It reads the run
// folder only (plan.json, status.json, progress.jsonl, events.jsonl), so
// the run in this process and loadl watch on a detached run look the same.
package dashboard

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/rundir"
	"github.com/akinfemi/load-lore/internal/runner"
)

// Snapshot is everything the dashboard shows, read from a run folder.
type Snapshot struct {
	Plan    *plan.Plan
	Status  runner.Status
	Samples []runner.ProgressLine
	Events  []Event
}

// Event is a moment worth marking on the charts: a fault, the stop.
type Event struct {
	AtS    float64
	Kind   string // fault_start, fault_end, load_stop
	Detail string
}

// Load reads a run folder. A run that hasn't written its status yet, or
// samples yet, comes back with what there is.
func Load(dir string) (*Snapshot, error) {
	s := &Snapshot{}
	if err := readJSON(filepath.Join(dir, "plan.json"), &s.Plan); err != nil {
		return nil, err
	}
	if err := readJSON(filepath.Join(dir, rundir.StatusFile), &s.Status); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	_ = eachLine(filepath.Join(dir, rundir.ProgressFile), func(b []byte) {
		var l runner.ProgressLine
		if json.Unmarshal(b, &l) == nil && l.Sample != nil {
			s.Samples = append(s.Samples, l)
		}
	})
	_ = eachLine(filepath.Join(dir, "events.jsonl"), func(b []byte) {
		var e rundir.Event
		if json.Unmarshal(b, &e) != nil || s.Status.LoadStarted == nil {
			return
		}
		at := e.Time.Sub(*s.Status.LoadStarted).Seconds()
		detail, _ := e.Fields["detail"].(string)
		switch e.Kind {
		case "fault_start", "fault_end":
			s.Events = append(s.Events, Event{AtS: at, Kind: e.Kind, Detail: detail})
		case "load_stop":
			s.Events = append(s.Events, Event{AtS: at, Kind: e.Kind, Detail: "the load stopped after the break"})
		}
	})
	return s, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func eachLine(path string, f func([]byte)) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		f(sc.Bytes())
	}
	return sc.Err()
}
