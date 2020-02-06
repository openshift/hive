package util

import (
	"fmt"
	"strings"

	netopv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"
	"github.com/openshift/installer/pkg/ipnet"
	installtypes "github.com/openshift/installer/pkg/types"
	installtypesaws "github.com/openshift/installer/pkg/types/aws"
	installtypesazure "github.com/openshift/installer/pkg/types/azure"
	installtypesgcp "github.com/openshift/installer/pkg/types/gcp"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/pkg/apis/hive/v1/azure"
	hivev1gcp "github.com/openshift/hive/pkg/apis/hive/v1/gcp"
	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
	hiveapiaws "github.com/openshift/hive/pkg/hive/apis/hive/aws"
	hiveapiazure "github.com/openshift/hive/pkg/hive/apis/hive/azure"
	hiveapigcp "github.com/openshift/hive/pkg/hive/apis/hive/gcp"
	"github.com/openshift/hive/pkg/hive/apis/hive/hiveconversion"
)

// CheckpointToHiveV1 turns a v1alpha1 Checkpoint into a v1 Checkpoint.
func CheckpointToHiveV1(in *hiveapi.Checkpoint, out *hivev1.Checkpoint) error {
	return hiveconversion.Convert_v1alpha1_Checkpoint_To_v1_Checkpoint(in.DeepCopy(), out, nil)
}

// CheckpointFromHiveV1 turns a v1 Checkpoint into a v1alpha1 Checkpoint.
func CheckpointFromHiveV1(in *hivev1.Checkpoint, out *hiveapi.Checkpoint) error {
	return hiveconversion.Convert_v1_Checkpoint_To_v1alpha1_Checkpoint(in.DeepCopy(), out, nil)
}

