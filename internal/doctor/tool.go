package doctor

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// DownloadPolicy controls whether ResolveTool may fetch a missing binary.
type DownloadPolicy int

const (
	// DownloadNever errors with install guidance when the tool is absent.
	// This is the default in non-interactive contexts (CI).
	DownloadNever DownloadPolicy = iota
	// DownloadInteractive prompts for confirmation before downloading and
	// falls back to DownloadNever when confirm is nil or declines.
	DownloadInteractive
	// DownloadAlways downloads without prompting - an explicit opt-in for
	// scripted automation (--download-tools / VIGIE_AUTO_DOWNLOAD).
	DownloadAlways
)

// ToolSpec describes an external CLI (kind, k3d) that vigie shells out to and
// everything ResolveTool needs to locate, validate, and optionally fetch it.
type ToolSpec struct {
	// Name is the binary name (e.g. "kind"), used for PATH lookup and messages.
	Name string
	// UserPath is an explicit binary path (from --<tool>-binary); "" when unset.
	UserPath string
	// MinVersion is the lowest CLI version whose contract vigie supports.
	// Empty disables the version floor.
	MinVersion string
	// CacheVersion is the version downloaded and looked up in the cache.
	CacheVersion string
	// VersionArgs print the version (e.g. {"version"}); output feeds ParseVersion.
	VersionArgs []string
	// ParseVersion extracts a semver string from VersionArgs output, or "" when
	// it cannot be parsed.
	ParseVersion func([]byte) string
	// CachePath returns the cache location for the binary at a given version.
	CachePath func(version string) (string, error)
	// Download fetches the binary at version into the cache and returns its path.
	Download func(ctx context.Context, version string, progress io.Writer) (string, error)
	// InstallHint is appended to not-found errors (e.g. "install from https://k3d.io").
	InstallHint string
}

// ResolvedTool is a located, validated tool binary.
type ResolvedTool struct {
	Path    string
	Version string // "" when the version could not be parsed
	Source  string // human-readable provenance, for logs
}

// ResolveTool locates spec's binary using a fixed precedence:
//
//  1. spec.UserPath      - explicit --<tool>-binary; must be usable or it errors
//  2. $PATH              - respect an existing install
//  3. vigie binary cache - a previous download
//  4. download           - per policy (interactive confirm on a TTY, or opt-in)
//
// A candidate is "usable" when it executes and reports a version >= MinVersion.
// An unparseable version is tolerated with a warning rather than rejected, so a
// working binary whose `version` output changed does not brick the run.
func ResolveTool(ctx context.Context, spec ToolSpec, policy DownloadPolicy, confirm func(prompt string) bool, progress io.Writer) (ResolvedTool, error) {
	// 1. Explicit user path - must work; no silent fallback to other sources.
	if spec.UserPath != "" {
		resolved, err := probeTool(spec, spec.UserPath, fmt.Sprintf("--%s-binary %s", spec.Name, spec.UserPath), progress)
		if err != nil {
			return ResolvedTool{}, fmt.Errorf("%s binary from --%s-binary is unusable: %w", spec.Name, spec.Name, err)
		}
		return resolved, nil
	}

	var tried []string

	// 2. PATH - prefer a system-managed binary.
	if pathBin, err := lookPath(spec.Name); err == nil {
		if resolved, probeErr := probeTool(spec, pathBin, "PATH", progress); probeErr == nil {
			return resolved, nil
		} else {
			tried = append(tried, fmt.Sprintf("PATH (%s): %v", pathBin, probeErr))
		}
	}

	// 3. Cache - a binary a previous run downloaded.
	if spec.CachePath != nil {
		if cached, err := spec.CachePath(spec.CacheVersion); err == nil && isExecutable(cached) {
			if resolved, probeErr := probeTool(spec, cached, "vigie cache", progress); probeErr == nil {
				return resolved, nil
			} else {
				tried = append(tried, fmt.Sprintf("cache (%s): %v", cached, probeErr))
			}
		}
	}

	// 4. Download, subject to policy.
	if allowDownload(policy, spec, confirm) {
		path, err := spec.Download(ctx, spec.CacheVersion, progress)
		if err != nil {
			return ResolvedTool{}, fmt.Errorf("downloading %s %s: %w", spec.Name, spec.CacheVersion, err)
		}
		resolved, probeErr := probeTool(spec, path, fmt.Sprintf("downloaded %s %s", spec.Name, spec.CacheVersion), progress)
		if probeErr != nil {
			return ResolvedTool{}, fmt.Errorf("downloaded %s is unusable: %w", spec.Name, probeErr)
		}
		return resolved, nil
	}

	return ResolvedTool{}, notFoundError(spec, tried)
}

