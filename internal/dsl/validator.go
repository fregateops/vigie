package dsl

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	schemav1 "github.com/fregateops/vigie/pkg/api/schema/v1"
	"github.com/kaptinlin/jsonschema"
	"gopkg.in/yaml.v3"
)

// SchemaJSON returns the embedded test file JSON Schema as raw bytes.
func SchemaJSON() []byte {
	return schemav1.TestFileSchema
}

var compiledSchema *jsonschema.Schema

func getSchema() (*jsonschema.Schema, error) {
	if compiledSchema != nil {
		return compiledSchema, nil
	}
	var err error
	compiledSchema, err = jsonschema.NewCompiler().Compile(SchemaJSON())
	if err != nil {
		return nil, fmt.Errorf("compiling schema: %w", err)
	}
	return compiledSchema, nil
}

// Validate checks raw YAML bytes against the test file JSON Schema (draft 2020-12).
func Validate(rawYAML []byte) error {
	schema, err := getSchema()
	if err != nil {
		return err
	}

	// YAML → JSON for the validator.
	var doc any
	if err := yaml.Unmarshal(rawYAML, &doc); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	jsonBytes, err := json.Marshal(normalizeForJSON(doc))
	if err != nil {
		return fmt.Errorf("converting to JSON: %w", err)
	}

	result := schema.ValidateJSON(jsonBytes)
	if result.IsValid() {
		return nil
	}

	// DetailedErrors drills into the failing leaf and keys each message by its
	// instance location (a JSON pointer like /tests/0/asserts/0/eqaul), so a
	// mistyped matcher points at the exact offending key instead of a vague
	// top-level "tests does not match" rollup.
	detailed := result.DetailedErrors()
	if len(detailed) > 0 {
		locations := make([]string, 0, len(detailed))
		for loc := range detailed {
			locations = append(locations, loc)
		}
		sort.Strings(locations)
		var msgs []string
		for _, loc := range locations {
			// Skip the structural rollup nodes ($ref/items/properties) that just
			// say "does not match" — keep the concrete leaf violation (e.g. the
			// additionalProperties error that names the offending key).
			if isRollupLocation(loc) {
				continue
			}
			where := loc
			if where == "" {
				where = "(root)"
			}
			msgs = append(msgs, fmt.Sprintf("%s: %s", where, detailed[loc]))
		}
		if len(msgs) > 0 {
			return fmt.Errorf("schema validation errors:\n  %s", strings.Join(msgs, "\n  "))
		}
	}

	// Fallback: the shallow, top-level errors if no leaf detail is available.
	var msgs []string
	for _, e := range result.Errors {
		msgs = append(msgs, e.Error())
	}
	return fmt.Errorf("schema validation errors:\n  %s", strings.Join(msgs, "\n  "))
}

// isRollupLocation reports whether a JSON-pointer instance location ends in a
// structural navigator keyword ($ref/items/properties). Those nodes carry only
// generic "does not match" rollups; the actionable message lives at the leaf.
func isRollupLocation(loc string) bool {
	slash := strings.LastIndexByte(loc, '/')
	last := loc[slash+1:]
	switch last {
	case "$ref", "items", "properties":
		return true
	}
	return false
}

// normalizeForJSON ensures all map keys are strings, as required by JSON marshaling.
func normalizeForJSON(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, v2 := range val {
			out[k] = normalizeForJSON(v2)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, v2 := range val {
			out[i] = normalizeForJSON(v2)
		}
		return out
	default:
		return v
	}
}
