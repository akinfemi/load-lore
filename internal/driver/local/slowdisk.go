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
// stack starts with no write delay; the fault reloads the device's table
// with a write and flush delay, so every write and fsync waits that long,
// and clearing it reloads it with none. A privileged helper container does
// the device-mapper work in the Docker host's kernel (on Colima, its VM).
//
// A design's disk block uses the same device: its read delay is in the
// table from the start (and kept when a fault reloads it), and the compose
// file caps the container's reads and writes on the device at its IOPS.

// slowDiskTools is installed in the helper at run time.
const slowDiskTools = "apk add -q --no-progress device-mapper e2fsprogs util-linux >/dev/null"

// slowDiskName is the dm device for one run's service.
func (d *Driver) slowDiskName(svc string) string { return "lore-" + d.runID + "-" + svc }

// slowDiskVolume makes vol an ext4 volume on a new dm-delay device for one
// container of s (each of several read replicas has its own), sized well
// above its dataset, with no write delay yet.
func (d *Driver) slowDiskVolume(ctx context.Context, s *plan.Service, container, vol string) error {
	name := d.slowDiskName(container)
	img := vol + "-img"
	read, loop := 0, ""
	if s.Disk != nil {
		// Direct I/O: without it the loop device reads its image file one
		// request at a time per cgroup, through the Docker host's page cache,
		// and on Colima that capped Postgres at about 4,000 reads a second
		// whatever the disk block allowed. It also keeps the image out of the
		// host's page cache, where it would be memory outside the database's
		// limit. Fault-only slow disks keep the buffered loop they were
		// measured with.
		read, loop = s.Disk.ReadMs, "--direct-io=on --sector-size 4096 "
	}
	size := max(int64(2)<<30, 6*s.Dataset.Rows*int64(s.Dataset.RowBytes))
	if _, err := d.docker(ctx, "volume", "create", "--label", LabelOwner+"=1", "--label", LabelRun+"="+d.runID, img); err != nil {
		return err
	}
	script := fmt.Sprintf(`set -e
%s
truncate -s %d /img/disk.img
L=$(losetup -f --show %s/img/disk.img)
S=$(blockdev --getsz $L)
dmsetup create %s --table "0 $S delay $L 0 %d $L 0 0 $L 0 0"
mkfs.ext4 -q /dev/mapper/%s
echo "minor=$(dmsetup info -c --noheadings -o minor %s)"`, slowDiskTools, size, loop, name, read, name, name)
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
	dev := "/dev/dm-" + strings.TrimSpace(minor)
	if d.diskDevs == nil {
		d.diskDevs = map[string]string{}
	}
	d.diskDevs[container] = dev
	_, err = d.docker(ctx, "volume", "create", "--label", LabelOwner+"=1", "--label", LabelRun+"="+d.runID,
		"--driver", "local", "--opt", "type=ext4", "--opt", "device="+dev, vol)
	return err
}

// dropPageCache empties the Docker host's page cache. Data copied into a
// volume stays cached, charged to the helper that copied it, so the
// database would read it from memory whatever its own memory limit; with a
// disk block, the limit must hold for reads to reach the disk.
func (d *Driver) dropPageCache(ctx context.Context) error {
	_, err := d.docker(ctx, "run", "--rm", "--privileged", "--label", LabelOwner+"=1", "--label", LabelRun+"="+d.runID,
		ImageAlpine, "sh", "-c", "sync && echo 1 > /proc/sys/vm/drop_caches")
	return err
}

// slowDisk sets the write and flush delay of a service's dm-delay device
// to its disk's own write wait plus ms, keeping its read wait: a disk_slow
// fault adds ms, and clearing it (ms 0) leaves the disk's own.
func (d *Driver) slowDisk(ctx context.Context, svc string, ms int) error {
	read, write := 0, 0
	if s := d.plan.Service(svc); s != nil && s.Disk != nil {
		read, write = s.Disk.ReadMs, s.Disk.WriteMs
	}
	return d.diskDelay(ctx, d.slowDiskName(svc), read, write+ms)
}

// diskDelay reloads dm-delay device name's table with these read and
// write (and flush) waits, keeping the device and size.
func (d *Driver) diskDelay(ctx context.Context, name string, read, write int) error {
	// The table reads "0 <sectors> delay <dev> 0 <read ms> <dev> 0 <write
	// ms> <dev> 0 <flush ms>".
	script := fmt.Sprintf(`set -e
%s
set -- $(dmsetup table %s)
dmsetup suspend %s
dmsetup reload %s --table "0 $2 delay $4 0 %d $4 0 %d $4 0 %d"
dmsetup resume %s`, slowDiskTools, name, name, name, read, write, write, name)
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

// trimDockerDisk hands blocks freed inside the Docker host back to the
// machine under it, best effort. Each run copies its datasets into fresh
// volumes (a slow-disk run writes them twice: the volume and the device's
// image file), and on a VM such as Colima's the disk image on the Mac
// keeps that space after the volumes are removed until something trims
// it; a scenario check's few runs can fill a laptop's disk.
func (d *Driver) trimDockerDisk(ctx context.Context) {
	out, err := d.docker(ctx, "info", "-f", "{{.DockerRootDir}}")
	root := strings.TrimSpace(string(out))
	if err != nil || root == "" {
		return
	}
	_, _ = d.docker(ctx, "run", "--rm", "--privileged", "--label", LabelOwner+"=1",
		"-v", root+":/docker-root", ImageAlpine, "fstrim", "/docker-root")
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
