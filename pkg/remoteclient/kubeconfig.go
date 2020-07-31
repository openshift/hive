package remoteclient

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
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

func (b *kubeconfigBuilder) Build() (client.Client, error) {
	cfg, err := b.RESTConfig()
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	corev1.SchemeBuilder.AddToScheme(scheme)
	hivev1.SchemeBuilder.AddToScheme(scheme)

	return client.New(cfg, client.Options{
		Scheme: scheme,
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