// ClusterDeploymentToHiveV1 turns a v1alpha1 ClusterDeployment into a v1 ClusterDeployment.
func ClusterDeploymentToHiveV1(in *hiveapi.ClusterDeployment, sshKey string, out *hivev1.ClusterDeployment, installConfig *installtypes.InstallConfig, machinePools *[]*hivev1.MachinePool) error {
	in = in.DeepCopy()

	// Preserve the resource version since the v1alpha1 resource version is an amalgamation of resource versions from
	// multiple resources.
	outResourceVersion := out.ResourceVersion

	if err := hiveconversion.Convert_v1alpha1_ClusterDeployment_To_v1_ClusterDeployment(in, out, nil); err != nil {
		return err
	}

	out.ResourceVersion = outResourceVersion

	installConfig.ObjectMeta.Name = in.Spec.ClusterName
	installConfig.TypeMeta.APIVersion = installtypes.InstallConfigVersion
	installConfig.SSHKey = sshKey
	installConfig.BaseDomain = in.Spec.BaseDomain
	if installConfig.Networking == nil {
		installConfig.Networking = &installtypes.Networking{}
	}
	installConfig.Networking.NetworkType = string(in.Spec.Networking.Type)
	installConfig.Networking.ServiceNetwork = []ipnet.IPNet{*parseCIDR(in.Spec.Networking.ServiceCIDR)}
	installConfig.Networking.MachineCIDR = parseCIDR(in.Spec.Networking.MachineCIDR)
	// installConfig.PullSecret = <filled in by the Hive installmanager>
	if installConfig.ControlPlane == nil {
		installConfig.ControlPlane = &installtypes.MachinePool{}
	}
	machinePoolToInstallConfig(&in.Spec.ControlPlane, installConfig.ControlPlane)
	if nets := in.Spec.ClusterNetworks; nets != nil {
		installConfig.ClusterNetwork = make([]installtypes.ClusterNetworkEntry, len(nets))
		for i, inNet := range nets {
			outNet := &installConfig.ClusterNetwork[i]
			outNet.CIDR = *parseCIDR(inNet.CIDR)
			// In v1alpha1, Hive is mis-interpreting the host subnet length as the host prefix.
			outNet.HostPrefix = int32(inNet.HostSubnetLength)
		}
	} else {
		installConfig.ClusterNetwork = nil
	}
	if inAWS := in.Spec.AWS; inAWS != nil {
		if installConfig.AWS == nil {
			installConfig.AWS = &installtypesaws.Platform{}
		}
		outAWS := installConfig.AWS
		outAWS.Region = inAWS.Region
		outAWS.UserTags = inAWS.UserTags
		if p := inAWS.DefaultMachinePlatform; p != nil {
			if outAWS.DefaultMachinePlatform == nil {
				outAWS.DefaultMachinePlatform = &installtypesaws.MachinePool{}
			}
			awsMachinePoolToInstallConfig(p, outAWS.DefaultMachinePlatform)
		} else {
			outAWS.DefaultMachinePlatform = nil
		}
	} else {
		installConfig.AWS = nil
	}
	if inAzure := in.Spec.Azure; inAzure != nil {
		if installConfig.Azure == nil {
			installConfig.Azure = &installtypesazure.Platform{}
		}
		outAzure := installConfig.Azure
		outAzure.Region = inAzure.Region
		outAzure.BaseDomainResourceGroupName = inAzure.BaseDomainResourceGroupName
		if p := inAzure.DefaultMachinePlatform; p != nil {
			if outAzure.DefaultMachinePlatform == nil {
				outAzure.DefaultMachinePlatform = &installtypesazure.MachinePool{}
			}
			azureMachinePoolToInstallConfig(p, outAzure.DefaultMachinePlatform)
		} else {
			outAzure.DefaultMachinePlatform = nil
		}
	} else {
		installConfig.Azure = nil
	}
	if inGCP := in.Spec.GCP; inGCP != nil {
		if installConfig.GCP == nil {
			installConfig.GCP = &installtypesgcp.Platform{}
		}
		outGCP := installConfig.GCP
		outGCP.ProjectID = inGCP.ProjectID
		outGCP.Region = inGCP.Region
		if p := inGCP.DefaultMachinePlatform; p != nil {
			if outGCP.DefaultMachinePlatform == nil {
				outGCP.DefaultMachinePlatform = &installtypesgcp.MachinePool{}
			}
			gcpMachinePoolToInstallConfig(p, outGCP.DefaultMachinePlatform)
		} else {
			outGCP.DefaultMachinePlatform = nil
		}
	} else {
		installConfig.GCP = nil
	}
	installConfig.ControlPlane.Name = "master"
	installConfig.Compute = nil
	for _, inCompute := range in.Spec.Compute {
		if inCompute.Name != "worker" {
			continue
		}
		outCompute := installtypes.MachinePool{}
		machinePoolToInstallConfig(&inCompute, &outCompute)
		installConfig.Compute = append(installConfig.Compute, outCompute)
	}

	if compute := in.Spec.Compute; compute != nil {
		newMachinePools := make([]*hivev1.MachinePool, len(compute))
		for i, c := range compute {
			var pool *hivev1.MachinePool
			for _, p := range *machinePools {
				if c.Name == p.Spec.Name {
					pool = p
					break
				}
			}
			if pool == nil {
				pool = &hivev1.MachinePool{}
				pool.Name = fmt.Sprintf("%s-%s", in.Name, c.Name)
				pool.Spec.ClusterDeploymentRef.Name = in.Name
			}
			machinePoolToHiveV1(&c, &pool.Spec)
			newMachinePools[i] = pool
		}
		*machinePools = newMachinePools
	} else {
		*machinePools = nil
	}

	return nil
}

