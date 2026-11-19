package cienv

import "testing"

func TestIsCI(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"nothing set", map[string]string{}, false},
		{"github", map[string]string{"GITHUB_ACTIONS": "true"}, true},
		{"generic CI=true", map[string]string{"CI": "true"}, true},
		{"VIGIE_CI=none overrides CI=true", map[string]string{"CI": "true", "VIGIE_CI": "none"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"VIGIE_CI", "GITHUB_ACTIONS", "GITLAB_CI", "CI"} {
				t.Setenv(k, tc.env[k])
			}
			if got := IsCI(); got != tc.want {
				t.Errorf("IsCI() = %v, want %v", got, tc.want)
			}
		})
	}
}
