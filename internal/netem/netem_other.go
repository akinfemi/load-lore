//go:build !linux

package netem

import (
	"errors"
	"net"
)

var errLinux = errors.New("netem needs Linux: it runs inside a container")

// Base needs Linux.
func Base(int) error { return errLinux }

// Apply needs Linux.
func Apply(int, float64, int) error { return errLinux }

// Cut needs Linux.
func Cut([]int, []net.IP, int) error { return errLinux }

// Clear needs Linux.
func Clear() error { return errLinux }
