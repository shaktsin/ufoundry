// Package version holds build metadata, set with -ldflags at build time.
package version

var (
	// Version is the engine/CLI version.
	Version = "0.1.0-dev"
	// Commit is the git commit the binary was built from.
	Commit = "unknown"
	// BuildID identifies one app+engine bundle build, including dirty builds.
	BuildID = ""
)
