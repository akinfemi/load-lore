// Package power reads whether this machine runs on battery and in a
// low-power mode, which slow it down and make runs drift. Only macOS is
// read, with pmset; elsewhere the state is unknown.
package power

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
)

// State is the machine's power state. Known is false when it couldn't be
// read, and then the other fields mean nothing.
type State struct {
	Known     bool `json:"known"`
	OnBattery bool `json:"on_battery"`
	LowPower  bool `json:"low_power_mode"`
}

// Read reads the power state.
func Read(ctx context.Context) State {
	if runtime.GOOS != "darwin" {
		return State{}
	}
	batt, err1 := exec.CommandContext(ctx, "pmset", "-g", "batt").Output()
	settings, err2 := exec.CommandContext(ctx, "pmset", "-g").Output()
	if err1 != nil || err2 != nil {
		return State{}
	}
	return Parse(string(batt), string(settings))
}

// Parse reads `pmset -g batt` and `pmset -g` output.
func Parse(batt, settings string) State {
	var s State
	switch {
	case strings.Contains(batt, "'Battery Power'"):
		s.Known, s.OnBattery = true, true
	case strings.Contains(batt, "'AC Power'"):
		s.Known = true
	default:
		return State{}
	}
	for _, line := range strings.Split(settings, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "lowpowermode" {
			s.LowPower = f[1] != "0"
		}
	}
	return s
}
