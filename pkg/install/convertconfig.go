package install

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"
	"github.com/openshift/installer/pkg/ipnet"
	"github.com/openshift/installer/pkg/types"
	installeraws "github.com/openshift/installer/pkg/types/aws"
	installerazure "github.com/openshift/installer/pkg/types/azure"
	installergcp "github.com/openshift/installer/pkg/types/gcp"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1alpha1/aws"
	hivev1azure "github.com/openshift/hive/pkg/apis/hive/v1alpha1/azure"
	hivev1gcp "github.com/openshift/hive/pkg/apis/hive/v1alpha1/gcp"
)

// GenerateInstallConfig builds an InstallConfig for the installer from our ClusterDeploymentSpec.
// The two types are extremely similar, but have different goals and in some cases deviation was required
// as ClusterDeployment is used as a CRD API.
//
// It is assumed the caller will lookup the admin password and ssh key from their respective secrets.
func GenerateInstallConfig(cd *hivev1.ClusterDeployment, sshKey, pullSecret string, generateForInstall bool) (*types.InstallConfig, error) {
	spec := cd.Spec

	platform := types.Platform{}
	switch {
	case spec.Platform.AWS != nil:
		aws := spec.Platform.AWS
		platform.AWS = &installeraws.Platform{
			Region:                 aws.Region,
			UserTags:               aws.UserTags,
			DefaultMachinePlatform: convertAWSMachinePool(aws.DefaultMachinePlatform),
		}
	case spec.Platform.Azure != nil:
		azure := spec.Platform.Azure
		platform.Azure = &installerazure.Platform{
			Region:                      azure.Region,
			BaseDomainResourceGroupName: azure.BaseDomainResourceGroupName,
			DefaultMachinePlatform:      convertAzureMachinePool(azure.DefaultMachinePlatform),
		}
	case spec.Platform.GCP != nil:
		gcp := spec.Platform.GCP
		platform.GCP = &installergcp.Platform{
			ProjectID:              gcp.ProjectID,
			Region:                 gcp.Region,
			DefaultMachinePlatform: convertGCPMachinePool(gcp.DefaultMachinePlatform),
		}
	}

	spec.ControlPlane.Name = "master"
	controlPlaneMachinePool := convertMachinePools(spec.ControlPlane)[0]
	computeMachinePools := convertMachinePools(spec.Compute...)

	if generateForInstall {
		for _, mp := range spec.Compute {
			// Only add "worker" pool when generating config for installer.
			if mp.Name == "worker" {
				computeMachinePools = convertMachinePools(mp)
			}
		}
	}

	ic := &types.InstallConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: spec.ClusterName,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: types.InstallConfigVersion,
		},
		SSHKey:     sshKey,
		BaseDomain: spec.BaseDomain,
		Networking: &types.Networking{
			// TODO: deviation from installer API here
			NetworkType: string(spec.Networking.Type),
			// TODO: deviation from installer API here
			ServiceNetwork: []ipnet.IPNet{*parseCIDR(spec.Networking.ServiceCIDR)},
			// TODO: deviation from installer API here
			ClusterNetwork: convertClusterNetworks(spec.Networking.ClusterNetworks),
			MachineCIDR:    parseCIDR(spec.Networking.MachineCIDR),
		},
		PullSecret:   pullSecret,
		Platform:     platform,
		ControlPlane: &controlPlaneMachinePool,
		Compute:      computeMachinePools,
	}
	return ic, nil
}

func convertMachinePools(pools ...hivev1.MachinePool) []types.MachinePool {
	machinePools := make([]types.MachinePool, len(pools))
	for i, mp := range pools {
		machinePools[i] = types.MachinePool{
			Name:     mp.Name,
			Replicas: mp.Replicas,
			Platform: *convertMachinePoolPlatform(&mp.Platform),
		}
	}
	return machinePools
}

func convertMachinePoolPlatform(p *hivev1.MachinePoolPlatform) *types.MachinePoolPlatform {
	return &types.MachinePoolPlatform{
		AWS:   convertAWSMachinePool(p.AWS),
		Azure: convertAzureMachinePool(p.Azure),
		GCP:   convertGCPMachinePool(p.GCP),
	}
}

func convertAWSMachinePool(p *hivev1aws.MachinePoolPlatform) *installeraws.MachinePool {
	if p == nil {
		return nil
	}
	return &installeraws.MachinePool{
		InstanceType: p.InstanceType,
		EC2RootVolume: installeraws.EC2RootVolume{
			IOPS: p.EC2RootVolume.IOPS,
			Size: p.EC2RootVolume.Size,
			Type: p.EC2RootVolume.Type,
		},
		Zones: p.Zones,
	}
}

func convertAzureMachinePool(p *hivev1azure.MachinePool) *installerazure.MachinePool {
	if p == nil {
		return nil
	}
	return &installerazure.MachinePool{
		Zones:        p.Zones,
		InstanceType: p.InstanceType,
		OSDisk: installerazure.OSDisk{
			DiskSizeGB: p.OSDisk.DiskSizeGB,
		},
	}
}

func convertGCPMachinePool(p *hivev1gcp.MachinePool) *installergcp.MachinePool {
	if p == nil {
		return nil
	}
	return &installergcp.MachinePool{
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

func convertClusterNetworks(networks []networkv1.ClusterNetwork) []types.ClusterNetworkEntry {
	output := make([]types.ClusterNetworkEntry, len(networks))
	for i, network := range networks {
		output[i] = types.ClusterNetworkEntry{
			CIDR: *parseCIDR(network.CIDR),
			// TODO: deviation from installer API here
			HostPrefix: int32(network.HostSubnetLength),
		}
	}
	return output
}
