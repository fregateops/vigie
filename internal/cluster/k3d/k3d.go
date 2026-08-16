package k3d

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fregateops/vigie/internal/clog"
	"github.com/fregateops/vigie/internal/cluster/procutil"
	"github.com/fregateops/vigie/internal/doctor"
)

// errNotStarted is returned by operations that require a running cluster when
// Start has not completed successfully.
var errNotStarted = errors.New("k3d cluster not started")

// Backend implements the e2e Backend interface by shelling out to the k3d binary.
type Backend struct {
	mu          sync.Mutex
	clusterName string
	kubeVersion string
	extraArgs   []string
	opts        doctor.ResolveOptions
	k3dBinPath  string
	kubecfgPath string
	restConfig  *rest.Config
	started     bool
}

// New returns an unstarted k3d Backend.
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

// Start creates a k3d cluster and blocks until the API server is ready.
func (b *Backend) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	resolved, err := doctor.ResolveK3d(ctx, b.opts)
	if err != nil {
		return err
	}
	k3dPath := resolved.Path
	b.k3dBinPath = k3dPath
	clog.Progress("k3d: using %s (%s)", resolved.Path, resolved.Source)

	dataDir, err := os.MkdirTemp("", "vigie-k3d-*")
	if err != nil {
		return fmt.Errorf("creating k3d kubeconfig dir: %w", err)
	}
	b.kubecfgPath = filepath.Join(dataDir, "kubeconfig.yaml")
	clog.Progress("k3d: dataDir=%s kubeconfig=%s", dataDir, b.kubecfgPath)

	// Pre-allocate a free port on the host and ask k3d to bind the API
	// server there as `localhost:<port>`. Without this, k3d binds on
	// 0.0.0.0:<auto-port> and writes that into kubeconfig, which fails
	// "EOF" / "connection refused" on hosts where 0.0.0.0 doesn't route
	// to the docker-proxy until late (e.g. dual-stack IPv4/IPv6, certain
	// nix-shell setups). The `localhost:<port>` form mirrors the
	// kubeAPI.host/hostIP pattern in the working k3d config the user
	// already has on another project.
	apiPort, err := procutil.PickFreePort()
	if err != nil {
		_ = os.RemoveAll(dataDir)
		return fmt.Errorf("allocating k3d API port: %w", err)
	}

	imageTag := k3sImageForVersion(ctx, k3dPath, b.kubeVersion)
	// On hosts with device-mapper backed filesystems (LUKS, btrfs, dm-crypt),
	// kubelet inside the k3s container can't determine free space because the
	// underlying /dev/mapper/<name> isn't visible in the container's namespace,
	// which historically prevented ContainerManager from starting. Bind-mount
	// /dev/mapper through when it exists on the host. The eviction-args
	// fallback below remains as defense-in-depth for newer k3s versions that
	// downgrade the failure to a warning. See https://k3d.io/v5.8.3/faq/faq/.
	extraVolumes := devMapperVolumeIfPresent()
	args := buildCreateArgs(b.clusterName, imageTag, b.kubecfgPath, apiPort, extraVolumes, b.extraArgs)
	if len(extraVolumes) > 0 {
		clog.Progress("k3d: detected /dev/mapper on host; bind-mounting into k3s container")
	}
	clog.Progress("k3d: creating cluster %q image=%q apiPort=localhost:%d", b.clusterName, imageTag, apiPort)
	createCmd := exec.CommandContext(ctx, k3dPath, args...)
	if out, createErr := createCmd.CombinedOutput(); createErr != nil {
		_ = os.RemoveAll(dataDir)
		return fmt.Errorf("creating k3d cluster %q: %w\n%s", b.clusterName, createErr, out)
	}
	// Mark started right after `k3d cluster create` succeeds - docker
	// containers now exist and must be torn down on any subsequent failure
	// (writeKubeconfig, BuildConfigFromFlags, waitAPIStable). Before this,
	// `stopLocked` was a no-op when called from those failure paths because
	// `started` was still false, leaking the cluster.
	b.started = true

	if err := b.writeKubeconfig(ctx, k3dPath); err != nil {
		_ = b.stopLocked(context.Background())
		return err
	}

	restCfg, err := clientcmd.BuildConfigFromFlags("", b.kubecfgPath)
	if err != nil {
		_ = b.stopLocked(context.Background())
		return fmt.Errorf("building REST config from k3d kubeconfig: %w", err)
	}

	// `k3d cluster create --wait` returns once the server container is up,
	// but the API server inside it can still drop the first few connections
	// with "connection reset by peer" or "EOF" while etcd / kube-apiserver
	// settle. Block until a discovery ping succeeds N times in a row before
	// handing the REST config back, so the first caller (dep installer,
	// namespace create, helm install) doesn't race the cooldown.
	clog.Progress("k3d: cluster %q created; waiting for API server to stabilise (up to 90s)", b.clusterName)
	if err := waitAPIStable(ctx, restCfg); err != nil {
		_ = b.stopLocked(context.Background())
		return fmt.Errorf("k3d cluster %q API server did not stabilise: %w", b.clusterName, err)
	}
	clog.Progress("k3d: cluster %q ready", b.clusterName)

	b.restConfig = restCfg
	return nil
}