// probeTool verifies that path is an executable that runs and, when a version
// floor is set, meets it. A version that cannot be parsed or compared is a
// warning (emitted to progress), not a failure - a working binary should not
// be rejected for cosmetic output changes.
func probeTool(spec ToolSpec, path, source string, progress io.Writer) (ResolvedTool, error) {
	if !isExecutable(path) {
		return ResolvedTool{}, fmt.Errorf("%s is not an executable file", path)
	}
	out, err := runCommand(path, spec.VersionArgs...)
	if err != nil {
		return ResolvedTool{}, fmt.Errorf("running %q: %w", strings.TrimSpace(path+" "+strings.Join(spec.VersionArgs, " ")), err)
	}

	version := ""
	if spec.ParseVersion != nil {
		version = spec.ParseVersion(out)
	}
	if version == "" {
		emitProgress(progress, "%s: could not parse version from %s; proceeding without a version check\n", spec.Name, source)
		return ResolvedTool{Path: path, Source: source}, nil
	}

	if spec.MinVersion != "" {
		below, cmpErr := versionBelow(version, spec.MinVersion)
		switch {
		case cmpErr != nil:
			emitProgress(progress, "%s: could not compare version %s against minimum %s (%v); proceeding\n", spec.Name, version, spec.MinVersion, cmpErr)
		case below:
			return ResolvedTool{}, fmt.Errorf("%s %s at %s is older than the minimum supported %s; upgrade it, pass --%s-binary <path>, or enable tool download",
				spec.Name, version, path, spec.MinVersion, spec.Name)
		}
	}
	return ResolvedTool{Path: path, Version: version, Source: source}, nil
}

// versionBelow reports whether have is a lower semver than min.
func versionBelow(have, min string) (bool, error) {
	haveVer, err := semver.NewVersion(have)
	if err != nil {
		return false, fmt.Errorf("parsing version %q: %w", have, err)
	}
	minVer, err := semver.NewVersion(min)
	if err != nil {
		return false, fmt.Errorf("parsing minimum %q: %w", min, err)
	}
	return haveVer.LessThan(minVer), nil
}

// allowDownload decides whether the download step may run under policy.
func allowDownload(policy DownloadPolicy, spec ToolSpec, confirm func(prompt string) bool) bool {
	switch policy {
	case DownloadAlways:
		return true
	case DownloadInteractive:
		if confirm == nil {
			return false
		}
		return confirm(fmt.Sprintf("%s %s was not found; download it to the vigie cache?", spec.Name, spec.CacheVersion))
	default:
		return false
	}
}

// notFoundError builds the terminal "couldn't resolve the tool" error, listing
// what was tried and how to fix it.
func notFoundError(spec ToolSpec, tried []string) error {
	msg := fmt.Sprintf("%s not found on PATH or in the vigie cache", spec.Name)
	if len(tried) > 0 {
		msg += " [" + strings.Join(tried, "; ") + "]"
	}
	hint := spec.InstallHint
	if hint != "" {
		hint += ", "
	}
	return fmt.Errorf("%s; %spass --%s-binary <path>, or enable tool download", msg, hint, spec.Name)
}

// CheckTool resolves spec without downloading and reports the outcome as a
// Check for `vigie doctor`.
func CheckTool(spec ToolSpec) Check {
	resolved, err := ResolveTool(context.Background(), spec, DownloadNever, nil, nil)
	if err != nil {
		return Check{Name: spec.Name, Status: StatusWarning, Detail: err.Error()}
	}
	detail := fmt.Sprintf("%s (%s", resolved.Path, resolved.Source)
	if resolved.Version != "" {
		detail += ", " + resolved.Version
	}
	detail += ")"
	return Check{Name: spec.Name, Status: StatusOK, Detail: detail}
}
