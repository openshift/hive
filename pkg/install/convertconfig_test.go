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
	"testing"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	"github.com/openshift/installer/pkg/ipnet"
	installtypes "github.com/openshift/installer/pkg/types"
	installawstypes "github.com/openshift/installer/pkg/types/aws"

	corev1 "k8s.io/api/core/v1"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

const (
	testName        = "foo"
	testNamespace   = "default"
	testClusterID   = "foo"
	testAMI         = "ami-totallyfake"
	adminPassword   = "adminpassword"
	adminSSHKey     = "adminSSH"
	pullSecret      = "pullSecret"
	vpcCIDRBlock    = "10.1.0.0/16"
	awsInstanceType = "fake-aws-type"
	awsRegion       = "us-east-1"
	iamRoleName     = "rolename"
	ec2VolIOPS      = 100
	ec2VolSize      = 500
	ec2VolType      = "sometype"
)

func buildValidClusterDeployment() *hivev1.ClusterDeployment {
	replicas := int64(3)
	return &hivev1.ClusterDeployment{
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterUUID: testClusterID,
			Config: hivev1.InstallConfig{
				BaseDomain: "test.example.com",
				SSHKey: &corev1.LocalObjectReference{
					Name: "ssh-key",
				},
				PullSecret: corev1.LocalObjectReference{
					Name: "pull-secret",
				},
				Platform: hivev1.Platform{
					AWS: &hivev1.AWSPlatform{
						Region: awsRegion,
						UserTags: map[string]string{
							"foo": "bar",
						},
						VPCCIDRBlock: vpcCIDRBlock,
						DefaultMachinePlatform: &hivev1.AWSMachinePoolPlatform{
							InstanceType: awsInstanceType,
							IAMRoleName:  iamRoleName,
							EC2RootVolume: hivev1.EC2RootVolume{
								IOPS: ec2VolIOPS,
								Size: ec2VolSize,
								Type: ec2VolType,
							},
						},
					},
				},
				Networking: hivev1.Networking{
					Type:        hivev1.NetworkTypeOpenshiftSDN,
					ServiceCIDR: "172.30.0.0/16",
					PodCIDR:     "10.128.0.0/14",
				},
				Machines: []hivev1.MachinePool{
					{
						Name:     "masters",
						Replicas: &replicas,
						Platform: hivev1.MachinePoolPlatform{
							AWS: &hivev1.AWSMachinePoolPlatform{
								InstanceType: awsInstanceType,
								IAMRoleName:  iamRoleName,
								EC2RootVolume: hivev1.EC2RootVolume{
									IOPS: ec2VolIOPS,
									Size: ec2VolSize,
									Type: ec2VolType,
								},
								AMIID: testAMI,
								Zones: []string{"us-east-1a", "us-east-1b"},
							},
						},
					},
					{
						Name:     "workers",
						Replicas: &replicas,
						Platform: hivev1.MachinePoolPlatform{
							AWS: &hivev1.AWSMachinePoolPlatform{
								InstanceType: awsInstanceType,
								IAMRoleName:  iamRoleName,
								EC2RootVolume: hivev1.EC2RootVolume{
									IOPS: ec2VolIOPS,
									Size: ec2VolSize,
									Type: ec2VolType,
								},
							},
						},
					},
				},
			},
		},
	}
}

