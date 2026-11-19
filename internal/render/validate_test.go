package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const configMapSchema = `{
  "type": "object",
  "properties": {
    "apiVersion": {"type": "string"},
    "kind": {"type": "string"},
    "metadata": {"type": "object"},
    "data": {"type": "object", "additionalProperties": {"type": "string"}}
  }
}`

func localSchemas(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	strictSchema := strings.Replace(configMapSchema, `"type": "object",`, `"type": "object", "additionalProperties": false,`, 1)
	for name, schema := range map[string]string{"configmap-v1.json": configMapSchema, "configmap-v1-strict.json": strictSchema} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(schema), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func configMap(extra map[string]any) map[string]any {
	doc := map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "cm"}}
	for k, v := range extra {
		doc[k] = v
	}
	return doc
}

func validatorFor(t *testing.T, opts SchemaOptions) *SchemaValidator {
	t.Helper()
	v, err := NewSchemaValidator(opts)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

const localLocation = "{{ .ResourceKind }}{{ .KindSuffix }}{{ .StrictSuffix }}.json"

func TestValidate_UnknownFieldsFailByDefault(t *testing.T) {
	loc := filepath.Join(localSchemas(t), localLocation)
	doc := configMap(map[string]any{"dta": map[string]any{"a": "b"}})

	report, err := validatorFor(t, SchemaOptions{Locations: []string{loc}}).Validate([]map[string]any{doc})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Errors) != 1 {
		t.Fatalf("strict: want the misspelled field rejected, got %+v", report)
	}

	report, err = validatorFor(t, SchemaOptions{Locations: []string{loc}, AllowUnknownFields: true}).Validate([]map[string]any{doc})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("allowUnknownFields: want no error, got %+v", report.Errors)
	}
}

func TestValidate_MissingSchemaIsReportedAsUnchecked(t *testing.T) {
	loc := filepath.Join(localSchemas(t), localLocation)
	crd := map[string]any{"apiVersion": "monitoring.coreos.com/v1", "kind": "ServiceMonitor", "metadata": map[string]any{"name": "sm"}}

	report, err := validatorFor(t, SchemaOptions{Locations: []string{loc}}).Validate([]map[string]any{crd, crd, configMap(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Errors) != 0 || len(report.UncheckedKinds) != 1 || report.UncheckedKinds[0] != "ServiceMonitor" {
		t.Fatalf("want ServiceMonitor reported once as unchecked, got %+v", report)
	}
}

func TestValidate_UnreachableSchemaFails(t *testing.T) {
	unreachable := "http://127.0.0.1:9/{{ .ResourceKind }}.json"
	report, err := validatorFor(t, SchemaOptions{Locations: []string{unreachable}}).Validate([]map[string]any{configMap(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Errors) != 1 || !strings.Contains(report.Errors[0].Message, "could not be validated") {
		t.Fatalf("want a fetch failure to fail validation, got %+v", report)
	}
}

func TestResolveSchemaLocation(t *testing.T) {
	cases := map[string]string{
		"default":                     "default",
		"https://example.com/x.json":  "https://example.com/x.json",
		"/abs/dir":                    "/abs/dir",
		"schemas/{{ .ResourceKind }}": "/chart/schemas/{{ .ResourceKind }}",
	}
	for in, want := range cases {
		if got := resolveSchemaLocation(in, "/chart"); got != want {
			t.Errorf("resolveSchemaLocation(%q) = %q, want %q", in, got, want)
		}
	}
}
