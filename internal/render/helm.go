package render

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
)

const (
	DefaultReleaseName = "release-name"
	DefaultNamespace   = "default"
	// DefaultRevision matches `helm template`, which renders as a first install.
	DefaultRevision = 1
	notesSuffix     = "/NOTES.txt"
)

// Request specifies what to render and with which inputs.
type Request struct {
	ChartPath string
	// Values are the user-supplied values (see BuildValues), coalesced onto the
	// chart's values.yaml the way `helm install` does.
	Values      map[string]any
	ReleaseName string
	Namespace   string
	Revision    int
	IsUpgrade   bool
	KubeVersion string
	APIVersions []string
	// Templates limits rendering to these paths relative to the chart root
	// (e.g. `templates/deployment.yaml`, `charts/sub/templates/*.yaml`).
	// Glob patterns follow path.Match. Empty = all.
	Templates []string
}

// Document is one rendered YAML document and the template that produced it.
type Document struct {
	// Source is the template path relative to the chart root.
	Source string
	Object map[string]any
}

// Result is the output of a render operation.
type Result struct {
	// Docs contains all rendered, non-empty YAML documents, ordered by source.
	Docs []Document
}

// Objects returns the rendered objects without their sources.
func (r *Result) Objects() []map[string]any {
	objs := make([]map[string]any, len(r.Docs))
	for i, d := range r.Docs {
		objs[i] = d.Object
	}
	return objs
}

// Render loads a chart and renders it in-process using the Helm engine.
func Render(req Request) (*Result, error) {
	chrt, err := loader.Load(req.ChartPath)
	if err != nil {
		return nil, fmt.Errorf("loading chart %s: %w", req.ChartPath, err)
	}
	return RenderChart(chrt, req)
}

// RenderChart renders an already-loaded chart, resolving subchart
// conditions/tags and checking declared dependencies exactly like `helm install`.
func RenderChart(chrt *chart.Chart, req Request) (*Result, error) {
	if chrt.Metadata.Dependencies != nil {
		if err := action.CheckDependencies(chrt, chrt.Metadata.Dependencies); err != nil {
			return nil, fmt.Errorf("%w (run `helm dependency build %s`)", err, req.ChartPath)
		}
	}
	vals := req.Values
	if vals == nil {
		vals = map[string]any{}
	}
	if err := chartutil.ProcessDependenciesWithMerge(chrt, vals); err != nil {
		return nil, fmt.Errorf("processing chart dependencies: %w", err)
	}

	caps, err := buildCapabilities(req.KubeVersion, req.APIVersions)
	if err != nil {
		return nil, err
	}
	revision := req.Revision
	if revision == 0 {
		revision = DefaultRevision
	}
	releaseMeta := chartutil.ReleaseOptions{
		Name:      coalesce(req.ReleaseName, DefaultReleaseName),
		Namespace: coalesce(req.Namespace, DefaultNamespace),
		Revision:  revision,
		IsInstall: !req.IsUpgrade,
		IsUpgrade: req.IsUpgrade,
	}
	renderVals, err := chartutil.ToRenderValues(chrt, vals, releaseMeta, caps)
	if err != nil {
		return nil, fmt.Errorf("building render values: %w", err)
	}

	rendered, err := engine.Render(chrt, renderVals)
	if err != nil {
		return nil, fmt.Errorf("rendering chart: %w", err)
	}

	sources := make([]string, 0, len(rendered))
	for f := range rendered {
		sources = append(sources, f)
	}
	sort.Strings(sources)

	result := &Result{}
	chartPrefix := chrt.Name() + "/"
	matched := make(map[string]bool, len(req.Templates))
	for _, file := range sources {
		if strings.HasSuffix(file, notesSuffix) {
			continue
		}
		source := strings.TrimPrefix(file, chartPrefix)
		if len(req.Templates) > 0 && !matchesTemplate(source, req.Templates, matched) {
			continue
		}
		objs, err := parseYAMLDocs(rendered[file])
		if err != nil {
			return nil, fmt.Errorf("parsing rendered output of %s: %w", source, err)
		}
		for _, obj := range objs {
			result.Docs = append(result.Docs, Document{Source: source, Object: obj})
		}
	}
	for _, pattern := range req.Templates {
		if !matched[pattern] {
			return nil, fmt.Errorf("templates: %q matches no template in the chart (paths are relative to the chart root, e.g. templates/deployment.yaml)", pattern)
		}
	}
	return result, nil
}

func buildCapabilities(kubeVersion string, apiVersions []string) (*chartutil.Capabilities, error) {
	caps := chartutil.DefaultCapabilities.Copy()
	if kubeVersion != "" {
		kv, err := chartutil.ParseKubeVersion(kubeVersion)
		if err != nil {
			return nil, fmt.Errorf("invalid kubeVersion %q: %w", kubeVersion, err)
		}
		caps.KubeVersion = *kv
	}
	caps.APIVersions = append(caps.APIVersions, apiVersions...)
	return caps, nil
}

func parseYAMLDocs(content string) ([]map[string]any, error) {
	var docs []map[string]any
	decoder := yaml.NewDecoder(bytes.NewBufferString(content))
	for {
		var doc map[string]any
		err := decoder.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		if doc != nil {
			docs = append(docs, doc)
		}
	}
}

// matchesTemplate reports whether source matches any pattern, recording every
// pattern that matched so unused ones can be reported as typos.
func matchesTemplate(source string, patterns []string, matched map[string]bool) bool {
	hit := false
	for _, pattern := range patterns {
		if ok, _ := path.Match(pattern, source); ok {
			matched[pattern] = true
			hit = true
		}
	}
	return hit
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
