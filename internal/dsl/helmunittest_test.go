package dsl

import (
	"errors"
	"strings"
	"testing"
)

func TestParse_DetectsHelmUnittestFiles(t *testing.T) {
	cases := map[string]struct{ yaml, marker string }{
		"test-level set": {`suite: s
templates: [deployment.yaml]
tests:
  - it: a
    set: { replicaCount: 2 }
    asserts:
      - equal: { path: spec.replicas, value: 2 }
`, "test-level `set:`"},
		"map-form hasDocuments": {`suite: s
tests:
  - it: a
    asserts:
      - hasDocuments: { count: 1 }
`, "`hasDocuments: {count: ...}`"},
		"helm-unittest-only matcher": {`suite: s
tests:
  - it: a
    asserts:
      - isNullOrEmpty: { path: spec.x }
`, "matcher `isNullOrEmpty`"},
		"suite-level values": {`suite: s
values: [values-prod.yaml]
tests:
  - it: a
    asserts:
      - exists: { path: kind }
`, "suite-level `values:`"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml), "x_test.yaml")
			var hu *HelmUnittestError
			if !errors.As(err, &hu) {
				t.Fatalf("want HelmUnittestError, got %v", err)
			}
			if hu.Marker != tc.marker || hu.Suite != "s" || hu.Tests != 1 {
				t.Errorf("got %+v, want marker %q, suite s, 1 test", hu, tc.marker)
			}
		})
	}
}

func TestParse_MalformedVigieFileIsNotHelmUnittest(t *testing.T) {
	_, err := Parse([]byte(`suite: s
tests:
  - it: a
    asertz:
      - exists: { path: kind }
`), "x_test.yaml")
	var hu *HelmUnittestError
	if err == nil || errors.As(err, &hu) || !strings.Contains(err.Error(), "asertz") {
		t.Fatalf("want a plain schema error naming the typo, got %v", err)
	}
}
