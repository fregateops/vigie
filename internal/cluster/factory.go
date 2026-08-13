package cluster

import (
	"fmt"

	"github.com/fregateops/vigie/internal/cluster/envtest"
)

// DefaultType is the cluster backend used when Config.Type is empty.
// envtest is fast, dependency-free, and matches the `--cluster envtest`
// default - making it the friendliest landing spot for new users.
const DefaultType = "envtest"

// New returns a Backend matching cfg.Type. Empty Type defaults to envtest.
//
// Only the envtest backend is available in this milestone; the node-backed
// (kind, k3d, kubeconfig) and simulated backends arrive in later milestones
// and currently return an error so misconfiguration fails loudly.
func New(cfg Config) (Backend, error) {
	backendType := cfg.Type
	if backendType == "" {
		backendType = DefaultType
	}
	switch backendType {
	case "envtest":
		return envtest.New(cfg.KubeVersion), nil
	default:
		return nil, fmt.Errorf("unknown cluster backend %q: the only supported backend is envtest", backendType)
	}
}
