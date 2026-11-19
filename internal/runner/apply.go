package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/fregateops/vigie/internal/clog"
	"github.com/fregateops/vigie/internal/cluster"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/deps"
	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/kubeclient"
	"github.com/fregateops/vigie/internal/render"
	"github.com/fregateops/vigie/internal/snapshot"
)

// ApplyOptions configures the apply tier of `vigie test` (--cluster). The
// caller is responsible for constructing Backend (via cluster.New); BackendType
// records which Config backend value was resolved so the runner can honour tier
// filters and gate integration-only features (deps, hooks) that some backends
// don't support.
type ApplyOptions struct {
	ChartPath   string
	TestFiles   []string
	Parallelism int
	Cfg         *config.Config
	// Backend is the cluster backend already constructed via cluster.New.
	Backend cluster.Backend
	// BackendType records the resolved backend type ("envtest", "kubeconfig",
	// ...). Used for tier filtering and to gate integration-only features that
	// some backends do not support.
	BackendType string
	// Match is a regex evaluated against expanded test display names. Empty
	// runs every discovered test.
	Match string
	// SnapshotDir lets callers override where snapshot files live. Empty
	// defaults to <ChartPath>/tests/snapshots.
	SnapshotDir  string
	SnapshotMode snapshot.Mode
	// FailFast cancels queued tests after the first test failure. In-flight
	// tests run to completion and clean up.
	FailFast bool
	// Images are local container images loaded into the cluster's nodes before
	// any test runs (kind/k3d), so a chart can be tested against an image the
	// CI job just built without pushing it to a registry.
	Images []string
	// KeepCluster preserves the cluster after the run for manual debugging.
	// Honoured by node-backed backends; envtest tears down regardless.
	KeepCluster bool
}

// RunApply runs the apply tier of `vigie test --cluster`: it starts the
// configured cluster backend, walks each test file through the apply-tier state machine
// (LOAD -> EXPAND -> EXECUTE -> REPORT), then stops the backend (unless
// KeepCluster). A suite declaring integration-only features (dependencies,
// lifecycle hooks) the active tier cannot provide is skipped whole rather than
// run with those features dropped.
func RunApply(ctx context.Context, opts ApplyOptions) ([]SuiteResult, error) {
	slog.Debug("starting apply runner",
		"files", len(opts.TestFiles), "parallelism", opts.Parallelism,
		"backend", opts.BackendType, "match", opts.Match, "keepCluster", opts.KeepCluster)

	if len(opts.TestFiles) == 0 {
		return nil, nil
	}

	matchRE, err := compileMatchRegex(opts.Match)
	if err != nil {
		return nil, err
	}

	if opts.Backend == nil {
		return nil, errors.New("setup error: ApplyOptions.Backend is nil")
	}

	clog.Progress("apply: starting %s backend (this may take a moment)", opts.BackendType)
	if err := opts.Backend.Start(ctx); err != nil {
		return nil, fmt.Errorf("setup error: starting %s backend: %w", opts.BackendType, err)
	}
	defer stopBackend(opts)

	if len(opts.Images) > 0 {
		clog.Progress("apply: loading %d image(s) into the cluster: %s", len(opts.Images), strings.Join(opts.Images, ", "))
		if err := opts.Backend.LoadImages(ctx, opts.Images); err != nil {
			return nil, fmt.Errorf("setup error: loading images: %w", err)
		}
	}

	restCfg := opts.Backend.RESTConfig()
	if restCfg == nil {
		return nil, fmt.Errorf("setup error: %s backend returned nil REST config", opts.BackendType)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("setup error: building kube clientset: %w", err)
	}

	if kc := opts.Backend.Kubeconfig(); kc != "" {
		clog.Progress("apply: backend ready (kubeconfig=%s); running %d file(s)", kc, len(opts.TestFiles))
	} else {
		clog.Progress("apply: backend ready; running %d file(s)", len(opts.TestFiles))
	}

	runner := &applyRunner{
		opts:       opts,
		kubeCfg:    restCfg,
		kubeconfig: opts.Backend.Kubeconfig(),
		clientset:  clientset,
		matchRE:    matchRE,
		activeTier: backendTier(opts.BackendType),
	}

	return runner.run(ctx)
}

