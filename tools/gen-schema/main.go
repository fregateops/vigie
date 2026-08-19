// Command gen-schema generates the JSON Schemas under pkg/api/schema/v1 from
// their Go source types using github.com/invopop/jsonschema with AddGoComments
// — field doc comments become JSON Schema descriptions.
//
// Two schemas ship: testfile.json from the DSL types in internal/dsl, and
// config.json from the `.vigie.yaml` types in internal/config.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/invopop/jsonschema"

	"github.com/fregateops/vigie/internal/config"
	"github.com/fregateops/vigie/internal/dsl"
)

// schemaBaseURL is where the published schemas are fetched from by editors, so
// a `$schema=` modeline resolves. It points at the raw files on main.
const schemaBaseURL = "https://raw.githubusercontent.com/fregateops/vigie/refs/heads/main/pkg/api/schema/v1"

// target describes one generated schema: the root Go type, the package whose
// doc comments annotate it, and the file it is written to.
type target struct {
	file    string
	title   string
	pkgDir  string
	subject any
}

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

	targets := []target{
		{
			file:    "testfile.json",
			title:   "Vigie test file",
			pkgDir:  "./internal/dsl",
			subject: &dsl.Suite{},
		},
		{
			file:    "config.json",
			title:   "Vigie configuration file (.vigie.yaml)",
			pkgDir:  "./internal/config",
			subject: &config.Config{},
		},
	}
	for _, t := range targets {
		if err := generate(t); err != nil {
			return fmt.Errorf("%s: %w", t.file, err)
		}
	}
	return nil
}

func generate(t target) error {
	reflector := &jsonschema.Reflector{
		ExpandedStruct: true,
	}
	if err := reflector.AddGoComments("github.com/fregateops/vigie", t.pkgDir); err != nil {
		return fmt.Errorf("loading comments: %w", err)
	}

	schema := reflector.Reflect(t.subject)
	// $id must resolve so editors can fetch it via a `$schema=` modeline.
	schema.ID = jsonschema.ID(fmt.Sprintf("%s/%s", schemaBaseURL, t.file))
	schema.Title = t.title

	out, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return os.WriteFile(outputPath(t.file), append(out, '\n'), 0o644)
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

func outputPath(file string) string {
	return filepath.Join("pkg", "api", "schema", "v1", file)
}
