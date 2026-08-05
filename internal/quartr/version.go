package quartr

import (
	"runtime/debug"
	"strings"
)

// buildVersion is overridden at link time by the release build:
//
//	go build -ldflags "-X github.com/TJC-LP/quartr-cli/internal/quartr.buildVersion=1.2.3"
//
// It is deliberately empty by default. Everything else falls back to the build
// information the go tool stamps into the binary on its own, so a user who runs
// `go install github.com/TJC-LP/quartr-cli/cmd/quartr@v0.1.0` still gets a
// binary that reports 0.1.0 without any linker flags involved.
var buildVersion = ""

// Version is the release string reported by `quartr --version`. It is resolved
// once, here, rather than in cli, so the version the CLI prints and the one it
// puts on the wire in User-Agent cannot drift apart.
var Version = resolveVersion()

// UserAgent is sent on every outbound request.
var UserAgent = "quartr-cli/" + Version

// resolveVersion reports, in order of preference: the linker-injected release
// string, the module version recorded by `go install module@version`, or the
// VCS revision stamped into a `go build` from a checkout. "dev" is the answer
// only when none of the three is available.
func resolveVersion() string {
	if v := strings.TrimSpace(buildVersion); v != "" {
		return strings.TrimPrefix(v, "v")
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}

	// Set when the binary came from `go install module@version`. A plain
	// `go build` inside the module reports "" or "(devel)" instead.
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return strings.TrimPrefix(v, "v")
	}

	var revision string
	var modified bool
	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return "dev"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}