// applyRunner is the per-run state for RunApply.
type applyRunner struct {
	opts       ApplyOptions
	kubeCfg    *rest.Config
	kubeconfig string
	clientset  *kubernetes.Clientset
	matchRE    *regexp.Regexp
	// activeTier is the tier the backend runs at ("apiserver" for envtest,
	// "e2e" for real-cluster backends), compared against what each matcher and
	// each suite's integration features require.
	activeTier string
}

// stopBackend honours --keep-cluster and uses a detached context so teardown
// runs even when the parent context has been cancelled (Ctrl-C).
func stopBackend(opts ApplyOptions) {
	if opts.KeepCluster {
		if kc := opts.Backend.Kubeconfig(); kc != "" {
			clog.Progress("apply: --keep-cluster: leaving cluster running (kubeconfig=%s)", kc)
		} else {
			clog.Progress("apply: --keep-cluster: leaving cluster running")
		}
		return
	}
	clog.Progress("apply: tearing down %s backend", opts.BackendType)
	stopCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if stopErr := opts.Backend.Stop(stopCtx); stopErr != nil {
		slog.Warn("stopping backend", "backend", opts.BackendType, "err", stopErr)
	}
}

// run drives the file-parallel execution loop. Each file is parsed and run
// independently.
func (r *applyRunner) run(parent context.Context) ([]SuiteResult, error) {
	parallelism := r.opts.Parallelism
	if parallelism <= 0 {
		parallelism = runtime.NumCPU()
	}

	runCtx, cancelRun := context.WithCancel(parent)
	defer cancelRun()

	var failFastOnce sync.Once

	return runParallel(r.opts.TestFiles, parallelism, func(_ int, path string) (SuiteResult, error) {
		select {
		case <-runCtx.Done():
			slog.Debug("apply: file cancelled (fail-fast)", "file", path)
			return SuiteResult{File: path}, nil
		default:
		}

		sr, fileErr := r.runFile(runCtx, path)
		if r.opts.FailFast && fileErr == nil && SuiteHasFailure(sr) {
			failFastOnce.Do(func() {
				slog.Debug("apply: fail-fast triggered", "file", path)
				cancelRun()
			})
		}
		return sr, fileErr
	})
}

