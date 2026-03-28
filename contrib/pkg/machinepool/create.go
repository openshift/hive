package machinepool

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/printers"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/machinepoolresource"
	"github.com/openshift/hive/pkg/util/scheme"
)

// CreateOptions holds options for machinepool create.
type CreateOptions struct {
	ClusterDeploymentName string
	PoolName              string
	Namespace             string
	Replicas              int64
	Output                string

	AWSInstanceType               string
	AWSWorkerInstanceType         string
	AWSUserTags                   []string
	AWSZones                      []string
	AWSSubnets                    []string
	AWSRootVolumeSize             int
	AWSRootVolumeType             string
	AWSRootVolumeIOPS             int
	AWSRootVolumeKMSKeyARN        string
	AWSSpotMaxPrice               string
	AWSEC2MetadataAuthentication  string
	AWSAdditionalSecurityGroupIDs []string

	// Azure platform
	AzureInstanceType             string
	AzureZones                    []string
	AzureOSDiskSizeGB             int
	AzureOSDiskType               string
	AzureDiskEncryptionSetSubID   string
	AzureDiskEncryptionSetRG      string
	AzureDiskEncryptionSetName    string
	AzureNetworkResourceGroupName string
	AzureComputeSubnet            string
	AzureVirtualNetwork           string
	AzureVMNetworkingType         string
	AzureOutboundType             string
	AzureOSImagePlan              string
	AzureOSImagePublisher         string
	AzureOSImageOffer             string
	AzureOSImageSKU               string
	AzureOSImageVersion           string

	// GCP platform
	GCPInstanceType           string
	GCPZones                  []string
	GCPOSDiskType             string
	GCPOSDiskSize             int64
	GCPEncryptionKeyName      string
	GCPEncryptionKeyKeyRing   string
	GCPEncryptionKeyProjectID string
	GCPEncryptionKeyLocation  string
	GCPKMSKeyServiceAccount   string
	GCPNetworkProjectID       string
	GCPSecureBoot             string
	GCPOnHostMaintenance      string
	GCPServiceAccount         string
	GCPUserTags               []string // parentID:key=value
	GCPTags                   []string

	// vSphere platform
	VSphereResourcePool      string
	VSphereNumCPUs           int32
	VSphereNumCoresPerSocket int32
	VSphereMemoryMiB         int64
	VSphereOSDiskSize        int32
	VSphereTagIDs            []string

	// IBM Cloud platform
	IBMCloudInstanceType            string
	IBMCloudZones                   []string
	IBMCloudBootVolumeEncryptionKey string
	IBMCloudDedicatedHosts          []string // name=xxx,profile=yyy

	// Nutanix platform
	NutanixNumCPUs           int64
	NutanixNumCoresPerSocket int64
	NutanixMemoryMiB         int64
	NutanixOSDiskSizeGiB     int64
	NutanixBootType          string
	NutanixFailureDomains    []string

	// OpenStack platform
	OpenStackFlavor                     string
	OpenStackRootVolumeSize             int
	OpenStackRootVolumeType             string
	OpenStackAdditionalSecurityGroupIDs []string

	log log.FieldLogger
}

