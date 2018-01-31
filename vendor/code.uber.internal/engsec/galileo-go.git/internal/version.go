package internal

import "fmt"

// Version is the semver compliant version string of the galileo-go client
// library. Update this by hand and then run `make release` to cut a new version.
// See README.md for release process and instructions.
const Version = "1.7.0"

// LibraryVersion returns the language and version so we can track adoption
// through various logs and metrics.
// Differentiates us from the galileo libraries in java, python, node, etc.
func LibraryVersion() string {
	return fmt.Sprintf("galileo-go: %s", Version)
}
