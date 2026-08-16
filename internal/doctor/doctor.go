// Package doctor implements environment prerequisite checks and binary
// resolution used by the cluster backends. The full `vigie doctor` command and
// the simulated-backend checks land in later milestones; this package carries
// the envtest bootstrap plus the node-backed (kind, k3d) diagnostics.
//
// The package is split into focused units:
//
//   - doctor.go (this file) - shared types (Status, Check) and constants.
//   - envtest.go - KUBEBUILDER_ASSETS validation and setup-envtest fallback.
//   - envtest_download.go - auto-download fallback for envtest binaries.
//   - k8s_binary_download.go - shared downloader for dl.k8s.io / GitHub-released tools.
//   - runtime.go - docker/podman container-runtime detection.
//   - kind.go, k3d.go - node-backend binary resolution and readiness checks.
//   - exec.go - small indirection over os/exec to ease testing.
package doctor

import (
	"fmt"
	"io"
	"time"

	"github.com/fregateops/vigie/internal/config"
)

// Status of a single check.
type Status string

const (
	StatusOK      Status = "ok"
	StatusWarning Status = "warning"
	StatusError   Status = "error"
)

// Check is the outcome of one prerequisite check.
type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
	// Group tags the check for grouped display in the doctor output.
	// Consumers that only care about name/status/detail may ignore this field.
	Group string `json:"group,omitempty"`
}

// DefaultKubeVersion is the Kubernetes version vigie provisions when no
// version is requested. It aliases config.DefaultKubeVersion so the literal
// lives in exactly one place (internal/config/kubeversion.go), while the
// envtest backend - which discovers binaries through this package - can
// reference it without importing config. Must be a full X.Y.Z semver: the
// dl.k8s.io download URL requires the patch component.
const DefaultKubeVersion = config.DefaultKubeVersion

// emitProgress writes a progress line to w when w is non-nil.
func emitProgress(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, format, args...)
}

// elapsed rounds a Since() duration to millisecond precision.
func elapsed(start time.Time) time.Duration {
	return time.Since(start).Round(time.Millisecond)
}
