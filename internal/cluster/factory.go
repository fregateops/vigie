package cluster

import (
	"fmt"

	"github.com/fregateops/vigie/internal/cluster/envtest"
	"github.com/fregateops/vigie/internal/cluster/kubeconfig"
)

// DefaultType is the cluster backend used when Config.Type is empty.
// envtest is fast, dependency-free, and matches the `--cluster envtest`
// default - making it the friendliest landing spot for new users.
const DefaultType = "envtest"

// New returns a Backend matching cfg.Type. Empty Type defaults to envtest.
//
// envtest (in-process apiserver) and kubeconfig (an external cluster the user
// already runs) are available; the node-backed (kind, k3d) and simulated
// backends arrive in later milestones and currently return an error so
// misconfiguration fails loudly.
func New(cfg Config) (Backend, error) {
	backendType := cfg.Type
	if backendType == "" {
		backendType = DefaultType
	}
	switch backendType {
	case "envtest":
		return envtest.New(cfg.KubeVersion), nil
	case "kubeconfig":
		if cfg.Kubeconfig == "" {
			return nil, fmt.Errorf("kubeconfig backend requires a non-empty kubeconfig path")
		}
		return kubeconfig.New(cfg.Kubeconfig), nil
	default:
		return nil, fmt.Errorf("unknown cluster backend %q: valid values are envtest, kubeconfig", backendType)
	}
}
