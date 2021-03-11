package installmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
)

// AWSCredentials is a supported external process credential provider as detailed in
// https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sourcing-external.html
type AWSCredentials struct {
	output     io.WriteCloser
	kubeClient client.Client

	ServiceProviderSecretName      string
	ServiceProviderSecretNamespace string

	RoleARN    string
	ExternalID string
}

// NewInstallManagerAWSCredentials is the entrypoint to load credentials for AWS SDK
// using the service provider credentials. It supports the external process credential
// provider as mentioned in https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sourcing-external.html
func NewInstallManagerAWSCredentials() *cobra.Command {
	options := &AWSCredentials{}
	cmd := &cobra.Command{
		Use:   "aws-credentials",
		Short: "Loads AWS credentials using the service provider credentials",
		Long:  "This loads the AWS credentials using the service provider credentials and then returns them in format defined in https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sourcing-external.html",
		Run: func(cmd *cobra.Command, args []string) {
			if err := options.Complete(args); err != nil {
				fmt.Fprint(os.Stderr, err.Error())
				os.Exit(1)
				return
			}

			if err := options.Validate(); err != nil {
				fmt.Fprint(os.Stderr, err.Error())
				os.Exit(1)
				return
			}

			var err error
			options.kubeClient, err = contributils.GetClient()
			if err != nil {
				fmt.Fprint(os.Stderr, err.Error())
				os.Exit(1)
				return
			}

			if err := options.Run(); err != nil {
				fmt.Fprint(os.Stderr, err.Error())
				os.Exit(1)
				return
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&options.ServiceProviderSecretNamespace, "namespace", "", "The namespace where the service provider secret is stored")
	cmd.MarkFlagRequired("namespace")
	flags.StringVar(&options.RoleARN, "role-arn", "", "The IAM role that should be assumed")
	cmd.MarkFlagRequired("role-arn")
	flags.StringVar(&options.ExternalID, "external-id", "", "External identifier required to assume the role specified.")

	return cmd
}

// Validate the options
func (options *AWSCredentials) Validate() error { return nil }

// Complete the options using the args
func (options *AWSCredentials) Complete(args []string) error {
	options.output = os.Stdout
	options.ServiceProviderSecretName = os.Getenv(constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar)
	return nil
}

// Run runs the command using the options.
func (options *AWSCredentials) Run() error {
	var secret *corev1.Secret
	if options.ServiceProviderSecretName != "" {
		secret = &corev1.Secret{}
		if err := options.kubeClient.Get(context.TODO(),
			client.ObjectKey{Namespace: options.ServiceProviderSecretNamespace, Name: options.ServiceProviderSecretName},
			secret); err != nil {
			return errors.Wrap(err, "failed to get the service provider secret")
		}
	}

	sess, err := awsclient.NewSessionFromSecret(secret, "")
	if err != nil {
		return errors.Wrap(err, "failed to create AWS session")
	}

	duration := stscreds.DefaultDuration
	creds := stscreds.NewCredentials(sess, options.RoleARN, func(p *stscreds.AssumeRoleProvider) {
		p.Duration = duration
		if options.ExternalID != "" {
			p.ExternalID = &options.ExternalID
		}
	})
	v, err := creds.Get()
	if err != nil {
		return errors.Wrap(err, "failed to Assume the require role")
	}

	resp, err := newCredentialProcessResponse(v, time.Now().Add(-1*time.Minute).Add(duration))
	if err != nil {
		return errors.Wrap(err, "failed to create response for credential process")
	}
	_, err = fmt.Fprint(options.output, resp)
	return err
}

func newCredentialProcessResponse(v credentials.Value, expiry time.Time) (string, error) {
	resp := &credentialProcessResponse{
		Version:         1,
		AccessKeyID:     v.AccessKeyID,
		SecretAccessKey: v.SecretAccessKey,
		SessionToken:    v.SessionToken,
		Expiration:      &expiry,
	}
	outRaw, err := json.Marshal(resp)
	if err != nil {
		return "", errors.Wrap(err, "failed to create the credential process response")
	}
	outRawCompact := &bytes.Buffer{}
	if err := json.Compact(outRawCompact, outRaw); err != nil {
		return "", errors.Wrap(err, "failed to compact the JSON response")
	}

	return outRawCompact.String(), nil
}

type credentialProcessResponse struct {
	Version         int
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string
	SessionToken    string
	Expiration      *time.Time
}
