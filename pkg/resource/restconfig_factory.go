package resource

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

func (r *Helper) getRESTConfigFactory() (*namespacedFactory, error) {
	if r.metricsEnabled {
		// Copy the possibly shared restConfig reference and add a metrics wrapper.
		cfg := rest.CopyConfig(r.restConfig)
		controllerutils.AddControllerMetricsTransportWrapper(cfg, r.controllerName, false)
		r.restConfig = cfg
	}
	r.logger.WithField("cache-dir", r.cacheDir).Debug("creating cmdutil.Factory from REST client config and cache directory")
	f := newFactory(&restConfigClientGetter{restConfig: r.restConfig, cacheDir: r.cacheDir})
	return f, nil
}

type restConfigClientGetter struct {
	restConfig *rest.Config
	cacheDir   string
}

// ToRESTConfig returns restconfig
func (r *restConfigClientGetter) ToRESTConfig() (*rest.Config, error) {
	return r.restConfig, nil
}

// ToDiscoveryClient returns discovery client
func (r *restConfigClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config := rest.CopyConfig(r.restConfig)
	return getDiscoveryClient(config, r.cacheDir)
}

// ToRESTMapper returns a restmapper
func (r *restConfigClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	expander := restmapper.NewShortcutExpander(mapper, discoveryClient)
	return expander, nil
}

// ToRawKubeConfigLoader return kubeconfig loader as-is
func (r *restConfigClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	cfg := GenerateClientConfigFromRESTConfig("default", r.restConfig)
	return clientcmd.NewNonInteractiveClientConfig(*cfg, "", &clientcmd.ConfigOverrides{}, nil)
}
