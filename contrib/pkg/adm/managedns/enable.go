package managedns

import (
	"context"
	"os/user"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	awsutils "github.com/openshift/hive/contrib/pkg/utils/aws"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/resource"
)

const longDesc = `
OVERVIEW
The enable command can be used to enable global
managed DNS functionality in HiveConfig.

The command will update your current HiveConfig to configure the requested
managed domains, create a credentials secret for your cloud provider, and link it in
the ExternalDNS section of HiveConfig.
`
const (
	cloudAWS                   = "aws"
	hiveNamespace              = "hive"
	manageDNSCredentialsSecret = "manage-dns-creds"
)

var (
	validClouds = map[string]bool{
		cloudAWS: true,
	}
)

// Options is the set of options to generate and apply a new cluster deployment
type Options struct {
	Cloud     string
	CredsFile string
	homeDir   string
}

// NewEnableManageDNSCommand creates a command that generates and applies artifacts to enable managed
// DNS globally for the Hive cluster.
func NewEnableManageDNSCommand() *cobra.Command {
	opt := &Options{}

	cmd := &cobra.Command{
		Use:   `enable domain1.example.com domain2.example.com ...`,
		Short: "Enable managed DNS globally for the Hive cluster.",
		Long:  longDesc,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			if err := opt.Complete(cmd, args); err != nil {
				return
			}
			if err := opt.Validate(cmd); err != nil {
				return
			}
			dynClient, err := contributils.GetClient()
			if err != nil {
				log.WithError(err).Fatal("error creating kube clients")
			}

			err = opt.Run(dynClient, args)
			if err != nil {
				log.WithError(err).Error("Error")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opt.Cloud, "cloud", cloudAWS, "Cloud provider: aws(default)|gcp)")
	flags.StringVar(&opt.CredsFile, "creds-file", "", "Cloud credentials file (defaults vary depending on cloud)")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	o.homeDir = "."

	if u, err := user.Current(); err == nil {
		o.homeDir = u.HomeDir
	}
	return nil
}

// Validate ensures that option values make sense
func (o *Options) Validate(cmd *cobra.Command) error {
	return nil
}

// Run executes the command
func (o *Options) Run(dynClient client.Client, args []string) error {
	if err := apis.AddToScheme(scheme.Scheme); err != nil {
		return err
	}
	rh, err := o.getResourceHelper()
	if err != nil {
		return err
	}

	// Update the current HiveConfig, which should always exist as the operator will
	// create a default one once run.
	hc := &hivev1.HiveConfig{}
	err = dynClient.Get(context.Background(), types.NamespacedName{Name: "hive"}, hc)
	if err != nil {
		log.WithError(err).Fatal("error looking up HiveConfig 'hive'")
	}

	hc.Spec.ManagedDomains = args

	var credsSecret *corev1.Secret

	if o.Cloud == cloudAWS {
		// Apply a secret for credentials to manage the root domain:
		credsSecret, err = o.generateAWSCredentialsSecret()
		if err != nil {
			log.WithError(err).Fatal("error generating manageDNS credentials secret")
		}
		accessor, err := meta.Accessor(credsSecret)
		if err != nil {
			log.WithError(err).Errorf("Cannot create accessor for object of type %T", credsSecret)
			return err
		}
		accessor.SetNamespace(hiveNamespace)
		rh.ApplyRuntimeObject(credsSecret, scheme.Scheme)
	}

	hc.Spec.ExternalDNS = &hivev1.ExternalDNSConfig{
		AWS: &hivev1.ExternalDNSAWSConfig{
			Credentials: corev1.LocalObjectReference{Name: manageDNSCredentialsSecret},
		},
	}

	err = dynClient.Update(context.Background(), hc)
	if err != nil {
		log.WithError(err).Fatal("error updating HiveConfig")
	}

	return nil
}

func (o *Options) generateAWSCredentialsSecret() (*corev1.Secret, error) {
	defaultCredsFilePath := filepath.Join(o.homeDir, ".aws", "credentials")
	accessKeyID, secretAccessKey, err := awsutils.GetAWSCreds(o.CredsFile, defaultCredsFilePath)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      manageDNSCredentialsSecret,
			Namespace: hiveNamespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"aws_access_key_id":     accessKeyID,
			"aws_secret_access_key": secretAccessKey,
		},
	}, nil
}

func (o *Options) getResourceHelper() (*resource.Helper, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		log.WithError(err).Error("Cannot get client config")
		return nil, err
	}
	helper := resource.NewHelperFromRESTConfig(cfg, log.WithField("command", "create-cluster"))
	return helper, nil
}
