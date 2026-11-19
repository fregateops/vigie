package render

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yannh/kubeconform/pkg/resource"
	"github.com/yannh/kubeconform/pkg/validator"
	"gopkg.in/yaml.v3"
)

// defaultSchemaLocation is kubeconform's keyword for the upstream Kubernetes schemas.
const defaultSchemaLocation = "default"

// SchemaOptions configures a SchemaValidator.
type SchemaOptions struct {
	// KubeVersion pins the Kubernetes schemas, e.g. "1.30.0".
	KubeVersion string
	// Locations in kubeconform `-schema-location` syntax; empty = upstream only.
	Locations []string
	// BaseDir resolves relative local-directory locations.
	BaseDir            string
	AllowUnknownFields bool
}

// SchemaError describes a document that failed validation, or could not be
// validated because its schema could not be fetched.
type SchemaError struct {
	Kind    string
	Name    string
	Message string
}

// SchemaReport is the outcome of validating a set of documents.
type SchemaReport struct {
	Errors []SchemaError
	// UncheckedKinds lists the kinds no location had a schema for (typically
	// CRDs), so the documents passed without being checked.
	UncheckedKinds []string
}

// SchemaValidator validates rendered documents against Kubernetes JSON schemas.
// It is safe for concurrent use; kubeconform caches fetched schemas in memory,
// so reusing one instance across tests amortises the fetch cost.
type SchemaValidator struct {
	v validator.Validator
}

// NewSchemaValidator builds a validator from opts.
func NewSchemaValidator(opts SchemaOptions) (*SchemaValidator, error) {
	locations := make([]string, len(opts.Locations))
	for i, loc := range opts.Locations {
		locations[i] = resolveSchemaLocation(loc, opts.BaseDir)
	}
	v, err := validator.New(locations, validator.Opts{
		KubernetesVersion:    opts.KubeVersion,
		Strict:               !opts.AllowUnknownFields,
		IgnoreMissingSchemas: true,
	})
	if err != nil {
		return nil, fmt.Errorf("creating schema validator: %w", err)
	}
	return &SchemaValidator{v: v}, nil
}

func resolveSchemaLocation(loc, baseDir string) string {
	if loc == defaultSchemaLocation || strings.Contains(loc, "://") || filepath.IsAbs(loc) {
		return loc
	}
	return filepath.Join(baseDir, loc)
}

// Validate validates docs. A schema that cannot be fetched is an error for that
// document rather than a silent pass.
func (s *SchemaValidator) Validate(docs []map[string]any) (SchemaReport, error) {
	var report SchemaReport
	unchecked := map[string]bool{}
	for _, doc := range docs {
		docBytes, err := yaml.Marshal(doc)
		if err != nil {
			return SchemaReport{}, fmt.Errorf("marshalling %s for schema validation: %w", docRef(doc), err)
		}
		res := s.v.ValidateResource(resource.Resource{Bytes: docBytes})
		kind, name := kindAndName(doc)
		switch res.Status {
		case validator.Invalid:
			for _, ve := range res.ValidationErrors {
				report.Errors = append(report.Errors, SchemaError{Kind: kind, Name: name, Message: ve.Error()})
			}
		case validator.Error:
			report.Errors = append(report.Errors, SchemaError{
				Kind: kind, Name: name,
				Message: fmt.Sprintf("could not be validated: %v (offline? point schemas.locations at a local schema directory)", res.Err),
			})
		case validator.Skipped:
			if !unchecked[kind] {
				unchecked[kind] = true
				report.UncheckedKinds = append(report.UncheckedKinds, kind)
			}
		}
	}
	return report, nil
}

func kindAndName(doc map[string]any) (string, string) {
	kind, _ := doc["kind"].(string)
	meta, _ := doc["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	return kind, name
}

func docRef(doc map[string]any) string {
	kind, name := kindAndName(doc)
	return kind + "/" + name
}
