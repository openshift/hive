package clusterpool

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	testclaim "github.com/openshift/hive/pkg/test/clusterclaim"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testcdc "github.com/openshift/hive/pkg/test/clusterdeploymentcustomization"
	testcp "github.com/openshift/hive/pkg/test/clusterpool"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	testsecret "github.com/openshift/hive/pkg/test/secret"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	testNamespace     = "test-namespace"
	testLeasePoolName = "aws-us-east-1"
	credsSecretName   = "aws-creds"
	imageSetName      = "test-image-set"
	testFinalizer     = "test-finalizer"
)

func TestReconcileClusterPool(t *testing.T) {
	scheme := scheme.GetScheme()

	// See calculatePoolVersion. If these change, the easiest way to figure out the new value is
	// to pull it from the test failure :)
	initialPoolVersion := "04a8f79a4a2e9733"
	inventoryPoolVersion := "a6dcbd6776a3c92b"
	inventoryAndCustomizationPoolVersion := "f1a16535dbc27559"
	customizationPoolVersion := "564f99a732a771e9"
	openstackPoolVersion := "0be50b7ba396d313"

	poolBuilder := testcp.FullBuilder(testNamespace, testLeasePoolName, scheme).
		GenericOptions(
			testgeneric.WithFinalizer(finalizer),
		).
		Options(
			testcp.ForAWS(credsSecretName, "us-east-1"),
			testcp.WithBaseDomain("test-domain"),
			testcp.WithImageSet(imageSetName),
		)
	initializedPoolBuilder := poolBuilder.Options(
		testcp.WithCondition(hivev1.ClusterPoolCondition{
			Status: corev1.ConditionUnknown,
			Type:   hivev1.ClusterPoolMissingDependenciesCondition,
		}),
		testcp.WithCondition(hivev1.ClusterPoolCondition{
			Status: corev1.ConditionUnknown,
			Type:   hivev1.ClusterPoolCapacityAvailableCondition,
		}),
		testcp.WithCondition(hivev1.ClusterPoolCondition{
			Status: corev1.ConditionUnknown,
			Type:   hivev1.ClusterPoolAllClustersCurrentCondition,
		}),
		testcp.WithCondition(hivev1.ClusterPoolCondition{
			Status: corev1.ConditionUnknown,
			Type:   hivev1.ClusterPoolInventoryValidCondition,
		}),
		testcp.WithCondition(hivev1.ClusterPoolCondition{
			Status: corev1.ConditionUnknown,
			Type:   hivev1.ClusterPoolDeletionPossibleCondition,
		}),
	)

	inventoryPoolBuilder := func() testcp.Builder {
		return initializedPoolBuilder.Options(
			testcp.WithInventory([]string{"test-cdc-1"}),
			testcp.WithCondition(hivev1.ClusterPoolCondition{
				Status: corev1.ConditionUnknown,
				Type:   hivev1.ClusterPoolInventoryValidCondition,
			}),
		)
	}

	cdBuilder := func(name string) testcd.Builder {
		return testcd.FullBuilder(name, name, scheme).Options(
			testcd.WithPowerState(hivev1.ClusterPowerStateHibernating),
			testcd.WithPoolVersion(initialPoolVersion),
		)
	}
	unclaimedCDBuilder := func(name string) testcd.Builder {
		return cdBuilder(name).Options(
			testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
		)
	}

	nowish := time.Now()

	tests := []struct {
		name                               string
		existing                           []runtime.Object
		noClusterImageSet                  bool
		noCredsSecret                      bool
		expectError                        bool
		expectedPools                      []string
		expectedTotalClusters              int
		expectedObservedSize               int32
		expectedObservedReady              int32
		expectedDeletedClusters            []string
		expectFinalizerRemoved             bool
		expectedMissingDependenciesStatus  corev1.ConditionStatus
		expectedCapacityStatus             corev1.ConditionStatus
		expectedCDCurrentStatus            corev1.ConditionStatus
		expectedInventoryValidStatus       corev1.ConditionStatus
		expectedInventoryMessage           map[string][]string
		expectedDeletionPossibleCondition  *hivev1.ClusterPoolCondition
		expectedCDCFinalizers              map[string][]string
		expectedCDCReason                  map[string]string
		expectedPoolVersion                string
		expectedMissingDependenciesMessage string
		expectedAssignedClaims             int
		expectedUnassignedClaims           int
		expectedAssignedCDs                int
		expectedAssignedCDCs               map[string]string
		// This will be:
		// 2 if using inventory AND pool.Spec.CustomizationRef
		// 1 if using one xor the other
		// 0 if using neither
		expectedCDCsInCDNamespaces int
		expectedRunning            int
		expectedLabels             map[string]string // Tested on all clusters, so will not work if your test has pre-existing cds in the pool.
		// Map, keyed by claim name, of expected Status.Conditions['Pending'].Reason.
		// (The clusterpool controller always sets this condition's Status to True.)
		// Not checked if nil.
		expectedClaimPendingReasons      map[string]string
		expectedInventoryAssignmentOrder []string
		expectPoolVersionChanged         bool
	}{
		{
			name: "initialize conditions",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(1)),
			},
			expectedMissingDependenciesStatus: corev1.ConditionUnknown,
			expectedCapacityStatus:            corev1.ConditionUnknown,
			expectedCDCurrentStatus:           corev1.ConditionUnknown,
			expectedInventoryValidStatus:      corev1.ConditionUnknown,
			expectedDeletionPossibleCondition: &hivev1.ClusterPoolCondition{
				Status: corev1.ConditionUnknown,
			},
		},
		{
			name: "copyover fields",
			existing: []runtime.Object{
				// The test driver makes sure that "copyover fields" -- those we copy from the pool
				// spec to the CD spec -- match for CDs owned by the pool. This test case just
				// needs to a) set those fields in the pool spec, and b) have a nonzero size so the
				// pool actually creates CDs to compare.
				// TODO: Add coverage for more "copyover fields".
				initializedPoolBuilder.Build(
					testcp.WithSize(2),
					testcp.WithInstallAttemptsLimit(5),
					testcp.WithInstallerEnv(
						[]corev1.EnvVar{
							{
								Name:  "ENV1",
								Value: "VAL1",
							},
						},
					),
				),
			},
			expectedTotalClusters: 2,
		},
		{
			name: "poolVersion changes with Platform",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithPlatform(hivev1.Platform{
					AWS: &aws.Platform{
						CredentialsSecretRef: corev1.LocalObjectReference{Name: "foo"},
					},
				})),
			},
			expectPoolVersionChanged: true,
		},
		{
			name: "poolVersion changes with BaseDomain",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithBaseDomain("foo.example.com")),
			},
			expectPoolVersionChanged: true,
		},
		{
			name: "poolVersion changes with ImageSet",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithImageSet("abc123")),
			},
			expectPoolVersionChanged: true,
		},
		{
			name: "poolVersion changes with InstallConfigSecretTemplate",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithInstallConfigSecretTemplateRef("abc123")),
			},
			expectPoolVersionChanged: true,
		},
		{
			name: "cp with inventory and cdc exists is valid",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(),
			},
			expectedTotalClusters:        1,
			expectedObservedSize:         0,
			expectedObservedReady:        0,
			expectedInventoryValidStatus: corev1.ConditionTrue,
			expectedPoolVersion:          inventoryPoolVersion,
			expectedAssignedCDCs:         map[string]string{"test-cdc-1": testLeasePoolName},
			expectedCDCsInCDNamespaces:   1,
			expectedCDCReason:            map[string]string{"test-cdc-1": hivev1.CustomizationApplyReasonInstallationPending},
		},
		{
			name: "cp with inventory and customizationRef",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(
					testcp.WithSize(1),
					testcp.WithCustomizationRef("non-inventory-cdc")),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(),
				testcdc.FullBuilder(testNamespace, "non-inventory-cdc", scheme).Build(),
			},
			expectedTotalClusters:        1,
			expectedObservedSize:         0,
			expectedObservedReady:        0,
			expectedInventoryValidStatus: corev1.ConditionTrue,
			expectedPoolVersion:          inventoryAndCustomizationPoolVersion,
			expectedAssignedCDCs:         map[string]string{"test-cdc-1": testLeasePoolName},
			expectedCDCsInCDNamespaces:   2,
			expectedCDCReason:            map[string]string{"test-cdc-1": hivev1.CustomizationApplyReasonInstallationPending},
		},
		{
			name: "cp with customizationRef",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithCustomizationRef("non-inventory-cdc")),
				testcdc.FullBuilder(testNamespace, "non-inventory-cdc", scheme).Build(),
			},
			expectedTotalClusters:        1,
			expectedObservedSize:         0,
			expectedObservedReady:        0,
			expectedInventoryValidStatus: corev1.ConditionUnknown,
			expectedPoolVersion:          customizationPoolVersion,
			expectedCDCsInCDNamespaces:   1,
		},
		// TODO: Revise once https://issues.redhat.com/browse/HIVE-2284 solved
		/*{
			name: "cp with inventory and available cdc deleted without hold",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
				testcdc.FullBuilder(
					testNamespace, "test-cdc-1", scheme,
				).GenericOptions(testgeneric.Deleted()).Build(),
			},
			expectedTotalClusters:   0,
			expectedObservedSize:    0,
			expectedObservedReady:   0,
			expectedPoolVersion:     inventoryPoolVersion,
			expectError:             true,
			expectedCDCurrentStatus: corev1.ConditionUnknown,
		},*/
		{
			name: "cp with inventory and available cdc with finalizer deleted without hold",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
				testcdc.FullBuilder(
					testNamespace, "test-cdc-1", scheme,
				).GenericOptions(
					testgeneric.Deleted(),
					testgeneric.WithFinalizer(fmt.Sprintf("hive.openshift.io/%s", testLeasePoolName)),
				).Build(),
			},
			expectedTotalClusters:   0,
			expectedObservedSize:    0,
			expectedObservedReady:   0,
			expectedPoolVersion:     inventoryPoolVersion,
			expectError:             true,
			expectedCDCurrentStatus: corev1.ConditionUnknown,
		},
		{
			name: "cp with inventory and reserved cdc deletion on hold",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
				testcd.FullBuilder(testNamespace, "c1", scheme).Build(
					testcd.WithPoolVersion(inventoryPoolVersion),
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "claim"),
					testcd.WithCustomization("test-cdc-1"),
					testcd.Running(),
				),
				testcdc.FullBuilder(
					testNamespace, "test-cdc-1", scheme,
				).GenericOptions(
					testgeneric.Deleted(),
					testgeneric.WithFinalizer(finalizer),
				).Build(
					testcdc.WithPool(testLeasePoolName),
					testcdc.WithCD("c1"),
					testcdc.Reserved(),
				),
			},
			expectedTotalClusters:    1,
			expectedRunning:          1,
			expectedObservedSize:     0,
			expectedObservedReady:    0,
			expectedPoolVersion:      inventoryPoolVersion,
			expectError:              false,
			expectedCDCurrentStatus:  corev1.ConditionTrue,
			expectedAssignedCDs:      1,
			expectedUnassignedClaims: 0,
			expectedAssignedCDCs:     map[string]string{"test-cdc-1": "c1"},
		},
		{
			name: "finalizer will be added to cdc if missing",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
				initializedPoolBuilder.Build(
					testcp.Generic(testgeneric.WithName("test-cp-2")),
					testcp.WithInventory([]string{"test-cdc-2"}),
				),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(),
				testcdc.FullBuilder(testNamespace, "test-cdc-2", scheme).Build(),
			},
			expectedPools: []string{testLeasePoolName, "test-cp-2"},
			expectedCDCFinalizers: map[string][]string{
				"test-cdc-1": {fmt.Sprintf("hive.openshift.io/%s", testLeasePoolName)},
				"test-cdc-2": {"hive.openshift.io/test-cp-2"},
			},
			expectedCDCsInCDNamespaces: 1,
			expectedTotalClusters:      1,
			expectedPoolVersion:        inventoryPoolVersion,
			expectError:                false,
		},
		{
			name: "cp with inventory and cdc doesn't exist is not valid - missing",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
			},
			expectedTotalClusters:        0,
			expectedObservedSize:         0,
			expectedObservedReady:        0,
			expectedInventoryValidStatus: corev1.ConditionFalse,
			expectedInventoryMessage:     map[string][]string{"Missing": {"test-cdc-1"}},
			expectedCDCurrentStatus:      corev1.ConditionTrue, // huh?
			expectedPoolVersion:          inventoryPoolVersion,
			expectError:                  false,
		},
		{
			name: "cp with inventory - fix cdc reservation",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(2)),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(
					testcdc.WithCD("c1"),
					testcdc.WithPool(testLeasePoolName),
					testcdc.Available(),
				),
				testcd.FullBuilder("c1", "c1", scheme).Build(
					testcd.WithPoolVersion(inventoryPoolVersion),
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithCustomization("test-cdc-1"),
					testcd.Running(),
				),
			},
			expectedTotalClusters:        1,
			expectedObservedSize:         1,
			expectedObservedReady:        1,
			expectedPoolVersion:          inventoryPoolVersion,
			expectedInventoryValidStatus: corev1.ConditionTrue,
			expectError:                  false,
			expectedAssignedCDCs:         map[string]string{"test-cdc-1": "c1"},
		},
		{
			name: "cp with inventory - break on dirty cdc - cd ref",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(
					testcdc.WithCD("c1"),
					testcdc.Available(),
				),
			},
			expectedTotalClusters:        0,
			expectedObservedSize:         0,
			expectedObservedReady:        0,
			expectedInventoryValidStatus: corev1.ConditionTrue,
			expectedPoolVersion:          inventoryPoolVersion,
			expectedCDCurrentStatus:      corev1.ConditionUnknown,
			expectError:                  true,
		},
		{
			name: "cp with inventory - break on dirty cdc - cp ref",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(
					testcdc.WithPool("c1"),
					testcdc.Available(),
				),
			},
			expectedTotalClusters:        0,
			expectedObservedSize:         0,
			expectedObservedReady:        0,
			expectedInventoryValidStatus: corev1.ConditionTrue,
			expectedPoolVersion:          inventoryPoolVersion,
			expectedCDCurrentStatus:      corev1.ConditionUnknown,
			expectedCDCsInCDNamespaces:   1,
			expectError:                  true,
		},
		{
			name: "cp with inventory and cdc patch broken is not valid - BrokenBySyntax",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(
					testcdc.WithInstallConfigPatch("/broken/path", "replace", "x"),
				),
			},
			expectedTotalClusters:        0,
			expectedObservedSize:         0,
			expectedObservedReady:        0,
			expectedInventoryValidStatus: corev1.ConditionFalse,
			expectedInventoryMessage:     map[string][]string{"BrokenBySyntax": {"test-cdc-1"}},
			expectedCDCReason:            map[string]string{"test-cdc-1": hivev1.CustomizationApplyReasonBrokenSyntax},
			expectedPoolVersion:          inventoryPoolVersion,
			expectedCDCurrentStatus:      corev1.ConditionUnknown,
			expectError:                  true,
		},
		{
			name: "cp with inventory and cd provisioning failed is not valid - BrokenByCloud",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(),
				testcd.FullBuilder("c1", "c1", scheme).Build(
					testcd.WithPoolVersion("e0bc44f74a546c63"),
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithCustomization("test-cdc-1"),
					testcd.Broken(),
				),
			},
			expectedTotalClusters:        0,
			expectedObservedSize:         1,
			expectedObservedReady:        0,
			expectedInventoryValidStatus: corev1.ConditionFalse,
			expectedInventoryMessage:     map[string][]string{"BrokenByCloud": {"test-cdc-1"}},
			expectedCDCReason:            map[string]string{"test-cdc-1": hivev1.CustomizationApplyReasonBrokenCloud},
			expectedPoolVersion:          inventoryPoolVersion,
			expectedAssignedCDCs:         map[string]string{"test-cdc-1": "c1"},
		},
		{
			name: "cp with inventory and good cdc is valid, cd installed, claimed, but not assigned",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(
					testcdc.Reserved(),
					testcdc.WithCD("c1"),
					testcdc.WithPool(testLeasePoolName),
				),
				testclaim.FullBuilder(testNamespace, "test", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				unclaimedCDBuilder("c1").Build(
					testcd.Installed(),
					testcd.WithStatusPowerState(hivev1.ClusterPowerStateHibernating),
					testcd.WithCustomization("test-cdc-1"),
				),
			},
			expectedTotalClusters:        1,
			expectedObservedSize:         1,
			expectedRunning:              1,
			expectedAssignedClaims:       0,
			expectedAssignedCDs:          0,
			expectedUnassignedClaims:     1,
			expectedInventoryValidStatus: corev1.ConditionTrue,
			expectedCDCurrentStatus:      corev1.ConditionFalse,
			expectedPoolVersion:          inventoryPoolVersion,
		},
		{
			name: "cp with inventory and good cdc is valid, cd running, claimed, is assigned",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(
					testcdc.Reserved(),
					testcdc.WithCD("c1"),
					testcdc.WithPool(testLeasePoolName),
				),
				testclaim.FullBuilder(testNamespace, "test", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				unclaimedCDBuilder("c1").Build(
					testcd.Installed(),
					testcd.Running(),
					testcd.WithCustomization("test-cdc-1"),
				),
			},
			expectedTotalClusters:        1,
			expectedObservedSize:         1,
			expectedObservedReady:        1,
			expectedRunning:              1,
			expectedAssignedClaims:       1,
			expectedAssignedCDs:          1,
			expectedInventoryValidStatus: corev1.ConditionTrue,
			expectedCDCurrentStatus:      corev1.ConditionFalse,
			expectedPoolVersion:          inventoryPoolVersion,
		},
		{
			name: "openstack cp with inventory and good cdc is valid, cd installed, claimed, is assigned",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(
					testcp.WithSize(1),
					testcp.WithRunningCount(1),
					testcp.ForOpenstack(credsSecretName),
				),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(
					testcdc.Reserved(),
					testcdc.WithCD("c1"),
					testcdc.WithPool(testLeasePoolName),
				),
				testclaim.FullBuilder(testNamespace, "test", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				unclaimedCDBuilder("c1").Build(
					testcd.Installed(),
					testcd.Running(),
					testcd.WithCustomization("test-cdc-1"),
				),
			},
			expectedTotalClusters:        1,
			expectedObservedSize:         1,
			expectedRunning:              1,
			expectedObservedReady:        1,
			expectedAssignedClaims:       1,
			expectedAssignedCDs:          1,
			expectedInventoryValidStatus: corev1.ConditionTrue,
			expectedCDCurrentStatus:      corev1.ConditionFalse,
			expectedPoolVersion:          openstackPoolVersion,
			expectedClaimPendingReasons:  map[string]string{"test": "ClusterAssigned"},
		},
		{
			name: "cp with inventory and good cdc is valid, cd created",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(testcp.WithSize(1)),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(),
				unclaimedCDBuilder("c1").Build(
					testcd.WithCustomization("test-cdc-1"),
					testcd.Running(),
				),
			},
			expectedTotalClusters:        0,
			expectedObservedSize:         1,
			expectedObservedReady:        1,
			expectedInventoryValidStatus: corev1.ConditionTrue,
			expectedCDCReason:            map[string]string{"test-cdc-1": hivev1.CustomizationApplyReasonSucceeded},
			expectedPoolVersion:          inventoryPoolVersion,
			expectedAssignedCDCs:         map[string]string{"test-cdc-1": "c1"},
		},
		{
			name: "cp with inventory - correct prioritization - same status",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithInventory([]string{"test-cdc-successful-old", "test-cdc-unused-new"}),
				),
				testcdc.FullBuilder(testNamespace, "test-cdc-successful-old", scheme).Build(
					testcdc.WithApplySucceeded(hivev1.CustomizationApplyReasonSucceeded, nowish.Add(-time.Hour)),
				),
				testcdc.FullBuilder(testNamespace, "test-cdc-unused-new", scheme).Build(),
			},
			expectedTotalClusters:            1,
			expectedInventoryValidStatus:     corev1.ConditionTrue,
			expectedPoolVersion:              inventoryPoolVersion,
			expectedInventoryAssignmentOrder: []string{"test-cdc-successful-old"},
			expectedAssignedCDCs:             map[string]string{"test-cdc-successful-old": ""},
			expectedCDCsInCDNamespaces:       1,
		},
		{
			name: "cp with inventory - correct prioritization - mix and multiple deployments",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(2),
					testcp.WithInventory([]string{"test-cdc-successful-old", "test-cdc-unused-new", "test-cdc-broken-old"}),
				),
				testcdc.FullBuilder(testNamespace, "test-cdc-successful-old", scheme).Build(
					testcdc.WithApplySucceeded(hivev1.CustomizationApplyReasonSucceeded, nowish.Add(-time.Hour)),
				),
				testcdc.FullBuilder(testNamespace, "test-cdc-broken-old", scheme).Build(
					testcdc.WithApplySucceeded(hivev1.CustomizationApplyReasonBrokenCloud, nowish.Add(-time.Hour)),
				),
				testcdc.FullBuilder(testNamespace, "test-cdc-unused-new", scheme).Build(),
			},
			expectedTotalClusters:            2,
			expectedInventoryValidStatus:     corev1.ConditionFalse,
			expectedPoolVersion:              inventoryPoolVersion,
			expectedInventoryAssignmentOrder: []string{"test-cdc-successful-old", "test-cdc-unused-new"},
			expectedAssignedCDCs: map[string]string{
				"test-cdc-successful-old": "",
				"test-cdc-unused-new":     "",
			},
			expectedCDCsInCDNamespaces: 1,
		},

		{
			name: "cp with inventory - correct prioritization - successful vs broken",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithInventory([]string{"test-cdc-successful-new", "test-cdc-broken-old"}),
				),
				testcdc.FullBuilder(testNamespace, "test-cdc-broken-old", scheme).Build(
					testcdc.WithApplySucceeded(hivev1.CustomizationApplyReasonBrokenCloud, nowish.Add(-time.Hour)),
				),
				testcdc.FullBuilder(testNamespace, "test-cdc-successful-new", scheme).Build(
					testcdc.WithApplySucceeded(hivev1.CustomizationApplyReasonSucceeded, nowish),
				),
			},
			expectedTotalClusters:            1,
			expectedInventoryValidStatus:     corev1.ConditionFalse,
			expectedPoolVersion:              inventoryPoolVersion,
			expectedInventoryAssignmentOrder: []string{"test-cdc-successful-new"},
			expectedAssignedCDCs:             map[string]string{"test-cdc-successful-new": ""},
			expectedCDCsInCDNamespaces:       1,
		},
		{
			name: "cp with inventory - release cdc when cd is missing",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(
					testcp.WithSize(1),
					testcp.WithInventory([]string{"test-cdc-1"}),
				),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(
					testcdc.WithApplySucceeded(hivev1.CustomizationApplyReasonSucceeded, nowish.Add(-time.Hour)),
					testcdc.WithPool(testLeasePoolName),
					testcdc.WithCD("c1"),
					testcdc.Reserved(),
				),
			},
			expectedTotalClusters:      1,
			expectedPoolVersion:        inventoryPoolVersion,
			expectedAssignedCDCs:       map[string]string{"test-cdc-1": ""},
			expectedCDCsInCDNamespaces: 1,
		},
		{
			name: "cp with inventory - fix cdc when cd reference exists",
			existing: []runtime.Object{
				inventoryPoolBuilder().Build(
					testcp.WithSize(1),
					testcp.WithInventory([]string{"test-cdc-1"}),
				),
				testcdc.FullBuilder(testNamespace, "test-cdc-1", scheme).Build(
					testcdc.Available(),
				),
				testcd.FullBuilder("c1", "c1", scheme).Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithCustomization("test-cdc-1"),
				),
			},
			expectedTotalClusters:   1,
			expectedObservedSize:    1,
			expectedAssignedCDCs:    map[string]string{"test-cdc-1": "c1"},
			expectedPoolVersion:     inventoryPoolVersion,
			expectedCDCurrentStatus: corev1.ConditionUnknown,
		},
		{
			// This also proves we only delete one stale cluster at a time
			name: "delete oldest stale cluster first",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				unclaimedCDBuilder("c1").Build(
					testcd.WithPoolVersion("abc123"),
					testcd.Installed(),
					testcd.Generic(testgeneric.WithCreationTimestamp(nowish)),
				),
				unclaimedCDBuilder("c2").Build(
					testcd.WithPoolVersion("def345"),
					testcd.Installed(),
					testcd.Generic(testgeneric.WithCreationTimestamp(nowish.Add(-time.Hour))),
				),
			},
			expectedTotalClusters: 1,
			// Note: these observed counts are calculated before we start adding/deleting
			// clusters, so they include the stale one we deleted.
			expectedObservedSize:    2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
			expectedDeletedClusters: []string{"c2"},
		},
		{
			name: "delete stale unknown-version cluster first",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				// Two CDs: One with an "unknown" poolVersion, the other older. Proving we favor the unknown for deletion.
				unclaimedCDBuilder("c1").Build(
					testcd.WithPoolVersion(""),
					testcd.Installed(),
					testcd.Generic(testgeneric.WithCreationTimestamp(nowish)),
				),
				unclaimedCDBuilder("c2").Build(
					testcd.WithPoolVersion("bogus, but set"),
					testcd.Installed(),
					testcd.Generic(testgeneric.WithCreationTimestamp(nowish.Add(-time.Hour))),
				),
			},
			expectedTotalClusters:   1,
			expectedObservedSize:    2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
			expectedDeletedClusters: []string{"c1"},
		},
		{
			name: "stale clusters not deleted if any CD still installing",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				unclaimedCDBuilder("c1").Build(),
				unclaimedCDBuilder("c2").Build(
					testcd.WithPoolVersion("stale"),
					testcd.Installed(),
					testcd.Generic(testgeneric.WithCreationTimestamp(nowish.Add(-time.Hour))),
				),
			},
			expectedTotalClusters:   2,
			expectedObservedSize:    2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
		},
		{
			name: "stale clusters not deleted if under capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(
					testcd.Installed(),
				),
				unclaimedCDBuilder("c2").Build(
					testcd.WithPoolVersion("stale"),
					testcd.Installed(),
					testcd.Generic(testgeneric.WithCreationTimestamp(nowish.Add(-time.Hour))),
				),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
		},
		{
			name: "stale clusters not deleted if over capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
				unclaimedCDBuilder("c1").Build(
					testcd.Installed(),
				),
				unclaimedCDBuilder("c2").Build(
					testcd.WithPoolVersion("stale"),
					testcd.Running(),
				),
			},
			// This deletion happens because we're over capacity, not because we have staleness.
			// This is proven below.
			expectedTotalClusters:   1,
			expectedObservedSize:    2,
			expectedObservedReady:   1,
			expectedCDCurrentStatus: corev1.ConditionFalse,
			// So this is kind of interesting. We delete c1 even though c2 is a) older, b) stale.
			// This is because we prioritize keeping running clusters, as they can satisfy claims
			// more quickly. Possible subject of a future optimization.
			expectedDeletedClusters: []string{"c1"},
		},
		{
			name: "delete broken unclaimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4)),
				unclaimedCDBuilder("c1").Build(testcd.Broken()),
				// ProvisionStopped+Installed doesn't make sense IRL; but we're proving it would
				// be counted as broken if it did.
				unclaimedCDBuilder("c2").Build(testcd.Broken(), testcd.Installed()),
				// Unbroken CDs don't get deleted
				unclaimedCDBuilder("c3").Build(),
				// ...including assignable ones
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				// Claimed CDs don't get deleted
				cdBuilder("c5").Build(testcd.Installed(), testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test1")),
				// ...even if they're broken
				cdBuilder("c6").Build(testcd.Broken(), testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test2")),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  4,
			// The broken one doesn't get counted as Ready
			expectedDeletedClusters: []string{"c1", "c2"},
			expectedAssignedCDs:     2,
		},
		{
			name: "deleting broken clusters is bounded by maxConcurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Broken()),
				unclaimedCDBuilder("c2").Build(testcd.Broken()),
				unclaimedCDBuilder("c3").Build(testcd.Broken()),
				unclaimedCDBuilder("c4").Build(testcd.Broken()),
			},
			expectedTotalClusters: 2,
			expectedObservedSize:  4,
			// Note: We can't expectDeletedClusters because we delete broken clusters in arbitrary order
		},
		{
			name: "adding takes precedence over deleting broken clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Broken()),
				unclaimedCDBuilder("c2").Build(),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  2,
		},
		{
			name: "delete broken clusters first when reducing capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
				unclaimedCDBuilder("c1").Build(testcd.Broken()),
				unclaimedCDBuilder("c2").Build(),
			},
			expectedTotalClusters:   1,
			expectedObservedSize:    2,
			expectedDeletedClusters: []string{"c1"},
		},
		{
			// HIVE-1684
			name: "broken clusters are deleted at MaxSize",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2), testcp.WithMaxSize(3)),
				unclaimedCDBuilder("c1").Build(
					testcd.Broken(),
				),
				cdBuilder("c2").Build(testcd.Installed(), testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "aclaim")),
				cdBuilder("c3").Build(testcd.Installed(), testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "bclaim")),
			},
			expectedTotalClusters:   2,
			expectedObservedSize:    1,
			expectedObservedReady:   0,
			expectedAssignedCDs:     2,
			expectedDeletedClusters: []string{"c1"},
		},
		{
			name: "create all clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithClusterDeploymentLabels(map[string]string{"foo": "bar"})),
			},
			expectedTotalClusters: 5,
			expectedObservedSize:  0,
			expectedObservedReady: 0,
			expectedLabels:        map[string]string{"foo": "bar"},
		},
		{
			name: "scale up",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 5,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "scale up with no more capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   3,
			expectedObservedReady:  1,
			expectedCapacityStatus: corev1.ConditionFalse,
		},
		{
			name: "scale up with some capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxSize(4)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  4,
			expectedObservedSize:   3,
			expectedObservedReady:  1,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "scale up with no more capacity including claimed",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxSize(3)),
				testclaim.FullBuilder(testNamespace, "test", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				cdBuilder("c1").Build(
					testcd.Installed(),
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test"),
				),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   2,
			expectedAssignedClaims: 1,
			expectedAssignedCDs:    1,
			expectedCapacityStatus: corev1.ConditionFalse,
		},
		{
			name: "scale up with some capacity including claimed",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxSize(4)),
				testclaim.FullBuilder(testNamespace, "test", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				cdBuilder("c1").Build(
					testcd.Installed(),
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test"),
				),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  4,
			expectedObservedSize:   2,
			expectedAssignedClaims: 1,
			expectedAssignedCDs:    1,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "scale up with no more max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(1)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "scale up with one more max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "scale up with max concurrent and max size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(6), testcp.WithMaxSize(5), testcp.WithMaxConcurrent(1)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   3,
			expectedObservedReady:  1,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "scale up with max concurrent and max size 2",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(6), testcp.WithMaxSize(5), testcp.WithMaxConcurrent(1)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Running()),
			},
			expectedTotalClusters:  4,
			expectedObservedSize:   3,
			expectedObservedReady:  2,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "scale up with max concurrent and max size 3",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(6), testcp.WithMaxSize(4), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  4,
			expectedObservedSize:   3,
			expectedObservedReady:  1,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "no scale up with max concurrent and some deleting",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  2,
			// runningCount==0 and we don't have pending claims, but we don't muck with the
			// Running-ness of the deleting cluster.
			expectedRunning: 1,
		},
		{
			name: "no scale up with max concurrent and some deleting claimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(2)),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				cdBuilder("c1").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
				),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   2,
			expectedAssignedClaims: 1,
			expectedAssignedCDs:    1,
		},
		{
			name: "scale up with max concurrent and some deleting",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(3)),
				unclaimedCDBuilder("c1").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  2,
			expectedObservedReady: 1,
		},
		{
			name: "scale down",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(testcd.Running()),
				unclaimedCDBuilder("c5").Build(testcd.Running()),
				unclaimedCDBuilder("c6").Build(testcd.Running()),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  6,
			expectedObservedReady: 3,
		},
		{
			name: "scale down with max concurrent enough",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3), testcp.WithMaxConcurrent(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(testcd.Running()),
				unclaimedCDBuilder("c5").Build(testcd.Running()),
				unclaimedCDBuilder("c6").Build(testcd.Running()),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  6,
			expectedObservedReady: 3,
		},
		{
			name: "scale down with max concurrent not enough",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(testcd.Running()),
				unclaimedCDBuilder("c5").Build(testcd.Running()),
				unclaimedCDBuilder("c6").Build(testcd.Running()),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  6,
			expectedObservedReady: 3,
		},
		{
			name: "delete installing clusters first",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:   2,
			expectedObservedSize:    3,
			expectedObservedReady:   1,
			expectedDeletedClusters: []string{"c3"},
		},
		{
			name: "delete most recent installing clusters first",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
				unclaimedCDBuilder("c1").GenericOptions(
					testgeneric.WithCreationTimestamp(time.Date(2020, 1, 2, 3, 4, 5, 6, time.UTC)),
				).Build(),
				unclaimedCDBuilder("c2").GenericOptions(
					testgeneric.WithCreationTimestamp(time.Date(2020, 2, 2, 3, 4, 5, 6, time.UTC)),
				).Build(),
			},
			expectedTotalClusters:   1,
			expectedObservedSize:    2,
			expectedObservedReady:   0,
			expectedDeletedClusters: []string{"c2"},
		},
		{
			// Also shows we delete installed clusters before running ones
			name: "delete installed clusters when there are not enough installing to delete",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				unclaimedCDBuilder("c4").Build(testcd.Running()),
				unclaimedCDBuilder("c5").Build(testcd.Running()),
				unclaimedCDBuilder("c6").Build(),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    6,
			expectedObservedReady:   3,
			expectedDeletedClusters: []string{"c2", "c3", "c6"},
		},
		{
			name: "deleted pool: clusters deleted, pool held while clusters pending deletion",
			existing: []runtime.Object{
				initializedPoolBuilder.GenericOptions(testgeneric.Deleted()).Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
			},
			expectedTotalClusters:   0,
			expectedCDCurrentStatus: corev1.ConditionUnknown,
			expectedDeletionPossibleCondition: &hivev1.ClusterPoolCondition{
				Status: corev1.ConditionFalse,
				Reason: "ClusterDeploymentsPendingCleanup",
				// This message gets set while we're issuing Delete()s
				Message: "The following ClusterDeployments are pending cleanup: c1, c2, c3",
			},
		},
		{
			name: "deleted pool: cluster deletions adhere to MaxConcurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(testcp.WithSize(4), testcp.WithMaxConcurrent(3)),
				unclaimedCDBuilder("c0").Build(testcd.Installed()),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				// Already deleting -- will count against MaxConcurrent
				cdBuilder("c2").GenericOptions(
					testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
				),
				// This one is "Installing" so it will count against MaxConcurrent
				unclaimedCDBuilder("c3").Build(),
			},
			// This is the "Installing" one and one of the "Installed" ones -- the other "Installed" one gets
			// deleted to make up MaxConcurrent=3.
			// TODO: Due to https://github.com/kubernetes-sigs/controller-runtime/issues/2184 c2, which starts with a
			// DeletionTimestamp, isn't getting garbage collected by the fake client. This is the expected result once
			// the issue is fixed:
			// expectedTotalClusters: 2,
			expectedTotalClusters: 3,
			// But this one is right
			expectedObservedSize:    2,
			expectedCDCurrentStatus: corev1.ConditionUnknown,
			expectedDeletionPossibleCondition: &hivev1.ClusterPoolCondition{
				Status:  corev1.ConditionFalse,
				Reason:  "ClusterDeploymentsPendingCleanup",
				Message: "The following ClusterDeployments are pending cleanup: c0, c1, c2, c3",
			},
		},
		{
			name: "deleted pool: claimed clusters not deleted",
			existing: []runtime.Object{
				initializedPoolBuilder.GenericOptions(testgeneric.Deleted()).Build(testcp.WithSize(3)),
				cdBuilder("c1").Build(
					testcd.Installed(),
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test"),
				),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
			},
			expectedTotalClusters:   1,
			expectedAssignedCDs:     1,
			expectedCDCurrentStatus: corev1.ConditionUnknown,
			expectedDeletionPossibleCondition: &hivev1.ClusterPoolCondition{
				Status:  corev1.ConditionFalse,
				Reason:  "ClusterDeploymentsPendingCleanup",
				Message: "The following ClusterDeployments are pending cleanup: c1, c2, c3",
			},
		},
		{
			name: "deleted pool: claimed clusters deleted if marked for deletion",
			existing: []runtime.Object{
				initializedPoolBuilder.GenericOptions(testgeneric.Deleted()).Build(testcp.WithSize(3)),
				cdBuilder("c1").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.Installed(),
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test"),
					),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
			},
			expectedCDCurrentStatus: corev1.ConditionUnknown,
			expectedDeletionPossibleCondition: &hivev1.ClusterPoolCondition{
				Status:  corev1.ConditionFalse,
				Reason:  "ClusterDeploymentsPendingCleanup",
				Message: "The following ClusterDeployments are pending cleanup: c1, c2, c3",
			},
		},
		{
			name: "deleted pool: DeletionPossible condition True when no clusters pending deletion",
			existing: []runtime.Object{
				initializedPoolBuilder.GenericOptions(
					testgeneric.Deleted(),
					testgeneric.WithFinalizer("something-to-hold-the-pool"),
				).Build(),
			},
			expectedTotalClusters:   0,
			expectFinalizerRemoved:  true,
			expectedCDCurrentStatus: corev1.ConditionUnknown,
			expectedDeletionPossibleCondition: &hivev1.ClusterPoolCondition{
				Status:  corev1.ConditionTrue,
				Reason:  "DeletionPossible",
				Message: "No ClusterDeployments pending cleanup",
			},
		},
		{
			name: "finalizer added to clusterpool",
			existing: []runtime.Object{
				initializedPoolBuilder.GenericOptions(testgeneric.WithoutFinalizer(finalizer)).Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "clusters not part of pool are not counted against pool size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "claimed clusters are not counted against pool size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
				),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
			expectedAssignedCDs:   1,
		},
		{
			name: "clusters in different pool are not counted against pool size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(
					testcd.WithClusterPoolReference(testNamespace, "other-pool", "test-claim"),
					testcd.WithPoolVersion("aversion")),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "deleting clusters are not counted against pool size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.Installed()),
				cdBuilder("c5").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.Running()),
				cdBuilder("c6").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
				),
			},
			expectedTotalClusters: 6,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
			expectedRunning:       1,
		},
		{
			name: "missing ClusterImageSet",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
			},
			noClusterImageSet:                  true,
			expectError:                        true,
			expectedMissingDependenciesStatus:  corev1.ConditionTrue,
			expectedMissingDependenciesMessage: `cluster image set: clusterimagesets.hive.openshift.io "test-image-set" not found`,
			expectedCDCurrentStatus:            corev1.ConditionUnknown,
		},
		{
			name: "missing creds secret",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
			},
			noCredsSecret:                      true,
			expectError:                        true,
			expectedMissingDependenciesStatus:  corev1.ConditionTrue,
			expectedMissingDependenciesMessage: `credentials secret: secrets "aws-creds" not found`,
			expectedCDCurrentStatus:            corev1.ConditionUnknown,
		},
		{
			name: "missing pool-wide CDC",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1), testcp.WithCustomizationRef("non-inventory-cdc")),
			},
			expectError:                        true,
			expectedMissingDependenciesStatus:  corev1.ConditionTrue,
			expectedMissingDependenciesMessage: `pool-wide customizationRef non-inventory-cdc not found`,
			expectPoolVersionChanged:           true,
			expectedCDCurrentStatus:            corev1.ConditionUnknown,
		},
		{
			name: "multiple missing dependents",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1), testcp.WithCustomizationRef("non-inventory-cdc")),
			},
			noClusterImageSet:                  true,
			noCredsSecret:                      true,
			expectError:                        true,
			expectedMissingDependenciesStatus:  corev1.ConditionTrue,
			expectedMissingDependenciesMessage: `[cluster image set: clusterimagesets.hive.openshift.io "test-image-set" not found, credentials secret: secrets "aws-creds" not found, pool-wide customizationRef non-inventory-cdc not found]`,
			expectPoolVersionChanged:           true,
			expectedCDCurrentStatus:            corev1.ConditionUnknown,
		},
		{
			name: "missing dependents resolved",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithCondition(hivev1.ClusterPoolCondition{
						Type:   hivev1.ClusterPoolMissingDependenciesCondition,
						Status: corev1.ConditionTrue,
					}),
				),
			},
			expectedMissingDependenciesStatus:  corev1.ConditionFalse,
			expectedMissingDependenciesMessage: "Dependencies verified",
			expectedTotalClusters:              1,
			expectedObservedSize:               0,
			expectedObservedReady:              0,
			// Wedge in a check for the default labels. We're doing this on a test case that ends
			// up with at least one cluster, but where we don't prepopulate any (which we would
			// have to label explicitly -- see note in the test driver).
			expectedLabels: map[string]string{},
		},
		{
			name: "max size should include the deleting unclaimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(2),
					testcp.WithMaxSize(3),
					testcp.WithCondition(hivev1.ClusterPoolCondition{
						Type:   hivev1.ClusterPoolCapacityAvailableCondition,
						Status: corev1.ConditionFalse,
					}),
				),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(testcd.Running()),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   2,
			expectedObservedReady:  1,
			expectedRunning:        1,
			expectedCapacityStatus: corev1.ConditionFalse,
		},
		{
			name: "max capacity resolved",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(2),
					testcp.WithMaxSize(3),
					testcp.WithCondition(hivev1.ClusterPoolCondition{
						Type:   hivev1.ClusterPoolCapacityAvailableCondition,
						Status: corev1.ConditionFalse,
					}),
				),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
			},
			expectedTotalClusters:  2,
			expectedObservedSize:   2,
			expectedObservedReady:  1,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "with pull secret",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithPullSecret("test-pull-secret"),
				),
				testsecret.FullBuilder(testNamespace, "test-pull-secret", scheme).
					Build(testsecret.WithDataKeyValue(".dockerconfigjson", []byte("test docker config data"))),
			},
			expectedTotalClusters: 1,
			expectedObservedSize:  0,
			expectedObservedReady: 0,
		},
		{
			name: "missing pull secret",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithPullSecret("test-pull-secret"),
				),
			},
			expectError:                        true,
			expectedMissingDependenciesStatus:  corev1.ConditionTrue,
			expectedMissingDependenciesMessage: `pull secret: secrets "test-pull-secret" not found`,
			expectedCDCurrentStatus:            corev1.ConditionUnknown,
		},
		{
			name: "pull secret missing docker config",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithPullSecret("test-pull-secret"),
				),
				testsecret.FullBuilder(testNamespace, "test-pull-secret", scheme).Build(),
			},
			expectError:                        true,
			expectedMissingDependenciesStatus:  corev1.ConditionTrue,
			expectedMissingDependenciesMessage: `pull secret: pull secret does not contain .dockerconfigjson data`,
			expectedCDCurrentStatus:            corev1.ConditionUnknown,
		},
		{
			name: "assign to claim",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedTotalClusters:       4,
			expectedObservedSize:        3,
			expectedObservedReady:       1,
			expectedAssignedClaims:      1,
			expectedAssignedCDs:         1,
			expectedRunning:             1,
			expectedClaimPendingReasons: map[string]string{"test-claim": "ClusterAssigned"},
		},
		{
			name: "no ready clusters to assign to claim",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedTotalClusters:       4,
			expectedObservedSize:        3,
			expectedObservedReady:       0,
			expectedAssignedClaims:      0,
			expectedUnassignedClaims:    1,
			expectedRunning:             1,
			expectedClaimPendingReasons: map[string]string{"test-claim": "NoClusters"},
		},
		{
			name: "assign to multiple claims",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4)),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(),
				// Claims are assigned in FIFO order by creationTimestamp
				testclaim.FullBuilder(testNamespace, "test-claim-1", scheme).Build(
					testclaim.WithPool(testLeasePoolName),
					testclaim.Generic(testgeneric.WithCreationTimestamp(nowish.Add(-time.Second*2))),
				),
				testclaim.FullBuilder(testNamespace, "test-claim-2", scheme).Build(
					testclaim.WithPool(testLeasePoolName),
					testclaim.Generic(testgeneric.WithCreationTimestamp(nowish.Add(-time.Second))),
				),
				testclaim.FullBuilder(testNamespace, "test-claim-3", scheme).Build(
					testclaim.WithPool(testLeasePoolName),
					testclaim.Generic(testgeneric.WithCreationTimestamp(nowish)),
				),
			},
			expectedTotalClusters:    7,
			expectedObservedSize:     4,
			expectedObservedReady:    2,
			expectedAssignedClaims:   2,
			expectedAssignedCDs:      2,
			expectedRunning:          3,
			expectedUnassignedClaims: 1,
			expectedClaimPendingReasons: map[string]string{
				"test-claim-1": "ClusterAssigned",
				"test-claim-2": "ClusterAssigned",
				"test-claim-3": "NoClusters",
			},
		},
		{
			name: "do not assign to claims for other pools",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(
					testclaim.WithPool("other-pool"),
					testclaim.WithCondition(hivev1.ClusterClaimCondition{
						Type:    hivev1.ClusterClaimPendingCondition,
						Status:  corev1.ConditionFalse,
						Reason:  "ThisShouldNotChange",
						Message: "Claim ignored because not in the pool",
					}),
				),
			},
			expectedTotalClusters:       3,
			expectedObservedSize:        3,
			expectedObservedReady:       1,
			expectedAssignedClaims:      0,
			expectedUnassignedClaims:    1,
			expectedClaimPendingReasons: map[string]string{"test-claim": "ThisShouldNotChange"},
		},
		{
			name: "do not assign to claims in other namespaces",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder("other-namespace", "test-claim", scheme).Build(
					testclaim.WithPool(testLeasePoolName),
					testclaim.WithCondition(hivev1.ClusterClaimCondition{
						Type:    hivev1.ClusterClaimPendingCondition,
						Status:  corev1.ConditionFalse,
						Reason:  "ThisShouldNotChange",
						Message: "Claim ignored because not in the namespace",
					}),
				),
			},
			expectedTotalClusters:       3,
			expectedObservedSize:        3,
			expectedObservedReady:       1,
			expectedAssignedClaims:      0,
			expectedUnassignedClaims:    1,
			expectedClaimPendingReasons: map[string]string{"test-claim": "ThisShouldNotChange"},
		},
		{
			name: "do not delete previously claimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
				),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
			expectedAssignedCDs:   1,
		},
		{
			name: "do not delete previously claimed clusters 2",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer), testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
			expectedAssignedCDs:   1,
		},
		{
			name: "delete previously claimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    3,
			expectedObservedReady:   1,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "deleting previously claimed clusters should use max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedAssignedCDs:   1,
		},
		{
			name: "deleting previously claimed clusters should use max concurrent 2",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  2,
			expectedAssignedCDs:   1,
		},
		{
			name: "deleting previously claimed clusters should use max concurrent 3",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3), testcp.WithMaxConcurrent(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    2,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "scale up should include previouly deleted in max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   2,
			expectedObservedSize:    2,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "scale up should include previouly deleted in max concurrent all used by deleting previously claimed",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Running()),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
				cdBuilder("c5").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   2,
			expectedObservedSize:    2,
			expectedObservedReady:   1,
			expectedDeletedClusters: []string{"c4", "c5"},
		},
		{
			name: "scale up should include previouly deleted in max concurrent all used by installing one cluster and deleting one previously claimed cluster",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
				cdBuilder("c5").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    2,
			expectedAssignedCDs:     1,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "scale down should include previouly deleted in max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    3,
			expectedObservedReady:   1,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "scale down should include previouly deleted in max concurrent all used by deleting previously claimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Running()),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
				cdBuilder("c5").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    3,
			expectedObservedReady:   2,
			expectedDeletedClusters: []string{"c4", "c5"},
		},
		{
			name: "scale down should include previouly deleted in max concurrent all used by one installing cluster and deleting one previously claimed cluster",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
				cdBuilder("c5").
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   4,
			expectedObservedSize:    3,
			expectedObservedReady:   1,
			expectedAssignedCDs:     1,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "claims exceed capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				testclaim.FullBuilder(testNamespace, "test-claim1", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim2", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim3", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedTotalClusters: 5,
			// The assignments don't happen until a subsequent reconcile after the CDs are ready
			expectedUnassignedClaims: 3,
			// Even though runningCount is zero, we run enough clusters to fulfill the excess claims
			expectedRunning: 3,
		},
		{
			name: "zero size pool",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(0)),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedTotalClusters: 1,
			// The assignments don't happen until a subsequent reconcile after the CDs are ready
			expectedUnassignedClaims: 1,
			// Even though runningCount is zero, we run enough clusters to fulfill the excess claims
			expectedRunning: 1,
		},
		{
			name: "no CDs match pool version",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				cdBuilder("c1").Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithPoolVersion("a-previous-version")),
				cdBuilder("c2").Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithPoolVersion("some-other-version")),
			},
			expectedObservedSize:    2,
			expectedTotalClusters:   2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
		},
		{
			name: "not all CDs match pool version",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				cdBuilder("c1").Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithPoolVersion("a-previous-version")),
				unclaimedCDBuilder("c2").Build(),
			},
			expectedObservedSize:    2,
			expectedTotalClusters:   2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
		},
		{
			name: "empty pool version results in unknown CDCurrent status",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
				cdBuilder("c1").Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithPoolVersion("")),
			},
			expectedObservedSize:    1,
			expectedTotalClusters:   1,
			expectedCDCurrentStatus: corev1.ConditionUnknown,
		},
		{
			name: "mismatched CDCurrent status takes precedence over unknown",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				cdBuilder("c1").Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithPoolVersion("")),
				cdBuilder("c2").Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithPoolVersion("some-other-version")),
			},
			expectedObservedSize:    2,
			expectedTotalClusters:   2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
		},
		{
			name: "claimed CD at old version doesn't affect version status",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				cdBuilder("c1").Build(
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					testcd.WithPoolVersion("a-previous-version")),
				unclaimedCDBuilder("c2").Build(),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedAssignedCDs:    1,
			expectedAssignedClaims: 1,
			expectedObservedSize:   2,
			expectedTotalClusters:  3,
		},
		{
			name: "runningCount < size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(2),
				),
			},
			expectedTotalClusters: 4,
			expectedRunning:       2,
		},
		{
			name: "runningCount == size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(4),
				),
			},
			expectedTotalClusters: 4,
			expectedRunning:       4,
		},
		{
			name: "runningCount > size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(400),
				),
			},
			expectedTotalClusters: 4,
			expectedRunning:       4,
		},
		{
			name: "runningCount restored after deletion",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(2),
				),
				unclaimedCDBuilder("c1").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(testcd.WithPowerState(hivev1.ClusterPowerStateRunning)),
				unclaimedCDBuilder("c2").Build(testcd.WithPowerState(hivev1.ClusterPowerStateRunning)),
				unclaimedCDBuilder("c3").Build(),
				unclaimedCDBuilder("c4").Build(),
			},
			expectedObservedSize:  3,
			expectedTotalClusters: 5,
			expectedRunning:       3,
		},
		{
			name: "runningCount restored after claim",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(2),
				),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedObservedSize:   4,
			expectedObservedReady:  2,
			expectedTotalClusters:  5,
			expectedRunning:        3,
			expectedAssignedCDs:    1,
			expectedAssignedClaims: 1,
		},
		{
			name: "pool size > number of claims > runningCount",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(3),
				),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Running()),
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				testclaim.FullBuilder(testNamespace, "test-claim1", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim2", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim3", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedObservedSize:   4,
			expectedObservedReady:  3,
			expectedTotalClusters:  7,
			expectedRunning:        6,
			expectedAssignedCDs:    3,
			expectedAssignedClaims: 3,
		},
		{
			name: "number of claims > pool size > runningCount",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(2),
				),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				testclaim.FullBuilder(testNamespace, "test-claim1", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim2", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim3", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim4", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim5", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedObservedSize:  4,
			expectedObservedReady: 2,
			expectedTotalClusters: 9,
			// The two original running pool CDs got claimed.
			// We create five new CDs to satisfy the pool size plus the additional three claims.
			// Of those, we start two for runningCount, and three more for the excess claims.
			// Including the two originally running that we assigned to claims, that's seven.
			expectedRunning:          7,
			expectedAssignedCDs:      2,
			expectedAssignedClaims:   2,
			expectedUnassignedClaims: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !test.noClusterImageSet {
				test.existing = append(
					test.existing,
					&hivev1.ClusterImageSet{
						ObjectMeta: metav1.ObjectMeta{Name: imageSetName},
						Spec:       hivev1.ClusterImageSetSpec{ReleaseImage: "test-release-image"},
					},
				)
			}
			if !test.noCredsSecret {
				test.existing = append(
					test.existing, testsecret.FullBuilder(testNamespace, credsSecretName, scheme).
						Build(testsecret.WithDataKeyValue("dummykey", []byte("dummyval"))),
				)
			}
			expectedPoolVersion := initialPoolVersion
			if test.expectedPoolVersion != "" {
				expectedPoolVersion = test.expectedPoolVersion
			}

			fakeClient := testfake.NewFakeClientBuilder().
				WithIndex(&hivev1.ClusterDeployment{}, cdClusterPoolIndex, indexClusterDeploymentsByClusterPool).
				WithIndex(&hivev1.ClusterClaim{}, claimClusterPoolIndex, indexClusterClaimsByClusterPool).
				WithRuntimeObjects(test.existing...).
				Build()
			logger := log.New()
			logger.SetLevel(log.DebugLevel)
			controllerExpectations := controllerutils.NewExpectations(logger)

			expectedPools := []string{testLeasePoolName}
			if test.expectedPools != nil {
				expectedPools = test.expectedPools
			}
			for _, poolName := range expectedPools {
				rcp := &ReconcileClusterPool{
					Client:       fakeClient,
					logger:       logger,
					expectations: controllerExpectations,
				}

				reconcileRequest := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      poolName,
						Namespace: testNamespace,
					},
				}

				_, err := rcp.Reconcile(context.TODO(), reconcileRequest)
				if test.expectError {
					assert.Error(t, err, "expected error from reconcile")
				} else {
					assert.NoError(t, err, "expected no error from reconcile")
				}
			}

			pool := &hivev1.ClusterPool{}
			err := fakeClient.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testLeasePoolName}, pool)

			assert.NoError(t, err, "unexpected error getting clusterpool")
			if test.expectFinalizerRemoved {
				assert.NotContains(t, pool.Finalizers, finalizer, "did not expect finalizer on clusterpool")
			} else {
				assert.Contains(t, pool.Finalizers, finalizer, "expected finalizer on clusterpool")
			}
			assert.Equal(t, test.expectedObservedSize, pool.Status.Size, "unexpected observed size")
			assert.Equal(t, test.expectedObservedReady, pool.Status.Ready, "unexpected observed ready count")
			currentPoolVersion := calculatePoolVersion(pool)
			assert.Equal(
				t, test.expectPoolVersionChanged, currentPoolVersion != expectedPoolVersion,
				"expectPoolVersionChanged is %t\ninitial %q\nfinal   %q",
				test.expectPoolVersionChanged, expectedPoolVersion, currentPoolVersion)
			expectedCDCurrentStatus := test.expectedCDCurrentStatus
			if expectedCDCurrentStatus == "" {
				expectedCDCurrentStatus = corev1.ConditionTrue
			}
			cdCurrentCondition := controllerutils.FindCondition(pool.Status.Conditions, hivev1.ClusterPoolAllClustersCurrentCondition)
			if assert.NotNil(t, cdCurrentCondition, "did not find ClusterDeploymentsCurrent condition") {
				assert.Equal(t, expectedCDCurrentStatus, cdCurrentCondition.Status,
					"unexpected ClusterDeploymentsCurrent condition")
			}

			if test.expectedMissingDependenciesStatus != "" {
				missingDependenciesCondition := controllerutils.FindCondition(pool.Status.Conditions, hivev1.ClusterPoolMissingDependenciesCondition)
				if assert.NotNil(t, missingDependenciesCondition, "did not find MissingDependencies condition") {
					assert.Equal(t, test.expectedMissingDependenciesStatus, missingDependenciesCondition.Status,
						"unexpected MissingDependencies conditon status")
					if test.expectedMissingDependenciesMessage != "" {
						assert.Equal(t, test.expectedMissingDependenciesMessage, missingDependenciesCondition.Message,
							"unexpected MissingDependencies conditon message")
					}
				}
			}
			if test.expectedCapacityStatus != "" {
				capacityAvailableCondition := controllerutils.FindCondition(pool.Status.Conditions, hivev1.ClusterPoolCapacityAvailableCondition)
				if assert.NotNil(t, capacityAvailableCondition, "did not find CapacityAvailable condition") {
					assert.Equal(t, test.expectedCapacityStatus, capacityAvailableCondition.Status,
						"unexpected CapacityAvailable conditon status")
				}
			}

			if test.expectedInventoryValidStatus != "" {
				inventoryValidCondition := controllerutils.FindCondition(pool.Status.Conditions, hivev1.ClusterPoolInventoryValidCondition)
				if assert.NotNil(t, inventoryValidCondition, "did not find InventoryValid condition") {
					assert.Equal(t, test.expectedInventoryValidStatus, inventoryValidCondition.Status,
						"unexpected InventoryValid condition status %s", inventoryValidCondition.Status)
				}
			}

			if test.expectedInventoryMessage != nil {
				inventoryValidCondition := controllerutils.FindCondition(pool.Status.Conditions, hivev1.ClusterPoolInventoryValidCondition)
				if assert.NotNil(t, inventoryValidCondition, "did not find InventoryValid condition") {
					expectedInventoryMessage := map[string][]string{}
					err := json.Unmarshal([]byte(inventoryValidCondition.Message), &expectedInventoryMessage)
					if err != nil {
						assert.Error(t, err, "unable to parse inventory condition message")
					}
					for key, value := range test.expectedInventoryMessage {
						if val, ok := expectedInventoryMessage[key]; ok {
							assert.ElementsMatch(t, value, val, "unexpected inventory message for %s: %s", key, inventoryValidCondition.Message)
						} else {
							assert.Fail(t, "expected inventory message to contain key %s: %s", key, inventoryValidCondition.Message)
						}
					}
				}
			}

			if edpc := test.expectedDeletionPossibleCondition; edpc != nil {
				adpc := controllerutils.FindCondition(pool.Status.Conditions, hivev1.ClusterPoolDeletionPossibleCondition)
				if assert.NotNil(t, adpc, "did not find DeletionPossible condition") {
					if edpc.Status != "" {
						assert.Equal(t, edpc.Status, adpc.Status, "unexpected DeletionPossible condition status")
					}
					if edpc.Reason != "" {
						assert.Equal(t, edpc.Reason, adpc.Reason, "unexpected DeletionPossible condition reason")
					}
					if edpc.Message != "" {
						assert.Equal(t, edpc.Message, adpc.Message, "unexpected DeletionPossible condition message")
					}
				}
			}

			cds := &hivev1.ClusterDeploymentList{}
			err = fakeClient.List(context.Background(), cds)
			require.NoError(t, err)

			assert.Len(t, cds.Items, test.expectedTotalClusters, "unexpected number of total clusters")

			for _, expectedDeletedName := range test.expectedDeletedClusters {
				for _, cd := range cds.Items {
					assert.NotEqual(t, expectedDeletedName, cd.Name, "expected cluster to have been deleted")
				}
			}

			var actualAssignedCDs, actualUnassignedCDs, actualRunning, actualHibernating int
			cdNamespaces := sets.Set[string]{}
			for _, cd := range cds.Items {
				cdNamespaces.Insert(cd.Namespace)

				poolRef := cd.Spec.ClusterPoolRef
				if poolRef == nil || poolRef.PoolName != testLeasePoolName || poolRef.ClaimName == "" {
					// TODO: Calling these "unassigned" isn't great. Some may not even belong to the pool.
					actualUnassignedCDs++
				} else {
					actualAssignedCDs++
				}
				// Match up copyover fields for any clusters belonging to the pool
				if poolRef != nil && poolRef.PoolName == testLeasePoolName {
					// TODO: These would need to be set on all CDs in test.existing
					// assert.Equal(t, pool.Spec.BaseDomain, cd.Spec.BaseDomain, "expected BaseDomain to match")
					if pool.Spec.InstallAttemptsLimit != nil {
						if assert.NotNil(t, cd.Spec.InstallAttemptsLimit, "expected InstallAttemptsLimit to be set") {
							assert.Equal(t, *pool.Spec.InstallAttemptsLimit, *cd.Spec.InstallAttemptsLimit, "expected InstallAttemptsLimit to match")
						}
					}
					if len(pool.Spec.InstallerEnv) != 0 {
						assert.Equal(t, pool.Spec.InstallerEnv, cd.Spec.Provisioning.InstallerEnv, "expected InstallerEnv to match")
					}
				}
				switch powerState := cd.Spec.PowerState; powerState {
				case hivev1.ClusterPowerStateRunning:
					actualRunning++
				case hivev1.ClusterPowerStateHibernating:
					actualHibernating++
				}

				// Many tests set up pre-existing pool clusters. Those won't have the default labels
				// (and we don't want to add them because then we might miss a regression in the actual
				// code) so only check for them if the test explicitly requests it.
				if test.expectedLabels != nil {
					assert.Equal(t, testNamespace, cd.Labels[clusterPoolNamespaceLabelKey], "unexpected namespace label")
					assert.Equal(t, testLeasePoolName, cd.Labels[clusterPoolNameLabelKey], "unexpected name label")
					for k, v := range test.expectedLabels {
						assert.Equal(t, v, cd.Labels[k])
					}
				}
			}
			assert.Equal(t, test.expectedAssignedCDs, actualAssignedCDs, "unexpected number of assigned CDs")
			assert.Equal(t, test.expectedTotalClusters-test.expectedAssignedCDs, actualUnassignedCDs, "unexpected number of unassigned CDs")
			assert.Equal(t, test.expectedRunning, actualRunning, "unexpected number of running CDs")
			assert.Equal(t, test.expectedTotalClusters-test.expectedRunning, actualHibernating, "unexpected number of hibernating CDs")

			claims := &hivev1.ClusterClaimList{}
			err = fakeClient.List(context.Background(), claims)
			require.NoError(t, err)

			// Set of namespaces of claimed clusters
			actualAssignedClaims := sets.Set[string]{}
			actualUnassignedClaims := 0
			for _, claim := range claims.Items {
				if test.expectedClaimPendingReasons != nil {
					if reason, ok := test.expectedClaimPendingReasons[claim.Name]; ok {
						actualCond := controllerutils.FindCondition(claim.Status.Conditions, hivev1.ClusterClaimPendingCondition)
						if assert.NotNil(t, actualCond, "did not find Pending condition on claim %s", claim.Name) {
							assert.Equal(t, reason, actualCond.Reason, "wrong reason on Pending condition for claim %s", claim.Name)
						}
					}
				}
				if ns := claim.Spec.Namespace; ns == "" {
					actualUnassignedClaims++
				} else {
					actualAssignedClaims.Insert(ns)
				}
			}
			assert.Equal(t, test.expectedAssignedClaims, len(actualAssignedClaims), "unexpected number of assigned claims")
			assert.Equal(t, test.expectedUnassignedClaims, actualUnassignedClaims, "unexpected number of unassigned claims")

			cdcs := &hivev1.ClusterDeploymentCustomizationList{}
			// Get CDCs from all namespaces
			err = fakeClient.List(context.Background(), cdcs)
			require.NoError(t, err)
			cdcMap := make(map[string]hivev1.ClusterDeploymentCustomization, len(cdcs.Items))
			// Keep track of the number of CDCs in each CD namespace. In the end, these should all
			// be the same, equal to expectedCDCsInCDNamespaces
			cdcsInCDNamespaces := map[string]int{}
			for _, cdc := range cdcs.Items {
				// Ensure CDCs were copied to pool CD namespaces
				if cdc.Namespace != pool.Namespace {
					assert.Contains(t, cdNamespaces, cdc.Namespace, "expected CDC to have been copied to CD namespace")
					cdcsInCDNamespaces[cdc.Namespace]++
					continue
				}

				cdcMap[cdc.Name] = cdc

				condition := meta.FindStatusCondition(cdc.Status.Conditions, hivev1.ApplySucceededCondition)
				if test.expectedCDCReason != nil {
					if reason, ok := test.expectedCDCReason[cdc.Name]; ok {
						if assert.NotNil(t, condition) {
							assert.Equal(t, reason, condition.Reason, "expected CDC status to match")
						}
					}
				}

				if test.expectedAssignedCDCs != nil {
					if cdName, ok := test.expectedAssignedCDCs[cdc.Name]; ok {
						assert.Regexp(t, regexp.MustCompile(cdName), cdc.Status.ClusterDeploymentRef.Name, "expected CDC assignment to CD match")
					}
				}

				if test.expectedCDCFinalizers != nil {
					if finalizers, ok := test.expectedCDCFinalizers[cdc.Name]; ok {
						for _, finalizer := range finalizers {
							assert.Contains(t, cdc.Finalizers, finalizer, "expected CDC finalizer to match")
						}
					}
				}
			}
			for ns, actualCDCsInCDNamespace := range cdcsInCDNamespaces {
				assert.Equal(t, test.expectedCDCsInCDNamespaces, actualCDCsInCDNamespace, "unexpected number of CDCs in namespace %s", ns)
			}

			if order := test.expectedInventoryAssignmentOrder; len(order) > 0 {
				lastTime := metav1.NewTime(nowish.Add(24 * -time.Hour))
				for _, cdcName := range order {
					cdc := cdcMap[cdcName]
					condition := meta.FindStatusCondition(cdc.Status.Conditions, "Available")
					if condition == nil || condition.Status == metav1.ConditionUnknown || condition.Status == metav1.ConditionTrue {
						assert.Failf(t, "expected CDC %s to be assigned", cdcName)
					}
					assert.True(t, lastTime.Before(&condition.LastTransitionTime) || lastTime.Equal(&condition.LastTransitionTime), "expected %s to be before %s", lastTime, condition.LastTransitionTime)
					lastTime = condition.LastTransitionTime
				}
			}
		})
	}
}

