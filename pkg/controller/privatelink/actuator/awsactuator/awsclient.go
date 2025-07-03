package awsactuator

import (
	"github.com/pkg/errors"

	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/apis/hive/v1/aws"
	awsclient "github.com/openshift/hive/pkg/awsclientv2"
)

type awsClientFn func(client.Client, awsclient.Options) (awsclient.Client, error)

type AssumeRole struct {
	Role            *aws.AssumeRole
	SecretName      string
	SecretNamespace string
}

func newAWSClient(client client.Client, clientFn awsClientFn, region string, namespace string, secretRef *corev1.LocalObjectReference, assumeRole *AssumeRole) (awsclient.Client, error) {
	if clientFn == nil {
		clientFn = awsclient.New
	}

	options := awsclient.Options{
		Region: region,
		CredentialsSource: awsclient.CredentialsSource{
			Secret: &awsclient.SecretCredentialsSource{
				Namespace: namespace,
				Ref:       secretRef,
			},
		},
	}

	if assumeRole != nil && assumeRole.Role != nil {
		options.CredentialsSource.AssumeRole = &awsclient.AssumeRoleCredentialsSource{
			SecretRef: corev1.SecretReference{
				Name:      assumeRole.SecretName,
				Namespace: assumeRole.SecretNamespace,
			},
			Role: assumeRole.Role,
		}
	}

	return clientFn(client, options)
}

// awsErrCodeEquals returns true if the error matches all these conditions:
//   - err is of type awserr.Error
//   - Error.Code() equals code
func isNoSuchHostedZone(err error) bool {
	if err == nil {
		return false
	}
	var nshze route53types.NoSuchHostedZone
	return errors.As(err, &nshze)
}
