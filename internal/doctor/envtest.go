package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvtestBinaries describes the resolved kube-apiserver + etcd binaries.
type EnvtestBinaries struct {
	APIServerPath string
	EtcdPath      string
	// Source is a human-readable label describing where the binaries
	// came from, e.g. "KUBEBUILDER_ASSETS" or "setup-envtest (k8s 1.30)".
	Source string
}

// LocateEnvtestBinaries resolves envtest binaries by checking
// KUBEBUILDER_ASSETS first, then falling back to setup-envtest. Returns
// nil binaries if neither path resolves to a valid pair.
//
// The returned []Check trace the discovery process: each step (env var,
// setup-envtest lookup) appends a Check so the doctor command can show
// the user exactly which path was tried. Callers that just want the
// resolved paths (e.g. the cluster envtest backend) can ignore the
// trace and check whether binaries is non-nil.
func LocateEnvtestBinaries(kubeVersion string) (binaries *EnvtestBinaries, trace []Check) {
	// 1. KUBEBUILDER_ASSETS env var.
	if b, c := CheckKubebuilderAssets(); b != nil {
		// Successful KUBEBUILDER_ASSETS resolution emits no trace check
		// (matches the original CLI behavior, which only surfaced
		// problems with the env var, not successes).
		return b, trace
	} else if c != nil {
		trace = append(trace, *c)
	}

	// 2. setup-envtest fallback.
	b, c := CheckSetupEnvtest(kubeVersion)
	trace = append(trace, c)
	if b != nil {
		return b, trace
	}

	return nil, trace
}

// CheckKubebuilderAssets validates the directory referenced by the
// KUBEBUILDER_ASSETS env var. Returns nil binaries when the env var is
// unset or the directory is invalid.
//
// The returned *Check describes a problem with the env var (directory
// missing, executables missing). On success both binaries and check are
// returned (the caller decides whether to surface the success).
// When the env var is unset, both return values are nil.
func CheckKubebuilderAssets() (binaries *EnvtestBinaries, check *Check) {
	kubebuilderAssets := os.Getenv("KUBEBUILDER_ASSETS")
	if kubebuilderAssets == "" {
		return nil, nil
	}

	apiserverCandidate := filepath.Join(kubebuilderAssets, "kube-apiserver")
	etcdCandidate := filepath.Join(kubebuilderAssets, "etcd")
	dirOK := dirExists(kubebuilderAssets)
	apiserverOK := isExecutable(apiserverCandidate)
	etcdOK := isExecutable(etcdCandidate)

	if dirOK && apiserverOK && etcdOK {
		return &EnvtestBinaries{
			APIServerPath: apiserverCandidate,
			EtcdPath:      etcdCandidate,
			Source:        "KUBEBUILDER_ASSETS",
		}, nil
	}

	if !dirOK {
		return nil, &Check{
			Name:   "KUBEBUILDER_ASSETS",
			Status: StatusError,
			Detail: fmt.Sprintf("directory does not exist: %s", kubebuilderAssets),
		}
	}

	missing := []string{}
	if !apiserverOK {
		missing = append(missing, "kube-apiserver")
	}
	if !etcdOK {
		missing = append(missing, "etcd")
	}
	return nil, &Check{
		Name:   "KUBEBUILDER_ASSETS",
		Status: StatusError,
		Detail: fmt.Sprintf("directory exists but missing executables: %v in %s", missing, kubebuilderAssets),
	}
}

// CheckSetupEnvtest looks up `setup-envtest` on $PATH and runs
// `setup-envtest use <kubeVersion> -p path` to resolve the asset directory.
// Returns nil binaries when the binary is missing or the resolution fails.
func CheckSetupEnvtest(kubeVersion string) (binaries *EnvtestBinaries, check Check) {
	setupEnvtestPath, err := lookPath("setup-envtest")
	if err != nil {
		return nil, Check{
			Name:   "setup-envtest",
			Status: StatusWarning,
			Detail: "not found on $PATH; install with: go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest",
		}
	}

	out, runErr := runCommand(setupEnvtestPath, "use", kubeVersion, "-p", "path")
	if runErr != nil {
		return nil, Check{
			Name:   "setup-envtest",
			Status: StatusWarning,
			Detail: fmt.Sprintf("found at %s but 'use %s' failed: %v", setupEnvtestPath, kubeVersion, runErr),
		}
	}

	resolvedPath := strings.TrimSpace(string(out))
	apiserverCandidate := filepath.Join(resolvedPath, "kube-apiserver")
	etcdCandidate := filepath.Join(resolvedPath, "etcd")
	if !isExecutable(apiserverCandidate) || !isExecutable(etcdCandidate) {
		return nil, Check{
			Name:   "setup-envtest",
			Status: StatusWarning,
			Detail: fmt.Sprintf("found at %s but resolved path %s missing executables", setupEnvtestPath, resolvedPath),
		}
	}

	return &EnvtestBinaries{
			APIServerPath: apiserverCandidate,
			EtcdPath:      etcdCandidate,
			Source:        fmt.Sprintf("setup-envtest (k8s %s)", kubeVersion),
		}, Check{
			Name:   "setup-envtest",
			Status: StatusOK,
			Detail: fmt.Sprintf("found at %s, binaries resolved to %s", setupEnvtestPath, resolvedPath),
		}
}

// dirExists reports whether path is an existing directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isExecutable reports whether path is an existing, executable regular file.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}
