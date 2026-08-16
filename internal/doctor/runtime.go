package doctor

import (
	"fmt"
	"strings"
)

// ContainerRuntime reports the container runtime to use ("docker" or
// "podman"), preferring docker, or "" when neither binary is on PATH. Unlike
// DetectContainerRuntime it does not probe the daemon: callers that only need
// to pick a provider (e.g. kind's KIND_EXPERIMENTAL_PROVIDER) shouldn't require
// the daemon to be responsive, and a present-but-down daemon still fixes which
// provider to select.
func ContainerRuntime() string {
	if _, err := lookPath("docker"); err == nil {
		return "docker"
	}
	if _, err := lookPath("podman"); err == nil {
		return "podman"
	}
	return ""
}

// DetectContainerRuntime checks for docker first, then podman. Returns a
// warning Check on failure (a container runtime is not required for the
// envtest tier - it is used by `test --cluster kind|k3d`).
func DetectContainerRuntime() Check {
	if check, found := probeRuntime("docker", []string{"version", "--format", "{{.Server.Version}}"},
		"daemon not responding", "server version"); found {
		return check
	}
	if check, found := probeRuntime("podman", []string{"version", "--format", "{{.Version}}"},
		"not responding", "version"); found {
		return check
	}

	return Check{
		Name:   "docker",
		Status: StatusWarning,
		Detail: "not found (required for test --cluster kind|k3d)",
	}
}

// probeRuntime resolves a container-runtime binary on PATH and queries its
// version. found is false when the binary isn't on PATH (so the caller can try
// the next runtime). When found, the Check is OK on a successful version query
// or a Warning when the binary is present but the query fails. notRespondingMsg
// and versionLabel tailor the wording per runtime.
func probeRuntime(name string, versionArgs []string, notRespondingMsg, versionLabel string) (Check, bool) {
	path, err := lookPath(name)
	if err != nil {
		return Check{}, false
	}
	out, runErr := runCommand(path, versionArgs...)
	if runErr != nil {
		return Check{
			Name:   name,
			Status: StatusWarning,
			Detail: fmt.Sprintf("binary found at %s but %s", path, notRespondingMsg),
		}, true
	}
	return Check{
		Name:   name,
		Status: StatusOK,
		Detail: fmt.Sprintf("found at %s (%s %s)", path, versionLabel, strings.TrimSpace(string(out))),
	}, true
}
