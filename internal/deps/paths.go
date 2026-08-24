package deps

import "path/filepath"

// resolveDepPath makes a relative path declared by a dependency absolute
// against baseDir - the directory of the test file that declared it - so a dep
// means the same thing however vigie was invoked.
//
// Without this, `manifest: ./fixtures/x.yaml` resolves against the process
// working directory: the suite passes when run from its own directory and fails
// from the repo root. Empty paths, absolute paths, and an empty baseDir are
// returned unchanged.
func resolveDepPath(path, baseDir string) string {
	if path == "" || baseDir == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}
