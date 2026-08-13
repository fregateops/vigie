package deps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"

	"k8s.io/client-go/rest"

	"github.com/fregateops/vigie/internal/dsl"
)

// InstallState holds the state of a completed Install call so that Teardown
// can reverse it in the correct order.
type InstallState struct {
	// Installed is the ordered list of dependencies that were successfully
	// installed. Teardown reverses this slice.
	Installed []dsl.Dependency
	// Exports holds the resolved export values from all installed deps.
	Exports ExportMap
	// restCfg is kept so Teardown can call the same cluster.
	restCfg *rest.Config
	// baseDir is kept so Teardown can resolve relative file paths the same
	// way Install did (e.g. for manifest/kustomize source teardown).
	baseDir string
}

// InstallOptions configures dep installation.
type InstallOptions struct {
	// Parallelism caps concurrent installs within a single DAG batch.
	// Zero or negative defaults to runtime.NumCPU().
	Parallelism int
	// BaseDir resolves relative paths in dep sources (secret.file, manifest:,
	// kustomize:) and is used as the working directory for secret generate
	// commands. Typically the directory containing the integration test file.
	BaseDir string
}

// Install installs all dependencies in DAG order and returns an InstallState
// along with the populated ExportMap. Independent deps at the same DAG level
// are installed concurrently using a worker pool bounded by opts.Parallelism.
func Install(ctx context.Context, deps []dsl.Dependency, restCfg *rest.Config, opts InstallOptions) (*InstallState, ExportMap, error) {
	if len(deps) == 0 {
		return &InstallState{restCfg: restCfg, baseDir: opts.BaseDir}, ExportMap{}, nil
	}

	resolved, err := ResolveRefs(deps)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving ref sources: %w", err)
	}

	batches, err := BuildBatches(resolved)
	if err != nil {
		return nil, nil, fmt.Errorf("building dependency DAG: %w", err)
	}

	parallelism := opts.Parallelism
	if parallelism <= 0 {
		parallelism = runtime.NumCPU()
	}

	var (
		installed []dsl.Dependency
		exports   = ExportMap{}
		mu        sync.Mutex
	)

	cache, cacheErr := NewCacheClient(restCfg)
	if cacheErr != nil {
		slog.Warn("dep cache unavailable, skipping cache checks", "err", cacheErr)
		cache = nil
	}

	for batchIdx, batch := range batches {
		slog.Debug("installing dep batch", "batch", batchIdx, "size", len(batch))
		batchErrors := installBatch(ctx, batch, restCfg, cache, parallelism, opts.BaseDir)
		for _, berr := range batchErrors {
			if berr != nil {
				return nil, nil, berr
			}
		}

		mu.Lock()
		installed = append(installed, batch...)
		mu.Unlock()

		for _, dep := range batch {
			collectExports(dep, exports)
		}
	}

	state := &InstallState{
		Installed: installed,
		Exports:   exports,
		restCfg:   restCfg,
		baseDir:   opts.BaseDir,
	}
	return state, exports, nil
}

// Teardown removes all installed deps in reverse install order.
func Teardown(ctx context.Context, state *InstallState) error {
	if state == nil || len(state.Installed) == 0 {
		return nil
	}

	cache, cacheErr := NewCacheClient(state.restCfg)

	// Reverse the installed list so the last-installed is torn down first.
	reversed := reverseDeps(state.Installed)
	var teardownErrs []error
	for _, dep := range reversed {
		if err := teardownOne(ctx, dep, state.restCfg); err != nil {
			slog.Warn("dep teardown failed", "dep", dep.Name, "err", err)
			teardownErrs = append(teardownErrs, err)
		}
		if cacheErr == nil && cache != nil {
			if err := cache.DeleteRecord(ctx, dep.Name); err != nil {
				slog.Debug("dep cache: failed to delete record", "dep", dep.Name, "err", err)
			}
		}
	}

	return errors.Join(teardownErrs...)
}

