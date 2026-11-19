// Package shellcmd runs the shell commands test files declare (lifecycle
// hooks, secret `generate:`) with an environment limited to an allowlist,
// so a command cannot read the CI secrets vigie itself runs with unless the
// test file names them in `passEnv:`.
package shellcmd

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/fregateops/vigie/internal/dsl"
)

const (
	defaultShell = "sh"
	// waitDelay bounds Wait after the process is killed, so a backgrounded
	// child holding stdio open cannot pin it.
	waitDelay = 5 * time.Second
)

type Spec struct {
	Command dsl.ShellCommand
	// Timeout applies when Command sets none.
	Timeout time.Duration
	// Dir applies when Command sets none.
	Dir string
	// Env holds the KEY=VALUE pairs vigie provides (KUBECONFIG, VIGIE_*).
	Env []string
}

type Output struct {
	Stdout, Stderr []byte
}

// Run executes spec. The process group is killed on timeout or cancellation,
// so children the shell spawned do not outlive it.
func Run(ctx context.Context, spec Spec) (Output, error) {
	timeout := spec.Timeout
	if spec.Command.Timeout != "" {
		d, err := time.ParseDuration(spec.Command.Timeout)
		if err != nil {
			return Output{}, fmt.Errorf("parsing timeout %q: %w", spec.Command.Timeout, err)
		}
		timeout = d
	}
	env, err := environment(spec)
	if err != nil {
		return Output{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, cmp.Or(spec.Command.Shell, defaultShell), "-c", spec.Command.Run)
	cmd.Dir = cmp.Or(spec.Command.Dir, spec.Dir)
	cmd.Env = env
	cmd.WaitDelay = waitDelay
	killProcessGroupOnCancel(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return Output{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

// environment is the allowlisted host variables, the ones the command passes
// through by name, vigie's own, then the command's literal `env:`.
func environment(spec Spec) ([]string, error) {
	var env []string
	for _, name := range hostAllowlist {
		if val, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+val)
		}
	}
	for _, name := range spec.Command.PassEnv {
		val, ok := os.LookupEnv(name)
		if !ok {
			return nil, fmt.Errorf("passEnv: %s is not set in vigie's environment", name)
		}
		env = append(env, name+"="+val)
	}
	env = append(env, spec.Env...)
	for key, val := range spec.Command.Env {
		env = append(env, key+"="+val)
	}
	return env, nil
}
