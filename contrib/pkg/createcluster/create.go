package createcluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/printers"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/clusterresource"
	"github.com/openshift/hive/pkg/constants"
	awscreds "github.com/openshift/hive/pkg/creds/aws"
	azurecreds "github.com/openshift/hive/pkg/creds/azure"
	gcpcreds "github.com/openshift/hive/pkg/creds/gcp"
	openstackcreds "github.com/openshift/hive/pkg/creds/openstack"
	"github.com/openshift/hive/pkg/gcpclient"
	"github.com/openshift/hive/pkg/util/scheme"
	installertypes "github.com/openshift/installer/pkg/types"
	installervsphere "github.com/openshift/installer/pkg/types/vsphere"
	"github.com/openshift/installer/pkg/validate"
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

GOVC_USERNAME and GOVC_PASSWORD - Are used to determine your vSphere
credentials.
GOVC_TLS_CA_CERTS - Is used to provide CA certificates for communicating
with the vSphere API.
GOVC_NETWORK, GOVC_DATACENTER, GOVC_DATASTORE and GOVC_HOST (vCenter host)
can be used as alternatives to the associated commandline argument.
These are only relevant for creating a cluster on vSphere.

IC_API_KEY - Used to determine your IBM Cloud API key. Required when
using --cloud=ibmcloud.

RELEASE_IMAGE - Release image to use to install the cluster. If not specified,
the --release-image flag is used. If that's not specified, a default image is
obtained from a the following URL:
https://amd64.ocp.releases.ci.openshift.org/api/v1/releasestream/4-stable/latest
`
const (
	hiveutilCreatedLabel = "hive.openshift.io/hiveutil-created"

	testFailureManifest = `apiVersion: v1
kind: NotARealSecret
metadata:
  name: foo
  namespace: bar
type: TestFailResource
`
)

var (
	validClouds = sets.New(
		constants.PlatformAWS,
		constants.PlatformAzure,
		constants.PlatformGCP,
		constants.PlatformIBMCloud,
		constants.PlatformOpenStack,
		constants.PlatformVSphere,
		constants.PlatformNutanix,
	)
	manualCCOModeClouds = sets.New(
		constants.PlatformIBMCloud,
	)
)

// Options is the set of options to generate and apply a new cluster deployment
type Options struct {
	Name                              string
	Namespace                         string
	SSHPublicKeyFile                  string
	SSHPublicKey                      string
	SSHPrivateKeyFile                 string
	BaseDomain                        string
	PullSecret                        string
	PullSecretFile                    string
	BoundServiceAccountSigningKeyFile string
	Cloud                             string
	CredsFile                         string
	CredentialsModeManual             bool
	ClusterImageSet                   string
	ReleaseImage                      string
	ReleaseImageSource                string
	DeleteAfter                       string
	HibernateAfter                    string
	HibernateAfterDur                 *time.Duration
	ServingCert                       string
	ServingCertKey                    string
	UseClusterImageSet                bool
	ManageDNS                         bool
	Output                            string
	IncludeSecrets                    bool
	InstallOnce                       bool
	UninstallOnce                     bool
	SimulateBootstrapFailure          bool
	WorkerNodesCount                  int64
	CreateSampleSyncsets              bool
	ManifestsDir                      string
	Adopt                             bool
	AdoptAdminKubeConfig              string
	AdoptInfraID                      string
	AdoptClusterID                    string
	AdoptMetadataJSON                 string
	AdoptAdminUsername                string
	AdoptAdminPassword                string
	MachineNetwork                    string
	Region                            string
	Labels                            []string
	Annotations                       []string
	SkipMachinePools                  bool
	AdditionalTrustBundle             string
	Internal                          bool
	FeatureSet                        string
	PrivateLink                       bool

	// AWS
	AWSUserTags           []string
	AWSPrivateLink        bool
	AWSInstanceType       string
	AWSWorkerInstanceType string

	// Azure
	AzureBaseDomainResourceGroupName string
	AzureCloudName                   string
	AzureResourceGroupName           string

	// GCP
	// This field is used by the cobra flag...
	DiscardLocalSsdOnHibernate bool
	// ...but we need a way to differentiate among `true` (discard), `false` (preserve), and `nil`
	// (user did not specify, so we won't send the option through the GCP API at all). Cobra gives
	// us a way to ask, after the fact, whether the field was "Changed"; we'll use that in the code
	// to set this (lowercase) option so we can trigger the correct behavior when talking to GCP.
	discardLocalSsdOnHibernate *bool

	// OpenStack
	OpenStackCloud             string
	OpenStackExternalNetwork   string
	OpenStackMasterFlavor      string
	OpenStackComputeFlavor     string
	OpenStackAPIFloatingIP     string
	OpenStackIngressFloatingIP string

	// VSphere
	VSphereVCenter          string
	VSphereDatacenter       string
	VSphereDefaultDataStore string
	VSphereFolder           string
	VSphereCluster          string
	VSphereAPIVIP           string
	VSphereIngressVIP       string
	VSphereNetwork          string
	VSpherePlatformSpecJSON string
	VSphereCACerts          string

	// IBM
	IBMCISInstanceCRN string
	IBMAccountID      string
	IBMInstanceType   string

	// Nutanix
	NutanixCACerts              string
	NutanixPrismCentralEndpoint string
	NutanixPrismCentralPort     int32
	NutanixPrismElementAddress  string
	NutanixPrismElementName     string
	NutanixPrismElementPort     int32
	NutanixPrismElementUUID     string
	NutanixAPIVIP               string
	NutanixIngressVIP           string
	NutanixSubnetUUIDs          []string
	NutanixAzName               string

	homeDir string
	log     log.FieldLogger
}

// NewCreateClusterCommand creates a command that generates and applies cluster deployment artifacts.
func NewCreateClusterCommand() *cobra.Command {
	opt := &Options{log: log.WithField("command", "create-cluster")}

	opt.homeDir = "."

	if u, err := user.Current(); err == nil {
		opt.homeDir = u.HomeDir
	}
	defaultSSHPublicKeyFile := filepath.Join(opt.homeDir, ".ssh", "id_rsa.pub")

	defaultPullSecretFile := filepath.Join(opt.homeDir, ".pull-secret")
	if _, err := os.Stat(defaultPullSecretFile); os.IsNotExist(err) {
		defaultPullSecretFile = ""
	} else if err != nil {
		opt.log.WithError(err).Errorf("%v can not be used", defaultPullSecretFile)
	}

	cmd := &cobra.Command{
		Use: `create-cluster CLUSTER_DEPLOYMENT_NAME
