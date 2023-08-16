package machinepool

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	compute "google.golang.org/api/compute/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"

	machineapi "github.com/openshift/api/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	gcpclient "github.com/openshift/hive/pkg/gcpclient"
	mockgcp "github.com/openshift/hive/pkg/gcpclient/mock"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	gcpCredsSecretName = "gcp-credentials"
	testProjectID      = "test-gcp-project-id"
	testNetworkID      = "test-gcp-network-id"
	testSubnetID       = "test-subnet-id"
)

func TestGCPActuator(t *testing.T) {
	tests := []struct {
		name                            string
		pool                            *hivev1.MachinePool
		requireLeases                   bool
		existing                        []runtime.Object
		mockGCPClient                   func(*mockgcp.MockClient)
		setupPendingCreationExpectation bool

		expectedMachineSetReplicas map[string]int64
		expectedErr                bool
	}{
		{
			name: "generate single machineset for single zone",
			pool: testGCPPool(testPoolName),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeZones(client, []string{"zone1"}, testRegion)
			},
			expectedMachineSetReplicas: map[string]int64{
				generateGCPMachineSetName("worker", "zone1"): 3,
			},
		},
		{
			name: "generate machinesets across zones",
			pool: testGCPPool(testPoolName),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeZones(client, []string{"zone1", "zone2", "zone3"}, testRegion)
			},
			expectedMachineSetReplicas: map[string]int64{
				generateGCPMachineSetName("worker", "zone1"): 1,
				generateGCPMachineSetName("worker", "zone2"): 1,
				generateGCPMachineSetName("worker", "zone3"): 1,
			},
		},
		{
			name: "generate machinesets for specified zones",
			pool: func() *hivev1.MachinePool {
				pool := testGCPPool(testPoolName)
				pool.Spec.Platform.GCP.Zones = []string{"zone1", "zone2", "zone3"}
				return pool
			}(),
			expectedMachineSetReplicas: map[string]int64{
				generateGCPMachineSetName("worker", "zone1"): 1,
				generateGCPMachineSetName("worker", "zone2"): 1,
				generateGCPMachineSetName("worker", "zone3"): 1,
			},
		},
		{
			name: "list zones returns zero",
			pool: testGCPPool(testPoolName),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeZones(client, []string{}, testRegion)
			},
			expectedErr: true,
		},
		{
			name: "generate machinesets for existing lease",
			pool: testGCPPool(testPoolName),
			existing: []runtime.Object{
				testPoolLease(testPoolName, testName, testInfraID, "w"),
			},
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeZones(client, []string{"zone1"}, testRegion)
			},
			expectedMachineSetReplicas: map[string]int64{
				generateGCPMachineSetName("w", "zone1"): 3,
			},
		},
		{
			name: "generate machinesets for different lease",
			pool: func() *hivev1.MachinePool {
				pool := testGCPPool("additional-compute")
				pool.Spec.Platform.GCP.Zones = []string{"zone1", "zone2", "zone3"}
				return pool
			}(),
			existing: []runtime.Object{
				testPoolLease("additional-compute", testName, testInfraID, "r"),
			},
			expectedMachineSetReplicas: map[string]int64{
				generateGCPMachineSetName("r", "zone1"): 1,
				generateGCPMachineSetName("r", "zone2"): 1,
				generateGCPMachineSetName("r", "zone3"): 1,
			},
		},
		{
			name:                            "no lease pending create expectation",
			pool:                            testGCPPool(testPoolName),
			requireLeases:                   true,
			setupPendingCreationExpectation: true,
		},
		{
			name: "generate machinesets with explicit disk type and size",
			pool: func() *hivev1.MachinePool {
				pool := testGCPPool(testPoolName)
				pool.Spec.Platform.GCP.OSDisk = hivev1gcp.OSDisk{
					DiskType:   "faketype",
					DiskSizeGB: 2000,
				}
				return pool
			}(),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeZones(client, []string{"zone1"}, testRegion)
			},
			expectedMachineSetReplicas: map[string]int64{
				generateGCPMachineSetName("worker", "zone1"): 3,
			},
		},
		{
			name: "generate machinesets with KMS disk encryption",
			pool: func() *hivev1.MachinePool {
				pool := testGCPPool(testPoolName)
				pool.Spec.Platform.GCP.OSDisk = hivev1gcp.OSDisk{
					DiskType:   "faketype",
					DiskSizeGB: 2000,
					EncryptionKey: &hivev1gcp.EncryptionKeyReference{
						KMSKey: &hivev1gcp.KMSKeyReference{
							Name:      "foo",
							KeyRing:   "keyring",
							ProjectID: "myproject",
							Location:  "Canada",
						},
						KMSKeyServiceAccount: "kmssa",
					},
				}
				return pool
			}(),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeZones(client, []string{"zone1"}, testRegion)
			},
			expectedMachineSetReplicas: map[string]int64{
				generateGCPMachineSetName("worker", "zone1"): 3,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)

			gClient := mockgcp.NewMockClient(mockCtrl)
			clusterDeployment := testGCPClusterDeployment(testName, testInfraID)

			logger := log.WithField("actuator", "gcpactuator")
			controllerExpectations := controllerutils.NewExpectations(logger)
			if test.setupPendingCreationExpectation {
				controllerExpectations.ExpectCreations(types.NamespacedName{
					Name:      test.pool.Name,
					Namespace: testNamespace,
				}.String(), 1)
			}

			test.existing = append(test.existing, clusterDeployment)
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()

			// set up mock expectations
			if test.mockGCPClient != nil {
				test.mockGCPClient(gClient)
			}

			ga := &GCPActuator{
				gcpClient:      gClient,
				logger:         logger,
				client:         fakeClient,
				scheme:         scheme,
				expectations:   controllerExpectations,
				projectID:      testProjectID,
				leasesRequired: test.requireLeases,
				network:        testNetworkID,
				subnet:         testSubnetID,
			}

			generatedMachineSets, _, err := ga.GenerateMachineSets(clusterDeployment, test.pool, ga.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				assert.Equal(t, len(test.expectedMachineSetReplicas), len(generatedMachineSets), "different number of machine sets generated than expected")

				for _, ms := range generatedMachineSets {
					expectedReplicas, ok := test.expectedMachineSetReplicas[ms.Name]
					if assert.True(t, ok, "unexpected machine set: ", ms.Name) {
						assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
					}

					gcpProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*machineapi.GCPMachineProviderSpec)
					assert.True(t, ok, "failed to convert to gcpProviderSpec")

					assert.Equal(t, testInstanceType, gcpProvider.MachineType, "unexpected instance type")

					// Ensure network details are propagated correctly.
					assert.Equal(t, ga.network, gcpProvider.NetworkInterfaces[0].Network)
					assert.Equal(t, ga.subnet, gcpProvider.NetworkInterfaces[0].Subnetwork)

					// Ensure GCP disk type and size was correctly set or defaulted and made it to the resulting MachineSets:
					expectedDiskType := test.pool.Spec.Platform.GCP.OSDisk.DiskType
					expectedDiskSizeGB := test.pool.Spec.Platform.GCP.OSDisk.DiskSizeGB
					if expectedDiskType == "" {
						expectedDiskType = defaultGCPDiskType
					}
					if expectedDiskSizeGB == 0 {
						expectedDiskSizeGB = defaultGCPDiskSizeGB
					}
					assert.Equal(t, expectedDiskType, gcpProvider.Disks[0].Type)
					assert.Equal(t, expectedDiskSizeGB, gcpProvider.Disks[0].SizeGB)

					// Ensure GCP disk encryption settings made it to the resulting MachineSet (if specified):
					encKey := test.pool.Spec.Platform.GCP.OSDisk.EncryptionKey
					if encKey != nil {
						assert.Equal(t, encKey.KMSKeyServiceAccount, gcpProvider.Disks[0].EncryptionKey.KMSKeyServiceAccount)
						assert.Equal(t, encKey.KMSKey.Name, gcpProvider.Disks[0].EncryptionKey.KMSKey.Name)
						assert.Equal(t, encKey.KMSKey.KeyRing, gcpProvider.Disks[0].EncryptionKey.KMSKey.KeyRing)
						assert.Equal(t, encKey.KMSKey.ProjectID, gcpProvider.Disks[0].EncryptionKey.KMSKey.ProjectID)
						assert.Equal(t, encKey.KMSKey.Location, gcpProvider.Disks[0].EncryptionKey.KMSKey.Location)
					}

				}
			}
		})
	}
}

