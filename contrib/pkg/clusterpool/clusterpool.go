package clusterpool

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/util/homedir"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/utils"
	awsutils "github.com/openshift/hive/contrib/pkg/utils/aws"
	azureutils "github.com/openshift/hive/contrib/pkg/utils/azure"
	gcputils "github.com/openshift/hive/contrib/pkg/utils/gcp"
	"github.com/openshift/hive/pkg/clusterresource"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	cloudAWS   = "aws"
	cloudAzure = "azure"
	cloudGCP   = "gcp"

	longDesc = `
OVERVIEW
The hiveutil clusterpool create-pool command generates and applies the
artifacts needed to create a new Hive cluster pool.
Currently cluster pools are supported in AWS, Azure and GCP.

IMAGES
An existing ClusterImageSet can be specified with the --image-set
flag. Otherwise, one will be generated using the image specified for the
cluster pool using --release-image flag. If neither of these flags are 
specified, default image is obtained from the following URL:
https://amd64.ocp.releases.ci.openshift.org/api/v1/releasestream/4-stable/latest

ENVIRONMENT VARIABLES
The command will use the following environment variables for its output:

PULL_SECRET - If present, it is used as the cluster pool's pull
secret and will override the --pull-secret flag. If not present, and
the --pull-secret flag is not specified, then the --pull-secret-file is
used.
`
)

var (
	validClouds = map[string]bool{
		cloudAWS:   true,
		cloudAzure: true,
		cloudGCP:   true,
	}
)

type ClusterPoolOptions struct {
	Name               string
	Namespace          string
	BaseDomain         string
	PullSecret         string
	PullSecretFile     string
	Cloud              string
	CredsFile          string
	CloudSecretName    string
	ClusterImageSet    string
	ReleaseImage       string
	ReleaseImageSource string
	Region             string
	Size               int32
	HibernateAfter     string
	HibernateAfterDur  *time.Duration

	AzureBaseDomainResourceGroupName string

	createCloudSecret bool
	homeDir           string
	log               log.FieldLogger
	Output            string
}

