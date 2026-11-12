// Package netem drops packets inside a container's network namespace, for
// the loss fault. Toxiproxy acts on whole connections and can't lose a
// packet, so loss uses the kernel's netem qdisc: run from a helper
// container that shares the target's network namespace and has NET_ADMIN.
//
// Only packets the target sends from one port are dropped: the replies on
// connections into it. Its own outbound calls, which leave from other
// ports, are untouched, and so are its SYN-ACKs (see Apply). TCP retransmits what is lost, so the loss shows as
// slower calls, from about 200 ms per lost segment, not as errors.
package netem

// Band is the prio band netem hangs from: the default priority map uses
// bands 1 to 3, so nothing reaches 4 but what the filter sends there.
const Band = 4