// runFile is the per-file state machine: parse -> install scoped deps ->
// beforeAll hooks -> run tests (with per-test namespace + scope-test deps +
// setup/teardown hooks) -> afterAll hooks -> teardown scoped deps. Suites that
// declare no integration-tier features (no `dependencies:`/`beforeAll:`/...)
// fall through the install + hook stages with empty payloads.
func (r *applyRunner) runFile(ctx context.Context, filePath string) (SuiteResult, error) {
	slog.Debug("loading test file", "file", filePath)
	start := time.Now()

	suite, expanded, err := loadSuite(filePath)
	if err != nil {
		return unloadedSuite(filePath, err, time.Since(start)), nil
	}
	slog.Debug("parsed suite", "file", filePath, "tier", suite.SuiteTier(), "suite", suite.SuiteName)

	sr := SuiteResult{File: filePath, Suite: suite.SuiteName}

	chrt, err := loader.Load(r.opts.ChartPath)
	if err != nil {
		return SuiteResult{File: filePath, Suite: suite.SuiteName}, fmt.Errorf("setup error: loading chart %s: %w", r.opts.ChartPath, err)
	}

	store := &snapshot.Store{Dir: resolveSnapshotDir(r.opts.SnapshotDir, r.opts.ChartPath), Mode: r.opts.SnapshotMode}

	baseDir := filepath.Dir(filePath)

	// A suite whose dependencies or hooks this tier cannot provide is skipped
	// whole: its tests' premises cannot be established, so running them anyway
	// reports on something other than what the author wrote.
	if skip, reason := integrationFeatureSkip(suite, r.activeTier); skip {
		slog.Debug("skipping suite (integration features unsupported)",
			"file", filePath, "backend", r.opts.BackendType, "reason", reason)
		sr.Results = skippedSuite(suite, expanded, r.matchRE, reason)
		sr.Duration = time.Since(start)
		return sr, nil
	}

	clusterDeps, suiteDeps, testDeps := splitDepsByScope(suite.Dependencies)

	clusterState, _, err := deps.Install(ctx, clusterDeps, r.kubeCfg, deps.InstallOptions{
		Parallelism: r.opts.Parallelism, BaseDir: baseDir,
	})
	if err != nil {
		return sr, fmt.Errorf("setup error: installing cluster-scoped deps: %w", err)
	}
	// Cluster-scoped deps persist across files for cache reuse; their teardown
	// will be tracked in a future change.
	_ = clusterState

	if len(suiteDeps) > 0 {
		clog.Progress("apply: %s — installing %d suite-scoped dependency(ies)", suite.SuiteName, len(suiteDeps))
	}
	suiteState, suiteExports, err := deps.Install(ctx, suiteDeps, r.kubeCfg, deps.InstallOptions{
		Parallelism: r.opts.Parallelism, BaseDir: baseDir,
	})
	if err != nil {
		return sr, fmt.Errorf("setup error: installing suite-scoped deps: %w", err)
	}
	defer func() {
		if r.opts.KeepCluster {
			slog.Info("--keep-cluster: preserving suite-scoped deps", "file", filePath)
			return
		}
		teardownCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := deps.Teardown(teardownCtx, suiteState); err != nil {
			slog.Warn("suite-scoped dep teardown failed", "file", filePath, "err", err)
		}
	}()

	suiteEnv := HookEnv{
		Kubeconfig: r.kubeconfig,
		Suite:      suite.SuiteName,
	}
	if err := RunHooks(ctx, "beforeAll", suite.BeforeAll, suiteEnv); err != nil {
		return sr, fmt.Errorf("setup error: beforeAll hook: %w", err)
	}
	defer func() {
		afterCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := RunHooks(afterCtx, "afterAll", suite.AfterAll, suiteEnv); err != nil {
			slog.Warn("afterAll hook failed", "file", filePath, "err", err)
		}
	}()

	clog.Progress("apply: %s — %d test(s), %d suite-dep(s), %d test-dep(s)",
		suite.SuiteName, len(expanded), len(suiteDeps), len(testDeps))

	for _, et := range expanded {
		if ctx.Err() != nil {
			slog.Debug("apply: run cancelled, not starting remaining tests", "file", filePath)
			break
		}
		if r.matchRE != nil && !r.matchRE.MatchString(et.DisplayName) {
			slog.Debug("skipping test (no match)", "test", et.DisplayName)
			continue
		}
		if skip, reason := matcherTierSkip(et.Test.Asserts, r.activeTier); skip {
			slog.Debug("skipping test (matcher tier requirement)",
				"test", et.DisplayName, "reason", reason)
			sr.Results = append(sr.Results, TestResult{
				SuiteName:  suite.SuiteName,
				TestName:   et.DisplayName,
				Pass:       true,
				Skipped:    true,
				SkipReason: reason,
			})
			continue
		}
		clog.Progress("apply: %s — running test: %s", suite.SuiteName, et.DisplayName)
		testStart := time.Now()
		tr := r.runTest(ctx, et, suite, chrt, store, baseDir, testDeps, suiteExports)
		sr.Results = append(sr.Results, tr)
		clog.Progress("apply: %s — %s", suite.SuiteName, formatTestProgress(tr, et.DisplayName, time.Since(testStart)))
	}
	sr.Duration = time.Since(start)
	slog.Debug("suite finished",
		"suite", suite.SuiteName, "tests", len(sr.Results), "duration", sr.Duration)
	return sr, nil
}

