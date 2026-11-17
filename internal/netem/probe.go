package netem

import (
	"fmt"
	"io"
	"net"
	"slices"
	"time"
)

// ProbePort is where Echo listens, in the namespace of a stack container
// that no service uses it in.
const ProbePort = 7999

// Echo answers every connection on port with what it sends, until killed:
// the far end of RTT.
func Echo(port int) error {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer c.Close()
			_, _ = io.Copy(c, c)
		}()
	}
}

// RTT measures the median round trip of n small messages to an Echo at
// addr, one at a time on one connection, retrying the connection for up
// to 10 s while the echo starts.
func RTT(addr string, n int) (time.Duration, error) {
	var c net.Conn
	var err error
	for deadline := time.Now().Add(10 * time.Second); ; {
		if c, err = net.DialTimeout("tcp", addr, time.Second); err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return 0, err
	}
	defer c.Close()
	buf := make([]byte, 64)
	ds := make([]time.Duration, 0, n)
	for range n {
		s := time.Now()
		if _, err := c.Write(buf); err != nil {
			return 0, err
		}
		if _, err := io.ReadFull(c, buf); err != nil {
			return 0, err
		}
		ds = append(ds, time.Since(s))
	}
	slices.Sort(ds)
	return ds[len(ds)/2], nil
}
