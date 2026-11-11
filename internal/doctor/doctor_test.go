package doctor

import (
	"errors"
	"strings"
	"testing"

	"github.com/akinfemi/load-lore/internal/bench"
	"github.com/akinfemi/load-lore/internal/design"
	"github.com/akinfemi/load-lore/internal/driver"
	"github.com/akinfemi/load-lore/internal/power"
	"github.com/akinfemi/load-lore/internal/scenario"
)

func scenarios() []*scenario.Scenario {
	sc := &scenario.Scenario{Name: "pool-trap"}
	sc.Meta.Host = scenario.Host{CPUs: 8, Memory: design.Size(8 << 30), Speed: 1000}
	return []*scenario.Scenario{sc}
}

func healthy() Facts {
	return Facts{
		GOOS: "darwin",
		Inspection: &driver.Inspection{
			Host: driver.Host{CPUs: 8, MemoryBytes: 10 << 30, EngineVersion: "29.5.2", CgroupVersion: "2", ComposeVersion: "2.40.0",
				OS: "Ubuntu 24.04 LTS", HostCPU: driver.HostCPUInVM, Speed: &bench.Speed{Score: bench.Score{Index: 990}}},
			DockerHost: "unix:///Users/someone/.colima/loadlore/docker.sock",
			Image:      "loadlore/synthetic:dev",
		},
		Power:     power.State{Known: true},
		Scenarios: scenarios(),
		SlowShare: 0.75,
	}
}

func byName(cs []Check) map[string]Check {
	m := map[string]Check{}
	for _, c := range cs {
		m[c.Name] = c
	}
	return m
}

func TestHealthyMac(t *testing.T) {
	cs := Checks(healthy())
	for _, c := range cs {
		if c.Status != OK {
			t.Errorf("%s: %+v", c.Name, c)
		}
	}
	if got := byName(cs)["Docker"].Detail; !strings.HasPrefix(got, "Colima, engine 29.5.2") {
		t.Errorf("Docker detail %q", got)
	}
	if Failed(cs) {
		t.Error("failed")
	}
}

func TestDockerUnreachable(t *testing.T) {
	f := healthy()
	f.Inspection, f.DockerErr = nil, errors.New("docker isn't reachable")
	f.Power = power.State{Known: true, OnBattery: true}
	cs := byName(Checks(f))
	if cs["Docker"].Status != Fail || !strings.Contains(cs["Docker"].Fix, "colima start") {
		t.Errorf("Docker %+v", cs["Docker"])
	}
	if cs["Image"].Status != Skip || cs["Power"].Status != Warn {
		t.Errorf("image %+v, power %+v", cs["Image"], cs["Power"])
	}
}

func TestProblems(t *testing.T) {
	f := healthy()
	f.Inspection.Host.CPUs, f.Inspection.Host.MemoryBytes = 4, 6<<30
	f.Inspection.ImageErr = errors.New("image loadlore/synthetic:dev isn't available to Docker")
	f.Power = power.State{Known: true, OnBattery: true, LowPower: true}
	cs := byName(Checks(f))
	if c := cs["Capacity"]; c.Status != Warn || !strings.Contains(c.Detail, "pool-trap needs 8 CPUs and 8 GiB (try --scale 0.5)") || !strings.Contains(c.Fix, "colima start --cpu 8 --memory 10") {
		t.Errorf("capacity %+v", c)
	}
	if cs["Image"].Status != Fail || !strings.Contains(cs["Image"].Fix, "make image") || cs["Speed"].Status != Skip {
		t.Errorf("image %+v, speed %+v", cs["Image"], cs["Speed"])
	}
	if c := cs["Power"]; c.Status != Warn || !strings.Contains(c.Fix, "Plug in and turn off Low Power Mode") {
		t.Errorf("power %+v", c)
	}
}

func TestSlowHostAndUnverifiedDocker(t *testing.T) {
	f := healthy()
	f.Inspection.DockerHost = "unix:///var/run/docker.sock"
	f.Inspection.Host.OS = "Docker Desktop"
	f.Inspection.Host.Speed = &bench.Speed{Score: bench.Score{Index: 600}}
	cs := byName(Checks(f))
	if c := cs["Docker"]; c.Status != Warn || !strings.Contains(c.Fix, "not yet on Docker Desktop") {
		t.Errorf("docker %+v", c)
	}
	if c := cs["Speed"]; c.Status != Warn || !strings.Contains(c.Detail, "speed index 600, against 1000") {
		t.Errorf("speed %+v", c)
	}
}

func TestPowerOffMac(t *testing.T) {
	f := healthy()
	f.GOOS = "linux"
	if c := byName(Checks(f))["Power"]; c.Status != Skip {
		t.Errorf("power %+v", c)
	}
}
