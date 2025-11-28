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
	"github.com/openshift/hive/pkg/test/fake"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	fakeClient := fake.NewFakeClientBuilder().Build()
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
					{
						Name: "valid-domain-1",
						PrismElement: hivev1nutanix.PrismElement{
							UUID: "test-prism-element-uuid-1",
							Endpoint: hivev1nutanix.PrismEndpoint{
								Address: "prism1.example.com",
								Port:    9440,
							},
						},
					},
					{
						Name: "valid-domain-2",
						PrismElement: hivev1nutanix.PrismElement{
							UUID: "test-prism-element-uuid-2",
							Endpoint: hivev1nutanix.PrismEndpoint{
								Address: "prism2.example.com",
								Port:    9440,
							},
						},
					},
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
			name: "Missing PrismElements",
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
			expectedErr: true,
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
		{
			name:              "Autoscaling without failure domains",
			clusterDeployment: testNutanixClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				p := testNutanixPool()
				p.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 1,
					MaxReplicas: 3,
				}
				return p
			}(),
			expectedErr: false,
		},
		{
			name: "Autoscaling with failure domains",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testNutanixClusterDeployment()
				cd.Spec.Platform.Nutanix.FailureDomains = []hivev1nutanix.FailureDomain{
					{
						Name: "fd1",
						PrismElement: hivev1nutanix.PrismElement{
							UUID: "test-prism-element-uuid-1",
							Endpoint: hivev1nutanix.PrismEndpoint{
								Address: "prism1.example.com",
								Port:    9440,
							},
						},
					},
					{
						Name: "fd2",
						PrismElement: hivev1nutanix.PrismElement{
							UUID: "test-prism-element-uuid-2",
							Endpoint: hivev1nutanix.PrismEndpoint{
								Address: "prism2.example.com",
								Port:    9440,
							},
						},
					},
					{
						Name: "fd3",
						PrismElement: hivev1nutanix.PrismElement{
							UUID: "test-prism-element-uuid-3",
							Endpoint: hivev1nutanix.PrismEndpoint{
								Address: "prism3.example.com",
								Port:    9440,
							},
						},
					},
				}
				return cd
			}(),
			pool: func() *hivev1.MachinePool {
				p := testNutanixPool()
				p.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 3,
					MaxReplicas: 9,
				}
				p.Spec.Platform.Nutanix.FailureDomains = []string{"fd1", "fd2", "fd3"}
				return p
			}(),
			expectedErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("actuator", "nutanixactuator_test")
			fakeClient := fake.NewFakeClientBuilder().WithRuntimeObjects(test.pool).Build()

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

// TODO: Fold this functionality into FakeClientWithCustomErrors
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
	baseClient := fake.NewFakeClientBuilder().Build()
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
	baseClient := fake.NewFakeClientBuilder().Build()
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

func TestNutanixActuator_AutoscalingGeneratesMachineSets(t *testing.T) {
	tests := []struct {
		name                    string
		pool                    *hivev1.MachinePool
		clusterDeployment       *hivev1.ClusterDeployment
		expectedMachineSetCount int
		expectedTotalReplicas   int32
	}{
		{
			name: "Autoscaling without failure domains creates one MachineSet",
			pool: func() *hivev1.MachinePool {
				p := testNutanixPool()
				p.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 1,
					MaxReplicas: 5,
				}
				return p
			}(),
			clusterDeployment:       testNutanixClusterDeployment(),
			expectedMachineSetCount: 1,
			expectedTotalReplicas:   1,
		},
		{
			name: "Autoscaling with three failure domains creates three MachineSets",
			pool: func() *hivev1.MachinePool {
				p := testNutanixPool()
				p.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
					MinReplicas: 3,
					MaxReplicas: 9,
				}
				p.Spec.Platform.Nutanix.FailureDomains = []string{"fd1", "fd2", "fd3"}
				return p
			}(),
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testNutanixClusterDeployment()
				cd.Spec.Platform.Nutanix.FailureDomains = []hivev1nutanix.FailureDomain{
					{
						Name: "fd1",
						PrismElement: hivev1nutanix.PrismElement{
							UUID: "test-prism-element-uuid-1",
							Endpoint: hivev1nutanix.PrismEndpoint{
								Address: "prism1.example.com",
								Port:    9440,
							},
						},
					},
					{
						Name: "fd2",
						PrismElement: hivev1nutanix.PrismElement{
							UUID: "test-prism-element-uuid-2",
							Endpoint: hivev1nutanix.PrismEndpoint{
								Address: "prism2.example.com",
								Port:    9440,
							},
						},
					},
					{
						Name: "fd3",
						PrismElement: hivev1nutanix.PrismElement{
							UUID: "test-prism-element-uuid-3",
							Endpoint: hivev1nutanix.PrismEndpoint{
								Address: "prism3.example.com",
								Port:    9440,
							},
						},
					},
				}
				return cd
			}(),
			expectedMachineSetCount: 3,
			expectedTotalReplicas:   3,
		},
		{
			name: "Non-autoscaling pool still works",
			pool: func() *hivev1.MachinePool {
				p := testNutanixPool()
				replicas := int64(2)
				p.Spec.Replicas = &replicas
				p.Spec.Platform.Nutanix.FailureDomains = []string{"fd1", "fd2"}
				return p
			}(),
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testNutanixClusterDeployment()
				cd.Spec.Platform.Nutanix.FailureDomains = []hivev1nutanix.FailureDomain{
					{
						Name: "fd1",
						PrismElement: hivev1nutanix.PrismElement{
							UUID: "test-prism-element-uuid-1",
							Endpoint: hivev1nutanix.PrismEndpoint{
								Address: "prism1.example.com",
								Port:    9440,
							},
						},
					},
					{
						Name: "fd2",
						PrismElement: hivev1nutanix.PrismElement{
							UUID: "test-prism-element-uuid-2",
							Endpoint: hivev1nutanix.PrismEndpoint{
								Address: "prism2.example.com",
								Port:    9440,
							},
						},
					},
				}
				return cd
			}(),
			expectedMachineSetCount: 2,
			expectedTotalReplicas:   2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("actuator", "nutanixactuator_test")
			fakeClient := fake.NewFakeClientBuilder().WithRuntimeObjects(test.pool).Build()

			actuator, err := NewNutanixActuator(fakeClient, getMasterMachineWithImage(t))
			require.NoError(t, err, "unexpected error creating NutanixActuator")

			cd := test.clusterDeployment
			cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{InfraID: "test-infra"}

			machineSets, proceed, err := actuator.GenerateMachineSets(cd, test.pool, logger)

			require.NoError(t, err, "expected no error for test case")
			assert.True(t, proceed, "expected actuator to proceed")
			assert.Len(t, machineSets, test.expectedMachineSetCount, "unexpected number of MachineSets")

			// Verify total replicas across all MachineSets
			var totalReplicas int32
			for _, ms := range machineSets {
				assert.NotNil(t, ms.Spec.Replicas, "MachineSet replicas should not be nil")
				totalReplicas += *ms.Spec.Replicas
			}
			assert.Equal(t, test.expectedTotalReplicas, totalReplicas, "unexpected total replicas across MachineSets")

			// For autoscaling tests, verify that MachineSets are created even though original pool had nil replicas
			if test.pool.Spec.Autoscaling != nil {
				assert.Greater(t, len(machineSets), 0, "autoscaling should generate at least one MachineSet")
			}
		})
	}
}
