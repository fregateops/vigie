package main

import (
	"testing"

	"github.com/fregateops/vigie/internal/runner"
)

func suiteWith(results ...runner.TestResult) runner.SuiteResult {
	return runner.SuiteResult{Results: results}
}

func TestCountTestOutcomes_SplitsSkippedFromExecuted(t *testing.T) {
	// Skipped tests carry Pass: true, so counting "cases" alone cannot tell a
	// green run from one that verified nothing.
	cases := []struct {
		name         string
		results      []runner.SuiteResult
		wantExecuted int
		wantSkipped  int
	}{
		{
			name:         "no suites",
			results:      nil,
			wantExecuted: 0,
			wantSkipped:  0,
		},
		{
			name:         "files parsed but no tests",
			results:      []runner.SuiteResult{suiteWith()},
			wantExecuted: 0,
			wantSkipped:  0,
		},
		{
			name: "every test skipped",
			results: []runner.SuiteResult{suiteWith(
				runner.TestResult{Pass: true, Skipped: true},
				runner.TestResult{Pass: true, Skipped: true},
			)},
			wantExecuted: 0,
			wantSkipped:  2,
		},
		{
			name: "mixed across suites",
			results: []runner.SuiteResult{
				suiteWith(runner.TestResult{Pass: true}, runner.TestResult{Pass: true, Skipped: true}),
				suiteWith(runner.TestResult{Pass: false}),
			},
			wantExecuted: 2,
			wantSkipped:  1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			executed, skipped := countTestOutcomes(tc.results)
			if executed != tc.wantExecuted || skipped != tc.wantSkipped {
				t.Errorf("countTestOutcomes = (%d executed, %d skipped), want (%d, %d)",
					executed, skipped, tc.wantExecuted, tc.wantSkipped)
			}
		})
	}
}
