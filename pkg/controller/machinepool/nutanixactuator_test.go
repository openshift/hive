package machinepool

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	machinev1 "github.com/openshift/api/machine/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1nutanix "github.com/openshift/hive/apis/hive/v1/nutanix"
	testfake "github.com/openshift/hive/pkg/test/fake"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func getMasterMachineWithImage(t assert.TestingT) *machineapi.Machine {
	masterMachine := testMachine("master0", "master")

	uuid := "some-machine-image-uuid"
	name := "nutanix-image-name"
	nmpc := machinev1.NutanixMachineProviderConfig{
		Image: machinev1.NutanixResourceIdentifier{
			Type: machinev1.NutanixIdentifierName,
			UUID: &uuid,
			Name: &name,
		},
	}
	providerSpecValue, err := json.Marshal(nmpc)
	assert.NoError(t, err, "unexpected error creating NutanixActuator")

	masterMachine.Spec.ProviderSpec.Value = &runtime.RawExtension{
		Raw: providerSpecValue,
	}

	return masterMachine
}

func TestNewNutanixActuator(t *testing.T) {
	fakeClient := testfake.NewFakeClientBuilder().Build()
	actuator, err := NewNutanixActuator(fakeClient, getMasterMachineWithImage(t))
	assert.NoError(t, err, "unexpected error creating NutanixActuator")
	assert.NotNil(t, actuator, "expected a valid NutanixActuator instance")
}

func TestGenerateMachineSets(t *testing.T) {
	tests := []struct {
		name              string
		clusterDeployment *hivev1.ClusterDeployment
		pool              *hivev1.MachinePool
		expectedErr       bool
	}{
		{
			name: "ClusterDeployment with Nutanix platform and with proper metadata",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testNutanixClusterDeployment()
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{InfraID: "valid-infra"}
				cd.Spec.Platform.Nutanix = &hivev1nutanix.Platform{
					FailureDomains: []hivev1nutanix.FailureDomain{
						{
							Name: "Name",
							PrismElement: hivev1nutanix.PrismElement{
								UUID: "test-prism-element-uuid",
								Endpoint: hivev1nutanix.PrismEndpoint{
									Address: "valid-prism.example.com",
									Port:    9440,
								},
								Name: "",
							},
						},
					},
				}
				return cd
			}(),
			pool:        testNutanixPool(),
			expectedErr: false,
		},
		{
			name: "ClusterDeployment missing metadata",
			clusterDeployment: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					Platform: hivev1.Platform{
						Nutanix: &hivev1nutanix.Platform{},
					},
				},
			},
			pool:        testNutanixPool(),
			expectedErr: true,
		},
		{
			name: "ClusterDeployment is not for Nutanix",
			clusterDeployment: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterMetadata: &hivev1.ClusterMetadata{InfraID: "test-infra"},
				},
			},
			pool:        testNutanixPool(),
			expectedErr: true,
		},
		{
			name: "ClusterDeployment with multiple FailureDomains",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testNutanixClusterDeployment()
				cd.Spec.Platform.Nutanix.FailureDomains = []hivev1nutanix.FailureDomain{
					{Name: "valid-domain-1", PrismElement: hivev1nutanix.PrismElement{UUID: "test-prism-element-uuid-1"}},
					{Name: "valid-domain-2", PrismElement: hivev1nutanix.PrismElement{UUID: "test-prism-element-uuid-2"}},
					{Name: "valid-domain-3", PrismElement: hivev1nutanix.PrismElement{UUID: "test-prism-element-uuid-3"}},
				}
				return cd
			}(),
			pool:        testNutanixPool(),
			expectedErr: false,
		},
		{
			name:              "Valid MachineSets generation",
			clusterDeployment: testNutanixClusterDeployment(),
			pool:              testNutanixPool(),
			expectedErr:       false,
		},
		{
			name:              "MachinePool missing Nutanix configuration",
			clusterDeployment: testNutanixClusterDeployment(),
			pool:              &hivev1.MachinePool{},
			expectedErr:       true,
		},
		{
			name:              "Invalid FailureDomains",
			clusterDeployment: testNutanixClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				p := testNutanixPool()
				p.Spec.Platform.Nutanix.FailureDomains = []string{"invalid-domain"}
				return p
			}(),
			expectedErr: true,
		},
		{
			name: "Valid FailureDomains",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testNutanixClusterDeployment()
				cd.Spec.Platform.Nutanix.FailureDomains = []hivev1nutanix.FailureDomain{
					{Name: "valid-domain-1"},
					{Name: "valid-domain-2"},
				}
				return cd
			}(),
			pool: func() *hivev1.MachinePool {
				p := testNutanixPool()
				p.Spec.Platform.Nutanix.FailureDomains = []string{"valid-domain-1", "valid-domain-2"}
				return p
			}(),
			expectedErr: false,
		},
		{
			name:              "BootType is correctly set",
			clusterDeployment: testNutanixClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				p := testNutanixPool()
				p.Spec.Platform.Nutanix.BootType = "UEFI"
				return p
			}(),
			expectedErr: false,
		},
		{
			name:              "OSDisk size is correctly set",
			clusterDeployment: testNutanixClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				p := testNutanixPool()
				p.Spec.Platform.Nutanix.OSDisk.DiskSizeGiB = 100
				return p
			}(),
			expectedErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("actuator", "nutanixactuator_test")
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.pool).Build()

			actuator, err := NewNutanixActuator(fakeClient, getMasterMachineWithImage(t))
			require.NoError(t, err, "unexpected error creating NutanixActuator")

			_, _, err = actuator.GenerateMachineSets(test.clusterDeployment, test.pool, logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				assert.NoError(t, err, "unexpected error for test case")
			}
		})
	}
}