func NewCreateClusterPoolCommand() *cobra.Command {
	opt := &ClusterPoolOptions{log: log.WithField("command", "clusterpool create-pool")}

	opt.homeDir = homedir.HomeDir()

	defaultPullSecretFile := ""
	pullSecretFileDesc := ""
	if _, err := os.Stat(filepath.Join(opt.homeDir, ".pull-secret")); err == nil {
		defaultPullSecretFile = filepath.Join(opt.homeDir, ".pull-secret")
		pullSecretFileDesc = fmt.Sprintf("Default value is %s", defaultPullSecretFile)
	}

	cmd := &cobra.Command{
		Use: `create-pool CLUSTER_POOL_NAME
create-pool CLUSTER_POOL_NAME --cloud=aws
create-pool CLUSTER_POOL_NAME --cloud=azure --azure-base-domain-resource-group-name=RESOURCE_GROUP_NAME
create-pool CLUSTER_POOL_NAME --cloud=gcp`,
		Short: "Creates a new Hive cluster pool",
		Long:  longDesc,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			if err := opt.complete(args); err != nil {
				opt.log.WithError(err).Fatal("Error")
			}
			if err := opt.validate(cmd); err != nil {
				opt.log.WithError(err).Fatal("Error")
			}
			err := opt.run()
			if err != nil {
				opt.log.WithError(err).Fatal("Error")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opt.Cloud, "cloud", cloudAWS, "Cloud provider: aws(default)|azure|gcp)")
	flags.StringVarP(&opt.Namespace, "namespace", "n", "", "Namespace to create cluster pool in")
	flags.StringVar(&opt.BaseDomain, "base-domain", "new-installer.openshift.com", "Base domain for the cluster pool")
	flags.StringVar(&opt.PullSecret, "pull-secret", "", "Pull secret for cluster pool. Takes precedence over pull-secret-file.")
	flags.StringVar(&opt.PullSecretFile, "pull-secret-file", defaultPullSecretFile, fmt.Sprintf("Pull secret file for cluster pool. %s", pullSecretFileDesc))
	flags.StringVar(&opt.CredsFile, "creds-file", "", "Cloud credentials file (defaults vary depending on cloud)")
	flags.StringVar(&opt.CloudSecretName, "cloud-secret", "", "Cloud secret name if already created")
	flags.StringVar(&opt.ClusterImageSet, "image-set", "", "Cluster image set to use for this cluster pool")
	flags.StringVar(&opt.ReleaseImage, "release-image", "", "Release image to use for installing the clusters in this pool")
	flags.StringVar(&opt.ReleaseImageSource, "release-image-source", "https://amd64.ocp.releases.ci.openshift.org/api/v1/releasestream/4-stable/latest", "URL to JSON describing the release image pull spec")
	flags.StringVar(&opt.Region, "region", "", "Region to which to install the cluster pool.")
	flags.Int32Var(&opt.Size, "size", 1, "Size of cluster pool")
	flags.StringVar(&opt.AzureBaseDomainResourceGroupName, "azure-base-domain-resource-group-name", "os4-common", "Resource group where the azure DNS zone for the base domain is found")
	flags.StringVar(&opt.HibernateAfter, "hibernate-after", "", "Automatically hibernate clusterpool clusters when they have been running for the given duration")
	flags.StringVarP(&opt.Output, "output", "o", "", "Output of this command (nothing will be created on cluster). Valid values: yaml,json")

	return cmd
}

// complete finishes parsing arguments for the command
func (o *ClusterPoolOptions) complete(args []string) error {
	o.Name = args[0]

	if o.Region == "" {
		switch o.Cloud {
		case cloudAWS:
			o.Region = "us-east-1"
		case cloudAzure:
			o.Region = "centralus"
		case cloudGCP:
			o.Region = "us-east1"
		}
	}

	if len(o.CloudSecretName) == 0 {
		o.createCloudSecret = true
	}

	if o.HibernateAfter != "" {
		dur, err := time.ParseDuration(o.HibernateAfter)
		if err != nil {
			return errors.Wrapf(err, "unable to parse HibernateAfter duration")
		}
		o.HibernateAfterDur = &dur
	}

	return nil
}

// validate ensures that option values make sense
func (o *ClusterPoolOptions) validate(cmd *cobra.Command) error {
	if len(o.Output) > 0 && o.Output != "yaml" && o.Output != "json" {
		cmd.Usage()
		o.log.Info("Invalid value for output. Valid values are: yaml, json.")
		return fmt.Errorf("invalid output")
	}

	if len(o.BaseDomain) == 0 {
		return fmt.Errorf("base domain is required")
	}

	releaseImageFlagsUsed := 0
	switch {
	case o.ClusterImageSet != "":
		releaseImageFlagsUsed++
	case o.ReleaseImage != "":
		releaseImageFlagsUsed++
	case o.ReleaseImageSource != "":
		releaseImageFlagsUsed++
	}
	if releaseImageFlagsUsed == 0 {
		cmd.Usage()
		return fmt.Errorf("must specify one of image set, release image or release image source")
	}
	if releaseImageFlagsUsed > 1 {
		cmd.Usage()
		return fmt.Errorf("must specify only one of image set, release image or release image source")
	}

	if !validClouds[o.Cloud] {
		cmd.Usage()
		return fmt.Errorf("unsupported cloud: %s", o.Cloud)
	}

	return nil
}

// run executes the command
func (o *ClusterPoolOptions) run() error {
	scheme := scheme.GetScheme()

	objs, err := o.generateObjects()
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
		printObjects(objs, scheme, printer)
		return err
	}

	rh, err := utils.GetResourceHelper("util-clusterpool", o.log)
	if err != nil {
		return err
	}
	if len(o.Namespace) == 0 {
		o.Namespace, err = utils.DefaultNamespace()
		if err != nil {
			return errors.Wrapf(err, "cannot determine default namespace")
		}
	}
	for _, obj := range objs {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			return errors.Wrapf(err, "cannot create accessor for object of type %T", obj)
		}
		accessor.SetNamespace(o.Namespace)
		if _, err := rh.ApplyRuntimeObject(obj, scheme); err != nil {
			return err
		}

	}
	return nil
}