// ClusterDeploymentFromHiveV1 turns a v1 ClusterDeployment into a v1alpha1 ClusterDeployment.
func ClusterDeploymentFromHiveV1(in *hivev1.ClusterDeployment, installConfig *installtypes.InstallConfig, machinePools []*hivev1.MachinePool, installConfigResourceVersion string, out *hiveapi.ClusterDeployment) error {
	in = in.DeepCopy()

	if err := hiveconversion.Convert_v1_ClusterDeployment_To_v1alpha1_ClusterDeployment(in, out, nil); err != nil {
		return err
	}

	resourceVersions := make([]string, 2+len(machinePools))
	resourceVersions[0] = in.ResourceVersion
	resourceVersions[1] = installConfigResourceVersion
	for i, pool := range machinePools {
		resourceVersions[i+2] = pool.ResourceVersion
	}
	out.ResourceVersion = strings.Join(resourceVersions, "-")

	if installConfig != nil {
		if networking := installConfig.Networking; networking != nil {
			if cidr := networking.MachineCIDR; cidr != nil {
				out.Spec.MachineCIDR = cidr.String()
			} else {
				out.Spec.MachineCIDR = ""
			}
			out.Spec.Type = hiveapi.NetworkType(networking.NetworkType)
			if len(networking.ServiceNetwork) > 0 {
				out.Spec.ServiceCIDR = networking.ServiceNetwork[0].String()
			} else {
				out.Spec.ServiceCIDR = ""
			}
			if nets := networking.ClusterNetwork; nets != nil {
				out.Spec.ClusterNetworks = make([]netopv1.ClusterNetwork, len(nets))
				for i, inNet := range nets {
					outNet := &out.Spec.ClusterNetworks[i]
					outNet.CIDR = inNet.CIDR.String()
					// In v1alpha1, Hive is mis-interpreting the host subnet length as the host prefix.
					outNet.HostSubnetLength = uint32(inNet.HostPrefix)
				}
			} else {
				out.Spec.ClusterNetworks = nil
			}
		} else {
			out.Spec.MachineCIDR = ""
			out.Spec.Type = ""
			out.Spec.ServiceCIDR = ""
			out.Spec.ClusterNetworks = nil
		}
		if cp := installConfig.ControlPlane; cp != nil {
			machinePoolFromInstallConfig(cp, &out.Spec.ControlPlane)
		} else {
			out.Spec.ControlPlane = hiveapi.MachinePool{}
		}
		if compute := installConfig.Compute; compute != nil {
			out.Spec.Compute = make([]hiveapi.MachinePool, len(compute))
			for i, c := range compute {
				machinePoolFromInstallConfig(&c, &out.Spec.Compute[i])
			}
		} else {
			out.Spec.Compute = nil
		}
		if inAWS := installConfig.AWS; inAWS != nil {
			if out.Spec.AWS == nil {
				out.Spec.AWS = &hiveapiaws.Platform{}
			}
			outAWS := out.Spec.AWS
			outAWS.Region = inAWS.Region
			outAWS.UserTags = inAWS.UserTags
			if p := inAWS.DefaultMachinePlatform; p != nil {
				if outAWS.DefaultMachinePlatform == nil {
					outAWS.DefaultMachinePlatform = &hiveapiaws.MachinePoolPlatform{}
				}
				awsMachinePoolFromInstallConfig(p, outAWS.DefaultMachinePlatform)
			} else {
				outAWS.DefaultMachinePlatform = nil
			}
		} else {
			out.Spec.AWS = nil
		}
		if inAzure := installConfig.Azure; inAzure != nil {
			if out.Spec.Azure == nil {
				out.Spec.Azure = &hiveapiazure.Platform{}
			}
			outAzure := out.Spec.Azure
			outAzure.Region = inAzure.Region
			outAzure.BaseDomainResourceGroupName = inAzure.BaseDomainResourceGroupName
			if p := inAzure.DefaultMachinePlatform; p != nil {
				if outAzure.DefaultMachinePlatform == nil {
					outAzure.DefaultMachinePlatform = &hiveapiazure.MachinePool{}
				}
				azureMachinePoolFromInstallConfig(p, outAzure.DefaultMachinePlatform)
			} else {
				outAzure.DefaultMachinePlatform = nil
			}
		} else {
			out.Spec.Azure = nil
		}
		if inGCP := installConfig.GCP; inGCP != nil {
			if out.Spec.GCP == nil {
				out.Spec.GCP = &hiveapigcp.Platform{}
			}
			outGCP := out.Spec.GCP
			outGCP.ProjectID = inGCP.ProjectID
			outGCP.Region = inGCP.Region
			if p := inGCP.DefaultMachinePlatform; p != nil {
				if outGCP.DefaultMachinePlatform == nil {
					outGCP.DefaultMachinePlatform = &hiveapigcp.MachinePool{}
				}
				gcpMachinePoolFromInstallConfig(p, outGCP.DefaultMachinePlatform)
			} else {
				outGCP.DefaultMachinePlatform = nil
			}
		} else {
			out.Spec.GCP = nil
		}
	} else {
		out.Spec.MachineCIDR = ""
		out.Spec.Type = ""
		out.Spec.ServiceCIDR = ""
		out.Spec.ClusterNetworks = nil
		out.Spec.ControlPlane = hiveapi.MachinePool{}
		out.Spec.Compute = nil
		out.Spec.AWS = nil
		out.Spec.Azure = nil
		out.Spec.GCP = nil
	}

	if machinePools != nil {
		newCompute := make([]hiveapi.MachinePool, len(machinePools))
		for i, p := range machinePools {
			for _, c := range out.Spec.Compute {
				if p.Spec.Name == c.Name {
					newCompute[i] = c
					break
				}
			}
			machinePoolFromHiveV1(&p.Spec, &newCompute[i])
		}
		out.Spec.Compute = newCompute
	} else {
		out.Spec.Compute = nil
	}

	return nil
}

