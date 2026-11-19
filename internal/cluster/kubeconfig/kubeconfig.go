// Package kubeconfig implements the e2e Backend interface for an already-running
// external cluster reached via a kubeconfig file.
package kubeconfig

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/fregateops/vigie/internal/clog"
)

// ErrImageLoadNotSupported is returned by LoadImages because image pre-loading
// requires direct node access, which is not available for external clusters.
var ErrImageLoadNotSupported = errors.New("image pre-loading is not supported for external kubeconfig clusters")

// Backend implements the e2e Backend interface for an already-running cluster
// reached via an external kubeconfig file. It does not create or destroy any
// cluster - Start only verifies reachability and Stop is a no-op.
type Backend struct {
	kubeconfigPath string
	// contextName selects the kubeconfig context; empty means the file's
	// current-context.
	contextName string
	restConfig  *rest.Config
	// pinnedPath is a kubeconfig holding only the selected context, handed to
	// lifecycle hooks so their kubectl targets the cluster vigie tests even
	// when the source file's current-context differs or changes mid-run.
	pinnedPath string
}

// New returns an unstarted Backend for the given kubeconfig and context.
func New(kubeconfigPath, contextName string) *Backend {
	return &Backend{kubeconfigPath: kubeconfigPath, contextName: contextName}
}

// Start resolves the context, verifies the API server answers, announces the
// target cluster, and writes the pinned kubeconfig. It creates nothing in the
// cluster.
func (b *Backend) Start(_ context.Context) error {
	clientCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: b.kubeconfigPath},
		&clientcmd.ConfigOverrides{CurrentContext: b.contextName},
	)
	raw, err := clientCfg.RawConfig()
	if err != nil {
		return fmt.Errorf("loading kubeconfig %q: %w", b.kubeconfigPath, err)
	}
	contextName := cmp.Or(b.contextName, raw.CurrentContext)
	if _, ok := raw.Contexts[contextName]; !ok {
		return fmt.Errorf("context %q not found in kubeconfig %q (available: %s)",
			contextName, b.kubeconfigPath, strings.Join(slices.Sorted(maps.Keys(raw.Contexts)), ", "))
	}
	restCfg, err := clientCfg.ClientConfig()
	if err != nil {
		return fmt.Errorf("building client config for context %q: %w", contextName, err)
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("building kubernetes client from kubeconfig %q: %w", b.kubeconfigPath, err)
	}
	serverVer, err := client.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("API server at %q not reachable (version ping failed): %w", restCfg.Host, err)
	}

	pinnedPath, err := writePinnedKubeconfig(raw, contextName)
	if err != nil {
		return err
	}
	clog.Progress("kubeconfig: testing against context %q, server %s (Kubernetes %s)",
		contextName, restCfg.Host, serverVer.GitVersion)
	b.restConfig = restCfg
	b.pinnedPath = pinnedPath
	return nil
}

func writePinnedKubeconfig(raw clientcmdapi.Config, contextName string) (string, error) {
	raw.CurrentContext = contextName
	if err := clientcmdapi.FlattenConfig(&raw); err != nil {
		return "", fmt.Errorf("inlining kubeconfig credentials: %w", err)
	}
	if err := clientcmdapi.MinifyConfig(&raw); err != nil {
		return "", fmt.Errorf("reducing kubeconfig to context %q: %w", contextName, err)
	}
	file, err := os.CreateTemp("", "vigie-kubeconfig-*.yaml")
	if err != nil {
		return "", fmt.Errorf("creating pinned kubeconfig: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("creating pinned kubeconfig: %w", err)
	}
	if err := clientcmd.WriteToFile(raw, file.Name()); err != nil {
		return "", fmt.Errorf("writing pinned kubeconfig: %w", err)
	}
	return file.Name(), nil
}

// Stop removes the pinned kubeconfig; the external cluster itself is left
// running.
func (b *Backend) Stop(_ context.Context) error {
	if b.pinnedPath == "" {
		return nil
	}
	if err := os.Remove(b.pinnedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing pinned kubeconfig: %w", err)
	}
	return nil
}

// RESTConfig returns the loaded REST config, or nil if Start has not been called.
func (b *Backend) RESTConfig() *rest.Config {
	return b.restConfig
}

// Kubeconfig returns the kubeconfig pinned to the selected context once
// started, the source file before.
func (b *Backend) Kubeconfig() string {
	return cmp.Or(b.pinnedPath, b.kubeconfigPath)
}

// LoadImages returns an error because image pre-loading is not supported for
// external clusters - there is no direct node access to push images into.
func (b *Backend) LoadImages(_ context.Context, _ []string) error {
	return fmt.Errorf("%w: use a registry or ensure images are already present on cluster nodes", ErrImageLoadNotSupported)
}
