package clog

import (
	"bytes"
	"strings"
	"testing"
)

func TestNew_DefaultVerbosityShowsWarningsOnly(t *testing.T) {
	var out bytes.Buffer
	logger := New(0, &out)
	logger.Debug("debug detail")
	logger.Warn("teardown leaked a namespace")

	if got := out.String(); !strings.Contains(got, "teardown leaked a namespace") || strings.Contains(got, "debug detail") {
		t.Errorf("verbosity 0 must show warnings and hide debug, got %q", got)
	}
}
