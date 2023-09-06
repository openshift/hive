package remoteclient

//go:generate mockgen -source=./remoteclient.go -destination=./mock/remoteclient_generated.go -package=mock

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	machnet "k8s.io/apimachinery/pkg/util/net"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/util/scheme"
)

// Builder is used to build API clients to the remote cluster
type Builder interface {
	// Build will return a static controller-runtime client for the remote cluster.
	Build() (client.Client, error)

	// BuildDynamic will return a dynamic kubeclient for the remote cluster.
	BuildDynamic() (dynamic.Interface, error)

	// BuildKubeClient will return a kubernetes client for the remote cluster.
	BuildKubeClient() (kubeclient.Interface, error)

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
// If the ClusterDeployment carries the fake cluster annotation, a fake client will be returned populated with
// runtime.Objects we need to query for in all our controllers.
func NewBuilder(c client.Client, cd *hivev1.ClusterDeployment, controllerName hivev1.ControllerName) Builder {
	if utils.IsFakeCluster(cd) {
		return &fakeBuilder{
			urlToUse: activeURL,
		}
	}
	return &builder{
		c:              c,
		cd:             cd,
		controllerName: controllerName,
		urlToUse:       activeURL,
	}
}

// ConnectToRemoteCluster connects to a remote cluster using the specified builder.
// If the ClusterDeployment is marked as unreachable, then no connection will be made.
// If there are problems connecting, then the specified clusterdeployment will be marked as unreachable.
func ConnectToRemoteCluster(
	cd *hivev1.ClusterDeployment,
	remoteClientBuilder Builder,
	localClient client.Client,
	logger log.FieldLogger,
) (remoteClient client.Client, unreachable, requeue bool) {
	var rawRemoteClient interface{}
	rawRemoteClient, unreachable, requeue = connectToRemoteCluster(
		cd,
		remoteClientBuilder,
		localClient,
		logger,
		func(builder Builder) (interface{}, error) { return builder.Build() },
	)
	if unreachable {
		return
	}
	remoteClient = rawRemoteClient.(client.Client)
	return
}

func connectToRemoteCluster(
	cd *hivev1.ClusterDeployment,
	remoteClientBuilder Builder,
	localClient client.Client,
	logger log.FieldLogger,
	buildFunc func(builder Builder) (interface{}, error),
) (remoteClient interface{}, unreachable, requeue bool) {
	if u, _ := Unreachable(cd); u {
		logger.Debug("skipping cluster with unreachable condition")
		unreachable = true
		return
	}
	var err error
	remoteClient, err = buildFunc(remoteClientBuilder)
	if err == nil {
		return
	}
	unreachable = true
	logger.WithError(err).Info("remote cluster is unreachable")
	SetUnreachableCondition(cd, err)
	if err := localClient.Status().Update(context.Background(), cd); err != nil {
		logger.WithError(err).Log(utils.LogLevel(err), "could not update clusterdeployment with unreachable condition")
		requeue = true
	}
	return
}

// InitialURL returns the initial API URL for the ClusterDeployment.
func InitialURL(c client.Client, cd *hivev1.ClusterDeployment) (string, error) {

	if utils.IsFakeCluster(cd) {
		return "https://example.com/veryfakeapi", nil
	}

	cfg, err := unadulteratedRESTConfig(c, cd)
	if err != nil {
		return "", err
	}
	return cfg.Host, nil
}

// Unreachable returns true if Hive has not been able to reach the remote cluster.
// Note that this function will not attempt to reach the remote cluster. It only checks the current conditions on
// the ClusterDeployment to determine if the remote cluster is reachable.
func Unreachable(cd *hivev1.ClusterDeployment) (unreachable bool, lastCheck time.Time) {
	cond := utils.FindCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
	if cond == nil || cond.Status == corev1.ConditionUnknown {
		unreachable = true
		return
	}
	return cond.Status == corev1.ConditionTrue, cond.LastProbeTime.Time
}

// IsPrimaryURLActive returns true if the remote cluster is reachable via the primary API URL.
func IsPrimaryURLActive(cd *hivev1.ClusterDeployment) bool {
	if cd.Spec.ControlPlaneConfig.APIURLOverride == "" {
		return true
	}
	cond := utils.FindCondition(cd.Status.Conditions, hivev1.ActiveAPIURLOverrideCondition)
	return cond != nil && cond.Status == corev1.ConditionTrue
}

// SetUnreachableCondition sets the Unreachable condition on the ClusterDeployment based on the specified error
// encountered when attempting to connect to the remote cluster.
func SetUnreachableCondition(cd *hivev1.ClusterDeployment, connectionError error) (changed bool) {
	status := corev1.ConditionFalse
	reason := "ClusterReachable"
	message := "cluster is reachable"
	// This needs to always update so that the probe time is updated. The probe time is used to determine when to
	// perform the next connectivity check.
	updateCheck := utils.UpdateConditionAlways
	if connectionError != nil {
		status = corev1.ConditionTrue
		reason = "ErrorConnectingToCluster"
		message = utils.ErrorScrub(connectionError)
		updateCheck = utils.UpdateConditionIfReasonOrMessageChange
	}
	cd.Status.Conditions, changed = utils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.UnreachableCondition,
		status,
		reason,
		message,
		updateCheck,
	)
	return
}