func TestFindAvailableLeaseChars(t *testing.T) {
	var (
		cluster1Name          = "cluster1"
		cluster1InfraID       = "clust1-x92la"
		cluster1Pool1Name     = "cluster1-worker"
		cluster1Pool1SpecName = "worker"
		cluster1Pool2Name     = "cluster1-worker2"
		cluster1Pool3Name     = "cluster1-worker3"
		cluster1Pool4Name     = "cluster1-worker4"

		cluster2Name      = "cluster2"
		cluster2InfraID   = "clust2-lap0q"
		cluster2Pool1Name = "cluster2-worker"
		cluster2Pool2Name = "cluster2-additional-workers"
	)
	tests := []struct {
		name string
		// clusterDeployment referenced by name, must be created in the existing array:
		clusterDeployment string
		existingLeases    []hivev1.MachinePoolNameLease
		existing          []runtime.Object // existing resources excluding leases
		expectedAvailable string
	}{
		{
			name:              "test all chars available",
			clusterDeployment: cluster1Name,
			existing: []runtime.Object{
				testGCPClusterDeployment(cluster1Name, cluster1InfraID),
				testGCPPoolForCluster(cluster1Pool1Name, cluster1Pool1SpecName, cluster1Name),
			},
			expectedAvailable: "abcdefghijklnopqrstuvxyz0123456789",
		},
		{
			name:              "test some chars leased",
			clusterDeployment: cluster1Name,
			existing: []runtime.Object{
				testGCPClusterDeployment(cluster1Name, cluster1InfraID),
				testGCPPoolForCluster(cluster1Pool1Name, cluster1Pool1SpecName, cluster1Name),
			},
			existingLeases: []hivev1.MachinePoolNameLease{
				*testPoolLease(cluster1Pool1Name, cluster1Name, cluster1InfraID, "q"),
				*testPoolLease(cluster1Pool2Name, cluster1Name, cluster1InfraID, "r"),
				*testPoolLease(cluster1Pool3Name, cluster1Name, cluster1InfraID, "s"),
				*testPoolLease(cluster1Pool4Name, cluster1Name, cluster1InfraID, "t"),
			},
			expectedAvailable: "abcdefghijklnopuvxyz0123456789",
		},
		{
			name:              "test some chars leased multi cluster",
			clusterDeployment: cluster1Name,
			existing: []runtime.Object{
				testGCPClusterDeployment(cluster1Name, cluster1InfraID),
				testGCPClusterDeployment(cluster2Name, cluster2InfraID),
				testGCPPoolForCluster(cluster1Pool1Name, cluster1Pool1SpecName, cluster1Name),
			},
			existingLeases: []hivev1.MachinePoolNameLease{
				*testPoolLease(cluster1Pool1Name, cluster1Name, cluster1InfraID, "q"),
				*testPoolLease(cluster1Pool2Name, cluster1Name, cluster1InfraID, "r"),
				*testPoolLease(cluster1Pool3Name, cluster1Name, cluster1InfraID, "s"),
				*testPoolLease(cluster1Pool4Name, cluster1Name, cluster1InfraID, "t"),
				// These pool 2 leases should not impact our expected results for pool 1.
				*testPoolLease(cluster2Pool1Name, cluster2Name, cluster2InfraID, "a"),
				*testPoolLease(cluster2Pool2Name, cluster2Name, cluster2InfraID, "b"),
			},
			expectedAvailable: "abcdefghijklnopuvxyz0123456789",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()
			ga := &GCPActuator{
				logger: log.WithField("actuator", "gcpactuator"),
				client: fakeClient,
				scheme: scheme,
			}

			cd := &hivev1.ClusterDeployment{}
			err := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: test.clusterDeployment}, cd)
			require.NoError(t, err)
			leaseList := &hivev1.MachinePoolNameLeaseList{
				Items: test.existingLeases,
			}
			availChars, err := ga.findAvailableLeaseChars(cd, leaseList)
			require.NoError(t, err)
			require.Equal(t, len(test.expectedAvailable), len(availChars))
			for _, char := range test.expectedAvailable {
				require.Contains(t, availChars, rune(char), "availableChars missing %s", string(char))
			}
		})
	}
}