// waitAPIStable returns nil once the API server answers `ServerVersion`
// successfully twice in a row within the timeout. Two consecutive successes
// filter out the "API up, then immediate connection reset / EOF" transient
// that `k3d cluster create --wait` doesn't cover. 90s allows for cold image
// pulls and slow disk.
func waitAPIStable(ctx context.Context, restCfg *rest.Config) error {
	const (
		timeout      = 90 * time.Second
		pollInterval = 1 * time.Second
		successesReq = 2
	)
	// k3d's --wait flag returns once the server container reports healthy,
	// but TLS / etcd / apiserver inside still need a beat. A small initial
	// pause makes the first ping less likely to land mid-handshake (it would
	// be retried anyway, but the log gets noisier).
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("building kubernetes client: %w", err)
	}
	deadline := time.Now().Add(timeout)
	successes := 0
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := client.Discovery().ServerVersion(); err != nil {
			lastErr = err
			successes = 0
		} else {
			successes++
			if successes >= successesReq {
				return nil
			}
		}
		// Honour cancellation while waiting between pings rather than
		// sleeping blind through a cancelled context.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	if lastErr != nil {
		return fmt.Errorf("after %s, last error: %w", timeout, lastErr)
	}
	return fmt.Errorf("did not see %d consecutive successful pings within %s", successesReq, timeout)
}

// Stop deletes the k3d cluster. Safe to call multiple times.
func (b *Backend) Stop(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stopLocked(ctx)
}

