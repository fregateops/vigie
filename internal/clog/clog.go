// Package clog wires log/slog for the vigie CLI.
//
// Verbosity is mapped onto slog levels:
//
//	0 → silent (logger discards everything)
//	1 → -v,  slog.LevelDebug
//	≥2 → -vv, LevelTrace (one step finer than Debug)
//
// All output goes to stderr so stdout stays reserved for reporter output that
// downstream tooling may parse.
package clog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
)

// LevelTrace is one step finer than slog.LevelDebug. Used by `-vv`.
const LevelTrace slog.Level = slog.LevelDebug - 4

// ConfigureDefault installs a slog logger as the process default.
// Subsequent slog.Debug / slog.Info calls (and clog.Trace below) honor the
// chosen verbosity. Pass 0 to silence all log output.
func ConfigureDefault(verbosity int) {
	slog.SetDefault(New(verbosity, os.Stderr))
}

// New builds a logger without touching the process default. Exposed for tests.
func New(verbosity int, w io.Writer) *slog.Logger {
	if verbosity <= 0 {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	lvl := slog.LevelDebug
	if verbosity >= 2 {
		lvl = LevelTrace
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.TimeKey:
				// Drop timestamps; CI logs already prefix lines with a clock.
				return slog.Attr{}
			case slog.LevelKey:
				if l, ok := a.Value.Any().(slog.Level); ok && l <= LevelTrace {
					return slog.String(slog.LevelKey, "TRACE")
				}
			}
			return a
		},
	}))
}

// Trace logs at LevelTrace through the default slog logger. slog has no
// built-in Trace helper, so this thin wrapper keeps call sites readable.
func Trace(msg string, args ...any) {
	slog.Default().Log(context.Background(), LevelTrace, msg, args...)
}

// Progress writes a single user-facing progress line to stderr. Unlike slog
// records, these messages are *always* shown regardless of -v verbosity — they
// announce the handful of long-running steps (cluster spin-up, dep install,
// teardown) where a silent CLI would feel hung. Keep messages short and prefix
// them with the component (e.g. "kind:", "k3d:", "e2e:") so users can locate
// the source. Newline is appended automatically.
func Progress(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[vigie] "+format+"\n", args...)
}
