package runner

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/fregateops/vigie/internal/cel"
	"github.com/fregateops/vigie/internal/clog"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/matchers"
	"github.com/fregateops/vigie/internal/matrix"
	"github.com/fregateops/vigie/internal/render"
	"github.com/fregateops/vigie/internal/snapshot"
)

// TestResult is the outcome of a single test case.
type TestResult struct {
	SuiteName  string
	TestName   string
	Pass       bool
	Failures   []string
	Skipped    bool
	SkipReason string        // populated when test.skip is a non-empty string
	Duration   time.Duration // wall-clock time spent running this test
}

// SuiteResult groups results from one test file.
type SuiteResult struct {
	File     string
	Suite    string
	Results  []TestResult
	Duration time.Duration // wall-clock time spent running the file
}

// Options controls runner behavior.
type Options struct {
	ChartPath       string
	TestFiles       []string
	Parallelism     int
	Cfg             *config.Config
	SnapshotDir     string // default: "<chartPath>/tests/snapshots"
	SnapshotUpdate  bool
	ValidateSchemas bool   // when true, kubeconform-validate each test's rendered docs
	KubeVersion     string // kubernetes version for schema validation; empty = config.DefaultKubeVersion
	// Match is a regex evaluated against expanded test display names. Empty
	// runs every discovered test.
	Match string
	// FailFast cancels queued files after the first test failure. In-flight
	// files run to completion.
	FailFast bool
}

// resolveSnapshotDir returns the snapshot directory: the explicit override
// when set, otherwise the per-chart default "<chartPath>/tests/snapshots".
func resolveSnapshotDir(override, chartPath string) string {
	if override != "" {
		return override
	}
	return filepath.Join(chartPath, "tests", "snapshots")
}

// expandedTest wraps a dsl.Test with its matrix/case bindings and display name.
type expandedTest struct {
	Test        dsl.Test
	MatrixEntry map[string]any
	CaseEntry   map[string]any
	DisplayName string
}

// Run executes all test files and returns suite-level results.
func Run(opts Options) ([]SuiteResult, error) {
	slog.Debug("starting runner", "files", len(opts.TestFiles), "parallelism", opts.Parallelism, "validateSchemas", opts.ValidateSchemas)

	// Build one schema validator shared across all files/tests; its in-memory
	// schema cache then amortises across every rendered document.
	var schemaValidator *render.SchemaValidator
	if opts.ValidateSchemas {
		kubeVer := opts.KubeVersion
		if kubeVer == "" {
			kubeVer = config.DefaultKubeVersion
		}
		sv, err := render.NewSchemaValidator(kubeVer)
		if err != nil {
			return nil, fmt.Errorf("setup error: %w", err)
		}
		schemaValidator = sv
		slog.Debug("schema validator ready", "kubeVersion", kubeVer)
	}

	matchRE, err := compileMatchRegex(opts.Match)
	if err != nil {
		return nil, err
	}

	// Fail-fast cancels files not yet started once any file reports a failure;
	// in-flight files still run to completion. Template runs are CPU-only, so
	// the context only gates the queue, not the render itself.
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	var failFastOnce sync.Once

	return runParallel(opts.TestFiles, opts.Parallelism, func(_ int, path string) (SuiteResult, error) {
		select {
		case <-runCtx.Done():
			slog.Debug("test: file cancelled (fail-fast)", "file", path)
			return SuiteResult{File: path}, nil
		default:
		}

		sr, fileErr := runFile(path, opts, schemaValidator, matchRE)
		if opts.FailFast && fileErr == nil && SuiteHasFailure(sr) {
			failFastOnce.Do(func() {
				slog.Debug("test: fail-fast triggered", "file", path)
				cancelRun()
			})
		}
		return sr, fileErr
	})
}

func runFile(filePath string, opts Options, sv *render.SchemaValidator, matchRE *regexp.Regexp) (SuiteResult, error) {
	slog.Debug("loading test file", "file", filePath)
	start := time.Now()

	suite, err := dsl.ParseFile(filePath)
	if err != nil {
		return SuiteResult{}, fmt.Errorf("setup error: %w", err)
	}
	dsl.MergeInputs(suite)

	sr := SuiteResult{File: filePath, Suite: suite.SuiteName}

	// EXPAND phase: matrix/cases expansion.
	expanded, err := expandTests(suite.Tests)
	if err != nil {
		return SuiteResult{}, fmt.Errorf("expand error: %w", err)
	}
	slog.Debug("expanded test cases", "suite", suite.SuiteName, "count", len(expanded))

	// Resolve snapshot store.
	store := &snapshot.Store{Dir: resolveSnapshotDir(opts.SnapshotDir, opts.ChartPath), Update: opts.SnapshotUpdate}

	for _, et := range expanded {
		if matchRE != nil && !matchRE.MatchString(et.DisplayName) {
			slog.Debug("skipping test (no match)", "test", et.DisplayName)
			continue
		}
		sr.Results = append(sr.Results, runTest(et, suite, opts, store, sv))
	}
	sr.Duration = time.Since(start)
	slog.Debug("suite finished", "suite", suite.SuiteName, "tests", len(sr.Results), "duration", sr.Duration)
	return sr, nil
}

