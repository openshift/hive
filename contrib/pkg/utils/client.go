package utils

import (
	"github.com/openshift/hive/pkg/util/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetClient returns a new dynamic controller-runtime client.
func GetClient() (client.Client, error) {
	cfg, err := GetClientConfig()
	if err != nil {
		return nil, err
	}

	dynamicClient, err := client.New(cfg, client.Options{Scheme: scheme.GetScheme()})
	if err != nil {
		return nil, err
	}

	return dynamicClient, nil
}

// GetClientConfig gets the config for the REST client.
func GetClientConfig() (*restclient.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	return kubeconfig.ClientConfig()
}
