package apiserver

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	kubeinformers "k8s.io/client-go/informers"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"

	hivev1client "github.com/openshift/hive/pkg/client/clientset-generated/clientset/typed/hive/v1"

	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/checkpoint"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/clusterdeployment"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/clusterdeprovisionrequest"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/clusterimageset"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/clusterprovision"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/clusterstate"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/dnszone"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/hiveconfig"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/selectorsyncidentityprovider"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/selectorsyncset"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/syncidentityprovider"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/syncset"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/syncsetinstance"
)

// ExtraConfig is extra config for the Hive API server.
type ExtraConfig struct {
	KubeAPIServerClientConfig *restclient.Config
	KubeInformers             kubeinformers.SharedInformerFactory

	// TODO these should all become local eventually
	Scheme *runtime.Scheme
	Codecs serializer.CodecFactory

	makeV1Storage sync.Once
	v1Storage     map[string]rest.Storage
	v1StorageErr  error
}

// HiveAPIServerConfig is the config for the Hive API server.
type HiveAPIServerConfig struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

// HiveAPIServer is an aggregated API server serving v1alpha1 as a proxy to v1.
type HiveAPIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

// CompletedConfig is the completed config for the Hive API server.
type CompletedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (c *HiveAPIServerConfig) Complete() CompletedConfig {
	return CompletedConfig{
		c.GenericConfig.Complete(),
		&c.ExtraConfig,
	}
}

// New returns a new instance of HiveAPIServer from the given config.
func (c CompletedConfig) New(delegationTarget genericapiserver.DelegationTarget) (*HiveAPIServer, error) {
	genericServer, err := c.GenericConfig.New("authorization.openshift.io-apiserver", delegationTarget)
	if err != nil {
		return nil, err
	}

	s := &HiveAPIServer{
		GenericAPIServer: genericServer,
	}

	v1Alpha1Storage, err := c.V1Alpha1RESTStorage()
	if err != nil {
		return nil, err
	}

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(hivev1alpha1.GroupName, c.ExtraConfig.Scheme, metav1.ParameterCodec, c.ExtraConfig.Codecs)
	apiGroupInfo.VersionedResourcesStorageMap[hivev1alpha1.SchemeGroupVersion.Version] = v1Alpha1Storage
	if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return nil, err
	}

	return s, nil
}

// V1Alpha1RESTStorage creates the storage to use for each of the Hive v1alpha1 kinds.
func (c *CompletedConfig) V1Alpha1RESTStorage() (map[string]rest.Storage, error) {
	c.ExtraConfig.makeV1Storage.Do(func() {
		c.ExtraConfig.v1Storage, c.ExtraConfig.v1StorageErr = c.newV1Alpha1RESTStorage()
	})

	return c.ExtraConfig.v1Storage, c.ExtraConfig.v1StorageErr
}

func (c *CompletedConfig) newV1Alpha1RESTStorage() (map[string]rest.Storage, error) {
	hiveClient, err := hivev1client.NewForConfig(c.ExtraConfig.KubeAPIServerClientConfig)
	if err != nil {
		return nil, err
	}
	coreClient, err := corev1client.NewForConfig(c.ExtraConfig.KubeAPIServerClientConfig)
	if err != nil {
		return nil, err
	}

	v1Storage := map[string]rest.Storage{}
	v1Storage["checkpoints"] = checkpoint.NewREST(hiveClient)
	v1Storage["clusterDeployments"] = clusterdeployment.NewREST(hiveClient, coreClient)
	v1Storage["clusterDeprovisionRequests"] = clusterdeprovisionrequest.NewREST(hiveClient)
	v1Storage["clusterImageSets"] = clusterimageset.NewREST(hiveClient)
	v1Storage["clusterProvisions"] = clusterprovision.NewREST(hiveClient)
	v1Storage["clusterStates"] = clusterstate.NewREST(hiveClient)
	v1Storage["dnsZones"] = dnszone.NewREST(hiveClient)
	v1Storage["hiveConfigs"] = hiveconfig.NewREST(hiveClient)
	v1Storage["syncIdentityProviders"] = syncidentityprovider.NewREST(hiveClient)
	v1Storage["selectorSyncIdentityProviders"] = selectorsyncidentityprovider.NewREST(hiveClient)
	v1Storage["syncSets"] = syncset.NewREST(hiveClient)
	v1Storage["selectorSyncSets"] = selectorsyncset.NewREST(hiveClient)
	v1Storage["syncSetInstances"] = syncsetinstance.NewREST(hiveClient)
	return v1Storage, nil
}
