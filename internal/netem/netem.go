// Package netem shapes packets inside a container's network namespace with
// the kernel's netem qdisc, run from a helper container that shares the
// namespace and has NET_ADMIN. It does three things:
//
//   - The network profile's delay (Base): half a round trip added to every
//     packet the container sends, so two containers talking add a whole one.
//   - The loss fault (Apply). Toxiproxy acts on whole connections and can't
//     lose a packet. Only packets the target sends from one port are
//     dropped: the replies on connections into it. Its own outbound calls,
//     which leave from other ports, are untouched, and so are its SYN-ACKs
//     (see Apply). TCP retransmits what is lost, so the loss shows as slower
//     calls, from about 200 ms per lost segment, not as errors.
//   - The partition fault (Cut): every packet to the other side dropped.
//
// Each call replaces what the namespace had, keeping the profile's delay
// under a fault; Base again ends the fault.
package netem

// Band is the prio band a fault's netem hangs from: everything else goes to
// band 1, so nothing reaches 4 but what a filter sends there.
const Band = 4
