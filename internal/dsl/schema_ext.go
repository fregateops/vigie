package dsl

import "github.com/invopop/jsonschema"

// JSONSchemaExtend constrains the isType matcher's `of` to the supported set
// of type names. The enum cannot live on the struct field via a tag because
// invopop's enum tag values become JSON strings without quotes — fine here,
// but explicit is clearer.
func (IsTypeSpec) JSONSchemaExtend(schema *jsonschema.Schema) {
	if schema.Properties == nil {
		return
	}
	if ofProp, ok := schema.Properties.Get("of"); ok {
		ofProp.Enum = []any{"string", "int", "float", "bool", "list", "map"}
	}
}

// JSONSchemaExtend forces the applies matcher to be an empty object — it
// carries no fields today and accepts no extra keys.
func (AppliesSpec) JSONSchemaExtend(schema *jsonschema.Schema) {
	zero := uint64(0)
	schema.MaxProperties = &zero
}

// JSONSchemaExtend disallows extra keys on the rejected matcher so that
// typos (e.g. `messsage:`) fail loudly at validation time.
func (RejectedSpec) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AdditionalProperties = jsonschema.FalseSchema
}

// JSONSchemaExtend tightens the `hasDocuments` property on Assertion so a
// negative count fails validation rather than confusing the runner. The
// `Assertion.HasDocuments` field is `*int`, which yields an integer schema;
// we add minimum=0 here because struct-tag minimum on a pointer-to-int isn't
// picked up by invopop's reflector.
func (Assertion) JSONSchemaExtend(schema *jsonschema.Schema) {
	if schema.Properties == nil {
		return
	}
	if prop, ok := schema.Properties.Get("hasDocuments"); ok {
		prop.Minimum = "0"
	}
}
