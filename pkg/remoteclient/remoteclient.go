package remoteclient

//go:generate mockgen -source=./remoteclient.go -destination=./mock/remoteclient_generated.go -package=mock

import (
	"context"

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	autoscalingv1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1"
	autoscalingv1beta1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1beta1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/utils"
)

const adminKubeconfigKey = "kubeconfig"

// Builder is used to build API clients to the remote cluster
type Builder interface {
	// Build will return a static kubeclient for the remote cluster.
	Build() (client.Client, error)

	// BuildDynamic will return a dynamic kubeclient for the remote cluster.
	BuildDynamic() (dynamic.Interface, error)

	// Unreachable returns true if Hive has not been able to reach the remote cluster.
	// Note that this function will not attempt to reach the remote cluster. It only checks the current conditions on
	// the ClusterDeployment to determine if the remote cluster is reachable.
	Unreachable() bool

	// APIURL returns the API URL used to connect to the remote cluster.
	APIURL() (string, error)

	// RESTConfig returns the config for a REST client that connects to the remote cluster.
	RESTConfig() (*rest.Config, error)

	// UsePrimaryAPIURL will use the primary API URL. If there is an API URL override, then that is the primary.
	// Otherwise, the primary is the default API URL.
	UsePrimaryAPIURL() Builder

	// UseSecondaryAPIURL will use the secondary API URL. If there is an API URL override, then the initial API URL
	// is the secondary.
	UseSecondaryAPIURL() Builder
}

// NewBuilder creates a new Builder for creating a client to connect to the remote cluster associated with the specified
// ClusterDeployment.
// The controllerName is needed for metrics.
func NewBuilder(c client.Client, cd *hivev1.ClusterDeployment, controllerName string) Builder {
	return &builder{
		c:              c,
		cd:             cd,
		controllerName: controllerName,
		urlToUse:       activeURL,
	}
}

type builder struct {
	c              client.Client
	cd             *hivev1.ClusterDeployment
	controllerName string
	urlToUse       int
}

const (
	activeURL = iota
	primaryURL
	secondaryURL
)

func (b *builder) Unreachable() bool {
	cond := utils.FindClusterDeploymentCondition(b.cd.Status.Conditions, hivev1.UnreachableCondition)
	return cond != nil && cond.Status == corev1.ConditionTrue
}

func (b *builder) Build() (client.Client, error) {
	cfg, err := b.RESTConfig()
	if err != nil {
		return nil, err
	}

	scheme, err := machineapi.SchemeBuilder.Build()
	if err != nil {
		return nil, err
	}

	autoscalingv1.SchemeBuilder.AddToScheme(scheme)
	autoscalingv1beta1.SchemeBuilder.AddToScheme(scheme)

	if err := openshiftapiv1.Install(scheme); err != nil {
		return nil, err
	}

	if err := routev1.Install(scheme); err != nil {
		return nil, err
	}

	return client.New(cfg, client.Options{
		Scheme: scheme,
	})
}

func (b *builder) BuildDynamic() (dynamic.Interface, error) {
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

func (b *builder) UsePrimaryAPIURL() Builder {
	b.urlToUse = primaryURL
	return b
}

func (b *builder) UseSecondaryAPIURL() Builder {
	b.urlToUse = secondaryURL
	return b
}

func (b *builder) RESTConfig() (*rest.Config, error) {
	kubeconfigSecret := &corev1.Secret{}
	if err := b.c.Get(
		context.Background(),
		client.ObjectKey{Namespace: b.cd.Namespace, Name: b.cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name},
		kubeconfigSecret,
	); err != nil {
		return nil, errors.Wrap(err, "could not get admin kubeconfig secret")
	}
	kubeconfigData, ok := kubeconfigSecret.Data[adminKubeconfigKey]
	if !ok {
		return nil, errors.Errorf("admin kubeconfig secret does not contain %q data", adminKubeconfigKey)
	}

	config, err := clientcmd.Load(kubeconfigData)
	if err != nil {
		return nil, err
	}
	kubeConfig := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{})
	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	utils.AddControllerMetricsTransportWrapper(cfg, b.controllerName, true)

	if override := b.cd.Spec.ControlPlaneConfig.APIURLOverride; override != "" {
		useOverrideURL := false
		switch b.urlToUse {
		case activeURL:
			useOverrideURL = b.apiURLOverrideActive()
		case primaryURL:
			useOverrideURL = true
		}
		if useOverrideURL {
			cfg.Host = b.cd.Spec.ControlPlaneConfig.APIURLOverride
		}
	}

	return cfg, nil
}

func (b *builder) APIURL() (string, error) {
	cfg, err := b.RESTConfig()
	if err != nil {
		return "", err
	}
	return cfg.Host, nil
}

func (b *builder) apiURLOverrideActive() bool {
	cond := utils.FindClusterDeploymentCondition(b.cd.Status.Conditions, hivev1.ActiveAPIURLOverrideCondition)
	return cond != nil && cond.Status == corev1.ConditionTrue
}
