package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fregateops/vigie/internal/cienv"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/runner"
)

// runTestSuite exercises the same stack as the test command: default config,
// unit-test discovery, and the template-tier runner. Snapshots are isolated in
// a temp dir so the first run creates them deterministically.
func runTestSuite(t *testing.T, chartPath string) []runner.SuiteResult {
	t.Helper()
	cfg, err := config.Load(chartPath)
	if err != nil {
		t.Fatalf("config.Load(%s): %v", chartPath, err)
	}
	files, err := runner.DiscoverTestFiles(chartPath, "")
	if err != nil {
		t.Fatalf("DiscoverTestFiles(%s): %v", chartPath, err)
	}
	if len(files) == 0 {
		t.Fatalf("no unit test files discovered under %s", chartPath)
	}
	results, err := runner.Run(runner.Options{
		ChartPath:   chartPath,
		TestFiles:   files,
		Parallelism: 2,
		Cfg:         cfg,
		SnapshotDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("runner.Run(%s): %v", chartPath, err)
	}
	return results
}

func TestTest_BasicChart_AllPass(t *testing.T) {
	results := runTestSuite(t, "../../testdata/charts/basic")
	if runner.AnyFailed(results) {
		t.Fatalf("basic chart should have no failing tests; results: %+v", results)
	}
	total := 0
	for _, sr := range results {
		total += len(sr.Results)
	}
	if total == 0 {
		t.Fatal("basic chart executed no tests")
	}
}

func TestTest_FailingChart_ReportsFailure(t *testing.T) {
	results := runTestSuite(t, "../../testdata/charts/test-failures")
	if !runner.AnyFailed(results) {
		t.Fatal("test-failures chart should report at least one failing test, got none")
	}

	// The suite deliberately mixes one passing and one failing test.
	var pass, fail int
	for _, sr := range results {
		for _, tr := range sr.Results {
			if tr.Pass {
				pass++
			} else {
				fail++
			}
		}
	}
	if pass == 0 || fail == 0 {
		t.Fatalf("expected both a passing and a failing test, got pass=%d fail=%d", pass, fail)
	}

	// The pretty reporter must surface the failure to the user.
	var buf bytes.Buffer
	rep := selectReporter("pretty", &buf, cienv.KindNone)
	if err := rep.Report(results); err != nil {
		t.Fatalf("Report: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "FAIL") || !strings.Contains(out, "fails on a wrong replica count") {
		t.Errorf("pretty output should name the failing test; got:\n%s", out)
	}
}
