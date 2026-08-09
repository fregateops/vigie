package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/fregateops/vigie/internal/clog"
	"github.com/fregateops/vigie/internal/version"
	"github.com/spf13/cobra"
)

var (
	flagVerbose     int
	flagParallelism int
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
	rootCmd.PersistentFlags().IntVarP(&flagParallelism, "parallelism", "p", runtime.NumCPU(), "Number of parallel tests")
}

// exitErr prints a formatted message to stderr and exits with code.
func exitErr(code int, msg string, args ...any) {
	fmt.Fprintf(os.Stderr, "vigie: "+msg+"\n", args...)
	os.Exit(code)
}
