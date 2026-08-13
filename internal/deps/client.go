package deps

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	discoverycached "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// buildRESTMapper builds a deferred discovery REST mapper from the REST config.
func buildRESTMapper(restCfg *rest.Config) (meta.RESTMapper, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("building discovery client: %w", err)
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(
		discoverycached.NewMemCacheClient(dc),
	), nil
}
