package doctor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultKindVersion is the kind release vigie downloads when it has to fetch
// a copy. Update this constant to offer a newer default.
const DefaultKindVersion = "v0.32.0"

// MinKindVersion is the lowest kind CLI version whose subcommand/flag contract
// vigie relies on (create/delete cluster, load image-archive). Binaries below
// this are rejected by the resolver.
const MinKindVersion = "v0.20.0"

// ErrKindPlatformUnsupported is returned when the current OS/arch has no
// kind release artifact.
var ErrKindPlatformUnsupported = errors.New("kind binary not published for this OS/arch")

// kindDownloadBase is the GitHub releases URL prefix for kind artifacts.
// Exposed as a package variable so tests can redirect downloads.
var kindDownloadBase = "https://github.com/kubernetes-sigs/kind/releases/download"

// kindSpec builds the ToolSpec that resolves the kind CLI. userPath is the
// explicit --kind-binary value ("" when unset).
func kindSpec(userPath string) ToolSpec {
	return ToolSpec{
		Name:         "kind",
		UserPath:     userPath,
		MinVersion:   MinKindVersion,
		CacheVersion: DefaultKindVersion,
		VersionArgs:  []string{"version"},
		ParseVersion: parseKindVersion,
		CachePath:    kindBinaryPath,
		Download:     downloadKind,
		InstallHint:  "install kind from https://kind.sigs.k8s.io/docs/user/quick-start/#installation",
	}
}

// ResolveKind locates a usable kind CLI (user path, PATH, cache, or download).
func ResolveKind(ctx context.Context, userPath string, policy DownloadPolicy, confirm func(prompt string) bool, progress io.Writer) (ResolvedTool, error) {
	return ResolveTool(ctx, kindSpec(userPath), policy, confirm, progress)
}

// CheckKind reports kind availability for `vigie doctor` without downloading.
func CheckKind(version string) Check {
	spec := kindSpec("")
	if version != "" {
		spec.CacheVersion = version
	}
	return CheckTool(spec)
}

// parseKindVersion extracts the version tag from `kind version` output.
// Expected format: "kind v0.32.0 go1.24.3 linux/amd64".
func parseKindVersion(output []byte) string {
	fields := strings.Fields(string(output))
	if len(fields) >= 2 && fields[0] == "kind" {
		return fields[1]
	}
	return ""
}

// downloadKind fetches the kind binary from GitHub releases, verifying the
// sha256 checksum from the published <binary>.sha256sum file.
// The sha256sum file contains a single line: "<sha256>  <filename>".
func downloadKind(ctx context.Context, version string, progress io.Writer) (string, error) {
	asset, err := kindAssetName()
	if err != nil {
		return "", err
	}
	binaryPath, err := kindBinaryPath(version)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		return "", fmt.Errorf("creating kind cache dir: %w", err)
	}

	binaryURL := fmt.Sprintf("%s/%s/%s", kindDownloadBase, version, asset)
	checksumURL := binaryURL + ".sha256sum"

	expectedHash, err := fetchKindChecksum(ctx, checksumURL)
	if err != nil {
		return "", fmt.Errorf("fetching kind checksum: %w", err)
	}

	verify := func(partialPath string) error {
		return verifySHA256(partialPath, expectedHash)
	}
	if err := downloadExecutable(ctx, binaryURL, binaryPath, verify, progress); err != nil {
		return "", fmt.Errorf("downloading kind from %s: %w", binaryURL, err)
	}
	return binaryPath, nil
}

// fetchKindChecksum fetches the kind sha256sum file and returns the hex hash.
// The file contains a single line: "<sha256>  <filename>".
func fetchKindChecksum(ctx context.Context, checksumURL string) (string, error) {
	body, err := fetchSHA256(ctx, checksumURL)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return "", fmt.Errorf("unexpected empty checksum response from %s", checksumURL)
	}
	return fields[0], nil
}

// kindAssetName returns the GitHub release asset name for the current platform.
func kindAssetName() (string, error) {
	switch runtime.GOOS {
	case "linux", "darwin":
	default:
		return "", fmt.Errorf("%w: GOOS=%s", ErrKindPlatformUnsupported, runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("%w: GOARCH=%s", ErrKindPlatformUnsupported, runtime.GOARCH)
	}
	return fmt.Sprintf("kind-%s-%s", runtime.GOOS, runtime.GOARCH), nil
}

// kindBinaryPath returns the cache path for the kind binary at the given version.
func kindBinaryPath(version string) (string, error) {
	asset, err := kindAssetName()
	if err != nil {
		return "", err
	}
	return githubBinaryPath("kind", version, asset)
}
