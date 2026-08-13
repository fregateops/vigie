package deps

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/fregateops/vigie/internal/dsl"
)

// depFile is the minimal structure of a dependency YAML file that the ref
// source can reference. It intentionally mirrors the top-level dependencies:
// block of an integration test suite.
type depFile struct {
	Dependencies []dsl.Dependency `yaml:"dependencies"`
}

// ResolveRefs resolves all ref-source dependencies in-place by loading the
// referenced YAML files and inlining their dependency lists. The expansion is
// done recursively but bounded to avoid infinite loops via a depth counter.
// The returned slice has all ref entries replaced by their resolved deps.
func ResolveRefs(deps []dsl.Dependency) ([]dsl.Dependency, error) {
	return resolveRefsDepth(deps, 0)
}

const maxRefDepth = 8

func resolveRefsDepth(deps []dsl.Dependency, depth int) ([]dsl.Dependency, error) {
	if depth > maxRefDepth {
		return nil, fmt.Errorf("ref source: exceeded maximum recursion depth %d (possible cycle)", maxRefDepth)
	}

	result := make([]dsl.Dependency, 0, len(deps))
	for _, dep := range deps {
		if dep.Source.Ref == "" {
			result = append(result, dep)
			continue
		}
		expanded, err := loadRefDeps(dep.Source.Ref, depth)
		if err != nil {
			return nil, fmt.Errorf("dep %q: loading ref %q: %w", dep.Name, dep.Source.Ref, err)
		}
		result = append(result, expanded...)
	}
	return result, nil
}

// loadRefDeps reads a dep YAML file and returns its dependencies list,
// recursively resolving any nested refs.
func loadRefDeps(refPath string, currentDepth int) ([]dsl.Dependency, error) {
	data, err := os.ReadFile(refPath)
	if err != nil {
		return nil, fmt.Errorf("reading ref file: %w", err)
	}

	var file depFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parsing ref file %q: %w", refPath, err)
	}

	if len(file.Dependencies) == 0 {
		return nil, nil
	}

	return resolveRefsDepth(file.Dependencies, currentDepth+1)
}
