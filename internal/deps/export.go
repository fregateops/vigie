package deps

// ExportMap holds resolved export values for all installed dependencies.
// It is keyed as name → key → value and is accessible in CEL bindings as
// deps.<name>.<key>.
type ExportMap map[string]map[string]string

// Set stores a single export entry, creating the inner map when needed.
func (m ExportMap) Set(depName, key, value string) {
	inner, ok := m[depName]
	if !ok {
		inner = make(map[string]string)
		m[depName] = inner
	}
	inner[key] = value
}

// Get retrieves a single export value. Returns ("", false) when not found.
func (m ExportMap) Get(depName, key string) (string, bool) {
	inner, ok := m[depName]
	if !ok {
		return "", false
	}
	val, exists := inner[key]
	return val, exists
}
