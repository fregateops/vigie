package matchers

import (
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

var stringDoc = map[string]any{
	"image":   "myrepo/myapp:v1.0",
	"name":    "my-release",
	"empty":   "",
	"tags":    []any{"v1.0", "v2.0", "latest"},
	"numbers": []any{1, 2, 3},
}

func TestEvalContains(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathContent
		ctx  EvalContext
		pass bool
	}{
		{"string contains substring", dsl.PathContent{Path: "image", Content: "myrepo"}, EvalContext{Doc: stringDoc}, true},
		{"string contains full value", dsl.PathContent{Path: "image", Content: "myrepo/myapp:v1.0"}, EvalContext{Doc: stringDoc}, true},
		{"string does not contain", dsl.PathContent{Path: "image", Content: "badrepo"}, EvalContext{Doc: stringDoc}, false},
		{"empty string contains empty string", dsl.PathContent{Path: "empty", Content: ""}, EvalContext{Doc: stringDoc}, true},
		{"slice contains string element", dsl.PathContent{Path: "tags", Content: "v1.0"}, EvalContext{Doc: stringDoc}, true},
		{"slice does not contain element", dsl.PathContent{Path: "tags", Content: "v3.0"}, EvalContext{Doc: stringDoc}, false},
		{"path not found fails", dsl.PathContent{Path: "missing", Content: "x"}, EvalContext{Doc: stringDoc}, false},
		{"nil doc fails", dsl.PathContent{Path: "image", Content: "x"}, EvalContext{Doc: nil}, false},
		// Helper test: empty path uses HelperOutput.
		{
			"helper empty path uses output",
			dsl.PathContent{Path: "", Content: "myrepo"},
			EvalContext{IsHelperTest: true, HelperOutput: "myrepo/myapp:v1.0"},
			true,
		},
		{
			"non-helper empty path fails",
			dsl.PathContent{Path: "", Content: "x"},
			EvalContext{Doc: stringDoc},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalContains(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
			if !r.Pass && r.Message == "" {
				t.Error("expected non-empty failure message")
			}
		})
	}
}

