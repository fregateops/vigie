package doctor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultK3dVersion is the k3d release vigie downloads when it has to fetch a
// copy. Update this constant to offer a newer default.
const DefaultK3dVersion = "v5.9.0"

// MinK3dVersion is the lowest k3d CLI version whose subcommand/flag contract
// vigie relies on (cluster create/delete, kubeconfig get, image import, the v5
// --k3s-arg node-filter syntax). Binaries below this are rejected.
const MinK3dVersion = "v5.4.0"

// ErrK3dPlatformUnsupported is returned when the current OS/arch has no
// k3d release artifact.
var ErrK3dPlatformUnsupported = errors.New("k3d binary not published for this OS/arch")

// k3dDownloadBase is the GitHub releases URL prefix for k3d artifacts.
// Exposed as a package variable so tests can redirect downloads to an
// in-process httptest.Server.
var k3dDownloadBase = "https://github.com/k3d-io/k3d/releases/download"

// k3dSpec builds the ToolSpec that resolves the k3d CLI. userPath is the
// explicit --k3d-binary value ("" when unset).
func k3dSpec(userPath string) ToolSpec {
	return ToolSpec{
		Name:         "k3d",
		UserPath:     userPath,
		MinVersion:   MinK3dVersion,
		CacheVersion: DefaultK3dVersion,
		VersionArgs:  []string{"version"},
		ParseVersion: parseK3dVersion,
		CachePath:    k3dBinaryPath,
		Download:     downloadK3d,
		InstallHint:  "install k3d from https://k3d.io",
	}
}

// ResolveK3d locates a usable k3d CLI (user path, PATH, cache, or download).
func ResolveK3d(ctx context.Context, opts ResolveOptions) (ResolvedTool, error) {
	return ResolveTool(ctx, k3dSpec(opts.Binary), opts.Policy, opts.Confirm, opts.Progress)
}

// CheckK3d reports k3d availability for `vigie doctor` without downloading.
func CheckK3d(version string) Check {
	spec := k3dSpec("")
	if version != "" {
		spec.CacheVersion = version
	}
	return CheckTool(spec)
}

// parseK3dVersion extracts the version tag from `k3d version` output.
// Expected first line: "k3d version v5.9.0".
func parseK3dVersion(output []byte) string {
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "k3d" && fields[1] == "version" {
			return fields[2]
		}
	}
	return ""
}

// downloadK3d fetches the k3d binary from GitHub releases, verifying the
// checksum from the published checksums.txt file.
func downloadK3d(ctx context.Context, version string, progress io.Writer) (string, error) {
	asset, err := k3dAssetName()
	if err != nil {
		return "", err
	}
	binaryPath, err := k3dBinaryPath(version)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		return "", fmt.Errorf("creating k3d cache dir: %w", err)
	}

	binaryURL := fmt.Sprintf("%s/%s/%s", k3dDownloadBase, version, asset)
	checksumURL := fmt.Sprintf("%s/%s/checksums.txt", k3dDownloadBase, version)

	expectedHash, err := fetchK3dChecksum(ctx, checksumURL, asset)
	if err != nil {
		return "", fmt.Errorf("fetching k3d checksum: %w", err)
	}

	verify := func(partialPath string) error {
		return verifySHA256(partialPath, expectedHash)
	}
	if err := downloadExecutable(ctx, binaryURL, binaryPath, verify, progress); err != nil {
		return "", fmt.Errorf("downloading k3d from %s: %w", binaryURL, err)
	}
	return binaryPath, nil
}

// fetchK3dChecksum fetches the k3d checksums.txt and extracts the sha256
// for the given asset. The file format is "<sha256>  _dist/<asset>" per line.
func fetchK3dChecksum(ctx context.Context, checksumURL, asset string) (string, error) {
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
		return "", fmt.Errorf("fetching checksums.txt: unexpected status %s", resp.Status)
	}

	// Each line: "<sha256>  _dist/<asset-name>".
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 64*1024))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		// The filename in checksums.txt has a "_dist/" prefix.
		if fields[1] == "_dist/"+asset || fields[1] == asset {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading checksums.txt: %w", err)
	}
	return "", fmt.Errorf("no checksum found for %s in checksums.txt", asset)
}

// k3dAssetName returns the GitHub release asset name for the current platform.
func k3dAssetName() (string, error) {
	switch runtime.GOOS {
	case "linux", "darwin":
	default:
		return "", fmt.Errorf("%w: GOOS=%s", ErrK3dPlatformUnsupported, runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("%w: GOARCH=%s", ErrK3dPlatformUnsupported, runtime.GOARCH)
	}
	return fmt.Sprintf("k3d-%s-%s", runtime.GOOS, runtime.GOARCH), nil
}

// k3dBinaryPath returns the cache path for the k3d binary at the given version.
func k3dBinaryPath(version string) (string, error) {
	asset, err := k3dAssetName()
	if err != nil {
		return "", err
	}
	return githubBinaryPath("k3d", version, asset)
}
