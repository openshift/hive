package awsactuator

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
)

var (
	requeueLater = reconcile.Result{RequeueAfter: 1 * time.Minute}
)

// initialURL returns the initial API URL for the ClusterProvision.
func initialURL(c client.Client, key client.ObjectKey) (string, error) {
	kubeconfigSecret := &corev1.Secret{}
	if err := c.Get(
		context.Background(),
		key,
		kubeconfigSecret,
	); err != nil {
		return "", err
	}
	cfg, err := restConfigFromSecret(kubeconfigSecret)
	if err != nil {
		return "", errors.Wrap(err, "failed to load the kubeconfig")
	}

	u, err := url.Parse(cfg.Host)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse the kubeconfig")
	}
	return strings.TrimSuffix(u.Hostname(), "."), nil
}

// ec2FilterForCluster is the filter that is used to find the resources tied to the cluster.
func ec2FilterForCluster(metadata *hivev1.ClusterMetadata) *ec2.Filter {
	return &ec2.Filter{
		Name:   aws.String("tag:hive.openshift.io/private-link-access-for"),
		Values: aws.StringSlice([]string{metadata.InfraID}),
	}
}

// ReadAWSPrivateLinkControllerConfigFile reads the configuration from the env
// and unmarshals. If the env is set to a file but that file doesn't exist it returns
// a zero-value configuration.
// Deprecated: Included for backwards compatability.
func ReadAWSPrivateLinkControllerConfigFile() (*hivev1.AWSPrivateLinkConfig, error) {
	fPath := os.Getenv(constants.AWSPrivateLinkControllerConfigFileEnvVar)
	if len(fPath) == 0 {
		return nil, nil
	}

	config := &hivev1.AWSPrivateLinkConfig{}

	fileBytes, err := os.ReadFile(fPath)
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil {
		return config, errors.Wrap(err, "failed to read the aws privatelink controller config file")
	}
	if err := json.Unmarshal(fileBytes, &config); err != nil {
		return config, err
	}

	return config, nil
}
func restConfigFromSecret(kubeconfigSecret *corev1.Secret) (*rest.Config, error) {
	kubeconfigData := kubeconfigSecret.Data[constants.RawKubeconfigSecretKey]
	if len(kubeconfigData) == 0 {
		kubeconfigData = kubeconfigSecret.Data[constants.KubeconfigSecretKey]
	}
	if len(kubeconfigData) == 0 {
		return nil, errors.New("kubeconfig secret does not contain necessary data")
	}
	config, err := clientcmd.Load(kubeconfigData)
	if err != nil {
		return nil, err
	}
	kubeConfig := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{})
	return kubeConfig.ClientConfig()
}

func initPrivateLinkStatus(cd *hivev1.ClusterDeployment) {
	if cd.Status.Platform == nil {
		cd.Status.Platform = &hivev1.PlatformStatus{}
	}
	if cd.Status.Platform.AWS == nil {
		cd.Status.Platform.AWS = &hivev1aws.PlatformStatus{}
	}
	if cd.Status.Platform.AWS.PrivateLink == nil {
		cd.Status.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccessStatus{}
	}
}

func updatePrivateLinkStatus(client *client.Client, cd *hivev1.ClusterDeployment) error {
	var retryBackoff = wait.Backoff{
		Steps:    5,
		Duration: 1 * time.Second,
		Factor:   1.0,
		Jitter:   0.1,
	}
	return retry.RetryOnConflict(retryBackoff, func() error {
		curr := &hivev1.ClusterDeployment{}
		err := (*client).Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, curr)
		if err != nil {
			return err
		}

		initPrivateLinkStatus(curr)
		curr.Status.Platform.AWS.PrivateLink = cd.Status.Platform.AWS.PrivateLink
		return (*client).Status().Update(context.TODO(), curr)
	})
}
