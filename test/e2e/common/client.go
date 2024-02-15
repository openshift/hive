package common

import (
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"k8s.io/client-go/dynamic"
	kclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	apiregv1client "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1"

	"github.com/openshift/hive/pkg/util/scheme"
)

func MustGetClient() client.WithWatch {
	return MustGetClientFromConfig(MustGetConfig())
}

func MustGetClientFromConfig(cfg *rest.Config) client.WithWatch {
	c, err := client.NewWithWatch(cfg, client.Options{Scheme: scheme.GetScheme()})
	if err != nil {
		log.Fatalf("Error obtaining client: %v", err)
	}
	return c
}

func MustGetKubernetesClient() kclient.Interface {
	c, err := kclient.NewForConfig(MustGetConfig())
	if err != nil {
		log.Fatalf("Error obtaining kubernetes client: %v", err)
	}
	return c
}

func MustGetAPIRegistrationClient() apiregv1client.ApiregistrationV1Interface {
	c, err := apiregv1client.NewForConfig(MustGetConfig())
	if err != nil {
		log.Fatalf("Error obtaining API registration client: %v", err)
	}
	return c
}

func MustGetDynamicClient() dynamic.Interface {
	c, err := dynamic.NewForConfig(MustGetConfig())
	if err != nil {
		log.Fatalf("Error obtaining dynamic client: %v", err)
	}
	return c
}

func MustGetConfig() *rest.Config {
	config, err := config.GetConfig()
	if err != nil {
		log.Fatalf("Error obtaining client config: %v", err)
	}
	return config
}
