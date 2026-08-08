package dsl

// Suite is the top-level structure of a test file.
type Suite struct {
	// Optional $schema URI for IDE validation.
	Schema string `yaml:"$schema" json:"$schema,omitempty"`

	// Human-readable name for this test file.
	SuiteName string `yaml:"suite" json:"suite"`

	// Unit-tier only (vigie test). Limits rendering to these templates (paths relative to chart root);
	// if omitted, all templates render. In apply-tier suites (test-apply api|simulated|e2e) this is ignored —
	// use target: per test to scope matchers instead.
	Templates []string `yaml:"templates" json:"templates,omitempty"`

	// Template files loaded for their define blocks only — no manifests are rendered.
	// Used for helper function tests (call:). Paths relative to chart root.
	Helpers []string `yaml:"helpers" json:"helpers,omitempty"`

	// Optional chart override for this suite.
	Chart *ChartSpec `yaml:"chart" json:"chart,omitempty"`

	// Cluster backend for live tiers (apiserver/simulated/e2e).
	Cluster *ClusterSpec `yaml:"cluster" json:"cluster,omitempty"`

	// Default inputs applied to every test in this suite.
	Defaults *Inputs `yaml:"defaults" json:"defaults,omitempty"`

	// Resources to install before tests run.
	Dependencies []Dependency `yaml:"dependencies" json:"dependencies,omitempty"`

	// Lifecycle hooks run once before all tests.
	BeforeAll []LifecycleHook `yaml:"beforeAll" json:"beforeAll,omitempty"`

	// Lifecycle hooks run once after all tests.
	AfterAll []LifecycleHook `yaml:"afterAll" json:"afterAll,omitempty"`

	// The list of test cases in this suite.
	Tests []Test `yaml:"tests" json:"tests"`
}

// ChartSpec optionally overrides the chart used by a suite.
type ChartSpec struct {
	// Path to the chart relative to the test file.
	Path string `yaml:"path" json:"path,omitempty"`

	// How to resolve chart dependencies before rendering.
	Dependencies string `yaml:"dependencies" json:"dependencies,omitempty" jsonschema:"enum=build,enum=update,enum=skip"`
}

// Inputs is the set of value overrides and release/capabilities settings.
type Inputs struct {
	// Helm --set overrides (dotted key paths to scalar values).
	Set map[string]any `yaml:"set" json:"set,omitempty"`

	// Helm --set-json overrides (dotted key paths to JSON-encoded values).
	SetJSON map[string]any `yaml:"setJson" json:"setJson,omitempty"`

	// Helm --set-literal overrides (dotted key paths to literal string values).
	SetLiteral map[string]string `yaml:"setLiteral" json:"setLiteral,omitempty"`

	// Extra values files to merge, relative to the chart root.
	Values []string `yaml:"values" json:"values,omitempty"`

	// Release metadata overrides.
	Release *ReleaseInputs `yaml:"release" json:"release,omitempty"`

	// Kubernetes API capabilities overrides.
	Capabilities *CapabilityInputs `yaml:"capabilities" json:"capabilities,omitempty"`
}

