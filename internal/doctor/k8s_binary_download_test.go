package doctor

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// withUserCacheDir swaps the package-level userCacheDir resolver for the test
// duration so cache paths land under a temp dir instead of the real OS cache.
func withUserCacheDir(t *testing.T, fn func() (string, error)) {
	t.Helper()
	orig := userCacheDir
	t.Cleanup(func() { userCacheDir = orig })
	userCacheDir = fn
}

// withDownloadBase swaps the published-release base URL for the test
// duration so EnsureKubernetesBinary talks to an in-process httptest.Server.
func withDownloadBase(t *testing.T, base string) {
	t.Helper()
	orig := k8sBinaryDownloadBase
	t.Cleanup(func() { k8sBinaryDownloadBase = orig })
	k8sBinaryDownloadBase = base
}

func TestEnsureKubernetesBinary_CacheHit(t *testing.T) {
	if !isSupportedPlatform(runtime.GOOS, runtime.GOARCH) {
		t.Skipf("platform %s/%s is not in the supported list", runtime.GOOS, runtime.GOARCH)
	}
	cacheRoot := t.TempDir()
	withUserCacheDir(t, func() (string, error) { return cacheRoot, nil })

	// Pre-populate the expected on-disk location.
	versionedDir := filepath.Join(cacheRoot, "vigie", "k8s", "1.30", runtime.GOOS+"-"+runtime.GOARCH)
	if err := os.MkdirAll(versionedDir, 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	binPath := filepath.Join(versionedDir, "kube-scheduler")
	writeFakeBinary(t, binPath, 0o755)

	// Point download base at an unreachable URL; cache hit must avoid it.
	withDownloadBase(t, "http://127.0.0.1:1")

	got, err := EnsureKubernetesBinary(context.Background(), "kube-scheduler", "1.30", nil)
	if err != nil {
		t.Fatalf("EnsureKubernetesBinary: %v", err)
	}
	if got != binPath {
		t.Errorf("path = %q, want %q", got, binPath)
	}
}

func TestEnsureKubernetesBinary_DownloadSucceeds(t *testing.T) {
	if !isSupportedPlatform(runtime.GOOS, runtime.GOARCH) {
		t.Skipf("platform %s/%s is not in the supported list", runtime.GOOS, runtime.GOARCH)
	}

	const payload = "#!/bin/sh\necho ok\n"
	var requestedPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		requestedPaths = append(requestedPaths, req.URL.Path)
		if strings.HasSuffix(req.URL.Path, ".sha256") {
			// Return the sha256 of payload so checksum verification passes.
			h := sha256.New()
			h.Write([]byte(payload))
			_, _ = fmt.Fprintf(writer, "%x", h.Sum(nil))
			return
		}
		_, _ = writer.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)
	withDownloadBase(t, srv.URL)

	cacheRoot := t.TempDir()
	withUserCacheDir(t, func() (string, error) { return cacheRoot, nil })

	got, err := EnsureKubernetesBinary(context.Background(), "kube-scheduler", "1.30", nil)
	if err != nil {
		t.Fatalf("EnsureKubernetesBinary: %v", err)
	}
	expected := filepath.Join(cacheRoot, "vigie", "k8s", "1.30", runtime.GOOS+"-"+runtime.GOARCH, "kube-scheduler")
	if got != expected {
		t.Errorf("path = %q, want %q", got, expected)
	}
	if !isExecutable(got) {
		t.Errorf("downloaded file is not executable: %s", got)
	}
	body, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(body) != payload {
		t.Errorf("downloaded body = %q, want %q", body, payload)
	}
	// Expect 2 requests: one for the .sha256 file, one for the binary.
	if len(requestedPaths) != 2 {
		t.Fatalf("expected 2 requests (checksum + binary), got %d (%v)", len(requestedPaths), requestedPaths)
	}
	wantBinaryPath := "/v1.30/bin/" + runtime.GOOS + "/" + runtime.GOARCH + "/kube-scheduler"
	wantChecksumPath := wantBinaryPath + ".sha256"
	if requestedPaths[0] != wantChecksumPath {
		t.Errorf("first request = %q, want checksum path %q", requestedPaths[0], wantChecksumPath)
	}
	if requestedPaths[1] != wantBinaryPath {
		t.Errorf("second request = %q, want binary path %q", requestedPaths[1], wantBinaryPath)
	}
}

