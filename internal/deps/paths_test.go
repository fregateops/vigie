package deps

import (
	"path/filepath"
	"testing"
)

func TestResolveDepPath(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		baseDir string
		want    string
	}{
		{
			// The bug this exists to prevent: `manifest: ./fixtures/x.yaml`
			// resolving against the working directory, so a suite passed from
			// its own directory and failed from the repo root.
			name:    "relative path joins the base",
			path:    "./fixtures/landing-page.yaml",
			baseDir: "testdata/charts/basic/tests/integration",
			want:    filepath.Join("testdata/charts/basic/tests/integration", "fixtures/landing-page.yaml"),
		},
		{
			name:    "absolute path is left alone",
			path:    "/etc/manifests/x.yaml",
			baseDir: "testdata/charts/basic/tests",
			want:    "/etc/manifests/x.yaml",
		},
		{
			name:    "no base leaves the path as written",
			path:    "./fixtures/x.yaml",
			baseDir: "",
			want:    "./fixtures/x.yaml",
		},
		{
			name:    "empty path stays empty",
			path:    "",
			baseDir: "testdata",
			want:    "",
		},
		{
			name:    "parent-relative path resolves above the base",
			path:    "../shared/x.yaml",
			baseDir: "testdata/charts/basic/tests/integration",
			want:    filepath.Join("testdata/charts/basic/tests", "shared/x.yaml"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveDepPath(tc.path, tc.baseDir); got != tc.want {
				t.Errorf("resolveDepPath(%q, %q) = %q, want %q", tc.path, tc.baseDir, got, tc.want)
			}
		})
	}
}
