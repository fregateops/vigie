package matchers

import (
	"fmt"
	"math"
	"reflect"

	"github.com/fregateops/vigie/internal/dsl"
)

func init() {
	Register(simpleMatcher{
		name:     "isNull",
		matches:  func(a dsl.Assertion) bool { return a.IsNull != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalIsNull(a.IsNull, ctx) },
	})
	Register(simpleMatcher{
		name:     "isNotNull",
		matches:  func(a dsl.Assertion) bool { return a.IsNotNull != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalIsNotNull(a.IsNotNull, ctx) },
	})
	Register(simpleMatcher{
		name:     "isEmpty",
		matches:  func(a dsl.Assertion) bool { return a.IsEmpty != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalIsEmpty(a.IsEmpty, ctx) },
	})
	Register(simpleMatcher{
		name:     "isNotEmpty",
		matches:  func(a dsl.Assertion) bool { return a.IsNotEmpty != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalIsNotEmpty(a.IsNotEmpty, ctx) },
	})
	Register(simpleMatcher{
		name:     "isType",
		matches:  func(a dsl.Assertion) bool { return a.IsType != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalIsType(a.IsType, ctx) },
	})
	Register(simpleMatcher{
		name:     "lengthEqual",
		matches:  func(a dsl.Assertion) bool { return a.LengthEqual != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalLengthEqual(a.LengthEqual, ctx) },
	})
	Register(simpleMatcher{
		name:     "isSubset",
		matches:  func(a dsl.Assertion) bool { return a.IsSubset != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalIsSubset(a.IsSubset, ctx) },
	})
}

func evalIsNull(spec *dsl.PathOnly, ctx EvalContext) Result {
	val, err := resolvePathRequired(ctx, spec.Path, "isNull")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	if val == nil {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("isNull: path %q: expected null, got %v", spec.Path, val),
	}
}

func evalIsNotNull(spec *dsl.PathOnly, ctx EvalContext) Result {
	val, err := resolvePathRequired(ctx, spec.Path, "isNotNull")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	if val != nil {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("isNotNull: path %q: expected non-null but got null", spec.Path),
	}
}

func evalIsEmpty(spec *dsl.PathOnly, ctx EvalContext) Result {
	val, err := resolvePathRequired(ctx, spec.Path, "isEmpty")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	if isEmpty(val) {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("isEmpty: path %q: value %v is not empty", spec.Path, val),
	}
}

func evalIsNotEmpty(spec *dsl.PathOnly, ctx EvalContext) Result {
	val, err := resolvePathRequired(ctx, spec.Path, "isNotEmpty")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	if !isEmpty(val) {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("isNotEmpty: path %q: value is empty but should not be", spec.Path),
	}
}

func evalIsType(spec *dsl.IsTypeSpec, ctx EvalContext) Result {
	val, err := resolvePathRequired(ctx, spec.Path, "isType")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	if matchesType(val, spec.Of) {
		return Result{Pass: true}
	}
	return Result{
		Pass:    false,
		Message: fmt.Sprintf("isType: path %q: expected type %q, got %T (%v)", spec.Path, spec.Of, val, val),
	}
}

func evalLengthEqual(spec *dsl.LengthEqualSpec, ctx EvalContext) Result {
	val, err := resolvePathRequired(ctx, spec.Path, "lengthEqual")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
		got := rv.Len()
		if got == spec.Value {
			return Result{Pass: true}
		}
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("lengthEqual: path %q: expected length %d, got %d", spec.Path, spec.Value, got),
		}
	default:
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("lengthEqual: path %q: value of type %T has no length", spec.Path, val),
		}
	}
}

func evalIsSubset(spec *dsl.PathContent, ctx EvalContext) Result {
	val, err := resolvePathRequired(ctx, spec.Path, "isSubset")
	if err != nil {
		return Result{Pass: false, Message: err.Error()}
	}
	target, ok := val.(map[string]any)
	if !ok {
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("isSubset: path %q: value is not a map (got %T)", spec.Path, val),
		}
	}
	subMap, ok := spec.Content.(map[string]any)
	if !ok {
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("isSubset: content must be a map (got %T)", spec.Content),
		}
	}
	for k, wantVal := range subMap {
		gotVal, exists := target[k]
		if !exists {
			return Result{
				Pass:    false,
				Message: fmt.Sprintf("isSubset: path %q: key %q not found in target map", spec.Path, k),
			}
		}
		if !deepEqual(gotVal, wantVal) {
			return Result{
				Pass:    false,
				Message: fmt.Sprintf("isSubset: path %q: key %q: expected %v, got %v", spec.Path, k, wantVal, gotVal),
			}
		}
	}
	return Result{Pass: true}
}

// resolvePathRequired resolves path in doc; returns an error if doc is nil or path not found.
func resolvePathRequired(ctx EvalContext, p string, matcherName string) (any, error) {
	if ctx.Doc == nil {
		return nil, fmt.Errorf("no document selected")
	}
	val, found, err := resolvePathInDoc(ctx.Doc, p)
	if err != nil {
		return nil, fmt.Errorf("path error: %v", err)
	}
	if !found {
		return nil, fmt.Errorf("%s: path %q not found", matcherName, p)
	}
	return val, nil
}

// isEmpty returns true if v is nil, empty string, empty slice/array, or empty map.
func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() == 0
	}
	return false
}

// matchesType checks whether v matches the DSL type name.
func matchesType(v any, typeName string) bool {
	if v == nil {
		return false
	}
	rv := reflect.ValueOf(v)
	switch typeName {
	case "string":
		return rv.Kind() == reflect.String
	case "int":
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return true
		case reflect.Float32, reflect.Float64:
			// YAML often decodes integers as float64; treat whole numbers as int.
			f := rv.Float()
			return f == math.Trunc(f)
		}
		return false
	case "float":
		return rv.Kind() == reflect.Float32 || rv.Kind() == reflect.Float64
	case "bool":
		return rv.Kind() == reflect.Bool
	case "list":
		return rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array
	case "map":
		return rv.Kind() == reflect.Map
	}
	return false
}