// NewCreateCommand returns the machinepool create subcommand.
func NewCreateCommand() *cobra.Command {
	opt := &CreateOptions{log: log.WithField("command", "machinepool create")}
	cmd := &cobra.Command{
		Use:   "create CLUSTER_DEPLOYMENT_NAME POOL_NAME",
		Short: "Create a MachinePool for an existing ClusterDeployment",
		Long:  "Creates a Hive MachinePool in the same namespace as the ClusterDeployment. Platform is inferred from the ClusterDeployment.",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			if err := opt.Complete(cmd, args); err != nil {
				opt.log.WithError(err).Fatal("Complete failed")
			}
			if err := opt.Validate(cmd); err != nil {
				opt.log.WithError(err).Fatal("Validate failed")
			}
			if err := opt.Run(); err != nil {
				opt.log.WithError(err).Fatal("Run failed")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&opt.Namespace, "namespace", "n", "", "Namespace of the ClusterDeployment (default from kubeconfig)")
	flags.Int64Var(&opt.Replicas, "replicas", 1, "Number of replicas for the machine pool")
	flags.StringVarP(&opt.Output, "output", "o", "", "Output format: yaml or json (no apply)")
	flags.StringVar(&opt.AWSInstanceType, "aws-instance-type", machinepoolresource.DefaultAWSInstanceType, "AWS instance type for this pool")
	flags.StringVar(&opt.AWSWorkerInstanceType, "aws-worker-instance-type", "", "AWS instance type for workers (overrides --aws-instance-type for this pool)")
	flags.StringSliceVar(&opt.AWSUserTags, "aws-user-tags", nil, "AWS tags key=value (comma-separated)")
	flags.StringSliceVar(&opt.AWSZones, "aws-zones", nil, "AWS availability zones for the machine pool")
	flags.StringSliceVar(&opt.AWSSubnets, "aws-subnets", nil, "AWS subnet IDs for the machine pool")
	flags.IntVar(&opt.AWSRootVolumeSize, "aws-root-volume-size", 0, "AWS root volume size in GiB (0 uses default)")
	flags.StringVar(&opt.AWSRootVolumeType, "aws-root-volume-type", "", "AWS root volume type (e.g. gp3, io1)")
	flags.IntVar(&opt.AWSRootVolumeIOPS, "aws-root-volume-iops", 0, "AWS root volume IOPS (for io1/gp3)")
	flags.StringVar(&opt.AWSRootVolumeKMSKeyARN, "aws-root-volume-kms-key-arn", "", "AWS KMS key ARN for root volume encryption")
	flags.StringVar(&opt.AWSSpotMaxPrice, "aws-spot-max-price", "", "AWS spot instance max price (enables spot)")
	flags.StringVar(&opt.AWSEC2MetadataAuthentication, "aws-ec2-metadata-auth", "", "AWS EC2 metadata authentication (e.g. Required)")
	flags.StringSliceVar(&opt.AWSAdditionalSecurityGroupIDs, "aws-additional-security-group-ids", nil, "AWS additional security group IDs")

	// Azure platform
	flags.StringVar(&opt.AzureInstanceType, "azure-instance-type", machinepoolresource.DefaultAzureInstanceType, "Azure VM size for this pool")
	flags.StringSliceVar(&opt.AzureZones, "azure-zones", nil, "Azure availability zones (e.g. 1,2,3)")
	flags.IntVar(&opt.AzureOSDiskSizeGB, "azure-os-disk-size", 0, "Azure OS disk size in GB (0 uses default)")
	flags.StringVar(&opt.AzureOSDiskType, "azure-os-disk-type", "", "Azure OS disk type (Standard_LRS, Premium_LRS, StandardSSD_LRS)")
	flags.StringVar(&opt.AzureDiskEncryptionSetSubID, "azure-disk-encryption-set-subscription-id", "", "Azure disk encryption set subscription ID")
	flags.StringVar(&opt.AzureDiskEncryptionSetRG, "azure-disk-encryption-set-resource-group", "", "Azure disk encryption set resource group")
	flags.StringVar(&opt.AzureDiskEncryptionSetName, "azure-disk-encryption-set-name", "", "Azure disk encryption set name")
	flags.StringVar(&opt.AzureNetworkResourceGroupName, "azure-network-resource-group", "", "Azure network resource group (existing VNet)")
	flags.StringVar(&opt.AzureComputeSubnet, "azure-compute-subnet", "", "Azure compute subnet for workers")
	flags.StringVar(&opt.AzureVirtualNetwork, "azure-virtual-network", "", "Azure virtual network name")
	flags.StringVar(&opt.AzureVMNetworkingType, "azure-vm-networking-type", "", "Azure VM networking (Accelerated or Basic)")
	flags.StringVar(&opt.AzureOutboundType, "azure-outbound-type", "", "Azure outbound type (e.g. Loadbalancer)")
	flags.StringVar(&opt.AzureOSImagePlan, "azure-os-image-plan", "", "Azure OS image plan (WithPurchasePlan or NoPurchasePlan)")
	flags.StringVar(&opt.AzureOSImagePublisher, "azure-os-image-publisher", "", "Azure OS image publisher")
	flags.StringVar(&opt.AzureOSImageOffer, "azure-os-image-offer", "", "Azure OS image offer")
	flags.StringVar(&opt.AzureOSImageSKU, "azure-os-image-sku", "", "Azure OS image SKU")
	flags.StringVar(&opt.AzureOSImageVersion, "azure-os-image-version", "", "Azure OS image version")

	// GCP platform
	flags.StringVar(&opt.GCPInstanceType, "gcp-instance-type", machinepoolresource.DefaultGCPInstanceType, "GCP instance type for this pool (e.g. n1-standard-4)")
	flags.StringSliceVar(&opt.GCPZones, "gcp-zones", nil, "GCP zones for the machine pool")
	flags.StringVar(&opt.GCPOSDiskType, "gcp-os-disk-type", "", "GCP OS disk type (pd-standard, pd-ssd, pd-balanced, hyperdisk-balanced)")
	flags.Int64Var(&opt.GCPOSDiskSize, "gcp-os-disk-size", 0, "GCP OS disk size in GB (0 uses default)")
	flags.StringVar(&opt.GCPEncryptionKeyName, "gcp-disk-encryption-key-name", "", "GCP KMS key name for disk encryption")
	flags.StringVar(&opt.GCPEncryptionKeyKeyRing, "gcp-disk-encryption-key-ring", "", "GCP KMS key ring name")
	flags.StringVar(&opt.GCPEncryptionKeyProjectID, "gcp-disk-encryption-key-project-id", "", "GCP project containing the KMS key ring")
	flags.StringVar(&opt.GCPEncryptionKeyLocation, "gcp-disk-encryption-key-location", "", "GCP location of the KMS key ring")
	flags.StringVar(&opt.GCPKMSKeyServiceAccount, "gcp-disk-encryption-kms-service-account", "", "GCP service account for KMS encryption requests")
	flags.StringVar(&opt.GCPNetworkProjectID, "gcp-network-project-id", "", "GCP project where network/subnets exist (if different from cluster)")
	flags.StringVar(&opt.GCPSecureBoot, "gcp-secure-boot", "", "GCP secure boot (Enabled or Disabled)")
	flags.StringVar(&opt.GCPOnHostMaintenance, "gcp-on-host-maintenance", "", "GCP on host maintenance (Migrate or Terminate)")
	flags.StringVar(&opt.GCPServiceAccount, "gcp-service-account", "", "GCP service account email for worker nodes")
	flags.StringSliceVar(&opt.GCPUserTags, "gcp-user-tags", nil, "GCP user tags as parentID:key=value")
	flags.StringSliceVar(&opt.GCPTags, "gcp-tags", nil, "GCP network tags for instances")

	// vSphere platform
	flags.StringVar(&opt.VSphereResourcePool, "vsphere-resource-pool", "", "vSphere resource pool for VMs")
	flags.Int32Var(&opt.VSphereNumCPUs, "vsphere-num-cpus", 0, "vSphere number of vCPUs per VM (0 uses default)")
	flags.Int32Var(&opt.VSphereNumCoresPerSocket, "vsphere-cores-per-socket", 0, "vSphere cores per socket (0 uses default)")
	flags.Int64Var(&opt.VSphereMemoryMiB, "vsphere-memory-mib", 0, "vSphere memory per VM in MiB (0 uses default)")
	flags.Int32Var(&opt.VSphereOSDiskSize, "vsphere-os-disk-size", 0, "vSphere OS disk size in GB (0 uses default)")
	flags.StringSliceVar(&opt.VSphereTagIDs, "vsphere-tag-ids", nil, "vSphere tag IDs for VMs (max 10)")

	// IBM Cloud platform
	flags.StringVar(&opt.IBMCloudInstanceType, "ibmcloud-instance-type", machinepoolresource.DefaultIBMCloudInstanceType, "IBM Cloud VSI profile for this pool (e.g. bx2-4x16)")
	flags.StringSliceVar(&opt.IBMCloudZones, "ibmcloud-zones", nil, "IBM Cloud availability zones for the machine pool")
	flags.StringVar(&opt.IBMCloudBootVolumeEncryptionKey, "ibmcloud-boot-volume-encryption-key", "", "IBM Cloud boot volume encryption key CRN (Key Protect or Hyper Protect)")
	flags.StringSliceVar(&opt.IBMCloudDedicatedHosts, "ibmcloud-dedicated-hosts", nil, "IBM Cloud dedicated hosts as name=xxx,profile=yyy")

	// Nutanix platform
	flags.Int64Var(&opt.NutanixNumCPUs, "nutanix-num-cpus", 0, "Nutanix number of vCPUs per VM (0 uses default)")
	flags.Int64Var(&opt.NutanixNumCoresPerSocket, "nutanix-cores-per-socket", 0, "Nutanix cores per socket (0 uses default)")
	flags.Int64Var(&opt.NutanixMemoryMiB, "nutanix-memory-mib", 0, "Nutanix memory per VM in MiB (0 uses default)")
	flags.Int64Var(&opt.NutanixOSDiskSizeGiB, "nutanix-os-disk-size", 0, "Nutanix OS disk size in GiB (0 uses default)")
	flags.StringVar(&opt.NutanixBootType, "nutanix-boot-type", "", "Nutanix boot type (Legacy, UEFI, SecureBoot)")
	flags.StringSliceVar(&opt.NutanixFailureDomains, "nutanix-failure-domains", nil, "Nutanix failure domain names (must exist in CD.Spec.Platform.Nutanix)")

	// OpenStack platform
	flags.StringVar(&opt.OpenStackFlavor, "openstack-flavor", machinepoolresource.DefaultOpenStackFlavor, "OpenStack Nova flavor for this pool (e.g. m1.medium)")
	flags.IntVar(&opt.OpenStackRootVolumeSize, "openstack-root-volume-size", 0, "OpenStack root volume size in GiB (0 = ephemeral disk)")
	flags.StringVar(&opt.OpenStackRootVolumeType, "openstack-root-volume-type", "", "OpenStack root volume type (required if root-volume-size > 0)")
	flags.StringSliceVar(&opt.OpenStackAdditionalSecurityGroupIDs, "openstack-additional-security-group-ids", nil, "OpenStack additional security group UUIDs")
	return cmd
}

// Complete fills options from args and defaults.
func (o *CreateOptions) Complete(cmd *cobra.Command, args []string) error {
	o.ClusterDeploymentName = args[0]
	o.PoolName = args[1]
	if o.Namespace == "" {
		ns, err := utils.DefaultNamespace()
		if err != nil {
			return errors.Wrap(err, "default namespace")
		}
		o.Namespace = ns
	}
	return nil
}

// Validate checks options.
func (o *CreateOptions) Validate(cmd *cobra.Command) error {
	if o.Replicas < 0 {
		return fmt.Errorf("replicas must be >= 0")
	}
	if o.Output != "" && o.Output != "yaml" && o.Output != "json" {
		cmd.Usage()
		return fmt.Errorf("output must be yaml or json")
	}
	return nil
}

// Run creates the MachinePool (apply or output).
func (o *CreateOptions) Run() error {
	cd, err := GetClusterDeployment(o.Namespace, o.ClusterDeploymentName)
	if err != nil {
		return err
	}
	platform, err := PlatformFromCD(cd)
	if err != nil {
		return err
	}

	var filler machinepoolresource.PlatformFiller
	switch platform {
	case constants.PlatformAWS:
		filler = o.awsOptions()
	case constants.PlatformAzure:
		filler = o.azureOptions()
	case constants.PlatformGCP:
		filler = o.gcpOptions()
	case constants.PlatformVSphere:
		filler = o.vsphereOptions()
	case constants.PlatformIBMCloud:
		filler = o.ibmcloudOptions()
	case constants.PlatformNutanix:
		filler = o.nutanixOptions()
	case constants.PlatformOpenStack:
		filler = o.openstackOptions()
	default:
		return fmt.Errorf("platform %q not yet supported by machinepool create (aws, azure, gcp, vsphere, ibmcloud, nutanix, openstack)", platform)
	}

	mp := machinepoolresource.BuildMachinePoolWithReplicas(cd.Name, cd.Namespace, o.PoolName, o.Replicas, filler)
	if mp == nil {
		return fmt.Errorf("failed to build MachinePool")
	}

	scheme := scheme.GetScheme()
	if o.Output != "" {
		var printer printers.ResourcePrinter
		if o.Output == "yaml" {
			printer = &printers.YAMLPrinter{}
		} else {
			printer = &printers.JSONPrinter{}
		}
		return printer.PrintObj(mp, nil)
	}

	rh, err := utils.GetResourceHelper("hiveutil-machinepool-create", o.log)
	if err != nil {
		return errors.Wrap(err, "get resource helper")
	}
	if _, err := rh.ApplyRuntimeObject(mp, scheme); err != nil {
		return errors.Wrap(err, "apply MachinePool")
	}
	o.log.Infof("MachinePool %s/%s created", mp.Namespace, mp.Name)
	return nil
}

func (o *CreateOptions) awsOptions() *machinepoolresource.AWSOptions {
	userTags := make(map[string]string)
	for _, t := range o.AWSUserTags {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) == 2 {
			userTags[parts[0]] = parts[1]
		} else if len(parts) == 1 {
			userTags[parts[0]] = ""
		}
	}
	opts := &machinepoolresource.AWSOptions{
		Zones:                      o.AWSZones,
		Subnets:                    o.AWSSubnets,
		InstanceType:               o.AWSInstanceType,
		WorkerInstanceType:         o.AWSWorkerInstanceType,
		RootVolumeSize:             o.AWSRootVolumeSize,
		RootVolumeType:             o.AWSRootVolumeType,
		RootVolumeIOPS:             o.AWSRootVolumeIOPS,
		RootVolumeKMSKeyARN:        o.AWSRootVolumeKMSKeyARN,
		EC2MetadataAuthentication:  o.AWSEC2MetadataAuthentication,
		AdditionalSecurityGroupIDs: o.AWSAdditionalSecurityGroupIDs,
		UserTags:                   userTags,
	}
	if o.AWSSpotMaxPrice != "" {
		opts.SpotMarketOptionsMaxPrice = &o.AWSSpotMaxPrice
	}
	return opts
}