// buildBaseExpectedInstallConfig should match the above buildValidClusterDeployment exactly.
func buildBaseExpectedInstallConfig() *installtypes.InstallConfig {
	replicas := int64(3)
	return &installtypes.InstallConfig{
		ClusterID:  testClusterID,
		BaseDomain: "test.example.com",
		SSHKey:     adminSSHKey,
		PullSecret: pullSecret,
		Networking: installtypes.Networking{
			// TODO: Hardcoded to match installer for now.
			Type: "OpenshiftSDN",
			ServiceCIDR: ipnet.IPNet{
				IPNet: parseCIDR("172.30.0.0/16"),
			},
			PodCIDR: &ipnet.IPNet{
				IPNet: parseCIDR("10.128.0.0/14"),
			},
		},
		Platform: installtypes.Platform{
			AWS: &installawstypes.Platform{
				Region: awsRegion,
				UserTags: map[string]string{
					"foo": "bar",
				},
				VPCCIDRBlock: vpcCIDRBlock,
				DefaultMachinePlatform: &installawstypes.MachinePool{
					InstanceType: awsInstanceType,
					IAMRoleName:  iamRoleName,
					EC2RootVolume: installawstypes.EC2RootVolume{
						IOPS: ec2VolIOPS,
						Size: ec2VolSize,
						Type: ec2VolType,
					},
				},
			},
		},
		Machines: []installtypes.MachinePool{
			{
				Name:     "masters",
				Replicas: &replicas,
				Platform: installtypes.MachinePoolPlatform{
					AWS: &installawstypes.MachinePool{
						InstanceType: awsInstanceType,
						IAMRoleName:  iamRoleName,
						EC2RootVolume: installawstypes.EC2RootVolume{
							IOPS: ec2VolIOPS,
							Size: ec2VolSize,
							Type: ec2VolType,
						},
						AMIID: testAMI,
						Zones: []string{"us-east-1a", "us-east-1b"},
					},
				},
			},
			{
				Name:     "workers",
				Replicas: &replicas,
				Platform: installtypes.MachinePoolPlatform{
					AWS: &installawstypes.MachinePool{
						InstanceType: awsInstanceType,
						IAMRoleName:  iamRoleName,
						EC2RootVolume: installawstypes.EC2RootVolume{
							IOPS: ec2VolIOPS,
							Size: ec2VolSize,
							Type: ec2VolType,
						},
					},
				},
			},
		},
	}
}

func TestConvert(t *testing.T) {
	tests := []struct {
		name                  string
		cd                    *hivev1.ClusterDeployment
		expectedInstallConfig *installtypes.InstallConfig
	}{
		{
			name: "full copy",
			cd:   buildValidClusterDeployment(),
			expectedInstallConfig: buildBaseExpectedInstallConfig(),
		},
		{
			name: "no default machine pool",
			cd: func() *hivev1.ClusterDeployment {
				cd := buildValidClusterDeployment()
				cd.Spec.Config.Platform.AWS.DefaultMachinePlatform = nil
				return cd
			}(),
			expectedInstallConfig: func() *installtypes.InstallConfig {
				ic := buildBaseExpectedInstallConfig()
				ic.Platform.AWS.DefaultMachinePlatform = nil
				return ic
			}(),
		},
		{
			name: "no platform",
			cd: func() *hivev1.ClusterDeployment {
				cd := buildValidClusterDeployment()
				cd.Spec.Config.Platform.AWS = nil
				return cd
			}(),
			expectedInstallConfig: func() *installtypes.InstallConfig {
				ic := buildBaseExpectedInstallConfig()
				ic.Platform.AWS = nil
				return ic
			}(),
		},
		{
			name: "no networking CIDRs",
			cd: func() *hivev1.ClusterDeployment {
				cd := buildValidClusterDeployment()
				cd.Spec.Config.Networking.PodCIDR = ""
				cd.Spec.Config.Networking.ServiceCIDR = ""
				return cd
			}(),
			expectedInstallConfig: func() *installtypes.InstallConfig {
				ic := buildBaseExpectedInstallConfig()
				ic.Networking.PodCIDR = &ipnet.IPNet{}
				ic.Networking.ServiceCIDR = ipnet.IPNet{}
				return ic
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ic, err := GenerateInstallConfig(test.cd, adminSSHKey, pullSecret)
			if assert.NoError(t, err) {
				assert.Equal(t, test.expectedInstallConfig, ic)
			}
		})
	}
}
