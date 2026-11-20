// Package integrationtest gates the `integration`-tagged tests on the
// external tools they drive (kind, k3d, a container runtime).
package integrationtest

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// requireEnv lists, comma-separated, the tools an environment guarantees
// (e.g. CI sets "k3d,docker"). A listed tool that is missing fails the test
// instead of skipping it, so a broken CI setup cannot pass by testing nothing.
const requireEnv = "VIGIE_INTEGRATION_REQUIRE"

// NodeBackendTools skips, or fails when required, unless the backend's CLI
// and a container runtime (docker or podman) are on PATH.
func NodeBackendTools(t testing.TB, backend string) {
	t.Helper()
	if !onPath(backend) {
		missing(t, backend, backend+" not found in PATH")
	}
	if !onPath("docker") && !onPath("podman") {
		missing(t, "docker", "neither docker nor podman found in PATH")
	}
}

func missing(t testing.TB, name, reason string) {
	t.Helper()
	if slices.Contains(strings.Split(os.Getenv(requireEnv), ","), name) {
		t.Fatalf("%s, but %s lists it as required", reason, requireEnv)
	}
	t.Skipf("%s; skipping (set %s=%s to require it)", reason, requireEnv, name)
}

func onPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
