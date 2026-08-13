package deps

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"helm.sh/helm/v3/pkg/action"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"k8s.io/client-go/rest"

	"github.com/fregateops/vigie/internal/dsl"
	"github.com/fregateops/vigie/internal/kubeclient"
)

// installHelm installs a Helm chart from a repository or local path using the
// helm.sh/helm/v3 action API. It respects the dep's namespace and values overrides.
// On scope=cluster, the caller is expected to have already checked the cache before
// calling installHelm.
func installHelm(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config) error {
	slog.Debug("installing helm dep", "name", dep.Name, "chart", dep.Source.Helm.Chart,
		"repo", dep.Source.Helm.Repo, "version", dep.Source.Helm.Version,
		"namespace", dep.Namespace)

	namespace := dep.Namespace
	if namespace == "" {
		namespace = dep.Name
	}

	chrt, err := locateAndLoadChart(dep.Source.Helm)
	if err != nil {
		return fmt.Errorf("dep %q: locating helm chart: %w", dep.Name, err)
	}

	cfg := new(action.Configuration)
	getter := kubeclient.NewRESTGetter(restCfg, namespace)
	logFn := func(format string, v ...interface{}) {
		slog.Debug("helm: " + fmt.Sprintf(format, v...))
	}
	if err := cfg.Init(getter, namespace, "secret", logFn); err != nil {
		return fmt.Errorf("dep %q: helm action.Configuration.Init: %w", dep.Name, err)
	}

	install := action.NewInstall(cfg)
	install.ReleaseName = dep.Name
	install.Namespace = namespace
	install.CreateNamespace = true
	install.Wait = false
	install.DisableHooks = false

	vals := dep.Values
	if vals == nil {
		vals = map[string]any{}
	}

	_, err = install.RunWithContext(ctx, chrt, vals)
	if err != nil {
		return fmt.Errorf("dep %q: helm install failed: %w", dep.Name, err)
	}
	slog.Debug("helm dep installed", "name", dep.Name)
	return nil
}

// teardownHelm uninstalls a previously-installed Helm release.
func teardownHelm(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config) error {
	namespace := dep.Namespace
	if namespace == "" {
		namespace = dep.Name
	}

	cfg := new(action.Configuration)
	getter := kubeclient.NewRESTGetter(restCfg, namespace)
	logFn := func(format string, v ...interface{}) {
		slog.Debug("helm: " + fmt.Sprintf(format, v...))
	}
	if err := cfg.Init(getter, namespace, "secret", logFn); err != nil {
		return fmt.Errorf("dep %q: helm teardown config init: %w", dep.Name, err)
	}

	uninstall := action.NewUninstall(cfg)
	uninstall.Wait = false
	uninstall.IgnoreNotFound = true

	_, err := uninstall.Run(dep.Name)
	if err != nil {
		return fmt.Errorf("dep %q: helm uninstall failed: %w", dep.Name, err)
	}
	slog.Debug("helm dep uninstalled", "name", dep.Name)
	return nil
}

// locateAndLoadChart resolves the chart from the given HelmSource, downloading
// from a remote repository or loading from a local path.
func locateAndLoadChart(src *dsl.HelmSource) (*helmchart.Chart, error) {
	// If no repo URL is specified, treat Chart as a local path.
	if src.Repo == "" {
		return loader.Load(src.Chart)
	}

	cacheDir, err := os.MkdirTemp("", "vigie-chart-*")
	if err != nil {
		return nil, fmt.Errorf("creating chart cache dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(cacheDir) }()

	settings := cli.New()
	settings.RepositoryCache = filepath.Join(cacheDir, "repository")

	opts := action.ChartPathOptions{
		RepoURL: src.Repo,
		Version: src.Version,
	}

	chartPath, err := opts.LocateChart(src.Chart, settings)
	if err != nil {
		return nil, fmt.Errorf("locating chart %q from repo %q: %w", src.Chart, src.Repo, err)
	}
	return loader.Load(chartPath)
}