// runTest renders the chart, installs it into a fresh per-test namespace,
// installs test-scoped deps, evaluates matchers, runs lifecycle hooks, and
// tears the namespace down. Helper tests (Call != "") fall back to the
// template-tier helper path so mixed suites still produce results.
func (r *applyRunner) runTest(ctx context.Context, et expandedTest, suite *dsl.Suite, chrt *chart.Chart, store *snapshot.Store, baseDir string, testDeps []dsl.Dependency, suiteExports deps.ExportMap) (tr TestResult) {
	test := et.Test
	tr = TestResult{SuiteName: suite.SuiteName, TestName: et.DisplayName}
	start := time.Now()
	defer func() {
		tr.Duration = time.Since(start)
		slog.Debug("test finished",
			"suite", suite.SuiteName, "test", et.DisplayName,
			"pass", tr.Pass, "skipped", tr.Skipped, "duration", tr.Duration)
	}()

	if skipped, reason := skipDecision(test.Skip); skipped {
		tr.Skipped = true
		tr.SkipReason = reason
		tr.Pass = true
		return tr
	}

	if test.Call != "" {
		runOpts := Options{ChartPath: r.opts.ChartPath, Cfg: r.opts.Cfg}
		return runHelperTest(et, suite, runOpts, store)
	}

	namespace := allocateNamespace(et.DisplayName)
	if err := r.createNamespace(ctx, namespace); err != nil {
		tr.Failures = append(tr.Failures, fmt.Sprintf("        → namespace create failed: %v", err))
		tr.Pass = false
		return tr
	}
	defer r.deleteNamespace(namespace)

	testScopedForThis := narrowTestDepsToNamespace(testDeps, namespace)
	testState, testExports, err := deps.Install(ctx, testScopedForThis, r.kubeCfg, deps.InstallOptions{
		Parallelism: r.opts.Parallelism, BaseDir: baseDir,
	})
	if err != nil {
		tr.Failures = append(tr.Failures, fmt.Sprintf("        → test-scoped dep install: %v", err))
		tr.Pass = false
		return tr
	}
	defer func() {
		if r.opts.KeepCluster {
			return
		}
		teardownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := deps.Teardown(teardownCtx, testState); err != nil {
			slog.Warn("test-scoped dep teardown failed", "test", et.DisplayName, "err", err)
		}
	}()

	mergedExports := mergeExports(suiteExports, testExports)

	hookEnv := HookEnv{
		Kubeconfig: r.kubeconfig,
		Suite:      suite.SuiteName,
		Test:       et.DisplayName,
		Namespace:  namespace,
	}
	// No tier guard here: runFile skips any suite declaring hooks the active
	// tier cannot run, so reaching a test means they are available.
	if err := RunHooks(ctx, "setup", test.Setup, hookEnv); err != nil {
		tr.Failures = append(tr.Failures, fmt.Sprintf("        → setup hook: %v", err))
		tr.Pass = false
		return tr
	}
	defer func() {
		teardownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := RunHooks(teardownCtx, "teardown", test.Teardown, hookEnv); err != nil {
			slog.Warn("teardown hook failed", "test", et.DisplayName, "err", err)
		}
	}()

	if test.Inputs != nil {
		inputs := *test.Inputs
		inputs.Set = applyDepsBindings(inputs.Set, mergedExports)
		test.Inputs = &inputs
	}
	renderOpts := Options{ChartPath: r.opts.ChartPath, Cfg: r.opts.Cfg}
	req, err := buildRenderRequest(test, suite, renderOpts)
	if err != nil {
		tr.Failures = append(tr.Failures, fmt.Sprintf("        → %v", err))
		return tr
	}
	req.Namespace = namespace

	clog.Trace("apply render request",
		"suite", suite.SuiteName, "test", et.DisplayName,
		"templates", req.Templates, "release", req.ReleaseName,
		"namespace", req.Namespace, "valueKeys", mapKeys(req.Values))
	renderResult, renderErr := render.Render(req)

	var allDocs []map[string]any
	if renderResult != nil {
		allDocs = renderResult.Objects()
	}

	releaseName := req.ReleaseName
	vals := req.Values

	var installErr error
	if renderErr == nil {
		installErr = r.helmInstall(ctx, chrt, vals, releaseName, namespace)
		defer r.helmUninstall(releaseName, namespace)
		// Suppress the install-error append when the test declares a
		// `rejected:` matcher: rejection is the *expected* outcome and the
		// matcher itself reads installErr to decide pass/fail. Without this,
		// `rejected:` tests would fail twice (install + matcher).
		if installErr != nil && !testExpectsRejection(test) {
			tr.Failures = append(tr.Failures, fmt.Sprintf("        → helm install failed: %v", installErr))
		}
	}

	// After an unexpected install failure the assertions have nothing to
	// observe; evaluating them would only sit out every live matcher's timeout.
	if installErr == nil || testExpectsRejection(test) {
		evaluateAssertions(ctx, &tr, et, suite, allDocs, renderErr, store, applyEvalExtras{
			InApplyTier: true,
			ApplyError:  installErr,
			RESTConfig:  r.kubeCfg,
			Namespace:   namespace,
		})
	}
	if len(tr.Failures) > 0 && renderErr == nil && ctx.Err() == nil {
		tr.Failures = append(tr.Failures, namespaceDiagnostics(r.clientset, namespace)...)
	}
	tr.Pass = len(tr.Failures) == 0
	return tr
}

