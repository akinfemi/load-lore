package dashboard

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/plan"
	"github.com/akinfemi/load-lore/internal/runner"
)

func trapPlan(t *testing.T) *plan.Plan {
	t.Helper()
	src, err := os.ReadFile("../../scenarios/connection-pool-trap/design.yaml")
	if err != nil {
		t.Fatal(err)
	}
	d, diags := design.Load(src)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	p, err := plan.Build(d, plan.Options{RunID: "trap", Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func f(v float64) *float64 { return &v }

// A pool trap run 2:10 in, its break confirmed.
func trapSnapshot(t *testing.T) *Snapshot {
	p := trapPlan(t)
	started := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	s := &Snapshot{Plan: p, Status: runner.Status{RunID: "connection-pool-trap-20261111-120000", Phase: runner.PhaseLoad, ProfileS: 420, ElapsedS: 130,
		LoadStarted: &started, BreakAtS: f(100), BreakConfirmedS: f(120), StopsAtS: f(180)}}
	for el := 10.0; el <= 130; el += 10 {
		offered := 100 + el*1300/300
		served, p99, inUse, pending := offered, 88.0, offered*0.081, 0.0
		if el > 100 {
			served, p99, inUse, pending = 494, 1400, 40, 700
		}
		s.Samples = append(s.Samples, runner.ProgressLine{ElapsedS: el, Sample: &driver.Sample{
			Offered: offered, Served: f(served), P50Ms: f(84), P95Ms: f(87), P99Ms: f(p99), Errors: f(0),
			CPU:            map[string]float64{"api": 0.12, "db": 0.06, "payments": 0.11},
			Pools:          map[string]driver.PoolSample{"api>db": {InUse: inUse, Max: 40, Pending: pending}, "api>payments": {InUse: inUse, Max: 200}},
			Connections:    map[string]float64{"db": 41},
			MaxConnections: map[string]float64{"db": 100},
		}})
	}
	s.Events = []Event{{AtS: 0, Kind: "fault_start", Detail: "latency +80 ms on connections into payments"}}
	return s
}

func TestRender(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii) // plain text, whatever the test's terminal
	out := Render(trapSnapshot(t), 100, "q: stop watching")
	for _, want := range []string{
		"loadl connection-pool-trap-20261111-120000  load 2:10 of 7:00", "q: stop watching",
		"break confirmed at 2:00 (from 1:40); the load stops at 3:00",
		"  load\n  └▶ lb", "└▶ api ×4", "db  cpu 6.0% · conns 41/100 · api's pool 40/40, 700 waiting", "payments  cpu 11%",
		"  offered  ", "  served   ", "  p99      ", "(log scale)", "▲", "^", "▲ fault  ^ break",
		"api         cpu 12% · pool to db 40/40 +700 waiting · pool to payments 40/200",
		"0:00  fault: latency +80 ms on connections into payments",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard lacks %q", want)
		}
	}
	for i, l := range strings.Split(out, "\n") {
		if w := lipgloss.Width(l); w > 100 {
			t.Errorf("line %d is %d columns: %q", i, w, l)
		}
	}
}

func TestRenderFinished(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	s := trapSnapshot(t)
	s.Status.Phase, s.Status.Status, s.Status.Verdict = runner.PhaseFinished, "completed", "broke"
	s.Status.VerdictSummary, s.Status.Report = "Broke 1:40 into the load, at 555 rps offered.", "runs/x/report.md"
	out := Render(s, 80, "")
	if !strings.Contains(out, "BROKE. Broke 1:40 into the load, at 555 rps offered.\nreport: runs/x/report.md") || strings.Contains(out, "break confirmed") {
		t.Errorf("finished:\n%s", out)
	}
}

func TestModelKeys(t *testing.T) {
	m := New(t.TempDir())
	m.snap = trapSnapshot(t)
	stopped := 0
	m.Interrupt = func() { stopped++ }
	_, cmd := m.Update(teaKey("ctrl+c"))
	_, _ = m.Update(teaKey("ctrl+c"))
	if stopped != 1 || cmd != nil || !strings.Contains(m.View(), "stopping: tearing down") {
		t.Errorf("in-process Ctrl-C: stopped %d times, view %q", stopped, m.View()[:80])
	}
	w := New(t.TempDir())
	w.snap = trapSnapshot(t)
	if _, cmd := w.Update(teaKey("q")); cmd == nil || !w.Quit() {
		t.Error("q didn't stop watching")
	}
}

func teaKey(s string) tea.KeyMsg {
	if s == "ctrl+c" {
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
