package cienv

import (
	"os"
	"strings"
)

// Kind identifies the active CI environment.
type Kind int

const (
	KindNone Kind = iota
	KindGitHubActions
	KindGitLabCI
)

// Detect returns the active CI environment. VIGIE_CI=github|gitlab|none
// overrides automatic detection. GitLab is checked before the generic CI=true
// fallback because GitLab also sets CI=true.
func Detect() Kind {
	switch strings.ToLower(os.Getenv("VIGIE_CI")) {
	case "github":
		return KindGitHubActions
	case "gitlab":
		return KindGitLabCI
	case "none":
		return KindNone
	}
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return KindGitHubActions
	}
	if os.Getenv("GITLAB_CI") == "true" {
		return KindGitLabCI
	}
	return KindNone
}