// helmHookTimeout bounds how long install and uninstall wait for chart hooks
// (e.g. a pre-install Job generating certificates) to complete.
const helmHookTimeout = 5 * time.Minute

func (r *applyRunner) helmConfig(namespace string) (*action.Configuration, error) {
	cfg := new(action.Configuration)
	getter := kubeclient.NewRESTGetter(r.kubeCfg, namespace)
	logFn := func(format string, args ...any) {
		slog.Debug("helm: " + fmt.Sprintf(format, args...))
	}
	if err := cfg.Init(getter, namespace, "secret", logFn); err != nil {
		return nil, fmt.Errorf("helm action.Configuration.Init: %w", err)
	}
	return cfg, nil
}

// runsChartHooks reports whether the tier can run chart hooks: a hook Job
// only completes where pods run, so below that tier every install would sit
// out helmHookTimeout instead.
func (r *applyRunner) runsChartHooks() bool {
	return dsl.TierProvides(r.activeTier)[dsl.RunningPods]
}

// helmInstall installs the chart against the live cluster. Wait=false so the
// install returns as soon as the API server accepts the resources; tests that
// need post-apply settling express it explicitly via waitFor / becomesReady /
// lookup matchers. Chart hooks run (and are waited for) where pods run.
func (r *applyRunner) helmInstall(ctx context.Context, chrt *chart.Chart, vals map[string]any, releaseName, namespace string) error {
	cfg, err := r.helmConfig(namespace)
	if err != nil {
		return err
	}
	install := action.NewInstall(cfg)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.DisableHooks = !r.runsChartHooks()
	install.Timeout = helmHookTimeout
	_, err = install.RunWithContext(ctx, chrt, vals)
	return err
}

// helmUninstall removes the test's release before its namespace goes, so the
// chart's cluster-scoped objects (ClusterRoles, webhook configurations, ...)
// do not outlive the test and collide with the next install. It runs on a
// detached context so an interrupted run still cleans up.
func (r *applyRunner) helmUninstall(releaseName, namespace string) {
	if r.opts.KeepCluster {
		return
	}
	cfg, err := r.helmConfig(namespace)
	if err != nil {
		slog.Warn("helm uninstall skipped", "release", releaseName, "namespace", namespace, "err", err)
		return
	}
	uninstall := action.NewUninstall(cfg)
	uninstall.DisableHooks = !r.runsChartHooks()
	uninstall.Timeout = helmHookTimeout
	uninstall.IgnoreNotFound = true
	if _, err := uninstall.Run(releaseName); err != nil {
		slog.Warn("helm uninstall failed; cluster-scoped objects of the release may remain",
			"release", releaseName, "namespace", namespace, "err", err)
	}
}