func TestReconcileRBAC(t *testing.T) {
	tests := []struct {
		name string

		existing []runtime.Object

		expectedBindings []rbacv1.RoleBinding
		expectedErr      string
	}{{
		name:     "no binding referring to cluster role 1",
		existing: []runtime.Object{},
	}, {
		name: "no binding referring to cluster role 2",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "admin"},
			},
		},
	}, {
		name: "binding referring to cluster role but no namespace for pool 1",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "binding referring to cluster role but no namespace for pool 2",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-namespace-1",
				},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "binding referring to cluster role but no namespace for pool 3",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-namespace-1",
				},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "some-namespace-2",
					Labels: map[string]string{constants.ClusterPoolNameLabel: "some-other-pool"},
				},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "binding referring to cluster role with one namespace for pool 4",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "binding referring to cluster role with multiple namespace for pool",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-2",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-2",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "multiple binding referring to cluster role with one namespace for pool",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "multiple binding referring to cluster role with multiple namespace for pool",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-2",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-2",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role binding that is same",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role bindings that are same",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role binding that are different 1",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role binding that are different 2",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role bindings that are different 3",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role bindings that are different 4",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}, {Kind: "Group", Name: "test-admin-group-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role bindings that are different 5",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-1"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()
			logger := log.New()
			logger.SetLevel(log.DebugLevel)
			controllerExpectations := controllerutils.NewExpectations(logger)
			rcp := &ReconcileClusterPool{
				Client:       fakeClient,
				logger:       logger,
				expectations: controllerExpectations,
			}

			err := rcp.reconcileRBAC(&hivev1.ClusterPool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testLeasePoolName,
					Namespace: testNamespace,
				},
			}, logger)
			if test.expectedErr == "" {
				require.NoError(t, err)

				rbs := &rbacv1.RoleBindingList{}
				err = fakeClient.List(context.Background(), rbs)
				require.NoError(t, err)
				sort.Slice(rbs.Items, func(i, j int) bool {
					return rbs.Items[i].Namespace < rbs.Items[j].Namespace && rbs.Items[i].Name < rbs.Items[j].Name
				})
				for idx := range rbs.Items {
					rbs.Items[idx].TypeMeta = metav1.TypeMeta{}
					rbs.Items[idx].ResourceVersion = ""
				}

				assert.Equal(t, test.expectedBindings, rbs.Items)
			} else {
				require.Regexp(t, err, test.expectedErr)
			}
		})
	}
}

