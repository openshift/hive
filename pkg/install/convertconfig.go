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

package install

import (
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"
	"github.com/openshift/installer/pkg/ipnet"
	"github.com/openshift/installer/pkg/types"
	installeraws "github.com/openshift/installer/pkg/types/aws"
)

// GenerateInstallConfig builds an InstallConfig for the installer from our ClusterDeploymentSpec.
// The two types are extremely similar, but have different goals and in some cases deviation was required
// as ClusterDeployment is used as a CRD API.
//
// It is assumed the caller will lookup the admin password and ssh key from their respective secrets.
func GenerateInstallConfig(cd *hivev1.ClusterDeployment, sshKey, pullSecret string) (*types.InstallConfig, error) {
	spec := cd.Spec

	platform := types.Platform{}
	if spec.Platform.AWS != nil {
		aws := spec.Platform.AWS
		platform.AWS = &installeraws.Platform{
			Region:   aws.Region,
			UserTags: aws.UserTags,
		}
		if aws.DefaultMachinePlatform != nil {
			platform.AWS.DefaultMachinePlatform = &installeraws.MachinePool{
				InstanceType: aws.DefaultMachinePlatform.InstanceType,
				EC2RootVolume: installeraws.EC2RootVolume{
					IOPS: aws.DefaultMachinePlatform.EC2RootVolume.IOPS,
					Size: aws.DefaultMachinePlatform.EC2RootVolume.Size,
					Type: aws.DefaultMachinePlatform.EC2RootVolume.Type,
				},
				Zones: aws.DefaultMachinePlatform.Zones,
			}
		}
	}

	machinePools := []types.MachinePool{}

	// combinedMachinePools contains spec.ControlPlane and spec.Compute MachinePools
	combinedMachinePools := []hivev1.MachinePool{}
	if spec.ControlPlane.Name != "master" {
		spec.ControlPlane.Name = "master"
	}
	combinedMachinePools = append(combinedMachinePools, spec.ControlPlane)
	for _, mp := range spec.Compute {
		combinedMachinePools = append(combinedMachinePools, mp)
	}

	for _, mp := range combinedMachinePools {
		newMP := types.MachinePool{
			Name:     mp.Name,
			Replicas: mp.Replicas,
		}
		if mp.Platform.AWS != nil {
			newMP.Platform.AWS = &installeraws.MachinePool{
				InstanceType: mp.Platform.AWS.InstanceType,
				EC2RootVolume: installeraws.EC2RootVolume{
					IOPS: mp.Platform.AWS.EC2RootVolume.IOPS,
					Size: mp.Platform.AWS.EC2RootVolume.Size,
					Type: mp.Platform.AWS.EC2RootVolume.Type,
				},
				Zones: mp.Platform.AWS.Zones,
			}
		}
		machinePools = append(machinePools, newMP)
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
			Type:            string(spec.Networking.Type),
			ServiceCIDR:     parseCIDR(spec.Networking.ServiceCIDR),
			ClusterNetworks: convertClusterNetworks(spec.Networking.ClusterNetworks),
			MachineCIDR:     parseCIDR(spec.Networking.MachineCIDR),
		},
		PullSecret: pullSecret,
		Platform:   platform,
		Machines:   machinePools,
	}
	return ic, nil
}

func parseCIDR(s string) *ipnet.IPNet {
	if s == "" {
		return &ipnet.IPNet{}
	}
	return ipnet.MustParseCIDR(s)
}

func convertClusterNetworks(networks []networkv1.ClusterNetwork) []types.ClusterNetworkEntry {
	output := make([]types.ClusterNetworkEntry, 0, len(networks))
	for _, network := range networks {
		output = append(output, types.ClusterNetworkEntry{
			CIDR:             *parseCIDR(network.CIDR),
			HostSubnetLength: int32(network.HostSubnetLength),
		})
	}
	return output
}