func (r *applyRunner) createNamespace(ctx context.Context, name string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	_, err := r.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// deleteNamespace tears down the per-test namespace fire-and-forget. Errors
// are logged but not surfaced because the test result already reflects
// success/failure. Uses GracePeriodSeconds=0 to avoid blocking on finalisers -
// envtest has no controllers to enforce them, and node-backed backends will
// finalise asynchronously after the runner exits.
func (r *applyRunner) deleteNamespace(name string) {
	if r.opts.KeepCluster {
		slog.Info("--keep-cluster: preserving test namespace", "namespace", name)
		return
	}
	gp := int64(0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := r.clientset.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: &gp,
	})
	if err != nil && !apierrors.IsNotFound(err) {
		slog.Debug("namespace delete failed", "namespace", name, "err", err)
	}
}

// compileMatchRegex compiles the --match flag value into a regexp. Returns nil
// when match is empty (meaning "run everything"). Must be called before the
// backend is started so user typos fail fast.
func compileMatchRegex(match string) (*regexp.Regexp, error) {
	if match == "" {
		return nil, nil
	}
	re, err := regexp.Compile(match)
	if err != nil {
		return nil, fmt.Errorf("invalid --match regex %q: %w", match, err)
	}
	return re, nil
}

// backendTier maps a cluster backend type to the tier label used by the
// `tier:` filter on individual tests. envtest implements the apiserver tier;
// every node-backed backend implements the e2e tier.
func backendTier(backendType string) string {
	switch backendType {
	case "envtest":
		return "apiserver"
	case "simulated":
		return "simulated"
	default:
		return "e2e"
	}
}

// testExpectsRejection reports whether any assertion (including those nested
// under allOf/anyOf) uses the `rejected:` matcher. Used by the apply-tier
// runner to know that a non-nil install error is the *expected* outcome
// for this test and shouldn't be appended as a failure on its own.
func testExpectsRejection(test dsl.Test) bool {
	return anyAssertion(test.Asserts, func(a dsl.Assertion) bool { return a.Rejected != nil })
}

// anyAssertion walks assertions (including allOf/anyOf nesting) and returns
// true as soon as match reports true.
func anyAssertion(asserts []dsl.Assertion, match func(dsl.Assertion) bool) bool {
	for _, a := range asserts {
		if match(a) {
			return true
		}
		if len(a.AllOf) > 0 && anyAssertion(a.AllOf, match) {
			return true
		}
		if len(a.AnyOf) > 0 && anyAssertion(a.AnyOf, match) {
			return true
		}
	}
	return false
}

// formatTestProgress renders a single end-of-test progress line. For failures
// it inlines the first failure message so users see *why* without scrolling to
// the final report.
func formatTestProgress(tr TestResult, displayName string, dur time.Duration) string {
	status := "PASS"
	switch {
	case tr.Skipped:
		status = "SKIP"
	case !tr.Pass:
		status = "FAIL"
	}
	durStr := dur.Round(time.Millisecond).String()
	if !tr.Pass && len(tr.Failures) > 0 {
		first := strings.TrimSpace(tr.Failures[0])
		first = strings.TrimPrefix(first, "→ ")
		if idx := strings.IndexByte(first, '\n'); idx >= 0 {
			first = first[:idx]
		}
		return fmt.Sprintf("%s %s (%s) — %s", status, displayName, durStr, first)
	}
	return fmt.Sprintf("%s %s (%s)", status, displayName, durStr)
}

// splitDepsByScope partitions deps by scope. The default scope (empty string)
// is treated as "suite" to match DESIGN.md §12.
func splitDepsByScope(allDeps []dsl.Dependency) (cluster, suite, test []dsl.Dependency) {
	for _, dep := range allDeps {
		switch dep.Scope {
		case "cluster":
			cluster = append(cluster, dep)
		case "test":
			test = append(test, dep)
		default:
			suite = append(suite, dep)
		}
	}
	return
}

