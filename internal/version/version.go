// Package version holds build metadata, set with -ldflags at build time.
package version

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// Version is the loadl release. "dev" for local builds.
var Version = "dev"

// Commit is the git commit the binary was built from.
var Commit = "unknown"

// Image is the loadlore/synthetic image this loadl runs. A release sets it
// with -ldflags to the published image of its own version; empty means
// DevImage.
var Image = ""

const (
	// DevImage is the image make image builds, for local builds.
	DevImage = "loadlore/synthetic:dev"
	// Published is where releases publish the synthetic image.
	Published = "ghcr.io/akinfemi/loadlore-synthetic"
)

func init() {
	if Version != "dev" {
		return
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v, ok := fromModule(bi.Main.Version); ok {
			Version = v
			if Image == "" {
				Image = Published + ":" + v
			}
		}
	}
}

var (
	tagRe    = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	pseudoRe = regexp.MustCompile(`\d{14}-[0-9a-f]{12}$`)
)

// fromModule reads the release from a module version, as go install
// pkg@v0.1.0 records it: v0.1.0 gives 0.1.0. Pseudo-versions and
// "(devel)" aren't releases.
func fromModule(v string) (string, bool) {
	if !tagRe.MatchString(v) || pseudoRe.MatchString(v) {
		return "", false
	}
	return strings.TrimPrefix(v, "v"), true
}

// SyntheticImage is the image for loadlore/synthetic services by default.
func SyntheticImage() string {
	if Image != "" {
		return Image
	}
	return DevImage
}

// Pullable says whether Docker can pull an image from a registry rather
// than needing it built: everything but a loadlore/synthetic image that
// make image builds.
func Pullable(image string) bool {
	return !strings.HasPrefix(image, "loadlore/synthetic:")
}
