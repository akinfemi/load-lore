// Package doctor checks whether this machine can run Loadlore's scenarios
// as their READMEs describe, and says how to fix what it can't.
package doctor

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/power"
	"github.com/akinfemi/load-lore/internal/scenario"
	"github.com/akinfemi/load-lore/internal/version"
)

// Status is a check's outcome.
type Status string

const (
	OK   Status = "ok"
	Warn Status = "warn" // runs work, but results may drift or differ from the READMEs
	Fail Status = "fail" // runs won't start
	Skip Status = "skip" // not checked here
)

// Check is one finding, with the fix when there's a problem.
type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// Facts are what doctor found; Checks judges them.
type Facts struct {
	GOOS       string
	DockerErr  error              // Docker unreachable; the rest of Docker's facts are then missing
	Inspection *driver.Inspection // nil when Docker is unreachable
	Power      power.State
	Scenarios  []*scenario.Scenario
	// SlowShare is how slow against a scenario's reference machine a host
	// may be before runs break noticeably earlier than its README says.
	SlowShare float64
}

// Checks judges the facts, in the order a person would fix them.
func Checks(f Facts) []Check {
	var out []Check
	add := func(name string, st Status, detail, fix string) {
		out = append(out, Check{Name: name, Status: st, Detail: detail, Fix: fix})
	}
	mac := f.GOOS == "darwin"

	// Docker.
	if f.DockerErr != nil || f.Inspection == nil {
		fix := "Start the Docker daemon (sudo systemctl start docker) and make sure your user can reach it (the docker group)."
		if mac {
			fix = "Start Colima (colima start --cpu 8 --memory 10) or Docker Desktop. With Colima, point Docker at it: docker context use colima, or set DOCKER_HOST to its socket."
		}
		detail := "Docker isn't reachable"
		if f.DockerErr != nil {
			detail = f.DockerErr.Error()
		}
		add("Docker", Fail, detail, fix)
		add("Compose", Skip, "needs Docker", "")
		add("Capacity", Skip, "needs Docker", "")
		add("Image", Skip, "needs Docker", "")
		add("Speed", Skip, "needs Docker", "")
		out = append(out, powerCheck(f.Power, mac))
		return out
	}
	in := f.Inspection
	h := in.Host
	kind := dockerKind(h, in.DockerHost)
	detail := fmt.Sprintf("%s, engine %s, cgroup v%s", kind, h.EngineVersion, h.CgroupVersion)
	switch {
	case kind == "Docker Desktop" || kind == "WSL2":
		add("Docker", Warn, detail, "Loadlore is verified on Colima and Linux, not yet on "+kind+": runs should work, but CPU limits and network faults may behave differently. Please report what you see.")
	case h.CgroupVersion != "2":
		add("Docker", Warn, detail, "Loadlore is tested on cgroup v2; on v1, CPU and memory limits may behave differently. Most current Linux distributions use v2 by default.")
	default:
		add("Docker", OK, detail, "")
	}

	// Compose.
	if in.ComposeErr != nil {
		add("Compose", Fail, in.ComposeErr.Error(), "Install Docker Compose 2.23 or later as a Docker CLI plugin (with Homebrew: brew install docker-compose, then add its plugin folder to cliPluginsExtraDirs in ~/.docker/config.json).")
	} else {
		add("Compose", OK, "version "+h.ComposeVersion, "")
	}

	out = append(out, capacityCheck(h, f.Scenarios, kind))

	// Image.
	if in.ImageErr != nil {
		fix := "Build it from the repository with make image."
		if version.Pullable(in.Image) {
			fix = "Check the network and that Docker can reach the registry, then run loadl doctor again; docker pull " + in.Image + " shows the error."
		}
		add("Image", Fail, in.ImageErr.Error(), fix)
		add("Speed", Skip, "needs the image", "")
	} else {
		add("Image", OK, in.Image, "")
		out = append(out, speedCheck(h.Speed, in.SpeedErr, f.Scenarios, f.SlowShare))
	}

	out = append(out, powerCheck(f.Power, mac))
	return out
}

// dockerKind names where Docker runs, from what it reports.
func dockerKind(h driver.Host, addr string) string {
	switch {
	case strings.Contains(addr, ".colima"):
		return "Colima"
	case strings.Contains(h.OS, "Docker Desktop"):
		return "Docker Desktop"
	case strings.Contains(strings.ToLower(h.Kernel), "microsoft") || strings.Contains(strings.ToLower(h.Kernel), "wsl"):
		return "WSL2"
	case h.HostCPU == driver.HostCPUSampled:
		return "Docker on this Linux machine"
	}
	return "Docker (" + h.OS + ")"
}

