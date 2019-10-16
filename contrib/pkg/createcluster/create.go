package createcluster

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	netopv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/resource"
)

const longDesc = `
OVERVIEW
The hiveutil create-cluster command generates and applies the artifacts needed
to create a new Hive cluster deployment. By default, the clusterdeployment is
generated along with corresponding secrets and then applied to the current
cluster. If you don't need secrets generated, specify --include-secrets=false
in the command line. If you don't want to apply the cluster deployment and
only output it locally, specify the output flag (-o json) or (-o yaml) to
specify your output format.

IMAGES
An existing ClusterImageSet can be specified with the --image-set
flag. Otherwise, one will be generated using the images specified for the
cluster deployment. If you don't wish to use a ClusterImageSet, specify
--use-image-set=false. This will result in images only specified on the
cluster itself.


ENVIRONMENT VARIABLES
The command will use the following environment variables for its output:

PUBLIC_SSH_KEY - If present, it is used as the new cluster's public SSH key.
It overrides the public ssh key flags. If not, --ssh-public-key will be used.
If that is not specified, then --ssh-public-key-file is used.
That file's default value is %[1]s.

PULL_SECRET - If present, it is used as the cluster deployment's pull
secret and will override the --pull-secret flag. If not present, and
the --pull-secret flag is not specified, then the --pull-secret-file is
used. That file's default value is %[2]s.

AWS_SECRET_ACCESS_KEY and AWS_ACCESS_KEY_ID - Are used to determine your
AWS credentials. These are only relevant for creating a cluster on AWS. If
--creds-file is used it will take precedence over these environment
variables.

RELEASE_IMAGE - Release image to use to install the cluster. If not specified,
the --release-image flag is used. If that's not specified, a default image is
obtained from a the following URL:
https://openshift-release.svc.ci.openshift.org/api/v1/releasestream/4-stable/latest

INSTALLER_IMAGE - Installer image to use to install the cluster. If not specified,
the --installer-image flag is used. If that's not specified, the image is
derived from the release image at runtime.
`
const (
	deleteAfterAnnotation      = "hive.openshift.io/delete-after"
	tryInstallOnceAnnotation   = "hive.openshift.io/try-install-once"
	tryUninstallOnceAnnotation = "hive.openshift.io/try-uninstall-once"
	cloudAWS                   = "aws"
	cloudAzure                 = "azure"
	cloudGCP                   = "gcp"
)

var (
	validClouds = map[string]bool{
		cloudAWS:   true,
		cloudAzure: true,
		cloudGCP:   true,
	}
)

// Options is the set of options to generate and apply a new cluster deployment
type Options struct {
	Name                     string
	Namespace                string
	SSHPublicKeyFile         string
	SSHPublicKey             string
	SSHPrivateKeyFile        string
	BaseDomain               string
	PullSecret               string
	PullSecretFile           string
	Cloud                    string
	CredsFile                string
	ClusterImageSet          string
	InstallerImage           string
	ReleaseImage             string
	ReleaseImageSource       string
	DeleteAfter              string
	ServingCert              string
	ServingCertKey           string
	UseClusterImageSet       bool
	ManageDNS                bool
	Output                   string
	IncludeSecrets           bool
	InstallOnce              bool
	UninstallOnce            bool
	SimulateBootstrapFailure bool
	WorkerNodes              int64

	// Azure
	AzureBaseDomainResourceGroupName string

	// GCP
	GCPProjectID string

	homeDir       string
	cloudProvider cloudProvider
}