func (o *CreateOptions) azureOptions() *machinepoolresource.AzureOptions {
	return &machinepoolresource.AzureOptions{
		Zones:                    o.AzureZones,
		InstanceType:             o.AzureInstanceType,
		OSDiskSizeGB:             o.AzureOSDiskSizeGB,
		OSDiskType:               o.AzureOSDiskType,
		DiskEncryptionSetSubID:   o.AzureDiskEncryptionSetSubID,
		DiskEncryptionSetRG:      o.AzureDiskEncryptionSetRG,
		DiskEncryptionSetName:    o.AzureDiskEncryptionSetName,
		NetworkResourceGroupName: o.AzureNetworkResourceGroupName,
		ComputeSubnet:            o.AzureComputeSubnet,
		VirtualNetwork:           o.AzureVirtualNetwork,
		VMNetworkingType:         o.AzureVMNetworkingType,
		OutboundType:             o.AzureOutboundType,
		OSImagePlan:              o.AzureOSImagePlan,
		OSImagePublisher:         o.AzureOSImagePublisher,
		OSImageOffer:             o.AzureOSImageOffer,
		OSImageSKU:               o.AzureOSImageSKU,
		OSImageVersion:           o.AzureOSImageVersion,
	}
}

func (o *CreateOptions) gcpOptions() *machinepoolresource.GCPOptions {
	return &machinepoolresource.GCPOptions{
		Zones:                  o.GCPZones,
		InstanceType:           o.GCPInstanceType,
		OSDiskType:             o.GCPOSDiskType,
		OSDiskSizeGB:           o.GCPOSDiskSize,
		EncryptionKeyName:      o.GCPEncryptionKeyName,
		EncryptionKeyKeyRing:   o.GCPEncryptionKeyKeyRing,
		EncryptionKeyProjectID: o.GCPEncryptionKeyProjectID,
		EncryptionKeyLocation:  o.GCPEncryptionKeyLocation,
		KMSKeyServiceAccount:   o.GCPKMSKeyServiceAccount,
		NetworkProjectID:       o.GCPNetworkProjectID,
		SecureBoot:             o.GCPSecureBoot,
		OnHostMaintenance:      o.GCPOnHostMaintenance,
		ServiceAccount:         o.GCPServiceAccount,
		UserTags:               machinepoolresource.ParseGCPUserTags(o.GCPUserTags),
		Tags:                   o.GCPTags,
	}
}

