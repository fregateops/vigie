package envtest

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/fregateops/vigie/internal/doctor"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// Backend implements cluster.Backend using controller-runtime's envtest.
// It starts real kube-apiserver and etcd binaries as subprocesses.
type Backend struct {
	env         *envtest.Environment
	cfg         *rest.Config
	kubeVersion string
}

// New returns an envtest Backend configured for the given Kubernetes version.
// An empty kubeVersion uses the vigie default.
func New(kubeVersion string) *Backend {
	if kubeVersion == "" {
		kubeVersion = doctor.DefaultKubeVersion
	}
	return &Backend{kubeVersion: kubeVersion}
}

// ErrBinariesNotFound is returned by Start when neither KUBEBUILDER_ASSETS nor
// setup-envtest produce a usable kube-apiserver + etcd pair. Callers can
// errors.Is-check this sentinel to surface doctor-style guidance.
var ErrBinariesNotFound = errors.New("envtest binaries not found")

// Start resolves envtest binaries via the doctor helpers
// (KUBEBUILDER_ASSETS, then setup-envtest, then auto-download into the
// vigie cache) and launches the embedded apiserver + etcd. When no
// binaries can be located or downloaded the returned error wraps
// ErrBinariesNotFound and points the user at `vigie doctor`.
func (b *Backend) Start(ctx context.Context) error {
	binaries, err := doctor.EnsureEnvtestBinaries(ctx, b.kubeVersion, nil)
	if err != nil {
		return fmt.Errorf("%w: %w — run 'vigie doctor' to install prerequisites",
			ErrBinariesNotFound, err)
	}

	b.env = &envtest.Environment{
		BinaryAssetsDirectory: filepath.Dir(binaries.APIServerPath),
	}
	cfg, err := b.env.Start()
	if err != nil {
		return fmt.Errorf("starting envtest with binaries from %s: %w (run 'vigie doctor')",
			binaries.Source, err)
	}
	b.cfg = cfg
	return nil
}

// RESTConfig returns the REST config produced by Start, or nil if Start has
// not been called (or failed).
func (b *Backend) RESTConfig() *rest.Config {
	return b.cfg
}

// Kubeconfig returns the empty string because envtest does not write its
// credentials to disk - the REST config is in-process only.
func (b *Backend) Kubeconfig() string { return "" }

// LoadImages is a no-op because envtest has no node to load images onto.
// Tests that depend on cluster-side image pulls should use a node-backed
// backend (kind, k3d).
func (b *Backend) LoadImages(_ context.Context, _ []string) error { return nil }

// Stop tears down the embedded apiserver + etcd. Safe to call when Start was
// never called or returned an error.
func (b *Backend) Stop(_ context.Context) error {
	if b.env == nil {
		return nil
	}
	return b.env.Stop()
}

// CertDir returns the directory where the envtest control plane writes
// its TLS material (apiserver.crt, apiserver.key, sa-signer.crt,
// sa-signer.key). The simulated backend needs these paths to start
// auxiliary processes (kube-controller-manager, kube-scheduler) that
// authenticate against the same apiserver. Returns "" before Start has
// completed.
func (b *Backend) CertDir() string {
	if b.env == nil {
		return ""
	}
	apiserver := b.env.ControlPlane.GetAPIServer()
	if apiserver == nil {
		return ""
	}
	return apiserver.CertDir
}
