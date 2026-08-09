package matchers

// strPtr returns a pointer to s, for building assertion specs in tests.
func strPtr(s string) *string { return &s }
