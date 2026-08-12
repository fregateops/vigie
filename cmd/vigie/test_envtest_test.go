//go:build envtest

package main

import (
	"context"
	"testing"

	"github.com/fregateops/vigie/internal/cluster"
	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/doctor"
	"github.com/fregateops/vigie/internal/runner"
)

// runApplySuite exercises the same stack as `vigie test --cluster envtest`:
// default config, apply-tier discovery, an envtest backend, and the apply
// runner. It is the apply-tier counterpart to runTestSuite. Self-skips when
// envtest binaries cannot be located so the build tag is safe to enable in CI.
//
// Run with:
//
//	go test -tags envtest ./cmd/vigie/...
func runApplySuite(t *testing.T, chartPath string) []runner.SuiteResult {
	t.Helper()
	const kubeVersion = "1.30"
	if bins, _ := doctor.LocateEnvtestBinaries(kubeVersion); bins == nil {
		t.Skip("envtest binaries not available; run 'vigie doctor'")
	}

	cfg, err := config.Load(chartPath)
	if err != nil {
		t.Fatalf("config.Load(%s): %v", chartPath, err)
	}
	files, err := runner.DiscoverApplyTestFiles(chartPath, "")
	if err != nil {
		t.Fatalf("DiscoverApplyTestFiles(%s): %v", chartPath, err)
	}
	if len(files) == 0 {
		t.Fatalf("no test files discovered under %s", chartPath)
	}

	backend, err := cluster.New(cluster.Config{Type: "envtest", KubeVersion: kubeVersion})
	if err != nil {
		t.Fatalf("cluster.New: %v", err)
	}
	results, err := runner.RunApply(context.Background(), runner.ApplyOptions{
		ChartPath:   chartPath,
		TestFiles:   files,
		Parallelism: 2,
		Cfg:         cfg,
		Backend:     backend,
		BackendType: "envtest",
	})
	if err != nil {
		t.Fatalf("runner.RunApply(%s): %v", chartPath, err)
	}
	return results
}

// TestTest_BasicChart_Envtest_AllPass installs the basic chart for every unit
// test against a real envtest control plane and confirms they all pass - the
// apply-tier counterpart to TestTest_BasicChart_AllPass.
func TestTest_BasicChart_Envtest_AllPass(t *testing.T) {
	results := runApplySuite(t, "../../testdata/charts/basic")
	if runner.AnyFailed(results) {
		t.Fatalf("basic chart should have no failing tests on envtest; results: %+v", results)
	}
	total := 0
	for _, sr := range results {
		total += len(sr.Results)
	}
	if total == 0 {
		t.Fatal("basic chart executed no tests")
	}
}
