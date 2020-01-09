package util

import (
	netopv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/installer/pkg/ipnet"
	installtypes "github.com/openshift/installer/pkg/types"
	installtypesaws "github.com/openshift/installer/pkg/types/aws"
	installtypesazure "github.com/openshift/installer/pkg/types/azure"
	installtypesgcp "github.com/openshift/installer/pkg/types/gcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
	hiveapiaws "github.com/openshift/hive/pkg/hive/apis/hive/aws"
	hiveapiazure "github.com/openshift/hive/pkg/hive/apis/hive/azure"
	hiveapigcp "github.com/openshift/hive/pkg/hive/apis/hive/gcp"
	"github.com/openshift/hive/pkg/hive/apis/hive/hiveconversion"
)

// CheckpointToHiveV1 turns a v1alpha1 Checkpoint into a v1 Checkpoint.
// The returned object is safe to mutate.
func CheckpointToHiveV1(obj *hiveapi.Checkpoint) (*hivev1.Checkpoint, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.Checkpoint{}
	if err := hiveconversion.Convert_v1alpha1_Checkpoint_To_v1_Checkpoint(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// CheckpointFromHiveV1 turns a v1 Checkpoint into a v1alpha1 Checkpoint.
// The returned object is safe to mutate.
func CheckpointFromHiveV1(obj *hivev1.Checkpoint) (*hiveapi.Checkpoint, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.Checkpoint{}
	if err := hiveconversion.Convert_v1_Checkpoint_To_v1alpha1_Checkpoint(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// ClusterDeploymentToHiveV1 turns a v1alpha1 ClusterDeployment into a v1 ClusterDeployment.
// The returned object is safe to mutate.
func ClusterDeploymentToHiveV1(obj *hiveapi.ClusterDeployment, sshKey string) (*hivev1.ClusterDeployment, *installtypes.InstallConfig, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.ClusterDeployment{}
	if err := hiveconversion.Convert_v1alpha1_ClusterDeployment_To_v1_ClusterDeployment(objCopy, convertedObj, nil); err != nil {
		return nil, nil, err
	}

	installConfig := &installtypes.InstallConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: objCopy.Spec.ClusterName,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: installtypes.InstallConfigVersion,
		},
		SSHKey:     sshKey,
		BaseDomain: objCopy.Spec.BaseDomain,
		Networking: &installtypes.Networking{
			NetworkType:    string(objCopy.Spec.Networking.Type),
			ServiceNetwork: []ipnet.IPNet{*parseCIDR(objCopy.Spec.Networking.ServiceCIDR)},
			MachineCIDR:    parseCIDR(objCopy.Spec.Networking.MachineCIDR),
		},
		// PullSecret: <filled in by the Hive installmanager>
		ControlPlane: machinePoolToInstallConfig(&objCopy.Spec.ControlPlane),
	}
	installConfig.ClusterNetwork = make([]installtypes.ClusterNetworkEntry, len(objCopy.Spec.ClusterNetworks))
	for i, inNet := range objCopy.Spec.ClusterNetworks {
		outNet := &installConfig.ClusterNetwork[i]
		outNet.CIDR = *parseCIDR(inNet.CIDR)
		// In v1alpha1, Hive is mis-interpreting the host subnet length as the host prefix.
		outNet.HostPrefix = int32(inNet.HostSubnetLength)
	}
	if aws := objCopy.Spec.AWS; aws != nil {
		installConfig.AWS = &installtypesaws.Platform{
			Region:                 aws.Region,
			UserTags:               aws.UserTags,
			DefaultMachinePlatform: awsMachinePoolToInstallConfig(aws.DefaultMachinePlatform),
		}
	}
	if azure := objCopy.Spec.Azure; azure != nil {
		installConfig.Azure = &installtypesazure.Platform{
			Region:                      azure.Region,
			BaseDomainResourceGroupName: azure.BaseDomainResourceGroupName,
			DefaultMachinePlatform:      azureMachinePoolToInstallConfig(azure.DefaultMachinePlatform),
		}
	}
	if gcp := objCopy.Spec.GCP; gcp != nil {
		installConfig.GCP = &installtypesgcp.Platform{
			ProjectID:              gcp.ProjectID,
			Region:                 gcp.Region,
			DefaultMachinePlatform: gcpMachinePoolToInstallConfig(gcp.DefaultMachinePlatform),
		}
	}
	installConfig.ControlPlane.Name = "master"
	for _, compute := range objCopy.Spec.Compute {
		if compute.Name != "worker" {
			continue
		}
		installConfig.Compute = []installtypes.MachinePool{
			*machinePoolToInstallConfig(&compute),
		}
	}

	return convertedObj, installConfig, nil
}

// ClusterDeploymentFromHiveV1 turns a v1 ClusterDeployment into a v1alpha1 ClusterDeployment.
// The returned object is safe to mutate.
func ClusterDeploymentFromHiveV1(obj *hivev1.ClusterDeployment, installConfig *installtypes.InstallConfig) (*hiveapi.ClusterDeployment, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.ClusterDeployment{}
	if err := hiveconversion.Convert_v1_ClusterDeployment_To_v1alpha1_ClusterDeployment(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	if installConfig != nil {
		if networking := installConfig.Networking; networking != nil {
			if cidr := networking.MachineCIDR; cidr != nil {
				convertedObj.Spec.MachineCIDR = cidr.String()
			}
			convertedObj.Spec.Type = hiveapi.NetworkType(networking.NetworkType)
			if len(networking.ServiceNetwork) > 0 {
				convertedObj.Spec.ServiceCIDR = networking.ServiceNetwork[0].String()
			}
			convertedObj.Spec.ClusterNetworks = make([]netopv1.ClusterNetwork, len(networking.ClusterNetwork))
			for i, inNet := range networking.ClusterNetwork {
				outNet := &convertedObj.Spec.ClusterNetworks[i]
				outNet.CIDR = inNet.CIDR.String()
				// In v1alpha1, Hive is mis-interpreting the host subnet length as the host prefix.
				outNet.HostSubnetLength = uint32(inNet.HostPrefix)
			}
		}
		if cp := installConfig.ControlPlane; cp != nil {
			convertedObj.Spec.ControlPlane = *machinePoolFromInstallConfig(cp)
		}
		convertedObj.Spec.Compute = make([]hiveapi.MachinePool, len(installConfig.Compute))
		for i, c := range installConfig.Compute {
			convertedObj.Spec.Compute[i] = *machinePoolFromInstallConfig(&c)
		}
		if aws := installConfig.AWS; aws != nil {
			convertedObj.Spec.AWS = &hiveapiaws.Platform{
				Region:                 aws.Region,
				UserTags:               aws.UserTags,
				DefaultMachinePlatform: awsMachinePoolFromInstallConfig(aws.DefaultMachinePlatform),
			}
		}
		if azure := installConfig.Azure; azure != nil {
			convertedObj.Spec.Azure = &hiveapiazure.Platform{
				Region:                      azure.Region,
				BaseDomainResourceGroupName: azure.BaseDomainResourceGroupName,
				DefaultMachinePlatform:      azureMachinePoolFromInstallConfig(azure.DefaultMachinePlatform),
			}
		}
		if gcp := installConfig.GCP; gcp != nil {
			convertedObj.Spec.GCP = &hiveapigcp.Platform{
				ProjectID:              gcp.ProjectID,
				Region:                 gcp.Region,
				DefaultMachinePlatform: gcpMachinePoolFromInstallConfig(gcp.DefaultMachinePlatform),
			}
		}
	}

	return convertedObj, nil
}

// ClusterDeprovisionRequestToHiveV1 turns a v1alpha1 ClusterDeprovisionRequest into a v1 ClusterDeprovisionRequest.
// The returned object is safe to mutate.
func ClusterDeprovisionRequestToHiveV1(obj *hiveapi.ClusterDeprovisionRequest) (*hivev1.ClusterDeprovision, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.ClusterDeprovision{}
	if err := hiveconversion.Convert_v1alpha1_ClusterDeprovisionRequest_To_v1_ClusterDeprovision(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// ClusterDeprovisionRequestFromHiveV1 turns a v1 ClusterDeprovisionRequest into a v1alpha1 ClusterDeprovisionRequest.
// The returned object is safe to mutate.
func ClusterDeprovisionRequestFromHiveV1(obj *hivev1.ClusterDeprovision) (*hiveapi.ClusterDeprovisionRequest, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.ClusterDeprovisionRequest{}
	if err := hiveconversion.Convert_v1_ClusterDeprovision_To_v1alpha1_ClusterDeprovisionRequest(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// ClusterImageSetToHiveV1 turns a v1alpha1 ClusterImageSet into a v1 ClusterImageSet.
// The returned object is safe to mutate.
func ClusterImageSetToHiveV1(obj *hiveapi.ClusterImageSet) (*hivev1.ClusterImageSet, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.ClusterImageSet{}
	if err := hiveconversion.Convert_v1alpha1_ClusterImageSet_To_v1_ClusterImageSet(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// ClusterImageSetFromHiveV1 turns a v1 ClusterImageSet into a v1alpha1 ClusterImageSet.
// The returned object is safe to mutate.
func ClusterImageSetFromHiveV1(obj *hivev1.ClusterImageSet) (*hiveapi.ClusterImageSet, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.ClusterImageSet{}
	if err := hiveconversion.Convert_v1_ClusterImageSet_To_v1alpha1_ClusterImageSet(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// ClusterProvisionToHiveV1 turns a v1alpha1 ClusterProvision into a v1 ClusterProvision.
// The returned object is safe to mutate.
func ClusterProvisionToHiveV1(obj *hiveapi.ClusterProvision) (*hivev1.ClusterProvision, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.ClusterProvision{}
	if err := hiveconversion.Convert_v1alpha1_ClusterProvision_To_v1_ClusterProvision(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// ClusterProvisionFromHiveV1 turns a v1 ClusterProvision into a v1alpha1 ClusterProvision.
// The returned object is safe to mutate.
func ClusterProvisionFromHiveV1(obj *hivev1.ClusterProvision) (*hiveapi.ClusterProvision, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.ClusterProvision{}
	if err := hiveconversion.Convert_v1_ClusterProvision_To_v1alpha1_ClusterProvision(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// ClusterStateToHiveV1 turns a v1alpha1 ClusterState into a v1 ClusterState.
// The returned object is safe to mutate.
func ClusterStateToHiveV1(obj *hiveapi.ClusterState) (*hivev1.ClusterState, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.ClusterState{}
	if err := hiveconversion.Convert_v1alpha1_ClusterState_To_v1_ClusterState(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// ClusterStateFromHiveV1 turns a v1 ClusterState into a v1alpha1 ClusterState.
// The returned object is safe to mutate.
func ClusterStateFromHiveV1(obj *hivev1.ClusterState) (*hiveapi.ClusterState, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.ClusterState{}
	if err := hiveconversion.Convert_v1_ClusterState_To_v1alpha1_ClusterState(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// DNSZoneToHiveV1 turns a v1alpha1 DNSZone into a v1 DNSZone.
// The returned object is safe to mutate.
func DNSZoneToHiveV1(obj *hiveapi.DNSZone) (*hivev1.DNSZone, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.DNSZone{}
	if err := hiveconversion.Convert_v1alpha1_DNSZone_To_v1_DNSZone(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// DNSZoneFromHiveV1 turns a v1 DNSZone into a v1alpha1 DNSZone.
// The returned object is safe to mutate.
func DNSZoneFromHiveV1(obj *hivev1.DNSZone) (*hiveapi.DNSZone, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.DNSZone{}
	if err := hiveconversion.Convert_v1_DNSZone_To_v1alpha1_DNSZone(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// HiveConfigToHiveV1 turns a v1alpha1 HiveConfig into a v1 HiveConfig.
// The returned object is safe to mutate.
func HiveConfigToHiveV1(obj *hiveapi.HiveConfig) (*hivev1.HiveConfig, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.HiveConfig{}
	if err := hiveconversion.Convert_v1alpha1_HiveConfig_To_v1_HiveConfig(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// HiveConfigFromHiveV1 turns a v1 HiveConfig into a v1alpha1 HiveConfig.
// The returned object is safe to mutate.
func HiveConfigFromHiveV1(obj *hivev1.HiveConfig) (*hiveapi.HiveConfig, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.HiveConfig{}
	if err := hiveconversion.Convert_v1_HiveConfig_To_v1alpha1_HiveConfig(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// SyncIdentityProviderToHiveV1 turns a v1alpha1 SyncIdentityProvider into a v1 SyncIdentityProvider.
// The returned object is safe to mutate.
func SyncIdentityProviderToHiveV1(obj *hiveapi.SyncIdentityProvider) (*hivev1.SyncIdentityProvider, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.SyncIdentityProvider{}
	if err := hiveconversion.Convert_v1alpha1_SyncIdentityProvider_To_v1_SyncIdentityProvider(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// SyncIdentityProviderFromHiveV1 turns a v1 SyncIdentityProvider into a v1alpha1 SyncIdentityProvider.
// The returned object is safe to mutate.
func SyncIdentityProviderFromHiveV1(obj *hivev1.SyncIdentityProvider) (*hiveapi.SyncIdentityProvider, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.SyncIdentityProvider{}
	if err := hiveconversion.Convert_v1_SyncIdentityProvider_To_v1alpha1_SyncIdentityProvider(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// SelectorSyncIdentityProviderToHiveV1 turns a v1alpha1 SelectorSyncIdentityProvider into a v1 SelectorSyncIdentityProvider.
// The returned object is safe to mutate.
func SelectorSyncIdentityProviderToHiveV1(obj *hiveapi.SelectorSyncIdentityProvider) (*hivev1.SelectorSyncIdentityProvider, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.SelectorSyncIdentityProvider{}
	if err := hiveconversion.Convert_v1alpha1_SelectorSyncIdentityProvider_To_v1_SelectorSyncIdentityProvider(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// SelectorSyncIdentityProviderFromHiveV1 turns a v1 SelectorSyncIdentityProvider into a v1alpha1 SelectorSyncIdentityProvider.
// The returned object is safe to mutate.
func SelectorSyncIdentityProviderFromHiveV1(obj *hivev1.SelectorSyncIdentityProvider) (*hiveapi.SelectorSyncIdentityProvider, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.SelectorSyncIdentityProvider{}
	if err := hiveconversion.Convert_v1_SelectorSyncIdentityProvider_To_v1alpha1_SelectorSyncIdentityProvider(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// SyncSetToHiveV1 turns a v1alpha1 SyncSet into a v1 SyncSet.
// The returned object is safe to mutate.
func SyncSetToHiveV1(obj *hiveapi.SyncSet) (*hivev1.SyncSet, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.SyncSet{}
	if err := hiveconversion.Convert_v1alpha1_SyncSet_To_v1_SyncSet(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// SyncSetFromHiveV1 turns a v1 SyncSet into a v1alpha1 SyncSet.
// The returned object is safe to mutate.
func SyncSetFromHiveV1(obj *hivev1.SyncSet) (*hiveapi.SyncSet, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.SyncSet{}
	if err := hiveconversion.Convert_v1_SyncSet_To_v1alpha1_SyncSet(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// SelectorSyncSetToHiveV1 turns a v1alpha1 SelectorSyncSet into a v1 SelectorSyncSet.
// The returned object is safe to mutate.
func SelectorSyncSetToHiveV1(obj *hiveapi.SelectorSyncSet) (*hivev1.SelectorSyncSet, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.SelectorSyncSet{}
	if err := hiveconversion.Convert_v1alpha1_SelectorSyncSet_To_v1_SelectorSyncSet(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// SelectorSyncSetFromHiveV1 turns a v1 SelectorSyncSet into a v1alpha1 SelectorSyncSet.
// The returned object is safe to mutate.
func SelectorSyncSetFromHiveV1(obj *hivev1.SelectorSyncSet) (*hiveapi.SelectorSyncSet, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.SelectorSyncSet{}
	if err := hiveconversion.Convert_v1_SelectorSyncSet_To_v1alpha1_SelectorSyncSet(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// SyncSetInstanceToHiveV1 turns a v1alpha1 SyncSetInstance into a v1 SyncSetInstance.
// The returned object is safe to mutate.
func SyncSetInstanceToHiveV1(obj *hiveapi.SyncSetInstance) (*hivev1.SyncSetInstance, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hivev1.SyncSetInstance{}
	if err := hiveconversion.Convert_v1alpha1_SyncSetInstance_To_v1_SyncSetInstance(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

// SyncSetInstanceFromHiveV1 turns a v1 SyncSetInstance into a v1alpha1 SyncSetInstance.
// The returned object is safe to mutate.
func SyncSetInstanceFromHiveV1(obj *hivev1.SyncSetInstance) (*hiveapi.SyncSetInstance, error) {
	// do a deep copy here since conversion does not guarantee a new object.
	objCopy := obj.DeepCopy()

	convertedObj := &hiveapi.SyncSetInstance{}
	if err := hiveconversion.Convert_v1_SyncSetInstance_To_v1alpha1_SyncSetInstance(objCopy, convertedObj, nil); err != nil {
		return nil, err
	}

	return convertedObj, nil
}

func machinePoolFromInstallConfig(pool *installtypes.MachinePool) *hiveapi.MachinePool {
	return &hiveapi.MachinePool{
		Name:     pool.Name,
		Replicas: pool.Replicas,
		Platform: hiveapi.MachinePoolPlatform{
			AWS:   awsMachinePoolFromInstallConfig(pool.Platform.AWS),
			Azure: azureMachinePoolFromInstallConfig(pool.Platform.Azure),
			GCP:   gcpMachinePoolFromInstallConfig(pool.Platform.GCP),
		},
	}
}

func machinePoolToInstallConfig(pool *hiveapi.MachinePool) *installtypes.MachinePool {
	return &installtypes.MachinePool{
		Name:     pool.Name,
		Replicas: pool.Replicas,
		Platform: installtypes.MachinePoolPlatform{
			AWS:   awsMachinePoolToInstallConfig(pool.Platform.AWS),
			Azure: azureMachinePoolToInstallConfig(pool.Platform.Azure),
			GCP:   gcpMachinePoolToInstallConfig(pool.Platform.GCP),
		},
	}
}

func awsMachinePoolFromInstallConfig(p *installtypesaws.MachinePool) *hiveapiaws.MachinePoolPlatform {
	if p == nil {
		return nil
	}
	return &hiveapiaws.MachinePoolPlatform{
		Zones:        p.Zones,
		InstanceType: p.InstanceType,
		EC2RootVolume: hiveapiaws.EC2RootVolume{
			IOPS: p.IOPS,
			Size: p.Size,
			Type: p.Type,
		},
	}
}

func awsMachinePoolToInstallConfig(p *hiveapiaws.MachinePoolPlatform) *installtypesaws.MachinePool {
	if p == nil {
		return nil
	}
	return &installtypesaws.MachinePool{
		Zones:        p.Zones,
		InstanceType: p.InstanceType,
		EC2RootVolume: installtypesaws.EC2RootVolume{
			IOPS: p.IOPS,
			Size: p.Size,
			Type: p.Type,
		},
	}
}

func azureMachinePoolFromInstallConfig(p *installtypesazure.MachinePool) *hiveapiazure.MachinePool {
	if p == nil {
		return nil
	}
	return &hiveapiazure.MachinePool{
		Zones:        p.Zones,
		InstanceType: p.InstanceType,
		OSDisk: hiveapiazure.OSDisk{
			DiskSizeGB: p.DiskSizeGB,
		},
	}
}

func azureMachinePoolToInstallConfig(p *hiveapiazure.MachinePool) *installtypesazure.MachinePool {
	if p == nil {
		return nil
	}
	return &installtypesazure.MachinePool{
		Zones:        p.Zones,
		InstanceType: p.InstanceType,
		OSDisk: installtypesazure.OSDisk{
			DiskSizeGB: p.DiskSizeGB,
		},
	}
}

func gcpMachinePoolFromInstallConfig(p *installtypesgcp.MachinePool) *hiveapigcp.MachinePool {
	if p == nil {
		return nil
	}
	return &hiveapigcp.MachinePool{
		Zones:        p.Zones,
		InstanceType: p.InstanceType,
	}
}

func gcpMachinePoolToInstallConfig(p *hiveapigcp.MachinePool) *installtypesgcp.MachinePool {
	if p == nil {
		return nil
	}
	return &installtypesgcp.MachinePool{
		Zones:        p.Zones,
		InstanceType: p.InstanceType,
	}
}

func parseCIDR(s string) *ipnet.IPNet {
	if s == "" {
		return &ipnet.IPNet{}
	}
	return ipnet.MustParseCIDR(s)
}
