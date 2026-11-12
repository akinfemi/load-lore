package local

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/akinfemi/load-lore/internal/plan"
)

// The disk_slow fault adds real latency to a postgres's disk writes with
// the kernel's dm-delay target. The data volume is an ext4 filesystem on a
// dm-delay device, over a loop device on a sparse file, made before the
// stack starts with no delay; the fault reloads the device's table with a
// write and flush delay, so every write and fsync waits that long, and
// clearing it reloads it with none. A privileged helper container does the
// device-mapper work in the Docker host's kernel (on Colima, its VM).

// slowDiskTools is installed in the helper at run time.
const slowDiskTools = "apk add -q --no-progress device-mapper e2fsprogs util-linux >/dev/null"

// slowDiskName is the dm device for one run's service.
func (d *Driver) slowDiskName(svc string) string { return "lore-" + d.runID + "-" + svc }

// slowDiskVolume makes vol an ext4 volume on a new dm-delay device for s,
// sized well above its dataset, with no delay yet.
func (d *Driver) slowDiskVolume(ctx context.Context, s *plan.Service, vol string) error {
	name := d.slowDiskName(s.Name)
	img := vol + "-img"
	size := max(int64(2)<<30, 6*s.Dataset.Rows*int64(s.Dataset.RowBytes))
	if _, err := d.docker(ctx, "volume", "create", "--label", LabelOwner+"=1", "--label", LabelRun+"="+d.runID, img); err != nil {
		return err
	}
	script := fmt.Sprintf(`set -e
%s
truncate -s %d /img/disk.img
L=$(losetup -f --show /img/disk.img)
S=$(blockdev --getsz $L)
dmsetup create %s --table "0 $S delay $L 0 0 $L 0 0 $L 0 0"
mkfs.ext4 -q /dev/mapper/%s
echo "minor=$(dmsetup info -c --noheadings -o minor %s)"`, slowDiskTools, size, name, name, name)
	out, err := d.docker(ctx, "run", "--rm", "--privileged", "--label", LabelOwner+"=1", "--label", LabelRun+"="+d.runID,
		"-v", img+":/img", ImageAlpine, "sh", "-c", script)
	if err != nil {
		return err
	}
	// dmsetup made /dev/mapper/<name> inside the helper only; the kernel's
	// own node, /dev/dm-<minor>, is what the Docker host can mount.
	_, minor, ok := strings.Cut(strings.TrimSpace(string(out[bytes.LastIndex(out, []byte("minor=")):])), "minor=")
	if !ok || strings.TrimSpace(minor) == "" {
		return fmt.Errorf("no device minor from the helper:\n%s", out)
	}
	_, err = d.docker(ctx, "volume", "create", "--label", LabelOwner+"=1", "--label", LabelRun+"="+d.runID,
		"--driver", "local", "--opt", "type=ext4", "--opt", "device=/dev/dm-"+strings.TrimSpace(minor), vol)
	return err
}

// slowDisk sets the write and flush delay of a service's dm-delay device.
func (d *Driver) slowDisk(ctx context.Context, svc string, ms int) error {
	name := d.slowDiskName(svc)
	// The table reads "0 <sectors> delay <dev> 0 <read ms> <dev> 0 <write
	// ms> <dev> 0 <flush ms>"; keep the device and size, reads undelayed.
	script := fmt.Sprintf(`set -e
%s
set -- $(dmsetup table %s)
dmsetup suspend %s
dmsetup reload %s --table "0 $2 delay $4 0 0 $4 0 %d $4 0 %d"
dmsetup resume %s`, slowDiskTools, name, name, name, ms, ms, name)
	_, err := d.docker(ctx, "run", "--rm", "--privileged", "--label", LabelOwner+"=1", "--label", LabelRun+"="+d.runID, ImageAlpine, "sh", "-c", script)
	return err
}

// removeSlowDisks removes dm-delay devices and their loop devices: this
// run's, or with all, every one Loadlore made. Their volumes go with the
// rest of the run's.
func (d *Driver) removeSlowDisks(ctx context.Context, all bool) error {
	prefix := "lore-"
	if !all {
		prefix = d.slowDiskName("")
	}
	// sysfs lists device-mapper names without dmsetup: skip installing it
	// when there is nothing to remove.
	out, err := d.docker(ctx, "run", "--rm", "--privileged", "--label", LabelOwner+"=1", ImageAlpine, "sh", "-c", "cat /sys/block/dm-*/dm/name 2>/dev/null || true")
	if err != nil {
		return err
	}
	if !strings.Contains(string(out), prefix) {
		return nil
	}
	script := fmt.Sprintf(`%s
for n in $(dmsetup ls 2>/dev/null | awk '{print $1}' | grep '^%s' || true); do
  L=$(dmsetup table $n | awk '{print $4}')
  dmsetup remove $n
  case $L in /dev/*) losetup -d $L ;; *) losetup -d /dev/loop${L#*:} 2>/dev/null || true ;; esac
done`, slowDiskTools, prefix)
	_, err = d.docker(ctx, "run", "--rm", "--privileged", "--label", LabelOwner+"=1", ImageAlpine, "sh", "-c", script)
	return err
}

// hasSlowDisk is whether the plan has a dm-delay device to clean up.
func hasSlowDisk(p *plan.Plan) bool {
	for _, s := range p.Services {
		if s.SlowDisk {
			return true
		}
	}
	return false
}
