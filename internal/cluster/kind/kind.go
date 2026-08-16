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

	"github.com/fregateops/vigie/internal/clog"
	"github.com/fregateops/vigie/internal/doctor"
)

const (
	defaultNodeImage = "kindest/node"
	waitTimeout      = 2 * time.Minute
)

// errNotStarted is returned by operations that require a running cluster when
// Start has not completed successfully.
var errNotStarted = errors.New("kind cluster not started")

// Backend implements the cluster Backend interface by driving the external
// kind CLI. It provisions a throwaway kind cluster and hands back its
// kubeconfig / REST credentials.
type Backend struct {
	mu          sync.Mutex
	clusterName string
	kubeVersion string
	extraArgs   []string
	opts        doctor.ResolveOptions
	kindPath    string
	kubecfgPath string
	restConfig  *rest.Config
	started     bool
}

// New returns an unstarted kind Backend.
// clusterName is used as-is; pass a unique name (e.g. derived from a session ID)
// to avoid collisions between parallel runs.
func New(clusterName, kubeVersion string, extraArgs []string, opts doctor.ResolveOptions) *Backend {
	return &Backend{
		clusterName: clusterName,
		kubeVersion: kubeVersion,
		extraArgs:   extraArgs,
		opts:        opts,
	}
}

// Start resolves the kind CLI, creates a cluster, and blocks until the API
// server is ready.
func (b *Backend) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.started {
		return fmt.Errorf("kind cluster %q already started", b.clusterName)
	}

	resolved, err := doctor.ResolveKind(ctx, b.opts)
	if err != nil {
		return err
	}
	b.kindPath = resolved.Path
	clog.Progress("kind: using %s (%s)", resolved.Path, resolved.Source)

	dataDir, err := os.MkdirTemp("", "vigie-kind-*")
	if err != nil {
		return fmt.Errorf("creating kind kubeconfig dir: %w", err)
	}
	b.kubecfgPath = filepath.Join(dataDir, "kubeconfig.yaml")
	clog.Progress("kind: dataDir=%s kubeconfig=%s", dataDir, b.kubecfgPath)

	args := []string{"create", "cluster", "--name", b.clusterName, "--kubeconfig", b.kubecfgPath, "--wait", waitTimeout.String()}
	nodeImage := nodeImageForVersion(b.kubeVersion)
	if nodeImage != "" {
		args = append(args, "--image", nodeImage)
	}
	args = append(args, b.extraArgs...)

	clog.Progress("kind: creating cluster %q nodeImage=%q (timeout %s)", b.clusterName, nodeImage, waitTimeout)
	if out, createErr := b.runKind(ctx, args...); createErr != nil {
		_ = os.RemoveAll(dataDir)
		return fmt.Errorf("creating kind cluster %q: %w\n%s", b.clusterName, createErr, out)
	}
	b.started = true
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
	if !b.started {
		return nil
	}
	clog.Progress("kind: deleting cluster %q", b.clusterName)
	if out, delErr := b.runKind(ctx, "delete", "cluster", "--name", b.clusterName, "--kubeconfig", b.kubecfgPath); delErr != nil {
		// Treat "cluster not found" as success - idempotent stop.
		if !strings.Contains(strings.ToLower(string(out)), "not found") {
			return fmt.Errorf("deleting kind cluster %q: %w\n%s", b.clusterName, delErr, out)
		}
	}
	if b.kubecfgPath != "" {
		_ = os.RemoveAll(filepath.Dir(b.kubecfgPath))
	}
	b.started = false
	b.kubecfgPath = ""
	b.restConfig = nil
	return nil
}

// RESTConfig returns the REST config after a successful Start.
func (b *Backend) RESTConfig() *rest.Config { return b.restConfig }

// Kubeconfig returns the path to a kubeconfig file for the running cluster.
func (b *Backend) Kubeconfig() string { return b.kubecfgPath }

// LoadImages loads container images from the host runtime into the cluster
// nodes via `kind load docker-image`.
func (b *Backend) LoadImages(ctx context.Context, images []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.started {
		return errNotStarted
	}
	if len(images) == 0 {
		return nil
	}

	args := append([]string{"load", "docker-image"}, images...)
	args = append(args, "--name", b.clusterName)
	if out, loadErr := b.runKind(ctx, args...); loadErr != nil {
		return fmt.Errorf("loading images %v into kind cluster %q: %w\n%s", images, b.clusterName, loadErr, out)
	}
	return nil
}

// runKind executes the resolved kind binary, selecting the podman provider when
// docker is absent but podman is present (kind defaults to docker otherwise).
func (b *Backend) runKind(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, b.kindPath, args...)
	cmd.Env = append(os.Environ(), providerEnv()...)
	return cmd.CombinedOutput()
}

// providerEnv returns the environment overrides needed to make kind use podman
// when docker is unavailable. kind defaults to docker and only auto-detects
// podman via KIND_EXPERIMENTAL_PROVIDER, so we set it explicitly. Empty when
// docker is present or when neither runtime is found (kind then reports the
// error itself). Runtime detection lives in doctor to avoid duplication.
func providerEnv() []string {
	if doctor.ContainerRuntime() == "podman" {
		return []string{"KIND_EXPERIMENTAL_PROVIDER=podman"}
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