// NewCreateClusterCommand creates a command that generates and applies cluster deployment artifacts.
func NewCreateClusterCommand() *cobra.Command {
	opt := &Options{}

	opt.homeDir = "."

	if u, err := user.Current(); err == nil {
		opt.homeDir = u.HomeDir
	}
	defaultSSHPublicKeyFile := filepath.Join(opt.homeDir, ".ssh", "id_rsa.pub")

	defaultPullSecretFile := filepath.Join(opt.homeDir, ".pull-secret")
	if _, err := os.Stat(defaultPullSecretFile); os.IsNotExist(err) {
		defaultPullSecretFile = ""
	} else if err != nil {
		log.WithError(err).Errorf("%v can not be used", defaultPullSecretFile)
	}

	cmd := &cobra.Command{
		Use: `create-cluster CLUSTER_DEPLOYMENT_NAME
create-cluster CLUSTER_DEPLOYMENT_NAME --cloud=aws
create-cluster CLUSTER_DEPLOYMENT_NAME --cloud=azure --azure-base-domain-resource-group-name=RESOURCE_GROUP_NAME
create-cluster CLUSTER_DEPLOYMENT_NAME --cloud=gcp --gcp-project-id=PROJECT_ID`,
		Short: "Creates a new Hive cluster deployment",
		Long:  fmt.Sprintf(longDesc, defaultSSHPublicKeyFile, defaultPullSecretFile),
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			if err := opt.Complete(cmd, args); err != nil {
				return
			}
			if err := opt.Validate(cmd); err != nil {
				return
			}
			err := opt.Run()
			if err != nil {
				log.WithError(err).Error("Error")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opt.Cloud, "cloud", cloudAWS, "Cloud provider: aws(default)|azure|gcp)")
	flags.StringVarP(&opt.Namespace, "namespace", "n", "", "Namespace to create cluster deployment in")
	flags.StringVar(&opt.SSHPrivateKeyFile, "ssh-private-key-file", "", "file name containing private key contents")
	flags.StringVar(&opt.SSHPublicKeyFile, "ssh-public-key-file", defaultSSHPublicKeyFile, "file name of SSH public key for cluster")
	flags.StringVar(&opt.SSHPublicKey, "ssh-public-key", "", "SSH public key for cluster")
	flags.StringVar(&opt.BaseDomain, "base-domain", "new-installer.openshift.com", "Base domain for the cluster")
	flags.StringVar(&opt.PullSecret, "pull-secret", "", "Pull secret for cluster. Takes precedence over pull-secret-file.")
	flags.StringVar(&opt.DeleteAfter, "delete-after", "", "Delete this cluster after the given duration. (i.e. 8h)")
	flags.StringVar(&opt.PullSecretFile, "pull-secret-file", defaultPullSecretFile, "Pull secret file for cluster")
	flags.StringVar(&opt.CredsFile, "creds-file", "", "Cloud credentials file (defaults vary depending on cloud)")
	flags.StringVar(&opt.ClusterImageSet, "image-set", "", "Cluster image set to use for this cluster deployment")
	flags.StringVar(&opt.InstallerImage, "installer-image", "", "Installer image to use for installing this cluster deployment")
	flags.StringVar(&opt.ReleaseImage, "release-image", "", "Release image to use for installing this cluster deployment")
	flags.StringVar(&opt.ReleaseImageSource, "release-image-source", "https://openshift-release.svc.ci.openshift.org/api/v1/releasestream/4-stable/latest", "URL to JSON describing the release image pull spec")
	flags.StringVar(&opt.ServingCert, "serving-cert", "", "Serving certificate for control plane and routes")
	flags.StringVar(&opt.ServingCertKey, "serving-cert-key", "", "Serving certificate key for control plane and routes")
	flags.BoolVar(&opt.ManageDNS, "manage-dns", false, "Manage this cluster's DNS. This is only available for AWS.")
	flags.BoolVar(&opt.UseClusterImageSet, "use-image-set", true, "If true(default), use a cluster image set for this cluster")
	flags.StringVarP(&opt.Output, "output", "o", "", "Output of this command (nothing will be created on cluster). Valid values: yaml,json")
	flags.BoolVar(&opt.IncludeSecrets, "include-secrets", true, "Include secrets along with ClusterDeployment")
	flags.BoolVar(&opt.InstallOnce, "install-once", false, "Run the install only one time and fail if not successful")
	flags.BoolVar(&opt.UninstallOnce, "uninstall-once", false, "Run the uninstall only one time and fail if not successful")
	flags.BoolVar(&opt.SimulateBootstrapFailure, "simulate-bootstrap-failure", false, "Simulate an install bootstrap failure by injecting an invalid manifest.")
	flags.Int64Var(&opt.WorkerNodes, "workers", 3, "Number of worker nodes to create.")

	// Azure flags
	flags.StringVar(&opt.AzureBaseDomainResourceGroupName, "azure-base-domain-resource-group-name", "os4-common", "Resource group where the azure DNS zone for the base domain is found")

	// GCP flags
	flags.StringVar(&opt.GCPProjectID, "gcp-project-id", "", "Project ID is the ID of the GCP project to use")

	return cmd
}

// Complete finishes parsing arguments for the command
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	o.Name = args[0]
	return nil
}

