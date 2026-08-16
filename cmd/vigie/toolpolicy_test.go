package main

import (
	"testing"

	"github.com/fregateops/vigie/internal/doctor"
)

// TestToolDownloadPolicy covers the CI-relevant branches. go test runs without
// a TTY on stdin, so the interactive branch never fires here (that path is
// exercised manually / via the e2e smoke); the default therefore resolves to
// DownloadNever, and the opt-ins to DownloadAlways.
func TestToolDownloadPolicy(t *testing.T) {
	setFlag := func(t *testing.T, v bool) {
		t.Helper()
		orig := flagTestDownloadTools
		flagTestDownloadTools = v
		t.Cleanup(func() { flagTestDownloadTools = orig })
	}

	t.Run("default never (no TTY, no opt-in)", func(t *testing.T) {
		setFlag(t, false)
		t.Setenv("VIGIE_AUTO_DOWNLOAD", "")
		policy, confirm := toolDownloadPolicy()
		if policy != doctor.DownloadNever {
			t.Errorf("policy = %v, want DownloadNever", policy)
		}
		if confirm != nil {
			t.Error("confirm should be nil when not prompting")
		}
	})

	t.Run("--download-tools forces always", func(t *testing.T) {
		setFlag(t, true)
		t.Setenv("VIGIE_AUTO_DOWNLOAD", "")
		policy, confirm := toolDownloadPolicy()
		if policy != doctor.DownloadAlways {
			t.Errorf("policy = %v, want DownloadAlways", policy)
		}
		if confirm != nil {
			t.Error("DownloadAlways must not prompt (confirm should be nil)")
		}
	})

	t.Run("VIGIE_AUTO_DOWNLOAD forces always", func(t *testing.T) {
		setFlag(t, false)
		t.Setenv("VIGIE_AUTO_DOWNLOAD", "1")
		policy, _ := toolDownloadPolicy()
		if policy != doctor.DownloadAlways {
			t.Errorf("policy = %v, want DownloadAlways", policy)
		}
	})
}
