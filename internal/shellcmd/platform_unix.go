//go:build !windows

package shellcmd

import (
	"os/exec"
	"syscall"
)

// hostAllowlist is what a command needs to find tools and behave like a
// normal shell, and nothing that carries credentials.
var hostAllowlist = []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TMPDIR", "LANG", "LC_ALL", "TERM", "TZ"}

func killProcessGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
