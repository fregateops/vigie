package deps

import (
	"context"
	"fmt"
	"log/slog"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/fregateops/vigie/internal/dsl"
)

// applyKustomize builds a kustomization in-process using the krusty API,
// then applies the resulting documents via the dynamic client.
func applyKustomize(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config) error {
	slog.Debug("applying kustomize dep", "name", dep.Name, "path", dep.Source.Kustomize)
	raw, err := buildKustomization(dep.Source.Kustomize)
	if err != nil {
		return fmt.Errorf("dep %q: building kustomization: %w", dep.Name, err)
	}
	return applyRawDocs(ctx, dep.Name, raw, restCfg)
}

// teardownKustomize removes resources that were applied by applyKustomize by
// rebuilding the manifest from kustomize and deleting each resulting resource.
func teardownKustomize(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config) error {
	slog.Debug("tearing down kustomize dep", "name", dep.Name)
	raw, err := buildKustomization(dep.Source.Kustomize)
	if err != nil {
		slog.Debug("kustomize dep: rebuild failed during teardown, skipping", "dep", dep.Name, "err", err)
		return nil
	}
	return teardownRawDocs(ctx, dep.Name, raw, restCfg)
}

// buildKustomization runs krusty.MakeKustomizer in-process and returns the
// rendered multi-document YAML.
func buildKustomization(kustomizePath string) ([]byte, error) {
	fSys := filesys.MakeFsOnDisk()
	opts := krusty.MakeDefaultOptions()
	kustomizer := krusty.MakeKustomizer(opts)

	resMap, err := kustomizer.Run(fSys, kustomizePath)
	if err != nil {
		return nil, fmt.Errorf("krusty run: %w", err)
	}
	return resMap.AsYaml()
}