// expandTests expands all matrix/cases for a list of tests.
func expandTests(tests []dsl.Test) ([]expandedTest, error) {
	var out []expandedTest
	for _, test := range tests {
		switch {
		case test.Matrix != nil:
			entries, err := matrix.Expand(test.Matrix)
			if err != nil {
				return nil, fmt.Errorf("test %q matrix expand: %w", test.It, err)
			}
			for _, entry := range entries {
				// Wrap entry under "matrix" key so CEL expressions like
				// ${{ matrix.tier }} resolve correctly.
				bindings := map[string]any{"matrix": entry}
				interpolated, err := interpolateTest(test, bindings)
				if err != nil {
					return nil, fmt.Errorf("test %q matrix interpolate: %w", test.It, err)
				}
				name := fmt.Sprintf("%s [%s]", test.It, formatEntry(entry))
				out = append(out, expandedTest{
					Test:        interpolated,
					MatrixEntry: entry,
					DisplayName: name,
				})
			}

		case len(test.Cases) > 0:
			cases, err := matrix.ExpandCases(test.Cases)
			if err != nil {
				return nil, fmt.Errorf("test %q cases expand: %w", test.It, err)
			}
			for _, c := range cases {
				// Build case bindings: Extra + name + set.
				caseEntry := make(map[string]any, len(c.Extra)+2)
				for k, v := range c.Extra {
					caseEntry[k] = v
				}
				caseEntry["name"] = c.Name
				caseEntry["set"] = c.Set

				// Wrap caseEntry under "case" key so CEL expressions like
				// ${{ case.expectedReplicas }} resolve correctly.
				bindings := map[string]any{"case": caseEntry}
				interpolated, err := interpolateTest(test, bindings)
				if err != nil {
					return nil, fmt.Errorf("test %q case %q interpolate: %w", test.It, c.Name, err)
				}
				// Merge case.Set into interpolated test inputs.
				if c.Set != nil {
					if interpolated.Inputs == nil {
						interpolated.Inputs = &dsl.Inputs{}
					}
					if interpolated.Inputs.Set == nil {
						interpolated.Inputs.Set = make(map[string]any)
					}
					for k, v := range c.Set {
						interpolated.Inputs.Set[k] = v
					}
				}
				name := fmt.Sprintf("%s [%s]", test.It, c.Name)
				out = append(out, expandedTest{
					Test:        interpolated,
					CaseEntry:   caseEntry,
					DisplayName: name,
				})
			}

		default:
			out = append(out, expandedTest{
				Test:        test,
				DisplayName: test.It,
			})
		}
	}
	return out, nil
}

// interpolateTest applies ${{ }} interpolation over the entire test using bindings.
func interpolateTest(test dsl.Test, bindings map[string]any) (dsl.Test, error) {
	// Marshal to map[string]any via YAML.
	b, err := yaml.Marshal(test)
	if err != nil {
		return test, fmt.Errorf("interpolateTest marshal: %w", err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return test, fmt.Errorf("interpolateTest unmarshal to map: %w", err)
	}
	// Run interpolation.
	interpolated, err := matrix.Interpolate(raw, bindings)
	if err != nil {
		return test, fmt.Errorf("interpolateTest interpolate: %w", err)
	}
	// Marshal back to YAML and unmarshal into dsl.Test.
	b2, err := yaml.Marshal(interpolated)
	if err != nil {
		return test, fmt.Errorf("interpolateTest marshal result: %w", err)
	}
	var result dsl.Test
	if err := yaml.Unmarshal(b2, &result); err != nil {
		return test, fmt.Errorf("interpolateTest unmarshal result: %w", err)
	}
	return result, nil
}

// formatEntry formats a matrix entry as "k=v, k2=v2" (sorted keys).
func formatEntry(entry map[string]any) string {
	keys := make([]string, 0, len(entry))
	for k := range entry {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, entry[k]))
	}
	return strings.Join(parts, ", ")
}

