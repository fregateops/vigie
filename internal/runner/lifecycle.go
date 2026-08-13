package runner

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fregateops/vigie/internal/dsl"
)

const (
	defaultHookShell   = "sh"
	defaultHookTimeout = 2 * time.Minute
)

// HookEnv groups variables exposed to lifecycle hook commands.
//
// KUBECONFIG: path to the active cluster's kubeconfig (so the hook can run
// kubectl without needing a separate flag).
// VIGIE_SUITE / VIGIE_TEST: identification for hook scripts that
// branch on which suite/test is running. Empty when running suite-scoped
// hooks (beforeAll/afterAll).
type HookEnv struct {
	Kubeconfig string
	Suite      string
	Test       string
	Namespace  string
}

// RunHooks executes each ShellCommand in order, aborting on the first
// failure. The label is used in log + error messages (e.g. "beforeAll",
// "setup"). A zero-length hooks slice is a no-op.
func RunHooks(ctx context.Context, label string, hooks []dsl.LifecycleHook, env HookEnv) error {
	for hookIdx, hook := range hooks {
		if err := runOneHook(ctx, label, hookIdx, hook, env); err != nil {
			return err
		}
	}
	return nil
}

func runOneHook(parent context.Context, label string, idx int, hook dsl.ShellCommand, env HookEnv) error {
	if hook.Run == "" {
		return fmt.Errorf("%s[%d]: run is required", label, idx)
	}
	shell := hook.Shell
	if shell == "" {
		shell = defaultHookShell
	}
	timeout := defaultHookTimeout
	if hook.Timeout != "" {
		d, err := time.ParseDuration(hook.Timeout)
		if err != nil {
			return fmt.Errorf("%s[%d]: parsing timeout %q: %w", label, idx, hook.Timeout, err)
		}
		timeout = d
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, "-c", hook.Run)
	// Bound the wait after kill so a hook that leaves dangling pipes
	// (e.g. a backgrounded child holding stdio open) can't pin Wait
	// indefinitely. Without this, exec.CommandContext can block in
	// Wait() reading stdio long after the parent process is dead.
	cmd.WaitDelay = 5 * time.Second
	if hook.Dir != "" {
		cmd.Dir = hook.Dir
	}

	cmdEnv := append([]string{}, os.Environ()...)
	if env.Kubeconfig != "" {
		cmdEnv = append(cmdEnv, "KUBECONFIG="+env.Kubeconfig)
	}
	if env.Suite != "" {
		cmdEnv = append(cmdEnv, "VIGIE_SUITE="+env.Suite)
	}
	if env.Test != "" {
		cmdEnv = append(cmdEnv, "VIGIE_TEST="+env.Test)
	}
	if env.Namespace != "" {
		cmdEnv = append(cmdEnv, "VIGIE_NAMESPACE="+env.Namespace)
	}
	for key, val := range hook.Env {
		cmdEnv = append(cmdEnv, key+"="+val)
	}
	cmd.Env = cmdEnv

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	slog.Debug("running lifecycle hook", "label", label, "idx", idx, "shell", shell)
	err := cmd.Run()
	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())
	if stdoutStr != "" {
		slog.Debug("hook stdout", "label", label, "idx", idx, "out", stdoutStr)
	}
	if err != nil {
		if stderrStr != "" {
			return fmt.Errorf("%s[%d] failed: %w (stderr: %s)", label, idx, err, stderrStr)
		}
		return fmt.Errorf("%s[%d] failed: %w", label, idx, err)
	}
	if stderrStr != "" {
		slog.Debug("hook stderr", "label", label, "idx", idx, "err", stderrStr)
	}
	return nil
}
