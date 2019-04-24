/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package createcluster

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/ini.v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions/printers"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	netopv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/resource"
)

const longDesc = `
OVERVIEW
The hive-util create-cluster command generates and applies the artifacts needed 
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
It overrides the public ssh key flags. If not, --ssh-key will be used. 
If that is not specified, then --ssh-key-file is used. 
That file's default value is %[1]s.

PULL_SECRET - If present, it is used as the cluster deployment's pull 
secret and will override the --pull-secret flag. If not present, and
the --pull-secret flag is not specified, then the --pull-secret-file is 
used. That file's default value is %[2]s.

AWS_SECRET_ACCESS_KEY and AWS_ACCESS_KEY_ID - Are used to determine your
AWS credentials. If not present, then the --aws-creds-file is used. By
default, that flag's value is %[3]s.

HIVE_IMAGE - Hive image to use for installing/uninstalling the cluster.
If not specified, the --hive-image flag is used. If that's not specified, 
a default image is used: %[4]s.

RELEASE_IMAGE - Release image to use to install the cluster. If not specified,
the --release-image flag is used. If that's not specified, a default image is
used: %[5]s.

INSTALLER_IMAGE - Installer image to use to install the cluster. If not specified,
the --installer-image flag is used. If that's not specified, the image is 
derived from the release image at runtime.
`

// Options is the set of options to generate and apply a new cluster deployment
type Options struct {
	Name               string
	Namespace          string
	SSHKeyFile         string
	SSHKey             string
	BaseDomain         string
	PullSecret         string
	PullSecretFile     string
	AWSCredsFile       string
	ClusterImageSet    string
	HiveImage          string
	InstallerImage     string
	ReleaseImage       string
	UseClusterImageSet bool
	ManageDNS          bool
	Output             string
	IncludeSecrets     bool
}

const (
	defaultHiveImage    = "registry.svc.ci.openshift.org/openshift/hive-v4.0:hive"
	defaultReleaseImage = "registry.svc.ci.openshift.org/openshift/origin-release:v4.0"
)

// NewCreateClusterCommand creates a command that generates and applies cluster deployment artifacts.
func NewCreateClusterCommand() *cobra.Command {
	var homeDir = "."
	if u, err := user.Current(); err == nil {
		homeDir = u.HomeDir
	}
	defaultSSHKeyFile := filepath.Join(homeDir, ".ssh", "libra.pub")
	defaultPullSecretFile := filepath.Join(homeDir, "pull-secret.txt")
	defaultAWSCredsFile := filepath.Join(homeDir, ".aws", "credentials")

	opt := &Options{}
	cmd := &cobra.Command{
		Use:   "create-cluster CLUSTER_DEPLOYMENT_NAME",
		Short: "Creates a new Hive cluster deployment",
		Long:  fmt.Sprintf(longDesc, defaultSSHKeyFile, defaultPullSecretFile, defaultAWSCredsFile, defaultHiveImage, defaultReleaseImage),
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			if err := opt.Complete(cmd, args); err != nil {
				return
			}

			err := opt.Run()
			if err != nil {
				log.WithError(err).Error("Error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&opt.Namespace, "namespace", "p", "", "Namespace to create cluster deployment in")
	flags.StringVar(&opt.SSHKeyFile, "ssh-key-file", defaultSSHKeyFile, "file name of SSH public key for cluster")
	flags.StringVar(&opt.SSHKey, "ssh-key", "", "SSH public key for cluster")
	flags.StringVar(&opt.BaseDomain, "base-domain", "new-installer.openshift.com", "Base domain for the cluster")
	flags.StringVar(&opt.PullSecret, "pull-secret", "", "Pull secret for cluster. Takes precedence over pull-secret-file.")
	flags.StringVar(&opt.PullSecretFile, "pull-secret-file", defaultPullSecretFile, "Pull secret file for cluster")
	flags.StringVar(&opt.AWSCredsFile, "aws-creds-file", defaultAWSCredsFile, "AWS credentials file")
	flags.StringVar(&opt.ClusterImageSet, "image-set", "", "Cluster image set to use for this cluster deployment")
	flags.StringVar(&opt.HiveImage, "hive-image", "", "Hive image to use for installing/uninstalling this cluster deployment")
	flags.StringVar(&opt.InstallerImage, "installer-image", "", "Installer image to use for installing this cluster deployment")
	flags.StringVar(&opt.ReleaseImage, "release-image", "", "Release image to use for installing this cluster deployment")
	flags.BoolVar(&opt.ManageDNS, "manage-dns", false, "Manage this cluster's DNS")
	flags.BoolVar(&opt.UseClusterImageSet, "use-image-set", true, "If true(default), use a cluster image set for this cluster")
	flags.StringVarP(&opt.Output, "output", "o", "", "Output of this command (nothing will be created on cluster). Valid values: yaml,json")
	flags.BoolVar(&opt.IncludeSecrets, "include-secrets", true, "Include secrets along with ClusterDeployment")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		cmd.Usage()
		log.Info("You must specify a cluster deployment name")
		return fmt.Errorf("no cluster deployment specified")
	}
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
	}
	return nil
}

// Run executes the command
func (o *Options) Run() error {
	if err := apis.AddToScheme(scheme.Scheme); err != nil {
		return err
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
	resource, err := o.getResourceHelper()
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
			log.Errorf("Cannot create accessor for object of type %T", obj)
			return err
		}
		accessor.SetNamespace(o.Namespace)
		resource.ApplyRuntimeObject(obj, scheme.Scheme)
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
		log.Error("Cannot get client config")
		return nil, err
	}
	helper := resource.NewHelperFromRESTConfig(cfg, log.WithField("command", "create-cluster"))
	return helper, nil
}

