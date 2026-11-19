package dsl

import (
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"
)

// Keys helm-unittest accepts where vigie does not: vigie nests rendering inputs
// under `inputs:`/`defaults:` and selects documents with `target:`.
// See https://github.com/helm-unittest/helm-unittest/blob/main/DOCUMENT.md
var (
	helmUnittestSuiteKeys = []string{
		"set", "values", "release", "capabilities", "chart", "excludeTemplates",
		"documentSelector", "kubernetesProvider", "postRenderer", "skip",
	}
	helmUnittestTestKeys = []string{
		"set", "values", "release", "capabilities", "chart", "template", "templates",
		"documentIndex", "documentSelector", "kubernetesProvider", "postRenderer",
	}
	helmUnittestOnlyMatchers = []string{
		"containsDocument", "isNullOrEmpty", "isNotNullOrEmpty", "notFailedTemplate",
		"equalRaw", "notEqualRaw", "matchRegexRaw", "notMatchRegexRaw",
	}
	// Matchers vigie shares with helm-unittest but takes as a scalar, while
	// helm-unittest takes a map with this key (`hasDocuments: {count: 1}`).
	helmUnittestMapForms = map[string]string{"hasDocuments": "count", "isKind": "of", "isAPIVersion": "of"}
)

// HelmUnittestError reports a test file written for helm-unittest. vigie does
// not run it, and says so rather than failing it as a malformed vigie file.
type HelmUnittestError struct {
	Path  string
	Suite string
	Tests int
	// Marker is the first helm-unittest construct found, for the report.
	Marker string
}

func (e *HelmUnittestError) Error() string {
	return fmt.Sprintf("%s follows the helm-unittest format (%s)", e.Path, e.Marker)
}

type helmUnittestFile struct {
	Suite string           `yaml:"suite"`
	Keys  map[string]any   `yaml:",inline"`
	Tests []map[string]any `yaml:"tests"`
}

// detectHelmUnittest returns a HelmUnittestError when raw (a file that failed
// vigie's schema) uses a construct only helm-unittest accepts.
func detectHelmUnittest(raw []byte, path string) *HelmUnittestError {
	var file helmUnittestFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil
	}
	marker := helmUnittestMarker(file)
	if marker == "" {
		return nil
	}
	return &HelmUnittestError{Path: path, Suite: file.Suite, Tests: len(file.Tests), Marker: marker}
}

func helmUnittestMarker(file helmUnittestFile) string {
	for _, key := range helmUnittestSuiteKeys {
		if _, ok := file.Keys[key]; ok {
			return fmt.Sprintf("suite-level `%s:`", key)
		}
	}
	for _, test := range file.Tests {
		for _, key := range helmUnittestTestKeys {
			if _, ok := test[key]; ok {
				return fmt.Sprintf("test-level `%s:`", key)
			}
		}
		asserts, _ := test["asserts"].([]any)
		for _, a := range asserts {
			if marker := helmUnittestAssertMarker(a); marker != "" {
				return marker
			}
		}
	}
	return ""
}

func helmUnittestAssertMarker(assert any) string {
	matchers, _ := assert.(map[string]any)
	for name, spec := range matchers {
		if slices.Contains(helmUnittestOnlyMatchers, name) {
			return fmt.Sprintf("matcher `%s`", name)
		}
		key, shared := helmUnittestMapForms[name]
		specMap, isMap := spec.(map[string]any)
		if _, ok := specMap[key]; shared && isMap && ok {
			return fmt.Sprintf("`%s: {%s: ...}`", name, key)
		}
	}
	return ""
}
