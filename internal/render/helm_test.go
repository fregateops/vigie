package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeChart(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const configMapTemplate = `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Chart.Name }}
data:
  value: {{ .Values.value | toJson | quote }}
`

func umbrellaChart(t *testing.T) string {
	return writeChart(t, map[string]string{
		"Chart.yaml": `apiVersion: v2
name: umbrella
version: 0.1.0
dependencies:
  - name: sub
    version: 0.1.0
    condition: sub.enabled
`,
		"values.yaml":                   "sub:\n  enabled: false\nvalue: x\n",
		"templates/cm.yaml":             configMapTemplate,
		"charts/sub/Chart.yaml":         "apiVersion: v2\nname: sub\nversion: 0.1.0\n",
		"charts/sub/templates/cm.yaml":  configMapTemplate,
		"charts/sub/templates/svc.yaml": "apiVersion: v1\nkind: Service\nmetadata:\n  name: sub\n",
	})
}

func sources(r *Result) []string {
	out := make([]string, len(r.Docs))
	for i, d := range r.Docs {
		out[i] = d.Source
	}
	return out
}

func TestRender_SubchartConditionIsHonoured(t *testing.T) {
	chart := umbrellaChart(t)

	off, err := Render(Request{ChartPath: chart})
	if err != nil {
		t.Fatal(err)
	}
	if got := sources(off); len(got) != 1 || got[0] != "templates/cm.yaml" {
		t.Fatalf("disabled subchart rendered: %v", got)
	}

	on, err := Render(Request{ChartPath: chart, Values: map[string]any{"sub": map[string]any{"enabled": true}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(on.Docs); got != 3 {
		t.Fatalf("enabled subchart: want 3 docs, got %v", sources(on))
	}
}

func TestRender_MissingDependencyFails(t *testing.T) {
	chart := writeChart(t, map[string]string{
		"Chart.yaml":        "apiVersion: v2\nname: c\nversion: 0.1.0\ndependencies:\n  - name: absent\n    version: 0.1.0\n",
		"templates/cm.yaml": configMapTemplate,
	})
	_, err := Render(Request{ChartPath: chart})
	if err == nil || !strings.Contains(err.Error(), "absent") || !strings.Contains(err.Error(), "helm dependency build") {
		t.Fatalf("want missing-dependency error naming the chart and the fix, got %v", err)
	}
}

func TestRender_TemplatesAreGlobsRelativeToChartRoot(t *testing.T) {
	chart := umbrellaChart(t)
	enabled := map[string]any{"sub": map[string]any{"enabled": true}}

	res, err := Render(Request{ChartPath: chart, Values: enabled, Templates: []string{"templates/cm.yaml"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := sources(res); len(got) != 1 || got[0] != "templates/cm.yaml" {
		t.Fatalf("templates/cm.yaml must not match the subchart's cm.yaml: %v", got)
	}

	res, err = Render(Request{ChartPath: chart, Values: enabled, Templates: []string{"charts/sub/templates/*.yaml"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(res.Docs); got != 2 {
		t.Fatalf("glob: want 2 subchart docs, got %v", sources(res))
	}
}

func TestRender_UnknownTemplateIsAnError(t *testing.T) {
	_, err := Render(Request{ChartPath: umbrellaChart(t), Templates: []string{"templates/typo.yaml"}})
	if err == nil || !strings.Contains(err.Error(), "templates/typo.yaml") {
		t.Fatalf("want error naming the unmatched template, got %v", err)
	}
}

func TestRender_InvalidYAMLOutputIsAnError(t *testing.T) {
	chart := writeChart(t, map[string]string{
		"Chart.yaml":         "apiVersion: v2\nname: c\nversion: 0.1.0\n",
		"templates/bad.yaml": "kind: ConfigMap\n---\nkey: [unclosed\n",
	})
	_, err := Render(Request{ChartPath: chart})
	if err == nil || !strings.Contains(err.Error(), "templates/bad.yaml") {
		t.Fatalf("want parse error naming the template, got %v", err)
	}
}

func TestRender_InvalidKubeVersionIsAnError(t *testing.T) {
	if _, err := Render(Request{ChartPath: umbrellaChart(t), KubeVersion: "not-a-version"}); err == nil {
		t.Fatal("want error for an unparseable kubeVersion")
	}
}

func TestRender_ReleaseMetadata(t *testing.T) {
	chart := writeChart(t, map[string]string{
		"Chart.yaml": "apiVersion: v2\nname: c\nversion: 0.1.0\n",
		"templates/cm.yaml": `apiVersion: v1
kind: ConfigMap
metadata:
  name: c
data:
  revision: "{{ .Release.Revision }}"
  upgrade: "{{ .Release.IsUpgrade }}"
`,
	})
	data := func(req Request) map[string]any {
		t.Helper()
		res, err := Render(req)
		if err != nil {
			t.Fatal(err)
		}
		return res.Docs[0].Object["data"].(map[string]any)
	}

	if got := data(Request{ChartPath: chart}); got["revision"] != "1" || got["upgrade"] != "false" {
		t.Errorf("default release metadata = %v, want revision 1 install (helm template)", got)
	}
	if got := data(Request{ChartPath: chart, Revision: 4, IsUpgrade: true}); got["revision"] != "4" || got["upgrade"] != "true" {
		t.Errorf("upgrade release metadata = %v", got)
	}
}
