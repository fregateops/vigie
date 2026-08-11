package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/afero"
	setupenv "sigs.k8s.io/controller-runtime/tools/setup-envtest/env"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/remote"
	setupstore "sigs.k8s.io/controller-runtime/tools/setup-envtest/store"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/workflows"
)

// ErrAutoDownloadUnsupported is returned by EnsureEnvtestBinaries when the
// auto-download fallback is disabled via VIGIE_DISABLE_AUTO_DOWNLOAD.
var ErrAutoDownloadUnsupported = errors.New("envtest auto-download not supported in this environment")

// userCacheDir is a swappable indirection for tests so we don't need to
// fiddle with XDG_CACHE_HOME globally on every run.
var userCacheDir = os.UserCacheDir

// EnvDisableAutoDownload, when set to a non-empty value, instructs
// EnsureEnvtestBinaries to skip the auto-download fallback and behave
// like LocateEnvtestBinaries (return only KUBEBUILDER_ASSETS or
// setup-envtest results). Useful for offline CI runs and for tests that
// want to assert the "binaries not found" error path without hitting the
// network.
const EnvDisableAutoDownload = "VIGIE_DISABLE_AUTO_DOWNLOAD"

// envtestDownloadTimeout is the maximum time allowed for the envtest binary
// download. The setup-envtest library does not respect context cancellation
// (it uses context.TODO() internally), so we implement a hard timeout via
// goroutine + select. The archive is typically 100-200 MB.
const envtestDownloadTimeout = 10 * time.Minute

// EnsureEnvtestBinaries resolves envtest binaries (kube-apiserver + etcd),
// downloading them to a vigie-managed cache when neither
// KUBEBUILDER_ASSETS nor a `setup-envtest` binary on $PATH produces a
// working pair. Cache location: $XDG_CACHE_HOME/vigie/envtest/
// (defaults to ~/.cache/vigie/envtest/).
//
// When progress is non-nil, download status ticks are written every few
// seconds so callers know the download is in progress and haven't stalled.
//
// LocateEnvtestBinaries (used by `vigie doctor`) keeps its existing
// "check only, no side effects" behavior - this function is the
// download-aware variant for runtime callers (e.g. cluster backends).
func EnsureEnvtestBinaries(ctx context.Context, kubeVersion string, progress io.Writer) (*EnvtestBinaries, error) {
	// 1. Try the existing locate path (env var or setup-envtest CLI).
	if b, _ := LocateEnvtestBinaries(kubeVersion); b != nil {
		return b, nil
	}

	// 2. Honor the kill-switch env var: skip auto-download entirely.
	if os.Getenv(EnvDisableAutoDownload) != "" {
		return nil, fmt.Errorf("%w: auto-download disabled via %s",
			ErrAutoDownloadUnsupported, EnvDisableAutoDownload)
	}

	// 3. Resolve cache dir.
	cacheDir, err := envtestCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolving envtest cache dir: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating envtest cache dir %s: %w", cacheDir, err)
	}

	// 4. Drive the setup-envtest library Use workflow.
	// Use a minor-wildcard expression (e.g. "1.36.x") so setup-envtest resolves
	// the latest available patch - the envtest binary index lags behind
	// dl.k8s.io and may not yet publish every patch (e.g. envtest has 1.36.0
	// while dl.k8s.io already carries 1.36.1).
	spec, err := versions.FromExpr(envtestVersionExpr(kubeVersion))
	if err != nil {
		return nil, fmt.Errorf("parsing kube version %q: %w", kubeVersion, err)
	}

	var out bytes.Buffer
	env := &setupenv.Env{
		Log:           logr.Discard(),
		Out:           &out,
		Client:        &remote.HTTPClient{Log: logr.Discard(), IndexURL: remote.DefaultIndexURL},
		Store:         setupstore.NewAt(cacheDir),
		Version:       spec,
		Platform:      versions.PlatformItem{Platform: versions.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}},
		FS:            afero.Afero{Fs: afero.NewOsFs()},
		VerifySum:     true,
		ForceDownload: false,
		NoDownload:    false,
	}

	// The setup-envtest workflow uses panic+recover for exit codes, and uses
	// context.TODO() internally - it cannot be cancelled via the caller's
	// context. Run it in a goroutine so we can emit progress ticks and enforce
	// a hard timeout.
	workflowErr := runUseWorkflowWithProgress(env, progress)
	if workflowErr != nil {
		return nil, fmt.Errorf("downloading envtest binaries to %s: %w (run 'vigie doctor' for details)", cacheDir, workflowErr)
	}

	// setup-envtest intentionally strips write bits from extracted files and
	// directories (0555). Add u+w to every directory so the user can delete
	// the cache with rm -rf without needing root.
	_ = restoreWritableDirs(cacheDir)

	// 5. Resolve the asset path written to env.Out by PrintFormat=PrintPath.
	assetPath := strings.TrimSpace(out.String())
	if assetPath == "" {
		return nil, fmt.Errorf("setup-envtest workflow completed but did not report an asset path")
	}
	apiserver := filepath.Join(assetPath, "kube-apiserver")
	etcdBin := filepath.Join(assetPath, "etcd")
	if !isExecutable(apiserver) || !isExecutable(etcdBin) {
		return nil, fmt.Errorf("setup-envtest installed binaries to %s but kube-apiserver/etcd not executable", assetPath)
	}
	return &EnvtestBinaries{
		APIServerPath: apiserver,
		EtcdPath:      etcdBin,
		Source:        fmt.Sprintf("auto-download to %s (k8s %s)", cacheDir, kubeVersion),
	}, nil
}

