// Package cluster manages the lifecycle of the Kubernetes control plane used
// by the apply-tier runner. Implementations live in sub-packages
// (envtest, simulated, kind, k3d, kubeconfig); the factory selects one
// based on the user-provided Config.
package cluster

import (
	"context"

	"k8s.io/client-go/rest"
)

// Backend abstracts the Kubernetes control plane that the apply-tier runner
// installs charts into. Implementations may be real clusters (kind, k3d,
// external kubeconfig) or fast in-process control planes (envtest, eventually
// simulated).
//
// Start/Stop bracket the entire run; RESTConfig and Kubeconfig give callers
// the credentials they need. LoadImages is a hook for backends that can
// import container images into their node(s); backends that pull images at
// runtime (envtest) or that have no node access (kubeconfig) implement it as
// a no-op or return an error.
type Backend interface {
	// Start launches the cluster and blocks until the API server is reachable.
	Start(ctx context.Context) error
	// Stop tears down resources created by Start. Safe to call when Start was
	// never invoked or returned an error.
	Stop(ctx context.Context) error
	// RESTConfig returns the REST credentials after a successful Start.
	// Returns nil if Start has not been called.
	RESTConfig() *rest.Config
	// Kubeconfig returns the path to a kubeconfig file for the running cluster.
	// Returns empty string for backends that do not materialise an on-disk
	// kubeconfig (envtest).
	Kubeconfig() string
	// LoadImages pre-loads container images into the cluster. Returns a non-nil
	// error for backends that cannot push images directly (e.g. kubeconfig).
	LoadImages(ctx context.Context, images []string) error
}

// Config selects and configures the cluster backend for a test run.
type Config struct {
	// Type selects the implementation: envtest | simulated | kind | k3d | kubeconfig.
	// Defaults to "envtest" (fast and dependency-free).
	Type string
	// KubeVersion is the target Kubernetes server version. Used by envtest to
	// pin the binary asset version, and by node-backed backends (kind, k3d) to
	// pin the node image.
	KubeVersion string
	// Kubeconfig is the path to an external kubeconfig file. Only used when
	// Type == "kubeconfig".
	Kubeconfig string
	// ExtraArgs are additional flags passed verbatim to the backend's server
	// process (honoured by the node-backed backends kind and k3d).
	ExtraArgs []string
}
