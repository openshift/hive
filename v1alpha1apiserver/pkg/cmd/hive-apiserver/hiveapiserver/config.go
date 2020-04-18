package hiveapiserver

import (
	"net/http"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"

	genericapiserver "k8s.io/apiserver/pkg/server"
	genericapiserveroptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/api/legacyscheme"

	"github.com/openshift/library-go/pkg/apiserver/apiserverconfig"
)

// NewHiveAPIConfig creates new config for the Hive aggregated API server.
func NewHiveAPIConfig(options *genericapiserveroptions.RecommendedOptions) (*HiveAPIConfig, error) {
	kubeClientConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	// Do not do any throttling
	kubeClientConfig.RateLimiter = flowcontrol.NewFakeAlwaysRateLimiter()
	kubeClient, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return nil, err
	}
	kubeInformers := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)

	restOptsGetter, err := NewRESTOptionsGetter(options.Etcd)
	if err != nil {
		return nil, err
	}

	genericConfig := genericapiserver.NewRecommendedConfig(legacyscheme.Codecs)

	// TODO this is actually specific to the kubeapiserver
	genericConfig.SharedInformerFactory = kubeInformers
	genericConfig.ClientConfig = kubeClientConfig

	genericConfig.ExternalAddress = "hiveapi.hive.svc"
	genericConfig.BuildHandlerChainFunc = HiveHandlerChain
	genericConfig.RESTOptionsGetter = restOptsGetter
	genericConfig.LongRunningFunc = apiserverconfig.IsLongRunningRequest

	if err := options.SecureServing.ApplyTo(&genericConfig.Config.SecureServing, &genericConfig.Config.LoopbackClientConfig); err != nil {
		return nil, err
	}
	authenticationOptions := genericapiserveroptions.NewDelegatingAuthenticationOptions()
	if err := authenticationOptions.ApplyTo(&genericConfig.Authentication, genericConfig.SecureServing, genericConfig.OpenAPIConfig); err != nil {
		return nil, err
	}
	authorizationOptions := genericapiserveroptions.NewDelegatingAuthorizationOptions().WithAlwaysAllowPaths("/healthz", "/healthz/").WithAlwaysAllowGroups("system:masters")
	if err := authorizationOptions.ApplyTo(&genericConfig.Authorization); err != nil {
		return nil, err
	}

	ret := &HiveAPIConfig{
		GenericConfig: genericConfig,
		ExtraConfig: HiveAPIExtraConfig{
			InformerStart:             kubeInformers.Start,
			KubeAPIServerClientConfig: kubeClientConfig,
			KubeInformers:             kubeInformers, // TODO remove this and use the one from the genericconfig
		},
	}

	return ret, ret.ExtraConfig.Validate()
}

// HiveHandlerChain is the chain of http handlers for the Hive aggregated API server.
func HiveHandlerChain(apiHandler http.Handler, genericConfig *genericapiserver.Config) http.Handler {
	// this is the normal kube handler chain
	handler := genericapiserver.DefaultBuildHandlerChain(apiHandler, genericConfig)

	handler = apiserverconfig.WithCacheControl(handler, "no-store") // protected endpoints should not be cached

	return handler
}
