// Package scenarios holds the scenario library. Each folder is one
// scenario; loadl carries a copy so `loadl scenarios` and `loadl hint` work
// outside a checkout.
package scenarios

import "embed"

// FS is the library: one folder per scenario.
//
//go:embed */*
var FS embed.FS