func (o *CreateOptions) vsphereOptions() *machinepoolresource.VSphereOptions {
	return &machinepoolresource.VSphereOptions{
		ResourcePool:      o.VSphereResourcePool,
		NumCPUs:           o.VSphereNumCPUs,
		NumCoresPerSocket: o.VSphereNumCoresPerSocket,
		MemoryMiB:         o.VSphereMemoryMiB,
		OSDiskSizeGB:      o.VSphereOSDiskSize,
		TagIDs:            o.VSphereTagIDs,
	}
}

func (o *CreateOptions) ibmcloudOptions() *machinepoolresource.IBMCloudOptions {
	return &machinepoolresource.IBMCloudOptions{
		InstanceType:   o.IBMCloudInstanceType,
		Zones:          o.IBMCloudZones,
		BootVolumeKey:  o.IBMCloudBootVolumeEncryptionKey,
		DedicatedHosts: machinepoolresource.ParseIBMCloudDedicatedHosts(o.IBMCloudDedicatedHosts),
	}
}

func (o *CreateOptions) nutanixOptions() *machinepoolresource.NutanixOptions {
	return &machinepoolresource.NutanixOptions{
		NumCPUs:           o.NutanixNumCPUs,
		NumCoresPerSocket: o.NutanixNumCoresPerSocket,
		MemoryMiB:         o.NutanixMemoryMiB,
		OSDiskSizeGiB:     o.NutanixOSDiskSizeGiB,
		BootType:          o.NutanixBootType,
		FailureDomains:    o.NutanixFailureDomains,
	}
}

func (o *CreateOptions) openstackOptions() *machinepoolresource.OpenStackOptions {
	return &machinepoolresource.OpenStackOptions{
		Flavor:                     o.OpenStackFlavor,
		RootVolumeSize:             o.OpenStackRootVolumeSize,
		RootVolumeType:             o.OpenStackRootVolumeType,
		AdditionalSecurityGroupIDs: o.OpenStackAdditionalSecurityGroupIDs,
	}
}
