package managedns

import (
	"context"
	"fmt"
	"os/user"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	clientwatch "k8s.io/client-go/tools/watch"
	"k8s.io/kubectl/pkg/polymorphichelpers"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client/config"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveutils "github.com/openshift/hive/contrib/pkg/utils"
	awsutils "github.com/openshift/hive/contrib/pkg/utils/aws"
	azureutils "github.com/openshift/hive/contrib/pkg/utils/azure"
	gcputils "github.com/openshift/hive/contrib/pkg/utils/gcp"
	hiveclient "github.com/openshift/hive/pkg/client/clientset/versioned"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/resource"
	"github.com/openshift/hive/pkg/util/scheme"
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
	cloudAWS                = "aws"
	cloudGCP                = "gcp"
	cloudAzure              = "azure"
	hiveAdmissionDeployment = "hiveadmission"
	hiveConfigName          = "hive"
	waitTime                = time.Minute * 2
)

// Options is the set of options to generate and apply a new cluster deployment
type Options struct {
	Cloud     string
	CredsFile string
	homeDir   string

	AzureResourceGroup string

	dynamicClient dynamic.Interface
	hiveClient    *hiveclient.Clientset
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

			if err := opt.setupLocalClients(); err != nil {
				log.WithError(err).Fatal("error creating hive client")
			}

			if err := opt.Run(args); err != nil {
				log.WithError(err).Fatal("Failed while deploying updated managed dns")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opt.Cloud, "cloud", cloudAWS, "Cloud provider: aws(default)|gcp|azure)")
	flags.StringVar(&opt.CredsFile, "creds-file", "", "Cloud credentials file (defaults vary depending on cloud)")
	flags.StringVar(&opt.AzureResourceGroup, "azure-resource-group-name", "os4-common", "Azure Resource Group (Only applicable if --cloud azure)")
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
func (o *Options) Run(args []string) error {
	scheme := scheme.GetScheme()
	rh, err := o.getResourceHelper()
	if err != nil {
		return err
	}

	// Update the current HiveConfig, which should always exist as the operator will
	// create a default one once run.
	hc, err := o.hiveClient.HiveV1().HiveConfigs().Get(context.Background(), hiveConfigName, metav1.GetOptions{})
	if err != nil {
		log.WithError(err).Fatal("error looking up HiveConfig 'hive'")
	}

	dnsConf := hivev1.ManageDNSConfig{
		Domains: args,
	}

	var credsSecret *corev1.Secret

	switch o.Cloud {
	case cloudAWS:
		// Apply a secret for credentials to manage the root domain:
		credsSecret, err = o.generateAWSCredentialsSecret()
		if err != nil {
			log.WithError(err).Fatal("error generating manageDNS credentials secret")
		}
		dnsConf.AWS = &hivev1.ManageDNSAWSConfig{
			CredentialsSecretRef: corev1.LocalObjectReference{Name: credsSecret.Name},
		}
	case cloudGCP:
		// Apply a secret for credentials to manage the root domain:
		credsSecret, err = o.generateGCPCredentialsSecret()
		if err != nil {
			log.WithError(err).Fatal("error generating manageDNS credentials secret")
		}
		dnsConf.GCP = &hivev1.ManageDNSGCPConfig{
			CredentialsSecretRef: corev1.LocalObjectReference{Name: credsSecret.Name},
		}
	case cloudAzure:
		credsSecret, err = o.generateAzureCredentialsSecret()
		if err != nil {
			log.WithError(err).Fatal("error generating manageDNS credentials secret")
		}
		dnsConf.Azure = &hivev1.ManageDNSAzureConfig{
			CredentialsSecretRef: corev1.LocalObjectReference{Name: credsSecret.Name},
			ResourceGroupName:    o.AzureResourceGroup,
		}
	default:
		log.WithField("cloud", o.Cloud).Fatal("unsupported cloud")
	}

	log.Debug("adding new ManagedDomain config to existing HiveConfig")
	hc.Spec.ManagedDomains = append(hc.Spec.ManagedDomains, dnsConf)

	hiveNSName := hc.Spec.TargetNamespace
	if hiveNSName == "" {
		hiveNSName = constants.DefaultHiveNamespace
	}

	// Make it easier to find the secret generically
	credsSecret.Labels = map[string]string{
		"hive.openshift.io/managed-dns-credentials": "true",
	}

	log.Infof("created cloud credentials secret: %s", credsSecret.Name)
	credsSecret.Namespace = hiveNSName
	if _, err := rh.ApplyRuntimeObject(credsSecret, scheme); err != nil {
		log.WithError(err).Fatal("failed to save generated secret")
	}

	_, err = o.hiveClient.HiveV1().HiveConfigs().Update(context.Background(), hc, metav1.UpdateOptions{})
	if err != nil {
		log.WithError(err).Fatal("error updating HiveConfig")
	}
	log.Info("updated HiveConfig")

	// Adding manageDNS to HiveConfig triggers a new rollout of the hiveadmission pods.
	// To know when it's safe to proceed, we will wait for the HiveConfig to reflect
	// that it has been processed successfully and then do the equivalent of a
	// kubectl rollout status --wait

	err = waitForHiveConfigToBeProcessed(o.hiveClient)
	if err != nil {
		log.WithError(err).Fatal("gave up waiting for HiveConfig to be processed")
	}

	if err := waitForHiveAdmissionPods(o.dynamicClient, hiveNSName); err != nil {
		log.WithError(err).Fatal("hive admission pods never became available")
	}

	log.Info("Hive is now ready to create clusters with manageDNS=true")
	return nil
}

func waitForHiveAdmissionPods(dynClient dynamic.Interface, hiveNSName string) error {
	resourceName := "deployments"
	gvr := appsv1.SchemeGroupVersion.WithResource(resourceName)

	log.Info("waiting for new hiveadmission pods to deploy")

	statusViewer := &polymorphichelpers.DeploymentStatusViewer{}

	fieldSelector := fields.OneTermEqualSelector("metadata.name", hiveAdmissionDeployment).String()

	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return dynClient.Resource(gvr).Namespace(hiveNSName).List(context.Background(), options)

		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return dynClient.Resource(gvr).Namespace(hiveNSName).Watch(context.Background(), options)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), waitTime)
	defer cancel()

	_, err := clientwatch.UntilWithSync(ctx, lw, &unstructured.Unstructured{}, nil, func(e watch.Event) (bool, error) {
		switch t := e.Type; t {
		case watch.Added, watch.Modified:
			unstObj, ok := e.Object.(runtime.Unstructured)
			if !ok {
				return true, fmt.Errorf("failed to convert to unstructured runtime object")
			}
			_, done, err := statusViewer.Status(unstObj, 0)
			if err != nil {
				return false, err
			}
			if done {
				return true, nil
			}

			return false, nil
		case watch.Deleted:
			return true, fmt.Errorf("object has been deleted")
		default:
			return true, fmt.Errorf("internal error: unexpected event %#v", e)
		}
	})

	log.Debug("done waiting for hiveadmission pods")
	return err
}

