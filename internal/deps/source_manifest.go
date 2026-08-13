package deps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/fregateops/vigie/internal/dsl"
)

const (
	managedByAnnotation = "vigie.io/dep-name"
)

// applyManifest reads a multi-document YAML file and applies each document
// via the dynamic client.
func applyManifest(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config) error {
	slog.Debug("applying manifest dep", "name", dep.Name, "path", dep.Source.Manifest)
	raw, err := os.ReadFile(dep.Source.Manifest)
	if err != nil {
		return fmt.Errorf("dep %q: reading manifest %q: %w", dep.Name, dep.Source.Manifest, err)
	}
	return applyRawDocs(ctx, dep.Name, raw, restCfg)
}

// teardownManifest deletes all resources previously applied by applyManifest.
func teardownManifest(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config) error {
	slog.Debug("tearing down manifest dep", "name", dep.Name)
	raw, err := os.ReadFile(dep.Source.Manifest)
	if err != nil {
		slog.Debug("manifest dep: source file missing during teardown, skipping", "dep", dep.Name)
		return nil
	}
	return teardownRawDocs(ctx, dep.Name, raw, restCfg)
}

// applyRawDocs parses a multi-document YAML byte slice, labels each resource,
// and applies them via the dynamic client. Used by both manifest and kustomize sources.
func applyRawDocs(ctx context.Context, depName string, raw []byte, restCfg *rest.Config) error {
	docs, err := parseMultiDocYAML(raw)
	if err != nil {
		return fmt.Errorf("dep %q: parsing documents: %w", depName, err)
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("dep %q: building dynamic client: %w", depName, err)
	}
	mapper, err := buildRESTMapper(restCfg)
	if err != nil {
		return fmt.Errorf("dep %q: building REST mapper: %w", depName, err)
	}

	for docIdx, doc := range docs {
		labelResource(doc, depName)
		if err := applyDoc(ctx, dynClient, mapper, doc); err != nil {
			return fmt.Errorf("dep %q: applying document %d: %w", depName, docIdx, err)
		}
	}
	slog.Debug("documents applied", "dep", depName, "count", len(docs))
	return nil
}

// teardownRawDocs parses a multi-document YAML byte slice and deletes each
// resource via the dynamic client, logging individual delete failures rather
// than aborting. Used by both manifest and kustomize sources.
func teardownRawDocs(ctx context.Context, depName string, raw []byte, restCfg *rest.Config) error {
	docs, err := parseMultiDocYAML(raw)
	if err != nil {
		return fmt.Errorf("dep %q: parsing documents for teardown: %w", depName, err)
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("dep %q: building dynamic client for teardown: %w", depName, err)
	}
	mapper, err := buildRESTMapper(restCfg)
	if err != nil {
		return fmt.Errorf("dep %q: building REST mapper for teardown: %w", depName, err)
	}

	for _, doc := range docs {
		if err := deleteDoc(ctx, dynClient, mapper, doc); err != nil {
			slog.Debug("teardown: delete failed (continuing)", "dep", depName, "err", err)
		}
	}
	return nil
}

// parseMultiDocYAML splits a multi-document YAML byte slice into individual
// unstructured objects, skipping empty documents.
func parseMultiDocYAML(raw []byte) ([]*unstructured.Unstructured, error) {
	var docs []*unstructured.Unstructured
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(raw), 4096)
	for {
		var obj map[string]any
		if err := decoder.Decode(&obj); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if len(obj) == 0 {
			continue
		}
		docs = append(docs, &unstructured.Unstructured{Object: obj})
	}
	return docs, nil
}

// labelResource adds vigie management labels/annotations to the unstructured
// object before applying it to the cluster.
func labelResource(obj *unstructured.Unstructured, depName string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[managedByLabel] = managedByValue
	obj.SetLabels(labels)

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[managedByAnnotation] = depName
	obj.SetAnnotations(annotations)
}

// applyDoc applies a single unstructured document using server-side create
// with conflict resolution (update if already exists).
func applyDoc(ctx context.Context, dynClient dynamic.Interface, mapper meta.RESTMapper, obj *unstructured.Unstructured) error {
	gvr, ns, err := resolveGVR(mapper, obj)
	if err != nil {
		return err
	}

	var res dynamic.ResourceInterface
	if ns != "" {
		res = dynClient.Resource(gvr).Namespace(ns)
	} else {
		res = dynClient.Resource(gvr)
	}

	_, err = res.Create(ctx, obj, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := res.Get(ctx, obj.GetName(), metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("getting existing resource %s/%s: %w", obj.GetKind(), obj.GetName(), getErr)
		}
		obj.SetResourceVersion(existing.GetResourceVersion())
		_, err = res.Update(ctx, obj, metav1.UpdateOptions{})
	}
	return err
}

// deleteDoc deletes a single unstructured document.
func deleteDoc(ctx context.Context, dynClient dynamic.Interface, mapper meta.RESTMapper, obj *unstructured.Unstructured) error {
	gvr, ns, err := resolveGVR(mapper, obj)
	if err != nil {
		return err
	}

	var res dynamic.ResourceInterface
	if ns != "" {
		res = dynClient.Resource(gvr).Namespace(ns)
	} else {
		res = dynClient.Resource(gvr)
	}

	err = res.Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// resolveGVR maps the object's GVK to a GroupVersionResource and returns its
// namespace (empty for cluster-scoped resources).
func resolveGVR(mapper meta.RESTMapper, obj *unstructured.Unstructured) (schema.GroupVersionResource, string, error) {
	gvk := obj.GroupVersionKind()
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, "", fmt.Errorf("REST mapping for %s: %w", gvk.Kind, err)
	}

	ns := ""
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		ns = obj.GetNamespace()
		if ns == "" {
			ns = "default"
		}
	}
	return mapping.Resource, ns, nil
}