// ReleaseInputs overrides Helm release metadata.
type ReleaseInputs struct {
	// Release name passed to Helm.
	Name string `yaml:"name" json:"name,omitempty"`

	// Release namespace passed to Helm.
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`

	// Release revision number.
	Revision int `yaml:"revision" json:"revision,omitempty"`

	// Whether to treat this as an install.
	IsInstall bool `yaml:"isInstall" json:"isInstall,omitempty"`

	// Whether to treat this as an upgrade.
	IsUpgrade bool `yaml:"isUpgrade" json:"isUpgrade,omitempty"`
}

// CapabilityInputs overrides Kubernetes API capabilities visible to templates.
type CapabilityInputs struct {
	// Kubernetes version string (e.g. "1.29.0").
	KubeVersion string `yaml:"kubeVersion" json:"kubeVersion,omitempty"`

	// Additional API versions reported as available.
	APIVersions []string `yaml:"apiVersions" json:"apiVersions,omitempty"`
}

// Test is a single test case.
type Test struct {
	// Human-readable description of the scenario.
	It string `yaml:"it" json:"it"`

	// Tiers this test applies to. Default: [template, validate].
	Tier []string `yaml:"tier" json:"tier,omitempty"`

	// Arbitrary labels for filtering tests.
	Tags []string `yaml:"tags" json:"tags,omitempty"`

	// Skip condition — boolean or CEL expression string.
	Skip any `yaml:"skip" json:"skip,omitempty"`

	// Value overrides and release/capabilities settings for this test.
	Inputs *Inputs `yaml:"inputs" json:"inputs,omitempty"`

	// Cartesian product across declared dimensions. Reserved keys 'exclude' and 'include' filter combinations.
	Matrix map[string]any `yaml:"matrix" json:"matrix,omitempty"`

	// Explicit list of named parameter sets; no cartesian expansion.
	Cases []map[string]any `yaml:"cases" json:"cases,omitempty"`

	// When true, runs one test per rendered document instead of one test for all documents.
	ForEach bool `yaml:"forEach" json:"forEach,omitempty"`

	// Named template (define block) to invoke. Mutually exclusive with target:. Activates helper test mode.
	Call string `yaml:"call" json:"call,omitempty"`

	// Dict passed to the named template. Values are YAML literals or ${{ CEL }} expressions.
	Args map[string]any `yaml:"args" json:"args,omitempty"`

	// How to interpret the helper's rendered output. Default: string.
	// yaml/json enable path-based matchers on the parsed structure. bool treats non-empty output as true.
	OutputAs string `yaml:"outputAs" json:"outputAs,omitempty" jsonschema:"enum=string,enum=yaml,enum=json,enum=bool"`

	// Selects which rendered document(s) the asserts apply to.
	Target *TargetSpec `yaml:"target" json:"target,omitempty"`

	// Assertions to evaluate.
	Asserts []Assertion `yaml:"asserts" json:"asserts,omitempty"`

	// Lifecycle hooks run before this test.
	Setup []LifecycleHook `yaml:"setup,omitempty" json:"setup,omitempty"`

	// Lifecycle hooks run after this test.
	Teardown []LifecycleHook `yaml:"teardown,omitempty" json:"teardown,omitempty"`
}

// TargetSpec selects which rendered document(s) the asserts apply to.
type TargetSpec struct {
	// Kubernetes resource kind to select.
	Kind string `yaml:"kind" json:"kind,omitempty"`

	// Resource name to select.
	Name string `yaml:"name" json:"name,omitempty"`

	// Resource namespace to select.
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`

	// API version to select.
	APIVersion string `yaml:"apiVersion" json:"apiVersion,omitempty"`

	// Label selector to select resources.
	Labels map[string]string `yaml:"labels" json:"labels,omitempty"`

	// Zero-based index of the document to select when multiple documents match.
	DocumentIndex *int `yaml:"documentIndex" json:"documentIndex,omitempty" jsonschema:"minimum=0"`

	// CEL expression returning bool over each document.
	Expr string `yaml:"expr" json:"expr,omitempty"`
}

