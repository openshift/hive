package remoteclient

import (
	"github.com/openshift/hive/pkg/util/scheme"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewBuilderFromKubeconfig(c client.Client, secret *corev1.Secret) Builder {
	return &kubeconfigBuilder{
		c:      c,
		secret: secret,
	}
}

type kubeconfigBuilder struct {
	c      client.Client
	secret *corev1.Secret
}

// Build is also responsible for verifying reachability of client
func (b *kubeconfigBuilder) Build() (client.Client, error) {
	cfg, err := b.RESTConfig()
	if err != nil {
		return nil, err
	}

	// Verify reachability of client
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	_, err = restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return nil, err
	}
	return client.New(cfg, client.Options{
		Scheme: scheme.GetScheme(),
	})
}

func (b *kubeconfigBuilder) BuildDynamic() (dynamic.Interface, error) {
	cfg, err := b.RESTConfig()
	if err != nil {
		return nil, err
	}

	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (b *kubeconfigBuilder) BuildKubeClient() (kubeclient.Interface, error) {
	cfg, err := b.RESTConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubeclient.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (b *kubeconfigBuilder) UsePrimaryAPIURL() Builder {
	return b
}

func (b *kubeconfigBuilder) UseSecondaryAPIURL() Builder {
	return b
}

func (b *kubeconfigBuilder) RESTConfig() (*rest.Config, error) {
	return restConfigFromSecret(b.secret)
}
