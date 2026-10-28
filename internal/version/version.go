// Package version holds the sshu build version, injected at build time via
// goreleaser ldflags (-X <module>/internal/version.Version=..., <module> being
// go.mod's module path, its /vN included from v2 on).
// A local `go build` / `make build` produces a "dev" build.
//
// terminu family template (tdp project/version.go), copied to an app's
// internal/version/version.go; replace sshu.
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