// runUseWorkflowWithProgress runs the setup-envtest Use workflow in a goroutine
// and emits periodic progress ticks to progress (if non-nil) while waiting.
// Returns an error on workflow failure or timeout.
func runUseWorkflowWithProgress(env *setupenv.Env, progress io.Writer) error {
	done := make(chan error, 1)
	go func() { done <- runUseWorkflow(env) }()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timeout := time.NewTimer(envtestDownloadTimeout)
	defer timeout.Stop()

	start := time.Now()
	for {
		select {
		case err := <-done:
			// Emit a newline to terminate the \r progress line before returning.
			emitProgress(progress, "\n")
			return err
		case <-ticker.C:
			// Use \r to overwrite the current progress line in place, matching
			// the MB-progress style used by the other binary downloads.
			emitProgress(progress, "  downloading… %s elapsed\r", elapsed(start))
		case <-timeout.C:
			emitProgress(progress, "\n")
			return fmt.Errorf("timed out after %s — network may be slow or unreachable; set VIGIE_DISABLE_AUTO_DOWNLOAD=1 to skip", envtestDownloadTimeout)
		}
	}
}

// runUseWorkflow runs `workflows.Use{}.Do(env)` and converts the
// setup-envtest panic-on-exit-code idiom into a plain error.
func runUseWorkflow(env *setupenv.Env) (err error) {
	env.CheckCoherence()

	defer func() {
		cause := recover()
		if setupenv.CheckRecover(cause, func(code int, exitErr error) {
			if code != 0 {
				err = exitErr
			}
		}) {
			// Not an exit-code panic - re-raise.
			panic(cause)
		}
	}()

	(workflows.Use{
		PrintFormat: setupenv.PrintPath,
	}).Do(env)
	return nil
}

// envtestVersionExpr converts an exact version like "1.36.1" or "v1.36.1"
// into a minor-wildcard expression like "1.36.x" for the setup-envtest index.
// The envtest binary index publishes releases independently of dl.k8s.io and
// may not yet carry every patch level, so the wildcard finds the latest
// available patch for that minor version.
func envtestVersionExpr(kubeVersion string) string {
	v := strings.TrimPrefix(kubeVersion, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return v
	}
	return parts[0] + "." + parts[1] + ".x"
}

// envtestCacheDir returns the vigie-managed envtest cache directory,
// honoring XDG_CACHE_HOME on Linux.
func envtestCacheDir() (string, error) { return githubBinaryCacheDir("envtest") }

// githubBinaryCacheDir returns the vigie-managed cache directory for a named
// binary group (e.g. "envtest"), rooted at the OS user cache dir.
func githubBinaryCacheDir(subDir string) (string, error) {
	base, err := userCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "vigie", subDir), nil
}

// restoreWritableDirs walks root and adds user-write permission (u+w) to every
// directory. setup-envtest extracts archives with 0555 so directories end up
// non-writable, which prevents rm -rf from working without root. This is a
// post-extraction fixup - binary execution is unaffected by the change.
func restoreWritableDirs(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil // skip unreadable entries and files
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		return os.Chmod(path, info.Mode()|0o200)
	})
}
