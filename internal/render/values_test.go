package render

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildValues_HelmOrderAndTypes(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	prod := filepath.Join(dir, "prod.yaml")
	if err := os.WriteFile(base, []byte("image:\n  repo: app\n  tag: v1\nreplicas: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prod, []byte("image:\n  tag: v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := BuildValues(ValueInputs{
		Files:      []string{base, prod},
		SetJSON:    map[string]any{"args": `{"v":"3"}`, "replicas": 2},
		Set:        map[string]any{"replicas": 3, "hosts": []any{"a", "b"}, "flag": "true"},
		SetLiteral: map[string]string{"raw": "a,b=c"},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{
		"image":    map[string]any{"repo": "app", "tag": "v2"},
		"args":     map[string]any{"v": "3"},
		"replicas": float64(3),
		"hosts":    []any{"a", "b"},
		"flag":     "true",
		"raw":      "a,b=c",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildValues =\n%#v\nwant\n%#v", got, want)
	}
}

func TestBuildValues_InvalidSetJSONFails(t *testing.T) {
	if _, err := BuildValues(ValueInputs{SetJSON: map[string]any{"a": "{not json"}}); err == nil {
		t.Fatal("want error for invalid setJson text")
	}
}

func TestBuildValues_MissingFileFails(t *testing.T) {
	if _, err := BuildValues(ValueInputs{Files: []string{filepath.Join(t.TempDir(), "absent.yaml")}}); err == nil {
		t.Fatal("want error for a missing values file")
	}
}