// capacityCheck compares what Docker has with what each scenario needs at
// --scale 1, and suggests a scale for the ones that don't fit.
func capacityCheck(h driver.Host, scs []*scenario.Scenario, kind string) Check {
	have := fmt.Sprintf("%d CPUs and %s available to Docker", h.CPUs, gib(h.MemoryBytes))
	var short []string
	maxCPU, maxMem := 0.0, int64(0)
	for _, sc := range scs {
		need := sc.Meta.Host
		maxCPU, maxMem = math.Max(maxCPU, need.CPUs), max(maxMem, int64(need.Memory))
		if need.CPUs <= float64(h.CPUs) && int64(need.Memory) <= h.MemoryBytes {
			continue
		}
		scale := 1.0
		if need.CPUs > 0 {
			scale = math.Min(scale, float64(h.CPUs)/need.CPUs)
		}
		if need.Memory > 0 {
			scale = math.Min(scale, float64(h.MemoryBytes)/float64(need.Memory))
		}
		scale = math.Floor(scale*20) / 20
		short = append(short, fmt.Sprintf("%s needs %s CPUs and %s (try --scale %s)", sc.Name, fmtF(need.CPUs), gib(int64(need.Memory)), fmtF(scale)))
	}
	if len(short) == 0 {
		return Check{Name: "Capacity", Status: OK, Detail: have + ": enough for every scenario at --scale 1"}
	}
	fix := "Give Docker more CPUs and memory, or run with the --scale shown; a scaled-down run breaks at a lower load."
	if kind == "Colima" {
		fix = fmt.Sprintf("Give Colima more: colima stop, then colima start --cpu %s --memory %s (or run with the --scale shown).", fmtF(math.Ceil(maxCPU)), fmtF(math.Ceil(float64(maxMem)/(1<<30))+2))
	}
	return Check{Name: "Capacity", Status: Warn, Detail: have + "; " + strings.Join(short, "; "), Fix: fix}
}

// speedCheck compares the host speed index with the slowest-checked
// scenario's reference machine.
func speedCheck(sp *bench.Speed, err error, scs []*scenario.Scenario, slowShare float64) Check {
	if err != nil || sp == nil {
		detail := "the benchmark didn't run"
		if err != nil {
			detail = "the benchmark failed: " + err.Error()
		}
		return Check{Name: "Speed", Status: Warn, Detail: detail, Fix: "Runs still work, but their reports won't have a speed index to compare hosts with."}
	}
	ref := 0
	for _, sc := range scs {
		ref = max(ref, sc.Meta.Host.Speed)
	}
	if ref == 0 {
		ref = bench.ReferenceIndex
	}
	detail := fmt.Sprintf("speed index %d, against %d on the machine the scenarios were checked on", sp.Index, ref)
	if float64(sp.Index) < slowShare*float64(ref) {
		return Check{Name: "Speed", Status: Warn, Detail: detail, Fix: "Close other busy programs and run again. If it stays low, this machine is slower: designs break at lower loads than the READMEs say, and the lessons are in the relative results."}
	}
	return Check{Name: "Speed", Status: OK, Detail: detail}
}

func powerCheck(p power.State, mac bool) Check {
	switch {
	case !mac:
		return Check{Name: "Power", Status: Skip, Detail: "only checked on macOS"}
	case !p.Known:
		return Check{Name: "Power", Status: Skip, Detail: "pmset didn't say"}
	case p.OnBattery && p.LowPower:
		return Check{Name: "Power", Status: Warn, Detail: "on battery, Low Power Mode on", Fix: "Plug in and turn off Low Power Mode (System Settings, Battery). Together they more than doubled the CPU time per request in our runs, so designs break much earlier."}
	case p.LowPower:
		return Check{Name: "Power", Status: Warn, Detail: "Low Power Mode on", Fix: "Turn off Low Power Mode (System Settings, Battery): it slows the CPU, so designs break earlier and runs drift."}
	case p.OnBattery:
		return Check{Name: "Power", Status: Warn, Detail: "on battery", Fix: "Plug in: on battery a Mac can slow down mid-run, which makes runs drift."}
	}
	return Check{Name: "Power", Status: OK, Detail: "on mains power, Low Power Mode off"}
}

// Failed reports whether any check failed.
func Failed(cs []Check) bool {
	for _, c := range cs {
		if c.Status == Fail {
			return true
		}
	}
	return false
}

func gib(b int64) string {
	return strconv.FormatFloat(math.Round(float64(b)/(1<<30)*10)/10, 'f', -1, 64) + " GiB"
}

func fmtF(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