type builder struct {
	c              client.Client
	cd             *hivev1.ClusterDeployment
	controllerName hivev1.ControllerName
	urlToUse       int
}

const (
	activeURL = iota
	primaryURL
	secondaryURL
)

func (b *builder) Build() (client.Client, error) {
	cfg, err := b.RESTConfig()
	if err != nil {
		return nil, err
	}

	scheme := scheme.GetScheme()

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

func (b *builder) BuildKubeClient() (kubeclient.Interface, error) {
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

func (b *builder) UsePrimaryAPIURL() Builder {
	b.urlToUse = primaryURL
	return b
}

func (b *builder) UseSecondaryAPIURL() Builder {
	b.urlToUse = secondaryURL
	return b
}

func (b *builder) RESTConfig() (*rest.Config, error) {
	cfg, err := unadulteratedRESTConfig(b.c, b.cd)
	if err != nil {
		return nil, err
	}

	utils.AddControllerMetricsTransportWrapper(cfg, b.controllerName, true)

	if override := b.cd.Spec.ControlPlaneConfig.APIURLOverride; override != "" {
		if b.urlToUse == primaryURL ||
			(b.urlToUse == activeURL && IsPrimaryURLActive(b.cd)) {
			cfg.Host = override
		}
	}

	if override := b.cd.Spec.ControlPlaneConfig.APIServerIPOverride; override != "" {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		cfg.Dial = createDialContext(dialer, override)
		// HIVE-2272: Work around upstream memory leak per
		// https://github.com/kubernetes/kubernetes/issues/118703#issuecomment-1595072383
		// TODO: Revert or adapt when upstream fix is available
		cfg.Proxy = machnet.NewProxierWithNoProxyCIDR(http.ProxyFromEnvironment)
	}

	return cfg, nil
}

func unadulteratedRESTConfig(c client.Client, cd *hivev1.ClusterDeployment) (*rest.Config, error) {
	kubeconfigSecret := &corev1.Secret{}
	if err := c.Get(
		context.Background(),
		client.ObjectKey{Namespace: cd.Namespace, Name: cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name},
		kubeconfigSecret,
	); err != nil {
		return nil, errors.Wrap(err, "could not get admin kubeconfig secret")
	}
	return restConfigFromSecret(kubeconfigSecret)
}

func restConfigFromSecret(kubeconfigSecret *corev1.Secret) (*rest.Config, error) {
	kubeconfigData, ok := kubeconfigSecret.Data[constants.KubeconfigSecretKey]
	if !ok {
		return nil, errors.Errorf("kubeconfig secret does not contain %q data", constants.KubeconfigSecretKey)
	}
	config, err := clientcmd.Load(kubeconfigData)
	if err != nil {
		return nil, err
	}
	kubeConfig := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{})
	return kubeConfig.ClientConfig()
}
