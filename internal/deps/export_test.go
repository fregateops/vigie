package deps

import "testing"

func TestExportMap_setGet(t *testing.T) {
	em := ExportMap{}
	em.Set("mydb", "host", "localhost")
	em.Set("mydb", "port", "5432")

	host, ok := em.Get("mydb", "host")
	if !ok || host != "localhost" {
		t.Errorf("expected (localhost, true), got (%q, %v)", host, ok)
	}
	port, ok := em.Get("mydb", "port")
	if !ok || port != "5432" {
		t.Errorf("expected (5432, true), got (%q, %v)", port, ok)
	}
}

func TestExportMap_missingDep(t *testing.T) {
	em := ExportMap{}
	_, ok := em.Get("unknown", "host")
	if ok {
		t.Error("expected false for missing dep")
	}
}

func TestExportMap_missingKey(t *testing.T) {
	em := ExportMap{}
	em.Set("mydb", "host", "localhost")
	_, ok := em.Get("mydb", "missing-key")
	if ok {
		t.Error("expected false for missing key")
	}
}
