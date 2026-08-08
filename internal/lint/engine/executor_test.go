package engine

import (
	"strings"
	"testing"

	"github.com/fregateops/vigie/internal/lint"
)

func TestRunChartScope(t *testing.T) {
	def := RuleDef{
		ID: "api-version", Severity: "error", Scope: "chart",
		Expr:    `chart.apiVersion == "v2"`,
		Message: `apiVersion must be v2 (got "{{ chart.apiVersion }}")`,
	}
	id := "chart-yaml_api-version"

	pass := lint.Context{ChartMeta: map[string]any{"apiVersion": "v2"}}
	if got := runChartScope(pass, id, def); len(got) != 0 {
		t.Errorf("v2 should pass; got %d findings", len(got))
	}

	fail := lint.Context{ChartMeta: map[string]any{"apiVersion": "v1"}}
	got := runChartScope(fail, id, def)
	if len(got) != 1 {
		t.Fatalf("v1 should fail; got %d findings", len(got))
	}
	if got[0].RuleID != id || got[0].Message != `apiVersion must be v2 (got "v1")` {
		t.Errorf("unexpected finding: %+v", got[0])
	}
}

func TestRunDocumentScope_HardcodedNamespace(t *testing.T) {
	def := RuleDef{
		ID: "hardcoded-namespace", Severity: "error", Scope: "document",
		Expr:    `!has(doc.metadata) || !has(doc.metadata.namespace) || doc.metadata.namespace == "" || doc.metadata.namespace == renderNamespace`,
		Message: `{{ doc.kind }}/{{ doc.metadata.name }}: hardcoded namespace`,
	}
	docs := []map[string]any{
		{"kind": "Deployment", "metadata": map[string]any{"name": "ok", "namespace": "default"}},
		{"kind": "Deployment", "metadata": map[string]any{"name": "no-ns"}},
		{"kind": "Service", "metadata": map[string]any{"name": "bad", "namespace": "production"}},
	}
	got := runDocumentScope(lint.Context{RenderedDocs: docs}, "template-best-practices_hardcoded-namespace", def)
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d: %+v", len(got), got)
	}
	if got[0].Message != "Service/bad: hardcoded namespace" {
		t.Errorf("unexpected message: %q", got[0].Message)
	}
}

func TestRunContainerScope_MissingLimits(t *testing.T) {
	def := RuleDef{
		ID: "missing-resource-limits", Severity: "warning", Scope: "container",
		KindFilter: []string{"Deployment"},
		Expr:       `has(container.resources) && has(container.resources.limits) && size(container.resources.limits) > 0`,
		Message:    `{{ doc.kind }}/{{ doc.metadata.name }}: container "{{ container.name }}" has no resources.limits`,
	}
	deploy := func(name string, containers []any) map[string]any {
		return map[string]any{
			"kind":     "Deployment",
			"metadata": map[string]any{"name": name},
			"spec": map[string]any{
				"template": map[string]any{
					"spec": map[string]any{"containers": containers},
				},
			},
		}
	}
	docs := []map[string]any{
		deploy("good", []any{
			map[string]any{"name": "c1", "resources": map[string]any{"limits": map[string]any{"cpu": "100m"}}},
		}),
		deploy("missing", []any{
			map[string]any{"name": "c1"},
			map[string]any{"name": "c2", "resources": map[string]any{}},
		}),
		// Non-Deployment kind ignored.
		{"kind": "ConfigMap", "metadata": map[string]any{"name": "skip"}},
	}
	got := runContainerScope(lint.Context{RenderedDocs: docs}, "template-best-practices_missing-resource-limits", def)
	if len(got) != 2 {
		t.Fatalf("want 2 findings, got %d", len(got))
	}
}

func TestRunRemovedAPIScope_VersionFilter(t *testing.T) {
	def := RuleDef{
		ID: "deprecated-api-version", Severity: "error", Scope: "removedAPI",
		RemovedAPIs: []RemovedAPIDef{
			{APIVersion: "extensions/v1beta1", Kind: "Ingress", RemovedIn: "1.22", Replacement: "networking.k8s.io/v1"},
			{APIVersion: "batch/v1beta1", Kind: "CronJob", RemovedIn: "1.25", Replacement: "batch/v1"},
		},
	}
	docs := []map[string]any{
		{"apiVersion": "extensions/v1beta1", "kind": "Ingress", "metadata": map[string]any{"name": "ing"}},
		{"apiVersion": "batch/v1beta1", "kind": "CronJob", "metadata": map[string]any{"name": "cron"}},
	}

	// No target version → both flagged.
	got := runRemovedAPIScope(lint.Context{RenderedDocs: docs}, "deprecation_removed-api", def)
	if len(got) != 2 {
		t.Errorf("no target: want 2 findings, got %d", len(got))
	}

	// Target 1.23 → Ingress (removed 1.22) flagged, CronJob (removed 1.25) not.
	got = runRemovedAPIScope(lint.Context{RenderedDocs: docs, KubeVersion: "1.23"}, "deprecation_removed-api", def)
	if len(got) != 1 {
		t.Fatalf("1.23: want 1 finding, got %d: %+v", len(got), got)
	}
	if got[0].Message == "" || !strings.Contains(got[0].Message, "Ingress") {
		t.Errorf("1.23: expected Ingress finding, got %q", got[0].Message)
	}

	// Target 1.20 → neither flagged.
	got = runRemovedAPIScope(lint.Context{RenderedDocs: docs, KubeVersion: "1.20"}, "deprecation_removed-api", def)
	if len(got) != 0 {
		t.Errorf("1.20: want 0 findings, got %d", len(got))
	}
}
