package deps

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
)

func TestResolveSecretValue_Env(t *testing.T) {
	t.Setenv("HT_TEST_TOKEN", "secret123")
	val, source, err := resolveSecretValue(context.Background(), dsl.SecretKeySpec{
		Key: "token",
		Env: "HT_TEST_TOKEN",
	}, "")
	if err != nil {
		t.Fatalf("resolveSecretValue: %v", err)
	}
	if string(val) != "secret123" {
		t.Errorf("value = %q", val)
	}
	if source != "env:HT_TEST_TOKEN" {
		t.Errorf("source = %q", source)
	}
}

func TestResolveSecretValue_EnvEmpty_FallsThroughToFallback(t *testing.T) {
	t.Setenv("HT_MISSING", "")
	tmp := t.TempDir()
	fallbackPath := filepath.Join(tmp, "secret")
	if err := os.WriteFile(fallbackPath, []byte("from-file"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	val, source, err := resolveSecretValue(context.Background(), dsl.SecretKeySpec{
		Key: "token",
		Env: "HT_MISSING",
		Fallback: &dsl.SecretKeySpec{
			Key:  "token",
			File: fallbackPath,
		},
	}, "")
	if err != nil {
		t.Fatalf("resolveSecretValue: %v", err)
	}
	if string(val) != "from-file" {
		t.Errorf("value = %q", val)
	}
	if !strings.HasPrefix(source, "fallback:file:") {
		t.Errorf("source = %q", source)
	}
}

func TestResolveSecretValue_File_BinarySafe(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "blob")
	binaryContent := []byte{0x00, 0x01, 0x02, 0xff, 0xfe}
	if err := os.WriteFile(path, binaryContent, 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	val, _, err := resolveSecretValue(context.Background(), dsl.SecretKeySpec{
		Key:  "k",
		File: path,
	}, "")
	if err != nil {
		t.Fatalf("resolveSecretValue: %v", err)
	}
	if string(val) != string(binaryContent) {
		t.Errorf("binary mismatch: %v vs %v", val, binaryContent)
	}
}

func TestResolveSecretValue_File_RelativeToBaseDir(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "sub.txt"), []byte("relative"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	val, _, err := resolveSecretValue(context.Background(), dsl.SecretKeySpec{
		Key:  "k",
		File: "sub.txt",
	}, tmp)
	if err != nil {
		t.Fatalf("resolveSecretValue: %v", err)
	}
	if string(val) != "relative" {
		t.Errorf("value = %q", val)
	}
}

func TestResolveSecretValue_Generate_CapturesStdout(t *testing.T) {
	val, source, err := resolveSecretValue(context.Background(), dsl.SecretKeySpec{
		Key: "k",
		Generate: &dsl.ShellCommand{
			Run: "printf %s 'generated-output'",
		},
	}, "")
	if err != nil {
		t.Fatalf("resolveSecretValue: %v", err)
	}
	if string(val) != "generated-output" {
		t.Errorf("value = %q", val)
	}
	if source != "generate" {
		t.Errorf("source = %q", source)
	}
}

func TestResolveSecretValue_Generate_NonZeroExitErrors(t *testing.T) {
	_, _, err := resolveSecretValue(context.Background(), dsl.SecretKeySpec{
		Key: "k",
		Generate: &dsl.ShellCommand{
			Run: "echo bad >&2 && exit 7",
		},
	}, "")
	if err == nil {
		t.Fatal("expected error from non-zero exit")
	}
	if !strings.Contains(err.Error(), "stderr: bad") {
		t.Errorf("expected stderr in error message, got: %v", err)
	}
}

func TestResolveSecretValue_AllSourcesAbsent_Errors(t *testing.T) {
	t.Setenv("HT_UNSET", "")
	_, _, err := resolveSecretValue(context.Background(), dsl.SecretKeySpec{
		Key: "k",
		Env: "HT_UNSET",
	}, "")
	if err == nil {
		t.Fatal("expected error when no source produces a value")
	}
}

func TestResolveSecretValue_GenerateNeverIncludesStdoutInError(t *testing.T) {
	_, _, err := resolveSecretValue(context.Background(), dsl.SecretKeySpec{
		Key: "k",
		Generate: &dsl.ShellCommand{
			Run: "printf secret-value && exit 1",
		},
	}, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-value") {
		t.Errorf("error message leaked stdout: %v", err)
	}
}
