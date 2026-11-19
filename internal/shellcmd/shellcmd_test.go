//go:build !windows

package shellcmd

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/fregateops/vigie/internal/dsl"
)

func run(t *testing.T, cmd dsl.ShellCommand, env ...string) (string, error) {
	t.Helper()
	out, err := Run(context.Background(), Spec{Command: cmd, Timeout: 10 * time.Second, Env: env})
	return strings.TrimSpace(string(out.Stdout)), err
}

func TestRun_HostSecretsStayOutUnlessPassed(t *testing.T) {
	t.Setenv("VIGIE_TEST_SECRET", "s3cr3t")

	got, err := run(t, dsl.ShellCommand{Run: `printf '%s|%s' "$VIGIE_TEST_SECRET" "$KUBECONFIG"`}, "KUBECONFIG=/k")
	if err != nil {
		t.Fatal(err)
	}
	if got != "|/k" {
		t.Errorf("without passEnv: got %q, want the secret hidden and vigie's KUBECONFIG set", got)
	}

	got, err = run(t, dsl.ShellCommand{Run: `printf '%s' "$VIGIE_TEST_SECRET"`, PassEnv: []string{"VIGIE_TEST_SECRET"}})
	if err != nil {
		t.Fatal(err)
	}
	if got != "s3cr3t" {
		t.Errorf("with passEnv: got %q, want the secret passed through", got)
	}
}

func TestRun_PathIsKept(t *testing.T) {
	if got, err := run(t, dsl.ShellCommand{Run: `printf '%s' "$PATH"`}); err != nil || got != os.Getenv("PATH") {
		t.Errorf("PATH = %q (err %v), want the host PATH so tools resolve", got, err)
	}
}

func TestRun_UnsetPassEnvIsAnError(t *testing.T) {
	_, err := run(t, dsl.ShellCommand{Run: "true", PassEnv: []string{"VIGIE_SURELY_UNSET_VAR"}})
	if err == nil || !strings.Contains(err.Error(), "VIGIE_SURELY_UNSET_VAR") {
		t.Fatalf("want an error naming the unset variable, got %v", err)
	}
}

func TestRun_TimeoutKillsTheWholeProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	_, err := Run(context.Background(), Spec{
		Command: dsl.ShellCommand{Run: "sleep 30 & echo $! > " + pidFile + "; wait", Timeout: "300ms"},
	})
	if err == nil {
		t.Fatal("want a timeout error")
	}
	raw, readErr := os.ReadFile(pidFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("background child %d outlived the timed-out hook", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
