package deps

import (
	"bytes"
	"cmp"
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
func applyManifest(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config, baseDir string) error {
	path := resolveDepPath(dep.Source.Manifest, baseDir)
	slog.Debug("applying manifest dep", "name", dep.Name, "path", path)
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("dep %q: reading manifest %q: %w", dep.Name, path, err)
	}
	return applyRawDocs(ctx, dep, raw, restCfg)
}

// teardownManifest deletes all resources previously applied by applyManifest.
func teardownManifest(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config, baseDir string) error {
	slog.Debug("tearing down manifest dep", "name", dep.Name)
	raw, err := os.ReadFile(resolveDepPath(dep.Source.Manifest, baseDir))
	if err != nil {
		slog.Debug("manifest dep: source file missing during teardown, skipping", "dep", dep.Name)
		return nil
	}
	return teardownRawDocs(ctx, dep, raw, restCfg)
}

// applyRawDocs parses a multi-document YAML byte slice, labels each resource,
// and applies them via the dynamic client. Used by both manifest and kustomize sources.
func applyRawDocs(ctx context.Context, dep dsl.Dependency, raw []byte, restCfg *rest.Config) error {
	depName := dep.Name
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
		if err := applyDoc(ctx, dynClient, mapper, doc, dep.Namespace); err != nil {
			return fmt.Errorf("dep %q: applying document %d: %w", depName, docIdx, err)
		}
	}
	slog.Debug("documents applied", "dep", depName, "count", len(docs))
	return nil
}

// teardownRawDocs parses a multi-document YAML byte slice and deletes each
// resource via the dynamic client, logging individual delete failures rather
// than aborting. Used by both manifest and kustomize sources.
func teardownRawDocs(ctx context.Context, dep dsl.Dependency, raw []byte, restCfg *rest.Config) error {
	depName := dep.Name
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
		if err := deleteDoc(ctx, dynClient, mapper, doc, dep.Namespace); err != nil {
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

// applyDoc creates obj, or updates it when vigie created it earlier. An
// object vigie did not create is refused: on a shared cluster it may be real,
// and teardown would otherwise delete it (for a CRD, with every CR of its kind).
func applyDoc(ctx context.Context, dynClient dynamic.Interface, mapper meta.RESTMapper, obj *unstructured.Unstructured, depNamespace string) error {
	res, err := resourceFor(dynClient, mapper, obj, depNamespace)
	if err != nil {
		return err
	}
	_, err = res.Create(ctx, obj, metav1.CreateOptions{})
	if !apierrors.IsAlreadyExists(err) {
		return err
	}
	existing, err := res.Get(ctx, obj.GetName(), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting existing resource %s/%s: %w", obj.GetKind(), obj.GetName(), err)
	}
	if !managedByVigie(existing.GetLabels()) {
		return errNotManaged(obj.GetKind(), existing.GetNamespace(), obj.GetName())
	}
	obj.SetResourceVersion(existing.GetResourceVersion())
	_, err = res.Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

// deleteDoc deletes obj if vigie created it, and leaves anything else alone.
func deleteDoc(ctx context.Context, dynClient dynamic.Interface, mapper meta.RESTMapper, obj *unstructured.Unstructured, depNamespace string) error {
	res, err := resourceFor(dynClient, mapper, obj, depNamespace)
	if err != nil {
		return err
	}
	existing, err := res.Get(ctx, obj.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !managedByVigie(existing.GetLabels()) {
		slog.Debug("teardown: leaving an object vigie did not create", "kind", obj.GetKind(), "name", obj.GetName())
		return nil
	}
	err = res.Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func resourceFor(dynClient dynamic.Interface, mapper meta.RESTMapper, obj *unstructured.Unstructured, depNamespace string) (dynamic.ResourceInterface, error) {
	gvr, ns, err := resolveGVR(mapper, obj, depNamespace)
	if err != nil {
		return nil, err
	}
	if ns == "" {
		return dynClient.Resource(gvr), nil
	}
	return dynClient.Resource(gvr).Namespace(ns), nil
}

// managedByVigie reports whether an object carries the label vigie stamps on
// everything a dependency creates.
func managedByVigie(labels map[string]string) bool {
	_, ok := labels[managedByLabel]
	return ok
}

func errNotManaged(kind, namespace, name string) error {
	ref := kind + "/" + name
	if namespace != "" {
		ref = kind + " " + namespace + "/" + name
	}
	return fmt.Errorf("%s already exists and was not created by vigie; refusing to modify it "+
		"(on a shared cluster it may be in use: rename the dependency or remove the object)", ref)
}

// resolveGVR maps the object's GVK to a GroupVersionResource and returns its
// namespace (empty for cluster-scoped resources): the object's own, else the
// dependency's (a test-scoped dependency targets the test's namespace), else
// "default".
func resolveGVR(mapper meta.RESTMapper, obj *unstructured.Unstructured, depNamespace string) (schema.GroupVersionResource, string, error) {
	gvk := obj.GroupVersionKind()
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, "", fmt.Errorf("REST mapping for %s: %w", gvk.Kind, err)
	}

	ns := ""
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		ns = cmp.Or(obj.GetNamespace(), depNamespace, "default")
	}
	return mapping.Resource, ns, nil
}
