package awsprivatelink

import (
	"context"
	"errors"
	"os/user"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/awsprivatelink/common"
	"github.com/openshift/hive/pkg/awsclient"
	awscreds "github.com/openshift/hive/pkg/creds/aws"
	operatorutils "github.com/openshift/hive/pkg/operator/hive"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type enableOptions struct {
	homeDir string
	// AWS region of the Hive cluster
	region        string
	infraId       string
	dnsRecordType string

	// AWS clients in Hive cluster's region
	awsClients awsclient.Client
}

func NewEnableAWSPrivateLinkCommand() *cobra.Command {
	opt := &enableOptions{}

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable AWS PrivateLink",
		Long: `Enable AWS PrivateLink:
Depending on whether --creds-secret is specified, this command either:
1-1) Extract AWS Hub account credentials from the environment where this command is called
1-2) Create a Secret with the above credential
Or
1-1) Copy the passed-in Secret to Hive's namespace

And then
2-1) Add a reference to the newly-created Secret in HiveConfig.spec.awsPrivateLink.credentialsSecretRef
2-2) Add the active cluster's VPC to the list of HiveConfig.spec.awsPrivateLink.associatedVPCs`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(cmd, args); err != nil {
				return
			}
			if err := opt.Validate(cmd, args); err != nil {
				return
			}
			if err := opt.Run(cmd, args); err != nil {
				return
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opt.dnsRecordType, "dns-record-type", "Alias", "HiveConfig.spec.awsPrivateLink.dnsRecordType")
	return cmd
}

func (o *enableOptions) Complete(cmd *cobra.Command, args []string) error {
	// Get current user's home directory
	o.homeDir = "."
	u, err := user.Current()
	if err != nil {
		log.WithError(err).Warn("Failed to get the current user, is hiveutil running in Openshift ?")
	} else {
		o.homeDir = u.HomeDir
	}

	// Get AWS clients
	o.region, o.infraId, err = o.getRegionAndInfraId()
	if err != nil {
		log.WithError(err).Fatal("Failed to get region and infraID from Infrastructure/cluster")
	}
	log.Debugf("Found region = %v, infraId = %v", o.region, o.infraId)
	awsClients, err := awsclient.NewClientFromSecret(common.CredsSecret, o.region)
	if err != nil {
		log.WithError(err).Fatal("Failed to create AWS clients")
	}
	o.awsClients = awsClients

	return nil
}

func (o *enableOptions) Validate(cmd *cobra.Command, args []string) error {
	switch o.dnsRecordType {
	case "Alias", "ARecord":
	default:
		log.Fatal(`--dns-record-type must be one of "Alias", "ARecord"`)
	}

	return nil
}

func (o *enableOptions) Run(cmd *cobra.Command, args []string) error {
	// Get HiveConfig
	hiveConfig := &hivev1.HiveConfig{}
	if err := common.DynamicClient.Get(context.Background(), types.NamespacedName{Name: "hive"}, hiveConfig); err != nil {
		log.WithError(err).Fatal("Failed to get HiveConfig/hive")
	}
	if hiveConfig.Spec.AWSPrivateLink != nil {
		log.Fatal("AWS Private Link is already enabled. If a previous configuration attempt did not complete, " +
			"you can clean up via `hiveutil awsprivatelink disable` and try again.")
	}

	// Get active cluster's VPC, filtering by infra-id
	targetTagKey := "kubernetes.io/cluster/" + o.infraId
	describeVPCsOutput, err := o.awsClients.DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag-key"),
				Values: []string{targetTagKey},
			},
		},
	})
	if err != nil {
		log.WithError(err).Fatal("Failed to get VPC of the active cluster")
	}
	if len(describeVPCsOutput.Vpcs) == 0 {
		log.Fatal("VPC of the active cluster not found")
	}
	if len(describeVPCsOutput.Vpcs) > 1 {
		log.Fatalf("Multiple VPCs found with tag key %s, cannot determine VPC of the active cluster", targetTagKey)
	}
	vpcID := aws.ToString(describeVPCsOutput.Vpcs[0].VpcId)
	log.Debugf("Found VPC ID = %v for the active cluster", vpcID)

	hiveNS := operatorutils.GetHiveNamespace(hiveConfig)
	credsSecretInHiveNS, err := o.getOrCopyCredsSecret(common.CredsSecret, hiveNS)
	if err != nil {
		log.WithError(err).Fatal("Failed to generate Secret with AWS credentials")
	}

	switch err = common.DynamicClient.Create(context.Background(), credsSecretInHiveNS); {
	case err == nil:
		log.Infof("Secret/%s created in namespace %s", privateLinkHubAcctCredsName, hiveNS)
	case apierrors.IsAlreadyExists(err):
		log.Warnf("Secret/%s already exists in namespace %s", privateLinkHubAcctCredsName, hiveNS)
	default:
		log.WithError(err).Fatalf("Failed to create Secret/%s in namespace %s", privateLinkHubAcctCredsName, hiveNS)
	}

	// Update HiveConfig
	hiveConfig.Spec.AWSPrivateLink = &hivev1.AWSPrivateLinkConfig{
		AssociatedVPCs: []hivev1.AWSAssociatedVPC{
			{
				AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{VPCID: vpcID, Region: o.region},
			},
		},
		CredentialsSecretRef: corev1.LocalObjectReference{Name: credsSecretInHiveNS.Name},
		DNSRecordType:        hivev1.AWSPrivateLinkDNSRecordType(o.dnsRecordType),
	}
	if err = common.DynamicClient.Update(context.Background(), hiveConfig); err != nil {
		log.WithError(err).Fatal("Failed to update HiveConfig")
	}
	log.Info("HiveConfig updated")

	return nil
}

func (o *enableOptions) getRegionAndInfraId() (string, string, error) {
	infrastructure := &configv1.Infrastructure{}
	if err := common.DynamicClient.Get(context.Background(), types.NamespacedName{Name: "cluster"}, infrastructure); err != nil {
		return "", "", err
	}
	if infrastructure.Status.PlatformStatus == nil {
		return "", "", errors.New("Infrastructure.status.platformStatus is empty")
	}
	if infrastructure.Status.PlatformStatus.AWS == nil {
		return "", "", errors.New("Infrastructure.status.platformStatus.aws is empty")
	}

	return infrastructure.Status.PlatformStatus.AWS.Region, infrastructure.Status.InfrastructureName, nil
}

func (o *enableOptions) getOrCopyCredsSecret(source *corev1.Secret, namespace string) (*corev1.Secret, error) {
	out := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      privateLinkHubAcctCredsName,
			Namespace: namespace,
			// Secrets without this label (e.g., the ones created and configured manually) won't be deleted
			// when calling "hiveutil awsprivatelink disable".
			Labels: map[string]string{privateLinkHubAcctCredsLabel: "true"},
		},
		Type: corev1.SecretTypeOpaque,
	}

	switch {
	// Copy source
	case source != nil:
		out.Data = source.Data
	// Get creds from environment
	default:
		defaultCredsFilePath := filepath.Join(o.homeDir, ".aws", "credentials")
		accessKeyID, secretAccessKey, err := awscreds.GetAWSCreds("", defaultCredsFilePath)
		if err != nil {
			return nil, err
		}
		out.StringData = map[string]string{
			"aws_access_key_id":     accessKeyID,
			"aws_secret_access_key": secretAccessKey,
		}
	}
	return out, nil
}