func TestObtainLeaseChar(t *testing.T) {
	var (
		cluster1Name          = "cluster1"
		cluster1InfraID       = "clust1-x92la"
		cluster1Pool1Name     = "cluster1-worker"
		cluster1Pool1SpecName = "worker"
		cluster1Pool2Name     = "cluster1-worker2"
		cluster1Pool2SpecName = "worker2"
		cluster1Pool3Name     = "cluster1-worker3"
		cluster1Pool4Name     = "cluster1-worker4"
	)
	tests := []struct {
		name string
		// pool referenced by name, must be created in the existing array. This field should be of the form clusterName-poolName
		poolName   string
		existingCD *hivev1.ClusterDeployment
		existing   []runtime.Object

		expectedCharIn  string
		expectedInfraID string

		expectCondition *hivev1.MachinePoolCondition

		expectErr             bool
		expectProceed         bool
		expectationsSatisfied bool
		pendingCreation       bool
	}{
		{
			name:       "worker pool needs lease for w",
			poolName:   cluster1Pool1Name,
			existingCD: testGCPClusterDeployment(cluster1Name, cluster1InfraID),
			existing: []runtime.Object{
				testGCPPoolForCluster(cluster1Pool1Name, cluster1Pool1SpecName, cluster1Name),
			},
			// "w" should always be selected for the original worker pool
			expectedCharIn:        "w",
			expectedInfraID:       cluster1InfraID,
			expectProceed:         false,
			expectationsSatisfied: false,
		},
		{
			name:       "worker pool has lease",
			poolName:   cluster1Pool1Name,
			existingCD: testGCPClusterDeployment(cluster1Name, cluster1InfraID),
			existing: []runtime.Object{
				testGCPPoolForCluster(cluster1Pool1Name, cluster1Pool1SpecName, cluster1Name),
				testPoolLease(cluster1Pool1Name, cluster1Name, cluster1InfraID, "w"),
			},
			expectedCharIn:        "w",
			expectedInfraID:       cluster1InfraID,
			expectProceed:         true,
			expectationsSatisfied: true,
		},
		{
			name:       "additional pool needs lease",
			poolName:   cluster1Pool2Name,
			existingCD: testGCPClusterDeployment(cluster1Name, cluster1InfraID),
			existing: []runtime.Object{
				testGCPPoolForCluster(cluster1Pool2Name, cluster1Pool2SpecName, cluster1Name),
			},
			expectedCharIn:        "abcdefghijklnopqrstuvxyz0123456789",
			expectedInfraID:       cluster1InfraID,
			expectProceed:         false,
			expectationsSatisfied: false,
		},
		{
			name:       "additional pool needs lease clear no lease available condition",
			poolName:   cluster1Pool2Name,
			existingCD: testGCPClusterDeployment(cluster1Name, cluster1InfraID),
			existing: []runtime.Object{
				func() *hivev1.MachinePool {
					p := testGCPPoolForCluster(cluster1Pool2Name, cluster1Pool2SpecName, cluster1Name)
					p.Status.Conditions = []hivev1.MachinePoolCondition{
						{Type: hivev1.NoMachinePoolNameLeasesAvailable, Status: corev1.ConditionTrue},
					}
					return p
				}(),
			},
			expectedCharIn:        "abcdefghijklnopqrstuvxyz0123456789",
			expectedInfraID:       cluster1InfraID,
			expectProceed:         false,
			expectCondition:       &hivev1.MachinePoolCondition{Type: hivev1.NoMachinePoolNameLeasesAvailable, Status: corev1.ConditionFalse},
			expectationsSatisfied: false,
		},
		{
			name:       "additional pool needs lease expecting creation",
			poolName:   cluster1Pool2Name,
			existingCD: testGCPClusterDeployment(cluster1Name, cluster1InfraID),
			existing: []runtime.Object{
				testGCPPoolForCluster(cluster1Pool2Name, cluster1Pool2SpecName, cluster1Name),
			},
			expectedCharIn:        "abcdefghijklnopqrstuvxyz0123456789",
			expectedInfraID:       cluster1InfraID,
			expectProceed:         false,
			pendingCreation:       true,
			expectationsSatisfied: false,
		},
		{
			name:       "additional pool has lease",
			poolName:   cluster1Pool2Name,
			existingCD: testGCPClusterDeployment(cluster1Name, cluster1InfraID),
			existing: []runtime.Object{
				testGCPPoolForCluster(cluster1Pool2Name, cluster1Pool2SpecName, cluster1Name),
				testPoolLease(cluster1Pool2Name, cluster1Name, cluster1InfraID, "q"),
			},
			expectedCharIn:        "q",
			expectedInfraID:       cluster1InfraID,
			expectProceed:         true,
			expectationsSatisfied: true,
		},
		{
			name:       "additional pool has lease with malformed name",
			poolName:   cluster1Pool2Name,
			existingCD: testGCPClusterDeployment(cluster1Name, cluster1InfraID),
			existing: []runtime.Object{
				testGCPPoolForCluster(cluster1Pool2Name, cluster1Pool2SpecName, cluster1Name),
				testPoolLease(cluster1Pool2Name, cluster1Name, "badinfraid", "q"),
			},
			expectErr:             true,
			expectProceed:         false,
			expectationsSatisfied: true,
		},
		{
			name:       "additional pool needs lease some exist",
			poolName:   cluster1Pool2Name,
			existingCD: testGCPClusterDeployment(cluster1Name, cluster1InfraID),
			existing: []runtime.Object{
				testGCPPoolForCluster(cluster1Pool2Name, cluster1Pool2SpecName, cluster1Name),
				testPoolLease(cluster1Pool1Name, cluster1Name, cluster1InfraID, "w"),
				testPoolLease(cluster1Pool3Name, cluster1Name, cluster1InfraID, "s"),
				testPoolLease(cluster1Pool4Name, cluster1Name, cluster1InfraID, "t"),
			},
			expectedCharIn:        "abcdefghijklnopqruvxyz0123456789",
			expectedInfraID:       cluster1InfraID,
			expectProceed:         false,
			expectationsSatisfied: false,
		},
		{
			name:       "all lease chars but one in use",
			poolName:   cluster1Pool2Name,
			existingCD: testGCPClusterDeployment(cluster1Name, cluster1InfraID),
			existing: func() []runtime.Object {
				objects := []runtime.Object{
					testGCPPoolForCluster(cluster1Pool2Name, cluster1Pool2SpecName, cluster1Name),
					testPoolLease(cluster1Pool1Name, cluster1Name, cluster1InfraID, "w"),
				}
				// '9' is free:
				for _, c := range "abcdefghijklnopqrstuvxyz012345678" {
					objects = append(objects, testPoolLease(fmt.Sprintf("%s-pool-%s", cluster1Name, string(c)), cluster1Name, cluster1InfraID, string(c)))
				}
				return objects
			}(),
			expectedCharIn:        "9",
			expectedInfraID:       cluster1InfraID,
			expectProceed:         false,
			expectationsSatisfied: false,
		},
		{
			name:       "all lease chars in use",
			poolName:   cluster1Pool2Name,
			existingCD: testGCPClusterDeployment(cluster1Name, cluster1InfraID),
			existing: func() []runtime.Object {
				objects := []runtime.Object{
					testGCPPoolForCluster(cluster1Pool2Name, cluster1Pool2SpecName, cluster1Name),
					testPoolLease(cluster1Pool1Name, cluster1Name, cluster1InfraID, "w"),
				}
				for _, c := range "abcdefghijklnopqrstuvxyz0123456789" {
					objects = append(objects, testPoolLease(fmt.Sprintf("%s-pool-%s", cluster1Name, string(c)), cluster1Name, cluster1InfraID, string(c)))
				}
				return objects
			}(),
			expectErr:     false,
			expectProceed: false,
			expectCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.NoMachinePoolNameLeasesAvailable,
				Status: corev1.ConditionTrue,
			},
			expectationsSatisfied: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()

			logger := log.WithField("actuator", "gcpactuator")
			controllerExpectations := controllerutils.NewExpectations(logger)

			expectationsKey := types.NamespacedName{
				Name:      test.poolName,
				Namespace: testNamespace,
			}.String()
			if test.pendingCreation {
				controllerExpectations.ExpectCreations(expectationsKey, 1)
			}

			ga := &GCPActuator{
				logger:       log.WithField("actuator", "gcpactuator"),
				client:       fakeClient,
				scheme:       scheme,
				expectations: controllerExpectations,
			}

			pool := &hivev1.MachinePool{}
			err := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: test.poolName}, pool)
			require.NoError(t, err)

			leases := &hivev1.MachinePoolNameLeaseList{}
			err = fakeClient.List(context.TODO(), leases)
			require.NoError(t, err)

			leaseChar, proceed, err := ga.obtainLease(pool, test.existingCD, leases)
			if test.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.expectProceed, proceed, "unexpected proceed result")

			assert.Equal(t, test.expectationsSatisfied, controllerExpectations.SatisfiedExpectations(
				types.NamespacedName{Namespace: testNamespace, Name: test.poolName}.String()), "unexpected expectations result")

			require.Contains(t, test.expectedCharIn, string(leaseChar))

			if test.expectCondition != nil {
				cond := controllerutils.FindCondition(pool.Status.Conditions, test.expectCondition.Type)
				if assert.NotNilf(t, cond, "did not find expected condition type: %v", test.expectCondition.Type) {
					assert.Equal(t, test.expectCondition.Status, cond.Status, "condition found with unexpected status")
				}
			}

			if test.expectProceed {
				// ensure the lease exists:
				lease := &hivev1.MachinePoolNameLease{}
				err = fakeClient.Get(context.TODO(), types.NamespacedName{
					Namespace: testNamespace,
					Name:      fmt.Sprintf("%s-%s", test.expectedInfraID, string(leaseChar)),
				}, lease)
				require.NoError(t, err)

				// ensure labels we expect are set
				require.Equal(t, test.poolName, lease.Labels[constants.MachinePoolNameLabel])
				require.Equal(t, test.existingCD.Name, lease.Labels[constants.ClusterDeploymentNameLabel])

				// ensure owner reference is correct
				require.Equal(t, 1, len(lease.OwnerReferences), "unexpected ownerreferences count")
				require.Equal(t, "hive.openshift.io/v1", lease.OwnerReferences[0].APIVersion)
				require.Equal(t, "MachinePool", lease.OwnerReferences[0].Kind)
				require.Equal(t, test.poolName, lease.OwnerReferences[0].Name)
			}

		})
	}
}