func runTest(et expandedTest, suite *dsl.Suite, opts Options, store *snapshot.Store, sv *render.SchemaValidator) (tr TestResult) {
	test := et.Test
	tr = TestResult{SuiteName: suite.SuiteName, TestName: et.DisplayName}
	start := time.Now()
	defer func() {
		tr.Duration = time.Since(start)
		slog.Debug("test finished",
			"suite", suite.SuiteName, "test", et.DisplayName,
			"pass", tr.Pass, "skipped", tr.Skipped, "duration", tr.Duration)
	}()

	slog.Debug("running test", "suite", suite.SuiteName, "test", et.DisplayName)

	if skipped, reason := skipDecision(test.Skip); skipped {
		tr.Skipped = true
		tr.SkipReason = reason
		tr.Pass = true
		slog.Debug("skipping test", "suite", suite.SuiteName, "test", et.DisplayName, "reason", reason)
		return tr
	}

	// Helper test: test.Call != "".
	if test.Call != "" {
		return runHelperTest(et, suite, opts, store)
	}

	req := buildRenderRequest(test, suite, opts)
	clog.Trace("render request",
		"suite", suite.SuiteName, "test", et.DisplayName,
		"templates", req.Templates, "release", req.ReleaseName,
		"namespace", req.Namespace, "valueKeys", mapKeys(req.Values),
		"kubeVersion", req.KubeVersion)
	renderResult, renderErr := render.Render(req)

	var allDocs []map[string]any
	if renderResult != nil {
		allDocs = renderResult.Docs
	}

	// Schema validation (validate tier), on by default unless --no-schema.
	if sv != nil && renderErr == nil && len(allDocs) > 0 {
		schemaErrs, err := sv.Validate(allDocs)
		if err != nil {
			tr.Failures = append(tr.Failures, fmt.Sprintf("        schema validation error: %v", err))
		}
		for _, se := range schemaErrs {
			tr.Failures = append(tr.Failures, fmt.Sprintf("        schema: %s/%s: %s", se.Kind, se.Name, se.Message))
		}
	}

	evaluateAssertions(&tr, et, suite, allDocs, renderErr, store, applyEvalExtras{})

	tr.Pass = len(tr.Failures) == 0
	return tr
}

// runHelperTest handles tests with call: (helper template tests).
func runHelperTest(et expandedTest, suite *dsl.Suite, opts Options, store *snapshot.Store) TestResult {
	test := et.Test
	tr := TestResult{SuiteName: suite.SuiteName, TestName: et.DisplayName}

	// Resolve helper file paths relative to the chart root.
	resolvedHelpers := make([]string, len(suite.Helpers))
	for i, h := range suite.Helpers {
		if filepath.IsAbs(h) {
			resolvedHelpers[i] = h
		} else {
			resolvedHelpers[i] = filepath.Join(opts.ChartPath, h)
		}
	}

	clog.Trace("calling helper",
		"suite", suite.SuiteName, "test", et.DisplayName,
		"helper", test.Call, "outputAs", test.OutputAs,
		"helperFiles", resolvedHelpers, "argKeys", mapKeys(test.Args))
	helperResult, err := render.CallHelper(resolvedHelpers, test.Call, test.Args, test.OutputAs)
	if err != nil {
		tr.Failures = append(tr.Failures, fmt.Sprintf("helper render failed: %v", err))
		tr.Pass = false
		return tr
	}

	for i, assertion := range test.Asserts {
		ctx := matchers.EvalContext{
			IsHelperTest:  true,
			MatrixEntry:   et.MatrixEntry,
			CaseEntry:     et.CaseEntry,
			SuiteName:     suite.SuiteName,
			TestName:      et.DisplayName,
			AssertIdx:     i,
			SnapshotStore: store,
		}
		// For string outputAs, HelperOutput is the raw string.
		if test.OutputAs == "" || test.OutputAs == "string" {
			ctx.HelperOutput = helperResult.Raw
		} else {
			ctx.HelperOutput = helperResult.Parsed
		}

		result := matchers.Evaluate(assertion, ctx)
		if !result.Pass {
			tr.Failures = append(tr.Failures, fmt.Sprintf("        → %s", result.Message))
		}
	}

	tr.Pass = len(tr.Failures) == 0
	return tr
}