func testNutanixPool() *hivev1.MachinePool {
	return &hivev1.MachinePool{
		Spec: hivev1.MachinePoolSpec{
			Platform: hivev1.MachinePoolPlatform{
				Nutanix: &hivev1nutanix.MachinePool{
					NumCPUs:           4,
					NumCoresPerSocket: 2,
					MemoryMiB:         8192,
				},
			},
		},
	}
}

func testNutanixClusterDeployment() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterMetadata: &hivev1.ClusterMetadata{InfraID: "test-infra"},
			Platform: hivev1.Platform{
				Nutanix: &hivev1nutanix.Platform{
					FailureDomains: []hivev1nutanix.FailureDomain{
						{
							Name: "NAME",
							PrismElement: hivev1nutanix.PrismElement{
								UUID: "test-prism-element-uuid",
								Endpoint: hivev1nutanix.PrismEndpoint{
									Address: "prism.example.com",
									Port:    9440,
								},
							},
							SubnetUUIDs:       nil,
							StorageContainers: nil,
							DataSourceImages:  nil,
						},
					},
				},
			},
		},
	}
}

func TestDecodeNutanixMachineProviderSpec(t *testing.T) {
	t.Run("Valid provider spec", func(t *testing.T) {
		resourceName := "nutanix-image"
		providerConfig := &machinev1.NutanixMachineProviderConfig{
			Image: machinev1.NutanixResourceIdentifier{Name: &resourceName},
		}
		data, err := json.Marshal(providerConfig)
		assert.NoError(t, err)

		rawExt := &runtime.RawExtension{Raw: data}
		decodedSpec, err := decodeNutanixMachineProviderSpec(rawExt)
		assert.NoError(t, err)
		assert.NotNil(t, decodedSpec)
		assert.Equal(t, "nutanix-image", *decodedSpec.Image.Name)
	})

	t.Run("Nil provider spec", func(t *testing.T) {
		decodedSpec, err := decodeNutanixMachineProviderSpec(nil)
		assert.NoError(t, err)
		assert.NotNil(t, decodedSpec)
	})
}

