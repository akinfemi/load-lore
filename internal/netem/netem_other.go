//go:build !linux

package netem

import "errors"

var errLinux = errors.New("netem needs Linux: it runs inside a container")

// Apply needs Linux.
func Apply(int, float64) error { return errLinux }

// Cut needs Linux.
func Cut([]int) error { return errLinux }

// Clear needs Linux.
func Clear() error { return errLinux }
