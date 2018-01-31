package wonka

import "fmt"

// Version is the semver compliant version string of the wonka-go client
// library. Update this by hand and then run `make release` to cut a new version.
// See README.md for release process and instructions.
const Version = "1.6.0"

// BuildVersion is the git hash associated with the current build of the library.
var BuildVersion string

// LibraryVersion returns the language and version so we can track adoption
// through various logs and metrics.
// Differentiates us from potential future wonka implementations in other
// languages.
func LibraryVersion() string {
	return fmt.Sprintf("wonka-go: %s", Version)
}