// Validate ensures that option values make sense
func (o *Options) Validate(cmd *cobra.Command) error {
	if len(o.Output) > 0 && o.Output != "yaml" && o.Output != "json" {
		cmd.Usage()
		log.Info("Invalid value for output. Valid values are: yaml, json.")
		return fmt.Errorf("invalid output")
	}
	if !o.UseClusterImageSet && len(o.ClusterImageSet) > 0 {
		cmd.Usage()
		log.Info("If not using cluster image sets, do not specify the name of one")
		return fmt.Errorf("invalid option")
	}
	if len(o.ServingCert) > 0 && len(o.ServingCertKey) == 0 {
		cmd.Usage()
		log.Info("If specifying a serving certificate, specify a valid serving certificate key")
		return fmt.Errorf("invalid serving cert")
	}
	if !validClouds[o.Cloud] {
		cmd.Usage()
		log.Infof("Unsupported cloud: %s", o.Cloud)
		return fmt.Errorf("Unsupported cloud: %s", o.Cloud)
	}
	switch o.Cloud {
	case cloudGCP:
		if o.GCPProjectID == "" {
			cmd.Usage()
			log.Infof("Must specify the GCP project ID when installing on GCP. Use the --gcp-project-id flag.")
			return fmt.Errorf("gcp requires gcp-project-id flag")

		}
	}
	return nil
}

type cloudProvider interface {
	generateCredentialsSecret(o *Options) (*corev1.Secret, error)

	addPlatformDetails(o *Options, cd *hivev1.ClusterDeployment) error
}

// Run executes the command
func (o *Options) Run() error {
	if err := apis.AddToScheme(scheme.Scheme); err != nil {
		return err
	}

	switch o.Cloud {
	case cloudAWS:
		o.cloudProvider = &awsCloudProvider{}
	case cloudAzure:
		o.cloudProvider = &azureCloudProvider{}
	case cloudGCP:
		o.cloudProvider = &gcpCloudProvider{}
	}

	objs, err := o.GenerateObjects()
	if err != nil {
		return err
	}
	if len(o.Output) > 0 {
		var printer printers.ResourcePrinter
		if o.Output == "yaml" {
			printer = &printers.YAMLPrinter{}
		} else {
			printer = &printers.JSONPrinter{}
		}
		printObjects(objs, scheme.Scheme, printer)
		return err
	}
	rh, err := o.getResourceHelper()
	if err != nil {
		return err
	}
	if len(o.Namespace) == 0 {
		o.Namespace, err = o.defaultNamespace()
		if err != nil {
			log.Error("Cannot determine default namespace")
			return err
		}
	}
	for _, obj := range objs {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			log.WithError(err).Errorf("Cannot create accessor for object of type %T", obj)
			return err
		}
		accessor.SetNamespace(o.Namespace)
		rh.ApplyRuntimeObject(obj, scheme.Scheme)
	}
	return nil
}