// Assertion is a single assertion within a test. Exactly one matcher key is expected (optionally with not/on).
type Assertion struct {
	// Negate this assertion — a passing matcher fails and vice versa.
	Not bool `yaml:"not" json:"not,omitempty"`

	// Per-assertion document selector; overrides the test-level target: for this assertion only.
	On *TargetSpec `yaml:"on" json:"on,omitempty"`

	// Asserts the value at path deep-equals value (int/float normalized for YAML). All tiers.
	Equal *PathValue `yaml:"equal" json:"equal,omitempty"`

	// Asserts the value at path is not deep-equal to value. A missing path passes. All tiers.
	NotEqual *PathValue `yaml:"notEqual" json:"notEqual,omitempty"`

	// Asserts the numeric value at path is strictly greater than value. All tiers.
	GreaterThan *PathValue `yaml:"greaterThan" json:"greaterThan,omitempty"`

	// Asserts the numeric value at path is strictly less than value. All tiers.
	LessThan *PathValue `yaml:"lessThan" json:"lessThan,omitempty"`

	// Asserts the numeric value at path is greater than or equal to value. All tiers.
	GTE *PathValue `yaml:"gte" json:"gte,omitempty"`

	// Asserts the numeric value at path is less than or equal to value. All tiers.
	LTE *PathValue `yaml:"lte" json:"lte,omitempty"`

	// Asserts the string at path contains the substring content, or the list at path contains the item. All tiers.
	Contains *PathContent `yaml:"contains" json:"contains,omitempty"`

	// Asserts the string/list at path does not contain content. All tiers.
	NotContains *PathContent `yaml:"notContains" json:"notContains,omitempty"`

	// Asserts the string at path starts with value. All tiers.
	StartsWith *PathValue `yaml:"startsWith" json:"startsWith,omitempty"`

	// Asserts the string at path ends with value. All tiers.
	EndsWith *PathValue `yaml:"endsWith" json:"endsWith,omitempty"`

	// Asserts the string at path matches the RE2 regular expression pattern. All tiers.
	MatchRegex *PathPattern `yaml:"matchRegex" json:"matchRegex,omitempty"`

	// Asserts the string at path does not match the RE2 regular expression pattern. All tiers.
	NotMatchRegex *PathPattern `yaml:"notMatchRegex" json:"notMatchRegex,omitempty"`

	// Asserts the string at path matches a template pattern where ${VAR} placeholders match any text. All tiers.
	MatchTemplate *PathPattern `yaml:"matchTemplate" json:"matchTemplate,omitempty"`

	// Asserts path is present (any value, including null). All tiers.
	Exists *PathOnly `yaml:"exists" json:"exists,omitempty"`

	// Asserts path is absent. All tiers.
	NotExists *PathOnly `yaml:"notExists" json:"notExists,omitempty"`

	// Asserts the value at path is null. All tiers.
	IsNull *PathOnly `yaml:"isNull" json:"isNull,omitempty"`

	// Asserts the value at path is not null. All tiers.
	IsNotNull *PathOnly `yaml:"isNotNull" json:"isNotNull,omitempty"`

	// Asserts the value at path is empty (empty string, list, map, or null). All tiers.
	IsEmpty *PathOnly `yaml:"isEmpty" json:"isEmpty,omitempty"`

	// Asserts the value at path is not empty. All tiers.
	IsNotEmpty *PathOnly `yaml:"isNotEmpty" json:"isNotEmpty,omitempty"`

	// Asserts the selected document's kind field equals this value. All tiers.
	IsKind *string `yaml:"isKind" json:"isKind,omitempty"`

	// Asserts the selected document's apiVersion field equals this value. All tiers.
	IsAPIVersion *string `yaml:"isAPIVersion" json:"isAPIVersion,omitempty"`

	// Asserts the render produced exactly this many YAML documents. All tiers.
	HasDocuments *int `yaml:"hasDocuments" json:"hasDocuments,omitempty"`

	// Asserts the render itself failed; optionally that the error matches errorPattern. All tiers.
	FailedTemplate *FailedTemplateSpec `yaml:"failedTemplate" json:"failedTemplate,omitempty"`

	// Passes when every child assertion passes. Supported tiers are the intersection of the children's tiers.
	AllOf []Assertion `yaml:"allOf" json:"allOf,omitempty"`

	// Passes when at least one child assertion passes. Supported tiers are the intersection of the children's tiers.
	AnyOf []Assertion `yaml:"anyOf" json:"anyOf,omitempty"`

	// Asserts the value at path is of type of. All tiers.
	IsType *IsTypeSpec `yaml:"isType" json:"isType,omitempty"`

	// Asserts the collection at path has exactly value elements. All tiers.
	LengthEqual *LengthEqualSpec `yaml:"lengthEqual" json:"lengthEqual,omitempty"`

	// Asserts the map at path contains every key/value pair in content (deep-equal per key). All tiers.
	IsSubset *PathContent `yaml:"isSubset" json:"isSubset,omitempty"`

	// Asserts the document matches the stored snapshot; writes on first run or with --update-snapshots. All tiers.
	MatchSnapshot *MatchSnapshotSpec `yaml:"matchSnapshot" json:"matchSnapshot,omitempty"`

	// Asserts the value at path validates against an inline JSON Schema (draft 2020-12) fragment. All tiers.
	MatchSchema *MatchSchemaSpec `yaml:"matchSchema" json:"matchSchema,omitempty"`

	// CEL expression that must evaluate to true. Bindings: resources, doc, release, values, matrix, case, output (helper tests). All tiers.
	Expr *string `yaml:"expr" json:"expr,omitempty"`

	// Asserts the resource is accepted by the API server (admission, schema, RBAC, immutability). Apply tiers only (test-apply api/simulated/e2e).
	Applies *AppliesSpec `yaml:"applies" json:"applies,omitempty"`

	// Asserts the resource is rejected by the API server, optionally matching reason/message. Apply tiers only (test-apply api/simulated/e2e).
	Rejected *RejectedSpec `yaml:"rejected" json:"rejected,omitempty"`

	// Polls until the resource reports the given status condition. Apply tiers simulated and e2e only.
	WaitFor *WaitForSpec `yaml:"waitFor" json:"waitFor,omitempty"`

	// Polls until the resource is Ready (readiness inferred per kind). Apply tiers simulated and e2e only.
	BecomesReady *WaitForSpec `yaml:"becomesReady" json:"becomesReady,omitempty"`

	// Sends an HTTP request to a target in the cluster and asserts on the response. e2e tier only.
	HTTP *HTTPAssert `yaml:"http" json:"http,omitempty"`

	// Fetches a live resource and runs nested assertions on it. Apply tiers simulated and e2e only.
	Lookup *LookupAssert `yaml:"lookup" json:"lookup,omitempty"`

	// Asserts a pod's logs contain a substring or /regex/ within a timeout. e2e tier only.
	LogsContain *LogsAssert `yaml:"logsContain" json:"logsContain,omitempty"`

	// Asserts a Kubernetes Event with the given reason/type was emitted for the involved object. Apply tiers simulated and e2e only.
	EventEmitted *EventAssert `yaml:"eventEmitted" json:"eventEmitted,omitempty"`

	// Planned (not yet implemented): asserts a specific template produced no output. See DESIGN.md §8.
	NotRendered *NotRenderedSpec `yaml:"notRendered,omitempty" json:"notRendered,omitempty"`

	// Planned (not yet implemented): asserts a Prometheus-format metric on a target endpoint matches an expected value. See DESIGN.md §8.
	MetricEquals *MetricEqualsSpec `yaml:"metricEquals,omitempty" json:"metricEquals,omitempty"`
}

