package quartr

import (
	"strings"
	"testing"
)

func TestResolveVersionPrefersInjectedBuildVersion(t *testing.T) {
	original := buildVersion
	t.Cleanup(func() { buildVersion = original })

	// The release build injects a bare "1.2.3", but a tag-shaped "v1.2.3"
	// must not produce a doubled prefix in `quartr vv1.2.3`.
	for _, injected := range []string{"1.2.3", "v1.2.3", "  1.2.3  "} {
		buildVersion = injected
		if got := resolveVersion(); got != "1.2.3" {
			t.Errorf("resolveVersion() with buildVersion=%q = %q, want %q", injected, got, "1.2.3")
		}
	}
}

// Without an injected version the value comes from the build info the go tool
// stamps in. Under `go test` that is the VCS revision, so the only invariant
// worth asserting is that the fallback chain always yields something usable.
func TestResolveVersionFallsBackToBuildInfo(t *testing.T) {
	original := buildVersion
	t.Cleanup(func() { buildVersion = original })

	buildVersion = ""
	got := resolveVersion()
	if got == "" {
		t.Fatal("resolveVersion() returned an empty string; want a revision or \"dev\"")
	}
	if strings.HasPrefix(got, "v") {
		t.Errorf("resolveVersion() = %q; the leading v should be stripped", got)
	}
}

func TestUserAgentTracksVersion(t *testing.T) {
	if want := "quartr-cli/" + Version; UserAgent != want {
		t.Errorf("UserAgent = %q, want %q", UserAgent, want)
	}
	if strings.Contains(UserAgent, "0.1.0") && Version != "0.1.0" {
		t.Error("UserAgent carries a hardcoded version that Version does not agree with")
	}
}