func TestRequireLeases(t *testing.T) {
	cases := []struct {
		name            string
		clusterVersion  string
		machineSetNames []string
		expectedResult  bool
	}{
		{
			name:           "before 4.4.7",
			clusterVersion: "4.4.6",
			expectedResult: true,
		},
		{
			name:           "4.4.7",
			clusterVersion: "4.4.7",
			expectedResult: false,
		},
		{
			name:           "after 4.4.7",
			clusterVersion: "4.4.8",
			expectedResult: false,
		},
		{
			name:           "4.5",
			clusterVersion: "4.5.0",
			expectedResult: false,
		},
		{
			name:           "after 4.5",
			clusterVersion: "4.6.0",
			expectedResult: false,
		},
		{
			name:           "invalid version",
			clusterVersion: "bad-version",
			expectedResult: false,
		},
		{
			name:            "worker machine pool",
			clusterVersion:  "4.5.0",
			machineSetNames: []string{"cluster-id-worker-a", "cluster-id-worker-b"},
			expectedResult:  false,
		},
		{
			name:            "w machine pool",
			clusterVersion:  "4.5.0",
			machineSetNames: []string{"cluster-id-w-a", "cluster-id-w-a"},
			expectedResult:  true,
		},
		{
			name:            "worker and w machine pools",
			clusterVersion:  "4.5.0",
			machineSetNames: []string{"cluster-id-worker-a", "cluster-id-worker-a", "cluster-id-w-a", "cluster-id-w-a"},
			expectedResult:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			machineSets := make([]machineapi.MachineSet, len(tc.machineSetNames))
			for i, n := range tc.machineSetNames {
				machineSets[i].Name = n
			}
			actualResult := requireLeases(tc.clusterVersion, machineSets, log.WithFields(nil))
			assert.Equal(t, tc.expectedResult, actualResult)
		})
	}
}

