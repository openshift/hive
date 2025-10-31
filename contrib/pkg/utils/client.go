package utils

import (
	"context"

	"k8s.io/apimachinery/pkg/watch"
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
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).
		ClientConfig()
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