create-cluster CLUSTER_DEPLOYMENT_NAME --cloud=aws
create-cluster CLUSTER_DEPLOYMENT_NAME --cloud=azure --azure-base-domain-resource-group-name=RESOURCE_GROUP_NAME
create-cluster CLUSTER_DEPLOYMENT_NAME --cloud=gcp
create-cluster CLUSTER_DEPLOYMENT_NAME --cloud=ibmcloud --region="us-east" --base-domain=ibm.hive.openshift.com --manifests=/manifests --credentials-mode-manual
create-cluster CLUSTER_DEPLOYMENT_NAME --cloud=openstack --openstack-api-floating-ip=192.168.1.2 --openstack-cloud=mycloud
create-cluster CLUSTER_DEPLOYMENT_NAME --cloud=vsphere --vsphere-vcenter=vmware.devcluster.com --vsphere-datacenter=dc1 --vsphere-default-datastore=nvme-ds1 --vsphere-api-vip=192.168.1.2 --vsphere-ingress-vip=192.168.1.3 --vsphere-cluster=devel --vsphere-network="VM Network" --vsphere-ca-certs=/path/to/cert`,
		Short: "Creates a new Hive cluster deployment",
		Long:  fmt.Sprintf(longDesc, defaultSSHPublicKeyFile, defaultPullSecretFile),
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			if err := opt.Complete(cmd, args); err != nil {
				opt.log.WithError(err).Fatal("Error")
			}
			if err := opt.Validate(cmd); err != nil {
				opt.log.WithError(err).Fatal("Error")
			}
			err := opt.Run()
			if err != nil {
				opt.log.WithError(err).Fatal("Error")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opt.Cloud, "cloud", constants.PlatformAWS, fmt.Sprintf("Cloud provider: %s", strings.Join(sets.List(validClouds), "|")))
	flags.StringVarP(&opt.Namespace, "namespace", "n", "", "Namespace to create cluster deployment in")
	flags.StringVar(&opt.SSHPrivateKeyFile, "ssh-private-key-file", "", "file name containing private key contents")
	flags.StringVar(&opt.SSHPublicKeyFile, "ssh-public-key-file", defaultSSHPublicKeyFile, "file name of SSH public key for cluster")
	flags.StringVar(&opt.SSHPublicKey, "ssh-public-key", "", "SSH public key for cluster")
	flags.StringVar(&opt.BaseDomain, "base-domain", "new-installer.openshift.com", "Base domain for the cluster")
	flags.StringVar(&opt.PullSecret, "pull-secret", "", "Pull secret for cluster. Takes precedence over pull-secret-file.")
	flags.StringVar(&opt.DeleteAfter, "delete-after", "", "Delete this cluster after the given duration. (e.g. 8h)")
	flags.StringVar(&opt.HibernateAfter, "hibernate-after", "", "Automatically hibernate the cluster whenever it has been running for the given duration")
	flags.StringVar(&opt.PullSecretFile, "pull-secret-file", defaultPullSecretFile, "Pull secret file for cluster")
	flags.StringVar(&opt.BoundServiceAccountSigningKeyFile, "bound-service-account-signing-key-file", "", "Private service account signing key (often created with ccoutil create key-pair)")
	flags.BoolVar(&opt.CredentialsModeManual, "credentials-mode-manual", false, fmt.Sprintf(`Configure the Cloud Credential Operator in the target cluster to Manual mode.
