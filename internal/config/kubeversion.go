package config

import (
	"fmt"
	"regexp"
)

// DefaultKubeVersion is the Kubernetes version vigie validates against when a
// command or config supplies none — kubeconform fetches the matching schemas
// and `helm template` reports it as `.Capabilities.KubeVersion`. Cluster
// backends (M6+) will provision this same version by default.
const DefaultKubeVersion = "1.36.1"

// kubeVersionPattern matches a full Kubernetes semver: X.Y.Z with an optional
// leading "v". Truncated forms like "1.30" are rejected because the dl.k8s.io
// download URL (used by EnsureKubernetesBinary for kube-controller-manager and
// kube-scheduler) requires the patch component — "v1.30/bin/..." returns 404
// while "v1.30.0/bin/..." resolves.
var kubeVersionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// ValidateKubeVersion enforces that a user-supplied Kubernetes version is a
// full X.Y.Z semver (optionally prefixed with "v"). It returns a descriptive
// error pointing at the source field when the value is truncated or malformed.
// Empty strings are accepted — callers treat them as "fall back to the default".
func ValidateKubeVersion(source, value string) error {
	if value == "" {
		return nil
	}
	if !kubeVersionPattern.MatchString(value) {
		return fmt.Errorf("%s: %q is not a full Kubernetes version — use X.Y.Z (e.g. 1.36.1), not X.Y; the binary download URL requires the patch component", source, value)
	}
	return nil
}

// validateKubeVersions runs ValidateKubeVersion across a slice, returning the
// first failure. Used by config.Load to vet every kubeVersions list at once.
func validateKubeVersions(source string, values []string) error {
	for idx, v := range values {
		if err := ValidateKubeVersion(fmt.Sprintf("%s[%d]", source, idx), v); err != nil {
			return err
		}
	}
	return nil
}
