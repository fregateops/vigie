package doctor

import "os/exec"

// lookPath and runCommand are package-level vars so tests can swap them
// for fake implementations. They default to the real os/exec package.
var (
	lookPath = exec.LookPath

	runCommand = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).Output()
	}
)