// PathValue is a dotted/bracketed path plus an expected value.
type PathValue struct {
	// Dotted/bracketed path into the rendered document or parsed helper output. Omit in helper tests to target the whole output.
	Path string `yaml:"path" json:"path,omitempty"`

	// Expected value to compare against.
	Value any `yaml:"value" json:"value,omitempty"`
}

// PathContent is a dotted/bracketed path plus content to look for.
type PathContent struct {
	// Dotted/bracketed path into the rendered document or parsed helper output. Omit in helper tests to target the whole output.
	Path string `yaml:"path" json:"path,omitempty"`

	// Substring, list item, or sub-map to test for, depending on the matcher.
	Content any `yaml:"content" json:"content,omitempty"`
}

// PathPattern is a dotted/bracketed path plus a pattern.
type PathPattern struct {
	// Dotted/bracketed path into the rendered document or parsed helper output. Omit in helper tests to target the whole output.
	Path string `yaml:"path" json:"path,omitempty"`

	// RE2 regular expression (matchRegex/notMatchRegex) or template with ${VAR} placeholders (matchTemplate).
	Pattern string `yaml:"pattern" json:"pattern,omitempty"`
}

// PathOnly is a bare dotted/bracketed path with no comparison value.
type PathOnly struct {
	// Dotted/bracketed path into the rendered document or parsed helper output. Omit in helper tests to target the whole output.
	Path string `yaml:"path" json:"path,omitempty"`
}

