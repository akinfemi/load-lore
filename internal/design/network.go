package design

import (
	"maps"
	"slices"
)

// NetworkProfile is a named network between the stack's containers, with
// a round trip from its maker's published figures, so every design stands
// on the same network rather than on the Docker bridge's few microseconds.
type NetworkProfile struct {
	Name string
	// RTTMs is the round trip added between any two of the stack's
	// containers: half of it on each side's outgoing packets.
	RTTMs float64
	// Source is where the figure comes from, and Basis how it was read
	// from it, both printed in every report's assumptions.
	Source, Basis string
}

// Network profiles.
const (
	NetworkLocal     = "local"
	NetworkSameZone  = "same-zone"
	NetworkCrossZone = "cross-zone"
	// DefaultNetwork is the profile a design gets when it names none.
	DefaultNetwork = NetworkSameZone
)

// NetworkProfiles are the named networks a design's network block can pick.
var NetworkProfiles = map[string]NetworkProfile{
	NetworkLocal: {
		Name:   NetworkLocal,
		Source: "none: the Docker bridge on one machine",
		Basis:  "nothing is added: containers on one Docker bridge reach each other in tens of microseconds, faster than any two machines in a data center",
	},
	NetworkSameZone: {
		Name: NetworkSameZone, RTTMs: 0.3,
		Source: "AWS Architecture Blog, Improving performance and reducing cost using Availability Zone Affinity: https://aws.amazon.com/blogs/architecture/improving-performance-and-reducing-cost-using-availability-zone-affinity/",
		Basis: "every service in one availability zone. AWS says a round trip between two instances in the same zone is generally sub-millisecond with enhanced networking, and publishes no exact figure, " +
			"so the 0.3 ms round trip Loadlore adds between any two containers is its own estimate, inside that bound",
	},
	NetworkCrossZone: {
		Name: NetworkCrossZone, RTTMs: 1.5,
		Source: "AWS Fault Isolation Boundaries whitepaper, Availability Zones: https://docs.aws.amazon.com/whitepapers/latest/aws-fault-isolation-boundaries/availability-zones.html",
		Basis: "every call crossing between availability zones, as when a service's replicas are spread over zones and callers don't keep to their own. AWS says zones in a region are close enough for synchronous replication with single-digit millisecond latency, and publishes no exact figure, " +
			"so the 1.5 ms round trip Loadlore adds between any two containers is its own estimate, at the low end of that range",
	},
}

// NetworkProfileNames lists the profiles, sorted.
func NetworkProfileNames() []string { return slices.Sorted(maps.Keys(NetworkProfiles)) }
