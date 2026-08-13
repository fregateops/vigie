package deps

import (
	"fmt"
	"strings"

	"github.com/fregateops/vigie/internal/dsl"
)

// Batch is a group of dependencies that can be installed concurrently because
// none of them depends on another within the same batch.
type Batch []dsl.Dependency

// BuildBatches performs a topological sort on deps using dependsOn edges and
// returns them as ordered batches. All deps in a batch are independent and can
// be installed in parallel. Returns an error on unknown name references or
// cycle detection.
func BuildBatches(deps []dsl.Dependency) ([]Batch, error) {
	if len(deps) == 0 {
		return nil, nil
	}
	if err := validateNames(deps); err != nil {
		return nil, err
	}
	sorted, err := topoSort(deps)
	if err != nil {
		return nil, err
	}
	return buildLevelBatches(sorted), nil
}

// validateNames checks that every dependsOn reference names an existing dep.
func validateNames(deps []dsl.Dependency) error {
	known := make(map[string]struct{}, len(deps))
	for _, dep := range deps {
		known[dep.Name] = struct{}{}
	}
	for _, dep := range deps {
		for _, ref := range dep.DependsOn {
			if _, ok := known[ref]; !ok {
				return fmt.Errorf("dependency %q references unknown dep %q in dependsOn", dep.Name, ref)
			}
		}
	}
	return nil
}

// topoSort returns deps in a valid install order (Kahn's algorithm).
// Returns an error if a cycle is detected.
func topoSort(deps []dsl.Dependency) ([]dsl.Dependency, error) {
	// Build in-degree map and adjacency list.
	inDegree := make(map[string]int, len(deps))
	byName := make(map[string]dsl.Dependency, len(deps))
	for _, dep := range deps {
		inDegree[dep.Name] = len(dep.DependsOn)
		byName[dep.Name] = dep
	}

	// Dependents: for a given dep, which deps must wait for it.
	dependents := make(map[string][]string, len(deps))
	for _, dep := range deps {
		for _, prereq := range dep.DependsOn {
			dependents[prereq] = append(dependents[prereq], dep.Name)
		}
	}

	// Collect roots (zero in-degree).
	queue := make([]string, 0, len(deps))
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	result := make([]dsl.Dependency, 0, len(deps))
	for len(queue) > 0 {
		// Consume head of queue.
		name := queue[0]
		queue = queue[1:]
		result = append(result, byName[name])
		for _, dependent := range dependents[name] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(result) != len(deps) {
		return nil, buildCycleError(deps, result)
	}
	return result, nil
}

// buildCycleError reports which dependencies form cycles by comparing installed
// vs remaining names.
func buildCycleError(all, installed []dsl.Dependency) error {
	installedNames := make(map[string]struct{}, len(installed))
	for _, dep := range installed {
		installedNames[dep.Name] = struct{}{}
	}
	cyclic := make([]string, 0)
	for _, dep := range all {
		if _, ok := installedNames[dep.Name]; !ok {
			cyclic = append(cyclic, dep.Name)
		}
	}
	return fmt.Errorf("cycle detected in dependency graph: %s", strings.Join(cyclic, ", "))
}

// buildLevelBatches groups sorted deps into parallel batches. Two deps land in
// the same batch only when none of them depends on the other. We achieve this
// by tracking which names have been emitted and grouping by "all my deps are
// already in a previous batch."
func buildLevelBatches(sorted []dsl.Dependency) []Batch {
	// Level assignment: level[name] = max(level[prereq] for prereq in dependsOn) + 1
	level := make(map[string]int, len(sorted))
	maxLevel := 0
	for _, dep := range sorted {
		depLevel := 0
		for _, prereq := range dep.DependsOn {
			if level[prereq]+1 > depLevel {
				depLevel = level[prereq] + 1
			}
		}
		level[dep.Name] = depLevel
		if depLevel > maxLevel {
			maxLevel = depLevel
		}
	}

	batches := make([]Batch, maxLevel+1)
	for _, dep := range sorted {
		lvl := level[dep.Name]
		batches[lvl] = append(batches[lvl], dep)
	}
	return batches
}