func (b *Backend) stopLocked(ctx context.Context) error {
	if !b.started {
		return nil
	}
	clog.Progress("k3d: deleting cluster %q", b.clusterName)
	deleteCmd := exec.CommandContext(ctx, b.k3dBinPath, "cluster", "delete", b.clusterName)
	if out, deleteErr := deleteCmd.CombinedOutput(); deleteErr != nil {
		// k3d exits non-zero when the cluster does not exist; treat as success.
		if !clusterNotFoundError(out) {
			return fmt.Errorf("deleting k3d cluster %q: %w\n%s", b.clusterName, deleteErr, out)
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

// LoadImages loads container images into the k3d cluster using `k3d image import`.
func (b *Backend) LoadImages(ctx context.Context, images []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.started {
		return errNotStarted
	}

	args := append([]string{"image", "import", "--cluster", b.clusterName}, images...)
	importCmd := exec.CommandContext(ctx, b.k3dBinPath, args...)
	if out, importErr := importCmd.CombinedOutput(); importErr != nil {
		return fmt.Errorf("importing images into k3d cluster %q: %w\n%s", b.clusterName, importErr, out)
	}
	return nil
}

// buildCreateArgs constructs the argument list for `k3d cluster create`.
// imageTag is the fully-qualified rancher/k3s image tag (e.g. "rancher/k3s:v1.31.2-k3s1"),
// or empty to let k3d pick its default. apiPort is a pre-allocated free port
// on the host where the API server's bind socket lands.
//
// We keep the k3d-side LB (no --no-lb here) because it adds a small TCP-level
// buffer in front of the still-warming kube-apiserver, which together with
// waitAPIStable's "two consecutive pings" rule makes the first dep install
// reliable.
//
// --api-port localhost:<port> mirrors the kubeAPI.host="localhost" /
// hostIP="127.0.0.1" pattern in a working k3d simple-config: the kubeconfig
// gets `server: https://localhost:<port>` (resolves to 127.0.0.1) instead of
// the default `https://0.0.0.0:<port>`, which is unreachable on hosts where
// 0.0.0.0 doesn't route to the docker-proxy reliably.
//
// We pass kubelet eviction args (`--k3s-arg --kubelet-arg=eviction-hard=…@server:*`
// matching the working conduktor-public-charts/k3d/config.yaml) so kubelet
// doesn't refuse to start when it can't determine free-space on LUKS /
// device-mapper / overlay rootfs setups. Default thresholds (15%) tell
// kubelet to evict pods under disk pressure; lowering them to <1% means
// "never evict" which sidesteps the failed `stat /dev/mapper/crypted` ->
// `Failed to start ContainerManager` chain that EOFs the API server.
// Without these args, the cluster comes up healthy on a typical CI runner
// but is unusable on common dev laptops with encrypted root.
func buildCreateArgs(clusterName, imageTag, kubecfgPath string, apiPort int, extraVolumes, extraArgs []string) []string {
	args := []string{
		"cluster", "create", clusterName,
		"--api-port", fmt.Sprintf("localhost:%d", apiPort),
		"--wait",
		"--kubeconfig-update-default=false",
		"--kubeconfig-switch-context=false",
		"--k3s-arg", "--kubelet-arg=eviction-hard=imagefs.available<1%,nodefs.available<1%@server:*;agent:*",
		"--k3s-arg", "--kubelet-arg=eviction-minimum-reclaim=imagefs.available=1%,nodefs.available=1%@server:*;agent:*",
	}
	for _, v := range extraVolumes {
		args = append(args, "-v", v)
	}
	if imageTag != "" {
		args = append(args, "--image", imageTag)
	}
	args = append(args, extraArgs...)
	return args
}

// devMapperVolumeIfPresent returns a k3d -v argument that exposes the host's
// /dev/mapper into the k3s container, but only if /dev/mapper actually exists
// on the host. This is the cure for "Failed to get device for dir ...: could
// not find device with major:minor ..." that kubelet emits on LUKS / btrfs /
// dm-crypt rootfs (see https://k3d.io/v5.8.3/faq/faq/). On systems without
// device-mapper the slice is empty so no flag is passed.
func devMapperVolumeIfPresent() []string {
	if _, err := os.Stat("/dev/mapper"); err != nil {
		return nil
	}
	return []string{"/dev/mapper:/dev/mapper"}
}

// defaultK3sImage is used when no kubeVersion is requested. k3d 5.x falls
// back to a several-years-old k3s (v1.21.7 in 5.8.3) which fails on common
// dev-laptop setups (LUKS rootfs, cgroup-v2 quirks) that modern k3s handles
// fine. Pin a recent stable so the out-of-the-box experience works.
const defaultK3sImage = "rancher/k3s:v1.31.5-k3s1"

// k3sImageForVersion builds the rancher/k3s image tag for the given kubeVersion.
// If kubeVersion already includes a k3s suffix (e.g. "1.31.2-k3s1"), it is used
// as-is. Otherwise k3d version is queried to discover the default k3s suffix.
// An empty kubeVersion returns defaultK3sImage (not "") so we don't inherit
// k3d's stale bundled default.
func k3sImageForVersion(ctx context.Context, k3dPath, kubeVersion string) string {
	if kubeVersion == "" {
		return defaultK3sImage
	}
	ver := strings.TrimPrefix(kubeVersion, "v")
	if strings.Contains(ver, "-k3s") {
		return fmt.Sprintf("rancher/k3s:v%s", ver)
	}
	suffix := resolveK3sSuffix(ctx, k3dPath)
	return fmt.Sprintf("rancher/k3s:v%s%s", ver, suffix)
}

// resolveK3sSuffix runs `k3d version` to discover the bundled k3s release suffix
// (e.g. "-k3s1"). Falls back to "-k3s1" if the output cannot be parsed.
func resolveK3sSuffix(ctx context.Context, k3dPath string) string {
	out, err := exec.CommandContext(ctx, k3dPath, "version").Output()
	if err != nil {
		return "-k3s1"
	}
	// Output contains a line like: "k3s version v1.31.2-k3s1 (default)"
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "k3s version ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		tag := strings.TrimPrefix(parts[2], "v")
		if idx := strings.Index(tag, "-k3s"); idx != -1 {
			return tag[idx:]
		}
	}
	return "-k3s1"
}

// writeKubeconfig writes the kubeconfig for the cluster to b.kubecfgPath.
func (b *Backend) writeKubeconfig(ctx context.Context, k3dPath string) error {
	out, err := exec.CommandContext(ctx, k3dPath, "kubeconfig", "get", b.clusterName).Output()
	if err != nil {
		return fmt.Errorf("getting k3d kubeconfig for cluster %q: %w", b.clusterName, err)
	}
	if writeErr := os.WriteFile(b.kubecfgPath, bytes.TrimSpace(out), 0600); writeErr != nil {
		return fmt.Errorf("writing k3d kubeconfig to %s: %w", b.kubecfgPath, writeErr)
	}
	return nil
}

// clusterNotFoundError returns true when k3d output indicates the cluster was not found.
func clusterNotFoundError(output []byte) bool {
	lower := strings.ToLower(string(output))
	return strings.Contains(lower, "no nodes found for cluster") ||
		strings.Contains(lower, "not found") ||
		strings.Contains(lower, "does not exist")
}