func TestGetNetwork(t *testing.T) {
	cases := []struct {
		name              string
		remoteMachineSets []machineapi.MachineSet
		expectError       bool
	}{
		{
			name:              "valid remote machinesets",
			remoteMachineSets: []machineapi.MachineSet{mockMachineSet("worker1", "worker", false, 1, 0)},
		},
		{
			name: "invalid remote machineset",
			remoteMachineSets: func() []machineapi.MachineSet {
				ms := []machineapi.MachineSet{mockMachineSet("worker1", "worker", false, 1, 0)}
				ms[0].Spec.Template.Spec.ProviderSpec.Value = nil
				return ms
			}(),
			expectError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := scheme.GetScheme()
			network, subnet, actualErr := getNetwork(tc.remoteMachineSets, scheme, log.StandardLogger())
			if tc.expectError {
				assert.Error(t, actualErr, "expected an error")
			} else {
				if assert.NoError(t, actualErr, "unexpected error") {
					assert.Equal(t, testNetworkID, network, "unexpected network ID")
					assert.Equal(t, testSubnetID, subnet, "unexpected subnet ID")
				}
			}
		})
	}
}

func mockMachineSet(name string, machineType string, unstompedAnnotation bool, replicas int, generation int) machineapi.MachineSet {
	msReplicas := int32(replicas)
	ms := machineapi.MachineSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: machineapi.SchemeGroupVersion.String(),
			Kind:       "MachineSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: machineAPINamespace,
			Labels: map[string]string{
				machinePoolNameLabel:                       machineType,
				"machine.openshift.io/cluster-api-cluster": testInfraID,
				constants.HiveManagedLabel:                 "true",
			},
			Generation: int64(generation),
		},

		Spec: machineapi.MachineSetSpec{
			Replicas: &msReplicas,
			Template: machineapi.MachineTemplateSpec{
				Spec: mockMachineSpec(machineType),
			},
		},
	}
	// Add a pre-existing annotation which we will ensure remains in updated machinesets.
	if unstompedAnnotation {
		ms.Annotations = map[string]string{
			"hive.openshift.io/unstomped": "true",
		}
	}
	return ms
}

