package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fregateops/vigie/internal/dsl"
)

func TestRunHooks_Empty(t *testing.T) {
	if err := RunHooks(context.Background(), "beforeAll", nil, HookEnv{}); err != nil {
		t.Fatalf("empty hook list should be a no-op, got %v", err)
	}
}

func TestRunHooks_Sequential(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "marker")
	hooks := []dsl.LifecycleHook{
		{Run: "echo first > " + marker},
		{Run: "echo second >> " + marker},
	}
	if err := RunHooks(context.Background(), "beforeAll", hooks, HookEnv{}); err != nil {
		t.Fatalf("RunHooks: %v", err)
	}
	raw, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("reading marker: %v", err)
	}
	if string(raw) != "first\nsecond\n" {
		t.Errorf("unexpected marker content: %q", raw)
	}
}

func TestRunHooks_FailFastOnError(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "marker")
	hooks := []dsl.LifecycleHook{
		{Run: "exit 1"},
		{Run: "echo never > " + marker},
	}
	err := RunHooks(context.Background(), "beforeAll", hooks, HookEnv{})
	if err == nil {
		t.Fatal("expected error from first failing hook")
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Error("second hook should not have run after first failed")
	}
}

func TestRunHooks_EnvVarsExposed(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "marker")
	hooks := []dsl.LifecycleHook{
		{Run: "printf %s,%s,%s,%s \"$KUBECONFIG\" \"$VIGIE_SUITE\" \"$VIGIE_TEST\" \"$VIGIE_NAMESPACE\" > " + marker},
	}
	env := HookEnv{Kubeconfig: "/tmp/kc", Suite: "S", Test: "T", Namespace: "N"}
	if err := RunHooks(context.Background(), "setup", hooks, env); err != nil {
		t.Fatalf("RunHooks: %v", err)
	}
	raw, _ := os.ReadFile(marker)
	if got := string(raw); got != "/tmp/kc,S,T,N" {
		t.Errorf("env not exposed: %q", got)
	}
}

func TestRunHooks_RespectsTimeout(t *testing.T) {
	// sleep 60 is far longer than any reasonable kill-propagation delay so a
	// failure mode where Cancel doesn't fire is unambiguous in CI logs.
	hooks := []dsl.LifecycleHook{{Run: "sleep 60", Timeout: "500ms"}}
	start := time.Now()
	err := RunHooks(context.Background(), "setup", hooks, HookEnv{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	// WaitDelay bounds the post-kill wait at 5s; anything <30s proves the
	// timeout fired (the natural sleep is 60s).
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Errorf("timeout not enforced: took %v", elapsed)
	}
	if !strings.Contains(err.Error(), "setup[0]") {
		t.Errorf("expected error to identify hook label/idx: %v", err)
	}
}