// FailedTemplateSpec is used by the failedTemplate matcher.
type FailedTemplateSpec struct {
	// Optional RE2 regex the render error message must match.
	ErrorPattern string `yaml:"errorPattern" json:"errorPattern,omitempty"`
}

// IsTypeSpec is used by the isType matcher. Of is one of: "string", "int", "float", "bool", "list", "map".
type IsTypeSpec struct {
	// Dotted/bracketed path into the selected document or parsed helper output.
	Path string `yaml:"path" json:"path,omitempty"`

	// Expected type name. Whole-number floats are accepted as int.
	Of string `yaml:"of" json:"of,omitempty"`
}

// LengthEqualSpec is used by the lengthEqual matcher.
type LengthEqualSpec struct {
	// Dotted/bracketed path into the selected document or parsed helper output.
	Path string `yaml:"path" json:"path,omitempty"`

	// Expected length.
	Value int `yaml:"value" json:"value,omitempty"`
}

// MatchSnapshotSpec is used by the matchSnapshot matcher.
type MatchSnapshotSpec struct {
	// Optional dotted/bracketed path to snapshot; omit to snapshot the whole document/helper output.
	Path string `yaml:"path" json:"path,omitempty"`
}

// MatchSchemaSpec is used by the matchSchema matcher.
// Schema is an inline JSON Schema fragment (draft 2020-12).
type MatchSchemaSpec struct {
	// Dotted/bracketed path into the selected document or parsed helper output.
	Path string `yaml:"path" json:"path,omitempty"`

	// Inline JSON Schema fragment the value must satisfy.
	Schema map[string]any `yaml:"schema" json:"schema,omitempty"`
}

// AppliesSpec is used by the applies matcher.
// It asserts that the resource was accepted by the API server.
// Only valid in test-apply api tier or higher.
type AppliesSpec struct{}

// RejectedSpec is used by the rejected matcher.
// It asserts that the resource was rejected by the API server.
// Reason and Message are optional; if set they narrow the check.
type RejectedSpec struct {
	// Expected API error reason (e.g. "Invalid", "Forbidden").
	Reason string `yaml:"reason" json:"reason,omitempty"`

	// Regex matched against the error message.
	Message string `yaml:"message" json:"message,omitempty"`
}

// NotRenderedSpec is the spec for the notRendered planned matcher.
type NotRenderedSpec struct {
	// Template path that should produce no output.
	Template string `yaml:"template" json:"template,omitempty"`
}

// MetricEqualsSpec is the spec for the metricEquals planned matcher.
type MetricEqualsSpec struct{}

// Integration-tier types follow. These are used by test-apply simulated/e2e tiers.

// ClusterSpec selects the cluster backend used by live tiers.
type ClusterSpec struct {
	// Cluster backend name.
	Backend string `yaml:"backend" json:"backend,omitempty" jsonschema:"enum=kind,enum=k3d,enum=kubeconfig,enum=envtest,enum=simulated"`

	// Kubernetes version string (semver).
	KubeVersion string `yaml:"kubeVersion" json:"kubeVersion,omitempty" jsonschema:"pattern=^[0-9]+\\.[0-9]+(\\.[0-9]+)?$"`

	// Number of nodes to provision (where supported).
	Nodes int `yaml:"nodes" json:"nodes,omitempty" jsonschema:"minimum=1"`

	// Path to an external kubeconfig (for the kubeconfig backend).
	Kubeconfig string `yaml:"kubeconfig" json:"kubeconfig,omitempty"`

	// Optional kwok node simulator configuration.
	Kwok *KwokSpec `yaml:"kwok,omitempty" json:"kwok,omitempty"`
}