Implies the use of --manifests to inject custom Secrets for all CredentialsRequests in the cluster.
This option is redundant (but permitted) for following clouds, which always use manual mode: %s`, strings.Join(sets.List(manualCCOModeClouds), "|")))
	flags.StringVar(&opt.CredsFile, "creds-file", "", "Cloud credentials file (defaults vary depending on cloud)")
	flags.StringVar(&opt.ClusterImageSet, "image-set", "", "Cluster image set to use for this cluster deployment")
	flags.StringVar(&opt.ReleaseImage, "release-image", "", "Release image to use for installing this cluster deployment")
	flags.StringVar(&opt.ReleaseImageSource, "release-image-source", "https://amd64.ocp.releases.ci.openshift.org/api/v1/releasestream/4-stable/latest", "URL to JSON describing the release image pull spec")
	flags.StringVar(&opt.ServingCert, "serving-cert", "", "Serving certificate for control plane and routes")
	flags.StringVar(&opt.ServingCertKey, "serving-cert-key", "", "Serving certificate key for control plane and routes")
	flags.BoolVar(&opt.ManageDNS, "manage-dns", false, "Manage this cluster's DNS. This is only available for AWS and GCP.")
	flags.BoolVar(&opt.UseClusterImageSet, "use-image-set", true, "If true, use a cluster image set for this cluster")
	flags.StringVarP(&opt.Output, "output", "o", "", "Output of this command (nothing will be created on cluster). Valid values: yaml,json")
	flags.BoolVar(&opt.IncludeSecrets, "include-secrets", true, "Include secrets along with ClusterDeployment")
	flags.BoolVar(&opt.InstallOnce, "install-once", false, "Run the install only one time and fail if not successful")
	flags.BoolVar(&opt.UninstallOnce, "uninstall-once", false, "Run the uninstall only one time and fail if not successful")
	flags.BoolVar(&opt.SimulateBootstrapFailure, "simulate-bootstrap-failure", false, "Simulate an install bootstrap failure by injecting an invalid manifest.")
	flags.Int64Var(&opt.WorkerNodesCount, "workers", 3, "Number of worker nodes to create.")
	flags.BoolVar(&opt.CreateSampleSyncsets, "create-sample-syncsets", false, "Create a set of sample syncsets for testing")
	flags.StringVar(&opt.ManifestsDir, "manifests", "", "Directory containing manifests to add during installation")
	flags.StringVar(&opt.MachineNetwork, "machine-network", "10.0.0.0/16", "Cluster's MachineNetwork to pass to the installer")
	flags.StringVar(&opt.Region, "region", "", "Region to which to install the cluster. This is only relevant to AWS, Azure, GCP and IBM.")
	flags.StringSliceVarP(&opt.Labels, "labels", "l", nil, "Label to apply to the ClusterDeployment (key=val). Multiple labels may be delimited by commas (key1=val1,key2=val2).")
	flags.StringSliceVarP(&opt.Annotations, "annotations", "a", nil, "Annotation to apply to the ClusterDeployment (key=val)")
	flags.BoolVar(&opt.SkipMachinePools, "skip-machine-pools", false, "Skip generation of Hive MachinePools for day 2 MachineSet management")
	flags.BoolVar(&opt.Internal, "internal", false, `When set, it configures the install-config.yaml's publish field to Internal.
