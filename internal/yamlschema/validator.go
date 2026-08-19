// Package yamlschema validates YAML documents against a JSON Schema
// (draft 2020-12), reporting violations at the key that caused them.
//
// It backs both schema-checked file formats: test files (internal/dsl) and
// `.vigie.yaml` (internal/config).
package yamlschema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/kaptinlin/jsonschema"
	"gopkg.in/yaml.v3"
)

// Validator compiles a JSON Schema once, on first use, and validates YAML
// documents against it. Safe for concurrent use — the runner validates test
// files in parallel.
type Validator struct {
	raw []byte

	once     sync.Once
	compiled *jsonschema.Schema
	compErr  error

	emptyKeysAsAbsent bool
}

// Option configures a Validator.
type Option func(*Validator)

// EmptyKeysAsAbsent treats a key written with no value (`cluster:` on its own
// line) as unset instead of as an explicit null, so a block whose sub-keys are
// all commented out still validates. Without it such a key fails against an
// object-typed property.
func EmptyKeysAsAbsent() Option {
	return func(v *Validator) { v.emptyKeysAsAbsent = true }
}

// New returns a Validator for the given JSON Schema. Compilation is deferred to
// the first Validate call, so constructing one at package scope is free.
func New(schemaJSON []byte, opts ...Option) *Validator {
	v := &Validator{raw: schemaJSON}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

func (v *Validator) schema() (*jsonschema.Schema, error) {
	v.once.Do(func() {
		v.compiled, v.compErr = jsonschema.NewCompiler().Compile(v.raw)
		if v.compErr != nil {
			v.compErr = fmt.Errorf("compiling schema: %w", v.compErr)
		}
	})
	return v.compiled, v.compErr
}

// Validate checks raw YAML bytes against the schema.
func (v *Validator) Validate(rawYAML []byte) error {
	schema, err := v.schema()
	if err != nil {
		return err
	}

	// YAML → JSON for the validator.
	var doc any
	if err := yaml.Unmarshal(rawYAML, &doc); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	normalized := normalizeForJSON(doc)
	if v.emptyKeysAsAbsent {
		normalized = pruneNulls(normalized)
	}
	jsonBytes, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("converting to JSON: %w", err)
	}

	result := schema.ValidateJSON(jsonBytes)
	if result.IsValid() {
		return nil
	}

	// DetailedErrors drills into the failing leaf and keys each message by its
	// instance location (a JSON pointer like /tests/0/asserts/0/eqaul), so a
	// mistyped key points at the exact offending field instead of a vague
	// top-level "does not match" rollup.
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

// pruneNulls drops mapping entries whose value is nil, recursively. List
// elements keep their nils: only a key with nothing after it means "unset".
func pruneNulls(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, v2 := range val {
			if v2 == nil {
				continue
			}
			out[k] = pruneNulls(v2)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, v2 := range val {
			out[i] = pruneNulls(v2)
		}
		return out
	default:
		return v
	}
}
