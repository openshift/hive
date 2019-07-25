package install

import (
	"testing"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	"github.com/openshift/installer/pkg/ipnet"
	installtypes "github.com/openshift/installer/pkg/types"
	installawstypes "github.com/openshift/installer/pkg/types/aws"

	netopv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

const (
	testName                 = "foo"
	testNamespace            = "default"
	testAMI                  = "ami-totallyfake"
	adminPassword            = "adminpassword"
	adminSSHKey              = "adminSSH"
	pullSecret               = "pullSecret"
	awsInstanceType          = "fake-aws-type"
	awsRegion                = "us-east-1"
	ec2VolIOPS               = 100
	ec2VolSize               = 500
	ec2VolType               = "sometype"
	hiveDefaultAMIAnnotation = "hive.openshift.io/default-AMI"
)

var (
	vpcCIDRBlock = ipnet.MustParseCIDR("10.1.0.0/16")
)

func buildValidClusterDeployment() *hivev1.ClusterDeployment {
	replicas := int64(3)
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				hiveDefaultAMIAnnotation: testAMI,
			},
		},
		Spec: hivev1.ClusterDeploymentSpec{
			BaseDomain: "test.example.com",
			SSHKey: &corev1.LocalObjectReference{
				Name: "ssh-key",
			},
			PullSecret: &corev1.LocalObjectReference{
				Name: "pull-secret",
			},
			Platform: hivev1.Platform{
				AWS: &hivev1.AWSPlatform{
					Region: awsRegion,
					UserTags: map[string]string{
						"foo": "bar",
					},
					DefaultMachinePlatform: &hivev1.AWSMachinePoolPlatform{
						InstanceType: awsInstanceType,
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
				ClusterNetworks: []netopv1.ClusterNetwork{
					{
						CIDR:             "10.128.0.0/14",
						HostSubnetLength: 9,
					},
				},
				MachineCIDR: vpcCIDRBlock.String(),
			},
			ControlPlane: hivev1.MachinePool{
				Name:     "master",
				Replicas: &replicas,
				Platform: hivev1.MachinePoolPlatform{
					AWS: &hivev1.AWSMachinePoolPlatform{
						InstanceType: awsInstanceType,
						EC2RootVolume: hivev1.EC2RootVolume{
							IOPS: ec2VolIOPS,
							Size: ec2VolSize,
							Type: ec2VolType,
						},
						Zones: []string{"us-east-1a", "us-east-1b"},
					},
				},
			},
			Compute: []hivev1.MachinePool{
				{
					Name:     "worker",
					Replicas: &replicas,
					Platform: hivev1.MachinePoolPlatform{
						AWS: &hivev1.AWSMachinePoolPlatform{
							InstanceType: awsInstanceType,
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
	}
}

// buildBaseExpectedInstallConfig should match the above buildValidClusterDeployment exactly.
func buildBaseExpectedInstallConfig() *installtypes.InstallConfig {
	replicas := int64(3)
	return &installtypes.InstallConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: installtypes.InstallConfigVersion,
		},
		BaseDomain: "test.example.com",
		SSHKey:     adminSSHKey,
		PullSecret: pullSecret,
		Networking: &installtypes.Networking{
			// TODO: Hardcoded to match installer for now.
			NetworkType:    "OpenShiftSDN",
			ServiceNetwork: []ipnet.IPNet{*ipnet.MustParseCIDR("172.30.0.0/16")},
			ClusterNetwork: []installtypes.ClusterNetworkEntry{
				{
					CIDR:       *ipnet.MustParseCIDR("10.128.0.0/14"),
					HostPrefix: 9,
				},
			},
			MachineCIDR: vpcCIDRBlock,
		},
		Platform: installtypes.Platform{
			AWS: &installawstypes.Platform{
				Region: awsRegion,
				UserTags: map[string]string{
					"foo": "bar",
				},
				DefaultMachinePlatform: &installawstypes.MachinePool{
					InstanceType: awsInstanceType,
					EC2RootVolume: installawstypes.EC2RootVolume{
						IOPS: ec2VolIOPS,
						Size: ec2VolSize,
						Type: ec2VolType,
					},
				},
			},
		},
		ControlPlane: &installtypes.MachinePool{
			Name:     "master",
			Replicas: &replicas,
			Platform: installtypes.MachinePoolPlatform{
				AWS: &installawstypes.MachinePool{
					InstanceType: awsInstanceType,
					EC2RootVolume: installawstypes.EC2RootVolume{
						IOPS: ec2VolIOPS,
						Size: ec2VolSize,
						Type: ec2VolType,
					},
					Zones: []string{"us-east-1a", "us-east-1b"},
				},
			},
		},

		Compute: []installtypes.MachinePool{
			{
				Name:     "worker",
				Replicas: &replicas,
				Platform: installtypes.MachinePoolPlatform{
					AWS: &installawstypes.MachinePool{
						InstanceType: awsInstanceType,
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
		name                     string
		cd                       *hivev1.ClusterDeployment
		expectedInstallConfig    *installtypes.InstallConfig
		generateConfigForInstall bool
	}{
		{
			name:                     "full copy",
			cd:                       buildValidClusterDeployment(),
			expectedInstallConfig:    buildBaseExpectedInstallConfig(),
			generateConfigForInstall: true,
		},
		{
			name: "no default machine pool",
			cd: func() *hivev1.ClusterDeployment {
				cd := buildValidClusterDeployment()
				cd.Spec.Platform.AWS.DefaultMachinePlatform = nil
				return cd
			}(),
			expectedInstallConfig: func() *installtypes.InstallConfig {
				ic := buildBaseExpectedInstallConfig()
				ic.Platform.AWS.DefaultMachinePlatform = nil
				return ic
			}(),
			generateConfigForInstall: true,
		},
		{
			name: "no platform",
			cd: func() *hivev1.ClusterDeployment {
				cd := buildValidClusterDeployment()
				cd.Spec.Platform.AWS = nil
				return cd
			}(),
			expectedInstallConfig: func() *installtypes.InstallConfig {
				ic := buildBaseExpectedInstallConfig()
				ic.Platform.AWS = nil
				return ic
			}(),
			generateConfigForInstall: true,
		},
		{
			name: "no networking CIDRs",
			cd: func() *hivev1.ClusterDeployment {
				cd := buildValidClusterDeployment()
				cd.Spec.Networking.ClusterNetworks = []netopv1.ClusterNetwork{}
				cd.Spec.Networking.ServiceCIDR = ""
				cd.Spec.Networking.MachineCIDR = ""
				return cd
			}(),
			expectedInstallConfig: func() *installtypes.InstallConfig {
				ic := buildBaseExpectedInstallConfig()
				ic.Networking.ServiceNetwork = []ipnet.IPNet{{}}
				ic.Networking.MachineCIDR = &ipnet.IPNet{}
				ic.Networking.ClusterNetwork = []installtypes.ClusterNetworkEntry{}
				return ic
			}(),
			generateConfigForInstall: true,
		},
		{
			name: "control plane pool name not master",
			cd: func() *hivev1.ClusterDeployment {
				cd := buildValidClusterDeployment()
				cd.Spec.ControlPlane.Name = "notmaster"
				return cd
			}(),
			expectedInstallConfig:    buildBaseExpectedInstallConfig(),
			generateConfigForInstall: true,
		},
		{
			name: "remove non-worker compute pool for install job",
			cd: func() *hivev1.ClusterDeployment {
				cd := buildValidClusterDeployment()
				replicas := int64(3)
				extraPool := hivev1.MachinePool{
					Name:     "extra",
					Replicas: &replicas,
					Platform: hivev1.MachinePoolPlatform{
						AWS: &hivev1.AWSMachinePoolPlatform{
							InstanceType: awsInstanceType,
							EC2RootVolume: hivev1.EC2RootVolume{
								IOPS: ec2VolIOPS,
								Size: ec2VolSize,
								Type: ec2VolType,
							},
						},
					},
				}
				cd.Spec.Compute = append(cd.Spec.Compute, extraPool)
				return cd
			}(),
			expectedInstallConfig:    buildBaseExpectedInstallConfig(),
			generateConfigForInstall: true,
		},
		{
			name: "leave non-worker pool when not generating for install job",
			cd: func() *hivev1.ClusterDeployment {
				cd := buildValidClusterDeployment()
				replicas := int64(3)
				extraPool := hivev1.MachinePool{
					Name:     "extra",
					Replicas: &replicas,
					Platform: hivev1.MachinePoolPlatform{
						AWS: &hivev1.AWSMachinePoolPlatform{
							InstanceType: awsInstanceType,
							EC2RootVolume: hivev1.EC2RootVolume{
								IOPS: ec2VolIOPS,
								Size: ec2VolSize,
								Type: ec2VolType,
							},
						},
					},
				}
				cd.Spec.Compute = append(cd.Spec.Compute, extraPool)
				return cd
			}(),
			expectedInstallConfig: func() *installtypes.InstallConfig {
				ic := buildBaseExpectedInstallConfig()
				replicas := int64(3)
				extraPool := installtypes.MachinePool{
					Name:     "extra",
					Replicas: &replicas,
					Platform: installtypes.MachinePoolPlatform{
						AWS: &installawstypes.MachinePool{
							InstanceType: awsInstanceType,
							EC2RootVolume: installawstypes.EC2RootVolume{
								IOPS: ec2VolIOPS,
								Size: ec2VolSize,
								Type: ec2VolType,
							},
						},
					},
				}
				ic.Compute = append(ic.Compute, extraPool)
				return ic
			}(),
			generateConfigForInstall: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ic, err := GenerateInstallConfig(test.cd, adminSSHKey, pullSecret, test.generateConfigForInstall)
			if assert.NoError(t, err) {
				assert.Equal(t, test.expectedInstallConfig, ic)
			}
		})
	}
}
