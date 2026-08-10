package main

import (
	"path/filepath"
	"testing"
)

func TestResolveOverlayPaths(t *testing.T) {
	chart := "charts/api"
	abs := filepath.Join(t.TempDir(), "prod.yaml")

	cases := []struct {
		name     string
		overlays []string
		want     []string
	}{
		{"nil", nil, nil},
		{"bare name resolves against chart", []string{"values-prod.yaml"},
			[]string{filepath.Join(chart, "values-prod.yaml")}},
		{"explicit ./ kept as-is", []string{"./local.yaml"}, []string{"./local.yaml"}},
		{"explicit ../ kept as-is", []string{"../shared.yaml"}, []string{"../shared.yaml"}},
		{"absolute kept as-is", []string{abs}, []string{abs}},
		{"blank entries dropped", []string{"  ", "a.yaml"},
			[]string{filepath.Join(chart, "a.yaml")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveOverlayPaths(chart, tc.overlays)
			if len(got) != len(tc.want) {
				t.Fatalf("resolveOverlayPaths(%q) = %v, want %v", tc.overlays, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
