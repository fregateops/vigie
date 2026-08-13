// Package kubeconfig implements the e2e Backend interface for an already-running
// external cluster reached via a kubeconfig file.
package kubeconfig

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ErrImageLoadNotSupported is returned by LoadImages because image pre-loading
// requires direct node access, which is not available for external clusters.
var ErrImageLoadNotSupported = errors.New("image pre-loading is not supported for external kubeconfig clusters")

// Backend implements the e2e Backend interface for an already-running cluster
// reached via an external kubeconfig file. It does not create or destroy any
// cluster - Start only verifies reachability and Stop is a no-op.
type Backend struct {
	kubeconfigPath string
	restConfig     *rest.Config
}

// New returns an unstarted Backend configured to connect via the given
// kubeconfig path.
func New(kubeconfigPath string) *Backend {
	return &Backend{kubeconfigPath: kubeconfigPath}
}

// Start loads the kubeconfig, builds a REST config, and verifies the API
// server is reachable by performing a version ping. It does not create any
// cluster resources.
func (b *Backend) Start(_ context.Context) error {
	restCfg, err := clientcmd.BuildConfigFromFlags("", b.kubeconfigPath)
	if err != nil {
		return fmt.Errorf("loading kubeconfig %q: %w", b.kubeconfigPath, err)
	}

	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("building kubernetes client from kubeconfig %q: %w", b.kubeconfigPath, err)
	}

	serverVer, err := client.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("API server at %q not reachable (version ping failed): %w", restCfg.Host, err)
	}

	slog.Info("connected to external cluster", "server", restCfg.Host, "version", serverVer.GitVersion)
	b.restConfig = restCfg
	return nil
}

// Stop is a no-op; the external cluster is not torn down.
// It logs a warning reminding users that namespace cleanup depends on the
// runner completing without interruption.
func (b *Backend) Stop(_ context.Context) error {
	slog.Warn("external kubeconfig cluster not torn down; namespace cleanup depends on the runner completing without interruption",
		"kubeconfig", b.kubeconfigPath)
	return nil
}

// RESTConfig returns the loaded REST config, or nil if Start has not been called.
func (b *Backend) RESTConfig() *rest.Config {
	return b.restConfig
}

// Kubeconfig returns the kubeconfig path provided to New.
func (b *Backend) Kubeconfig() string {
	return b.kubeconfigPath
}

// LoadImages returns an error because image pre-loading is not supported for
// external clusters - there is no direct node access to push images into.
func (b *Backend) LoadImages(_ context.Context, _ []string) error {
	return fmt.Errorf("%w: use a registry or ensure images are already present on cluster nodes", ErrImageLoadNotSupported)
}
