package matchers

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
	"k8s.io/client-go/rest"
)

// logsCtx builds an EvalContext for logsContain tests.
func logsCtx(inApplyTier bool, restCfg *rest.Config) EvalContext {
	return EvalContext{
		InApplyTier: inApplyTier,
		RESTConfig:  restCfg,
	}
}

func TestEvalLogsContain_TierGuard(t *testing.T) {
	spec := &dsl.LogsAssert{
		Pod:     dsl.LogsPodSelector{Name: "my-pod", Namespace: "default"},
		Pattern: "ready",
		Within:  "5s",
	}

	tests := []struct {
		name       string
		ctx        EvalContext
		wantPass   bool
		wantErrMsg string
	}{
		{
			name:       "not in apply tier — fail with tier mismatch",
			ctx:        logsCtx(false, nil),
			wantPass:   false,
			wantErrMsg: errLogsContainTierMismatch,
		},
		{
			name:       "in apply tier but no RESTConfig — fail with tier mismatch",
			ctx:        logsCtx(true, nil),
			wantPass:   false,
			wantErrMsg: errLogsContainTierMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evalLogsContain(spec, tt.ctx)
			if result.Pass != tt.wantPass {
				t.Errorf("pass=%v want=%v message=%q", result.Pass, tt.wantPass, result.Message)
			}
			if tt.wantErrMsg != "" && result.Message != tt.wantErrMsg {
				t.Errorf("message=%q want=%q", result.Message, tt.wantErrMsg)
			}
		})
	}
}

func TestCompilePattern_PlainSubstring(t *testing.T) {
	matcher, err := compilePattern("listening on :8080")
	if err != nil {
		t.Fatalf("compilePattern returned unexpected error: %v", err)
	}

	if !matcher("server listening on :8080 started") {
		t.Error("expected substring match to succeed")
	}
	if matcher("server started on different port") {
		t.Error("expected substring match to fail for non-matching line")
	}
}

func TestCompilePattern_Regex(t *testing.T) {
	matcher, err := compilePattern("/listening on :\\d+/")
	if err != nil {
		t.Fatalf("compilePattern returned unexpected error: %v", err)
	}

	if !matcher("server listening on :8080 started") {
		t.Error("expected regex match to succeed")
	}
	if !matcher("listening on :9000") {
		t.Error("expected regex match to succeed for alternate port")
	}
	if matcher("not a server line") {
		t.Error("expected regex match to fail for non-matching line")
	}
}

func TestCompilePattern_InvalidRegex(t *testing.T) {
	_, err := compilePattern("/[invalid/")
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

func TestCompilePattern_EmptyRegex(t *testing.T) {
	_, err := compilePattern("//")
	if err == nil {
		t.Error("expected error for empty regex pattern //")
	}
}

func TestCompilePattern_SlashLiteral(t *testing.T) {
	// A single slash is not a regex delimiter - treated as a plain substring.
	matcher, err := compilePattern("/")
	if err != nil {
		t.Fatalf("compilePattern returned unexpected error: %v", err)
	}
	if !matcher("path/to/resource") {
		t.Error("expected single slash to match as substring")
	}
}

func TestParseWithin_Default(t *testing.T) {
	dur, err := parseWithin("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dur != defaultLogsWithin {
		t.Errorf("got %v want %v", dur, defaultLogsWithin)
	}
}

func TestParseWithin_Valid(t *testing.T) {
	dur, err := parseWithin("45s")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dur.Seconds() != 45 {
		t.Errorf("got %v want 45s", dur)
	}
}

func TestParseWithin_Invalid(t *testing.T) {
	_, err := parseWithin("not-a-duration")
	if err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestAppendRing_BelowMax(t *testing.T) {
	buf := appendRing(nil, "line1", 3)
	buf = appendRing(buf, "line2", 3)
	if len(buf) != 2 {
		t.Errorf("expected 2 lines, got %d", len(buf))
	}
}

func TestAppendRing_EvictsOldest(t *testing.T) {
	buf := appendRing(nil, "line1", 3)
	buf = appendRing(buf, "line2", 3)
	buf = appendRing(buf, "line3", 3)
	buf = appendRing(buf, "line4", 3)
	if len(buf) != 3 {
		t.Errorf("expected ring to cap at 3, got %d", len(buf))
	}
	if buf[0] != "line2" {
		t.Errorf("expected oldest line evicted, buf[0]=%q", buf[0])
	}
	if buf[2] != "line4" {
		t.Errorf("expected newest line last, buf[2]=%q", buf[2])
	}
}

func TestEvaluate_LogsContain_TierMismatch(t *testing.T) {
	assertion := dsl.Assertion{
		LogsContain: &dsl.LogsAssert{
			Pod:     dsl.LogsPodSelector{Name: "my-pod"},
			Pattern: "ready",
		},
	}
	result := Evaluate(assertion, EvalContext{InApplyTier: false})
	if result.Pass {
		t.Error("expected Evaluate to fail for logsContain outside apply tier")
	}
	if result.Message != errLogsContainTierMismatch {
		t.Errorf("expected tier mismatch message, got %q", result.Message)
	}
}

func TestResolveNS_UsesSpecNS(t *testing.T) {
	ns := resolveNS("my-ns", EvalContext{})
	if ns != "my-ns" {
		t.Errorf("expected my-ns, got %q", ns)
	}
}

func TestResolveNS_FallsBackToCtxNamespace(t *testing.T) {
	ns := resolveNS("", EvalContext{Namespace: "ctx-ns"})
	if ns != "ctx-ns" {
		t.Errorf("expected ctx-ns, got %q", ns)
	}
}
