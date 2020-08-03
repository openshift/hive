package resource

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

func (r *Helper) getKubeconfigFactory() (*namespacedFactory, error) {
	config, err := clientcmd.Load(r.kubeconfig)
	if err != nil {
		r.logger.WithError(err).Error("an error occurred loading the kubeconfig")
		return nil, err
	}
	clientConfig := clientcmd.NewNonInteractiveClientConfig(*config, "", &clientcmd.ConfigOverrides{}, nil)
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	if r.metricsEnabled {
		controllerutils.AddControllerMetricsTransportWrapper(restConfig, r.controllerName, r.remote)
	}

	r.logger.WithField("cache-dir", r.cacheDir).Debug("creating cmdutil.Factory from client config and cache directory")
	f := newFactory(&kubeconfigClientGetter{
		clientConfig:   clientConfig,
		cacheDir:       r.cacheDir,
		controllerName: r.controllerName,
		metricsEnabled: r.metricsEnabled,
		restConfig:     restConfig,
	})
	return f, nil
}

type kubeconfigClientGetter struct {
	clientConfig   clientcmd.ClientConfig
	cacheDir       string
	controllerName string
	metricsEnabled bool
	restConfig     *rest.Config
}

// ToRESTConfig returns restconfig
func (r *kubeconfigClientGetter) ToRESTConfig() (*rest.Config, error) {
	return r.restConfig, nil
}

// ToDiscoveryClient returns discovery client
func (r *kubeconfigClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := r.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	return getDiscoveryClient(config, r.cacheDir)
}

// ToRESTMapper returns a restmapper
func (r *kubeconfigClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	expander := restmapper.NewShortcutExpander(mapper, discoveryClient)
	return expander, nil
}

// ToRawKubeConfigLoader return kubeconfig loader as-is
func (r *kubeconfigClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return r.clientConfig
}
