//go:build windows

package shellcmd

import "os/exec"

// hostAllowlist is what a command needs to find tools and behave like a
// normal shell, and nothing that carries credentials.
var hostAllowlist = []string{"PATH", "PATHEXT", "SYSTEMROOT", "COMSPEC", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "TEMP", "TMP", "USERNAME", "LANG", "TZ"}

// killProcessGroupOnCancel keeps exec's default (kill the shell only):
// Windows has no process groups to signal.
func killProcessGroupOnCancel(*exec.Cmd) {}
