package main

import (
	"fmt"
	"os"

	"github.com/fregateops/vigie/internal/dsl"
	"github.com/spf13/cobra"
)

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Print the test file JSON Schema",
	Example: `  # Save the schema for editor autocomplete, then reference it from a
  # test file with:  # yaml-language-server: $schema=./.vigie.schema.json
  vigie schema > .vigie.schema.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(os.Stdout, "%s\n", dsl.SchemaJSON())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(schemaCmd)
}
