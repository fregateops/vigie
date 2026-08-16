package doctor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrUnsupportedPlatform is returned by EnsureKubernetesBinary when the
// current GOOS/GOARCH pair has no published Kubernetes release artifact.
var ErrUnsupportedPlatform = errors.New("unsupported platform for kubernetes release binary download")

// k8sBinaryDownloadBase is the published-release URL prefix
// (https://dl.k8s.io/release/v<version>/bin/<os>/<arch>/<name>). Exposed as
// a package variable so tests can point it at an in-process test server.
var k8sBinaryDownloadBase = "https://dl.k8s.io/release"

// httpClient is the http.Client used by EnsureKubernetesBinary. Exposed for
// tests so they can swap in a transport that talks to httptest.Server.
var httpClient = &http.Client{Timeout: 5 * time.Minute}

// EnsureKubernetesBinary resolves a Kubernetes component binary (e.g.
// kube-scheduler, kube-controller-manager) from the vigie cache, downloading
// it from the published Kubernetes release tarball when absent. Binaries are
// cached under $XDG_CACHE_HOME/vigie/k8s/<version>/<os>-<arch>/<name> so
// every vigie component shares the same on-disk artifact.
//
// The kubeVersion argument is the bare semver (e.g. "1.30" or "1.30.5"); the
// "v" prefix is added automatically. When only major.minor is provided the
// upstream redirector resolves it to the latest published patch.
// progress, when non-nil, receives MB/total-MB progress lines during download.
func EnsureKubernetesBinary(ctx context.Context, name, kubeVersion string, progress io.Writer) (string, error) {
	if name == "" {
		return "", fmt.Errorf("binary name is required")
	}
	if kubeVersion == "" {
		return "", fmt.Errorf("kube version is required")
	}
	if !isSupportedPlatform(runtime.GOOS, runtime.GOARCH) {
		return "", fmt.Errorf("%w: %s/%s — set %s or pre-install %s",
			ErrUnsupportedPlatform, runtime.GOOS, runtime.GOARCH, EnvDisableAutoDownload, name)
	}

	cacheDir, err := kubernetesBinaryCacheDir(kubeVersion)
	if err != nil {
		return "", fmt.Errorf("resolving kubernetes binary cache dir: %w", err)
	}
	target := filepath.Join(cacheDir, name)

	if isExecutable(target) {
		return target, nil
	}

	if os.Getenv(EnvDisableAutoDownload) != "" {
		return "", fmt.Errorf("%w: auto-download disabled via %s and %s not present at %s",
			ErrAutoDownloadUnsupported, EnvDisableAutoDownload, name, target)
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("creating cache dir %s: %w", cacheDir, err)
	}

	versionRef := kubeVersion
	if versionRef[0] != 'v' {
		versionRef = "v" + versionRef
	}
	downloadURL := fmt.Sprintf("%s/%s/bin/%s/%s/%s",
		k8sBinaryDownloadBase, versionRef, runtime.GOOS, runtime.GOARCH, name)

	// Fetch the published sha256 checksum before downloading so we can verify
	// integrity before accepting the binary into the cache.
	expectedHash, hashErr := fetchSHA256(ctx, downloadURL+".sha256")
	if hashErr != nil {
		return "", fmt.Errorf("fetching checksum for %s: %w", name, hashErr)
	}

	verify := func(partialPath string) error {
		return verifySHA256(partialPath, expectedHash)
	}
	if err := downloadExecutable(ctx, downloadURL, target, verify, progress); err != nil {
		return "", fmt.Errorf("downloading %s from %s: %w", name, downloadURL, err)
	}
	return target, nil
}

// downloadExecutable streams url to dst (atomically via a *.partial file) and
// marks the file executable. Non-2xx responses are surfaced as errors so the
// caller can fall through to the "binary not found" guidance instead of
// silently caching an HTML error page. verify, when non-nil, is called with
// the partial path before the atomic rename - return an error to abort.
// progress, when non-nil, receives "  X MB / Y MB (Z%)" lines during download.
func downloadExecutable(ctx context.Context, url, dst string, verify func(partialPath string) error, progress io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}

	partial := dst + ".partial"
	out, err := os.OpenFile(partial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}

	body := io.Reader(resp.Body)
	if progress != nil {
		body = &progressReader{r: resp.Body, total: resp.ContentLength, progress: progress}
	}
	if _, copyErr := io.Copy(out, body); copyErr != nil {
		_ = out.Close()
		_ = os.Remove(partial)
		return copyErr
	}
	if progress != nil {
		// Finish the progress line with a newline so subsequent output starts
		// on a fresh line.
		fmt.Fprintln(progress)
	}
	if closeErr := out.Close(); closeErr != nil {
		_ = os.Remove(partial)
		return closeErr
	}
	if err := os.Chmod(partial, 0o755); err != nil {
		_ = os.Remove(partial)
		return err
	}
	if verify != nil {
		if err := verify(partial); err != nil {
			_ = os.Remove(partial)
			return err
		}
	}
	if err := os.Rename(partial, dst); err != nil {
		_ = os.Remove(partial)
		return err
	}
	return nil
}

// progressReader wraps an io.Reader to emit periodic MB/total-MB progress
// lines to a writer. Progress is emitted at each whole-MB boundary using \r
// to overwrite the current line in a terminal.
type progressReader struct {
	r        io.Reader
	total    int64 // Content-Length; -1 if unknown
	written  int64
	progress io.Writer
	lastMB   int64
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.written += int64(n)
	currentMB := pr.written >> 20
	if currentMB != pr.lastMB {
		pr.lastMB = currentMB
		if pr.total > 0 {
			totalMB := pr.total >> 20
			pct := pr.written * 100 / pr.total
			fmt.Fprintf(pr.progress, "  %d / %d MB (%d%%)\r", currentMB, totalMB, pct)
		} else {
			fmt.Fprintf(pr.progress, "  %d MB\r", currentMB)
		}
	}
	return n, err
}

// fetchSHA256 downloads a sha256 checksum file and returns the trimmed hex string.
func fetchSHA256(ctx context.Context, checksumURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

// verifySHA256 hashes the file at path and compares it to expected (hex-encoded).
func verifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, expected)
	}
	return nil
}

// kubernetesBinaryCacheDir returns the vigie-managed cache directory for
// versioned Kubernetes component binaries.
func kubernetesBinaryCacheDir(kubeVersion string) (string, error) {
	base, err := userCacheDir()
	if err != nil {
		return "", err
	}
	platform := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	return filepath.Join(base, "vigie", "k8s", kubeVersion, platform), nil
}

// isSupportedPlatform reports whether the published Kubernetes release set
// includes binaries for the given os/arch pair. The list intentionally tracks
// the platforms the project supports - extend it when CI starts running on
// additional architectures.
func isSupportedPlatform(os, arch string) bool {
	switch os {
	case "linux":
		return arch == "amd64" || arch == "arm64"
	case "darwin":
		return arch == "amd64" || arch == "arm64"
	}
	return false
}

// githubBinaryCacheDir returns the vigie cache directory for a GitHub-released binary tool.
// subDir is the tool name (e.g. "k3d", "kind", "kwok").
func githubBinaryCacheDir(subDir string) (string, error) {
	base, err := userCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "vigie", subDir), nil
}

// githubBinaryPath returns the versioned cache path for a GitHub release binary.
func githubBinaryPath(subDir, version, assetName string) (string, error) {
	dir, err := githubBinaryCacheDir(subDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, version, assetName), nil
}
