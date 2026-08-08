// Package v1 hosts the embedded JSON Schema for vigie test files.
//
// The schema is generated from the Go structs under internal/dsl by the
// tooling at tools/gen-schema/. Run `go generate ./pkg/api/schema/v1/...`
// (or `make check-schema`) to regenerate testfile.json after editing
// internal/dsl/types.go.
package v1

import _ "embed"

//go:generate go run github.com/fregateops/vigie/tools/gen-schema

//go:embed testfile.json
var TestFileSchema []byte
