// Package procutil provides process-management helpers shared across cluster
// backend implementations (simulated, k3d, kind). Keeping
// them here avoids import cycles: the parent cluster package imports all
// backends, so utilities cannot live there.
package procutil

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// ManagedProcess wraps a spawned child process with graceful shutdown and
// best-effort cleanup of a temporary work directory and log file. The
// simulated backend components (kube-controller-manager, kube-scheduler,
// kwok) all share this lifecycle, which previously lived as three slightly
// divergent copies.
type ManagedProcess struct {
	cmd     *exec.Cmd
	done    chan error
	workDir string
	logFile *os.File
}

// ProcessSpec configures a ManagedProcess started by Start.
type ProcessSpec struct {
	BinaryPath string
	Args       []string
	// WorkDir, when non-empty, is removed when Stop completes.
	WorkDir string
	// Stdout and Stderr receive the child's output. They are ignored when
	// LogFile is set.
	Stdout io.Writer
	Stderr io.Writer
	// LogFile, when non-empty, receives both stdout and stderr and is closed
	// on Stop.
	LogFile *os.File
	// NewProcessGroup puts the child in its own process group (Setpgid) so a
	// SIGTERM aimed at the vigie parent's group isn't re-routed to it.
	NewProcessGroup bool
}

// Start spawns the process described by spec and begins reaping it in a
// background goroutine. On error the caller remains responsible for cleaning
// up any WorkDir/LogFile it created, since no ManagedProcess is returned.
func Start(spec ProcessSpec) (*ManagedProcess, error) {
	if spec.BinaryPath == "" {
		return nil, errors.New("binary path is empty")
	}

	cmd := exec.Command(spec.BinaryPath, spec.Args...) //nolint:gosec // args are constant + caller-controlled paths
	if spec.LogFile != nil {
		cmd.Stdout = spec.LogFile
		cmd.Stderr = spec.LogFile
	} else {
		cmd.Stdout = spec.Stdout
		cmd.Stderr = spec.Stderr
	}
	if spec.NewProcessGroup {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	proc := &ManagedProcess{
		cmd:     cmd,
		done:    make(chan error, 1),
		workDir: spec.WorkDir,
		logFile: spec.LogFile,
	}
	go func() {
		proc.done <- cmd.Wait()
		close(proc.done)
	}()
	return proc, nil
}

// Exited returns a channel that yields the process's wait result and is then
// closed. Callers use it to detect an immediate crash shortly after Start;
// reading it does not prevent a later Stop from draining the same channel
// (Stop then observes the closed channel and returns nil).
func (proc *ManagedProcess) Exited() <-chan error {
	return proc.done
}

// Stop sends SIGTERM, waits up to grace for a clean exit, then escalates to
// SIGKILL. Termination by signal is treated as success; only a genuine
// non-signal exit error or a failed force-kill propagates. The work
// directory and log file are always cleaned up. Safe to call on a nil
// receiver or more than once.
func (proc *ManagedProcess) Stop(grace time.Duration) error {
	if proc == nil {
		return nil
	}
	if proc.cmd == nil || proc.cmd.Process == nil {
		proc.cleanup()
		return nil
	}
	defer proc.cleanup()

	// A SIGTERM failure (including "already gone") is non-fatal: we fall
	// through to the wait/kill below on the same PID either way.
	_ = proc.cmd.Process.Signal(syscall.SIGTERM)

	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case waitErr := <-proc.done:
		return IgnoreSignalExit(waitErr)
	case <-timer.C:
		if err := proc.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("force-killing process: %w", err)
		}
		return IgnoreSignalExit(<-proc.done)
	}
}

// cleanup closes the log file and removes the work directory. Idempotent.
func (proc *ManagedProcess) cleanup() {
	if proc.logFile != nil {
		_ = proc.logFile.Close()
		proc.logFile = nil
	}
	if proc.workDir != "" {
		_ = os.RemoveAll(proc.workDir)
		proc.workDir = ""
	}
}

// PickFreePort asks the kernel for a free TCP port on 127.0.0.1 by binding
// :0 and closing the listener. The port is then passed to a subprocess for
// its secure-serving endpoint. A short race window between close and reuse
// is unavoidable but acceptable here.
func PickFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = listener.Close() }()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}
	return addr.Port, nil
}

// ReadLogTail returns up to the last 4 KiB of the file at path. Best-effort:
// returns the empty string when the file cannot be read so callers can safely
// interpolate the result into an error message.
func ReadLogTail(path string) string {
	const tailSize = 4096
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	stat, err := file.Stat()
	if err != nil {
		return ""
	}
	offset := int64(0)
	if stat.Size() > tailSize {
		offset = stat.Size() - tailSize
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	buf, err := io.ReadAll(file)
	if err != nil {
		return ""
	}
	return string(buf)
}

// IgnoreSignalExit returns nil when err represents a process that was
// terminated by a signal (SIGTERM, SIGKILL, SIGINT). These are expected
// outcomes when Stop() is called on a subprocess and should not bubble up
// as errors to callers.
func IgnoreSignalExit(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			switch status.Signal() {
			case syscall.SIGTERM, syscall.SIGKILL, syscall.SIGINT:
				return nil
			}
		}
	}
	return err
}
