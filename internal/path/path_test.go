package path

import (
	"testing"
)

func TestGet(t *testing.T) {
	doc := map[string]any{
		"kind": "Deployment",
		"spec": map[string]any{
			"replicas": 3,
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{
						map[string]any{"name": "app", "image": "myrepo/app:v1"},
						map[string]any{"name": "sidecar", "image": "sidecar:latest"},
					},
				},
			},
		},
	}

	cases := []struct {
		expr  string
		want  any
		found bool
	}{
		{"kind", "Deployment", true},
		{"spec.replicas", 3, true},
		{"spec.template.spec.containers[0].image", "myrepo/app:v1", true},
		{"spec.template.spec.containers[1].name", "sidecar", true},
		{".", doc, true},
		{"missing", nil, false},
		{"spec.missing.deep", nil, false},
		{"spec.template.spec.containers[99].name", nil, false},
	}

	for _, tc := range cases {
		got, found, err := Get(doc, tc.expr)
		if err != nil {
			t.Errorf("Get(%q) error: %v", tc.expr, err)
			continue
		}
		if found != tc.found {
			t.Errorf("Get(%q) found=%v, want %v", tc.expr, found, tc.found)
			continue
		}
		if found && tc.expr != "." && got != tc.want {
			t.Errorf("Get(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}
