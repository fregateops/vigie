package render

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/strvals"
)

// Request specifies what to render and with which inputs.
type Request struct {
	ChartPath string
	// Values is applied with `helm --set` semantics: dotted keys are expanded
	// into nested structure (e.g. `image.tag` → `{image: {tag: ...}}`).
	Values map[string]any
	// OverlayValues is a pre-parsed YAML overlay applied with `helm -f`
	// semantics: deep-merged onto the chart's `values.yaml` before any --set
	// overrides. Use this for full values files (e.g. `values-prod.yaml`).
	OverlayValues map[string]any
	ReleaseName   string
	Namespace     string
	KubeVersion   string
	APIVersions   []string
	// Templates limits rendering to these file names (relative to chart). Empty = all.
	Templates []string
}

// Result is the output of a render operation.
type Result struct {
	// Docs contains all rendered, non-empty YAML documents in order.
	Docs []map[string]any
	// Files maps template filename → rendered content (for diagnostics).
	Files map[string]string
}

// Render loads a chart and renders it in-process using the Helm engine.
func Render(req Request) (*Result, error) {
	chrt, err := loader.Load(req.ChartPath)
	if err != nil {
		return nil, fmt.Errorf("loading chart %s: %w", req.ChartPath, err)
	}

	vals, err := buildValues(chrt, req.OverlayValues, req.Values)
	if err != nil {
		return nil, err
	}

	caps := buildCapabilities(req.KubeVersion, req.APIVersions)

	releaseMeta := chartutil.ReleaseOptions{
		Name:      coalesce(req.ReleaseName, "release-name"),
		Namespace: coalesce(req.Namespace, "default"),
		IsInstall: true,
	}

	renderVals, err := chartutil.ToRenderValues(chrt, vals, releaseMeta, caps)
	if err != nil {
		return nil, fmt.Errorf("building render values: %w", err)
	}

	eng := engine.Engine{}
	rendered, err := eng.Render(chrt, renderVals)
	if err != nil {
		return nil, fmt.Errorf("rendering chart: %w", err)
	}

	result := &Result{Files: rendered, Docs: nil}

	// Sort filenames for deterministic document order across runs.
	filenames := make([]string, 0, len(rendered))
	for f := range rendered {
		filenames = append(filenames, f)
	}
	sort.Strings(filenames)

	for _, filename := range filenames {
		content := rendered[filename]
		// Skip non-template files and NOTES.txt
		if strings.HasSuffix(filename, "/NOTES.txt") {
			continue
		}
		if len(req.Templates) > 0 && !matchesTemplate(filename, req.Templates) {
			continue
		}
		docs, err := parseYAMLDocs(content)
		if err != nil {
			return nil, fmt.Errorf("parsing rendered output of %s: %w", filename, err)
		}
		result.Docs = append(result.Docs, docs...)
	}

	return result, nil
}

func buildValues(chrt *chart.Chart, overlay, sets map[string]any) (map[string]any, error) {
	// Start from the overlay (deep-merged onto chart defaults later via
	// chartutil.CoalesceValues). Copy so callers can reuse the input map.
	expanded := map[string]any{}
	for k, v := range overlay {
		expanded[k] = v
	}
	// Apply --set style overrides on top: dotted keys expand into nested maps.
	for k, v := range sets {
		valStr := fmt.Sprintf("%v", v)
		if err := strvals.ParseInto(k+"="+valStr, expanded); err != nil {
			// Fall back to storing the key verbatim if strvals rejects it.
			expanded[k] = v
		}
	}

	merged, err := chartutil.CoalesceValues(chrt, expanded)
	if err != nil {
		return nil, fmt.Errorf("merging values: %w", err)
	}
	return merged.AsMap(), nil
}

func buildCapabilities(kubeVersion string, apiVersions []string) *chartutil.Capabilities {
	caps := chartutil.DefaultCapabilities.Copy()
	if kubeVersion != "" {
		kv, err := chartutil.ParseKubeVersion(kubeVersion)
		if err == nil {
			caps.KubeVersion = *kv
		}
	}
	if len(apiVersions) > 0 {
		caps.APIVersions = append(caps.APIVersions, apiVersions...)
	}
	return caps
}

func parseYAMLDocs(content string) ([]map[string]any, error) {
	var docs []map[string]any
	decoder := yaml.NewDecoder(bytes.NewBufferString(content))
	for {
		var doc map[string]any
		err := decoder.Decode(&doc)
		if err != nil {
			break
		}
		if doc != nil {
			docs = append(docs, doc)
		}
	}
	return docs, nil
}

func matchesTemplate(filename string, templates []string) bool {
	for _, tmpl := range templates {
		if strings.HasSuffix(filename, tmpl) || strings.Contains(filename, tmpl) {
			return true
		}
	}
	return false
}

// LoadValuesFile reads a YAML values file and returns the parsed map. Use the
// result as Request.OverlayValues for `helm -f`-style overlays.
func LoadValuesFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading values file %s: %w", path, err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing values file %s: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
