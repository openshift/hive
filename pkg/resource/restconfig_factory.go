package resource

import (
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sync"
)

// HIVE-2399: In the cluster sync controller, getRESTConfigFactory is called once per
// reconcile loop. Previously, a restConfigClientGetter was created for every reconcile,
// used to apply (or rather, attempt to apply) various synced objects, and then thrown
// away at the end of the reconcile pass.
//
// The restConfigClientGetter (or more accurately, the  cmdutil.Factory that wraps it) is
// used by the cluster sync controller (via this helper) to apply the various objects to
// the target cluster during the syncing process. During the application process (see apply.go),
// the apply command is set up (aptly named setupApplyCommand()) using the factory returned
// by the getRESTConfigFactory() below. Within this setup, the ToRESTMapper() function of the
// restConfigClientGetter is invoked. While the mapper returned is a deferred variant,
// eventually the inner NewDiscoveryRESTMapper() function is invoked, which is very expensive
// and allocates a huge amount of memory over time if repeatedly called in a hot loop. A graphic
// demonstrating these allocations is attached the the HIVE-2399 card.
//
// The solution is twofold:
// 1) store the deferred mapper on the restConfigClientGetter so that it can be reused
// 2) cache the restConfigClientGetter for each rest config and reuse it for future reconciles
var restConfigClientGetterCache sync.Map

func (r *helper) getRESTConfigFactory(namespace string) (cmdutil.Factory, error) {
	if r.metricsEnabled {
		// Copy the possibly shared restConfig reference and add a metrics wrapper.
		cfg := rest.CopyConfig(r.restConfig)
		controllerutils.AddControllerMetricsTransportWrapper(cfg, r.controllerName, false)
		r.restConfig = cfg
	}

	// HIVE-2399: reuse the same restConfigClientGetter for each restConfig
	restClientGetter, loaded := restConfigClientGetterCache.LoadOrStore(r.restConfig, &restConfigClientGetter{
		restConfig: r.restConfig,
		cacheDir:   r.cacheDir,
		namespace:  namespace,
		logger:     r.logger,
	})

	r.logger.
		WithField("cache-dir", r.cacheDir).
		WithField("reused-getter", !loaded).
		Debug("creating cmdutil.Factory from REST client config and cache directory")

	f := cmdutil.NewFactory(restClientGetter.(*restConfigClientGetter))
	return f, nil
}

type restConfigClientGetter struct {
	restConfig *rest.Config
	cacheDir   string
	namespace  string
	logger     log.FieldLogger

	// HIVE-2399: since we are reusing the restConfigClientGetter, it may used concurrently
	mu              sync.Mutex
	mapper          *restmapper.DeferredDiscoveryRESTMapper
	discoveryClient discovery.CachedDiscoveryInterface
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
	// HIVE-2399: store the mapper and its discovery client for future use
	r.mu.Lock()
	if r.mapper == nil {
		r.logger.Debugf("creating discovery client and mapper for ToRESTMapper()")
		discoveryClient, err := r.ToDiscoveryClient()
		if err != nil {
			return nil, err
		}
		r.mapper = restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
		r.discoveryClient = discoveryClient
	} else {
		r.logger.Debugf("reusing discovery client and mapper for ToRESTMapper()")
	}
	r.mu.Unlock()

	expander := restmapper.NewShortcutExpander(
		r.mapper, r.discoveryClient,
		func(warning string) {
			r.logger.Warnln(warning)
		})
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
