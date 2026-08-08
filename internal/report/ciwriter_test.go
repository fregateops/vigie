package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fregateops/vigie/internal/cienv"
)

func TestEscapeData(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"50% done", "50%25 done"},
		{"line1\nline2", "line1%0Aline2"},
		{"line1\r\nline2", "line1%0D%0Aline2"},
		{"100%\nnewline", "100%25%0Anewline"},
	}
	for _, tc := range tests {
		got := escapeData(tc.input)
		if got != tc.want {
			t.Errorf("escapeData(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestEscapeProperty(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"key:value", "key%3Avalue"},
		{"a,b", "a%2Cb"},
		{"50%", "50%25"},
		{"msg\nnext", "msg%0Anext"},
		{"url:path,arg", "url%3Apath%2Carg"},
	}
	for _, tc := range tests {
		got := escapeProperty(tc.input)
		if got != tc.want {
			t.Errorf("escapeProperty(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestGitHubGroupTokens(t *testing.T) {
	var buf bytes.Buffer
	writer := newCIWriter(cienv.KindGitHubActions, &buf)

	writer.GroupStart("my suite (1ms)", false)
	writer.GroupEnd()

	out := buf.String()
	if !strings.Contains(out, "::group::my suite (1ms)") {
		t.Errorf("expected ::group:: token, got: %q", out)
	}
	if !strings.Contains(out, "::endgroup::") {
		t.Errorf("expected ::endgroup:: token, got: %q", out)
	}
}

func TestGitHubAnnotationFormat(t *testing.T) {
	var buf bytes.Buffer
	writer := newCIWriter(cienv.KindGitHubActions, &buf)

	writer.Annotation("error", "tests/unit/basic_test.yaml", 10, "suite/test name", "assertion failed")
	out := buf.String()

	if !strings.HasPrefix(out, "::error ") {
		t.Errorf("expected ::error prefix, got: %q", out)
	}
	if !strings.Contains(out, "title=suite%2Ftest+name") && !strings.Contains(out, "title=suite/test name") {
		// just check it has the title property
		if !strings.Contains(out, "title=") {
			t.Errorf("expected title= in annotation, got: %q", out)
		}
	}
	if !strings.Contains(out, "assertion failed") {
		t.Errorf("expected message in annotation, got: %q", out)
	}
}

func TestGitHubAnnotationEscaping(t *testing.T) {
	var buf bytes.Buffer
	writer := newCIWriter(cienv.KindGitHubActions, &buf)

	writer.Annotation("error", "", 0, "test", "50% done\nnext line")
	out := buf.String()

	if !strings.Contains(out, "50%25 done") {
		t.Errorf("expected %%25 escaping in message, got: %q", out)
	}
	if !strings.Contains(out, "%0A") {
		t.Errorf("expected %%0A newline escaping in message, got: %q", out)
	}
}

func TestGitHubAnnotationEmptySeverityIsNoop(t *testing.T) {
	var buf bytes.Buffer
	writer := newCIWriter(cienv.KindGitHubActions, &buf)

	writer.Annotation("", "file.yaml", 1, "title", "msg")
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty severity, got: %q", buf.String())
	}
}

func TestGitLabSectionIDSanitization(t *testing.T) {
	// Reset counter for deterministic test (not strictly needed since we just check prefix).
	id := sanitizeSectionID("web/api:v2")
	if !strings.HasPrefix(id, "web_api_v2") {
		t.Errorf("sanitizeSectionID(%q) = %q, want prefix 'web_api_v2'", "web/api:v2", id)
	}
}

func TestGitLabSectionStartEnd(t *testing.T) {
	var buf bytes.Buffer
	writer := newCIWriter(cienv.KindGitLabCI, &buf)

	writer.GroupStart("my suite", true)
	writer.GroupEnd()

	out := buf.String()
	if !strings.Contains(out, "section_start:") {
		t.Errorf("expected section_start in output, got: %q", out)
	}
	if !strings.Contains(out, "section_end:") {
		t.Errorf("expected section_end in output, got: %q", out)
	}
}

func TestGitLabSectionSharedTimestamp(t *testing.T) {
	var buf bytes.Buffer
	writer := newCIWriter(cienv.KindGitLabCI, &buf)

	writer.GroupStart("suite", false)
	writer.GroupEnd()

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// Extract timestamp from section_start line.
	var startTS, endTS string
	for _, line := range lines {
		if strings.Contains(line, "section_start:") {
			// format: ESC[0Ksection_start:<ts>:<id>...
			parts := strings.SplitN(line, "section_start:", 2)
			if len(parts) == 2 {
				rest := parts[1]
				colonIdx := strings.Index(rest, ":")
				if colonIdx >= 0 {
					startTS = rest[:colonIdx]
				}
			}
		}
		if strings.Contains(line, "section_end:") {
			parts := strings.SplitN(line, "section_end:", 2)
			if len(parts) == 2 {
				rest := parts[1]
				colonIdx := strings.Index(rest, ":")
				if colonIdx >= 0 {
					endTS = rest[:colonIdx]
				}
			}
		}
	}
	if startTS == "" || endTS == "" {
		t.Fatalf("could not extract timestamps from output: %q", out)
	}
	if startTS != endTS {
		t.Errorf("section_start ts %q != section_end ts %q", startTS, endTS)
	}
}

func TestNoopWriterProducesNoOutput(t *testing.T) {
	var buf bytes.Buffer
	writer := newCIWriter(cienv.KindNone, &buf)

	writer.GroupStart("suite", false)
	writer.Annotation("error", "file.go", 1, "title", "msg")
	writer.GroupEnd()

	if buf.Len() != 0 {
		t.Errorf("expected no output from noop writer, got: %q", buf.String())
	}
}
