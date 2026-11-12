//go:build linux

package netem

import (
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

// Apply drops percent of the TCP packets sent from sport, on every
// interface: a prio root qdisc, netem with the loss on band Band, and a u32
// filter sending sport's packets there.
func Apply(sport int, percent float64) error {
	ls, err := links()
	if err != nil {
		return err
	}
	for _, l := range ls {
		idx := l.Attrs().Index
		prio := netlink.NewPrio(netlink.QdiscAttrs{LinkIndex: idx, Handle: netlink.MakeHandle(1, 0), Parent: netlink.HANDLE_ROOT})
		prio.Bands = Band
		if err := netlink.QdiscAdd(prio); err != nil {
			return fmt.Errorf("%s: prio qdisc: %w", l.Attrs().Name, err)
		}
		nq := netlink.NewNetem(netlink.QdiscAttrs{LinkIndex: idx, Handle: netlink.MakeHandle(40, 0), Parent: netlink.MakeHandle(1, Band)},
			netlink.NetemQdiscAttrs{Loss: float32(percent)})
		if err := netlink.QdiscAdd(nq); err != nil {
			return fmt.Errorf("%s: netem qdisc: %w", l.Attrs().Name, err)
		}
		// The TCP source port is the first 16 bits after a 20-byte IPv4
		// header, as tc's "match ip sport" reads it; the flags byte is 13
		// bytes into the TCP header. Packets with SYN set (the SYN-ACKs of
		// new connections) aren't dropped: every service-to-service
		// connection runs through Toxiproxy, which appears to dial
		// upstream in the loop that accepts its clients, so one lost
		// SYN-ACK held every new connection behind it for a second retry,
		// and a 150 ms timeout's reconnects piled up into 30-second waits.
		// A direct client wouldn't wait behind other clients' handshakes.
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

// Clear removes what Apply added: the root qdisc, and with it the rest.
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
