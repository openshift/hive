package awsprivatelink

import (
	"context"
	"errors"
	"os/user"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveutils "github.com/openshift/hive/contrib/pkg/utils"
	awsutils "github.com/openshift/hive/contrib/pkg/utils/aws"
	"github.com/openshift/hive/pkg/awsclient"
	operatorutils "github.com/openshift/hive/pkg/operator/hive"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type enableOptions struct {
	homeDir string
	// AWS region of the Hive cluster
	region        string
	infraId       string
	dnsRecordType string

	dynamicClient client.Client
	// AWS clients in Hive cluster's region
	awsClients awsclient.Client
}

func NewEnableAWSPrivateLinkCommand() *cobra.Command {
	opt := &enableOptions{}

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable AWS PrivateLink",
		Long: `Enable AWS PrivateLink:
1) Extract AWS Hub account credentials from the environment where this command is called
2) Create a Secret with the above credential.
3) Add a reference to the Secret in HiveConfig.spec.awsPrivateLink.credentialsSecretRef.
4) Add the active cluster's VPC to the list of HiveConfig.spec.awsPrivateLink.associatedVPCs`,
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
	// Get controller-runtime dynamic client
	if err := configv1.Install(scheme.Scheme); err != nil {
		log.WithError(err).Fatal("Failed to add Openshift configv1 types to the default scheme")
	}
	dynamicClient, err := hiveutils.GetClient()
	if err != nil {
		log.WithError(err).Fatal("Failed to create controller-runtime client")
	}
	o.dynamicClient = dynamicClient

	// Get current user's home directory
	o.homeDir = "."
	u, err := user.Current()
	if err != nil {
		log.WithError(err).Fatal("Failed to get the current user")
	}
	o.homeDir = u.HomeDir

	// Get AWS clients
	o.region, o.infraId, err = o.getRegionAndInfraId()
	if err != nil {
		log.WithError(err).Fatal("Failed to get region and infraID from Infrastructure/cluster")
	}
	log.Debugf("Found region = %v, infraId = %v", o.region, o.infraId)
	awsClients, err := awsclient.NewClientFromSecret(nil, o.region)
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
	if err := o.dynamicClient.Get(context.Background(), types.NamespacedName{Name: "hive"}, hiveConfig); err != nil {
		log.WithError(err).Fatal("Failed to get HiveConfig/hive")
	}
	if hiveConfig.Spec.AWSPrivateLink != nil {
		log.Fatal("AWS Private Link is already enabled. If a previous configuration attempt did not complete, " +
			"you can clean up via `hiveutil awsprivatelink disable` and try again.")
	}

	// Get active cluster's VPC, filtering by infra-id
	targetTagKey := "kubernetes.io/cluster/" + o.infraId
	describeVPCsOutput, err := o.awsClients.DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag-key"),
				Values: []*string{aws.String(targetTagKey)},
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
	vpcID := *describeVPCsOutput.Vpcs[0].VpcId
	log.Debugf("Found VPC ID = %v for the active cluster", vpcID)

	// Create Hub account credentials Secret
	hiveNS := operatorutils.GetHiveNamespace(hiveConfig)
	hubCredentialsSecret, err := o.generateAWSCredentialsSecret(hiveNS)
	if err != nil {
		log.WithError(err).Fatal("Failed to generate Secret with AWS credentials")
	}
	switch err = o.dynamicClient.Create(context.Background(), hubCredentialsSecret); {
	case err == nil:
		log.Infof("Secret/%s created in namespace %s", awsutils.PrivateLinkHubAcctCredsName, hiveNS)
	case apierrors.IsAlreadyExists(err):
		log.Warnf("Secret/%s already exists in namespace %s", awsutils.PrivateLinkHubAcctCredsName, hiveNS)
	default:
		log.WithError(err).Fatalf("Failed to create Secret/%s in namespace %s", awsutils.PrivateLinkHubAcctCredsName, hiveNS)
	}

	// Update HiveConfig
	hiveConfig.Spec.AWSPrivateLink = &hivev1.AWSPrivateLinkConfig{
		AssociatedVPCs: []hivev1.AWSAssociatedVPC{
			{
				AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{VPCID: vpcID, Region: o.region},
			},
		},
		CredentialsSecretRef: corev1.LocalObjectReference{Name: hubCredentialsSecret.Name},
		DNSRecordType:        hivev1.AWSPrivateLinkDNSRecordType(o.dnsRecordType),
	}
	if err = o.dynamicClient.Update(context.Background(), hiveConfig); err != nil {
		log.WithError(err).Fatal("Failed to update HiveConfig")
	}
	log.Info("HiveConfig updated")

	return nil
}

func (o *enableOptions) getRegionAndInfraId() (string, string, error) {
	infrastructure := &configv1.Infrastructure{}
	if err := o.dynamicClient.Get(context.Background(), types.NamespacedName{Name: "cluster"}, infrastructure); err != nil {
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

// Adapted from contrib/pkg/adm/managedns/enable.go:generateAWSCredentialsSecret()
func (o *enableOptions) generateAWSCredentialsSecret(namespace string) (*corev1.Secret, error) {
	defaultCredsFilePath := filepath.Join(o.homeDir, ".aws", "credentials")
	accessKeyID, secretAccessKey, err := awsutils.GetAWSCreds("", defaultCredsFilePath)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      awsutils.PrivateLinkHubAcctCredsName,
			Namespace: namespace,
			// Secrets without this label (e.g. the ones created and configured manually) won't be deleted
			// when calling "hiveutil awsprivatelink disable".
			Labels: map[string]string{awsutils.PrivateLinkHubAcctCredsLabel: "true"},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"aws_access_key_id":     accessKeyID,
			"aws_secret_access_key": secretAccessKey,
		},
	}, nil
}
