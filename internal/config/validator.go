package config

import (
	"github.com/fregateops/vigie/internal/yamlschema"
	schemav1 "github.com/fregateops/vigie/pkg/api/schema/v1"
)

// SchemaJSON returns the embedded `.vigie.yaml` JSON Schema as raw bytes.
func SchemaJSON() []byte {
	return schemav1.ConfigSchema
}

// A config file is hand-maintained and full of commented-out keys, so a block
// left empty by commenting out its contents counts as unset rather than as a
// null that fails the schema.
var validator = yamlschema.New(schemav1.ConfigSchema, yamlschema.EmptyKeysAsAbsent())

// Validate checks raw `.vigie.yaml` bytes against the config JSON Schema
// (draft 2020-12).
func Validate(rawYAML []byte) error {
	return validator.Validate(rawYAML)
}