// ClusterDeprovisionRequestToHiveV1 turns a v1alpha1 ClusterDeprovisionRequest into a v1 ClusterDeprovisionRequest.
func ClusterDeprovisionRequestToHiveV1(in *hiveapi.ClusterDeprovisionRequest, out *hivev1.ClusterDeprovision) error {
	return hiveconversion.Convert_v1alpha1_ClusterDeprovisionRequest_To_v1_ClusterDeprovision(in.DeepCopy(), out, nil)
}

// ClusterDeprovisionRequestFromHiveV1 turns a v1 ClusterDeprovisionRequest into a v1alpha1 ClusterDeprovisionRequest.
func ClusterDeprovisionRequestFromHiveV1(in *hivev1.ClusterDeprovision, out *hiveapi.ClusterDeprovisionRequest) error {
	return hiveconversion.Convert_v1_ClusterDeprovision_To_v1alpha1_ClusterDeprovisionRequest(in.DeepCopy(), out, nil)
}

// ClusterImageSetToHiveV1 turns a v1alpha1 ClusterImageSet into a v1 ClusterImageSet.
func ClusterImageSetToHiveV1(in *hiveapi.ClusterImageSet, out *hivev1.ClusterImageSet) error {
	return hiveconversion.Convert_v1alpha1_ClusterImageSet_To_v1_ClusterImageSet(in.DeepCopy(), out, nil)
}

// ClusterImageSetFromHiveV1 turns a v1 ClusterImageSet into a v1alpha1 ClusterImageSet.
func ClusterImageSetFromHiveV1(in *hivev1.ClusterImageSet, out *hiveapi.ClusterImageSet) error {
	return hiveconversion.Convert_v1_ClusterImageSet_To_v1alpha1_ClusterImageSet(in.DeepCopy(), out, nil)
}

// ClusterProvisionToHiveV1 turns a v1alpha1 ClusterProvision into a v1 ClusterProvision.
func ClusterProvisionToHiveV1(in *hiveapi.ClusterProvision, out *hivev1.ClusterProvision) error {
	return hiveconversion.Convert_v1alpha1_ClusterProvision_To_v1_ClusterProvision(in.DeepCopy(), out, nil)
}

// ClusterProvisionFromHiveV1 turns a v1 ClusterProvision into a v1alpha1 ClusterProvision.
func ClusterProvisionFromHiveV1(in *hivev1.ClusterProvision, out *hiveapi.ClusterProvision) error {
	return hiveconversion.Convert_v1_ClusterProvision_To_v1alpha1_ClusterProvision(in.DeepCopy(), out, nil)
}

// ClusterStateToHiveV1 turns a v1alpha1 ClusterState into a v1 ClusterState.
func ClusterStateToHiveV1(in *hiveapi.ClusterState, out *hivev1.ClusterState) error {
	return hiveconversion.Convert_v1alpha1_ClusterState_To_v1_ClusterState(in.DeepCopy(), out, nil)
}

// ClusterStateFromHiveV1 turns a v1 ClusterState into a v1alpha1 ClusterState.
func ClusterStateFromHiveV1(in *hivev1.ClusterState, out *hiveapi.ClusterState) error {
	return hiveconversion.Convert_v1_ClusterState_To_v1alpha1_ClusterState(in.DeepCopy(), out, nil)
}

// DNSZoneToHiveV1 turns a v1alpha1 DNSZone into a v1 DNSZone.
func DNSZoneToHiveV1(in *hiveapi.DNSZone, out *hivev1.DNSZone) error {
	return hiveconversion.Convert_v1alpha1_DNSZone_To_v1_DNSZone(in.DeepCopy(), out, nil)
}

