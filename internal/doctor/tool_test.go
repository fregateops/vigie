package doctor

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeCachedBinary creates path's parent directory and writes a runnable
// fake binary there.
func writeCachedBinary(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	writeFakeBinary(t, path, 0o755)
}

// fakeSpec builds a ToolSpec over a temp cache whose download step writes a
// runnable fake binary. runCommand is expected to be stubbed to report a
// version for the resolved path.
func fakeSpec(t *testing.T, userPath string) ToolSpec {
	t.Helper()
	cacheRoot := t.TempDir()
	return ToolSpec{
		Name:         "faketool",
		UserPath:     userPath,
		MinVersion:   "v1.2.0",
		CacheVersion: "v1.5.0",
		VersionArgs:  []string{"version"},
		ParseVersion: func(out []byte) string { return strings.TrimSpace(string(out)) },
		CachePath:    func(version string) (string, error) { return filepath.Join(cacheRoot, version, "faketool"), nil },
		Download: func(_ context.Context, _ string, _ io.Writer) (string, error) {
			return "", nil // replaced per-test below
		},
		InstallHint: "install faketool from https://example.test",
	}
}

// versionRunner returns a runCommand stub that reports the given version for
// any binary path.
func versionRunner(version string) func(string, ...string) ([]byte, error) {
	return func(string, ...string) ([]byte, error) { return []byte(version), nil }
}

func TestResolveTool_UserPathUsable(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "faketool")
	writeFakeBinary(t, bin, 0o755)
	stubExec(t, func(string) (string, error) { return "", exec.ErrNotFound }, versionRunner("v1.5.0"))

	spec := fakeSpec(t, bin)
	got, err := ResolveTool(context.Background(), spec, DownloadNever, nil, nil)
	if err != nil {
		t.Fatalf("ResolveTool: %v", err)
	}
	if got.Path != bin || got.Version != "v1.5.0" {
		t.Fatalf("resolved = %+v, want path %q version v1.5.0", got, bin)
	}
	if !strings.Contains(got.Source, "--faketool-binary") {
		t.Errorf("Source %q should name the --faketool-binary origin", got.Source)
	}
}

func TestResolveTool_UserPathBelowMinVersion(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "faketool")
	writeFakeBinary(t, bin, 0o755)
	stubExec(t, func(string) (string, error) { return "", exec.ErrNotFound }, versionRunner("v1.0.0"))

	_, err := ResolveTool(context.Background(), fakeSpec(t, bin), DownloadNever, nil, nil)
	if err == nil {
		t.Fatal("expected error for below-minimum version")
	}
	if !strings.Contains(err.Error(), "older than the minimum") {
		t.Errorf("error %q should explain the version floor", err)
	}
}

func TestResolveTool_UnparseableVersionProceeds(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "faketool")
	writeFakeBinary(t, bin, 0o755)
	stubExec(t, func(string) (string, error) { return "", exec.ErrNotFound }, versionRunner("mystery build"))

	spec := fakeSpec(t, bin)
	spec.ParseVersion = func([]byte) string { return "" } // simulate unrecognised output
	got, err := ResolveTool(context.Background(), spec, DownloadNever, nil, nil)
	if err != nil {
		t.Fatalf("unparseable version should proceed, got error: %v", err)
	}
	if got.Path != bin || got.Version != "" {
		t.Fatalf("resolved = %+v, want path %q empty version", got, bin)
	}
}

func TestResolveTool_PathThenCache(t *testing.T) {
	t.Run("found on PATH", func(t *testing.T) {
		pathBin := filepath.Join(t.TempDir(), "faketool")
		writeFakeBinary(t, pathBin, 0o755)
		stubExec(t,
			func(name string) (string, error) {
				if name == "faketool" {
					return pathBin, nil
				}
				return "", exec.ErrNotFound
			},
			versionRunner("v1.5.0"),
		)

		got, err := ResolveTool(context.Background(), fakeSpec(t, ""), DownloadNever, nil, nil)
		if err != nil {
			t.Fatalf("ResolveTool: %v", err)
		}
		if got.Path != pathBin || got.Source != "PATH" {
			t.Fatalf("resolved = %+v, want PATH source at %q", got, pathBin)
		}
	})

	t.Run("falls back to cache", func(t *testing.T) {
		stubExec(t, func(string) (string, error) { return "", exec.ErrNotFound }, versionRunner("v1.5.0"))
		spec := fakeSpec(t, "")
		cached, _ := spec.CachePath(spec.CacheVersion)
		writeCachedBinary(t, cached)

		got, err := ResolveTool(context.Background(), spec, DownloadNever, nil, nil)
		if err != nil {
			t.Fatalf("ResolveTool: %v", err)
		}
		if got.Path != cached || got.Source != "vigie cache" {
			t.Fatalf("resolved = %+v, want cache source at %q", got, cached)
		}
	})
}