func TestGetRHCOSImageNameFromMasterMachine(t *testing.T) {
	logger := log.New()
	cd := &hivev1.ClusterDeployment{Spec: hivev1.ClusterDeploymentSpec{Platform: hivev1.Platform{}}}

	t.Run("Valid RHCOS image name", func(t *testing.T) {
		masterMachine := getMasterMachineWithImage(t)
		imageName, err := getRHCOSImageNameFromMasterMachine(masterMachine, cd, logger)
		assert.NoError(t, err)
		assert.Equal(t, "nutanix-image-name", imageName)
	})

	t.Run("Missing RHCOS image name", func(t *testing.T) {
		masterMachine := testMachine("master0", "master")
		imageName, err := getRHCOSImageNameFromMasterMachine(masterMachine, cd, logger)
		assert.Error(t, err)
		assert.Empty(t, imageName)
	})
}

type erroringStatusWriter struct {
	client.StatusWriter
}

func (e *erroringStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return fmt.Errorf("simulated status update failure")
}

type fakeClientWithStatusError struct {
	client.Client
	statusWriter client.StatusWriter
}

func (f *fakeClientWithStatusError) Status() client.StatusWriter {
	return f.statusWriter
}

func TestNutanixActuator_GenerateMachineSets_DataDiskFailure_StatusUpdateFails(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, hivev1.AddToScheme(scheme))
	require.NoError(t, machineapi.AddToScheme(scheme))
	require.NoError(t, machinev1.AddToScheme(scheme))

	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	clientWithError := &fakeClientWithStatusError{
		Client:       baseClient,
		statusWriter: &erroringStatusWriter{baseClient.Status()},
	}

	actuator, _ := NewNutanixActuator(clientWithError, &machineapi.Machine{})

	pool := testNutanixPool()
	pool.ObjectMeta = metav1.ObjectMeta{
		Namespace: "test-namespace",
		Name:      "test-pool",
	}
	pool.Spec.Platform.Nutanix.DataDisks = []machinev1.NutanixVMDisk{
		{
			DataSource: &machinev1.NutanixResourceIdentifier{
				UUID: nil,
			},
		},
	}

	cd := testNutanixClusterDeployment()
	cd.ObjectMeta = metav1.ObjectMeta{
		Namespace: "test-namespace",
		Name:      "test-cd",
	}
	cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{InfraID: "infra"}

	logger := log.New()
	_, _, err := actuator.GenerateMachineSets(cd, pool, logger)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated status update failure")
}

func TestNutanixActuator_GenerateMachineSets_MissingRHCOSImage_StatusUpdateFails(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, hivev1.AddToScheme(scheme))
	require.NoError(t, machineapi.AddToScheme(scheme))
	require.NoError(t, machinev1.AddToScheme(scheme))

	baseClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	clientWithError := &fakeClientWithStatusError{
		Client:       baseClient,
		statusWriter: &erroringStatusWriter{baseClient.Status()},
	}

	masterMachine := &machineapi.Machine{
		Spec: machineapi.MachineSpec{
			ProviderSpec: machineapi.ProviderSpec{
				Value: &runtime.RawExtension{Raw: []byte(`{}`)},
			},
		},
	}

	actuator, _ := NewNutanixActuator(clientWithError, masterMachine)

	pool := testNutanixPool()
	pool.ObjectMeta = metav1.ObjectMeta{
		Namespace: "test-namespace",
		Name:      "test-pool",
	}

	cd := testNutanixClusterDeployment()
	cd.ObjectMeta = metav1.ObjectMeta{
		Namespace: "test-namespace",
		Name:      "test-cd",
	}
	cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{InfraID: "infra"}

	logger := log.New()
	_, _, err := actuator.GenerateMachineSets(cd, pool, logger)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated status update failure")
}
