package runner

import (
	"strings"
	"testing"

	"github.com/fregateops/vigie/internal/config"
)

func TestRun_UnloadableFilesDoNotAbortTheRun(t *testing.T) {
	chart := t.TempDir()
	writeFile(t, chart, "Chart.yaml", "apiVersion: v2\nname: c\nversion: 0.1.0\n")
	writeFile(t, chart, "templates/cm.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: c\n")
	good := writeFile(t, chart, "tests/good_test.yaml", "suite: good\ntests:\n  - it: renders\n    asserts:\n      - isKind: ConfigMap\n")
	bad := writeFile(t, chart, "tests/bad_test.yaml", "suite: bad\ntests:\n  - it: typo\n    asertz:\n      - isKind: ConfigMap\n")
	helmUnittest := writeFile(t, chart, "tests/hu_test.yaml", "suite: hu\ntests:\n  - it: helm-unittest style\n    set:\n      a: b\n    asserts:\n      - isKind: { of: ConfigMap }\n")

	results, err := Run(Options{ChartPath: chart, TestFiles: []string{bad, helmUnittest, good}, Parallelism: 2, Cfg: config.DefaultConfig()})
	if err != nil {
		t.Fatalf("an unloadable file must not abort the run: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("want every file reported, got %d", len(results))
	}
	badRes, huRes, goodRes := results[0], results[1], results[2]
	if !SuiteHasFailure(badRes) || !strings.Contains(strings.Join(badRes.Results[0].Failures, ""), "asertz") {
		t.Errorf("bad file: want a failed suite naming the offending key, got %+v", badRes)
	}
	if hu := huRes.Results[0]; !hu.Skipped || !strings.Contains(hu.SkipReason, "helm-unittest") || huRes.Suite != "hu" {
		t.Errorf("helm-unittest file: want one skipped entry flagged as helm-unittest, got %+v", huRes)
	}
	if SuiteHasFailure(goodRes) || len(goodRes.Results) != 1 {
		t.Errorf("good file: want it to run and pass, got %+v", goodRes)
	}
}
