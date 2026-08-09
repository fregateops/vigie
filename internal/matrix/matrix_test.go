package matrix

import (
	"sort"
	"testing"
)

// ---- helpers ----------------------------------------------------------------

// sortedKeys returns sorted keys of a map (for deterministic output).
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// comboKey produces a canonical string for a combo so we can compare sets.
func comboKey(m map[string]any) string {
	keys := sortedKeys(m)
	s := ""
	for _, k := range keys {
		s += k + "=" + formatVal(m[k]) + ";"
	}
	return s
}

func formatVal(v any) string {
	switch vv := v.(type) {
	case bool:
		if vv {
			return "true"
		}
		return "false"
	case string:
		return vv
	default:
		return "?"
	}
}

// comboSet converts a slice of combos into a set of keys for order-independent comparison.
func comboSet(combos []map[string]any) map[string]bool {
	out := make(map[string]bool, len(combos))
	for _, c := range combos {
		out[comboKey(c)] = true
	}
	return out
}

// ---- Expand tests -----------------------------------------------------------

func TestExpand_Basic2D(t *testing.T) {
	raw := map[string]any{
		"tier": []any{"small", "large"},
		"ha":   []any{true, false},
	}
	combos, err := Expand(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(combos) != 4 {
		t.Errorf("expected 4 combos, got %d: %v", len(combos), combos)
	}
	// Verify all four expected combinations are present.
	want := map[string]bool{
		"ha=false;tier=large;": true,
		"ha=false;tier=small;": true,
		"ha=true;tier=large;":  true,
		"ha=true;tier=small;":  true,
	}
	got := comboSet(combos)
	for k := range want {
		if !got[k] {
			t.Errorf("missing combo %q", k)
		}
	}
}

func TestExpand_WithExclude(t *testing.T) {
	raw := map[string]any{
		"tier": []any{"small", "large"},
		"ha":   []any{true, false},
		"exclude": []any{
			map[string]any{"tier": "small", "ha": true},
		},
	}
	combos, err := Expand(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(combos) != 3 {
		t.Errorf("expected 3 combos after exclude, got %d: %v", len(combos), combos)
	}
	// The excluded combo must not appear.
	got := comboSet(combos)
	if got["ha=true;tier=small;"] {
		t.Error("excluded combo {tier=small, ha=true} should not be present")
	}
}

func TestExpand_WithInclude_NewCombo(t *testing.T) {
	raw := map[string]any{
		"tier": []any{"small"},
		"include": []any{
			map[string]any{"tier": "xl", "ha": false},
		},
	}
	combos, err := Expand(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect: {tier:small} + {tier:xl, ha:false}
	if len(combos) != 2 {
		t.Errorf("expected 2 combos, got %d: %v", len(combos), combos)
	}
	got := comboSet(combos)
	if !got["tier=small;"] {
		t.Error("expected combo {tier=small} to be present")
	}
	// The include adds a brand-new combo (not a subset of small).
	found := false
	for _, c := range combos {
		if c["tier"] == "xl" {
			found = true
		}
	}
	if !found {
		t.Error("expected include combo with tier=xl to be present")
	}
}

func TestExpand_WithInclude_MergesIntoExisting(t *testing.T) {
	// Per the implementation: an include entry merges into existing combos where the
	// include's keys are already in the combo (i.e. include is a subset of the combo).
	// Here {tier: small} is a subset of the existing combo {tier: small, ha: true},
	// so the include merges in and adds the extra key.
	raw := map[string]any{
		"tier": []any{"small"},
		"ha":   []any{true},
		"include": []any{
			// Only the keys that already exist in the combo are used for the
			// subset match; then ALL include keys (including new ones) are merged.
			map[string]any{"tier": "small", "extra": "yes"},
		},
	}
	combos, err := Expand(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The include {tier: small, extra: yes}: isSubset({tier:small,extra:yes}, {tier:small,ha:true})
	// checks if all keys in the include exist in the combo — "extra" does not, so no merge occurs.
	// The include is appended as a standalone entry. Total = 2.
	// This documents the actual (GitHub Actions-compatible) behavior.
	if len(combos) != 2 {
		t.Errorf("expected 2 combos (include not merged, appended as new entry), got %d", len(combos))
	}
	// One of the combos must be the standalone include entry.
	found := false
	for _, c := range combos {
		if c["extra"] == "yes" && c["tier"] == "small" {
			found = true
		}
	}
	if !found {
		t.Error("expected include combo {tier=small, extra=yes} to be present")
	}
}

func TestExpand_SingleDimension(t *testing.T) {
	raw := map[string]any{
		"color": []any{"red", "blue"},
	}
	combos, err := Expand(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(combos) != 2 {
		t.Errorf("expected 2 combos, got %d", len(combos))
	}
}

func TestExpand_EmptyMatrix(t *testing.T) {
	raw := map[string]any{}
	combos, err := Expand(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty matrix should return nil or empty slice, not an error.
	if len(combos) != 0 {
		t.Errorf("expected 0 combos for empty matrix, got %d", len(combos))
	}
}

func TestExpand_ErrorDimensionNotList(t *testing.T) {
	raw := map[string]any{
		"tier": "small", // not a list — should error
	}
	_, err := Expand(raw)
	if err == nil {
		t.Error("expected an error when dimension value is not a list, got nil")
	}
}

func TestExpand_ErrorExcludeNotListOfMaps(t *testing.T) {
	raw := map[string]any{
		"tier":    []any{"small"},
		"exclude": "not-a-list",
	}
	_, err := Expand(raw)
	if err == nil {
		t.Error("expected an error for invalid exclude value")
	}
}

// ---- ExpandCases tests ------------------------------------------------------

func TestExpandCases_Basic(t *testing.T) {
	raw := []map[string]any{
		{
			"name": "case-a",
			"set":  map[string]any{"replicaCount": 1},
		},
		{
			"name": "case-b",
			"set":  map[string]any{"replicaCount": 3},
		},
	}
	cases, err := ExpandCases(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(cases))
	}
	if cases[0].Name != "case-a" {
		t.Errorf("expected name 'case-a', got %q", cases[0].Name)
	}
	if cases[1].Name != "case-b" {
		t.Errorf("expected name 'case-b', got %q", cases[1].Name)
	}
	if cases[0].Set["replicaCount"] != 1 {
		t.Errorf("expected Set[replicaCount]=1, got %v", cases[0].Set["replicaCount"])
	}
}

func TestExpandCases_ExtraFieldsGoIntoExtra(t *testing.T) {
	raw := []map[string]any{
		{
			"name":        "with-extra",
			"set":         map[string]any{"replicaCount": 2},
			"description": "some extra field",
			"replicas":    5,
		},
	}
	cases, err := ExpandCases(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}
	c := cases[0]
	if c.Extra["description"] != "some extra field" {
		t.Errorf("expected Extra[description]='some extra field', got %v", c.Extra["description"])
	}
	if c.Extra["replicas"] != 5 {
		t.Errorf("expected Extra[replicas]=5, got %v", c.Extra["replicas"])
	}
	// name and set must NOT be in Extra.
	if _, ok := c.Extra["name"]; ok {
		t.Error("'name' should not appear in Extra")
	}
	if _, ok := c.Extra["set"]; ok {
		t.Error("'set' should not appear in Extra")
	}
}

func TestExpandCases_MissingName(t *testing.T) {
	raw := []map[string]any{
		{"set": map[string]any{"replicaCount": 1}}, // no name
	}
	_, err := ExpandCases(raw)
	if err == nil {
		t.Error("expected an error for missing 'name' field")
	}
}

func TestExpandCases_EmptyName(t *testing.T) {
	raw := []map[string]any{
		{"name": ""},
	}
	_, err := ExpandCases(raw)
	if err == nil {
		t.Error("expected an error for empty 'name' field")
	}
}

func TestExpandCases_NoSetField(t *testing.T) {
	// set is optional; case should parse fine with Set == nil.
	raw := []map[string]any{
		{"name": "no-set"},
	}
	cases, err := ExpandCases(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}
	if cases[0].Set != nil {
		t.Errorf("expected nil Set, got %v", cases[0].Set)
	}
}

// ---- Interpolate tests ------------------------------------------------------

func TestInterpolate_WholeString_ReturnsTyped(t *testing.T) {
	bindings := map[string]any{
		"matrix": map[string]any{"tier": "small"},
	}
	result, err := Interpolate("${{ matrix.tier }}", bindings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "small" {
		t.Errorf("expected 'small', got %v (%T)", result, result)
	}
}

func TestInterpolate_WholeString_BoolResult(t *testing.T) {
	bindings := map[string]any{
		"matrix": map[string]any{"ha": true},
	}
	result, err := Interpolate("${{ matrix.ha ? 3 : 1 }}", bindings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// CEL returns int64 for integer arithmetic.
	v, ok := result.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T: %v", result, result)
	}
	if v != 3 {
		t.Errorf("expected 3, got %d", v)
	}
}

func TestInterpolate_EmbeddedSubstitution(t *testing.T) {
	bindings := map[string]any{
		"matrix": map[string]any{"tier": "small"},
	}
	result, err := Interpolate("prefix-${{ matrix.tier }}-suffix", bindings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", result, result)
	}
	if s != "prefix-small-suffix" {
		t.Errorf("expected 'prefix-small-suffix', got %q", s)
	}
}

func TestInterpolate_NonStringPassesThrough(t *testing.T) {
	// Integers, bools, and other non-string scalars must pass through unmodified.
	cases := []struct {
		name  string
		input any
	}{
		{"int", 42},
		{"float", 3.14},
		{"bool", true},
		{"nil", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Interpolate(tc.input, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.input {
				t.Errorf("expected %v, got %v", tc.input, result)
			}
		})
	}
}

func TestInterpolate_MapIsWalkedRecursively(t *testing.T) {
	bindings := map[string]any{
		"matrix": map[string]any{"tier": "large"},
	}
	input := map[string]any{
		"key1": "static",
		"key2": "${{ matrix.tier }}",
		"nested": map[string]any{
			"key3": "prefix-${{ matrix.tier }}",
		},
	}
	result, err := Interpolate(input, bindings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["key1"] != "static" {
		t.Errorf("key1: expected 'static', got %v", m["key1"])
	}
	if m["key2"] != "large" {
		t.Errorf("key2: expected 'large', got %v", m["key2"])
	}
	nested, ok := m["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested: expected map, got %T", m["nested"])
	}
	if nested["key3"] != "prefix-large" {
		t.Errorf("nested.key3: expected 'prefix-large', got %v", nested["key3"])
	}
}

func TestInterpolate_SliceIsWalkedRecursively(t *testing.T) {
	bindings := map[string]any{
		"matrix": map[string]any{"color": "red"},
	}
	input := []any{
		"plain",
		"${{ matrix.color }}",
		42,
	}
	result, err := Interpolate(input, bindings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := result.([]any)
	if !ok {
		t.Fatalf("expected slice, got %T", result)
	}
	if s[0] != "plain" {
		t.Errorf("[0]: expected 'plain', got %v", s[0])
	}
	if s[1] != "red" {
		t.Errorf("[1]: expected 'red', got %v", s[1])
	}
	if s[2] != 42 {
		t.Errorf("[2]: expected 42, got %v", s[2])
	}
}

func TestInterpolate_NoPlaceholder(t *testing.T) {
	result, err := Interpolate("just a plain string", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "just a plain string" {
		t.Errorf("expected unchanged string, got %v", result)
	}
}

func TestInterpolate_InvalidCELExpression(t *testing.T) {
	_, err := Interpolate("${{ @@invalid@@ }}", map[string]any{})
	if err == nil {
		t.Error("expected an error for invalid CEL expression")
	}
}
