package dsl

import (
	"github.com/fregateops/vigie/internal/yamlschema"
	schemav1 "github.com/fregateops/vigie/pkg/api/schema/v1"
)

// SchemaJSON returns the embedded test file JSON Schema as raw bytes.
func SchemaJSON() []byte {
	return schemav1.TestFileSchema
}

var validator = yamlschema.New(schemav1.TestFileSchema)

// Validate checks raw YAML bytes against the test file JSON Schema (draft 2020-12).
func Validate(rawYAML []byte) error {
	return validator.Validate(rawYAML)
}