func mockMachineSpec(machineType string) machineapi.MachineSpec {
	rawGCPProviderSpec, err := encodeGCPMachineProviderSpec(testGCPProviderSpec(), scheme.GetScheme())
	if err != nil {
		log.WithError(err).Fatal("error encoding GCP machine provider spec")
	}
	return machineapi.MachineSpec{
		ObjectMeta: machineapi.ObjectMeta{
			Labels: map[string]string{
				"machine.openshift.io/cluster-api-cluster":      testInfraID,
				"machine.openshift.io/cluster-api-machine-role": machineType,
				"machine.openshift.io/cluster-api-machine-type": machineType,
			},
		},
		ProviderSpec: machineapi.ProviderSpec{
			Value: rawGCPProviderSpec,
		},

		Taints: []corev1.Taint{
			{
				Key:    "foo",
				Value:  "bar",
				Effect: corev1.TaintEffectNoSchedule,
			},
		},
	}
}

func testGCPProviderSpec() *machineapi.GCPMachineProviderSpec {
	return &machineapi.GCPMachineProviderSpec{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GCPMachineProviderSpec",
			APIVersion: machineapi.SchemeGroupVersion.String(),
		},
		NetworkInterfaces: []*machineapi.GCPNetworkInterface{
			{
				Network:    testNetworkID,
				Subnetwork: testSubnetID,
			},
		},
	}
}