func buildRenderRequest(test dsl.Test, suite *dsl.Suite, opts Options) render.Request {
	req := render.Request{
		ChartPath:   opts.ChartPath,
		Templates:   suite.Templates,
		ReleaseName: opts.Cfg.Defaults.Release.Name,
		Namespace:   opts.Cfg.Defaults.Release.Namespace,
	}

	if test.Inputs != nil {
		if test.Inputs.Release != nil {
			if test.Inputs.Release.Name != "" {
				req.ReleaseName = test.Inputs.Release.Name
			}
			if test.Inputs.Release.Namespace != "" {
				req.Namespace = test.Inputs.Release.Namespace
			}
		}
		vals := map[string]any{}
		for k, v := range test.Inputs.Set {
			vals[k] = v
		}
		req.Values = vals
		if test.Inputs.Capabilities != nil {
			req.KubeVersion = test.Inputs.Capabilities.KubeVersion
			req.APIVersions = test.Inputs.Capabilities.APIVersions
		}
	}

	return req
}

// selectAllDocs returns all docs matching the target (for forEach).
func selectAllDocs(target *dsl.TargetSpec, docs []map[string]any) []map[string]any {
	if target == nil {
		return docs
	}
	var matches []map[string]any
	for _, doc := range docs {
		if matchesTarget(doc, target) {
			matches = append(matches, doc)
		}
	}
	return matches
}

// selectDoc picks the target document for an assertion.
// Returns (doc, diagnosticMessage).
func selectDoc(a dsl.Assertion, target *dsl.TargetSpec, docs []map[string]any, renderErr error) (map[string]any, string) {
	if a.FailedTemplate != nil || a.HasDocuments != nil {
		return nil, ""
	}

	effective := target
	if a.On != nil {
		effective = a.On
	}

	if effective == nil {
		if len(docs) == 1 {
			return docs[0], ""
		}
		if len(docs) == 0 {
			return nil, "        (no documents were rendered)"
		}
		return docs[0], ""
	}

	if effective.DocumentIndex != nil {
		idx := *effective.DocumentIndex
		if idx >= len(docs) {
			return nil, fmt.Sprintf("        (documentIndex %d out of range, %d documents rendered)", idx, len(docs))
		}
		return docs[idx], ""
	}

	var matches []map[string]any
	for _, doc := range docs {
		if matchesTarget(doc, effective) {
			matches = append(matches, doc)
		}
	}
	if len(matches) == 1 {
		return matches[0], ""
	}
	if len(matches) == 0 {
		return nil, buildTargetDiagnostic(effective, docs)
	}
	return matches[0], ""
}

func matchesTarget(doc map[string]any, target *dsl.TargetSpec) bool {
	// CEL expression takes priority if set.
	if target.Expr != "" {
		result, err := cel.Eval(target.Expr, map[string]any{"doc": doc})
		if err != nil {
			return false
		}
		pass, ok := result.(bool)
		if !ok {
			return false
		}
		return pass
	}

	if target.Kind != "" && fmt.Sprintf("%v", doc["kind"]) != target.Kind {
		return false
	}
	if target.APIVersion != "" && fmt.Sprintf("%v", doc["apiVersion"]) != target.APIVersion {
		return false
	}
	if target.Name != "" {
		name := ""
		if meta, ok := doc["metadata"].(map[string]any); ok {
			name = fmt.Sprintf("%v", meta["name"])
		}
		if name != target.Name {
			return false
		}
	}
	if target.Namespace != "" {
		namespace := ""
		if meta, ok := doc["metadata"].(map[string]any); ok {
			namespace = fmt.Sprintf("%v", meta["namespace"])
		}
		if namespace != target.Namespace {
			return false
		}
	}
	if len(target.Labels) > 0 {
		meta, ok := doc["metadata"].(map[string]any)
		if !ok {
			return false
		}
		labels, ok := meta["labels"].(map[string]any)
		if !ok {
			return false
		}
		for k, v := range target.Labels {
			if fmt.Sprintf("%v", labels[k]) != v {
				return false
			}
		}
	}
	return true
}

func buildTargetDiagnostic(target *dsl.TargetSpec, docs []map[string]any) string {
	if target == nil {
		return "        (no documents were rendered)"
	}
	msg := fmt.Sprintf("        (no document matched kind=%q name=%q — rendered:", target.Kind, target.Name)
	for _, doc := range docs {
		kind := fmt.Sprintf("%v", doc["kind"])
		name := ""
		if meta, ok := doc["metadata"].(map[string]any); ok {
			name = fmt.Sprintf("%v", meta["name"])
		}
		msg += fmt.Sprintf(" %s/%s", kind, name)
	}
	msg += ")"
	return msg
}

// skipDecision reports whether the test should be skipped, and the reason
// when skip is a non-empty string.
func skipDecision(skip any) (bool, string) {
	switch v := skip.(type) {
	case bool:
		return v, ""
	case string:
		return v != "", v
	}
	return false, ""
}

