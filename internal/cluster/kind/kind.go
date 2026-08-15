package kind

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/nodeutils"

	"github.com/fregateops/vigie/internal/clog"
)

const (
	defaultNodeImage = "kindest/node"
	waitTimeout      = 2 * time.Minute
)

// errNotStarted is returned by operations that require a running cluster when
// Start has not completed successfully.
var errNotStarted = errors.New("kind cluster not started")

// Backend implements the e2e Backend interface using kind as a Go library.
type Backend struct {
	mu               sync.Mutex
	clusterName      string
	kubeVersion      string
	extraArgs        []string
	kubecfgPath      string
	restConfig       *rest.Config
	provider         *cluster.Provider
	containerRuntime string
}

// New returns an unstarted kind Backend.
// clusterName is used as-is; pass a unique name (e.g. derived from a session ID)
// to avoid collisions between parallel runs.
func New(clusterName, kubeVersion string, extraArgs []string) *Backend {
	return &Backend{
		clusterName: clusterName,
		kubeVersion: kubeVersion,
		extraArgs:   extraArgs,
	}
}

// Start creates a kind cluster and blocks until the API server is ready.
func (b *Backend) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.provider != nil {
		return fmt.Errorf("kind cluster %q already started", b.clusterName)
	}

	providerOpt, runtime, err := detectProvider()
	if err != nil {
		return err
	}
	b.containerRuntime = runtime
	b.provider = cluster.NewProvider(providerOpt)
	clog.Progress("kind: container runtime=%s cluster=%q", runtime, b.clusterName)

	dataDir, err := os.MkdirTemp("", "vigie-kind-*")
	if err != nil {
		return fmt.Errorf("creating kind kubeconfig dir: %w", err)
	}
	b.kubecfgPath = filepath.Join(dataDir, "kubeconfig.yaml")
	clog.Progress("kind: dataDir=%s kubeconfig=%s", dataDir, b.kubecfgPath)

	createOpts := []cluster.CreateOption{
		cluster.CreateWithKubeconfigPath(b.kubecfgPath),
		cluster.CreateWithWaitForReady(waitTimeout),
	}
	nodeImage := nodeImageForVersion(b.kubeVersion)
	if nodeImage != "" {
		createOpts = append(createOpts, cluster.CreateWithNodeImage(nodeImage))
	}
	if rawCfg, cfgErr := kindConfigFromArgs(b.extraArgs); cfgErr != nil {
		_ = os.RemoveAll(dataDir)
		return cfgErr
	} else if len(rawCfg) > 0 {
		createOpts = append(createOpts, cluster.CreateWithRawConfig(rawCfg))
	}

	clog.Progress("kind: creating cluster %q nodeImage=%q (timeout %s)", b.clusterName, nodeImage, waitTimeout)
	if err := b.provider.Create(b.clusterName, createOpts...); err != nil {
		_ = os.RemoveAll(dataDir)
		return fmt.Errorf("creating kind cluster %q: %w", b.clusterName, err)
	}
	clog.Progress("kind: cluster %q ready", b.clusterName)

	restCfg, err := clientcmd.BuildConfigFromFlags("", b.kubecfgPath)
	if err != nil {
		_ = b.stopLocked(context.Background())
		return fmt.Errorf("building REST config from kind kubeconfig: %w", err)
	}
	b.restConfig = restCfg
	return nil
}

// Stop deletes the kind cluster. Safe to call multiple times.
func (b *Backend) Stop(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stopLocked(ctx)
}

func (b *Backend) stopLocked(ctx context.Context) error {
	if b.provider == nil {
		return nil
	}
	clog.Progress("kind: deleting cluster %q", b.clusterName)
	if err := b.provider.Delete(b.clusterName, b.kubecfgPath); err != nil {
		// Treat "cluster not found" as success - idempotent stop.
		if !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("deleting kind cluster %q: %w", b.clusterName, err)
		}
	}
	if b.kubecfgPath != "" {
		_ = os.RemoveAll(filepath.Dir(b.kubecfgPath))
	}
	b.provider = nil
	b.kubecfgPath = ""
	b.restConfig = nil
	return nil
}

// RESTConfig returns the REST config after a successful Start.
func (b *Backend) RESTConfig() *rest.Config { return b.restConfig }

// Kubeconfig returns the path to a kubeconfig file for the running cluster.
func (b *Backend) Kubeconfig() string { return b.kubecfgPath }

// LoadImages loads container images from the host container runtime into all cluster nodes.
func (b *Backend) LoadImages(ctx context.Context, images []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.provider == nil {
		return errNotStarted
	}

	runtime := b.containerRuntime
	if runtime == "" {
		runtime = "docker"
	}

	clusterNodes, err := b.provider.ListNodes(b.clusterName)
	if err != nil {
		return fmt.Errorf("listing kind cluster nodes: %w", err)
	}

	imageTar, err := os.CreateTemp("", "vigie-kind-images-*.tar")
	if err != nil {
		return fmt.Errorf("creating temp file for image archive: %w", err)
	}
	imageTarPath := imageTar.Name()
	if closeErr := imageTar.Close(); closeErr != nil {
		return fmt.Errorf("closing temp image archive: %w", closeErr)
	}
	defer func() { _ = os.Remove(imageTarPath) }()

	saveArgs := append([]string{"save", "-o", imageTarPath}, images...)
	saveCmd := exec.CommandContext(ctx, runtime, saveArgs...)
	if out, saveErr := saveCmd.CombinedOutput(); saveErr != nil {
		return fmt.Errorf("saving %s images %v: %w\n%s", runtime, images, saveErr, out)
	}

	for _, node := range clusterNodes {
		archiveFile, openErr := os.Open(imageTarPath)
		if openErr != nil {
			return fmt.Errorf("opening image archive: %w", openErr)
		}
		loadErr := nodeutils.LoadImageArchive(node, archiveFile)
		_ = archiveFile.Close()
		if loadErr != nil {
			return fmt.Errorf("loading image archive into node %s: %w", node, loadErr)
		}
	}

	return nil
}

// nodeImageForVersion maps a kubeVersion string to a kindest/node image tag.
// Returns empty string when kubeVersion is empty (let kind pick its default).
func nodeImageForVersion(kubeVersion string) string {
	if kubeVersion == "" {
		return ""
	}
	ver := strings.TrimPrefix(kubeVersion, "v")
	// Ensure the version has a "v" prefix for the image tag.
	return fmt.Sprintf("%s:v%s", defaultNodeImage, ver)
}

// kindConfigFromArgs parses extraArgs for a --config <path> flag and returns
// its file contents for use with cluster.CreateWithRawConfig.
// Returns nil when no --config flag is present.
func kindConfigFromArgs(args []string) ([]byte, error) {
	for idx := 0; idx < len(args)-1; idx++ {
		if args[idx] == "--config" {
			return os.ReadFile(args[idx+1])
		}
	}
	return nil, nil
}

// detectProvider returns the appropriate ProviderOption and runtime name,
// returning a clear error if neither Docker nor Podman is available.
func detectProvider() (cluster.ProviderOption, string, error) {
	if _, err := exec.LookPath("docker"); err == nil {
		return cluster.ProviderWithDocker(), "docker", nil
	}
	if _, err := exec.LookPath("podman"); err == nil {
		return cluster.ProviderWithPodman(), "podman", nil
	}
	// Attempt auto-detection as a fallback.
	opt, err := cluster.DetectNodeProvider()
	if err != nil {
		return nil, "", fmt.Errorf("no container runtime found (docker or podman required for kind): %w", err)
	}
	return opt, "docker", nil
}