func encodeGCPMachineProviderSpec(gcoProviderSpec *machineapi.GCPMachineProviderSpec, scheme *runtime.Scheme) (*runtime.RawExtension, error) {
	serializer := jsonserializer.NewSerializer(jsonserializer.DefaultMetaFactory, scheme, scheme, false)
	var buffer bytes.Buffer
	err := serializer.Encode(gcoProviderSpec, &buffer)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{
		Raw: buffer.Bytes(),
	}, nil
}

func mockListComputeZones(gClient *mockgcp.MockClient, zones []string, region string) {
	zoneList := &compute.ZoneList{}

	for _, zone := range zones {
		zoneList.Items = append(zoneList.Items,
			&compute.Zone{
				Name: zone,
			})
	}

	filter := gcpclient.ListComputeZonesOptions{
		Filter: fmt.Sprintf("(region eq '.*%s.*') (status eq UP)", region),
	}
	gClient.EXPECT().ListComputeZones(gomock.Eq(filter)).Return(
		zoneList, nil,
	)
}

func generateGCPMachineSetName(leaseChar, zone string) string {
	return fmt.Sprintf("%s-%s-%s", testInfraID, leaseChar, zone)
}

func testGCPPool(name string) *hivev1.MachinePool {
	p := testMachinePool()
	p.Name = name
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		GCP: &hivev1gcp.MachinePool{
			InstanceType: testInstanceType,
		},
	}
	return p
}

