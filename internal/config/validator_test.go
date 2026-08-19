package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSchemaJSON_IsAValidSchemaDocument(t *testing.T) {
	var doc struct {
		ID    string `json:"$id"`
		Title string `json:"title"`
		Type  string `json:"type"`
	}
	if err := json.Unmarshal(SchemaJSON(), &doc); err != nil {
		t.Fatalf("embedded config schema is not valid JSON: %v", err)
	}
	// A resolvable $id is what lets an editor fetch the schema from a modeline.
	if !strings.HasSuffix(doc.ID, "/config.json") {
		t.Errorf("$id %q does not point at config.json", doc.ID)
	}
	if doc.Title == "" {
		t.Error("schema has no title")
	}
	if doc.Type != "object" {
		t.Errorf("schema root type: want object, got %q", doc.Type)
	}
}

func TestValidate_RejectsUnknownKeyByName(t *testing.T) {
	// `ruleSet` is a typo for `ruleSets`. The schema names the offending key and
	// the block it sits in, which the strict decoder's message does not.
	err := Validate([]byte("lint:\n  ruleSet:\n    - chart-yaml\n"))
	if err == nil {
		t.Fatal("Validate must reject an unknown key, got nil error")
	}
	for _, want := range []string{"/lint", "ruleSet"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestValidate_RejectsWrongType(t *testing.T) {
	// kubeVersions is a list; a bare scalar is a common mistake.
	err := Validate([]byte("test:\n  kubeVersions: \"1.36.1\"\n"))
	if err == nil {
		t.Fatal("Validate must reject a scalar where a list is expected, got nil error")
	}
	if !strings.Contains(err.Error(), "/test/kubeVersions") {
		t.Errorf("error %q does not point at the offending field", err)
	}
}

func TestValidate_EmptyBlockCountsAsUnset(t *testing.T) {
	// Every sub-key commented out leaves the parent key with a null value. That
	// is how a hand-edited config looks mid-experiment, so it must still load.
	doc := `
test:
  cluster:
    kind:
lint:
`
	if err := Validate([]byte(doc)); err != nil {
		t.Fatalf("Validate on a config with emptied blocks: %v", err)
	}
}

func TestValidate_AcceptsFullySpecifiedConfig(t *testing.T) {
	doc := `
defaults:
  release:
    name: release-name
    namespace: default
lint:
  ruleSets: [chart-yaml]
  disableRules: [chart-yaml_name]
  kubeVersions: ["1.30"]
  ignore:
    - rule: chart-yaml_name
      paths: [templates/*.yaml]
validate:
  valuesFiles: [values-prod.yaml]
  kubeVersions: ["1.36.1"]
  set: [a=b]
  setJson: ['a={"b":1}']
  setLiteral: [a=b]
  ignore:
    - kind: Ingress
      name: my-ingress
      messageRegex: networking
test:
  skipSchema: true
  kubeVersions: ["1.36.1"]
  testsDir: tests/unit
  cluster:
    envtest:
      kubeVersion: "1.36.1"
    kind:
      kubeVersion: "1.36.1"
      binary: /usr/local/bin/kind
      extraArgs: [--config, kind.yaml]
    k3d:
      kubeVersion: "1.36.1"
      binary: /usr/local/bin/k3d
      extraArgs: [-v, /host:/node]
    kubeconfig:
      path: /tmp/kubeconfig.yaml
run:
  applyTiers: [envtest, kind]
`
	if err := Validate([]byte(doc)); err != nil {
		t.Fatalf("Validate on a config exercising every key: %v", err)
	}
}
