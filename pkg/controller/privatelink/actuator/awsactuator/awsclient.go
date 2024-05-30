package awsactuator

import (
	"os"

	"github.com/aws/aws-sdk-go/aws/awserr"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type awsClient struct {
	hub  awsclient.Client
	user awsclient.Client
}

type awsClientFn func(client.Client, awsclient.Options) (awsclient.Client, error)

func newAWSClient(client client.Client, clientFn awsClientFn, cd *hivev1.ClusterDeployment, config *hivev1.AWSPrivateLinkConfig) (*awsClient, error) {
	uClient, err := clientFn(client, awsclient.Options{
		Region: cd.Spec.Platform.AWS.Region,
		CredentialsSource: awsclient.CredentialsSource{
			Secret: &awsclient.SecretCredentialsSource{
				Namespace: cd.Namespace,
				Ref:       &cd.Spec.Platform.AWS.CredentialsSecretRef,
			},
			AssumeRole: &awsclient.AssumeRoleCredentialsSource{
				SecretRef: corev1.SecretReference{
					Name:      os.Getenv(constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar),
					Namespace: controllerutils.GetHiveNamespace(),
				},
				Role: cd.Spec.Platform.AWS.CredentialsAssumeRole,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	hClient, err := clientFn(client, awsclient.Options{
		Region: cd.Spec.Platform.AWS.Region,
		CredentialsSource: awsclient.CredentialsSource{
			Secret: &awsclient.SecretCredentialsSource{
				Namespace: controllerutils.GetHiveNamespace(),
				Ref:       &config.CredentialsSecretRef,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return &awsClient{hub: hClient, user: uClient}, nil
}

// awsErrCodeEquals returns true if the error matches all these conditions:
//   - err is of type awserr.Error
//   - Error.Code() equals code
func awsErrCodeEquals(err error, code string) bool {
	if err == nil {
		return false
	}
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == code
	}
	return false
}