func testGCPPoolForCluster(poolName, poolSpecName, clusterName string) *hivev1.MachinePool {
	p := testMachinePool()
	// validation ensures that all machine pools must be named [cdname]-[spec.name]
	p.Name = poolName
	p.Spec.Name = poolSpecName
	p.Spec.ClusterDeploymentRef.Name = clusterName
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		GCP: &hivev1gcp.MachinePool{
			InstanceType: testInstanceType,
		},
	}
	return p
}

func testGCPClusterDeployment(clusterName, infraID string) *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Name = clusterName
	cd.Spec.ClusterName = clusterName
	cd.Spec.ClusterMetadata.InfraID = infraID
	cd.Spec.Platform = hivev1.Platform{
		GCP: &hivev1gcp.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: gcpCredsSecretName,
			},
			Region: testRegion,
		},
	}
	return cd
}

func testPoolLease(poolOwnerName, cdName, infraID, leaseChar string) *hivev1.MachinePoolNameLease {
	return &hivev1.MachinePoolNameLease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", infraID, leaseChar),
			Namespace: testNamespace,
			Labels: map[string]string{
				constants.ClusterDeploymentNameLabel: cdName,
				constants.MachinePoolNameLabel:       poolOwnerName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "hive.openshift.io/v1",
					Kind:       "MachinePool",
					Name:       poolOwnerName,
					// skipping some owner reference fields we will not examine
				},
			},
		},
	}
}
