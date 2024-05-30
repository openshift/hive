package awsactuator

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
)

const (
	defaultRequeueLater = 1 * time.Minute
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
		return "", err
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

// ec2TagSpecification is the list of tags that should be added to the resources
// created for the cluster.
func ec2TagSpecification(metadata *hivev1.ClusterMetadata, resource string) *ec2.TagSpecification {
	return &ec2.TagSpecification{
		ResourceType: aws.String(resource),
		Tags: []*ec2.Tag{{
			Key:   aws.String("hive.openshift.io/private-link-access-for"),
			Value: aws.String(metadata.InfraID),
		}, {
			Key:   aws.String("Name"),
			Value: aws.String(metadata.InfraID + "-" + resource),
		}},
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

func updatePrivateLinkStatus(client *client.Client, cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
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

func waitForState(state string, timeout time.Duration, currentState func() (string, error), logger log.FieldLogger) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return wait.PollImmediateUntil(1*time.Minute, func() (done bool, err error) {
		curr, err := currentState()
		if err != nil {
			logger.WithError(err).Error("failed to get the current state")
			return false, nil
		}
		if curr != state {
			logger.Debugf("Desired state %q is not yet achieved, currently %q", state, curr)
			return false, nil
		}
		return true, nil
	}, ctx.Done())
}
