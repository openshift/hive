package machinepool

import (
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	machinev1alpha1 "github.com/openshift/api/machine/v1alpha1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1osp "github.com/openshift/hive/apis/hive/v1/openstack"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testmp "github.com/openshift/hive/pkg/test/machinepool"

	clientconfig "github.com/gophercloud/utils/openstack/clientconfig"
)

const (
	cloudName = "rhos-d"
)

func TestOpenStackActuator(t *testing.T) {
	tests := []struct {
		name                       string
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		expectedMachineSetReplicas map[string]int64
		expectedErr                bool
	}{
		{
			name:              "generate machineset",
			clusterDeployment: testOSPClusterDeployment(),
			pool:              testOSPPool(),
			expectedMachineSetReplicas: map[string]int64{
				fmt.Sprintf("%s-worker-%d", testInfraID, 0): 3,
			},
		},
	}
	cloudBytes, _ := yaml.Marshal(clientconfig.Clouds{
		Clouds: map[string]clientconfig.Cloud{
			cloudName: {},
		},
	})
	remoteFakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(
		&corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      "osp-credentials",
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"clouds.yaml": cloudBytes,
			},
		},
	).Build()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actuator := &OpenStackActuator{
				logger:     log.WithField("actuator", "openstackactuator_test"),
				kubeClient: remoteFakeClient,
				trunkSupportDiscoverer: func(cloud, alias string, opts *clientconfig.ClientOpts) (bool, error) {
					return true, nil
				},
			}

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, test.pool, actuator.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				require.NoError(t, err, "unexpected error for test case")
				validateOSPMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas)
			}
		})
	}
}

func Test_getOpenStackOSImage(t *testing.T) {
	logger := log.WithField("actuator", "openstackactuator_test")
	tests := []struct {
		name             string
		providerSpecYaml string
		wantOSImage      string
		wantErr          bool
	}{
		{
			name:             "Un-unmarshalable providerSpec",
			providerSpecYaml: "this is not valid yaml",
			wantErr:          true,
		},
		{
			name: "Old apiVersion, OSImage in image",
			// This is a real providerSpec from a real live test. We won't bother with
			// complete ones after this.
			providerSpecYaml: `apiVersion: openstackproviderconfig.openshift.io/v1alpha1
cloudName: openstack
cloudsSecret:
  name: openstack-cloud-credentials
  namespace: openshift-machine-api
flavor: m1.xlarge
image: clc-auto-psi-fh6rg-rhcos
kind: OpenstackProviderSpec
metadata:
  creationTimestamp: null
networks:
- filter: {}
  subnets:
  - filter:
    name: clc-auto-psi-fh6rg-nodes
    tags: openshiftClusterID=clc-auto-psi-fh6rg
securityGroups:
- filter: {}
  name: clc-auto-psi-fh6rg-worker
serverGroupName: clc-auto-psi-fh6rg-worker
serverMetadata:
  Name: clc-auto-psi-fh6rg-worker
  openshiftClusterID: clc-auto-psi-fh6rg
tags:
- openshiftClusterID=clc-auto-psi-fh6rg
trunk: true
userDataSecret:
  name: worker-user-data
`,
			wantOSImage: "clc-auto-psi-fh6rg-rhcos",
		},
		{
			name: "Future apiVersion, OSImage in rootVolume",
			providerSpecYaml: `apiVersion: something.we.do.not.know.yet/v1theta1
kind: OpenstackProviderSpec
rootVolume:
  sourceUUID: the-image
`,
			wantOSImage: "the-image",
		},
		{
			name: "sourceUUID takes precedence over image",
			providerSpecYaml: `apiVersion: whatever/v1
image: not-the-image
kind: OpenstackProviderSpec
rootVolume:
  sourceUUID: the-image
`,
			wantOSImage: "the-image",
		},
		{
			name: "empty rootVolume ignored",
			providerSpecYaml: `apiVersion: whatever/v1
image: the-image
kind: OpenstackProviderSpec
rootVolume: {}
`,
			wantOSImage: "the-image",
		},
		{
			name: "empty sourceUUID ignored",
			providerSpecYaml: `apiVersion: whatever/v1
image: the-image
kind: OpenstackProviderSpec
rootVolume:
  sourceUUID: ""
`,
			wantOSImage: "the-image",
		},
		{
			name: "rootVolume not castable to map",
			providerSpecYaml: `apiVersion: whatever/v1
kind: OpenstackProviderSpec
rootVolume: 42
`,
			wantErr: true,
		},
		{
			name: "sourceUUID not castable to string",
			providerSpecYaml: `apiVersion: whatever/v1
kind: OpenstackProviderSpec
rootVolume:
  sourceUUID: 42
`,
			wantErr: true,
		},
		{
			name: "image not castable to string",
			providerSpecYaml: `apiVersion: whatever/v1
image: 42
kind: OpenstackProviderSpec
`,
			wantErr: true,
		},
		{
			name: "no osImage found",
			providerSpecYaml: `apiVersion: whatever/v1
image: ""
kind: OpenstackProviderSpec
rootVolume:
  sourceUUID: ""
`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providerSpecJSON, err := yaml.YAMLToJSON([]byte(tt.providerSpecYaml))
			assert.NoError(t, err, "Couldn't convert yaml providerSpec test input to JSON")
			masterMachine := &machinev1beta1.Machine{
				Spec: machinev1beta1.MachineSpec{
					ProviderSpec: machinev1beta1.ProviderSpec{
						Value: &runtime.RawExtension{
							Raw: providerSpecJSON,
						},
					},
				},
			}
			gotOSImage, err := getOpenStackOSImage(masterMachine, logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("getOpenStackOSImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotOSImage != tt.wantOSImage {
				t.Errorf("getOpenStackOSImage() = %v, want %v", gotOSImage, tt.wantOSImage)
			}
		})
	}
}

func validateOSPMachineSets(t *testing.T, mSets []*machinev1beta1.MachineSet, expectedMSReplicas map[string]int64) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set: %s", ms.Name) {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch for MachineSet %s", ms.Name)
		}

		ospProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*machinev1alpha1.OpenstackProviderSpec)
		if assert.True(t, ok, "failed to convert to openstack provider spec") {
			assert.Equal(t, "Flav", ospProvider.Flavor, "unexpected instance type")
		}
	}
}

func testOSPPool(opts ...testmp.Option) *hivev1.MachinePool {
	p := testMachinePool(opts...)
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		OpenStack: &hivev1osp.MachinePool{
			Flavor: "Flav",
		},
	}
	return p
}

func testOSPClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Platform = hivev1.Platform{
		OpenStack: &hivev1osp.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "osp-credentials",
			},
			Cloud: cloudName,
		},
	}
	return cd
}