func (o *ClusterPoolOptions) generateObjects() ([]runtime.Object, error) {
	pullSecret, err := utils.GetPullSecret(o.log, o.PullSecret, o.PullSecretFile)
	if err != nil {
		return nil, err
	}

	builder := &clusterresource.Builder{
		Name:       o.Name,
		Namespace:  o.Namespace,
		PullSecret: pullSecret,
		BaseDomain: o.BaseDomain,
	}

	var awsBuilder *clusterresource.AWSCloudBuilder
	if o.Cloud == cloudAWS {
		awsBuilder = &clusterresource.AWSCloudBuilder{
			Region: o.Region,
			// TODO: CLI option for this
			InstanceType: clusterresource.AWSInstanceTypeDefault,
		}
		builder.CloudBuilder = awsBuilder
	}

	if o.createCloudSecret {
		switch o.Cloud {
		case cloudAWS:
			defaultCredsFilePath := filepath.Join(o.homeDir, ".aws", "credentials")
			accessKeyID, secretAccessKey, err := awsutils.GetAWSCreds(o.CredsFile, defaultCredsFilePath)
			if err != nil {
				o.log.WithError(err).Error("Failed to get AWS credentials")
				return nil, err
			}
			// Update AWS cloud builder with creds
			awsBuilder.AccessKeyID = accessKeyID
			awsBuilder.SecretAccessKey = secretAccessKey
		case cloudAzure:
			creds, err := azureutils.GetCreds(o.CredsFile)
			if err != nil {
				o.log.WithError(err).Error("Failed to read in Azure credentials")
				return nil, err
			}
			builder.CloudBuilder = &clusterresource.AzureCloudBuilder{
				ServicePrincipal:            creds,
				BaseDomainResourceGroupName: o.AzureBaseDomainResourceGroupName,
				Region:                      o.Region,
			}
		case cloudGCP:
			creds, err := gcputils.GetCreds(o.CredsFile)
			if err != nil {
				o.log.WithError(err).Error("Failed to get GCP credentials")
				return nil, err
			}
			builder.CloudBuilder = &clusterresource.GCPCloudBuilder{
				ServiceAccount: creds,
				Region:         o.Region,
			}
		}
	}

	imageSet, err := o.configureImages()
	if err != nil {
		return nil, err
	}

	result, err := o.build(builder)
	if err != nil {
		return nil, err
	}

	if imageSet != nil {
		result = append(result, imageSet)
	}
	if len(pullSecret) != 0 {
		result = append(result, builder.GeneratePullSecretSecret())
	}

	return result, nil
}

func (o *ClusterPoolOptions) configureImages() (*hivev1.ClusterImageSet, error) {
	if len(o.ClusterImageSet) > 0 {
		return nil, nil
	}

	if o.ReleaseImage == "" {
		var err error
		o.ReleaseImage, err = utils.DetermineReleaseImageFromSource(o.ReleaseImageSource)
		if err != nil {
			return nil, fmt.Errorf("cannot determine release image: %v", err)
		}
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
			ReleaseImage: o.ReleaseImage,
		},
	}
	o.ClusterImageSet = imageSet.Name
	return imageSet, nil
}

func (o *ClusterPoolOptions) build(builder *clusterresource.Builder) ([]runtime.Object, error) {

	var allObjects []runtime.Object

	allObjects = append(allObjects, o.generateClusterPool(builder))

	if o.createCloudSecret {
		cloudCredsSecret := builder.CloudBuilder.GenerateCredentialsSecret(builder)
		if cloudCredsSecret != nil {
			allObjects = append(allObjects, cloudCredsSecret)
		}
	}

	return allObjects, nil
}

func (o *ClusterPoolOptions) generateClusterPool(builder *clusterresource.Builder) *hivev1.ClusterPool {
	cp := &hivev1.ClusterPool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterPool",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.Name,
			Namespace: o.Namespace,
		},
		Spec: hivev1.ClusterPoolSpec{
			BaseDomain: o.BaseDomain,
			Size:       o.Size,
		},
	}
	if o.PullSecret != "" || o.PullSecretFile != "" {
		cp.Spec.PullSecretRef = &corev1.LocalObjectReference{Name: builder.GetPullSecretSecretName()}
	}
	if o.ClusterImageSet != "" {
		cp.Spec.ImageSetRef = hivev1.ClusterImageSetReference{Name: o.ClusterImageSet}
	}
	if o.HibernateAfterDur != nil {
		cp.Spec.HibernateAfter = &metav1.Duration{Duration: *o.HibernateAfterDur}
	}
	cp.Spec.Platform = builder.CloudBuilder.GetCloudPlatform(builder)

	return cp
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
