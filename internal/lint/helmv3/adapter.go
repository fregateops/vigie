// Package helmv3 implements lint.ContextBuilder and lint.HelmLintProvider on
// top of helm.sh/helm/v3.
//
// All direct dependencies on Helm v3 packages live here; the rest of the lint
// package treats Helm as an interface so a future v4 adapter can be slotted in.
package helmv3

import (
	"github.com/Masterminds/semver/v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/lint/support"

	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/render"
)

// ruleSet is the rule set name that helm-v3 lint findings carry.
const ruleSet = "helm-v3-lint"

func init() { lint.RegisterProvider(New()) }

// Adapter implements both lint.ContextBuilder and lint.HelmLintProvider for
// Helm v3.
type Adapter struct{}

// New returns a ready-to-use v3 adapter.
func New() *Adapter { return &Adapter{} }

// RuleSet identifies the helm-v3 lint findings produced by LintFindings.
func (a *Adapter) RuleSet() string { return ruleSet }

// PrepareContext loads the chart and renders its templates, returning a Context
// populated with chart metadata, rendered docs, and the target Kubernetes
// version pulled from cfg.KubeVersions.
func (a *Adapter) PrepareContext(chartPath string, cfg *config.LintConfig) (lint.Context, error) {
	chrt, err := loader.Load(chartPath)
	if err != nil {
		return lint.Context{}, err
	}

	kubeVer := firstNonEmpty(cfg.KubeVersions)

	res, _ := render.Render(render.Request{
		ChartPath:   chartPath,
		ReleaseName: "release-name",
		Namespace:   "default",
		KubeVersion: kubeVer,
	})
	var docs []map[string]any
	if res != nil {
		docs = res.Docs
	}

	return lint.Context{
		ChartPath:    chartPath,
		Chart:        chrt,
		ChartMeta:    chartMetaToMap(chrt.Metadata),
		RenderedDocs: docs,
		KubeVersion:  kubeVer,
		Cfg:          cfg,
	}, nil
}

// LintFindings runs `helm lint` against the chart and converts its messages
// into Findings tagged with rule ID "helm-v3-lint".
func (a *Adapter) LintFindings(chartPath string) ([]lint.Finding, error) {
	client := action.NewLint()
	result := client.Run([]string{chartPath}, nil)

	findings := make([]lint.Finding, 0, len(result.Messages))
	for _, msg := range result.Messages {
		findings = append(findings, lint.Finding{
			RuleID:   ruleSet,
			Severity: severityFromHelm(msg.Severity),
			File:     msg.Path,
			Message:  msgText(msg),
		})
	}
	return findings, nil
}

func chartMetaToMap(m *chart.Metadata) map[string]any {
	if m == nil {
		return nil
	}
	_, semverErr := semver.NewVersion(m.Version)
	return map[string]any{
		"apiVersion":   m.APIVersion,
		"name":         m.Name,
		"version":      m.Version,
		"description":  m.Description,
		"type":         m.Type,
		"appVersion":   m.AppVersion,
		"home":         m.Home,
		"icon":         m.Icon,
		"versionValid": semverErr == nil,
	}
}

func severityFromHelm(s int) lint.Severity {
	switch s {
	case support.ErrorSev:
		return lint.SeverityError
	case support.WarningSev:
		return lint.SeverityWarning
	default:
		return lint.SeverityInfo
	}
}

func msgText(m support.Message) string {
	if m.Err != nil {
		return m.Err.Error()
	}
	return ""
}

func firstNonEmpty(ss []string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
