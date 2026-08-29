package dsl

import (
	"fmt"
	"strings"

	"github.com/invopop/jsonschema"
)

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
	annotateMatcherTiers(schema)
}

// annotateMatcherTiers records what each cluster-tier matcher needs on its own
// schema property, so an editor answers "can I use this here, and if not what
// do I run?" on hover instead of the user finding out from a skipped run.
// Matchers that work everywhere are left alone - there is nothing to warn about.
func annotateMatcherTiers(schema *jsonschema.Schema) {
	everywhere := len(AllTiers())
	for name, needs := range matcherCapabilities {
		tiers := TiersProviding(needs)
		if len(tiers) == everywhere || len(tiers) == 0 {
			continue
		}
		prop, ok := schema.Properties.Get(name)
		if !ok {
			continue
		}
		floor := tiers[0]
		prop.Extras = map[string]any{"x-vigie-capabilities": needs}
		prop.Description = strings.TrimSpace(prop.Description + fmt.Sprintf(
			"\n\nNeeds %s: run with --cluster %s.",
			joinCapabilities(needs), strings.Join(BackendsForTier(floor), "|")))
	}
}

func joinCapabilities(needs []Capability) string {
	out := make([]string, len(needs))
	for i, c := range needs {
		out[i] = string(c)
	}
	return strings.Join(out, " + ")
}

// JSONSchemaExtend publishes what each tier provides, so a reader of the schema
// can resolve a matcher's `x-vigie-capabilities` to a tier without knowing
// vigie's internals. Only tiers a `--cluster` value can reach are listed.
func (Suite) JSONSchemaExtend(schema *jsonschema.Schema) {
	tiers := make(map[string][]Capability)
	for _, tier := range RunnableTiers() {
		tiers[tier] = TierCapabilities(tier)
	}
	schema.Extras = map[string]any{"x-vigie-tiers": tiers}

	// Constrain `tier:` to what a --cluster value can actually reach, so editors
	// never offer a tier whose backend does not exist.
	if schema.Properties == nil {
		return
	}
	if prop, ok := schema.Properties.Get("tier"); ok {
		for _, tier := range RunnableTiers() {
			prop.Enum = append(prop.Enum, tier)
		}
	}
}
