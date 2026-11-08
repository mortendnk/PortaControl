// Package version holds build information set with -ldflags.
package version

// Set by the build (-X).
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String is "version (commit, date)".
func String() string { return Version + " (" + Commit + ", " + Date + ")" }
