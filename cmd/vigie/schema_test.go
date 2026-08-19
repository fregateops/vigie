package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSchemaFor_SelectsTheRightDocument(t *testing.T) {
	cases := []struct {
		target   string
		idSuffix string
	}{
		{schemaTargetTestFile, "/testfile.json"},
		{schemaTargetConfig, "/config.json"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			schema, err := schemaFor(tc.target)
			if err != nil {
				t.Fatalf("schemaFor(%q): %v", tc.target, err)
			}
			if !bytes.Contains(schema, []byte(tc.idSuffix+`"`)) {
				t.Errorf("schema for %q does not carry an $id ending in %q", tc.target, tc.idSuffix)
			}
		})
	}
}

func TestSchemaFor_UnknownTargetListsValidOnes(t *testing.T) {
	_, err := schemaFor("values")
	if err == nil {
		t.Fatal("schemaFor must reject an unknown target, got nil error")
	}
	for _, want := range []string{"values", schemaTargetTestFile, schemaTargetConfig} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// A bare `vigie schema` must keep printing the test-file schema: the README
// documents `vigie schema > .vigie.schema.json` as the way to get it.
func TestSchemaTarget_DefaultsToTheTestFile(t *testing.T) {
	if got, want := schemaTarget(nil), schemaTargetTestFile; got != want {
		t.Errorf("schemaTarget(nil): want %q, got %q", want, got)
	}
	if got, want := schemaTarget([]string{schemaTargetConfig}), schemaTargetConfig; got != want {
		t.Errorf("schemaTarget([config]): want %q, got %q", want, got)
	}
}
