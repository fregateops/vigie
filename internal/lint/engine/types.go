package engine

// RuleFile is the top-level structure of a YAML rule data file.
type RuleFile struct {
	RuleSet string    `yaml:"ruleSet"`
	Rules   []RuleDef `yaml:"rules"`
}

// RuleDef is a single declarative rule.
//
// Scope determines how the engine evaluates Expr and emits findings:
//   - "chart":      bind `chart` (chart metadata map) and evaluate once.
//   - "document":   bind `doc` (rendered manifest) and `renderNamespace`,
//     evaluate per rendered document.
//   - "container":  bind `doc` and `container`, evaluate per container in
//     workload kinds (filtered by KindFilter).
//   - "removedAPI": ignore Expr; flag any rendered doc whose (apiVersion,kind)
//     is in RemovedAPIs and removed at or before the configured
//     target Kubernetes version.
type RuleDef struct {
	ID       string `yaml:"id"`
	Severity string `yaml:"severity"`
	Scope    string `yaml:"scope"`
	Expr     string `yaml:"expr,omitempty"`
	Message  string `yaml:"message"`
	HelpURI  string `yaml:"helpURI,omitempty"`
	// Project, when non-empty, marks a removedAPI rule as a project-CRD
	// deprecation rather than a Kubernetes core-API one. Project rules ignore
	// lint.kubeVersions and use the project name in their finding message
	// (e.g. "removed in cert-manager 1.6"). Filtering by project version is
	// not implemented yet — every entry is unconditionally active.
	Project     string          `yaml:"project,omitempty"`
	KindFilter  []string        `yaml:"kindFilter,omitempty"`
	RemovedAPIs []RemovedAPIDef `yaml:"removedAPIs,omitempty"`
}

// RemovedAPIDef is one entry in a deprecation table.
type RemovedAPIDef struct {
	APIVersion  string `yaml:"apiVersion"`
	Kind        string `yaml:"kind"`
	RemovedIn   string `yaml:"removedIn"`
	Replacement string `yaml:"replacement"`
}
