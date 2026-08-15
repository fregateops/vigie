package cluster

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/fregateops/vigie/internal/cluster/envtest"
	"github.com/fregateops/vigie/internal/cluster/k3d"
	"github.com/fregateops/vigie/internal/cluster/kind"
	"github.com/fregateops/vigie/internal/cluster/kubeconfig"
)

// DefaultType is the cluster backend used when Config.Type is empty.
// envtest is fast, dependency-free, and matches the `--cluster envtest`
// default - making it the friendliest landing spot for new users.
const DefaultType = "envtest"

// New returns a Backend matching cfg.Type. Empty Type defaults to envtest.
//
// envtest (in-process apiserver), kubeconfig (an external cluster the user
// already runs), and the node-backed kind and k3d backends (each provisions a
// throwaway cluster on the fly) are available; the simulated backend arrives
// in a later milestone and currently returns an error so misconfiguration
// fails loudly.
func New(cfg Config) (Backend, error) {
	backendType := cfg.Type
	if backendType == "" {
		backendType = DefaultType
	}
	switch backendType {
	case "envtest":
		return envtest.New(cfg.KubeVersion), nil
	case "kind":
		return kind.New(clusterSessionName("vigie-kind"), cfg.KubeVersion, cfg.ExtraArgs), nil
	case "k3d":
		return k3d.New(clusterSessionName("vigie-k3d"), cfg.KubeVersion, cfg.ExtraArgs), nil
	case "kubeconfig":
		if cfg.Kubeconfig == "" {
			return nil, fmt.Errorf("kubeconfig backend requires a non-empty kubeconfig path")
		}
		return kubeconfig.New(cfg.Kubeconfig), nil
	default:
		return nil, fmt.Errorf("unknown cluster backend %q: valid values are envtest, kind, k3d, kubeconfig", backendType)
	}
}

// clusterSessionName returns a unique, DNS-label-safe cluster name suffixed
// with an 8-char random hex to keep parallel runs from clashing on shared
// docker daemons.
func clusterSessionName(prefix string) string {
	sessionID := uuid.New().String()[:8]
	return fmt.Sprintf("%s-%s", prefix, sessionID)
}
