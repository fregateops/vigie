package render

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"

	"helm.sh/helm/v3/pkg/strvals"
	"sigs.k8s.io/yaml"
)

// ValueInputs are a render's value sources, applied in the order the helm CLI
// applies `-f`, `--set-json`, `--set` and `--set-literal`.
type ValueInputs struct {
	// Files are values files, deep-merged in order (`helm -f`).
	Files []string
	// SetJSON values are JSON text (a string) or an already-structured value.
	SetJSON map[string]any
	// Set values keep their YAML type: a list stays a list, "true" stays a string.
	Set        map[string]any
	SetLiteral map[string]string
}

// BuildValues merges the inputs into the user-supplied values map helm expects.
func BuildValues(in ValueInputs) (map[string]any, error) {
	vals := map[string]any{}
	for _, file := range in.Files {
		loaded, err := LoadValuesFile(file)
		if err != nil {
			return nil, err
		}
		vals = MergeValues(vals, loaded)
	}
	for _, key := range slices.Sorted(maps.Keys(in.SetJSON)) {
		text, err := jsonText(in.SetJSON[key])
		if err != nil {
			return nil, fmt.Errorf("setJson %s: %w", key, err)
		}
		if err := strvals.ParseJSON(key+"="+text, vals); err != nil {
			return nil, fmt.Errorf("setJson %s: %w", key, err)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(in.Set)) {
		text, err := json.Marshal(in.Set[key])
		if err != nil {
			return nil, fmt.Errorf("set %s: %w", key, err)
		}
		if err := strvals.ParseJSON(key+"="+string(text), vals); err != nil {
			return nil, fmt.Errorf("set %s: %w", key, err)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(in.SetLiteral)) {
		if err := strvals.ParseLiteralInto(key+"="+in.SetLiteral[key], vals); err != nil {
			return nil, fmt.Errorf("setLiteral %s: %w", key, err)
		}
	}
	return vals, nil
}

func jsonText(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	b, err := json.Marshal(value)
	return string(b), err
}

// LoadValuesFile reads a YAML values file the way `helm -f` does.
func LoadValuesFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading values file %s: %w", path, err)
	}
	m := map[string]any{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing values file %s: %w", path, err)
	}
	return m, nil
}

// MergeValues deep-merges src onto dst (src wins), as `helm -f a -f b` does.
func MergeValues(dst, src map[string]any) map[string]any {
	out := make(map[string]any, len(dst)+len(src))
	maps.Copy(out, dst)
	for k, v := range src {
		if srcMap, ok := v.(map[string]any); ok {
			if dstMap, ok := out[k].(map[string]any); ok {
				out[k] = MergeValues(dstMap, srcMap)
				continue
			}
		}
		out[k] = v
	}
	return out
}
