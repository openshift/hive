package resource

import (
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

func (r *Helper) getRESTConfigFactory(namespace string) (cmdutil.Factory, error) {
	if r.metricsEnabled {
		// Copy the possibly shared restConfig reference and add a metrics wrapper.
		cfg := rest.CopyConfig(r.restConfig)
		controllerutils.AddControllerMetricsTransportWrapper(cfg, r.controllerName, false)
		r.restConfig = cfg
	}
	r.logger.WithField("cache-dir", r.cacheDir).Debug("creating cmdutil.Factory from REST client config and cache directory")
	f := cmdutil.NewFactory(&restConfigClientGetter{restConfig: r.restConfig, cacheDir: r.cacheDir, namespace: namespace})
	return f, nil
}

type restConfigClientGetter struct {
	restConfig *rest.Config
	cacheDir   string
	namespace  string
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
	overrides := &clientcmd.ConfigOverrides{}
	if len(r.namespace) > 0 {
		overrides.Context.Namespace = r.namespace
	}
	return clientcmd.NewNonInteractiveClientConfig(*cfg, "", overrides, nil)
}
