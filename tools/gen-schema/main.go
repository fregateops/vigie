// Command gen-schema generates pkg/api/schema/v1/testfile.json from the
// Go DSL types in internal/dsl using github.com/invopop/jsonschema with
// AddGoComments — field doc comments become JSON Schema descriptions.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/invopop/jsonschema"

	"github.com/fregateops/vigie/internal/dsl"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen-schema:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := os.Chdir(repoRoot()); err != nil {
		return fmt.Errorf("chdir to repo root: %w", err)
	}

	reflector := &jsonschema.Reflector{
		ExpandedStruct: true,
	}
	if err := reflector.AddGoComments("github.com/fregateops/vigie", "./internal/dsl"); err != nil {
		return fmt.Errorf("loading comments: %w", err)
	}

	schema := reflector.Reflect(&dsl.Suite{})
	schema.ID = "https://schemas.vigie.io/v1/test.json"
	schema.Title = "Vigie test file"

	out, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return os.WriteFile(outputPath(), append(out, '\n'), 0o644)
}

// main.go sits at tools/gen-schema/main.go — exactly 3 levels below the repo root.
// If this file is moved, update levelsToRoot accordingly.
const levelsToRoot = 3

func repoRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := thisFile
	for range levelsToRoot {
		dir = filepath.Dir(dir)
	}
	return dir
}

func outputPath() string {
	return filepath.Join("pkg", "api", "schema", "v1", "testfile.json")
}
