package runner

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

func intPtr(i int) *int { return &i }

// docs used across the target-selection tests.
var (
	deployDoc = map[string]any{
		"kind":       "Deployment",
		"apiVersion": "apps/v1",
		"metadata": map[string]any{
			"name":      "web",
			"namespace": "prod",
			"labels":    map[string]any{"app": "web", "tier": "frontend"},
		},
	}
	svcDoc = map[string]any{
		"kind":       "Service",
		"apiVersion": "v1",
		"metadata": map[string]any{
			"name":   "web-svc",
			"labels": map[string]any{"app": "web"},
		},
	}
)

func TestMatchesTarget(t *testing.T) {
	tests := []struct {
		name   string
		doc    map[string]any
		target dsl.TargetSpec
		want   bool
	}{
		{"kind match", deployDoc, dsl.TargetSpec{Kind: "Deployment"}, true},
		{"kind mismatch", deployDoc, dsl.TargetSpec{Kind: "Service"}, false},
		{"apiVersion match", deployDoc, dsl.TargetSpec{APIVersion: "apps/v1"}, true},
		{"apiVersion mismatch", deployDoc, dsl.TargetSpec{APIVersion: "v1"}, false},
		{"name match", deployDoc, dsl.TargetSpec{Name: "web"}, true},
		{"name mismatch", deployDoc, dsl.TargetSpec{Name: "api"}, false},
		{"namespace match", deployDoc, dsl.TargetSpec{Namespace: "prod"}, true},
		{"namespace mismatch", deployDoc, dsl.TargetSpec{Namespace: "dev"}, false},
		{"labels subset match", deployDoc, dsl.TargetSpec{Labels: map[string]string{"app": "web"}}, true},
		{"labels mismatch", deployDoc, dsl.TargetSpec{Labels: map[string]string{"app": "api"}}, false},
		{"labels missing on doc", svcDoc, dsl.TargetSpec{Labels: map[string]string{"tier": "frontend"}}, false},
		{"combined kind+name+labels", deployDoc, dsl.TargetSpec{Kind: "Deployment", Name: "web", Labels: map[string]string{"tier": "frontend"}}, true},
		{"expr true", deployDoc, dsl.TargetSpec{Expr: `doc.kind == "Deployment"`}, true},
		{"expr false", deployDoc, dsl.TargetSpec{Expr: `doc.kind == "Service"`}, false},
		{"expr non-bool result", deployDoc, dsl.TargetSpec{Expr: `doc.kind`}, false},
		{"expr error", deployDoc, dsl.TargetSpec{Expr: `doc.nope.deeper`}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := tt.target
			if got := matchesTarget(tt.doc, &target); got != tt.want {
				t.Errorf("matchesTarget = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectDoc(t *testing.T) {
	docs := []map[string]any{deployDoc, svcDoc}

	t.Run("no target, single doc", func(t *testing.T) {
		doc, diag := selectDoc(dsl.Assertion{}, nil, []map[string]any{deployDoc}, nil)
		if doc == nil || diag != "" {
			t.Fatalf("want deployDoc, no diag; got doc=%v diag=%q", doc, diag)
		}
	})

	t.Run("no target, no docs", func(t *testing.T) {
		doc, diag := selectDoc(dsl.Assertion{}, nil, nil, nil)
		if doc != nil || diag == "" {
			t.Fatalf("want nil doc + diagnostic; got doc=%v diag=%q", doc, diag)
		}
	})

	t.Run("documentIndex in range", func(t *testing.T) {
		doc, diag := selectDoc(dsl.Assertion{}, &dsl.TargetSpec{DocumentIndex: intPtr(1)}, docs, nil)
		if diag != "" || doc["kind"] != "Service" {
			t.Fatalf("want svcDoc, no diag; got doc=%v diag=%q", doc, diag)
		}
	})

	t.Run("documentIndex out of range", func(t *testing.T) {
		doc, diag := selectDoc(dsl.Assertion{}, &dsl.TargetSpec{DocumentIndex: intPtr(9)}, docs, nil)
		if doc != nil || diag == "" {
			t.Fatalf("want nil doc + out-of-range diagnostic; got doc=%v diag=%q", doc, diag)
		}
	})

	t.Run("target selects single match", func(t *testing.T) {
		doc, diag := selectDoc(dsl.Assertion{}, &dsl.TargetSpec{Kind: "Service"}, docs, nil)
		if diag != "" || doc["kind"] != "Service" {
			t.Fatalf("want svcDoc, no diag; got doc=%v diag=%q", doc, diag)
		}
	})

	t.Run("target matches nothing yields diagnostic", func(t *testing.T) {
		doc, diag := selectDoc(dsl.Assertion{}, &dsl.TargetSpec{Kind: "ConfigMap"}, docs, nil)
		if doc != nil || diag == "" {
			t.Fatalf("want nil doc + no-match diagnostic; got doc=%v diag=%q", doc, diag)
		}
	})

	t.Run("assertion On overrides suite target", func(t *testing.T) {
		// suite target picks Deployment; per-assertion On picks Service.
		doc, diag := selectDoc(dsl.Assertion{On: &dsl.TargetSpec{Kind: "Service"}}, &dsl.TargetSpec{Kind: "Deployment"}, docs, nil)
		if diag != "" || doc["kind"] != "Service" {
			t.Fatalf("want On override to Service; got doc=%v diag=%q", doc, diag)
		}
	})

	t.Run("document-less matchers select nothing", func(t *testing.T) {
		// failedTemplate and hasDocuments operate on the whole result set, so
		// selectDoc must return (nil, "") without attempting doc selection.
		for _, a := range []dsl.Assertion{
			{FailedTemplate: &dsl.FailedTemplateSpec{}},
			{HasDocuments: intPtr(2)},
		} {
			doc, diag := selectDoc(a, &dsl.TargetSpec{Kind: "ConfigMap"}, docs, nil)
			if doc != nil || diag != "" {
				t.Fatalf("want (nil, \"\") for document-less matcher; got doc=%v diag=%q", doc, diag)
			}
		}
	})
}

func TestSkipDecision(t *testing.T) {
	tests := []struct {
		name       string
		skip       any
		wantSkip   bool
		wantReason string
	}{
		{"bool true", true, true, ""},
		{"bool false", false, false, ""},
		{"non-empty string skips with reason", "not ready", true, "not ready"},
		{"empty string does not skip", "", false, ""},
		{"nil does not skip", nil, false, ""},
		{"unsupported type does not skip", 42, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSkip, gotReason := skipDecision(tt.skip)
			if gotSkip != tt.wantSkip || gotReason != tt.wantReason {
				t.Errorf("skipDecision(%v) = (%v, %q), want (%v, %q)", tt.skip, gotSkip, gotReason, tt.wantSkip, tt.wantReason)
			}
		})
	}
}
