// Package version holds build metadata, set with -ldflags at build time.
package version

// Version is the loadl release. "dev" for local builds.
var Version = "dev"

// Commit is the git commit the binary was built from.
var Commit = "unknown"
