package dashboard

import (
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/akinfemi/load-lore/internal/runner"
)

// pollEvery is how often the dashboard rereads the run folder. Samples
// come every 10 s; phases change at any time.
var pollEvery = time.Second

// Model is the dashboard as a Bubble Tea program.
type Model struct {
	dir   string
	snap  *Snapshot
	err   error
	width int
	// Interrupt, when set, is called on Ctrl-C: the run is in this
	// process, so Ctrl-C stops it (with teardown) and the dashboard stays
	// until it has finished. Without it, q and Ctrl-C only stop watching.
	Interrupt   func()
	interrupted bool
	quit        bool
}

// New watches the run in dir. It starts at COLUMNS wide (else 100) until
// the terminal says its size.
func New(dir string) *Model {
	width := 100
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 0 {
		width = n
	}
	return &Model{dir: dir, width: width}
}

type tickMsg struct{}

func tick() tea.Cmd { return tea.Tick(pollEvery, func(time.Time) tea.Msg { return tickMsg{} }) }

func (m *Model) load() {
	if s, err := Load(m.dir); err == nil {
		m.snap, m.err = s, nil
	} else if m.snap == nil {
		m.err = err
	}
}

// Init loads the run and starts polling.
func (m *Model) Init() tea.Cmd { m.load(); return tick() }

// Update handles keys, resizes and polls.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.Interrupt != nil {
				if !m.interrupted {
					m.interrupted = true
					m.Interrupt()
				}
				return m, nil
			}
			m.quit = true
			return m, tea.Quit
		case "q":
			if m.Interrupt == nil {
				m.quit = true
				return m, tea.Quit
			}
		}
	case tickMsg:
		m.load()
		if m.Finished() {
			return m, tea.Quit
		}
		return m, tick()
	}
	return m, nil
}

// Finished reports whether the run has finished.
func (m *Model) Finished() bool {
	return m.snap != nil && m.snap.Status.Phase == runner.PhaseFinished
}

// Quit reports whether the viewer stopped watching before the run ended.
func (m *Model) Quit() bool { return m.quit }

// View draws the dashboard.
func (m *Model) View() string {
	if m.snap == nil {
		if m.err != nil {
			return "waiting for the run: " + m.err.Error() + "\n"
		}
		return "waiting for the run\n"
	}
	hint := "q: stop watching (the run goes on)"
	switch {
	case m.Interrupt != nil && m.interrupted:
		hint = "stopping: tearing down"
	case m.Interrupt != nil:
		hint = "Ctrl-C: stop the run"
	}
	return Render(m.snap, m.width, hint)
}

// Frame is the dashboard as it stands, for printing once it has exited.
func (m *Model) Frame() string {
	if m.snap == nil {
		return ""
	}
	return Render(m.snap, m.width, "")
}
