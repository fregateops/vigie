package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/strvals"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/fregateops/vigie/internal/clog"
	"github.com/fregateops/vigie/internal/cluster"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/kubeclient"
	"github.com/fregateops/vigie/internal/matchers"
	"github.com/fregateops/vigie/internal/render"
	"github.com/fregateops/vigie/internal/snapshot"
)

// ApplyOptions configures `vigie test-apply`. The caller is responsible for
// constructing Backend (via cluster.New); BackendType records which Config
// backend value was resolved so the runner can honour tier filters.
type ApplyOptions struct {
	ChartPath   string
	TestFiles   []string
	Parallelism int
	Cfg         *config.Config
	// Backend is the cluster backend already constructed via cluster.New.
	Backend cluster.Backend
	// BackendType records the resolved backend type ("envtest", ...). Used for
	// tier filtering and progress messages.
	BackendType string
	// Match is a regex evaluated against expanded test display names. Empty
	// runs every discovered test.
	Match string
	// SnapshotDir lets callers override where snapshot files live. Empty
	// defaults to <ChartPath>/tests/snapshots.
	SnapshotDir string
	// FailFast cancels queued tests after the first test failure. In-flight
	// tests run to completion and clean up.
	FailFast bool
	// KeepCluster preserves the cluster after the run for manual debugging.
	// Honoured by node-backed backends; envtest tears down regardless.
	KeepCluster bool
}

// RunApply executes `vigie test-apply`: it starts the configured cluster
// backend, walks each test file through the apply-tier state machine
// (LOAD -> EXPAND -> EXECUTE -> REPORT), then stops the backend (unless
// KeepCluster). The api tier (envtest) installs each test's chart against a
// real apiserver and evaluates the assertions; dependencies and lifecycle
// hooks belong to the integration tiers and are not handled here.
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

	clog.Progress("test-apply: starting %s backend (this may take a moment)", opts.BackendType)
	if err := opts.Backend.Start(ctx); err != nil {
		return nil, fmt.Errorf("setup error: starting %s backend: %w", opts.BackendType, err)
	}
	defer stopBackend(opts)

	restCfg := opts.Backend.RESTConfig()
	if restCfg == nil {
		return nil, fmt.Errorf("setup error: %s backend returned nil REST config", opts.BackendType)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("setup error: building kube clientset: %w", err)
	}

	if kc := opts.Backend.Kubeconfig(); kc != "" {
		clog.Progress("test-apply: backend ready (kubeconfig=%s); running %d file(s)", kc, len(opts.TestFiles))
	} else {
		clog.Progress("test-apply: backend ready; running %d file(s)", len(opts.TestFiles))
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
	// activeTier is the value compared against each test's `tier:` field
	// ("apiserver" for envtest, "e2e" for real-cluster backends).
	activeTier string
}

// stopBackend honours --keep-cluster and uses a detached context so teardown
// runs even when the parent context has been cancelled (Ctrl-C).
func stopBackend(opts ApplyOptions) {
	if opts.KeepCluster {
		if kc := opts.Backend.Kubeconfig(); kc != "" {
			clog.Progress("test-apply: --keep-cluster: leaving cluster running (kubeconfig=%s)", kc)
		} else {
			clog.Progress("test-apply: --keep-cluster: leaving cluster running")
		}
		return
	}
	clog.Progress("test-apply: tearing down %s backend", opts.BackendType)
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
			slog.Debug("test-apply: file cancelled (fail-fast)", "file", path)
			return SuiteResult{File: path}, nil
		default:
		}

		sr, fileErr := r.runFile(runCtx, path)
		if r.opts.FailFast && fileErr == nil && SuiteHasFailure(sr) {
			failFastOnce.Do(func() {
				slog.Debug("test-apply: fail-fast triggered", "file", path)
				cancelRun()
			})
		}
		return sr, fileErr
	})
}