func (o *Options) defaultNamespace() (string, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	ns, _, err := kubeconfig.Namespace()
	return ns, err
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

// GenerateObjects generates resources for a new cluster deployment
func (o *Options) GenerateObjects() ([]runtime.Object, error) {
	result := []runtime.Object{}

	pullSecret, err := o.generatePullSecret()
	if err != nil {
		return nil, err
	}

	cd, err := o.GenerateClusterDeployment(pullSecret)
	if err != nil {
		return nil, err
	}

	if err := o.cloudProvider.addPlatformDetails(o, cd); err != nil {
		return nil, err
	}

	imageSet, err := o.configureImages(cd)
	if err != nil {
		return nil, err
	}
	if imageSet != nil {
		result = append(result, imageSet)
	}

	if o.IncludeSecrets {
		if pullSecret != nil {
			result = append(result, pullSecret)
		}

		creds, err := o.cloudProvider.generateCredentialsSecret(o)
		if err != nil {
			return nil, err
		}
		result = append(result, creds)

		sshSecret, err := o.generateSSHSecret()
		if err != nil {
			return nil, err
		}
		result = append(result, sshSecret)

		servingCertSecret, err := o.generateServingCertSecret()
		if err != nil {
			return nil, err
		}
		if servingCertSecret != nil {
			result = append(result, servingCertSecret)
		}
	}

	result = append(result, cd)

	return result, err
}

func (o *Options) getPullSecret() (string, error) {
	pullSecret := os.Getenv("PULL_SECRET")
	if len(pullSecret) > 0 {
		return pullSecret, nil
	}
	if len(o.PullSecret) > 0 {
		return o.PullSecret, nil
	}
	if len(o.PullSecretFile) > 0 {
		data, err := ioutil.ReadFile(o.PullSecretFile)
		if err != nil {
			log.Error("Cannot read pull secret file")
			return "", err
		}
		pullSecret = strings.TrimSpace(string(data))
		return pullSecret, nil
	}
	return "", nil
}

func (o *Options) generatePullSecret() (*corev1.Secret, error) {
	pullSecret, err := o.getPullSecret()
	if err != nil {
		return nil, err
	}
	if len(pullSecret) == 0 {
		return nil, nil
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-pull-secret", o.Name),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		StringData: map[string]string{
			corev1.DockerConfigJsonKey: pullSecret,
		},
	}, nil
}

func (o *Options) getSSHPublicKey() (string, error) {
	sshPublicKey := os.Getenv("PUBLIC_SSH_KEY")
	if len(sshPublicKey) > 0 {
		return sshPublicKey, nil
	}
	if len(o.SSHPublicKey) > 0 {
		return o.SSHPublicKey, nil
	}
	if len(o.SSHPublicKeyFile) > 0 {
		data, err := ioutil.ReadFile(o.SSHPublicKeyFile)
		if err != nil {
			log.Error("Cannot read SSH public key file")
			return "", err
		}
		sshPublicKey = strings.TrimSpace(string(data))
		return sshPublicKey, nil
	}

	log.Error("Cannot determine SSH key to use")
	return "", fmt.Errorf("no ssh key")
}

func (o *Options) getSSHPrivateKey() (string, error) {
	if len(o.SSHPrivateKeyFile) > 0 {
		data, err := ioutil.ReadFile(o.SSHPrivateKeyFile)
		if err != nil {
			log.Error("Cannot read SSH private key file")
			return "", err
		}
		sshPrivateKey := strings.TrimSpace(string(data))
		return sshPrivateKey, nil
	}
	log.Debug("No private SSH key file provided")
	return "", nil
}

func (o *Options) generateSSHSecret() (*corev1.Secret, error) {
	sshPublicKey, err := o.getSSHPublicKey()
	if err != nil {
		return nil, err
	}

	sshPrivateKey, err := o.getSSHPrivateKey()
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-ssh-key", o.Name),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"ssh-publickey":  sshPublicKey,
			"ssh-privatekey": sshPrivateKey,
		},
	}, nil
}

func (o *Options) generateServingCertSecret() (*corev1.Secret, error) {
	if len(o.ServingCert) == 0 {
		return nil, nil
	}
	servingCert, err := ioutil.ReadFile(o.ServingCert)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %v", o.ServingCert, err)
	}
	servingCertKey, err := ioutil.ReadFile(o.ServingCertKey)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %v", o.ServingCertKey, err)
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-serving-cert", o.Name),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeTLS,
		StringData: map[string]string{
			"tls.crt": string(servingCert),
			"tls.key": string(servingCertKey),
		},
	}, nil
}