// KwokSpec configures the kwok node simulator.
type KwokSpec struct {
	// Whether to enable kwok.
	Enabled bool `yaml:"enabled" json:"enabled,omitempty"`

	// Number of kwok nodes to provision.
	NodeCount int `yaml:"nodeCount,omitempty" json:"nodeCount,omitempty" jsonschema:"minimum=1"`
}

// Dependency declares a resource that must be installed before tests run.
type Dependency struct {
	// Unique dependency name.
	Name string `yaml:"name" json:"name"`

	// Namespace to install the dependency into.
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`

	// Where the dependency comes from (helm chart, manifest, kustomize, secret, or ref).
	Source DependencySource `yaml:"source" json:"source"`

	// Values to pass when installing the dependency.
	Values map[string]any `yaml:"values" json:"values,omitempty"`

	// Conditions that must hold before the dependency is considered ready.
	WaitFor []WaitForSpec `yaml:"waitFor" json:"waitFor,omitempty"`

	// Map of export names to fields on the dependency (for downstream use).
	Exports map[string]string `yaml:"exports" json:"exports,omitempty"`

	// Lifetime of the dependency: suite (default), test, or cluster.
	Scope string `yaml:"scope" json:"scope,omitempty" jsonschema:"enum=suite,enum=test,enum=cluster"`

	// Names of other dependencies that must be installed first.
	DependsOn []string `yaml:"dependsOn" json:"dependsOn,omitempty"`

	// What to do on failure: clean (default) or keep the partial state.
	OnFail string `yaml:"onFail" json:"onFail,omitempty" jsonschema:"enum=clean,enum=keep"`
}

// DependencySource is the union of supported dependency source types.
type DependencySource struct {
	// Install a Helm chart from a repo.
	Helm *HelmSource `yaml:"helm" json:"helm,omitempty"`

	// Apply a path/glob of raw Kubernetes manifests.
	Manifest string `yaml:"manifest" json:"manifest,omitempty"`

	// Apply a kustomize directory.
	Kustomize string `yaml:"kustomize" json:"kustomize,omitempty"`

	// Resolve and create a Kubernetes Secret.
	Secret *SecretSource `yaml:"secret,omitempty" json:"secret,omitempty"`

	// Reference another dependency by name.
	Ref string `yaml:"ref" json:"ref,omitempty"`
}

// HelmSource installs a Helm chart from a repo or local path.
type HelmSource struct {
	// Chart name or local path.
	Chart string `yaml:"chart" json:"chart"`

	// Chart repository URL.
	Repo string `yaml:"repo" json:"repo,omitempty"`

	// Chart version constraint.
	Version string `yaml:"version" json:"version,omitempty"`
}

// WaitForSpec polls a resource until a status condition is met. Used by the waitFor/becomesReady
// matchers and by dependency.waitFor. Apply tiers only.
type WaitForSpec struct {
	// Resource kind to poll (e.g. Deployment, Pod, StatefulSet).
	Kind string `yaml:"kind" json:"kind"`

	// Resource name. Provide this or labelSelector.
	Name string `yaml:"name" json:"name,omitempty"`

	// Namespace to poll in. Defaults to the test's per-test namespace.
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`

	// Label selector identifying the resource(s) to poll. Provide this or name.
	LabelSelector string `yaml:"labelSelector" json:"labelSelector,omitempty"`

	// status.conditions[] type that must become True (e.g. Available, Ready).
	// For becomesReady this is inferred from the kind.
	Condition string `yaml:"condition" json:"condition,omitempty"`

	// Maximum time to poll before failing, as a Go duration (e.g. 30s, 2m). Default 2m.
	Timeout string `yaml:"timeout" json:"timeout,omitempty" jsonschema:"pattern=^[0-9]+(ns|us|ms|s|m|h)$"`
}

