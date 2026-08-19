package main

import (
	"fmt"
	"os"

	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/dsl"
	"github.com/spf13/cobra"
)

// Schema targets accepted by `vigie schema`. testfile stays the default so the
// documented `vigie schema > .vigie.schema.json` idiom keeps working.
const (
	schemaTargetTestFile = "testfile"
	schemaTargetConfig   = "config"
)

var schemaCmd = &cobra.Command{
	Use:   "schema [testfile|config]",
	Short: "Print a JSON Schema: the test file format (default) or .vigie.yaml",
	Long: "Print one of vigie's JSON Schemas, for editor autocomplete and validation:\n\n" +
		"  testfile  the test file format (tests/**/*_test.yaml) — the default\n" +
		"  config    the per-chart configuration file (.vigie.yaml)",
	Example: `  # Save the test file schema, then reference it from a test file with:
  #   # yaml-language-server: $schema=./.vigie.schema.json
  vigie schema > .vigie.schema.json

  # Save the config schema, then reference it from .vigie.yaml with:
  #   # yaml-language-server: $schema=./.vigie.config.schema.json
  vigie schema config > .vigie.config.schema.json`,
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{schemaTargetTestFile, schemaTargetConfig},
	RunE:      runSchemaCmd,
}

func init() {
	rootCmd.AddCommand(schemaCmd)
}

func runSchemaCmd(cmd *cobra.Command, args []string) error {
	schema, err := schemaFor(schemaTarget(args))
	if err != nil {
		exitErr(3, "%v", err)
	}
	fmt.Fprintf(os.Stdout, "%s\n", schema)
	return nil
}

// schemaTarget returns the requested target, defaulting to the test file so a
// bare `vigie schema` keeps printing it.
func schemaTarget(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return schemaTargetTestFile
}

// schemaFor returns the embedded schema a target name selects.
func schemaFor(target string) ([]byte, error) {
	switch target {
	case schemaTargetTestFile:
		return dsl.SchemaJSON(), nil
	case schemaTargetConfig:
		return config.SchemaJSON(), nil
	default:
		return nil, fmt.Errorf("unknown schema %q: valid values are %s, %s",
			target, schemaTargetTestFile, schemaTargetConfig)
	}
}