// DNSZoneFromHiveV1 turns a v1 DNSZone into a v1alpha1 DNSZone.
func DNSZoneFromHiveV1(in *hivev1.DNSZone, out *hiveapi.DNSZone) error {
	return hiveconversion.Convert_v1_DNSZone_To_v1alpha1_DNSZone(in.DeepCopy(), out, nil)
}

// HiveConfigToHiveV1 turns a v1alpha1 HiveConfig into a v1 HiveConfig.
func HiveConfigToHiveV1(in *hiveapi.HiveConfig, out *hivev1.HiveConfig) error {
	return hiveconversion.Convert_v1alpha1_HiveConfig_To_v1_HiveConfig(in.DeepCopy(), out, nil)
}

// HiveConfigFromHiveV1 turns a v1 HiveConfig into a v1alpha1 HiveConfig.
func HiveConfigFromHiveV1(in *hivev1.HiveConfig, out *hiveapi.HiveConfig) error {
	return hiveconversion.Convert_v1_HiveConfig_To_v1alpha1_HiveConfig(in.DeepCopy(), out, nil)
}

// SyncIdentityProviderToHiveV1 turns a v1alpha1 SyncIdentityProvider into a v1 SyncIdentityProvider.
func SyncIdentityProviderToHiveV1(in *hiveapi.SyncIdentityProvider, out *hivev1.SyncIdentityProvider) error {
	return hiveconversion.Convert_v1alpha1_SyncIdentityProvider_To_v1_SyncIdentityProvider(in.DeepCopy(), out, nil)
}

// SyncIdentityProviderFromHiveV1 turns a v1 SyncIdentityProvider into a v1alpha1 SyncIdentityProvider.
func SyncIdentityProviderFromHiveV1(in *hivev1.SyncIdentityProvider, out *hiveapi.SyncIdentityProvider) error {
	return hiveconversion.Convert_v1_SyncIdentityProvider_To_v1alpha1_SyncIdentityProvider(in.DeepCopy(), out, nil)
}

// SelectorSyncIdentityProviderToHiveV1 turns a v1alpha1 SelectorSyncIdentityProvider into a v1 SelectorSyncIdentityProvider.
func SelectorSyncIdentityProviderToHiveV1(in *hiveapi.SelectorSyncIdentityProvider, out *hivev1.SelectorSyncIdentityProvider) error {
	return hiveconversion.Convert_v1alpha1_SelectorSyncIdentityProvider_To_v1_SelectorSyncIdentityProvider(in.DeepCopy(), out, nil)
}

// SelectorSyncIdentityProviderFromHiveV1 turns a v1 SelectorSyncIdentityProvider into a v1alpha1 SelectorSyncIdentityProvider.
func SelectorSyncIdentityProviderFromHiveV1(in *hivev1.SelectorSyncIdentityProvider, out *hiveapi.SelectorSyncIdentityProvider) error {
	return hiveconversion.Convert_v1_SelectorSyncIdentityProvider_To_v1alpha1_SelectorSyncIdentityProvider(in.DeepCopy(), out, nil)
}

// SyncSetToHiveV1 turns a v1alpha1 SyncSet into a v1 SyncSet.
func SyncSetToHiveV1(in *hiveapi.SyncSet, out *hivev1.SyncSet) error {
	return hiveconversion.Convert_v1alpha1_SyncSet_To_v1_SyncSet(in.DeepCopy(), out, nil)
}

// SyncSetFromHiveV1 turns a v1 SyncSet into a v1alpha1 SyncSet.
func SyncSetFromHiveV1(in *hivev1.SyncSet, out *hiveapi.SyncSet) error {
	return hiveconversion.Convert_v1_SyncSet_To_v1alpha1_SyncSet(in.DeepCopy(), out, nil)
}

// SelectorSyncSetToHiveV1 turns a v1alpha1 SelectorSyncSet into a v1 SelectorSyncSet.
func SelectorSyncSetToHiveV1(in *hiveapi.SelectorSyncSet, out *hivev1.SelectorSyncSet) error {
	return hiveconversion.Convert_v1alpha1_SelectorSyncSet_To_v1_SelectorSyncSet(in.DeepCopy(), out, nil)
}

