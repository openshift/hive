package common

import (
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/client-go/dynamic"
	kclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	apiregv1client "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1"

	hiveapis "github.com/openshift/hive/pkg/apis"
)

func init() {
	apiextv1beta1.AddToScheme(scheme.Scheme)
	hiveapis.AddToScheme(scheme.Scheme)
	admissionv1beta1.AddToScheme(scheme.Scheme)
}

func MustGetClient() client.Client {
	c, err := client.New(MustGetConfig(), client.Options{Scheme: scheme.Scheme})
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
