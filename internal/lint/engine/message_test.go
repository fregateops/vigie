package engine

import "testing"

func TestInterpolate(t *testing.T) {
	bindings := map[string]any{
		"chart": map[string]any{
			"apiVersion": "v2",
			"name":       "mychart",
		},
		"doc": map[string]any{
			"kind": "Deployment",
			"metadata": map[string]any{
				"name":      "release-name-app",
				"namespace": "prod",
			},
		},
	}

	tests := []struct {
		in   string
		want string
	}{
		{"plain text", "plain text"},
		{"got {{ chart.apiVersion }}", "got v2"},
		{"{{ chart.name }} ok", "mychart ok"},
		{"{{ doc.kind }}/{{ doc.metadata.name }}", "Deployment/release-name-app"},
		{"missing {{ chart.unknown }}", "missing "},
		{"deep {{ doc.metadata.namespace }}", "deep prod"},
	}
	for _, tc := range tests {
		got := interpolate(tc.in, bindings)
		if got != tc.want {
			t.Errorf("interpolate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