// GenerateClusterDeployment generates a new cluster deployment
func (o *Options) GenerateClusterDeployment(pullSecret *corev1.Secret) (*hivev1.ClusterDeployment, error) {
	cd := &hivev1.ClusterDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterDeployment",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        o.Name,
			Namespace:   o.Namespace,
			Annotations: map[string]string{},
		},
		Spec: hivev1.ClusterDeploymentSpec{
			SSHKey: corev1.LocalObjectReference{
				Name: fmt.Sprintf("%s-ssh-key", o.Name),
			},
			Images: hivev1.ProvisionImages{
				InstallerImagePullPolicy: corev1.PullAlways,
			},
			ClusterName: o.Name,
			BaseDomain:  o.BaseDomain,
			// TODO: Generate networking from installer default
			Networking: hivev1.Networking{
				Type:        hivev1.NetworkTypeOpenshiftSDN,
				ServiceCIDR: "172.30.0.0/16",
				MachineCIDR: "10.0.0.0/16",
				ClusterNetworks: []netopv1.ClusterNetwork{
					{
						CIDR:             "10.128.0.0/14",
						HostSubnetLength: 23,
					},
				},
			},
			ControlPlane: hivev1.MachinePool{
				Name:     "master",
				Replicas: pointer.Int64Ptr(3),
			},
			Compute: []hivev1.MachinePool{
				{
					Name:     "worker",
					Replicas: pointer.Int64Ptr(o.WorkerNodes),
				},
			},
			ManageDNS: o.ManageDNS,
		},
	}

	if o.InstallOnce {
		cd.Annotations[tryInstallOnceAnnotation] = "true"
	}
	if o.UninstallOnce {
		cd.Annotations[tryUninstallOnceAnnotation] = "true"
	}
	if o.SimulateBootstrapFailure {
		cd.Annotations[constants.InstallFailureTestAnnotation] = "true"
	}
	if pullSecret != nil {
		cd.Spec.PullSecret = &corev1.LocalObjectReference{Name: pullSecret.Name}
	}
	if len(o.ServingCert) > 0 {
		cd.Spec.CertificateBundles = []hivev1.CertificateBundleSpec{
			{
				Name: "serving-cert",
				SecretRef: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-serving-cert", o.Name),
				},
			},
		}
		cd.Spec.ControlPlaneConfig.ServingCertificates.Default = "serving-cert"
		cd.Spec.Ingress = []hivev1.ClusterIngress{
			{
				Name:               "default",
				Domain:             fmt.Sprintf("apps.%s.%s", o.Name, o.BaseDomain),
				ServingCertificate: "serving-cert",
			},
		}
	}

	if o.DeleteAfter != "" {
		cd.ObjectMeta.Annotations[deleteAfterAnnotation] = o.DeleteAfter
	}

	return cd, nil
}

func (o *Options) configureImages(cd *hivev1.ClusterDeployment) (*hivev1.ClusterImageSet, error) {
	if len(o.ClusterImageSet) > 0 {
		cd.Spec.ImageSet = &hivev1.ClusterImageSetReference{
			Name: o.ClusterImageSet,
		}
		return nil, nil
	}
	if o.ReleaseImage == "" {
		if o.ReleaseImageSource == "" {
			return nil, fmt.Errorf("Specify either a release image or a release image source")
		}
		var err error
		o.ReleaseImage, err = determineReleaseImageFromSource(o.ReleaseImageSource)
		if err != nil {
			return nil, fmt.Errorf("Cannot determine release image: %v", err)
		}
	}
	if o.UseClusterImageSet {
		cd.Spec.Images.InstallerImage = o.InstallerImage
		cd.Spec.Images.ReleaseImage = o.ReleaseImage
		return nil, nil
	}

	imageSet := &hivev1.ClusterImageSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-imageset", o.Name),
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterImageSet",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		Spec: hivev1.ClusterImageSetSpec{
			ReleaseImage:   &o.ReleaseImage,
			InstallerImage: &o.InstallerImage,
		},
	}
	cd.Spec.ImageSet = &hivev1.ClusterImageSetReference{
		Name: imageSet.Name,
	}

	return imageSet, nil
}

func printObjects(objects []runtime.Object, scheme *runtime.Scheme, printer printers.ResourcePrinter) {
	typeSetterPrinter := printers.NewTypeSetter(scheme).ToPrinter(printer)
	switch len(objects) {
	case 0:
		return
	case 1:
		typeSetterPrinter.PrintObj(objects[0], os.Stdout)
	default:
		list := &metav1.List{
			TypeMeta: metav1.TypeMeta{
				Kind:       "List",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ListMeta: metav1.ListMeta{},
		}
		meta.SetList(list, objects)
		typeSetterPrinter.PrintObj(list, os.Stdout)
	}
}

type releasePayload struct {
	PullSpec string `json:"pullSpec"`
}

func determineReleaseImageFromSource(sourceURL string) (string, error) {
	resp, err := http.Get(sourceURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	payload := &releasePayload{}
	err = json.Unmarshal(data, payload)
	if err != nil {
		return "", err
	}
	return payload.PullSpec, nil
}
