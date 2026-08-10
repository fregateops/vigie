package render

import (
	"fmt"

	"github.com/yannh/kubeconform/pkg/resource"
	"github.com/yannh/kubeconform/pkg/validator"
	"gopkg.in/yaml.v3"
)

// SchemaError describes a kubeconform schema violation for a single document.
type SchemaError struct {
	Kind    string
	Name    string
	Message string
}

// SchemaValidator validates rendered documents against the Kubernetes JSON
// schemas for a given kubeVersion. It is safe for concurrent use; the
// underlying kubeconform validator caches downloaded schemas in memory, so
// reusing one instance across tests amortises the schema fetch cost.
type SchemaValidator struct {
	v validator.Validator
}

// NewSchemaValidator builds a validator pinned to the given Kubernetes version
// (e.g. "1.30.0"). Missing schemas are skipped silently.
func NewSchemaValidator(kubeVersion string) (*SchemaValidator, error) {
	v, err := validator.New(nil, validator.Opts{
		KubernetesVersion:    kubeVersion,
		Strict:               false,
		IgnoreMissingSchemas: true,
	})
	if err != nil {
		return nil, fmt.Errorf("creating schema validator: %w", err)
	}
	return &SchemaValidator{v: v}, nil
}

// Validate validates a slice of rendered documents and returns any schema
// violations.
func (s *SchemaValidator) Validate(docs []map[string]any) ([]SchemaError, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	var errs []SchemaError
	for _, doc := range docs {
		docBytes, err := yaml.Marshal(doc)
		if err != nil {
			continue
		}

		res := s.v.ValidateResource(resource.Resource{Bytes: docBytes})
		if res.Status == validator.Invalid {
			kind := fmt.Sprintf("%v", doc["kind"])
			name := ""
			if meta, ok := doc["metadata"].(map[string]any); ok {
				name = fmt.Sprintf("%v", meta["name"])
			}
			for _, ve := range res.ValidationErrors {
				errs = append(errs, SchemaError{
					Kind:    kind,
					Name:    name,
					Message: ve.Error(),
				})
			}
		}
	}
	return errs, nil
}
