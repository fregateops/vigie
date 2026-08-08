package lint

// Rule is a single lint check.
type Rule interface {
	ID() string
	SetName() string
	DefaultSeverity() Severity
	Run(ctx Context) []Finding
}
