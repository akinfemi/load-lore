//go:build linux

package netem

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// links are the namespace's interfaces other than loopback.
func links() ([]netlink.Link, error) {
	all, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}
	var out []netlink.Link
	for _, l := range all {
		if l.Attrs().Flags&net.FlagLoopback == 0 {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no network interface besides loopback")
	}
	return out, nil
}

// Base adds delayUs to every packet the namespace sends, the network
// profile's half of a round trip: a prio root qdisc sending everything to
// band 1, with netem's delay there. With 0 it leaves the default qdisc.
func Base(delayUs int) error {
	if err := Clear(); err != nil {
		return err
	}
	if delayUs == 0 {
		return nil
	}
	ls, err := links()
	if err != nil {
		return err
	}
	for _, l := range ls {
		if err := rootPrio(l, delayUs, 0); err != nil {
			return err
		}
	}
	return nil
}

// Apply drops percent of the TCP packets sent from sport, on every
// interface: a prio root qdisc, netem with the loss on band Band, and a u32
// filter sending sport's packets there. delayUs is the network profile's
// delay, kept on every packet, the dropped ones' retransmits included.
func Apply(sport int, percent float64, delayUs int) error {
	if err := Clear(); err != nil {
		return err
	}
	ls, err := links()
	if err != nil {
		return err
	}
	for _, l := range ls {
		idx := l.Attrs().Index
		if err := rootPrio(l, delayUs, percent); err != nil {
			return err
		}
		// The TCP source port is the first 16 bits after a 20-byte IPv4
		// header, as tc's "match ip sport" reads it; the flags byte is 13
		// bytes into the TCP header. Packets with SYN set (the SYN-ACKs of
		// new connections) aren't dropped. That began for connections
		// through Toxiproxy, which dials upstream in the loop that accepts
		// its clients, so one lost SYN-ACK held every new connection behind
		// it for a second retry and a 150 ms timeout's reconnects piled up
		// into 30-second waits. It stays for direct connections too, so
		// the loss fault means the same on both: lost data, not lost
		// handshakes.
		f := &netlink.U32{
			FilterAttrs: netlink.FilterAttrs{LinkIndex: idx, Parent: netlink.MakeHandle(1, 0), Priority: 1, Protocol: unix.ETH_P_IP},
			ClassId:     netlink.MakeHandle(1, Band),
			Sel: &netlink.TcU32Sel{Flags: netlink.TC_U32_TERMINAL, Nkeys: 2, Keys: []netlink.TcU32Key{
				{Mask: 0xffff0000, Val: uint32(sport) << 16, Off: 20},
				{Mask: 0x00020000, Val: 0, Off: 32}, // SYN clear
			}},
		}
		if err := netlink.FilterAdd(f); err != nil {
			return fmt.Errorf("%s: u32 filter: %w", l.Attrs().Name, err)
		}
	}
	return nil
}

// Cut drops every packet sent to the given TCP ports or IPv4 addresses,
// handshakes included: a partition, as a network that silently stopped
// carrying them. delayUs is the network profile's delay, kept on the rest.
func Cut(dports []int, dsts []net.IP, delayUs int) error {
	if err := Clear(); err != nil {
		return err
	}
	ls, err := links()
	if err != nil {
		return err
	}
	for _, l := range ls {
		idx := l.Attrs().Index
		if err := rootPrio(l, delayUs, 100); err != nil {
			return err
		}
		var keys []netlink.TcU32Key
		for _, port := range dports {
			// The destination port is the second 16 bits of the TCP header.
			keys = append(keys, netlink.TcU32Key{Mask: 0x0000ffff, Val: uint32(port), Off: 20})
		}
		for _, ip := range dsts {
			v4 := ip.To4()
			if v4 == nil {
				return fmt.Errorf("%s isn't an IPv4 address", ip)
			}
			// The destination address is 16 bytes into the IPv4 header.
			keys = append(keys, netlink.TcU32Key{Mask: 0xffffffff, Val: binary.BigEndian.Uint32(v4), Off: 16})
		}
		for i, k := range keys {
			f := &netlink.U32{
				FilterAttrs: netlink.FilterAttrs{LinkIndex: idx, Parent: netlink.MakeHandle(1, 0), Priority: uint16(1 + i), Protocol: unix.ETH_P_IP},
				ClassId:     netlink.MakeHandle(1, Band),
				Sel:         &netlink.TcU32Sel{Flags: netlink.TC_U32_TERMINAL, Nkeys: 1, Keys: []netlink.TcU32Key{k}},
			}
			if err := netlink.FilterAdd(f); err != nil {
				return fmt.Errorf("%s: u32 filter %d: %w", l.Attrs().Name, i+1, err)
			}
		}
	}
	return nil
}

// netemLimit is how many packets each netem holds: Linux's default 1,000
// would drop packets at a busy container's rate behind a delay.
const netemLimit = 100000

// rootPrio adds the prio root qdisc, every packet on band 1 by default,
// with the profile's delay there if any, and netem on band Band with the
// delay and the loss.
func rootPrio(l netlink.Link, delayUs int, percent float64) error {
	idx := l.Attrs().Index
	prio := netlink.NewPrio(netlink.QdiscAttrs{LinkIndex: idx, Handle: netlink.MakeHandle(1, 0), Parent: netlink.HANDLE_ROOT})
	prio.Bands = Band
	prio.PriorityMap = [netlink.PRIORITY_MAP_LEN]uint8{} // all to band 1
	if err := netlink.QdiscAdd(prio); err != nil {
		return fmt.Errorf("%s: prio qdisc: %w", l.Attrs().Name, err)
	}
	if delayUs > 0 {
		base := netlink.NewNetem(netlink.QdiscAttrs{LinkIndex: idx, Handle: netlink.MakeHandle(10, 0), Parent: netlink.MakeHandle(1, 1)},
			netlink.NetemQdiscAttrs{Latency: uint32(delayUs), Limit: netemLimit})
		if err := netlink.QdiscAdd(base); err != nil {
			return fmt.Errorf("%s: netem delay qdisc: %w", l.Attrs().Name, err)
		}
	}
	if percent == 0 {
		return nil
	}
	nq := netlink.NewNetem(netlink.QdiscAttrs{LinkIndex: idx, Handle: netlink.MakeHandle(40, 0), Parent: netlink.MakeHandle(1, Band)},
		netlink.NetemQdiscAttrs{Loss: float32(percent), Latency: uint32(delayUs), Limit: netemLimit})
	if err := netlink.QdiscAdd(nq); err != nil {
		return fmt.Errorf("%s: netem qdisc: %w", l.Attrs().Name, err)
	}
	return nil
}

// Clear removes everything: the root qdisc, and with it the rest. To end a
// fault and keep the network profile, call Base.
func Clear() error {
	ls, err := links()
	if err != nil {
		return err
	}
	var errs []error
	for _, l := range ls {
		root := &netlink.GenericQdisc{QdiscAttrs: netlink.QdiscAttrs{LinkIndex: l.Attrs().Index, Handle: netlink.MakeHandle(1, 0), Parent: netlink.HANDLE_ROOT}, QdiscType: "prio"}
		if err := netlink.QdiscDel(root); err != nil && !errors.Is(err, unix.ENOENT) && !errors.Is(err, unix.EINVAL) {
			errs = append(errs, fmt.Errorf("%s: %w", l.Attrs().Name, err))
		}
	}
	return errors.Join(errs...)
}