// runFile is the per-file state machine: parse -> expand -> run each test
// (with a per-test namespace) against the live backend. The api tier accepts
// both unit and integration suite shapes but ignores integration-only fields
// (dependencies, lifecycle hooks) - those land with the integration tiers.
func (r *applyRunner) runFile(ctx context.Context, filePath string) (SuiteResult, error) {
	slog.Debug("loading test file", "file", filePath)
	start := time.Now()

	suite, kind, err := dsl.ParseSuiteAuto(filePath)
	if err != nil {
		return SuiteResult{File: filePath}, fmt.Errorf("setup error: %w", err)
	}
	dsl.MergeInputs(suite)
	slog.Debug("parsed suite", "file", filePath, "kind", kind, "suite", suite.SuiteName)

	sr := SuiteResult{File: filePath, Suite: suite.SuiteName}

	expanded, err := expandTests(suite.Tests)
	if err != nil {
		return SuiteResult{File: filePath, Suite: suite.SuiteName}, fmt.Errorf("expand error: %w", err)
	}

	chrt, err := loader.Load(r.opts.ChartPath)
	if err != nil {
		return SuiteResult{File: filePath, Suite: suite.SuiteName}, fmt.Errorf("setup error: loading chart %s: %w", r.opts.ChartPath, err)
	}

	store := &snapshot.Store{Dir: resolveSnapshotDir(r.opts.SnapshotDir, r.opts.ChartPath)}

	clog.Progress("test-apply: %s — %d test(s)", suite.SuiteName, len(expanded))

	for _, et := range expanded {
		if r.matchRE != nil && !r.matchRE.MatchString(et.DisplayName) {
			slog.Debug("skipping test (no match)", "test", et.DisplayName)
			continue
		}
		if !tierApplies(et.Test.Tier, r.activeTier) {
			slog.Debug("skipping test (tier filter)", "test", et.DisplayName, "tier", et.Test.Tier)
			sr.Results = append(sr.Results, TestResult{
				SuiteName:  suite.SuiteName,
				TestName:   et.DisplayName,
				Pass:       true,
				Skipped:    true,
				SkipReason: fmt.Sprintf("tier %v excludes %s", et.Test.Tier, r.activeTier),
			})
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
		clog.Progress("test-apply: %s — running test: %s", suite.SuiteName, et.DisplayName)
		testStart := time.Now()
		tr := r.runTest(ctx, et, suite, chrt, store)
		sr.Results = append(sr.Results, tr)
		clog.Progress("test-apply: %s — %s", suite.SuiteName, formatTestProgress(tr, et.DisplayName, time.Since(testStart)))
	}
	sr.Duration = time.Since(start)
	slog.Debug("suite finished",
		"suite", suite.SuiteName, "tests", len(sr.Results), "duration", sr.Duration)
	return sr, nil
}

// runTest renders the chart, installs it into a fresh per-test namespace,
// evaluates matchers, and tears the namespace down. Helper tests (Call != "")
// fall back to the template-tier helper path so mixed suites still produce
// results.
func (r *applyRunner) runTest(ctx context.Context, et expandedTest, suite *dsl.Suite, chrt *chart.Chart, store *snapshot.Store) (tr TestResult) {
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

	renderOpts := Options{ChartPath: r.opts.ChartPath, Cfg: r.opts.Cfg}
	req := buildRenderRequest(test, suite, renderOpts)
	req.Namespace = namespace

	clog.Trace("apply render request",
		"suite", suite.SuiteName, "test", et.DisplayName,
		"templates", req.Templates, "release", req.ReleaseName,
		"namespace", req.Namespace, "valueKeys", mapKeys(req.Values))
	renderResult, renderErr := render.Render(req)

	var allDocs []map[string]any
	if renderResult != nil {
		allDocs = renderResult.Docs
	}

	releaseName := req.ReleaseName
	vals := buildValuesForInstall(test)

	var installErr error
	if renderErr == nil {
		installErr = r.helmInstall(ctx, chrt, vals, releaseName, namespace)
		// Suppress the install-error append when the test declares a
		// `rejected:` matcher: rejection is the *expected* outcome and the
		// matcher itself reads installErr to decide pass/fail. Without this,
		// `rejected:` tests would fail twice (install + matcher).
		if installErr != nil && !testExpectsRejection(test) {
			tr.Failures = append(tr.Failures, fmt.Sprintf("        → helm install failed: %v", installErr))
		}
	}

	evaluateAssertions(&tr, et, suite, allDocs, renderErr, store, applyEvalExtras{
		InApplyTier: true,
		ApplyError:  installErr,
		RESTConfig:  r.kubeCfg,
		Namespace:   namespace,
	})
	tr.Pass = len(tr.Failures) == 0
	return tr
}

// helmInstall installs the chart against the live cluster. Wait=false so the
// install returns as soon as the API server accepts the resources; tests that
// need post-apply settling express it explicitly via waitFor / becomesReady /
// lookup matchers. Hooks remain disabled - tests that need pre/post-install
// hooks should add them via setup/teardown.
func (r *applyRunner) helmInstall(ctx context.Context, chrt *chart.Chart, vals map[string]any, releaseName, namespace string) error {
	cfg := new(action.Configuration)
	getter := kubeclient.NewRESTGetter(r.kubeCfg, namespace)
	logFn := func(format string, args ...any) {
		slog.Debug("helm: " + fmt.Sprintf(format, args...))
	}
	if err := cfg.Init(getter, namespace, "secret", logFn); err != nil {
		return fmt.Errorf("helm action.Configuration.Init: %w", err)
	}

	install := action.NewInstall(cfg)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.CreateNamespace = false
	install.Wait = false
	install.DisableHooks = true
	install.DryRun = false
	install.Replace = false
	install.ClientOnly = false

	_, err := install.RunWithContext(ctx, chrt, vals)
	return err
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

// DiscoverApplyTestFiles walks the configured tests directory recursively and
// returns every `*_test.yaml` file regardless of suite shape (unit vs.
// integration). The runner branches on shape per-file at parse time - the
// caller no longer needs a `unit/` vs `integration/` split.
func DiscoverApplyTestFiles(chartPath, testsDir string) ([]string, error) {
	return discoverTestFiles(chartPath, testsDir, "")
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

// matcherTierSkip decides whether a test must be skipped because one of its
// matchers does not support the active backend's tier. The returned reason
// names the offending matcher so users can see *which* assertion caused the
// skip without re-reading the spec.
func matcherTierSkip(asserts []dsl.Assertion, activeTier string) (skip bool, reason string) {
	name, needTier, ok := matchers.FindUnsupportedMatcher(asserts, activeTier)
	if !ok {
		return false, ""
	}
	return true, fmt.Sprintf("%q matcher requires tier %s; active tier is %s", name, needTier, activeTier)
}

// tierApplies returns true when a test with the given tier list should run
// under the active tier. An empty/nil tier list means "all tiers". A "*"
// entry also means "all tiers".
func tierApplies(testTier []string, active string) bool {
	if len(testTier) == 0 {
		return true
	}
	for _, t := range testTier {
		if t == "*" || t == active {
			return true
		}
	}
	return false
}

// buildValuesForInstall mirrors the values-merge that buildRenderRequest
// performs for the local-render path: dotted keys in Inputs.Set are expanded
// into nested maps via strvals.ParseInto, the same way the Helm CLI does
// when you pass `--set foo.bar=baz`. Without this, install.Run literally
// stores Values["service.port"] and the chart template - which reads
// .Values.service.port - silently falls back to chart defaults.
func buildValuesForInstall(test dsl.Test) map[string]any {
	vals := map[string]any{}
	if test.Inputs == nil {
		return vals
	}
	for k, v := range test.Inputs.Set {
		valStr := fmt.Sprintf("%v", v)
		if err := strvals.ParseInto(k+"="+valStr, vals); err != nil {
			vals[k] = v
		}
	}
	return vals
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