// narrowTestDepsToNamespace clones each test-scoped dep with its namespace
// pinned to the per-test namespace when the dep didn't specify one. This lets
// users write `scope: test` deps without repeating namespaces.
func narrowTestDepsToNamespace(testDeps []dsl.Dependency, namespace string) []dsl.Dependency {
	if len(testDeps) == 0 {
		return nil
	}
	out := make([]dsl.Dependency, len(testDeps))
	for idx, dep := range testDeps {
		clone := dep
		if clone.Namespace == "" {
			clone.Namespace = namespace
		}
		out[idx] = clone
	}
	return out
}

// mergeExports overlays test-scope exports onto suite-scope exports. Test-scope
// entries win when keys collide.
func mergeExports(suite, test deps.ExportMap) deps.ExportMap {
	out := deps.ExportMap{}
	for name, kv := range suite {
		for key, val := range kv {
			out.Set(name, key, val)
		}
	}
	for name, kv := range test {
		for key, val := range kv {
			out.Set(name, key, val)
		}
	}
	return out
}

// applyDepsBindings walks a map of values and substitutes any string
// containing `${{ deps.<name>.<key> }}` with the matching export value.
// Other interpolation tokens are left untouched.
func applyDepsBindings(values map[string]any, exports deps.ExportMap) map[string]any {
	if len(values) == 0 || len(exports) == 0 {
		return values
	}
	out := make(map[string]any, len(values))
	for key, val := range values {
		out[key] = substituteDepsValue(val, exports)
	}
	return out
}

func substituteDepsValue(value any, exports deps.ExportMap) any {
	switch typed := value.(type) {
	case string:
		return substituteDepsString(typed, exports)
	case map[string]any:
		return applyDepsBindings(typed, exports)
	case []any:
		out := make([]any, len(typed))
		for idx, elem := range typed {
			out[idx] = substituteDepsValue(elem, exports)
		}
		return out
	default:
		return value
	}
}

var depsTokenRegex = regexp.MustCompile(`\$\{\{\s*deps\.([a-zA-Z0-9_-]+)\.([a-zA-Z0-9_-]+)\s*\}\}`)

func substituteDepsString(input string, exports deps.ExportMap) string {
	return depsTokenRegex.ReplaceAllStringFunc(input, func(match string) string {
		sub := depsTokenRegex.FindStringSubmatch(match)
		if len(sub) != 3 {
			return match
		}
		val, ok := exports.Get(sub[1], sub[2])
		if !ok {
			return match
		}
		return val
	})
}

// allocateNamespace returns a DNS-label-safe namespace name unique to a test
// run. The display name is sanitised to ASCII-lowercase + dashes; an 8-char
// random hex suffix prevents collisions across parallel workers and reruns.
//
// Kubernetes namespaces are limited to 63 chars, so we trim the sanitised
// portion to fit "vg-<name>-<8-char-suffix>".
func allocateNamespace(displayName string) string {
	const prefix = "vg-"
	const maxLen = 63
	suffix := randHex(8)
	budget := maxLen - len(prefix) - 1 - len(suffix)
	clean := sanitizeDNSLabel(displayName)
	if len(clean) > budget {
		clean = clean[:budget]
	}
	if clean == "" {
		clean = "test"
	}
	return prefix + clean + "-" + suffix
}

// sanitizeDNSLabel maps an arbitrary string to the DNS-1123 label alphabet
// (lower-case alphanumerics and dashes).
func sanitizeDNSLabel(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || r == ' ' || r == '/' || r == '.':
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	out := b.String()
	out = strings.Trim(out, "-")
	return out
}

// randHex returns n hex characters of cryptographic randomness. Falls back
// to a deterministic placeholder if the system RNG fails.
func randHex(n int) string {
	buf := make([]byte, (n+1)/2)
	if _, err := rand.Read(buf); err != nil {
		return strings.Repeat("0", n)
	}
	s := hex.EncodeToString(buf)
	if len(s) > n {
		s = s[:n]
	}
	return s
}
