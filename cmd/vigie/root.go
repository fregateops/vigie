package main

import (
	"fmt"
	"os"

	"github.com/fregateops/vigie/internal/clog"
	"github.com/fregateops/vigie/internal/version"
	"github.com/spf13/cobra"
)

var (
	flagVerbose     int
	flagParallelism int
	flagOutput      string
)

var rootCmd = &cobra.Command{
	Use:   "vigie",
	Short: "Helm chart testing framework",
	Long:  "vigie — a unified lint, template, and integration testing tool for Helm charts.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Inherit Helm's debug flag so `helm --debug vigie ...` enables verbose output.
		if os.Getenv("HELM_DEBUG") == "1" && flagVerbose == 0 {
			flagVerbose = 1
		}
		clog.ConfigureDefault(flagVerbose)
	},
}

func init() {
	rootCmd.Version = version.Version
	rootCmd.PersistentFlags().CountVarP(&flagVerbose, "verbose", "v", "Increase verbosity: -v debug, -vv trace (logs go to stderr)")
	rootCmd.PersistentFlags().StringVarP(&flagOutput, "output", "o", "pretty", "Output format: pretty, json, junit, sarif, tap")
}

// exitErr prints a formatted message to stderr and exits with code.
func exitErr(code int, msg string, args ...any) {
	fmt.Fprintf(os.Stderr, "vigie: "+msg+"\n", args...)
	os.Exit(code)
}

// argOrCwd returns the first positional argument, or "." (the current
// directory) when none is given. Chart commands default to the cwd so running
// inside a chart directory just works.
func argOrCwd(args []string) string {
	if len(args) > 0 && args[0] != "" {
		return args[0]
	}
	return "."
}
