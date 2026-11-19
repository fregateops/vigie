package runner

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/shellcmd"
)

const defaultHookTimeout = 2 * time.Minute

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

func runOneHook(ctx context.Context, label string, idx int, hook dsl.ShellCommand, env HookEnv) error {
	if hook.Run == "" {
		return fmt.Errorf("%s[%d]: run is required", label, idx)
	}
	slog.Debug("running lifecycle hook", "label", label, "idx", idx, "shell", hook.Shell)
	out, err := shellcmd.Run(ctx, shellcmd.Spec{Command: hook, Timeout: defaultHookTimeout, Env: env.vars()})
	stdout := strings.TrimSpace(string(out.Stdout))
	stderr := strings.TrimSpace(string(out.Stderr))
	if stdout != "" {
		slog.Debug("hook stdout", "label", label, "idx", idx, "out", stdout)
	}
	if err != nil {
		if stderr != "" {
			return fmt.Errorf("%s[%d] failed: %w (stderr: %s)", label, idx, err, stderr)
		}
		return fmt.Errorf("%s[%d] failed: %w", label, idx, err)
	}
	if stderr != "" {
		slog.Debug("hook stderr", "label", label, "idx", idx, "err", stderr)
	}
	return nil
}

// vars renders the env as the KEY=VALUE pairs hooks receive, omitting unset ones.
func (env HookEnv) vars() []string {
	var out []string
	for key, val := range map[string]string{
		"KUBECONFIG":      env.Kubeconfig,
		"VIGIE_SUITE":     env.Suite,
		"VIGIE_TEST":      env.Test,
		"VIGIE_NAMESPACE": env.Namespace,
	} {
		if val != "" {
			out = append(out, key+"="+val)
		}
	}
	return out
}
