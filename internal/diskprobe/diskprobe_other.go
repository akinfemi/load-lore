//go:build !linux

package diskprobe

import "errors"

var errLinux = errors.New("the disk probe needs Linux: it runs inside a container")

// Probe needs Linux.
func Probe(string, int, int) (*Result, error) { return nil, errLinux }

// Clean needs Linux.
func Clean(string) error { return errLinux }
