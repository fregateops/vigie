package cienv

import (
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Kind
	}{
		{
			name: "no env set returns none",
			env:  map[string]string{},
			want: KindNone,
		},
		{
			name: "GITHUB_ACTIONS=true returns github",
			env:  map[string]string{"GITHUB_ACTIONS": "true"},
			want: KindGitHubActions,
		},
		{
			name: "GITLAB_CI=true returns gitlab",
			env:  map[string]string{"GITLAB_CI": "true"},
			want: KindGitLabCI,
		},
		{
			name: "VIGIE_CI=none overrides GITHUB_ACTIONS=true",
			env:  map[string]string{"VIGIE_CI": "none", "GITHUB_ACTIONS": "true"},
			want: KindNone,
		},
		{
			name: "VIGIE_CI=github returns github",
			env:  map[string]string{"VIGIE_CI": "github"},
			want: KindGitHubActions,
		},
		{
			name: "VIGIE_CI=gitlab returns gitlab",
			env:  map[string]string{"VIGIE_CI": "gitlab"},
			want: KindGitLabCI,
		},
		{
			name: "VIGIE_CI=GITHUB (uppercase) returns github",
			env:  map[string]string{"VIGIE_CI": "GITHUB"},
			want: KindGitHubActions,
		},
		{
			name: "VIGIE_CI=none overrides GITLAB_CI=true",
			env:  map[string]string{"VIGIE_CI": "none", "GITLAB_CI": "true"},
			want: KindNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Clear relevant env vars before each test.
			for _, key := range []string{"VIGIE_CI", "GITHUB_ACTIONS", "GITLAB_CI"} {
				t.Setenv(key, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got := Detect()
			if got != tc.want {
				t.Errorf("Detect() = %v, want %v", got, tc.want)
			}
		})
	}
}
