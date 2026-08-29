package dsl

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseFile reads, validates, and parses a test YAML file.
func ParseFile(path string) (*Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return Parse(data, path)
}

// Parse validates rawYAML against the JSON Schema and unmarshals it into a Suite.
func Parse(rawYAML []byte, sourcePath string) (*Suite, error) {
	if err := Validate(rawYAML); err != nil {
		return nil, fmt.Errorf("%s: %w", sourcePath, err)
	}

	var suite Suite
	if err := yaml.Unmarshal(rawYAML, &suite); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", sourcePath, err)
	}
	if err := ValidateTierCeiling(&suite); err != nil {
		return nil, fmt.Errorf("%s: %w", sourcePath, err)
	}
	return &suite, nil
}

// MergeInputs merges suite-level defaults into each test where fields are unset.
func MergeInputs(suite *Suite) {
	if suite.Defaults == nil {
		return
	}
	d := suite.Defaults
	for i := range suite.Tests {
		t := &suite.Tests[i]
		if t.Inputs == nil {
			t.Inputs = &Inputs{}
		}
		if d.Release != nil && t.Inputs.Release == nil {
			t.Inputs.Release = d.Release
		}
		if d.Values != nil && len(t.Inputs.Values) == 0 {
			t.Inputs.Values = d.Values
		}
		if d.Set != nil && t.Inputs.Set == nil {
			t.Inputs.Set = d.Set
		}
		if d.Capabilities != nil && t.Inputs.Capabilities == nil {
			t.Inputs.Capabilities = d.Capabilities
		}
	}
}
