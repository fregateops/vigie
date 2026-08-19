package yamlschema

import (
	"strings"
	"sync"
	"testing"
)

// objectSchema requires `block` to be an object when present, which is what a
// null value collides with.
const objectSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "block": {
      "type": "object",
      "additionalProperties": false,
      "properties": {"key": {"type": "string"}}
    }
  }
}`

func TestValidate_ReportsTheOffendingKey(t *testing.T) {
	err := New([]byte(objectSchema)).Validate([]byte("block:\n  ky: value\n"))
	if err == nil {
		t.Fatal("Validate must reject an unknown property, got nil error")
	}
	if !strings.Contains(err.Error(), "/block") {
		t.Errorf("error %q does not point at the offending object", err)
	}
}

func TestValidate_EmptyKeyIsNullByDefault(t *testing.T) {
	err := New([]byte(objectSchema)).Validate([]byte("block:\n"))
	if err == nil {
		t.Fatal("without EmptyKeysAsAbsent, a null block must fail an object-typed property")
	}
}

func TestValidate_EmptyKeysAsAbsentSkipsNullBlocks(t *testing.T) {
	v := New([]byte(objectSchema), EmptyKeysAsAbsent())
	if err := v.Validate([]byte("block:\n")); err != nil {
		t.Fatalf("EmptyKeysAsAbsent must treat `block:` as unset, got: %v", err)
	}
	// Pruning must not lose real violations nested under a populated block.
	if err := v.Validate([]byte("block:\n  key: 1\n")); err == nil {
		t.Fatal("pruning nulls must not mask a wrong-typed value")
	}
}

func TestValidate_InvalidYAMLIsReportedAsSuch(t *testing.T) {
	err := New([]byte(objectSchema)).Validate([]byte("block: [unclosed\n"))
	if err == nil || !strings.Contains(err.Error(), "invalid YAML") {
		t.Fatalf("want an invalid-YAML error, got: %v", err)
	}
}

// The runner validates test files in parallel, so a Validator shared at package
// scope must compile its schema exactly once without racing (run under -race).
func TestValidate_IsSafeForConcurrentUse(t *testing.T) {
	v := New([]byte(objectSchema))
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := v.Validate([]byte("block:\n  key: value\n")); err != nil {
				t.Errorf("Validate: %v", err)
			}
		}()
	}
	wg.Wait()
}