// GenerateObjects generates resources for a new cluster deployment
func (o *Options) GenerateObjects() ([]runtime.Object, error) {
	result := []runtime.Object{}
	cd, imageSet, err := o.GenerateClusterDeployment()
	if err != nil {
		return nil, err
	}
	if imageSet != nil {
		result = append(result, imageSet)
	}
	result = append(result, cd)
	if !o.IncludeSecrets {
		return result, nil
	}
	awsCreds, err := o.generateAWSCredsSecret()
	if err != nil {
		return nil, err
	}
	pullSecret, err := o.generatePullSecret()
	if err != nil {
		return nil, err
	}
	sshSecret, err := o.generateSSHSecret()
	if err != nil {
		return nil, err
	}
	return append([]runtime.Object{awsCreds, pullSecret, sshSecret}, result...), err
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
	log.Error("Cannot determine pull secret to use")
	return "", fmt.Errorf("no pull secret")
}

func (o *Options) generatePullSecret() (*corev1.Secret, error) {
	pullSecret, err := o.getPullSecret()
	if err != nil {
		return nil, err
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

func (o *Options) getSSHKey() (string, error) {
	sshKey := os.Getenv("SSH_KEY")
	if len(sshKey) > 0 {
		return sshKey, nil
	}
	if len(o.SSHKey) > 0 {
		return o.SSHKey, nil
	}
	if len(o.SSHKeyFile) > 0 {
		data, err := ioutil.ReadFile(o.SSHKeyFile)
		if err != nil {
			log.Error("Cannot read SSH public key file")
			return "", err
		}
		sshKey = strings.TrimSpace(string(data))
		return sshKey, nil
	}
	log.Error("Cannot determine SSH key to use")
	return "", fmt.Errorf("no ssh key")
}

func (o *Options) generateSSHSecret() (*corev1.Secret, error) {
	sshKey, err := o.getSSHKey()
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
			"ssh-public-key": sshKey,
		},
	}, nil
}

func (o *Options) getAWSCreds() (string, string, error) {
	accessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	keyID := os.Getenv("AWS_ACCESS_KEY_ID")
	if len(accessKey) > 0 && len(keyID) > 0 {
		return accessKey, keyID, nil
	}
	if len(o.AWSCredsFile) > 0 {
		credFile, err := ini.Load(o.AWSCredsFile)
		if err != nil {
			log.Error("Cannot load AWS credentials")
			return "", "", err
		}
		defaultSection, err := credFile.GetSection("default")
		if err != nil {
			log.Error("Cannot get default section from AWS credentials file")
			return "", "", err
		}
		kAccessKey := defaultSection.Key("aws_access_key_id")
		kSecretKey := defaultSection.Key("aws_secret_access_key")
		if kAccessKey == nil || kSecretKey == nil {
			log.Error("AWS credentials file missing keys in default section")
		}
		accessKey, keyID = kAccessKey.String(), kSecretKey.String()
		return accessKey, keyID, nil
	}
	log.Error("Could not find AWS credentials")
	return "", "", fmt.Errorf("No AWS credentials")
}