// mapKeys returns a sorted slice of the keys of m (used for trace logs so we
// don't dump arbitrary user values into stderr).
func mapKeys(m map[string]any) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// matcherKind returns the field name of the populated matcher in a, or
// "<unknown>" if none is set.
func matcherKind(a dsl.Assertion) string {
	switch {
	case a.Equal != nil:
		return "equal"
	case a.NotEqual != nil:
		return "notEqual"
	case a.GreaterThan != nil:
		return "greaterThan"
	case a.LessThan != nil:
		return "lessThan"
	case a.GTE != nil:
		return "gte"
	case a.LTE != nil:
		return "lte"
	case a.Contains != nil:
		return "contains"
	case a.NotContains != nil:
		return "notContains"
	case a.StartsWith != nil:
		return "startsWith"
	case a.EndsWith != nil:
		return "endsWith"
	case a.MatchRegex != nil:
		return "matchRegex"
	case a.NotMatchRegex != nil:
		return "notMatchRegex"
	case a.MatchTemplate != nil:
		return "matchTemplate"
	case a.Exists != nil:
		return "exists"
	case a.NotExists != nil:
		return "notExists"
	case a.IsNull != nil:
		return "isNull"
	case a.IsNotNull != nil:
		return "isNotNull"
	case a.IsEmpty != nil:
		return "isEmpty"
	case a.IsNotEmpty != nil:
		return "isNotEmpty"
	case a.IsType != nil:
		return "isType"
	case a.LengthEqual != nil:
		return "lengthEqual"
	case a.IsSubset != nil:
		return "isSubset"
	case a.IsKind != nil:
		return "isKind"
	case a.IsAPIVersion != nil:
		return "isAPIVersion"
	case a.HasDocuments != nil:
		return "hasDocuments"
	case a.FailedTemplate != nil:
		return "failedTemplate"
	case len(a.AllOf) > 0:
		return "allOf"
	case len(a.AnyOf) > 0:
		return "anyOf"
	case a.Expr != nil:
		return "expr"
	case a.MatchSnapshot != nil:
		return "matchSnapshot"
	case a.MatchSchema != nil:
		return "matchSchema"
	}
	return "<unknown>"
}

// AnyFailed returns true if any test failed (not skipped).
func AnyFailed(results []SuiteResult) bool {
	for _, sr := range results {
		if SuiteHasFailure(sr) {
			return true
		}
	}
	return false
}

// SuiteHasFailure reports whether sr contains any non-skipped failing test.
func SuiteHasFailure(sr SuiteResult) bool {
	for _, result := range sr.Results {
		if !result.Pass && !result.Skipped {
			return true
		}
	}
	return false
}

// DiscoverTestFiles walks the configured tests directory recursively and
// returns every `*_test.yaml` file that declares the unit-suite shape (no
// top-level `cluster:` or `dependencies:`). testsDir overrides the default
// `<chartPath>/tests` root; relative testsDir resolves against chartPath.
func DiscoverTestFiles(chartPath, testsDir string) ([]string, error) {
	return discoverTestFiles(chartPath, testsDir, dsl.UnitSuiteKind)
}

// resolveTestsRoot resolves the user-configured testsDir against the chart
// directory. Empty testsDir falls back to `<chartPath>/tests`. Returns the
// absolute path the walker should crawl.
func resolveTestsRoot(chartPath, testsDir string) string {
	if testsDir == "" {
		return filepath.Join(chartPath, "tests")
	}
	if filepath.IsAbs(testsDir) {
		return testsDir
	}
	return filepath.Join(chartPath, testsDir)
}

// discoverTestFiles walks resolveTestsRoot(chartPath, testsDir) recursively for
// files matching `*_test.yaml`, filters by suite kind, and returns the results
// sorted by path. An empty `want` returns every test file regardless of shape.
// A missing root directory yields an empty slice (matches the pre-issue-64
// behaviour of glob-against-missing-dir).
func discoverTestFiles(chartPath, testsDir string, want dsl.SuiteKind) ([]string, error) {
	root := resolveTestsRoot(chartPath, testsDir)

	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat tests dir %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("tests dir %s is not a directory", root)
	}

	var matches []string
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.yaml") {
			return nil
		}
		if want == "" {
			matches = append(matches, path)
			return nil
		}
		kind, kerr := dsl.DetectKind(path)
		if kerr != nil {
			return kerr
		}
		if kind == want {
			matches = append(matches, path)
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("scanning tests dir %s: %w", root, walkErr)
	}
	sort.Strings(matches)
	return matches, nil
}
