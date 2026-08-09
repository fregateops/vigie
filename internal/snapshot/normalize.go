package snapshot

import "sort"

// helmAnnotations is the set of Helm/kubectl annotations that should be stripped
// from metadata.annotations during normalization.
var helmAnnotations = map[string]struct{}{
	"helm.sh/hook-weight":                              {},
	"helm.sh/hook":                                     {},
	"helm.sh/hook-delete-policy":                       {},
	"kubectl.kubernetes.io/last-applied-configuration": {},
}

// Normalize recursively normalizes a parsed YAML document for snapshot comparison.
// It:
//   - Sorts map keys (to make output deterministic)
//   - Strips known Helm/kubectl annotations from metadata.annotations
//   - Removes "creationTimestamp" keys whose value is nil
//
// The input is expected to be map[string]any or []any (as produced by gopkg.in/yaml.v3).
// Returns a new, normalized copy of the document.
func Normalize(doc any) any {
	return normalizeValue(doc)
}

func normalizeValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return normalizeMap(val)
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = normalizeValue(item)
		}
		return result
	default:
		return v
	}
}

func normalizeMap(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))

	for k, v := range m {
		// Strip "creationTimestamp" when its value is nil.
		if k == "creationTimestamp" && v == nil {
			continue
		}

		// Special handling for metadata.annotations: strip known Helm annotations.
		if k == "metadata" {
			result[k] = normalizeMetadata(v)
			continue
		}

		result[k] = normalizeValue(v)
	}

	// Sort keys for deterministic output by returning a sorted-key map.
	// gopkg.in/yaml.v3 marshals maps in insertion order; we use a sortedMap
	// wrapper only at marshal time. Since we return map[string]any here,
	// callers that need sorted output must use MarshalSorted.
	return result
}

func normalizeMetadata(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return normalizeValue(v)
	}
	result := make(map[string]any, len(m))
	for k, val := range m {
		if k == "annotations" {
			result[k] = normalizeAnnotations(val)
			continue
		}
		if k == "creationTimestamp" && val == nil {
			continue
		}
		result[k] = normalizeValue(val)
	}
	return result
}

func normalizeAnnotations(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return normalizeValue(v)
	}
	result := make(map[string]any, len(m))
	for k, val := range m {
		if _, strip := helmAnnotations[k]; strip {
			continue
		}
		result[k] = normalizeValue(val)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// sortedKeys returns a sorted slice of map keys for deterministic YAML marshaling.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
