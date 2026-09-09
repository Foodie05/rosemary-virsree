package buildinfo

import "strings"

// Version and Commit are replaced through -ldflags for release builds.
var (
	Version = "dev"
	Commit  = "unknown"
)

func NormalizedVersion() string {
	version := strings.TrimSpace(strings.TrimPrefix(Version, "v"))
	if version == "" {
		return "dev"
	}
	return version
}
