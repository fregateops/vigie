package report

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fregateops/vigie/internal/cienv"
)

// ciWriter emits CI-platform log tokens around suite groups and PR annotations.
type ciWriter interface {
	GroupStart(title string, collapsed bool)
	GroupEnd()
	// Annotation emits a PR-inline annotation. file and line may be empty/0
	// for global annotations. severity is "error", "warning", or "notice".
	Annotation(severity, file string, line int, title, msg string)
}

// newCIWriter constructs the appropriate ciWriter for the detected CI environment.
func newCIWriter(kind cienv.Kind, out io.Writer) ciWriter {
	switch kind {
	case cienv.KindGitHubActions:
		return &githubWriter{out: out}
	case cienv.KindGitLabCI:
		return &gitlabWriter{out: out}
	default:
		return noopWriter{}
	}
}

// escapeData escapes a string for use as a GitHub Actions workflow command value.
func escapeData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// escapeProperty escapes a string for use as a GitHub Actions workflow command property.
func escapeProperty(s string) string {
	s = escapeData(s)
	s = strings.ReplaceAll(s, ":", "%3A")
	s = strings.ReplaceAll(s, ",", "%2C")
	return s
}

// githubWriter emits GitHub Actions workflow commands.
type githubWriter struct {
	out io.Writer
}

func (w *githubWriter) GroupStart(title string, _ bool) {
	fmt.Fprintf(w.out, "::group::%s\n", title)
}

func (w *githubWriter) GroupEnd() {
	fmt.Fprintln(w.out, "::endgroup::")
}

func (w *githubWriter) Annotation(severity, file string, line int, title, msg string) {
	if severity == "" {
		return
	}
	props := fmt.Sprintf("title=%s", escapeProperty(title))
	if file != "" {
		props += fmt.Sprintf(",file=%s", escapeProperty(file))
	}
	if line > 0 {
		props += fmt.Sprintf(",line=%d", line)
	}
	fmt.Fprintf(w.out, "::%s %s::%s\n", severity, props, escapeData(msg))
}

var (
	gitlabSectionCounter atomic.Int64
	gitlabSectionIDRe    = regexp.MustCompile(`[^a-zA-Z0-9_.\-]+`)
)

// sanitizeSectionID converts a title into a GitLab-safe section ID (≤40 chars)
// and appends a monotonic counter to prevent collisions within a run.
func sanitizeSectionID(title string) string {
	clean := gitlabSectionIDRe.ReplaceAllString(title, "_")
	if len(clean) > 40 {
		clean = clean[:40]
	}
	counter := gitlabSectionCounter.Add(1)
	return fmt.Sprintf("%s_%d", clean, counter)
}

// gitlabWriter emits GitLab CI section markers.
type gitlabWriter struct {
	out       io.Writer
	currentID string
	currentTS int64
}

func (w *gitlabWriter) GroupStart(title string, collapsed bool) {
	ts := time.Now().Unix()
	id := sanitizeSectionID(title)
	w.currentID = id
	w.currentTS = ts

	collapsedFlag := ""
	if collapsed {
		collapsedFlag = "[collapsed=true]"
	}
	fmt.Fprintf(w.out, "\x1b[0Ksection_start:%d:%s%s\r\x1b[0K%s\n", ts, id, collapsedFlag, title)
}

func (w *gitlabWriter) GroupEnd() {
	fmt.Fprintf(w.out, "\x1b[0Ksection_end:%d:%s\r\x1b[0K\n", w.currentTS, w.currentID)
}

// Annotation is a no-op for GitLab; the text output inside sections is sufficient.
func (w *gitlabWriter) Annotation(_, _ string, _ int, _, _ string) {}

// noopWriter does nothing — used when no CI environment is detected.
type noopWriter struct{}

func (noopWriter) GroupStart(_ string, _ bool)                {}
func (noopWriter) GroupEnd()                                  {}
func (noopWriter) Annotation(_, _ string, _ int, _, _ string) {}
