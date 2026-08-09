package runner

import (
	"fmt"

	"github.com/fregateops/vigie/internal/clog"
	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/matchers"
	"github.com/fregateops/vigie/internal/snapshot"
)

// evaluateAssertions runs a test's assertion list against the rendered docs,
// appending any failures to tr.Failures. The apply-tier extras (live-cluster
// context) are threaded back in when those slices land.
//
// It does not set tr.Pass — callers decide that from len(tr.Failures) after
// any tier-specific pre-checks have also run.
func evaluateAssertions(tr *TestResult, et expandedTest, suite *dsl.Suite, allDocs []map[string]any, renderErr error, store *snapshot.Store) {
	test := et.Test

	evalContext := func(doc map[string]any, assertIdx int) matchers.EvalContext {
		return matchers.EvalContext{
			Docs:          allDocs,
			Doc:           doc,
			RenderErr:     renderErr,
			MatrixEntry:   et.MatrixEntry,
			CaseEntry:     et.CaseEntry,
			SnapshotStore: store,
			SuiteName:     suite.SuiteName,
			TestName:      et.DisplayName,
			AssertIdx:     assertIdx,
		}
	}

	if test.ForEach {
		matchedDocs := selectAllDocs(test.Target, allDocs)
		if len(matchedDocs) == 0 {
			tr.Failures = append(tr.Failures, buildTargetDiagnostic(test.Target, allDocs))
			return
		}
		for docIdx, doc := range matchedDocs {
			for assertIdx, assertion := range test.Asserts {
				clog.Trace("evaluating assertion",
					"suite", suite.SuiteName, "test", et.DisplayName,
					"assertion", assertIdx, "doc", docIdx, "matcher", matcherKind(assertion))
				result := matchers.Evaluate(assertion, evalContext(doc, assertIdx))
				if !result.Pass {
					tr.Failures = append(tr.Failures, fmt.Sprintf("        [doc %d] → %s", docIdx, result.Message))
				}
			}
		}
		return
	}

	for assertIdx, assertion := range test.Asserts {
		doc, diagMsg := selectDoc(assertion, test.Target, allDocs, renderErr)
		clog.Trace("evaluating assertion",
			"suite", suite.SuiteName, "test", et.DisplayName,
			"assertion", assertIdx, "matcher", matcherKind(assertion))
		result := matchers.Evaluate(assertion, evalContext(doc, assertIdx))
		if !result.Pass {
			msg := fmt.Sprintf("        → %s", result.Message)
			if diagMsg != "" {
				msg += "\n" + diagMsg
			}
			tr.Failures = append(tr.Failures, msg)
		}
	}
}
