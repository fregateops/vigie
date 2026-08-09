package matchers

import (
	"fmt"

	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/snapshot"
)

func init() {
	Register(simpleMatcher{
		name:     "matchSnapshot",
		matches:  func(a dsl.Assertion) bool { return a.MatchSnapshot != nil },
		tiers:    AllTiers,
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalMatchSnapshot(a.MatchSnapshot, ctx) },
	})
}

// evalMatchSnapshot checks or creates a snapshot for the resolved value.
// It delegates to ctx.SnapshotStore.Match which returns (bool, message).
func evalMatchSnapshot(spec *dsl.MatchSnapshotSpec, ctx EvalContext) Result {
	if ctx.SnapshotStore == nil {
		return Result{Pass: false, Message: "matchSnapshot: no snapshot store configured"}
	}

	var val any
	if spec.Path != "" {
		if ctx.Doc == nil {
			return Result{Pass: false, Message: "matchSnapshot: no document selected"}
		}
		resolved, found, err := resolvePathInDoc(ctx.Doc, spec.Path)
		if err != nil {
			return Result{Pass: false, Message: fmt.Sprintf("matchSnapshot: path error: %v", err)}
		}
		if !found {
			return Result{Pass: false, Message: fmt.Sprintf("matchSnapshot: path %q not found", spec.Path)}
		}
		val = resolved
	} else if ctx.IsHelperTest {
		val = ctx.HelperOutput
	} else {
		// Use the whole document.
		val = ctx.Doc
	}

	key := snapshot.Key(ctx.SuiteName, ctx.TestName, ctx.AssertIdx)
	pass, msg := ctx.SnapshotStore.Match(key, val)
	if pass {
		return Result{Pass: true}
	}
	return Result{Pass: false, Message: msg}
}