// wiatForHiveConfigToBeProcessed will wait for the HiveConfig.Status.ObservedGeneration to match
// the HiveConfig's Generation (and status showing ConfigApplied == true).
func waitForHiveConfigToBeProcessed(hiveClient *hiveclient.Clientset) error {
	fieldSelector := fields.OneTermEqualSelector("metadata.name", hiveConfigName).String()

	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fieldSelector
			return hiveClient.HiveV1().HiveConfigs().List(context.Background(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = fieldSelector
			return hiveClient.HiveV1().HiveConfigs().Watch(context.Background(), options)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), waitTime)
	defer cancel()

	_, err := clientwatch.UntilWithSync(ctx, lw, &hivev1.HiveConfig{}, nil, func(e watch.Event) (bool, error) {
		switch t := e.Type; t {
		case watch.Added, watch.Modified:
			hc, ok := e.Object.(*hivev1.HiveConfig)
			if !ok {
				return true, fmt.Errorf("failed to convert event object into HiveConfig")
			}
			if hc.Generation == hc.Status.ObservedGeneration && hc.Status.ConfigApplied {
				return true, nil
			}
			log.Debug("still waiting for hiveconfig to be processed")
			return false, nil
		case watch.Deleted:
			return true, fmt.Errorf("object has been deleted")
		default:
			return true, fmt.Errorf("internal error: unexpected event %#v", e)
		}
	})

	log.Debug("done waiting for hiveconfig to be processed")
	return err
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
			Name: fmt.Sprintf("aws-dns-creds-%s", uuid.New().String()[:5]),
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"aws_access_key_id":     accessKeyID,
			"aws_secret_access_key": secretAccessKey,
		},
	}, nil
}

func (o *Options) generateGCPCredentialsSecret() (*corev1.Secret, error) {
	saFileContents, err := gcputils.GetCreds(o.CredsFile)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("gcp-dns-creds-%s", uuid.New().String()[:5]),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			constants.GCPCredentialsName: saFileContents,
		},
	}, nil
}

func (o *Options) generateAzureCredentialsSecret() (*corev1.Secret, error) {
	spFileContents, err := azureutils.GetCreds(o.CredsFile)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("azure-dns-creds-%s", uuid.New().String()[:5]),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			constants.AzureCredentialsName: spFileContents,
		},
	}, nil
}

func (o *Options) getResourceHelper() (resource.Helper, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		log.WithError(err).Error("Cannot get client config")
		return nil, err
	}
	return resource.NewHelperFromRESTConfig(cfg, log.WithField("command", "adm manage-dns enable"))
}

func (o *Options) setupLocalClients() error {
	log.Debug("creating cluster client config")
	cfg, err := hiveutils.GetClientConfig()
	if err != nil {
		log.WithError(err).Error("cannot obtain client config")
		return err
	}

	hiveClient, err := hiveclient.NewForConfig(cfg)
	if err != nil {
		log.WithError(err).Error("failed to create a hive config client")
		return err
	}
	o.hiveClient = hiveClient

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.WithError(err).Error("failed to create a dynamic config client")
		return err
	}
	o.dynamicClient = dynamicClient

	return nil
}
