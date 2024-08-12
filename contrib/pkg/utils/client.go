package utils

import (
	"context"

	"k8s.io/apimachinery/pkg/watch"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/pkg/util/scheme"
)

// This goofiness is because client.NewWithWatch() doesn't chain, and client.WithFieldOwner returns a Client.
type wwClient struct {
	client.Client
	w client.WithWatch
}

// Watch implements client.WithWatch.
func (w wwClient) Watch(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) (watch.Interface, error) {
	return w.w.Watch(ctx, obj, opts...)
}

// GetClient returns a new dynamic controller-runtime client.
func GetClient(fieldManager string) (client.WithWatch, error) {
	cfg, err := GetClientConfig()
	if err != nil {
		return nil, err
	}

	dynamicClient, err := client.NewWithWatch(cfg, client.Options{Scheme: scheme.GetScheme()})
	if err != nil {
		return nil, err
	}

	return wwClient{
		Client: client.WithFieldOwner(dynamicClient, fieldManager),
		w:      dynamicClient,
	}, nil
}

// GetClientConfig gets the config for the REST client.
func GetClientConfig() (*restclient.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	return kubeconfig.ClientConfig()
}
