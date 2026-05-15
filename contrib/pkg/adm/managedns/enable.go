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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	clientwatch "k8s.io/client-go/tools/watch"
	"k8s.io/kubectl/pkg/polymorphichelpers"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
	awscreds "github.com/openshift/hive/pkg/creds/aws"
	azurecreds "github.com/openshift/hive/pkg/creds/azure"
	gcpcreds "github.com/openshift/hive/pkg/creds/gcp"
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

	hiveClient client.WithWatch
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
	flags.StringVar(&opt.Cloud, "cloud", constants.PlatformAWS, "Cloud provider: aws(default)|gcp|azure)")
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
	hc := &hivev1.HiveConfig{}
	if err := o.hiveClient.Get(context.TODO(), types.NamespacedName{Name: hiveConfigName}, hc); err != nil {
		log.WithError(err).Fatal("error looking up HiveConfig 'hive'")
	}

	dnsConf := hivev1.ManageDNSConfig{
		Domains: args,
	}

	var credsSecret *corev1.Secret

	switch o.Cloud {
	case constants.PlatformAWS:
		// Apply a secret for credentials to manage the root domain:
		credsSecret, err = o.generateAWSCredentialsSecret()
		if err != nil {
			log.WithError(err).Fatal("error generating manageDNS credentials secret")
		}
		dnsConf.AWS = &hivev1.ManageDNSAWSConfig{
			CredentialsSecretRef: corev1.LocalObjectReference{Name: credsSecret.Name},
		}
	case constants.PlatformGCP:
		// Apply a secret for credentials to manage the root domain:
		credsSecret, err = o.generateGCPCredentialsSecret()
		if err != nil {
			log.WithError(err).Fatal("error generating manageDNS credentials secret")
		}
		dnsConf.GCP = &hivev1.ManageDNSGCPConfig{
			CredentialsSecretRef: corev1.LocalObjectReference{Name: credsSecret.Name},
		}
	case constants.PlatformAzure:
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

	err = o.hiveClient.Update(context.TODO(), hc)
	if err != nil {
		log.WithError(err).Fatal("error updating HiveConfig")
	}
	log.Info("updated HiveConfig")

	// Adding manageDNS to HiveConfig triggers a new rollout of the hiveadmission pods.
	// To know when it's safe to proceed, we will wait for the HiveConfig to reflect
	// that it has been processed successfully and then do the equivalent of a
	// kubectl rollout status --wait

	err = o.waitForHiveConfigToBeProcessed()
	if err != nil {
		log.WithError(err).Fatal("gave up waiting for HiveConfig to be processed")
	}

	if err := o.waitForHiveAdmissionPods(hiveNSName); err != nil {
		log.WithError(err).Fatal("hive admission pods never became available")
	}

	log.Info("Hive is now ready to create clusters with manageDNS=true")
	return nil
}

func (o *Options) waitForHiveAdmissionPods(hiveNSName string) error {
	log.Info("waiting for new hiveadmission pods to deploy")

	statusViewer := &polymorphichelpers.DeploymentStatusViewer{}

	// Deployment GVR
	deploymentGVR := schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}

	lw, err := newDynamicListWatchWithBookmarks(deploymentGVR, hiveNSName, hiveAdmissionDeployment)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), waitTime)
	defer cancel()

	_, err = clientwatch.UntilWithSync(ctx, lw, &unstructured.Unstructured{}, nil, func(e watch.Event) (bool, error) {
		switch t := e.Type; t {
		case watch.Added, watch.Modified:
			obj, ok := e.Object.(*unstructured.Unstructured)
			if !ok {
				return true, fmt.Errorf("failed to convert event object to unstructured")
			}

			// Convert unstructured to typed Deployment
			var deployment appsv1.Deployment
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &deployment)
			if err != nil {
				return true, fmt.Errorf("failed to convert to Deployment: %w", err)
			}

			// Convert back to unstructured for status viewer
			unstObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&deployment)
			if err != nil {
				return true, fmt.Errorf("failed to convert to unstructured runtime object: %v", err)
			}
			_, done, err := statusViewer.Status(&unstructured.Unstructured{Object: unstObj}, 0)
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
func (o *Options) waitForHiveConfigToBeProcessed() error {
	// HiveConfig GVR
	hiveConfigGVR := schema.GroupVersionResource{
		Group:    "hive.openshift.io",
		Version:  "v1",
		Resource: "hiveconfigs",
	}

	lw, err := newDynamicListWatchWithBookmarks(hiveConfigGVR, "", hiveConfigName)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), waitTime)
	defer cancel()

	_, err = clientwatch.UntilWithSync(ctx, lw, &unstructured.Unstructured{}, nil, func(e watch.Event) (bool, error) {
		switch t := e.Type; t {
		case watch.Added, watch.Modified:
			obj, ok := e.Object.(*unstructured.Unstructured)
			if !ok {
				return true, fmt.Errorf("failed to convert event object to unstructured")
			}

			// Convert unstructured to typed HiveConfig
			var hc hivev1.HiveConfig
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &hc)
			if err != nil {
				return true, fmt.Errorf("failed to convert to HiveConfig: %w", err)
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

// newDynamicListWatchWithBookmarks creates a ListWatch for the given GVR with field selector and bookmarks enabled.
// If namespace is empty, it watches cluster-scoped resources.
func newDynamicListWatchWithBookmarks(gvr schema.GroupVersionResource, namespace, name string) (*cache.ListWatch, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get REST config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = "metadata.name=" + name
			if namespace == "" {
				return dynamicClient.Resource(gvr).List(context.Background(), options)
			}
			return dynamicClient.Resource(gvr).Namespace(namespace).List(context.Background(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.AllowWatchBookmarks = true
			options.FieldSelector = "metadata.name=" + name
			if namespace == "" {
				return dynamicClient.Resource(gvr).Watch(context.Background(), options)
			}
			return dynamicClient.Resource(gvr).Namespace(namespace).Watch(context.Background(), options)
		},
	}, nil
}

func (o *Options) generateAWSCredentialsSecret() (*corev1.Secret, error) {
	defaultCredsFilePath := filepath.Join(o.homeDir, ".aws", "credentials")
	accessKeyID, secretAccessKey, err := awscreds.GetAWSCreds(o.CredsFile, defaultCredsFilePath)
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
	saFileContents, err := gcpcreds.GetCreds(o.CredsFile)
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
	spFileContents, err := azurecreds.GetCreds(o.CredsFile)
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
	return resource.NewHelper(
		log.WithField("command", "adm manage-dns enable"),
		resource.FromRESTConfig(cfg),
		resource.WithControllerName("util-managedns-enable"))
}

func (o *Options) setupLocalClients() error {
	log.Debug("creating cluster client config")
	hiveClient, err := utils.GetClient("hiveutil-managedns-enable")
	if err != nil {
		log.WithError(err).Error("failed to create a hive config client")
		return err
	}
	o.hiveClient = hiveClient

	return nil
}
