package kubeconfig_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/fregateops/vigie/internal/cluster/kubeconfig"
)

// TestNew returns an unstarted backend with the given path.
func TestNew(t *testing.T) {
	backend := kubeconfig.New("/path/to/kubeconfig", "")
	if backend == nil {
		t.Fatal("New returned nil")
	}
	if backend.Kubeconfig() != "/path/to/kubeconfig" {
		t.Fatalf("expected kubeconfig path %q, got %q", "/path/to/kubeconfig", backend.Kubeconfig())
	}
	if backend.RESTConfig() != nil {
		t.Fatal("expected nil RESTConfig before Start")
	}
}

// TestStart_BadKubeconfigPath verifies that Start returns an error when the
// kubeconfig file does not exist, without requiring a real cluster.
func TestStart_BadKubeconfigPath(t *testing.T) {
	backend := kubeconfig.New("/nonexistent/path/to/kubeconfig.yaml", "")
	err := backend.Start(context.Background())
	if err == nil {
		t.Fatal("expected Start to fail with a missing kubeconfig file")
	}
	if backend.RESTConfig() != nil {
		t.Fatal("expected nil RESTConfig after failed Start")
	}
}

// TestStop_NoOp verifies that Stop always returns nil regardless of whether
// Start was called.
func TestStop_NoOp(t *testing.T) {
	backend := kubeconfig.New("/some/kubeconfig", "")
	if err := backend.Stop(context.Background()); err != nil {
		t.Fatalf("Stop should be a no-op, got: %v", err)
	}
}

// TestLoadImages_NotSupported verifies that LoadImages returns a descriptive error.
func TestLoadImages_NotSupported(t *testing.T) {
	backend := kubeconfig.New("/some/kubeconfig", "")
	err := backend.LoadImages(context.Background(), []string{"nginx:latest"})
	if err == nil {
		t.Fatal("expected LoadImages to return an error for external clusters")
	}
	if !errors.Is(err, kubeconfig.ErrImageLoadNotSupported) {
		t.Fatalf("expected ErrImageLoadNotSupported, got: %v", err)
	}
}

func fakeAPIServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"major":"1","minor":"36","gitVersion":"v1.36.1"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeTwoContextKubeconfig(t *testing.T, selectedServer string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	content := `apiVersion: v1
kind: Config
current-context: prod
clusters:
  - name: prod
    cluster: { server: "http://127.0.0.1:9" }
  - name: test
    cluster: { server: "` + selectedServer + `" }
users:
  - name: u
    user: { token: t }
contexts:
  - name: prod
    context: { cluster: prod, user: u }
  - name: test
    context: { cluster: test, user: u }
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStart_ExplicitContextWinsAndIsPinnedForHooks(t *testing.T) {
	srv := fakeAPIServer(t)
	backend := kubeconfig.New(writeTwoContextKubeconfig(t, srv.URL), "test")
	if err := backend.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := backend.RESTConfig().Host; got != srv.URL {
		t.Fatalf("REST host = %q, want the selected context's server %q", got, srv.URL)
	}

	pinned, err := clientcmd.LoadFromFile(backend.Kubeconfig())
	if err != nil {
		t.Fatalf("loading pinned kubeconfig: %v", err)
	}
	if pinned.CurrentContext != "test" || len(pinned.Contexts) != 1 {
		t.Fatalf("pinned kubeconfig must hold only the selected context, got current=%q contexts=%d",
			pinned.CurrentContext, len(pinned.Contexts))
	}

	if err := backend.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := os.Stat(pinned.Contexts["test"].LocationOfOrigin); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stop must remove the pinned kubeconfig, stat err = %v", err)
	}
}

func TestStart_UnknownContextListsAvailable(t *testing.T) {
	backend := kubeconfig.New(writeTwoContextKubeconfig(t, "http://127.0.0.1:9"), "staging")
	err := backend.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "prod, test") {
		t.Fatalf("want an error listing the available contexts, got %v", err)
	}
}
