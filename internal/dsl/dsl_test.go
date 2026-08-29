package dsl

import (
	"strings"
	"testing"
)

const minimalValidSuite = `
suite: example
tests:
  - it: renders something
`

func TestValidate_MinimalValid(t *testing.T) {
	if err := Validate([]byte(minimalValidSuite)); err != nil {
		t.Fatalf("minimal valid suite should pass schema validation: %v", err)
	}
}

func TestValidate_RejectsMissingRequired(t *testing.T) {
	// A test without the required `it` field must fail validation.
	doc := "suite: example\ntests:\n  - tier: template\n"
	if err := Validate([]byte(doc)); err == nil {
		t.Fatal("expected schema validation error for a test missing `it`, got nil")
	}
}

func TestValidate_RejectsUnknownKey(t *testing.T) {
	// additionalProperties:false — an unknown top-level key must fail.
	doc := minimalValidSuite + "bogusTopLevel: nope\n"
	if err := Validate([]byte(doc)); err == nil {
		t.Fatal("expected schema validation error for unknown top-level key, got nil")
	}
}

func TestParse_MinimalValid(t *testing.T) {
	suite, err := Parse([]byte(minimalValidSuite), "inline")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if suite.SuiteName != "example" {
		t.Errorf("SuiteName = %q, want %q", suite.SuiteName, "example")
	}
	if len(suite.Tests) != 1 || suite.Tests[0].It != "renders something" {
		t.Errorf("unexpected tests: %+v", suite.Tests)
	}
}

func TestParse_InvalidFailsBeforeUnmarshal(t *testing.T) {
	_, err := Parse([]byte("suite: example\ntests:\n  - tier: template\n"), "inline")
	if err == nil {
		t.Fatal("expected Parse to reject a schema-invalid suite")
	}
	if !strings.Contains(err.Error(), "inline") {
		t.Errorf("error should be tagged with the source path: %v", err)
	}
}

// A suite's tier defaults to the strictest one, so an undeclared file is held
// to render-only matchers rather than quietly allowed anything.
func TestSuiteTier_DefaultsToTemplate(t *testing.T) {
	if got := (&Suite{}).SuiteTier(); got != TierTemplate {
		t.Errorf("SuiteTier() on an undeclared suite = %q, want %q", got, TierTemplate)
	}
	if got := (&Suite{Tier: TierE2E}).SuiteTier(); got != TierE2E {
		t.Errorf("SuiteTier() = %q, want %q", got, TierE2E)
	}
}

func TestMergeInputs_FillsFromDefaults(t *testing.T) {
	suite := &Suite{
		Defaults: &Inputs{
			Release: &ReleaseInputs{Name: "from-defaults"},
			Values:  []string{"values-dev.yaml"},
		},
		Tests: []Test{
			{It: "inherits defaults"},
			{It: "keeps its own", Inputs: &Inputs{Release: &ReleaseInputs{Name: "own"}}},
		},
	}
	MergeInputs(suite)

	if got := suite.Tests[0].Inputs; got == nil || got.Release == nil || got.Release.Name != "from-defaults" {
		t.Errorf("test 0 should inherit the default release, got %+v", got)
	}
	if suite.Tests[0].Inputs == nil || len(suite.Tests[0].Inputs.Values) != 1 {
		t.Errorf("test 0 should inherit default values, got %+v", suite.Tests[0].Inputs)
	}
	if suite.Tests[1].Inputs.Release.Name != "own" {
		t.Errorf("test 1 should keep its own release, got %q", suite.Tests[1].Inputs.Release.Name)
	}
}

func TestValidate_ErrorNamesOffendingKeyAndLocation(t *testing.T) {
	// A mistyped matcher (`eqaul`) must produce an error that names the key and
	// points at its instance location, not a vague top-level rollup.
	doc := `
suite: example
tests:
  - it: typo
    asserts:
      - eqaul:
          path: kind
          value: ConfigMap
`
	err := Validate([]byte(doc))
	if err == nil {
		t.Fatal("expected schema validation error for a mistyped matcher, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "eqaul") {
		t.Errorf("error should name the offending key 'eqaul'; got:\n%s", msg)
	}
	if !strings.Contains(msg, "/tests/0/asserts/0") {
		t.Errorf("error should point at the assertion's instance location; got:\n%s", msg)
	}
	// The noisy structural rollups must be filtered out.
	if strings.Contains(msg, "Property 'tests' does not match") {
		t.Errorf("error should not include the top-level rollup line; got:\n%s", msg)
	}
}
