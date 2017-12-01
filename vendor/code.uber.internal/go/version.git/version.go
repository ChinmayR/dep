// Package version is used to provide build information to binaries
// build using go-build.
//
// It only works for binaries using go-build, as go-build provides the
// information for this package through linker variables.
package version

import (
	"os"
	"strconv"
	"time"
)

const (
	_defaultBuildHash = "unknown-build-hash"
	_refKey           = "GIT_DESCRIBE"
)

var (
	// BuildTime is set to the time on the build machine.
	BuildTime time.Time

	// BuildHash is information about the version that this was built using.
	// it is generated using "git describe --always --dirty".
	BuildHash = _defaultBuildHash

	// BuildUserHost is the user and hostname used to build the binary in
	// the format USER@HOSTNAME.
	BuildUserHost = "unknown@unknown"
)

// buildUnixSeconds is the string set through the linker flag that ends up
// being used to set BuildTime.
var buildUnixSeconds string

func init() {
	setBuildTime()
	fallbackBuildHash()
}

func setBuildTime() {
	if seconds, err := strconv.Atoi(buildUnixSeconds); err == nil {
		BuildTime = time.Unix(int64(seconds), 0)
	}
}

func fallbackBuildHash() {
	// If we couldn't determine the build hash at build time, we may be able to
	// do so at runtime.
	if BuildHash == "" || BuildHash == _defaultBuildHash {
		if hash := os.Getenv(_refKey); hash != "" {
			BuildHash = hash
		}
	}
}
