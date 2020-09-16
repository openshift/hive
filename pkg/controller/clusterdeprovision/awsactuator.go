package clusterdeprovision

import (
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-sdk-go/service/sts"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	awsclient "github.com/openshift/hive/pkg/awsclient"
)

func init() {
	registerActuator(&awsActuator{awsClientFn: getAWSClient})
}

// Ensure AWSActuator implements the Actuator interface. This will fail at compile time when false.
var _ Actuator = &awsActuator{}

// AWSActuator manages getting the desired state, getting the current state and reconciling the two.
type awsActuator struct {
	// awsClientFn is the function to build an AWS client, here for testing
	awsClientFn func(*hivev1.ClusterDeprovision, client.Client, log.FieldLogger) (awsclient.Client, error)
}

// CanHandle returns true if the actuator can handle a particular ClusterDeprovision
func (a *awsActuator) CanHandle(clusterDeprovision *hivev1.ClusterDeprovision) bool {
	return clusterDeprovision.Spec.Platform.AWS != nil
}

// TestCredentials ensures that the the aws credentials are usable.
func (a *awsActuator) TestCredentials(clusterDeprovision *hivev1.ClusterDeprovision, c client.Client, logger log.FieldLogger) error {
	awsClient, err := a.awsClientFn(clusterDeprovision, c, logger)
	if err != nil {
		return err
	}

	_, err = awsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		// Code "InvalidClientTokenId" means aws_access_key_id is invalid
		// Code "SignatureDoesNotMatch" means aws_secret_access_key is invalid
		return err
	}

	// Creds passed check.
	return nil
}

func getAWSClient(clusterDeprovision *hivev1.ClusterDeprovision, c client.Client, logger log.FieldLogger) (awsclient.Client, error) {
	awsClient, err := awsclient.NewClient(c, clusterDeprovision.Spec.Platform.AWS.CredentialsSecretRef.Name, clusterDeprovision.Namespace, clusterDeprovision.Spec.Platform.AWS.Region)
	if err != nil {
		logger.WithError(err).Error("failed to get AWS client")
	}
	return awsClient, err
}
