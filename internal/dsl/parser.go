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
	return &suite, nil
}

// SuiteKind identifies whether a test file is unit or integration format.
type SuiteKind string

const (
	UnitSuiteKind        SuiteKind = "unit"
	IntegrationSuiteKind SuiteKind = "integration"
)

// ParseSuiteAuto reads, validates, and parses a test YAML file, detecting
// whether it is a unit or integration suite from the presence of cluster: or
// dependencies: at the top level.
func ParseSuiteAuto(path string) (*Suite, SuiteKind, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("reading %s: %w", path, err)
	}
	kind := detectSuiteKind(data)
	suite, err := Parse(data, path)
	if err != nil {
		return nil, "", err
	}
	return suite, kind, nil
}

// detectSuiteKind inspects top-level YAML keys to determine suite kind.
func detectSuiteKind(data []byte) SuiteKind {
	var probe struct {
		Cluster      any   `yaml:"cluster"`
		Dependencies []any `yaml:"dependencies"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return UnitSuiteKind
	}
	if probe.Cluster != nil || len(probe.Dependencies) > 0 {
		return IntegrationSuiteKind
	}
	return UnitSuiteKind
}

// DetectKind reads a test file and reports whether it is a unit or integration
// suite based on its top-level keys. Used by recursive test discovery to
// filter files by tier without parsing them in full.
func DetectKind(path string) (SuiteKind, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return detectSuiteKind(data), nil
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