// installBatch concurrently installs all deps in a batch using a worker pool.
// Returns a slice of errors aligned by batch index (nil = success).
func installBatch(ctx context.Context, batch []dsl.Dependency, restCfg *rest.Config, cache *CacheClient, parallelism int, baseDir string) []error {
	errs := make([]error, len(batch))
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup

	for idx, dep := range batch {
		wg.Add(1)
		go func(depIdx int, dep dsl.Dependency) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			errs[depIdx] = installOne(ctx, dep, restCfg, cache, baseDir)
		}(idx, dep)
	}
	wg.Wait()
	return errs
}

// installOne installs a single dependency, respecting cache and scope rules.
func installOne(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config, cache *CacheClient, baseDir string) error {
	// Cluster-scoped deps: check cache and skip if already installed.
	if dep.Scope == "cluster" && cache != nil {
		cached, err := cache.IsCached(ctx, dep)
		if err != nil {
			slog.Debug("dep cache check error, proceeding with install", "dep", dep.Name, "err", err)
		} else if cached {
			slog.Debug("dep already cached at cluster scope, skipping install", "dep", dep.Name)
			return nil
		}
	}

	slog.Debug("installing dep", "name", dep.Name, "scope", dep.Scope)
	if err := installSource(ctx, dep, restCfg, baseDir); err != nil {
		return fmt.Errorf("dep %q: %w", dep.Name, err)
	}

	if err := waitForConditions(ctx, restCfg, dep.Name, dep.WaitFor); err != nil {
		return fmt.Errorf("dep %q: wait condition failed: %w", dep.Name, err)
	}

	if dep.Scope == "cluster" && cache != nil {
		if err := cache.StoreRecord(ctx, dep); err != nil {
			slog.Debug("dep cache: failed to store record", "dep", dep.Name, "err", err)
		}
	}

	slog.Debug("dep installed", "name", dep.Name)
	return nil
}

// installSource dispatches to the appropriate source installer based on
// which source field is populated in the DependencySource.
func installSource(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config, baseDir string) error {
	src := dep.Source
	switch {
	case src.Helm != nil:
		return installHelm(ctx, dep, restCfg)
	case src.Manifest != "":
		return applyManifest(ctx, dep, restCfg)
	case src.Kustomize != "":
		return applyKustomize(ctx, dep, restCfg)
	case src.Ref != "":
		// Refs are resolved before batching; reaching here is a programmer error.
		return fmt.Errorf("unresolved ref source %q: call ResolveRefs before Install", src.Ref)
	case src.Secret != nil:
		return applySecret(ctx, dep, restCfg, baseDir)
	default:
		return fmt.Errorf("no source specified (must be one of: helm, manifest, kustomize, secret, ref)")
	}
}

// teardownOne dispatches to the appropriate source teardown function.
func teardownOne(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config) error {
	src := dep.Source
	switch {
	case src.Helm != nil:
		return teardownHelm(ctx, dep, restCfg)
	case src.Manifest != "":
		return teardownManifest(ctx, dep, restCfg)
	case src.Kustomize != "":
		return teardownKustomize(ctx, dep, restCfg)
	case src.Secret != nil:
		return teardownSecret(ctx, dep, restCfg)
	default:
		slog.Debug("dep teardown: no source to tear down", "dep", dep.Name)
		return nil
	}
}

// collectExports reads dep.Exports (key → value template) and populates the
// ExportMap. Currently exports are passed through verbatim; value interpolation
// (e.g. querying the cluster for a Service IP) is reserved for a future pass.
func collectExports(dep dsl.Dependency, exports ExportMap) {
	for key, val := range dep.Exports {
		exports.Set(dep.Name, key, val)
	}
}

// reverseDeps returns a reversed copy of the slice without modifying the original.
func reverseDeps(deps []dsl.Dependency) []dsl.Dependency {
	reversed := make([]dsl.Dependency, len(deps))
	for idx, dep := range deps {
		reversed[len(deps)-1-idx] = dep
	}
	return reversed
}
