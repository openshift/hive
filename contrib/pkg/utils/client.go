package utils

import (
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift/hive/apis"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetClient returns a new dynamic controller-runtime client pointing to the data plane.
func GetClient() (client.Client, error) {
	cfg, err := GetDataPlaneClientConfig()
	if err != nil {
		return nil, err
	}

	apis.AddToScheme(scheme.Scheme)
	dynamicClient, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, err
	}

	return dynamicClient, nil
}

// GetDataPlaneClientConfig gets the config for the REST client that connects to the data plane. In
// normal (non-scale) mode, this is the same as the control plane client config.
func GetDataPlaneClientConfig() (*restclient.Config, error) {
	if cfg := controllerutils.LoadDataPlaneKubeConfigOrDie(); cfg != nil {
		return cfg, nil
	}
	// We're not in scale mode -- return the control plane config
	return GetControlPlaneClientConfig()
}

// GetControlPlaneClientConfig gets the config for the REST client that connects to the control plane.
func GetControlPlaneClientConfig() (*restclient.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	return kubeconfig.ClientConfig()
}
