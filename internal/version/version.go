// Package version holds build-time version metadata.
package version

import "fmt"

// Version is the semantic version. Commit and BuildDate are injected via
// -ldflags at build time.
const Version = "0.1.0"

var (
	Commit    = "dev"
	BuildDate = "unknown"
)

// String returns a human-readable version line.
func String() string {
	return fmt.Sprintf("archive-sync %s (commit %s, built %s)", Version, Commit, BuildDate)
}
