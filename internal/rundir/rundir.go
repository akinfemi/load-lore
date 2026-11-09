// Package rundir manages a run folder: .loadlore/runs/<run-id>/ holding the
// exact design, the generated stack, raw metrics, logs, the event log and,
// later, the report. Everything a report says comes from these files.
package rundir

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// Root is where runs are stored, relative to the working directory.
const Root = ".loadlore/runs"

// Dir is one run's folder.
type Dir struct {
	ID   string
	Path string

	mu     sync.Mutex
	events *os.File
}

var unsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// NewID makes a run id from the design name and the start time.
func NewID(name string, t time.Time) string {
	return unsafe.ReplaceAllString(name, "-") + "-" + t.UTC().Format("20060102-150405")
}

// Create makes the run folder and its subfolders.
func Create(root, id string) (*Dir, error) {
	path := filepath.Join(root, id)
	for _, sub := range []string{"", "metrics", "logs"} {
		if err := os.MkdirAll(filepath.Join(path, sub), 0o755); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(filepath.Join(path, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Dir{ID: id, Path: path, events: f}, nil
}

// File returns the path of a file in the run folder.
func (d *Dir) File(parts ...string) string {
	return filepath.Join(append([]string{d.Path}, parts...)...)
}

// Write writes a file in the run folder.
func (d *Dir) Write(name string, data []byte) error {
	p := d.File(name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// WriteJSON writes v as indented JSON.
func (d *Dir) WriteJSON(name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return d.Write(name, append(b, '\n'))
}

// Event is one line of events.jsonl.
type Event struct {
	Time   time.Time      `json:"time"`
	Kind   string         `json:"kind"`
	Fields map[string]any `json:"fields,omitempty"`
}

// Event appends an event. It is safe for concurrent use; write errors are
// returned but callers usually can't do better than report them once.
func (d *Dir) Event(kind string, fields map[string]any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.events == nil {
		return fmt.Errorf("events log is closed")
	}
	b, err := json.Marshal(Event{Time: time.Now().UTC(), Kind: kind, Fields: fields})
	if err != nil {
		return err
	}
	_, err = d.events.Write(append(b, '\n'))
	return err
}

// Close closes the event log.
func (d *Dir) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.events == nil {
		return nil
	}
	err := d.events.Close()
	d.events = nil
	return err
}
