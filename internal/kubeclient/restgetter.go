package kubeclient

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// restConfigGetter adapts an in-memory *rest.Config so it satisfies
// genericclioptions.RESTClientGetter. Helm's action.Configuration.Init takes
// a RESTClientGetter; the standard implementation reads kubeconfig from disk,
// which we don't have when talking to envtest. This wrapper bridges the gap.
type restConfigGetter struct {
	cfg       *rest.Config
	namespace string
}

// NewRESTGetter returns a RESTClientGetter backed by an in-memory *rest.Config.
// The namespace is surfaced to Helm via ToRawKubeConfigLoader so that
// `action.Install`'s storage driver and namespace-scoped lookups work.
func NewRESTGetter(cfg *rest.Config, namespace string) genericclioptions.RESTClientGetter {
	return &restConfigGetter{cfg: cfg, namespace: namespace}
}

// ToRESTConfig returns a defensive copy of the underlying *rest.Config so
// callers cannot mutate the shared backend config.
func (g *restConfigGetter) ToRESTConfig() (*rest.Config, error) {
	return rest.CopyConfig(g.cfg), nil
}

// ToDiscoveryClient builds a discovery client backed by an in-memory cache.
// Helm uses discovery to map kinds → REST endpoints during install.
func (g *restConfigGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(g.cfg)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(dc), nil
}

// ToRESTMapper returns a deferred REST mapper that resolves kinds lazily
// against the cached discovery client.
func (g *restConfigGetter) ToRESTMapper() (meta.RESTMapper, error) {
	dc, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(dc), nil
}

// ToRawKubeConfigLoader returns a synthetic clientcmd.ClientConfig that only
// carries a namespace override. Helm calls Namespace() on it to resolve the
// release storage namespace.
func (g *restConfigGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return clientcmd.NewDefaultClientConfig(*clientcmdapi.NewConfig(), &clientcmd.ConfigOverrides{
		Context: clientcmdapi.Context{Namespace: g.namespace},
	})
}