func TestEvalNotContains(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathContent
		ctx  EvalContext
		pass bool
	}{
		{"string does not contain — pass", dsl.PathContent{Path: "image", Content: "badrepo"}, EvalContext{Doc: stringDoc}, true},
		{"string contains — fail", dsl.PathContent{Path: "image", Content: "myrepo"}, EvalContext{Doc: stringDoc}, false},
		{"slice does not contain — pass", dsl.PathContent{Path: "tags", Content: "v3.0"}, EvalContext{Doc: stringDoc}, true},
		{"slice contains — fail", dsl.PathContent{Path: "tags", Content: "v1.0"}, EvalContext{Doc: stringDoc}, false},
		// Path not found must fail (not silently pass).
		{"path not found fails", dsl.PathContent{Path: "missing", Content: "x"}, EvalContext{Doc: stringDoc}, false},
		{"nil doc fails", dsl.PathContent{Path: "image", Content: "x"}, EvalContext{Doc: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalNotContains(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalStartsWith(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathValue
		ctx  EvalContext
		pass bool
	}{
		{"matches prefix", dsl.PathValue{Path: "image", Value: "myrepo/"}, EvalContext{Doc: stringDoc}, true},
		{"full string is prefix", dsl.PathValue{Path: "image", Value: "myrepo/myapp:v1.0"}, EvalContext{Doc: stringDoc}, true},
		{"does not match prefix", dsl.PathValue{Path: "image", Value: "badrepo/"}, EvalContext{Doc: stringDoc}, false},
		{"empty prefix always matches", dsl.PathValue{Path: "image", Value: ""}, EvalContext{Doc: stringDoc}, true},
		{"path not found fails", dsl.PathValue{Path: "missing", Value: "x"}, EvalContext{Doc: stringDoc}, false},
		{"nil doc fails", dsl.PathValue{Path: "image", Value: "x"}, EvalContext{Doc: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalStartsWith(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalEndsWith(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathValue
		ctx  EvalContext
		pass bool
	}{
		{"matches suffix", dsl.PathValue{Path: "image", Value: ":v1.0"}, EvalContext{Doc: stringDoc}, true},
		{"does not match suffix", dsl.PathValue{Path: "image", Value: ":v2.0"}, EvalContext{Doc: stringDoc}, false},
		{"empty suffix always matches", dsl.PathValue{Path: "image", Value: ""}, EvalContext{Doc: stringDoc}, true},
		{"path not found fails", dsl.PathValue{Path: "missing", Value: "x"}, EvalContext{Doc: stringDoc}, false},
		{"nil doc fails", dsl.PathValue{Path: "image", Value: "x"}, EvalContext{Doc: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalEndsWith(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalMatchRegex(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathPattern
		ctx  EvalContext
		pass bool
	}{
		{"pattern matches", dsl.PathPattern{Path: "image", Pattern: `^myrepo/.+:v\d+\.\d+$`}, EvalContext{Doc: stringDoc}, true},
		{"pattern does not match", dsl.PathPattern{Path: "image", Pattern: `^badrepo/`}, EvalContext{Doc: stringDoc}, false},
		{"invalid pattern fails", dsl.PathPattern{Path: "image", Pattern: `[invalid`}, EvalContext{Doc: stringDoc}, false},
		{"path not found fails", dsl.PathPattern{Path: "missing", Pattern: `.*`}, EvalContext{Doc: stringDoc}, false},
		{"nil doc fails", dsl.PathPattern{Path: "image", Pattern: `.*`}, EvalContext{Doc: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalMatchRegex(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalNotMatchRegex(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathPattern
		ctx  EvalContext
		pass bool
	}{
		{"pattern does not match — pass", dsl.PathPattern{Path: "image", Pattern: `^badrepo/`}, EvalContext{Doc: stringDoc}, true},
		{"pattern matches — fail", dsl.PathPattern{Path: "image", Pattern: `^myrepo/`}, EvalContext{Doc: stringDoc}, false},
		{"invalid pattern fails", dsl.PathPattern{Path: "image", Pattern: `[invalid`}, EvalContext{Doc: stringDoc}, false},
		{"path not found fails", dsl.PathPattern{Path: "missing", Pattern: `.*`}, EvalContext{Doc: stringDoc}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalNotMatchRegex(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}

func TestEvalMatchTemplate(t *testing.T) {
	tests := []struct {
		name string
		spec dsl.PathPattern
		ctx  EvalContext
		pass bool
	}{
		{
			"template with two placeholders matches",
			dsl.PathPattern{Path: "image", Pattern: "${REPO}:${TAG}"},
			EvalContext{Doc: stringDoc},
			true,
		},
		{
			"template with fixed prefix and placeholder matches",
			dsl.PathPattern{Path: "image", Pattern: "myrepo/${APP}:${TAG}"},
			EvalContext{Doc: stringDoc},
			true,
		},
		{
			"template literal mismatch",
			dsl.PathPattern{Path: "image", Pattern: "badrepo/${APP}:${TAG}"},
			EvalContext{Doc: stringDoc},
			false,
		},
		{
			"no placeholders — exact match",
			dsl.PathPattern{Path: "image", Pattern: "myrepo/myapp:v1.0"},
			EvalContext{Doc: stringDoc},
			true,
		},
		{
			"no placeholders — mismatch",
			dsl.PathPattern{Path: "image", Pattern: "myrepo/myapp:v2.0"},
			EvalContext{Doc: stringDoc},
			false,
		},
		{
			"path not found fails",
			dsl.PathPattern{Path: "missing", Pattern: "${X}"},
			EvalContext{Doc: stringDoc},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evalMatchTemplate(&tt.spec, tt.ctx)
			if r.Pass != tt.pass {
				t.Errorf("pass=%v want=%v message=%q", r.Pass, tt.pass, r.Message)
			}
		})
	}
}
