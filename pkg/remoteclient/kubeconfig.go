package remoteclient

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/util/scheme"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewBuilderFromKubeconfig(c client.Client, secret *corev1.Secret, controllerName hivev1.ControllerName) Builder {
	return &kubeconfigBuilder{
		c:            c,
		secret:       secret,
		fieldManager: "hive3-" + string(controllerName),
	}
}

type kubeconfigBuilder struct {
	c            client.Client
	secret       *corev1.Secret
	fieldManager string
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
	c, err := client.New(cfg, client.Options{
		Scheme: scheme.GetScheme(),
	})
	if err != nil {
		return nil, err
	}
	return client.WithFieldOwner(c, b.fieldManager), nil
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
	return utils.RestConfigFromSecret(b.secret, false)
}