// SelectorSyncSetFromHiveV1 turns a v1 SelectorSyncSet into a v1alpha1 SelectorSyncSet.
func SelectorSyncSetFromHiveV1(in *hivev1.SelectorSyncSet, out *hiveapi.SelectorSyncSet) error {
	return hiveconversion.Convert_v1_SelectorSyncSet_To_v1alpha1_SelectorSyncSet(in.DeepCopy(), out, nil)
}

// SyncSetInstanceToHiveV1 turns a v1alpha1 SyncSetInstance into a v1 SyncSetInstance.
func SyncSetInstanceToHiveV1(in *hiveapi.SyncSetInstance, out *hivev1.SyncSetInstance) error {
	return hiveconversion.Convert_v1alpha1_SyncSetInstance_To_v1_SyncSetInstance(in.DeepCopy(), out, nil)
}

// SyncSetInstanceFromHiveV1 turns a v1 SyncSetInstance into a v1alpha1 SyncSetInstance.
func SyncSetInstanceFromHiveV1(in *hivev1.SyncSetInstance, out *hiveapi.SyncSetInstance) error {
	return hiveconversion.Convert_v1_SyncSetInstance_To_v1alpha1_SyncSetInstance(in.DeepCopy(), out, nil)
}

func machinePoolFromInstallConfig(in *installtypes.MachinePool, out *hiveapi.MachinePool) {
	out.Name = in.Name
	out.Replicas = in.Replicas
	if aws := in.Platform.AWS; aws != nil {
		if out.Platform.AWS == nil {
			out.Platform.AWS = &hiveapiaws.MachinePoolPlatform{}
		}
		awsMachinePoolFromInstallConfig(aws, out.Platform.AWS)
	} else {
		out.Platform.AWS = nil
	}
	if azure := in.Platform.Azure; azure != nil {
		if out.Platform.Azure == nil {
			out.Platform.Azure = &hiveapiazure.MachinePool{}
		}
		azureMachinePoolFromInstallConfig(azure, out.Platform.Azure)
	} else {
		out.Platform.Azure = nil
	}
	if gcp := in.Platform.GCP; gcp != nil {
		if out.Platform.GCP == nil {
			out.Platform.GCP = &hiveapigcp.MachinePool{}
		}
		gcpMachinePoolFromInstallConfig(gcp, out.Platform.GCP)
	} else {
		out.Platform.GCP = nil
	}
}

func machinePoolToInstallConfig(in *hiveapi.MachinePool, out *installtypes.MachinePool) {
	out.Name = in.Name
	out.Replicas = in.Replicas
	if aws := in.Platform.AWS; aws != nil {
		if out.Platform.AWS == nil {
			out.Platform.AWS = &installtypesaws.MachinePool{}
		}
		awsMachinePoolToInstallConfig(aws, out.Platform.AWS)
	} else {
		out.Platform.AWS = nil
	}
	if azure := in.Platform.Azure; azure != nil {
		if out.Platform.Azure == nil {
			out.Platform.Azure = &installtypesazure.MachinePool{}
		}
		azureMachinePoolToInstallConfig(azure, out.Platform.Azure)
	} else {
		out.Platform.Azure = nil
	}
	if gcp := in.Platform.GCP; gcp != nil {
		if out.Platform.GCP == nil {
			out.Platform.GCP = &installtypesgcp.MachinePool{}
		}
		gcpMachinePoolToInstallConfig(gcp, out.Platform.GCP)
	} else {
		out.Platform.GCP = nil
	}
}

func awsMachinePoolFromInstallConfig(in *installtypesaws.MachinePool, out *hiveapiaws.MachinePoolPlatform) {
	out.Zones = in.Zones
	out.InstanceType = in.InstanceType
	out.IOPS = in.IOPS
	out.Size = in.Size
	out.Type = in.Type
}

func awsMachinePoolToInstallConfig(in *hiveapiaws.MachinePoolPlatform, out *installtypesaws.MachinePool) {
	out.Zones = in.Zones
	out.InstanceType = in.InstanceType
	out.IOPS = in.IOPS
	out.Size = in.Size
	out.Type = in.Type
}

func azureMachinePoolFromInstallConfig(in *installtypesazure.MachinePool, out *hiveapiazure.MachinePool) {
	out.Zones = in.Zones
	out.InstanceType = in.InstanceType
	out.DiskSizeGB = in.DiskSizeGB
}

