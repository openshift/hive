package hiveapiserver

import (
	"fmt"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/api/legacyscheme"

	hiveapiserver "github.com/openshift/hive/pkg/hive/apiserver"

	// register api groups
	_ "github.com/openshift/hive/pkg/api/install"
)

// HiveAPIExtraConfig is extra config for the Hive aggregated API server
type HiveAPIExtraConfig struct {
	// we phrase it like this so we can build the post-start-hook, but no one can take more indirect dependencies on informers
	InformerStart func(stopCh <-chan struct{})

	KubeAPIServerClientConfig *rest.Config
	KubeInformers             kubeinformers.SharedInformerFactory
}

// Validate helps ensure that we build this config correctly, because there are lots of bits to remember for now
func (c *HiveAPIExtraConfig) Validate() error {
	ret := []error{}

	if c.KubeInformers == nil {
		ret = append(ret, fmt.Errorf("KubeInformers is required"))
	}

	return utilerrors.NewAggregate(ret)
}

// HiveAPIConfig is the config for the Hive aggregated API server
type HiveAPIConfig struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   HiveAPIExtraConfig
}

// HiveAPIServer is only responsible for serving the Hive v1alpha1 API
type HiveAPIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

// CompletedConfig if the result of completing the HiveAPIConfig
type CompletedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *HiveAPIExtraConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (c *HiveAPIConfig) Complete() CompletedConfig {
	return CompletedConfig{
		c.GenericConfig.Complete(),
		&c.ExtraConfig,
	}
}

func (c *CompletedConfig) withHiveAPIServer(delegateAPIServer genericapiserver.DelegationTarget) (genericapiserver.DelegationTarget, error) {
	cfg := &hiveapiserver.HiveAPIServerConfig{
		GenericConfig: &genericapiserver.RecommendedConfig{Config: *c.GenericConfig.Config, SharedInformerFactory: c.GenericConfig.SharedInformerFactory},
		ExtraConfig: hiveapiserver.ExtraConfig{
			KubeAPIServerClientConfig: c.ExtraConfig.KubeAPIServerClientConfig,
			KubeInformers:             c.ExtraConfig.KubeInformers,
			Codecs:                    legacyscheme.Codecs,
			Scheme:                    legacyscheme.Scheme,
		},
	}
	config := cfg.Complete()
	server, err := config.New(delegateAPIServer)
	if err != nil {
		return nil, err
	}
	server.GenericAPIServer.PrepareRun() // this triggers openapi construction

	return server.GenericAPIServer, nil
}

type apiServerAppenderFunc func(delegateAPIServer genericapiserver.DelegationTarget) (genericapiserver.DelegationTarget, error)

func addAPIServerOrDie(delegateAPIServer genericapiserver.DelegationTarget, apiServerAppenderFn apiServerAppenderFunc) genericapiserver.DelegationTarget {
	delegateAPIServer, err := apiServerAppenderFn(delegateAPIServer)
	if err != nil {
		klog.Fatal(err)
	}

	return delegateAPIServer
}

// New creates a new Hive aggregated API server.
func (c CompletedConfig) New(delegationTarget genericapiserver.DelegationTarget) (*HiveAPIServer, error) {
	delegateAPIServer := delegationTarget

	delegateAPIServer = addAPIServerOrDie(delegateAPIServer, c.withHiveAPIServer)

	genericServer, err := c.GenericConfig.New("hive-apiserver", delegateAPIServer)
	if err != nil {
		return nil, err
	}

	s := &HiveAPIServer{
		GenericAPIServer: genericServer,
	}

	s.GenericAPIServer.AddPostStartHookOrDie("openshift.io-startinformers", func(context genericapiserver.PostStartHookContext) error {
		c.ExtraConfig.InformerStart(context.StopCh)
		return nil
	})

	return s, nil
}