// ShellCommand is shared by generate: in secret sources, LifecycleHook, and script: source (M5).
type ShellCommand struct {
	// Shell command to execute.
	Run string `yaml:"run" json:"run"`

	// Shell binary (default: sh).
	Shell string `yaml:"shell,omitempty" json:"shell,omitempty"`

	// Working directory.
	Dir string `yaml:"dir,omitempty" json:"dir,omitempty"`

	// Environment variables to set.
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty"`

	// Maximum time to allow the command to run, as a Go duration.
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// LifecycleHook is a type alias for ShellCommand.
type LifecycleHook = ShellCommand

// SecretSource defines how to resolve and inject a K8s Secret.
// The parent Dependency.Name is the K8s Secret name.
// Keys must be non-empty; each entry populates one Secret.data key.
type SecretSource struct {
	// Non-empty list of Secret data keys to populate.
	Keys []SecretKeySpec `yaml:"keys" json:"keys" jsonschema:"minItems=1"`
}

// SecretKeySpec describes how to populate a single Secret data key.
type SecretKeySpec struct {
	// Secret data key name.
	Key string `yaml:"key" json:"key,omitempty"`

	// Environment variable to source the value from.
	Env string `yaml:"env,omitempty" json:"env,omitempty"`

	// File path to source the value from.
	File string `yaml:"file,omitempty" json:"file,omitempty"`

	// Shell command that produces the value on stdout.
	Generate *ShellCommand `yaml:"generate,omitempty" json:"generate,omitempty"`

	// Fallback source if the primary cannot be resolved.
	Fallback *SecretKeySpec `yaml:"fallback,omitempty" json:"fallback,omitempty"`
}

// HTTPAssert sends an HTTP request to a cluster target and asserts on the response. e2e tier only.
type HTTPAssert struct {
	// Routing mode to reach the target. Default: portforward.
	Via string `yaml:"via" json:"via,omitempty" jsonschema:"enum=portforward,enum=ingress,enum=nodeport,enum=direct"`

	// The Kubernetes resource to reach (Service, Ingress, etc.).
	Target *HTTPTarget `yaml:"target" json:"target,omitempty"`

	// HTTP method. Default: GET.
	Method string `yaml:"method" json:"method,omitempty"`

	// Request path appended to the resolved target URL.
	Path string `yaml:"path" json:"path,omitempty"`

	// Host header to send (e.g. for ingress routing).
	Host string `yaml:"host" json:"host,omitempty"`

	// Request headers.
	Headers map[string]string `yaml:"headers" json:"headers,omitempty"`

	// Request body; serialized to JSON when not a string.
	Body any `yaml:"body" json:"body,omitempty"`

	// Per-attempt timeout as a Go duration (e.g. 30s). Default 30s.
	Timeout string `yaml:"timeout" json:"timeout,omitempty"`

	// Retry policy for the request.
	Retries *HTTPRetries `yaml:"retries" json:"retries,omitempty"`

	// Conditions the response must satisfy.
	Assert *HTTPAssertBlock `yaml:"assert" json:"assert,omitempty"`
}

// HTTPTarget identifies the Kubernetes resource to reach (Service, Ingress, etc.).
type HTTPTarget struct {
	// Target resource kind.
	Kind string `yaml:"kind" json:"kind,omitempty"`

	// Target resource name.
	Name string `yaml:"name" json:"name,omitempty"`

	// Target resource namespace.
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`

	// Target port number.
	Port int `yaml:"port" json:"port,omitempty"`

	// Direct URL (only with via: direct).
	URL string `yaml:"url" json:"url,omitempty"`
}

// HTTPRetries is the retry policy for an HTTP assertion.
type HTTPRetries struct {
	// Maximum number of retries.
	Count int `yaml:"count" json:"count,omitempty"`

	// Delay between retries as a Go duration. Default 1s.
	Interval string `yaml:"interval" json:"interval,omitempty"`

	// CEL expression over status/body/headers; retries until it is true.
	Until string `yaml:"until" json:"until,omitempty"`
}

// HTTPAssertBlock is the set of response conditions an HTTP assertion checks.
type HTTPAssertBlock struct {
	// Expected HTTP status code.
	Status int `yaml:"status" json:"status,omitempty"`

	// Expected response headers; a value wrapped in /.../ is matched as a regex.
	Headers map[string]string `yaml:"headers" json:"headers,omitempty"`

	// Expected response body (used by some helpers).
	Body any `yaml:"body" json:"body,omitempty"`

	// Substring the response body must contain.
	BodyContains string `yaml:"bodyContains" json:"bodyContains,omitempty"`

	// Map of $.json.path expressions to expected values (compared as strings).
	JSONPath map[string]any `yaml:"jsonPath" json:"jsonPath,omitempty"`
}

// LookupAssert fetches a live resource and runs nested assertions on it. Apply tiers simulated and e2e only.
type LookupAssert struct {
	// Resource kind to fetch (e.g. Deployment, Service).
	Kind string `yaml:"kind" json:"kind"`

	// Resource name for a single get. Use this or labelSelector.
	Name string `yaml:"name" json:"name,omitempty"`

	// Namespace to look in. Defaults to the test's per-test namespace.
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`

	// Label selector for a list lookup; pair with forEach to assert per item.
	LabelSelector string `yaml:"labelSelector" json:"labelSelector,omitempty"`

	// Max time to poll for the resource as a Go duration (e.g. 30s). Default 2m.
	Within string `yaml:"within" json:"within,omitempty"`

	// Path/value condition that must hold before the then assertions run.
	Until *PathValue `yaml:"until" json:"until,omitempty"`

	// Assertions run against the single fetched resource.
	Then []Assertion `yaml:"then" json:"then,omitempty"`

	// Assertions run against each resource matched by labelSelector.
	ForEach []Assertion `yaml:"forEach" json:"forEach,omitempty"`
}

// LogsAssert asserts a pod's logs contain a pattern within a timeout. e2e tier only.
type LogsAssert struct {
	// Selects the pod(s) whose logs are scanned.
	Pod LogsPodSelector `yaml:"pod" json:"pod"`

	// Container name to read logs from; defaults to the pod's first container.
	Container string `yaml:"container" json:"container,omitempty"`

	// Substring to find, or /regex/ when wrapped in slashes.
	Pattern string `yaml:"pattern" json:"pattern"`

	// Max time to stream logs as a Go duration (e.g. 30s). Default 30s.
	Within string `yaml:"within" json:"within,omitempty"`
}

// LogsPodSelector selects the pod(s) whose logs are scanned.
type LogsPodSelector struct {
	// Pod name. Use this or labelSelector.
	Name string `yaml:"name" json:"name,omitempty"`

	// Label selector matching one or more pods.
	LabelSelector string `yaml:"labelSelector" json:"labelSelector,omitempty"`

	// Namespace of the pod(s). Defaults to the test's per-test namespace.
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`
}

// EventAssert asserts a Kubernetes Event was emitted for the involved object. Apply tiers simulated and e2e only.
type EventAssert struct {
	// Filters events by their involvedObject. Empty fields match any.
	InvolvedObject EventObjectRef `yaml:"involvedObject" json:"involvedObject,omitempty"`

	// Expected event reason (e.g. Scheduled, Pulled). Empty matches any.
	Reason string `yaml:"reason" json:"reason,omitempty"`

	// Expected event type. Empty matches any.
	Type string `yaml:"type" json:"type,omitempty" jsonschema:"enum=Normal,enum=Warning"`

	// Max time to poll for the event as a Go duration (e.g. 30s). Default 2m.
	Within string `yaml:"within" json:"within,omitempty"`
}

// EventObjectRef filters events by their involvedObject. Empty fields match any.
type EventObjectRef struct {
	// Object kind.
	Kind string `yaml:"kind" json:"kind,omitempty"`

	// Object name.
	Name string `yaml:"name" json:"name,omitempty"`

	// Object namespace.
	Namespace string `yaml:"namespace" json:"namespace,omitempty"`
}
