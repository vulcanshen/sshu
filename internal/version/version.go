// Package version holds the sshu build version, injected at build time via
// goreleaser ldflags (-X github.com/vulcanshen/sshu/internal/version.Version=...).
// A local `go build` / `make build` produces a "dev" build.
package version

// Version is the build version. Set to the goreleaser tag minus its "v" prefix
// (e.g. "0.1.0") for tagged releases, "dev" otherwise.
var Version = "dev"

// Display returns a human-friendly version string: "dev" for local builds,
// "v0.1.0" for tagged releases.
func Display() string {
	if Version == "dev" {
		return "dev"
	}
	return "v" + Version
}