func (o *Options) generateAWSCredsSecret() (*corev1.Secret, error) {
	accessKey, keyID, err := o.getAWSCreds()
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-aws-creds", o.Name),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"aws_access_key_id":     keyID,
			"aws_secret_access_key": accessKey,
		},
	}, nil
}

// GenerateClusterDeployment generates a new cluster deployment and optionally a corresponding cluster image set
func (o *Options) GenerateClusterDeployment() (*hivev1.ClusterDeployment, *hivev1.ClusterImageSet, error) {
	cd := &hivev1.ClusterDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterDeployment",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.Name,
			Namespace: o.Namespace,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			SSHKey: &corev1.LocalObjectReference{
				Name: fmt.Sprintf("%s-ssh-key", o.Name),
			},
			Images: hivev1.ProvisionImages{
				InstallerImagePullPolicy: corev1.PullAlways,
				HiveImagePullPolicy:      corev1.PullAlways,
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
			Platform: hivev1.Platform{
				AWS: &hivev1.AWSPlatform{
					Region: "us-east-1",
				},
			},
			PlatformSecrets: hivev1.PlatformSecrets{
				AWS: &hivev1.AWSPlatformSecrets{
					Credentials: corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-aws-creds", o.Name),
					},
				},
			},
			PullSecret: corev1.LocalObjectReference{
				Name: fmt.Sprintf("%s-pull-secret", o.Name),
			},
			ControlPlane: hivev1.MachinePool{
				Name:     "master",
				Replicas: int64ptr(3),
				Platform: hivev1.MachinePoolPlatform{
					AWS: &hivev1.AWSMachinePoolPlatform{
						InstanceType: "m4.large",
						EC2RootVolume: hivev1.EC2RootVolume{
							IOPS: 100,
							Size: 22,
							Type: "gp2",
						},
					},
				},
			},
			Compute: []hivev1.MachinePool{
				{
					Name:     "worker",
					Replicas: int64ptr(3),
					Platform: hivev1.MachinePoolPlatform{
						AWS: &hivev1.AWSMachinePoolPlatform{
							InstanceType: "m4.large",
							EC2RootVolume: hivev1.EC2RootVolume{
								IOPS: 100,
								Size: 22,
								Type: "gp2",
							},
						},
					},
				},
			},
			ManageDNS: o.ManageDNS,
		},
	}

	imageSet, err := o.configureImages(cd)
	if err != nil {
		return nil, nil, err
	}

	return cd, imageSet, nil
}

func (o *Options) configureImages(cd *hivev1.ClusterDeployment) (*hivev1.ClusterImageSet, error) {
	if len(o.ClusterImageSet) > 0 {
		cd.Spec.ImageSet = &hivev1.ClusterImageSetReference{
			Name: o.ClusterImageSet,
		}
		return nil, nil
	}
	if o.HiveImage == "" {
		o.HiveImage = defaultHiveImage
	}
	if o.ReleaseImage == "" {
		o.ReleaseImage = defaultReleaseImage
	}
	if !o.UseClusterImageSet {
		cd.Spec.Images.InstallerImage = o.InstallerImage
		cd.Spec.Images.ReleaseImage = o.ReleaseImage
		cd.Spec.Images.HiveImage = o.HiveImage
	}

	name := fmt.Sprintf("%s-imageset", o.Name)
	imageSet := &hivev1.ClusterImageSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterImageSet",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		Spec: hivev1.ClusterImageSetSpec{
			HiveImage:      strptr(o.HiveImage),
			ReleaseImage:   strptr(o.ReleaseImage),
			InstallerImage: strptr(o.InstallerImage),
		},
	}
	cd.Spec.ImageSet = &hivev1.ClusterImageSetReference{
		Name: name,
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

func int64ptr(n int64) *int64 {
	return &n
}

func strptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