func TestEnsureKubernetesBinary_VersionPrefixRespected(t *testing.T) {
	if !isSupportedPlatform(runtime.GOOS, runtime.GOARCH) {
		t.Skipf("platform %s/%s is not in the supported list", runtime.GOOS, runtime.GOARCH)
	}

	const payload = "payload"
	var seenPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		seenPaths = append(seenPaths, req.URL.Path)
		if strings.HasSuffix(req.URL.Path, ".sha256") {
			h := sha256.New()
			h.Write([]byte(payload))
			_, _ = fmt.Fprintf(writer, "%x", h.Sum(nil))
			return
		}
		_, _ = writer.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)
	withDownloadBase(t, srv.URL)

	cacheRoot := t.TempDir()
	withUserCacheDir(t, func() (string, error) { return cacheRoot, nil })

	if _, err := EnsureKubernetesBinary(context.Background(), "kube-scheduler", "v1.30.5", nil); err != nil {
		t.Fatalf("EnsureKubernetesBinary: %v", err)
	}
	// Both the checksum and binary requests must target the versioned path.
	for _, seen := range seenPaths {
		if !strings.HasPrefix(seen, "/v1.30.5/") {
			t.Errorf("URL %q does not start with /v1.30.5/", seen)
		}
	}
}

func TestEnsureKubernetesBinary_AutoDownloadDisabled(t *testing.T) {
	if !isSupportedPlatform(runtime.GOOS, runtime.GOARCH) {
		t.Skipf("platform %s/%s is not in the supported list", runtime.GOOS, runtime.GOARCH)
	}
	t.Setenv(EnvDisableAutoDownload, "1")
	cacheRoot := t.TempDir()
	withUserCacheDir(t, func() (string, error) { return cacheRoot, nil })

	_, err := EnsureKubernetesBinary(context.Background(), "kube-scheduler", "1.30", nil)
	if err == nil {
		t.Fatal("expected error when auto-download is disabled and cache is empty")
	}
	if !errors.Is(err, ErrAutoDownloadUnsupported) {
		t.Errorf("error does not wrap ErrAutoDownloadUnsupported: %v", err)
	}
}

func TestEnsureKubernetesBinary_UnsupportedPlatform(t *testing.T) {
	if isSupportedPlatform("plan9", "riscv64") {
		t.Skip("plan9/riscv64 became supported — pick a different unsupported pair")
	}

	got := isSupportedPlatform("plan9", "riscv64")
	if got {
		t.Fatalf("isSupportedPlatform(plan9, riscv64) = %v, want false", got)
	}
}

func TestEnsureKubernetesBinary_NoName(t *testing.T) {
	if _, err := EnsureKubernetesBinary(context.Background(), "", "1.30", nil); err == nil {
		t.Fatal("expected error when name is empty")
	}
	if _, err := EnsureKubernetesBinary(context.Background(), "kube-scheduler", "", nil); err == nil {
		t.Fatal("expected error when version is empty")
	}
}

func TestEnsureKubernetesBinary_HTTPError(t *testing.T) {
	if !isSupportedPlatform(runtime.GOOS, runtime.GOARCH) {
		t.Skipf("platform %s/%s is not in the supported list", runtime.GOOS, runtime.GOARCH)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "not found", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	withDownloadBase(t, srv.URL)

	cacheRoot := t.TempDir()
	withUserCacheDir(t, func() (string, error) { return cacheRoot, nil })

	_, err := EnsureKubernetesBinary(context.Background(), "kube-scheduler", "1.30", nil)
	if err == nil {
		t.Fatal("expected error on HTTP 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error %q should mention 404", err)
	}
}

func TestEnsureKubernetesBinary_ChecksumMismatch(t *testing.T) {
	if !isSupportedPlatform(runtime.GOOS, runtime.GOARCH) {
		t.Skipf("platform %s/%s is not in the supported list", runtime.GOOS, runtime.GOARCH)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, ".sha256") {
			// Return a deliberately wrong checksum.
			_, _ = writer.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000"))
			return
		}
		_, _ = writer.Write([]byte("binary content"))
	}))
	t.Cleanup(srv.Close)
	withDownloadBase(t, srv.URL)

	cacheRoot := t.TempDir()
	withUserCacheDir(t, func() (string, error) { return cacheRoot, nil })

	_, err := EnsureKubernetesBinary(context.Background(), "kube-scheduler", "1.30", nil)
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "sha256") {
		t.Errorf("error %q should mention sha256", err)
	}
}
