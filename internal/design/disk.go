package design

import (
	"maps"
	"math"
	"slices"
)

// DiskProfile is a named data disk, with numbers from its maker's
// published specs, so every design and scenario stands on the same disks
// rather than on waits and caps picked until a result shows.
type DiskProfile struct {
	Name string
	// ReadMs and WriteMs are asked of the kernel's dm-delay for every
	// read, and every write and flush, in whole milliseconds, its step.
	// dm-delay counts in 1 ms ticks, so a request waits between one tick
	// less than asked and what was asked: about half a millisecond less on
	// average. Each run measures the wait it stood on.
	ReadMs, WriteMs int
	// IOPS is the default when the design sets none; 0 means it must be
	// set (the volume type is bought by its provisioned IOPS). MinIOPS and
	// MaxIOPS bound what a design may provision; both 0 for a profile
	// without a cap.
	IOPS, MinIOPS, MaxIOPS int
	// MiBps is the throughput cap at a number of IOPS, each of reads and
	// writes; 0 for none.
	MiBps func(iops int) float64
	// Source is where the numbers come from, and Basis how they were read
	// from it, both printed in every report's assumptions.
	Source, Basis string
}

// Modeled is whether the profile puts the data on a modelled disk, with
// waits or caps; laptop leaves it on the Docker host's own.
func (p DiskProfile) Modeled() bool { return p.MaxIOPS > 0 }

// Disk profiles.
const (
	DiskLaptop = "laptop"
	DiskGP3    = "gp3"
	DiskIO2    = "io2"
)

// DiskProfiles are the named disks a design's disk block can pick.
var DiskProfiles = map[string]DiskProfile{
	DiskLaptop: {
		Name:   DiskLaptop,
		Source: "none: the Docker host's own disk",
		Basis: "the data stays on the Docker host's disk, unchanged. On a laptop that is an SSD whose fsync returns in a fraction of a millisecond, " +
			"and reads that miss the database's memory mostly come back from the host's own page cache, with no cap",
	},
	DiskGP3: {
		Name: DiskGP3, ReadMs: 1, WriteMs: 1,
		IOPS: 3000, MinIOPS: 3000, MaxIOPS: 80000,
		MiBps:  func(int) float64 { return 125 },
		Source: "AWS, Amazon EBS General Purpose SSD volumes: https://docs.aws.amazon.com/ebs/latest/userguide/general-purpose.html",
		Basis: "a gp3 volume at its baseline: 3,000 IOPS and 125 MiB/s, included in its price, and up to 80,000 IOPS provisioned. " +
			"AWS publishes no exact latency, only single-digit milliseconds. Loadlore asks dm-delay for 1 ms on every read, write and flush, but dm-delay counts in the kernel's 1 ms ticks, so a request waits 0 to 1 ms, about 0.45 ms on average (measured 19 Nov): below AWS's range, so the disk is flattered rather than handicapped. " +
			"A WAL flush is a write and a flush, each waiting, so a commit waits about 1.2 ms longer (measured the same day). Each run measures the waits it stood on, given below. " +
			"Reads and writes are capped separately, so a mixed load can reach up to twice the IOPS. Provisioned throughput isn't modelled",
	},
	DiskIO2: {
		Name: DiskIO2, MinIOPS: 100, MaxIOPS: 256000,
		MiBps:  func(iops int) float64 { return math.Min(4000, 0.256*float64(iops)) },
		Source: "AWS, Amazon EBS Provisioned IOPS SSD volumes: https://docs.aws.amazon.com/ebs/latest/userguide/provisioned-iops.html",
		Basis: "an io2 Block Express volume at its provisioned IOPS (100 to 256,000), with 0.256 MiB/s of throughput per provisioned IOPS, up to 4,000. " +
			"AWS states an average latency under 500 microseconds, below dm-delay's 1 ms step, so nothing is added and the Docker host's own disk sets the latency; each run measures it, given below. " +
			"Reads and writes are capped separately, so a mixed load can reach up to twice the IOPS",
	},
}

// DiskProfileNames lists the profiles, sorted.
func DiskProfileNames() []string { return slices.Sorted(maps.Keys(DiskProfiles)) }