OpenShift Installer publishes all the services of the cluster like API server and ingress to internal network and not the Internet.`)
	flags.StringVar(&opt.FeatureSet, "featureset", "", "FeatureSet to pass to the installer.")
	flags.BoolVar(&opt.PrivateLink, "private-link", false, "Enables access to cluster using AWS PrivateLink or GCP Private Service Connect")

	// Flags related to adoption.
	flags.BoolVar(&opt.Adopt, "adopt", false, "Enable adoption mode for importing a pre-existing cluster into Hive. Will require additional flags for adoption info.")
	flags.StringVar(&opt.AdoptAdminKubeConfig, "adopt-admin-kubeconfig", "", "Path to a cluster admin kubeconfig file for a cluster being adopted. (required if using --adopt)")
	flags.StringVar(&opt.AdoptInfraID, "adopt-infra-id", "", "Infrastructure ID for this cluster's cloud provider. (required if using --adopt)")
	flags.StringVar(&opt.AdoptClusterID, "adopt-cluster-id", "", "Cluster UUID used for telemetry. (required if using --adopt)")
	flags.StringVar(&opt.AdoptMetadataJSON, "adopt-metadata-json", "", "Path to a metadata.json file for a cluster being adopted. (optional)")
	flags.StringVar(&opt.AdoptAdminUsername, "adopt-admin-username", "", "Username for cluster web console administrator. (optional)")
	flags.StringVar(&opt.AdoptAdminPassword, "adopt-admin-password", "", "Password for cluster web console administrator. (optional)")

	// AWS flags
	flags.StringSliceVar(&opt.AWSUserTags, "aws-user-tags", nil, "Additional tags to add to resources. Must be in the form \"key=value\"")
	flags.BoolVar(&opt.AWSPrivateLink, "aws-private-link", false, "(Deprecated) See --private-link")
	flags.StringVar(&opt.AWSInstanceType, "aws-instance-type", clusterresource.AWSInstanceTypeDefault, "AWS cloud instance type for masters and workers (unless the latter is overridden by --aws-worker-instance-type)")
	flags.StringVar(&opt.AWSWorkerInstanceType, "aws-worker-instance-type", "", "AWS cloud instance type for workers only. If unset, --aws-instance-type is used.")

	// Azure flags
	flags.StringVar(&opt.AzureBaseDomainResourceGroupName, "azure-base-domain-resource-group-name", "os4-common", "Resource group where the azure DNS zone for the base domain is found")
	flags.StringVar(&opt.AzureCloudName, "azure-cloud-name", "AzurePublicCloud", "Azure Cloud in which cluster will be created")
	flags.StringVar(&opt.AzureResourceGroupName, "azure-resource-group-name", "", "Resource group where the cluster will be installed")

	// GCP flags
	flags.BoolVar(&opt.DiscardLocalSsdOnHibernate, "gcp-discard-local-ssd-on-hibernate", false, "(GCP) Preserve (false) or discard (true) contents of locally-attached SSDs when cluster is hibernated. Required when using hibernation with VM types with SSDs.")

	// OpenStack flags
	flags.StringVar(&opt.OpenStackCloud, "openstack-cloud", "openstack", "Section of clouds.yaml to use for API/auth")
	flags.StringVar(&opt.OpenStackExternalNetwork, "openstack-external-network", "provider_net_shared_3", "External OpenStack network name to deploy into")
	flags.StringVar(&opt.OpenStackMasterFlavor, "openstack-master-flavor", "ci.m4.xlarge", "Compute flavor to use for master nodes")
	flags.StringVar(&opt.OpenStackComputeFlavor, "openstack-compute-flavor", "m1.large", "Compute flavor to use for worker nodes")
	flags.StringVar(&opt.OpenStackAPIFloatingIP, "openstack-api-floating-ip", "", "Floating IP address to use for cluster's API")
	flags.StringVar(&opt.OpenStackIngressFloatingIP, "openstack-ingress-floating-ip", "", "Floating IP address to use for cluster's Ingress service")

	// vSphere flags
	flags.StringVar(&opt.VSphereVCenter, "vsphere-vcenter", "", "Domain name or IP address of the vCenter")
	flags.StringVar(&opt.VSphereDatacenter, "vsphere-datacenter", "", "Datacenter to use in the vCenter")
	flags.StringVar(&opt.VSphereDefaultDataStore, "vsphere-default-datastore", "", "Default datastore to use for provisioning volumes")
	flags.StringVar(&opt.VSphereFolder, "vsphere-folder", "", "Folder that will be used and/or created for virtual machines")
	flags.StringVar(&opt.VSphereCluster, "vsphere-cluster", "", "Cluster virtual machines will be cloned into")
	flags.StringVar(&opt.VSphereAPIVIP, "vsphere-api-vip", "", "Virtual IP address for the api endpoint")
	flags.StringVar(&opt.VSphereIngressVIP, "vsphere-ingress-vip", "", "Virtual IP address for ingress application routing")
	flags.StringVar(&opt.VSphereNetwork, "vsphere-network", "", "Name of the network to be used by the cluster")
	flags.StringVar(&opt.VSpherePlatformSpecJSON, "vsphere-platform-spec-json", "", "Installer vsphere platform spec, encoded as JSON")
	flags.StringVar(&opt.VSphereCACerts, "vsphere-ca-certs", "", "Path to vSphere CA certificate, multiple CA paths can be : delimited")

	// Nutanix
	flags.StringVar(&opt.NutanixPrismCentralEndpoint, constants.CliNutanixPcAddressOpt, "", "Domain name or IP address of the Nutanix Prism Central endpoint")
	flags.Int32Var(&opt.NutanixPrismCentralPort, constants.CliNutanixPcPortOpt, 0, "Port of the Nutanix Prism Central endpoint")
	flags.StringVar(&opt.NutanixPrismElementAddress, constants.CliNutanixPeAddressOpt, "", "Domain name or IP address of the Nutanix Prism Element endpoint")
	flags.StringVar(&opt.NutanixPrismElementName, constants.CliNutanixPeNameOpt, "", "Name of the Nutanix Prism Element endpoint")
	flags.Int32Var(&opt.NutanixPrismElementPort, constants.CliNutanixPePortOpt, 0, "Port of the Nutanix Prism Element endpoint")
	flags.StringVar(&opt.NutanixPrismElementUUID, constants.CliNutanixPeUUIDOpt, "", "UUID of the Nutanix Prism Element endpoint")
	flags.StringVar(&opt.NutanixAPIVIP, constants.CliNutanixApiVipOpt, "", "Virtual IP address for the api endpoint")
	flags.StringVar(&opt.NutanixIngressVIP, constants.CliNutanixIngressVipOpt, "", "Virtual IP address for ingress application routing")
	flags.StringVar(&opt.NutanixAzName, constants.CliNutanixAzNameOpt, "", "Name of the Prism Element Availability Zone")
	flags.StringSliceVar(&opt.NutanixSubnetUUIDs, constants.CliNutanixSubnetUUIDOpt, []string{}, "List of network subnets to be used by the cluster")
	flags.StringVar(&opt.NutanixCACerts, constants.CliNutanixCACertsOpt, "", "Path to a PEM-encoded CA certificate file used to verify the Nutanix endpoint's TLS certificates")

	// Additional CA Trust Bundle
	flags.StringVar(&opt.AdditionalTrustBundle, "additional-trust-bundle", "", "Path to a CA Trust Bundle which will be added to the nodes trusted certificate store.")

	// IBM flags
	flags.StringVar(&opt.IBMInstanceType, "ibm-instance-type", "bx2-4x16", "IBM Cloud instance type")

	return cmd
}

// Complete finishes parsing arguments for the command
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	o.Name = args[0]

	if o.Region == "" {
		switch o.Cloud {
		case constants.PlatformAWS:
			o.Region = "us-east-1"
		case constants.PlatformAzure:
			o.Region = "centralus"
		case constants.PlatformGCP:
			o.Region = "us-east1"
		case constants.PlatformIBMCloud:
			o.Region = "us-east"
		}
	}

	if o.HibernateAfter != "" {
		dur, err := time.ParseDuration(o.HibernateAfter)
		if err != nil {
			return errors.Wrapf(err, "unable to parse HibernateAfter duration")
		}
		o.HibernateAfterDur = &dur
	}

	if manualCCOModeClouds.Has(o.Cloud) && !o.CredentialsModeManual {
		o.CredentialsModeManual = true
		o.log.Infof("Using Manual credentials mode for cloud=%s", o.Cloud)
	}

	return nil
}

// Validate ensures that option values make sense
func (o *Options) Validate(cmd *cobra.Command) error {
	if len(o.Output) > 0 && o.Output != "yaml" && o.Output != "json" {
		cmd.Usage()
		o.log.Info("Invalid value for output. Valid values are: yaml, json.")
		return fmt.Errorf("invalid output")
	}
	if !o.UseClusterImageSet && len(o.ClusterImageSet) > 0 {
		cmd.Usage()
		o.log.Info("If not using cluster image sets, do not specify the name of one")
		return fmt.Errorf("invalid option")
	}
	if len(o.ServingCert) > 0 && len(o.ServingCertKey) == 0 {
		cmd.Usage()
		o.log.Info("If specifying a serving certificate, specify a valid serving certificate key")
		return fmt.Errorf("invalid serving cert")
	}
	if !validClouds.Has(o.Cloud) {
		cmd.Usage()
		o.log.Infof("Unsupported cloud: %s", o.Cloud)
		return fmt.Errorf("unsupported cloud: %s", o.Cloud)
	}

	if o.Cloud == constants.PlatformOpenStack {
		if o.OpenStackAPIFloatingIP == "" {
			msg := fmt.Sprintf("--openstack-api-floating-ip must be set when using --cloud=%q", constants.PlatformOpenStack)
			o.log.Info(msg)
			return errors.New(msg)
		}
		if o.OpenStackCloud == "" {
			msg := fmt.Sprintf("--openstack-cloud must be set when using --cloud=%q", constants.PlatformOpenStack)
			o.log.Info(msg)
			return errors.New(msg)
		}
	}

	if o.CredentialsModeManual && o.ManifestsDir == "" {
		return fmt.Errorf("Manual credentials mode requires --manifests containing custom Secrets with manually provisioned credentials")
	}

	if o.AWSPrivateLink && o.Cloud != constants.PlatformAWS {
		return fmt.Errorf("--aws-private-link can only be enabled when using --cloud=%q", constants.PlatformAWS)
	}

	if o.PrivateLink && o.Cloud != constants.PlatformAWS && o.Cloud != constants.PlatformGCP {
		return fmt.Errorf("--private-link can only be enabled when using --cloud={%q,%q}", constants.PlatformAWS, constants.PlatformGCP)
	}

	if o.Adopt {
		if o.AdoptAdminKubeConfig == "" || o.AdoptInfraID == "" || o.AdoptClusterID == "" {
			return fmt.Errorf("must specify the following options when using --adopt: --adopt-admin-kube-config, --adopt-infra-id, --adopt-cluster-id")
		}

		if _, err := os.Stat(o.AdoptAdminKubeConfig); os.IsNotExist(err) {
			return fmt.Errorf("--adopt-admin-kubeconfig does not exist: %s", o.AdoptAdminKubeConfig)
		}

		if o.AdoptMetadataJSON != "" {
			if _, err := os.Stat(o.AdoptMetadataJSON); os.IsNotExist(err) {
				return fmt.Errorf("--adopt-metadata-json does not exist: %s", o.AdoptMetadataJSON)
			}
		}

		// Admin username and password must both be specified if either are.
		if (o.AdoptAdminUsername != "" || o.AdoptAdminPassword != "") && !(o.AdoptAdminUsername != "" && o.AdoptAdminPassword != "") {
			return fmt.Errorf("--adopt-admin-username and --adopt-admin-password must be used together")
		}
	} else {
		if o.AdoptAdminKubeConfig != "" || o.AdoptInfraID != "" || o.AdoptClusterID != "" || o.AdoptMetadataJSON != "" || o.AdoptAdminUsername != "" || o.AdoptAdminPassword != "" {
			return fmt.Errorf("cannot use adoption options without --adopt: --adopt-admin-kube-config, --adopt-infra-id, --adopt-cluster-id, --adopt-metadata-json, --adopt-admin-username, --adopt-admin-password")
		}
	}

	if o.Region != "" {
		switch c := o.Cloud; c {
		case constants.PlatformAWS, constants.PlatformAzure, constants.PlatformGCP, constants.PlatformIBMCloud:
		default:
			return fmt.Errorf("cannot specify --region when using --cloud=%q", c)
		}
	}

	for _, ls := range o.Labels {
		tokens := strings.Split(ls, "=")
		if len(tokens) != 2 {
			return fmt.Errorf("unable to parse key=value label: %s", ls)
		}
	}
	for _, ls := range o.Annotations {
		tokens := strings.Split(ls, "=")
		if len(tokens) != 2 {
			return fmt.Errorf("unable to parse key=value annotation: %s", ls)
		}
	}

	// Differentiate among `true` (discard), `false` (preserve), and `nil` (user did not specify,
	// so we won't send the option through the GCP API at all).
	if f := cmd.Flags().Lookup("gcp-discard-local-ssd-on-hibernate"); f != nil && f.Changed {
		o.discardLocalSsdOnHibernate = &o.DiscardLocalSsdOnHibernate
	}

	return nil
}

// Run executes the command
func (o *Options) Run() error {
	scheme := scheme.GetScheme()

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
		printObjects(objs, scheme, printer)
		return err
	}
	rh, err := utils.GetResourceHelper("util-create-cluster", o.log)
	if err != nil {
		return err
	}
	if len(o.Namespace) == 0 {
		o.Namespace, err = utils.DefaultNamespace()
		if err != nil {
			o.log.Error("Cannot determine default namespace")
			return err
		}
	}
	for _, obj := range objs {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			o.log.WithError(err).Errorf("Cannot create accessor for object of type %T", obj)
			return err
		}
		accessor.SetNamespace(o.Namespace)
		if _, err := rh.ApplyRuntimeObject(obj, scheme); err != nil {
			return err
		}

	}
	return nil
}

// GenerateObjects generates resources for a new cluster deployment
func (o *Options) GenerateObjects() ([]runtime.Object, error) {

	pullSecret, err := utils.GetPullSecret(o.log, o.PullSecret, o.PullSecretFile)
	if err != nil {
		return nil, err
	}

	sshPrivateKey, err := o.getSSHPrivateKey()
	if err != nil {
		return nil, err
	}

	sshPublicKey, err := o.getSSHPublicKey()
	if err != nil {
		return nil, err
	}

	additionalTrustBundle, err := o.getAdditionalTrustBundle()
	if err != nil {
		return nil, err
	}

	// Load installer manifest files:
	manifestFileData, err := o.getManifestFileBytes()
	if err != nil {
		return nil, err
	}

	labels := map[string]string{
		hiveutilCreatedLabel: "true", // implied
	}
	for _, ls := range o.Labels {
		tokens := strings.Split(ls, "=")
		labels[tokens[0]] = tokens[1]
	}

	annotations := map[string]string{}
	for _, ls := range o.Annotations {
		tokens := strings.Split(ls, "=")
		annotations[tokens[0]] = tokens[1]
	}

	builder := &clusterresource.Builder{
		Name:                  o.Name,
		Namespace:             o.Namespace,
		WorkerNodesCount:      o.WorkerNodesCount,
		PullSecret:            pullSecret,
		SSHPrivateKey:         sshPrivateKey,
		SSHPublicKey:          sshPublicKey,
		InstallOnce:           o.InstallOnce,
		BaseDomain:            o.BaseDomain,
		ManageDNS:             o.ManageDNS,
		DeleteAfter:           o.DeleteAfter,
		HibernateAfter:        o.HibernateAfterDur,
		Labels:                labels,
		Annotations:           annotations,
		InstallerManifests:    manifestFileData,
		MachineNetwork:        o.MachineNetwork,
		SkipMachinePools:      o.SkipMachinePools,
		AdditionalTrustBundle: additionalTrustBundle,
		FeatureSet:            o.FeatureSet,
	}
	if o.Adopt {
		kubeconfigBytes, err := os.ReadFile(o.AdoptAdminKubeConfig)
		if err != nil {
			return nil, err
		}
		if o.AdoptMetadataJSON != "" {
			metadataJSONBytes, err := os.ReadFile(o.AdoptMetadataJSON)
			if err != nil {
				return nil, err
			}
			builder.AdoptMetadataJSON = metadataJSONBytes
		}
		builder.Adopt = o.Adopt
		builder.AdoptInfraID = o.AdoptInfraID
		builder.AdoptClusterID = o.AdoptClusterID
		builder.AdoptAdminKubeconfig = kubeconfigBytes
		builder.AdoptAdminUsername = o.AdoptAdminUsername
		builder.AdoptAdminPassword = o.AdoptAdminPassword
	}
	if len(o.BoundServiceAccountSigningKeyFile) != 0 {
		signingKey, err := os.ReadFile(o.BoundServiceAccountSigningKeyFile)
		if err != nil {
			return nil, fmt.Errorf("error reading %s: %v", o.BoundServiceAccountSigningKeyFile, err)
		}
		builder.BoundServiceAccountSigningKey = string(signingKey)
	}
	if o.CredentialsModeManual {
		builder.CredentialsMode = installertypes.ManualCredentialsMode
	}

	switch o.Cloud {
	case constants.PlatformAWS:
		defaultCredsFilePath := filepath.Join(o.homeDir, ".aws", "credentials")
		accessKeyID, secretAccessKey, err := awscreds.GetAWSCreds(o.CredsFile, defaultCredsFilePath)
		if err != nil {
			return nil, err
		}
		userTags := make(map[string]string, len(o.AWSUserTags))
		for _, t := range o.AWSUserTags {
			tagParts := strings.SplitN(t, "=", 2)
			switch len(tagParts) {
			case 0:
			case 1:
				userTags[tagParts[0]] = ""
			case 2:
				userTags[tagParts[0]] = tagParts[1]
			}
		}
		awsProvider := &clusterresource.AWSCloudBuilder{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
			UserTags:        userTags,
			Region:          o.Region,
			InstanceType:    o.AWSInstanceType,
			// Will default to above if unset
			WorkerInstanceType: o.AWSWorkerInstanceType,
			PrivateLink:        o.PrivateLink || o.AWSPrivateLink,
		}
		builder.CloudBuilder = awsProvider
	case constants.PlatformAzure:
		creds, err := azurecreds.GetCreds(o.CredsFile)
		if err != nil {
			o.log.WithError(err).Error("Failed to read in Azure credentials")
			return nil, err
		}

		azureProvider := &clusterresource.AzureCloudBuilder{
			ServicePrincipal:            creds,
			BaseDomainResourceGroupName: o.AzureBaseDomainResourceGroupName,
			Region:                      o.Region,
			CloudName:                   hivev1azure.CloudEnvironment(o.AzureCloudName),
			ResourceGroupName:           o.AzureResourceGroupName,
		}
		builder.CloudBuilder = azureProvider
	case constants.PlatformGCP:
		creds, err := gcpcreds.GetCreds(o.CredsFile)
		if err != nil {
			return nil, err
		}
		projectID, err := gcpclient.ProjectID(creds)
		if err != nil {
			return nil, err
		}

		gcpProvider := &clusterresource.GCPCloudBuilder{
			ProjectID:             projectID,
			ServiceAccount:        creds,
			Region:                o.Region,
			PrivateServiceConnect: o.PrivateLink,
			// NB: not Discard... (capitalized) -- see Validate()
			DiscardLocalSsdOnHibernate: o.discardLocalSsdOnHibernate,
		}
		builder.CloudBuilder = gcpProvider
	case constants.PlatformOpenStack:
		cloudsYAMLContent, err := openstackcreds.GetCreds(o.CredsFile)
		if err != nil {
			return nil, err
		}
		openStackProvider := &clusterresource.OpenStackCloudBuilder{
			Cloud:             o.OpenStackCloud,
			CloudsYAMLContent: cloudsYAMLContent,
			ExternalNetwork:   o.OpenStackExternalNetwork,
			ComputeFlavor:     o.OpenStackComputeFlavor,
			MasterFlavor:      o.OpenStackMasterFlavor,
			APIFloatingIP:     o.OpenStackAPIFloatingIP,
			IngressFloatingIP: o.OpenStackIngressFloatingIP,
		}
		builder.CloudBuilder = openStackProvider
	case constants.PlatformVSphere:
		vsphereUsername := os.Getenv(constants.VSphereUsernameEnvVar)
		if vsphereUsername == "" {
			return nil, fmt.Errorf("no %s env var set, cannot proceed", constants.VSphereUsernameEnvVar)
		}

		vspherePassword := os.Getenv(constants.VSpherePasswordEnvVar)
		if vspherePassword == "" {
			return nil, fmt.Errorf("no %s env var set, cannot proceed", constants.VSpherePasswordEnvVar)
		}

		vsphereCACerts := os.Getenv(constants.VSphereTLSCACertsEnvVar)
		if o.VSphereCACerts != "" {
			vsphereCACerts = o.VSphereCACerts
		}
		if vsphereCACerts == "" {
			return nil, fmt.Errorf("must provide --vsphere-ca-certs or set %s env var set", constants.VSphereTLSCACertsEnvVar)
		}
		caCerts := [][]byte{}
		for _, cert := range filepath.SplitList(vsphereCACerts) {
			caCert, err := os.ReadFile(cert)
			if err != nil {
				return nil, fmt.Errorf("error reading %s: %w", cert, err)
			}
			caCerts = append(caCerts, caCert)
		}

		vSphereNetwork := os.Getenv(constants.VSphereNetworkEnvVar)
		if o.VSphereNetwork != "" {
			vSphereNetwork = o.VSphereNetwork
		}

		vSphereDatacenter := os.Getenv(constants.VSphereDataCenterEnvVar)
		if o.VSphereDatacenter != "" {
			vSphereDatacenter = o.VSphereDatacenter
		}

		vSphereDatastore := os.Getenv(constants.VSphereDataStoreEnvVar)
		if o.VSphereDefaultDataStore != "" {
			vSphereDatastore = o.VSphereDefaultDataStore
		}

		vSphereVCenter := os.Getenv(constants.VSphereVCenterEnvVar)
		if o.VSphereVCenter != "" {
			vSphereVCenter = o.VSphereVCenter
		}

		vSphereFolder := o.VSphereFolder
		vSphereCluster := o.VSphereCluster
		vSphereAPIVIP := o.VSphereAPIVIP
		vSphereIngressVIP := o.VSphereIngressVIP

		platformBytes := []byte(os.Getenv(constants.VSpherePlatformSpecJSONEnvVar))
		if o.VSpherePlatformSpecJSON != "" {
			platformBytes = []byte(o.VSpherePlatformSpecJSON)
		}

		if len(platformBytes) > 0 {
			o.log.Info("using provided installer platform spec instead of other flags for vsphere (size: %v)", len(platformBytes))
			platform := installervsphere.Platform{}
			err = json.Unmarshal(platformBytes, &platform)
			if err != nil {
				return nil, fmt.Errorf("error decoding platform %s: %w", o.VSpherePlatformSpecJSON, err)
			}

			vSphereVCenter = platform.VCenters[0].Server
			vSphereDatacenter = platform.VCenters[0].Datacenters[0]
			if vSphereDatacenter == "" {
				vSphereDatacenter = platform.FailureDomains[0].Topology.Datacenter
			}
			vSphereDatastore = platform.FailureDomains[0].Topology.Datastore
			vSphereFolder = platform.FailureDomains[0].Topology.Folder
			vSphereCluster = platform.FailureDomains[0].Topology.ComputeCluster
			vSphereNetwork = platform.FailureDomains[0].Topology.Networks[0]
		}

		if vSphereDatacenter == "" {
			return nil, fmt.Errorf("must provide --vsphere-datacenter or set %s env var", constants.VSphereDataCenterEnvVar)
		}
		if vSphereDatastore == "" {
			return nil, fmt.Errorf("must provide --vsphere-default-datastore or set %s env var", constants.VSphereDataStoreEnvVar)
		}
		if vSphereVCenter == "" {
			return nil, fmt.Errorf("must provide --vsphere-vcenter or set %s env var", constants.VSphereVCenterEnvVar)
		}

		vsphereProvider := &clusterresource.VSphereCloudBuilder{
			VCenter:          vSphereVCenter,
			Username:         vsphereUsername,
			Password:         vspherePassword,
			Datacenter:       vSphereDatacenter,
			DefaultDatastore: vSphereDatastore,
			Folder:           vSphereFolder,
			Cluster:          vSphereCluster,
			APIVIP:           vSphereAPIVIP,
			IngressVIP:       vSphereIngressVIP,
			Network:          vSphereNetwork,
			CACert:           bytes.Join(caCerts, []byte("\n")),
		}
		builder.CloudBuilder = vsphereProvider
	case constants.PlatformIBMCloud:
		ibmCloudAPIKey := os.Getenv(constants.IBMCloudAPIKeyEnvVar)
		if ibmCloudAPIKey == "" {
			return nil, fmt.Errorf("%s env var is required when using --cloud=%q", constants.IBMCloudAPIKeyEnvVar, constants.PlatformIBMCloud)
		}
		ibmCloudProvider := &clusterresource.IBMCloudBuilder{
			APIKey:       ibmCloudAPIKey,
			Region:       o.Region,
			InstanceType: o.IBMInstanceType,
		}
		builder.CloudBuilder = ibmCloudProvider
	case constants.PlatformNutanix:
		builder.CloudBuilder, err = o.getNutanixCloudBuilder()
		if err != nil {
			return nil, err
		}
	}

	if o.Internal {
		builder.PublishStrategy = "Internal"
	}

	if len(o.ServingCert) != 0 {
		servingCert, err := os.ReadFile(o.ServingCert)
		if err != nil {
			return nil, fmt.Errorf("error reading %s: %v", o.ServingCert, err)
		}
		builder.ServingCert = string(servingCert)
		servingCertKey, err := os.ReadFile(o.ServingCertKey)
		if err != nil {
			return nil, fmt.Errorf("error reading %s: %v", o.ServingCertKey, err)
		}
		builder.ServingCertKey = string(servingCertKey)
	}

	imageSet, err := o.configureImages(builder)
	if err != nil {
		return nil, err
	}

	result, err := builder.Build()
	if err != nil {
		return nil, err
	}

	// Add some additional objects we don't yet want to move to the cluster builder library.
	if imageSet != nil {
		result = append(result, imageSet)
	}

	if o.CreateSampleSyncsets {
		result = append(result, o.generateSampleSyncSets()...)
	}

	return result, nil
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
		data, err := os.ReadFile(o.SSHPublicKeyFile)
		if err != nil {
			o.log.Error("Cannot read SSH public key file")
			return "", err
		}
		sshPublicKey = strings.TrimSpace(string(data))
		return sshPublicKey, nil
	}

	o.log.Error("Cannot determine SSH key to use")
	return "", nil
}

func (o *Options) getSSHPrivateKey() (string, error) {
	if len(o.SSHPrivateKeyFile) > 0 {
		data, err := os.ReadFile(o.SSHPrivateKeyFile)
		if err != nil {
			o.log.Error("Cannot read SSH private key file")
			return "", err
		}
		sshPrivateKey := strings.TrimSpace(string(data)) + "\n"
		return sshPrivateKey, nil
	}
	o.log.Debug("No private SSH key file provided")
	return "", nil
}

func (o *Options) getAdditionalTrustBundle() (string, error) {
	if len(o.AdditionalTrustBundle) > 0 {
		data, err := os.ReadFile(o.AdditionalTrustBundle)
		if err != nil {
			o.log.Error("Cannot read AdditionalTrustBundle file")
			return "", err
		}
		if err := validate.CABundle(string(data)); err != nil {
			o.log.Error("AdditionalTrustBundle is not valid")
			return "", err
		}
		additionalTrustBundle := string(data)
		return additionalTrustBundle, nil
	}
	o.log.Debug("No AdditionalTrustBundle provided")
	return "", nil
}

func (o *Options) getManifestFileBytes() (map[string][]byte, error) {
	if o.ManifestsDir == "" && !o.SimulateBootstrapFailure {
		return nil, nil
	}
	fileData := map[string][]byte{}
	if o.ManifestsDir != "" {
		files, err := os.ReadDir(o.ManifestsDir)
		if err != nil {
			return nil, errors.Wrap(err, "could not read manifests directory")
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(o.ManifestsDir, file.Name()))
			if err != nil {
				return nil, errors.Wrapf(err, "could not read manifest file %q", file.Name())
			}
			fileData[file.Name()] = data
		}
	}
	if o.SimulateBootstrapFailure {
		fileData["failure-test.yaml"] = []byte(testFailureManifest)
	}
	return fileData, nil
}

func (o *Options) configureImages(generator *clusterresource.Builder) (*hivev1.ClusterImageSet, error) {
	if len(o.ClusterImageSet) > 0 {
		generator.ImageSet = o.ClusterImageSet
		return nil, nil
	}
	// TODO: move release image lookup code to the cluster library
	if o.ReleaseImage == "" {
		if o.ReleaseImageSource == "" {
			return nil, fmt.Errorf("specify either a release image or a release image source")
		}
		var err error
		o.ReleaseImage, err = utils.DetermineReleaseImageFromSource(o.ReleaseImageSource)
		if err != nil {
			return nil, fmt.Errorf("cannot determine release image: %v", err)
		}
	}
	if !o.UseClusterImageSet {
		generator.ReleaseImage = o.ReleaseImage
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
			ReleaseImage: o.ReleaseImage,
		},
	}
	generator.ImageSet = imageSet.Name
	return imageSet, nil
}

func (o *Options) generateSampleSyncSets() []runtime.Object {
	var syncsets []runtime.Object
	for i := range [10]int{} {
		syncsets = append(syncsets, sampleSyncSet(fmt.Sprintf("%s-sample-syncset%d", o.Name, i), o.Namespace, o.Name))
		syncsets = append(syncsets, sampleSelectorSyncSet(fmt.Sprintf("sample-selector-syncset%d", i)))
	}
	return syncsets
}

func sampleSyncSet(name, namespace, cdName string) *hivev1.SyncSet {
	return &hivev1.SyncSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SyncSet",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: cdName,
				},
			},
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				ResourceApplyMode: hivev1.SyncResourceApplyMode,
				Resources: []runtime.RawExtension{
					{
						Object: sampleCM(fmt.Sprintf("%s-configmap", name)),
					},
				},
			},
		},
	}
}

func sampleSelectorSyncSet(name string) *hivev1.SelectorSyncSet {
	return &hivev1.SelectorSyncSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SelectorSyncSet",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: hivev1.SelectorSyncSetSpec{
			ClusterDeploymentSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{hiveutilCreatedLabel: "true"},
			},
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				ResourceApplyMode: hivev1.SyncResourceApplyMode,
				Resources: []runtime.RawExtension{
					{
						Object: sampleCM(fmt.Sprintf("%s-configmap", name)),
					},
				},
			},
		},
	}
}

func sampleCM(name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Data: map[string]string{
			"foo": "bar",
		},
	}
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
