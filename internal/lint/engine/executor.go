package engine

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/fregateops/vigie/internal/cel"
	"github.com/fregateops/vigie/internal/lint"
)

// runChartScope evaluates a rule once against ctx.ChartMeta.
// A rule fails when its CEL expression returns false; an evaluation error
// is treated as a pass so a malformed input doesn't spam findings.
func runChartScope(ctx lint.Context, id string, def RuleDef) []lint.Finding {
	if ctx.ChartMeta == nil {
		return nil
	}
	bindings := map[string]any{"chart": ctx.ChartMeta}
	pass, err := evalBool(def.Expr, bindings)
	if err != nil || pass {
		return nil
	}
	return []lint.Finding{{
		RuleID:   id,
		Severity: lint.Severity(def.Severity),
		File:     "Chart.yaml",
		Message:  interpolate(def.Message, bindings),
		HelpURI:  def.HelpURI,
	}}
}

// runDocumentScope evaluates the rule per rendered document.
func runDocumentScope(ctx lint.Context, id string, def RuleDef) []lint.Finding {
	var findings []lint.Finding
	for _, doc := range ctx.RenderedDocs {
		bindings := map[string]any{
			"doc":             doc,
			"renderNamespace": "default",
		}
		pass, err := evalBool(def.Expr, bindings)
		if err != nil || pass {
			continue
		}
		findings = append(findings, lint.Finding{
			RuleID:   id,
			Severity: lint.Severity(def.Severity),
			File:     "templates/",
			Message:  interpolate(def.Message, bindings),
			HelpURI:  def.HelpURI,
		})
	}
	return findings
}

// runContainerScope iterates containers within workload documents and
// evaluates the rule per container.
func runContainerScope(ctx lint.Context, id string, def RuleDef) []lint.Finding {
	kinds := make(map[string]bool, len(def.KindFilter))
	for _, k := range def.KindFilter {
		kinds[k] = true
	}
	var findings []lint.Finding
	for _, doc := range ctx.RenderedDocs {
		kind, _ := doc["kind"].(string)
		if len(kinds) > 0 && !kinds[kind] {
			continue
		}
		for _, c := range containersFromDoc(doc, kind) {
			bindings := map[string]any{
				"doc":       doc,
				"container": c,
			}
			pass, err := evalBool(def.Expr, bindings)
			if err != nil || pass {
				continue
			}
			findings = append(findings, lint.Finding{
				RuleID:   id,
				Severity: lint.Severity(def.Severity),
				File:     "templates/",
				Message:  interpolate(def.Message, bindings),
				HelpURI:  def.HelpURI,
			})
		}
	}
	return findings
}

// runRemovedAPIScope flags any rendered doc whose (apiVersion,kind) appears
// in the rule's removedAPIs table.
//
// For Kubernetes core APIs (def.Project == ""), entries are filtered against
// ctx.KubeVersion — entries whose RemovedIn is greater than the target are
// skipped. If ctx.KubeVersion is empty, the entire table is active.
//
// For project-CRD rules (def.Project != ""), no version filter is applied
// and the project name is used in the finding message instead of "Kubernetes".
func runRemovedAPIScope(ctx lint.Context, id string, def RuleDef) []lint.Finding {
	type key struct{ apiVersion, kind string }
	index := make(map[key]RemovedAPIDef, len(def.RemovedAPIs))

	var target *semver.Version
	if def.Project == "" && ctx.KubeVersion != "" {
		if v, err := semver.NewVersion(ctx.KubeVersion); err == nil {
			target = v
		}
	}

	for _, e := range def.RemovedAPIs {
		if target != nil {
			if removed, err := semver.NewVersion(e.RemovedIn); err == nil && removed.GreaterThan(target) {
				continue
			}
		}
		index[key{e.APIVersion, e.Kind}] = e
	}

	context := "Kubernetes"
	if def.Project != "" {
		context = def.Project
	}

	var findings []lint.Finding
	for _, doc := range ctx.RenderedDocs {
		av, _ := doc["apiVersion"].(string)
		kind, _ := doc["kind"].(string)
		entry, ok := index[key{av, kind}]
		if !ok {
			continue
		}
		name := docName(doc)
		findings = append(findings, lint.Finding{
			RuleID:   id,
			Severity: lint.Severity(def.Severity),
			File:     "templates/",
			Message: fmt.Sprintf(
				"%s/%s uses removed apiVersion %q (removed in %s %s; use %s)",
				kind, name, av, context, entry.RemovedIn, entry.Replacement,
			),
			HelpURI: def.HelpURI,
		})
	}
	return findings
}

// evalBool runs a CEL expression and asserts a boolean result.
func evalBool(expr string, bindings map[string]any) (bool, error) {
	out, err := cel.Eval(expr, bindings)
	if err != nil {
		return false, err
	}
	b, ok := out.(bool)
	if !ok {
		return false, fmt.Errorf("CEL expression %q returned %T, want bool", expr, out)
	}
	return b, nil
}

// containersFromDoc extracts the container list from a workload document,
// handling CronJob's nested jobTemplate path.
func containersFromDoc(doc map[string]any, kind string) []map[string]any {
	var spec map[string]any
	if kind == "CronJob" {
		s, _ := doc["spec"].(map[string]any)
		jt, _ := s["jobTemplate"].(map[string]any)
		js, _ := jt["spec"].(map[string]any)
		t, _ := js["template"].(map[string]any)
		spec, _ = t["spec"].(map[string]any)
	} else {
		s, _ := doc["spec"].(map[string]any)
		t, _ := s["template"].(map[string]any)
		spec, _ = t["spec"].(map[string]any)
	}
	raw, _ := spec["containers"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, c := range raw {
		if m, ok := c.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func docName(doc map[string]any) string {
	meta, _ := doc["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	return name
}
