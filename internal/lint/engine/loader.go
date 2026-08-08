package engine

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fregateops/vigie/internal/lint"
)

// expandRule materialises one or more YAMLRules from a parsed RuleDef.
//
// For most scopes a RuleDef maps 1:1 to a YAMLRule. The "removedAPI" scope is
// split into one rule per (removedIn, kind) group so each deprecated API can
// be disabled individually. The generated IDs are `<def.id>_<version>-<kind>`,
// preserving the order in which entries appear in the YAML.
func expandRule(ruleSet string, def RuleDef) []lint.Rule {
	if def.Scope != "removedAPI" {
		return []lint.Rule{&YAMLRule{ruleSet: ruleSet, def: def}}
	}

	type key struct{ ver, kind string }
	groups := map[key][]RemovedAPIDef{}
	var order []key
	for _, e := range def.RemovedAPIs {
		k := key{e.RemovedIn, e.Kind}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], e)
	}

	out := make([]lint.Rule, 0, len(order))
	for _, k := range order {
		sub := def
		sub.ID = fmt.Sprintf("%s_%s-%s", def.ID, k.ver, k.kind)
		sub.RemovedAPIs = groups[k]
		out = append(out, &YAMLRule{ruleSet: ruleSet, def: sub})
	}
	return out
}

//go:embed data
var dataFS embed.FS

// LoadAll parses every YAML rule file embedded under data/ and returns the
// resulting rules in deterministic (filename then declaration) order.
func LoadAll() ([]lint.Rule, error) {
	return loadFromFS(dataFS, "data")
}

func loadFromFS(fsys fs.FS, root string) ([]lint.Rule, error) {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("engine: reading %s: %w", root, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var rules []lint.Rule
	for _, name := range names {
		raw, err := fs.ReadFile(fsys, path.Join(root, name))
		if err != nil {
			return nil, fmt.Errorf("engine: reading %s: %w", name, err)
		}
		var rf RuleFile
		if err := yaml.Unmarshal(raw, &rf); err != nil {
			return nil, fmt.Errorf("engine: parsing %s: %w", name, err)
		}
		if rf.RuleSet == "" {
			return nil, fmt.Errorf("engine: %s: missing ruleSet", name)
		}
		for i := range rf.Rules {
			rules = append(rules, expandRule(rf.RuleSet, rf.Rules[i])...)
		}
	}
	return rules, nil
}