func azureMachinePoolToInstallConfig(in *hiveapiazure.MachinePool, out *installtypesazure.MachinePool) {
	out.Zones = in.Zones
	out.InstanceType = in.InstanceType
	out.DiskSizeGB = in.DiskSizeGB
}

func gcpMachinePoolFromInstallConfig(in *installtypesgcp.MachinePool, out *hiveapigcp.MachinePool) {
	out.Zones = in.Zones
	out.InstanceType = in.InstanceType
}

func gcpMachinePoolToInstallConfig(in *hiveapigcp.MachinePool, out *installtypesgcp.MachinePool) {
	out.Zones = in.Zones
	out.InstanceType = in.InstanceType
}

func machinePoolFromHiveV1(in *hivev1.MachinePoolSpec, out *hiveapi.MachinePool) {
	out.Name = in.Name
	out.Replicas = in.Replicas
	if inAWS := in.Platform.AWS; inAWS != nil {
		if out.Platform.AWS == nil {
			out.Platform.AWS = &hiveapiaws.MachinePoolPlatform{}
		}
		outAWS := out.Platform.AWS
		outAWS.Zones = inAWS.Zones
		outAWS.InstanceType = inAWS.InstanceType
		outAWS.IOPS = inAWS.IOPS
		outAWS.Size = inAWS.Size
		outAWS.Type = inAWS.Type
	} else {
		out.Platform.AWS = nil
	}
	if inAzure := in.Platform.Azure; inAzure != nil {
		if out.Platform.Azure == nil {
			out.Platform.Azure = &hiveapiazure.MachinePool{}
		}
		outAzure := out.Platform.Azure
		outAzure.Zones = inAzure.Zones
		outAzure.InstanceType = inAzure.InstanceType
		outAzure.DiskSizeGB = inAzure.DiskSizeGB
	} else {
		out.Platform.Azure = nil
	}
	if inGCP := in.Platform.GCP; inGCP != nil {
		if out.Platform.GCP == nil {
			out.Platform.GCP = &hiveapigcp.MachinePool{}
		}
		outGCP := out.Platform.GCP
		outGCP.Zones = inGCP.Zones
		outGCP.InstanceType = inGCP.InstanceType
	} else {
		out.Platform.GCP = nil
	}
	out.Labels = in.Labels
	out.Taints = in.Taints
}

func machinePoolToHiveV1(in *hiveapi.MachinePool, out *hivev1.MachinePoolSpec) {
	out.Name = in.Name
	out.Replicas = in.Replicas
	if out.Replicas != nil {
		out.Autoscaling = nil
	}
	if inAWS := in.Platform.AWS; inAWS != nil {
		if out.Platform.AWS == nil {
			out.Platform.AWS = &hivev1aws.MachinePoolPlatform{}
		}
		outAWS := out.Platform.AWS
		outAWS.Zones = inAWS.Zones
		outAWS.InstanceType = inAWS.InstanceType
		outAWS.IOPS = inAWS.IOPS
		outAWS.Size = inAWS.Size
		outAWS.Type = inAWS.Type
	} else {
		out.Platform.AWS = nil
	}
	if inAzure := in.Platform.Azure; inAzure != nil {
		if out.Platform.Azure == nil {
			out.Platform.Azure = &hivev1azure.MachinePool{}
		}
		outAzure := out.Platform.Azure
		outAzure.Zones = inAzure.Zones
		outAzure.InstanceType = inAzure.InstanceType
		outAzure.DiskSizeGB = inAzure.DiskSizeGB
	} else {
		out.Platform.Azure = nil
	}
	if inGCP := in.Platform.GCP; inGCP != nil {
		if out.Platform.GCP == nil {
			out.Platform.GCP = &hivev1gcp.MachinePool{}
		}
		outGCP := out.Platform.GCP
		outGCP.Zones = inGCP.Zones
		outGCP.InstanceType = inGCP.InstanceType
	} else {
		out.Platform.GCP = nil
	}
	out.Labels = in.Labels
	out.Taints = in.Taints
}

func parseCIDR(s string) *ipnet.IPNet {
	if s == "" {
		return &ipnet.IPNet{}
	}
	return ipnet.MustParseCIDR(s)
}
