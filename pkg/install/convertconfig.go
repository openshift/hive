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
	"fmt"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/installer/pkg/ipnet"
	"github.com/openshift/installer/pkg/types"
	installeraws "github.com/openshift/installer/pkg/types/aws"

	netopv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"
)

// GenerateInstallConfig builds an InstallConfig for the installer from our ClusterDeploymentSpec.
// The two types are extremely similar, but have different goals and in some cases deviation was required
// as ClusterDeployment is used as a CRD API.
//
// It is assumed the caller will lookup the admin password and ssh key from their respective secrets.
func GenerateInstallConfig(cd *hivev1.ClusterDeployment, sshKey, pullSecret string) (*types.InstallConfig, error) {
	spec := cd.Spec

	networkType, err := convertNetworkingType(spec.Config.Networking.Type)
	if err != nil {
		return nil, err
	}

	platform := types.Platform{}
	if spec.Config.Platform.AWS != nil {
		aws := spec.Config.Platform.AWS
		platform.AWS = &installeraws.Platform{
			Region:       aws.Region,
			UserTags:     aws.UserTags,
			VPCCIDRBlock: parseCIDR(aws.VPCCIDRBlock),
		}
		if aws.DefaultMachinePlatform != nil {
			platform.AWS.DefaultMachinePlatform = &installeraws.MachinePool{
				InstanceType: aws.DefaultMachinePlatform.InstanceType,
				IAMRoleName:  aws.DefaultMachinePlatform.IAMRoleName,
				EC2RootVolume: installeraws.EC2RootVolume{
					IOPS: aws.DefaultMachinePlatform.EC2RootVolume.IOPS,
					Size: aws.DefaultMachinePlatform.EC2RootVolume.Size,
					Type: aws.DefaultMachinePlatform.EC2RootVolume.Type,
				},
				AMIID: aws.DefaultMachinePlatform.AMIID,
				Zones: aws.DefaultMachinePlatform.Zones,
			}
		}
	}

	machinePools := []types.MachinePool{}
	for _, mp := range spec.Config.Machines {
		newMP := types.MachinePool{
			Name:     mp.Name,
			Replicas: mp.Replicas,
		}
		if mp.Platform.AWS != nil {
			newMP.Platform.AWS = &installeraws.MachinePool{
				InstanceType: mp.Platform.AWS.InstanceType,
				IAMRoleName:  mp.Platform.AWS.IAMRoleName,
				EC2RootVolume: installeraws.EC2RootVolume{
					IOPS: mp.Platform.AWS.EC2RootVolume.IOPS,
					Size: mp.Platform.AWS.EC2RootVolume.Size,
					Type: mp.Platform.AWS.EC2RootVolume.Type,
				},
				AMIID: mp.Platform.AWS.AMIID,
				Zones: mp.Platform.AWS.Zones,
			}
		}
		machinePools = append(machinePools, newMP)
	}

	ic := &types.InstallConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: spec.Config.ClusterID,
		},
		ClusterID:  cd.Spec.ClusterUUID,
		SSHKey:     sshKey,
		BaseDomain: spec.Config.BaseDomain,
		Networking: types.Networking{
			Type:        networkType,
			ServiceCIDR: *parseCIDR(spec.Config.Networking.ServiceCIDR),
			PodCIDR:     parseCIDR(spec.Config.Networking.PodCIDR),
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

func convertNetworkingType(hnt hivev1.NetworkType) (netopv1.NetworkType, error) {
	switch hnt {
	case hivev1.NetworkTypeOpenshiftOVN:
		return netopv1.NetworkTypeOVNKubernetes, nil
	case hivev1.NetworkTypeOpenshiftSDN:
		return netopv1.NetworkTypeOpenshiftSDN, nil
	default:
		return "", fmt.Errorf("unknown NetworkType: %s", hnt)
	}
}
