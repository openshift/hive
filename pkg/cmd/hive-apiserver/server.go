package hiveapiserver

import (
	"github.com/openshift/library-go/pkg/serviceability"
	"k8s.io/klog"

	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/kubernetes/pkg/capabilities"
	kubelettypes "k8s.io/kubernetes/pkg/kubelet/types"

	_ "k8s.io/kubernetes/pkg/client/metrics/prometheus" // for client metric registration
	_ "k8s.io/kubernetes/pkg/util/reflector/prometheus" // for reflector metric registration
	_ "k8s.io/kubernetes/pkg/util/workqueue/prometheus" // for workqueue metric registration

	"github.com/openshift/hive/pkg/cmd/hive-apiserver/hiveapiserver"
)

// RunHiveAPIServer runs the v1alpha1 aggregated API server
func RunHiveAPIServer(recommendedOptions *genericoptions.RecommendedOptions, stopCh <-chan struct{}) error {
	serviceability.InitLogrusFromKlog()
	// Allow privileged containers
	capabilities.Initialize(capabilities.Capabilities{
		AllowPrivileged: true,
		PrivilegedSources: capabilities.PrivilegedSources{
			HostNetworkSources: []string{kubelettypes.ApiserverSource, kubelettypes.FileSource},
			HostPIDSources:     []string{kubelettypes.ApiserverSource, kubelettypes.FileSource},
			HostIPCSources:     []string{kubelettypes.ApiserverSource, kubelettypes.FileSource},
		},
	})

	hiveAPIServerRuntimeConfig, err := hiveapiserver.NewHiveAPIConfig(recommendedOptions)
	if err != nil {
		return err
	}
	hiveAPIServer, err := hiveAPIServerRuntimeConfig.Complete().New(genericapiserver.NewEmptyDelegate())
	if err != nil {
		return err
	}
	// this sets up the openapi endpoints
	preparedHiveAPIServer := hiveAPIServer.GenericAPIServer.PrepareRun()

	klog.Infof("Starting Hive v1alpha1 API server on %s", recommendedOptions.SecureServing.BindAddress)

	return preparedHiveAPIServer.Run(stopCh)
}
