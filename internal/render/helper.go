package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
)

// HelperResult is the output of a helper function invocation.
type HelperResult struct {
	Raw    string // raw rendered string
	Parsed any    // parsed structure (for yaml/json outputAs)
}

// CallHelper renders a named template with an explicit args dict.
// helperFiles: list of template file paths to load (for define blocks).
// name: the named template to call (e.g. "common.resources.preset").
// args: dict passed as the template context.
// outputAs: "string" | "yaml" | "json" | "bool"
func CallHelper(helperFiles []string, name string, args map[string]any, outputAs string) (*HelperResult, error) {
	// Build a synthetic chart with the helper files as templates.
	ch := &chart.Chart{
		Metadata: &chart.Metadata{Name: "helper-test", Version: "0.1.0"},
	}

	for _, f := range helperFiles {
		content, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("callHelper: reading helper file %q: %w", f, err)
		}
		ch.Templates = append(ch.Templates, &chart.File{
			Name: "templates/" + filepath.Base(f),
			Data: content,
		})
	}

	// Add a caller template that invokes the named template with .Values.callArgs.
	// Helm puts user values under .Values in the template render context.
	callerTpl := fmt.Sprintf(`{{- include "%s" .Values.callArgs -}}`, name)
	ch.Templates = append(ch.Templates, &chart.File{
		Name: "templates/caller.tpl",
		Data: []byte(callerTpl),
	})

	// Build render values with callArgs set to args.
	vals := map[string]any{"callArgs": args}
	values, err := chartutil.ToRenderValues(
		ch,
		chartutil.Values(vals),
		chartutil.ReleaseOptions{Name: "test", Namespace: "default"},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("callHelper: building render values: %w", err)
	}

	eng := engine.Engine{}
	rendered, err := eng.Render(ch, values)
	if err != nil {
		return nil, fmt.Errorf("callHelper: rendering: %w", err)
	}

	// Extract the caller template output.
	raw := rendered["helper-test/templates/caller.tpl"]
	raw = strings.TrimSpace(raw)

	result := &HelperResult{Raw: raw}

	// Parse based on outputAs.
	switch outputAs {
	case "", "string":
		result.Parsed = raw

	case "yaml":
		var parsed any
		if err := yaml.Unmarshal([]byte(raw), &parsed); err != nil {
			return nil, fmt.Errorf("callHelper: parsing output as yaml: %w", err)
		}
		result.Parsed = parsed

	case "json":
		var parsed any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return nil, fmt.Errorf("callHelper: parsing output as json: %w", err)
		}
		result.Parsed = parsed

	case "bool":
		result.Parsed = raw != "" && raw != "false"

	default:
		return nil, fmt.Errorf("callHelper: unknown outputAs %q (valid: string, yaml, json, bool)", outputAs)
	}

	return result, nil
}
