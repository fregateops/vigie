package kubeclient

import (
	"testing"

	"k8s.io/client-go/rest"
)

// TestRESTGetter_RoundTrip ensures ToRESTConfig returns a defensive copy of
// the underlying *rest.Config.
func TestRESTGetter_RoundTrip(t *testing.T) {
	src := &rest.Config{Host: "https://example.invalid"}
	g := NewRESTGetter(src, "default")

	got, err := g.ToRESTConfig()
	if err != nil {
		t.Fatalf("ToRESTConfig: %v", err)
	}
	if got.Host != src.Host {
		t.Fatalf("Host mismatch: got %q want %q", got.Host, src.Host)
	}
	if got == src {
		t.Fatal("ToRESTConfig returned the source config; expected a copy")
	}

	got.Host = "https://mutated.invalid"
	if src.Host == got.Host {
		t.Fatalf("source config was mutated through the returned copy: %q", src.Host)
	}
}

// TestRESTGetter_Namespace verifies that the namespace passed to NewRESTGetter
// round-trips through ToRawKubeConfigLoader().Namespace().
func TestRESTGetter_Namespace(t *testing.T) {
	const ns = "vg-some-test-abcdef"
	g := NewRESTGetter(&rest.Config{Host: "https://example.invalid"}, ns)

	loader := g.ToRawKubeConfigLoader()
	if loader == nil {
		t.Fatal("ToRawKubeConfigLoader returned nil")
	}
	gotNS, _, err := loader.Namespace()
	if err != nil {
		t.Fatalf("Namespace(): %v", err)
	}
	if gotNS != ns {
		t.Fatalf("namespace mismatch: got %q want %q", gotNS, ns)
	}
}