func Test_isBroken(t *testing.T) {
	logger := log.New()

	poolNoHibernationConfig := hivev1.ClusterPool{
		Spec: hivev1.ClusterPoolSpec{},
	}
	poolEmptyHibernationConfig := hivev1.ClusterPool{
		Spec: hivev1.ClusterPoolSpec{
			HibernationConfig: &hivev1.HibernationConfig{},
		},
	}
	poolWithTimeout := hivev1.ClusterPool{
		Spec: hivev1.ClusterPoolSpec{
			HibernationConfig: &hivev1.HibernationConfig{
				ResumeTimeout: metav1.Duration{Duration: time.Second * 30},
			},
		},
	}

	tests := []struct {
		name string
		cd   *hivev1.ClusterDeployment
		pool *hivev1.ClusterPool
		want bool
	}{
		{
			name: "No ProvisionStopped condition (cluster still initializing)",
			cd:   testcd.BasicBuilder().Build(),
			pool: &poolNoHibernationConfig,
			want: false,
		},
		{
			name: "ProvisionStopped",
			cd: testcd.BasicBuilder().Options(testcd.WithCondition(
				hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionStoppedCondition,
					Status: corev1.ConditionTrue,
				},
			)).Build(),
			pool: &poolNoHibernationConfig,
			want: true,
		},
		{
			name: "No hibernation config",
			cd: testcd.BasicBuilder().Options(testcd.WithCondition(
				hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionStoppedCondition,
					Status: corev1.ConditionFalse,
				},
			)).Build(),
			pool: &poolNoHibernationConfig,
			want: false,
		},
		{
			name: "Empty hibernation config",
			cd: testcd.BasicBuilder().Options(testcd.WithCondition(
				hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionStoppedCondition,
					Status: corev1.ConditionFalse,
				},
			)).Build(),
			pool: &poolEmptyHibernationConfig,
			want: false,
		},
		{
			name: "CD not installed",
			cd: testcd.BasicBuilder().Options(testcd.WithCondition(
				hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionStoppedCondition,
					Status: corev1.ConditionFalse,
				},
			)).Build(),
			pool: &poolWithTimeout,
			want: false,
		},
		{
			name: "Desired powerState not Running",
			cd: testcd.BasicBuilder().Options(
				testcd.WithCondition(
					hivev1.ClusterDeploymentCondition{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionFalse,
					}),
				testcd.Installed(),
				testcd.WithPowerState(hivev1.ClusterPowerStateHibernating),
			).Build(),
			pool: &poolWithTimeout,
			want: false,
		},
		{
			name: "Actual powerState Hibernating",
			cd: testcd.BasicBuilder().Options(
				testcd.WithCondition(
					hivev1.ClusterDeploymentCondition{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionFalse,
					}),
				testcd.Installed(),
				testcd.WithPowerState(hivev1.ClusterPowerStateRunning),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateHibernating),
			).Build(),
			pool: &poolWithTimeout,
			want: false,
		},
		{
			name: "Actual powerState Running",
			cd: testcd.BasicBuilder().Options(
				testcd.WithCondition(
					hivev1.ClusterDeploymentCondition{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionFalse,
					}),
				testcd.Installed(),
				testcd.WithPowerState(hivev1.ClusterPowerStateRunning),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateRunning),
			).Build(),
			pool: &poolWithTimeout,
			want: false,
		},
		{
			name: "No Hibernating condition",
			cd: testcd.BasicBuilder().Options(
				testcd.WithCondition(
					hivev1.ClusterDeploymentCondition{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionFalse,
					}),
				testcd.Installed(),
				testcd.WithPowerState(hivev1.ClusterPowerStateRunning),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateWaitingForClusterOperators),
			).Build(),
			pool: &poolWithTimeout,
			want: false,
		},
		{
			name: "Hibernating condition nonsensical",
			cd: testcd.BasicBuilder().Options(
				testcd.WithCondition(
					hivev1.ClusterDeploymentCondition{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionFalse,
					}),
				testcd.WithCondition(
					hivev1.ClusterDeploymentCondition{
						Type:   hivev1.ClusterHibernatingCondition,
						Reason: hivev1.HibernatingReasonFailedToStop,
					}),
				testcd.Installed(),
				testcd.WithPowerState(hivev1.ClusterPowerStateRunning),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateWaitingForClusterOperators),
			).Build(),
			pool: &poolWithTimeout,
			want: false,
		},
		{
			name: "Timeout not expired",
			cd: testcd.BasicBuilder().Options(
				testcd.WithCondition(
					hivev1.ClusterDeploymentCondition{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionFalse,
					}),
				testcd.WithCondition(
					hivev1.ClusterDeploymentCondition{
						Type:               hivev1.ClusterHibernatingCondition,
						Reason:             hivev1.HibernatingReasonResumingOrRunning,
						LastTransitionTime: metav1.Now(),
					}),
				testcd.Installed(),
				testcd.WithPowerState(hivev1.ClusterPowerStateRunning),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateWaitingForClusterOperators),
			).Build(),
			pool: &poolWithTimeout,
			want: false,
		},
		{
			name: "Timeout expired",
			cd: testcd.BasicBuilder().Options(
				testcd.WithCondition(
					hivev1.ClusterDeploymentCondition{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionFalse,
					}),
				testcd.WithCondition(
					hivev1.ClusterDeploymentCondition{
						Type:               hivev1.ClusterHibernatingCondition,
						Reason:             hivev1.HibernatingReasonResumingOrRunning,
						LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
					}),
				testcd.Installed(),
				testcd.WithPowerState(hivev1.ClusterPowerStateRunning),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateWaitingForClusterOperators),
			).Build(),
			pool: &poolWithTimeout,
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBroken(tt.cd, tt.pool, logger); got != tt.want {
				t.Errorf("isBroken() = %v, want %v", got, tt.want)
			}
		})
	}
}
