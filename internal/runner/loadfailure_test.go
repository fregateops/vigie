package runner

import (
	"strings"
	"testing"

	"github.com/fregateops/vigie/internal/config"
)

func TestRun_BadFileIsAFailedSuiteNotAnAbortedRun(t *testing.T) {
	chart := t.TempDir()
	writeFile(t, chart, "Chart.yaml", "apiVersion: v2\nname: c\nversion: 0.1.0\n")
	writeFile(t, chart, "templates/cm.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: c\n")
	good := writeFile(t, chart, "tests/good_test.yaml", "suite: good\ntests:\n  - it: renders\n    asserts:\n      - isKind: ConfigMap\n")
	bad := writeFile(t, chart, "tests/bad_test.yaml", "suite: bad\ntests:\n  - it: helm-unittest style\n    set:\n      a: b\n    asserts:\n      - isKind: ConfigMap\n")

	results, err := Run(Options{ChartPath: chart, TestFiles: []string{bad, good}, Parallelism: 2, Cfg: config.DefaultConfig()})
	if err != nil {
		t.Fatalf("a bad file must not abort the run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want both files reported, got %d", len(results))
	}
	badRes, goodRes := results[0], results[1]
	if !SuiteHasFailure(badRes) || !strings.Contains(strings.Join(badRes.Results[0].Failures, ""), "set") {
		t.Errorf("bad file: want a failed suite naming the offending key, got %+v", badRes)
	}
	if SuiteHasFailure(goodRes) || len(goodRes.Results) != 1 {
		t.Errorf("good file: want it to run and pass, got %+v", goodRes)
	}
}
