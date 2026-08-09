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

func TestGet_QuotedBracketKeys(t *testing.T) {
	doc := map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{
				"app.kubernetes.io/name":     "basic",
				"app.kubernetes.io/instance": "release-name",
			},
		},
		"data": map[string]any{
			"config.yaml": "key: value",
		},
	}

	tests := []struct {
		name  string
		expr  string
		want  any
		found bool
	}{
		{"double-quoted dotted/slash key", `metadata.labels["app.kubernetes.io/name"]`, "basic", true},
		{"single-quoted dotted/slash key", `metadata.labels['app.kubernetes.io/instance']`, "release-name", true},
		{"dotted key at leaf", `data["config.yaml"]`, "key: value", true},
		{"missing quoted key", `metadata.labels["nope"]`, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found, err := Get(doc, tt.expr)
			if err != nil {
				t.Fatalf("Get(%q): %v", tt.expr, err)
			}
			if found != tt.found {
				t.Fatalf("Get(%q): found=%v want %v", tt.expr, found, tt.found)
			}
			if found && got != tt.want {
				t.Errorf("Get(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestParse_MalformedBrackets(t *testing.T) {
	for _, expr := range []string{
		`labels["unterminated`,
		`containers[1`,
		`containers[-1]`,
		`labels["x"`,
	} {
		if _, _, err := Get(map[string]any{}, expr); err == nil {
			t.Errorf("Get(%q): expected a parse error, got nil", expr)
		}
	}
}