func TestResolveTool_NotFoundNeverDownloads(t *testing.T) {
	stubExec(t, func(string) (string, error) { return "", exec.ErrNotFound }, versionRunner("v1.5.0"))
	_, err := ResolveTool(context.Background(), fakeSpec(t, ""), DownloadNever, nil, nil)
	if err == nil {
		t.Fatal("expected not-found error")
	}
	for _, want := range []string{"not found", "install faketool", "--faketool-binary"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err, want)
		}
	}
}

func TestResolveTool_DownloadPolicies(t *testing.T) {
	newSpecWithDownload := func(t *testing.T) ToolSpec {
		stubExec(t, func(string) (string, error) { return "", exec.ErrNotFound }, versionRunner("v1.5.0"))
		spec := fakeSpec(t, "")
		spec.Download = func(_ context.Context, version string, _ io.Writer) (string, error) {
			dst, _ := spec.CachePath(version)
			writeCachedBinary(t, dst)
			return dst, nil
		}
		return spec
	}

	t.Run("Always downloads", func(t *testing.T) {
		spec := newSpecWithDownload(t)
		got, err := ResolveTool(context.Background(), spec, DownloadAlways, nil, nil)
		if err != nil {
			t.Fatalf("ResolveTool: %v", err)
		}
		if !strings.HasPrefix(got.Source, "downloaded faketool") {
			t.Errorf("Source %q should indicate a download", got.Source)
		}
	})

	t.Run("Interactive declined does not download", func(t *testing.T) {
		spec := newSpecWithDownload(t)
		_, err := ResolveTool(context.Background(), spec, DownloadInteractive, func(string) bool { return false }, nil)
		if err == nil {
			t.Fatal("expected not-found error when the prompt is declined")
		}
	})

	t.Run("Interactive accepted downloads", func(t *testing.T) {
		spec := newSpecWithDownload(t)
		got, err := ResolveTool(context.Background(), spec, DownloadInteractive, func(string) bool { return true }, nil)
		if err != nil {
			t.Fatalf("ResolveTool: %v", err)
		}
		if !strings.HasPrefix(got.Source, "downloaded faketool") {
			t.Errorf("Source %q should indicate a download", got.Source)
		}
	})

	t.Run("Interactive with nil confirm does not download", func(t *testing.T) {
		spec := newSpecWithDownload(t)
		_, err := ResolveTool(context.Background(), spec, DownloadInteractive, nil, nil)
		if err == nil {
			t.Fatal("expected not-found error when confirm is nil")
		}
	})
}

func TestCheckTool(t *testing.T) {
	t.Run("ok when resolvable", func(t *testing.T) {
		pathBin := filepath.Join(t.TempDir(), "faketool")
		writeFakeBinary(t, pathBin, 0o755)
		stubExec(t,
			func(name string) (string, error) {
				if name == "faketool" {
					return pathBin, nil
				}
				return "", exec.ErrNotFound
			},
			versionRunner("v1.5.0"),
		)
		c := CheckTool(fakeSpec(t, ""))
		if c.Status != StatusOK {
			t.Fatalf("Status = %q, want ok (%q)", c.Status, c.Detail)
		}
	})

	t.Run("warning when absent", func(t *testing.T) {
		stubExec(t, func(string) (string, error) { return "", exec.ErrNotFound }, versionRunner("v1.5.0"))
		c := CheckTool(fakeSpec(t, ""))
		if c.Status != StatusWarning {
			t.Fatalf("Status = %q, want warning", c.Status)
		}
	})
}
