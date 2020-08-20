package clustersync

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/pkg/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
	"github.com/openshift/hive/pkg/resource"
	resourcemock "github.com/openshift/hive/pkg/resource/mock"
	hiveassert "github.com/openshift/hive/pkg/test/assert"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testcs "github.com/openshift/hive/pkg/test/clustersync"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	testsecret "github.com/openshift/hive/pkg/test/secret"
	testselectorsyncset "github.com/openshift/hive/pkg/test/selectorsyncset"
	testsyncset "github.com/openshift/hive/pkg/test/syncset"
)

const (
	testNamespace       = "test-namespace"
	testCDName          = "test-cluster-deployment"
	testClusterSyncName = "test-cluster-deployment"
	testLeaseName       = "test-cluster-deployment-sync"
)

type reconcileTest struct {
	logger                          log.FieldLogger
	c                               client.Client
	r                               *ReconcileClusterSync
	mockCtrl                        *gomock.Controller
	mockResourceHelper              *resourcemock.MockHelper
	mockRemoteClientBuilder         *remoteclientmock.MockBuilder
	expectedFailedMessage           string
	expectedSyncSetStatuses         []hiveintv1alpha1.SyncStatus
	expectedSelectorSyncSetStatuses []hiveintv1alpha1.SyncStatus
	expectUnchangedLeaseRenewTime   bool
	expectRequeue                   bool
	expectNoWorkDone                bool
}

func newReconcileTest(t *testing.T, mockCtrl *gomock.Controller, scheme *runtime.Scheme, existing ...runtime.Object) *reconcileTest {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	c := fake.NewFakeClientWithScheme(scheme, existing...)

	mockResourceHelper := resourcemock.NewMockHelper(mockCtrl)
	mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)

	r := &ReconcileClusterSync{
		Client:          c,
		logger:          logger,
		reapplyInterval: defaultReapplyInterval,
		resourceHelperBuilder: func(rc *rest.Config, _ log.FieldLogger) (resource.Helper, error) {
			return mockResourceHelper, nil
		},
		remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder {
			return mockRemoteClientBuilder
		},
	}

	return &reconcileTest{
		logger:                  logger,
		c:                       c,
		r:                       r,
		mockCtrl:                mockCtrl,
		mockResourceHelper:      mockResourceHelper,
		mockRemoteClientBuilder: mockRemoteClientBuilder,
	}
}

func (rt *reconcileTest) run(t *testing.T) {
	if !rt.expectNoWorkDone {
		rt.mockRemoteClientBuilder.EXPECT().RESTConfig().Return(&rest.Config{}, nil)
	}

	var origLeaseRenewTime metav1.MicroTime
	if rt.expectUnchangedLeaseRenewTime {
		lease := &hiveintv1alpha1.ClusterSyncLease{}
		rt.c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testLeaseName}, lease)
		origLeaseRenewTime = lease.Spec.RenewTime
	}

	reconcileRequest := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      testCDName,
		},
	}

	startTime := time.Now()
	timeSinceOrigLeaseRenewTime := time.Since(origLeaseRenewTime.Time)
	result, err := rt.r.Reconcile(reconcileRequest)
	require.NoError(t, err, "unexpected error from Reconcile")
	endTime := time.Now()

	if rt.expectNoWorkDone {
		assert.False(t, result.Requeue, "expected no requeue")
		assert.Zero(t, result.RequeueAfter, "expected no requeue after")
		err = rt.c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testLeaseName}, &hiveintv1alpha1.ClusterSyncLease{})
		assert.True(t, apierrors.IsNotFound(err), "expected no lease")
		err = rt.c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testClusterSyncName}, &hiveintv1alpha1.ClusterSync{})
		assert.True(t, apierrors.IsNotFound(err), "expected no ClusterSync")
		return
	}

	assert.True(t, result.Requeue, "expected requeue to be true")
	if rt.expectRequeue {
		assert.Zero(t, result.RequeueAfter, "unexpected requeue after")
	} else {
		var minRequeueAfter, maxRequeueAfter float64
		if rt.expectUnchangedLeaseRenewTime {
			minRequeueAfter = (defaultReapplyInterval - timeSinceOrigLeaseRenewTime).Seconds()
			maxRequeueAfter = minRequeueAfter + defaultReapplyInterval.Seconds()*reapplyIntervalJitter + endTime.Sub(startTime).Seconds()
		} else {
			minRequeueAfter = (defaultReapplyInterval - endTime.Sub(startTime)).Seconds()
			maxRequeueAfter = defaultReapplyInterval.Seconds() * (1 + reapplyIntervalJitter)
		}
		assert.GreaterOrEqual(t, result.RequeueAfter.Seconds(), minRequeueAfter, "requeue after too small")
		assert.LessOrEqual(t, result.RequeueAfter.Seconds(), maxRequeueAfter, "requeue after too large")
	}

	lease := &hiveintv1alpha1.ClusterSyncLease{}
	err = rt.c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testLeaseName}, lease)
	require.NoError(t, err, "unexpected error getting lease for ClusterSync")

	if rt.expectUnchangedLeaseRenewTime {
		assert.Equal(t, origLeaseRenewTime, lease.Spec.RenewTime, "expected lease renew time to be unchanged")
	} else {
		if renewTime := lease.Spec.RenewTime; assert.NotNil(t, renewTime, "expected renew time to be set") {
			hiveassert.BetweenTimes(t, renewTime.Time, startTime, endTime, "unexpected renew time")
		}
	}

	clusterSync := &hiveintv1alpha1.ClusterSync{}
	err = rt.c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testClusterSyncName}, clusterSync)
	require.NoError(t, err, "unexpected error getting ClusterSync")

	var syncFailedCond *hiveintv1alpha1.ClusterSyncCondition
	for i, cond := range clusterSync.Status.Conditions {
		if cond.Type == hiveintv1alpha1.ClusterSyncFailed {
			syncFailedCond = &clusterSync.Status.Conditions[i]
			break
		}
	}
	assert.NotNil(t, syncFailedCond, "expected a sync failed condition")
	expectedConditionStatus := corev1.ConditionTrue
	expectedConditionMessage := rt.expectedFailedMessage
	if expectedConditionMessage == "" {
		expectedConditionStatus = corev1.ConditionFalse
		expectedConditionMessage = "All SyncSets and SelectorSyncSets have been applied to the cluster"
	}
	assert.Equal(t, string(expectedConditionStatus), string(syncFailedCond.Status), "unexpected sync failed status")
	assert.Equal(t, expectedConditionMessage, syncFailedCond.Message, "unexpected sync failed message")

	assert.Equal(t, rt.expectedSyncSetStatuses, clusterSync.Status.SyncSets, "unexpected syncset statuses")
	assert.Equal(t, rt.expectedSelectorSyncSetStatuses, clusterSync.Status.SelectorSyncSets, "unexpected selectorsyncset statuses")
}

func TestReconcileClusterSync_NewClusterDeployment(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	scheme := newScheme()
	rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build())
	rt.run(t)
}

func TestReconcileClusterSync_NoWorkToDo(t *testing.T) {
	scheme := newScheme()
	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment
	}{
		{
			name: "no ClusterDeployment",
			cd:   nil,
		},
		{
			name: "deleted ClusterDeployment",
			cd:   cdBuilder(scheme).GenericOptions(testgeneric.Deleted()).Build(),
		},
		{
			name: "unreachable",
			cd: cdBuilder(scheme).Build(
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.UnreachableCondition,
					Status: corev1.ConditionTrue,
				}),
			),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			var existing []runtime.Object
			if tc.cd != nil {
				existing = append(existing, tc.cd)
			}
			rt := newReconcileTest(t, mockCtrl, scheme, existing...)
			rt.expectNoWorkDone = true
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_ApplyResource(t *testing.T) {
	cases := []struct {
		applyMode                hivev1.SyncSetResourceApplyMode
		includeResourcesToDelete bool
	}{
		{
			applyMode:                hivev1.UpsertResourceApplyMode,
			includeResourcesToDelete: false,
		},
		{
			applyMode:                hivev1.SyncResourceApplyMode,
			includeResourcesToDelete: true,
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.applyMode), func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			resourceToApply := testConfigMap("dest-namespace", "dest-name")
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				func(ss *hivev1.SyncSet) {
					ss.Generation = 1
					ss.Spec.ResourceApplyMode = tc.applyMode
					ss.Spec.Resources = []runtime.RawExtension{{
						Object: resourceToApply,
					}}
				},
			)
			rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), syncSet)
			rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply)).Return(resource.CreatedApplyResult, nil)
			syncSetStatus := hiveintv1alpha1.SyncStatus{
				Name:               "test-syncset",
				ObservedGeneration: 1,
				Result:             hiveintv1alpha1.SuccessSyncSetResult,
			}
			if tc.includeResourcesToDelete {
				syncSetStatus.ResourcesToDelete = []hiveintv1alpha1.SyncResourceReference{
					testConfigMapRef("dest-namespace", "dest-name"),
				}
			}
			rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, syncSetStatus)
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_ApplySecret(t *testing.T) {
	cases := []struct {
		applyMode                hivev1.SyncSetResourceApplyMode
		includeResourcesToDelete bool
	}{
		{
			applyMode:                hivev1.UpsertResourceApplyMode,
			includeResourcesToDelete: false,
		},
		{
			applyMode:                hivev1.SyncResourceApplyMode,
			includeResourcesToDelete: true,
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.applyMode), func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				func(ss *hivev1.SyncSet) {
					ss.Generation = 1
					ss.Spec.ResourceApplyMode = tc.applyMode
					ss.Spec.Secrets = []hivev1.SecretMapping{
						testSecretMapping("test-secret", "dest-namespace", "dest-name"),
					}
				},
			)
			srcSecret := testsecret.FullBuilder(testNamespace, "test-secret", scheme).Build(
				testsecret.WithDataKeyValue("test-key", []byte("test-data")),
			)
			rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), syncSet, srcSecret)
			secretToApply := testsecret.BasicBuilder().GenericOptions(
				testgeneric.WithNamespace("dest-namespace"),
				testgeneric.WithName("dest-name"),
				testgeneric.WithTypeMeta(scheme),
			).Build(
				testsecret.WithDataKeyValue("test-key", []byte("test-data")),
			)
			rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(secretToApply)).Return(resource.CreatedApplyResult, nil)
			syncSetStatus := hiveintv1alpha1.SyncStatus{
				Name:               "test-syncset",
				ObservedGeneration: 1,
				Result:             hiveintv1alpha1.SuccessSyncSetResult,
			}
			if tc.includeResourcesToDelete {
				syncSetStatus.ResourcesToDelete = []hiveintv1alpha1.SyncResourceReference{
					testSecretRef("dest-namespace", "dest-name"),
				}
			}
			rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, syncSetStatus)
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_ApplyPatch(t *testing.T) {
	cases := []struct {
		applyMode hivev1.SyncSetResourceApplyMode
	}{
		{applyMode: hivev1.UpsertResourceApplyMode},
		{applyMode: hivev1.SyncResourceApplyMode},
	}
	for _, tc := range cases {
		t.Run(string(tc.applyMode), func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				func(ss *hivev1.SyncSet) {
					ss.Generation = 1
					ss.Spec.ResourceApplyMode = tc.applyMode
					ss.Spec.Patches = []hivev1.SyncObjectPatch{{
						APIVersion: "v1",
						Kind:       "ConfigMap",
						Namespace:  "dest-namespace",
						Name:       "dest-name",
						PatchType:  "patch-type",
						Patch:      "test-patch",
					}}
				},
			)
			rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), syncSet)
			rt.mockResourceHelper.EXPECT().Patch(
				types.NamespacedName{Namespace: "dest-namespace", Name: "dest-name"},
				"ConfigMap",
				"v1",
				[]byte("test-patch"),
				"patch-type",
			).Return(nil)
			rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses,
				hiveintv1alpha1.SyncStatus{
					Name:               "test-syncset",
					ObservedGeneration: 1,
					Result:             hiveintv1alpha1.SuccessSyncSetResult,
				},
			)
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_ApplyAllTypes(t *testing.T) {
	cases := []struct {
		applyMode                hivev1.SyncSetResourceApplyMode
		includeResourcesToDelete bool
	}{
		{
			applyMode:                hivev1.UpsertResourceApplyMode,
			includeResourcesToDelete: false,
		},
		{
			applyMode:                hivev1.SyncResourceApplyMode,
			includeResourcesToDelete: true,
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.applyMode), func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			resourceToApply := testConfigMap("resource-namespace", "resource-name")
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				func(ss *hivev1.SyncSet) {
					ss.Generation = 1
					ss.Spec.ResourceApplyMode = tc.applyMode
					ss.Spec.Resources = []runtime.RawExtension{{
						Object: resourceToApply,
					}}
					ss.Spec.Secrets = []hivev1.SecretMapping{
						testSecretMapping("test-secret", "secret-namespace", "secret-name"),
					}
					ss.Spec.Patches = []hivev1.SyncObjectPatch{{
						APIVersion: "patch-api/v1",
						Kind:       "PatchKind",
						Namespace:  "patch-namespace",
						Name:       "patch-name",
						PatchType:  "patch-type",
						Patch:      "test-patch",
					}}
				},
			)
			srcSecret := testsecret.FullBuilder(testNamespace, "test-secret", scheme).Build(
				testsecret.WithDataKeyValue("test-key", []byte("test-data")),
			)
			rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), syncSet, srcSecret)
			secretToApply := testsecret.BasicBuilder().GenericOptions(
				testgeneric.WithNamespace("secret-namespace"),
				testgeneric.WithName("secret-name"),
				testgeneric.WithTypeMeta(scheme),
			).Build(
				testsecret.WithDataKeyValue("test-key", []byte("test-data")),
			)
			rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply)).Return(resource.CreatedApplyResult, nil)
			rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(secretToApply)).Return(resource.CreatedApplyResult, nil)
			rt.mockResourceHelper.EXPECT().Patch(
				types.NamespacedName{Namespace: "patch-namespace", Name: "patch-name"},
				"PatchKind",
				"patch-api/v1",
				[]byte("test-patch"),
				"patch-type",
			).Return(nil)
			syncSetStatus := hiveintv1alpha1.SyncStatus{
				Name:               "test-syncset",
				ObservedGeneration: 1,
				Result:             hiveintv1alpha1.SuccessSyncSetResult,
			}
			if tc.includeResourcesToDelete {
				syncSetStatus.ResourcesToDelete = []hiveintv1alpha1.SyncResourceReference{
					testConfigMapRef("resource-namespace", "resource-name"),
					testSecretRef("secret-namespace", "secret-name"),
				}
			}
			rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, syncSetStatus)
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_Reapply(t *testing.T) {
	cases := []struct {
		name          string
		noClusterSync bool
		noSyncLease   bool
		renewTime     time.Time
		expectApply   bool
	}{
		{
			name:        "too soon",
			renewTime:   time.Now().Add(-time.Hour),
			expectApply: false,
		},
		{
			name:        "time for reapply",
			renewTime:   time.Now().Add(-3 * time.Hour),
			expectApply: true,
		},
		{
			name:          "too soon but no ClusterSync",
			noClusterSync: true,
			renewTime:     time.Now().Add(-time.Hour),
			expectApply:   true,
		},
		{
			name:        "no sync lease",
			noSyncLease: true,
			expectApply: true,
		},
		{
			name:        "sync lease with no renew time",
			expectApply: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			resourceToApply := testConfigMap("dest-namespace", "dest-name")
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				func(ss *hivev1.SyncSet) {
					ss.Generation = 1
					ss.Spec.Resources = []runtime.RawExtension{{
						Object: resourceToApply,
					}}
				},
			)
			syncStatus := hiveintv1alpha1.SyncStatus{
				Name:               "test-syncset",
				ObservedGeneration: 1,
				Result:             hiveintv1alpha1.SuccessSyncSetResult,
			}
			existing := []runtime.Object{cdBuilder(scheme).Build(), syncSet}
			if !tc.noClusterSync {
				existing = append(existing, clusterSyncBuilder(scheme).Build(testcs.WithSyncSetStatus(syncStatus)))
			}
			if !tc.noSyncLease {
				existing = append(existing, buildSyncLease(tc.renewTime))
			}
			rt := newReconcileTest(t, mockCtrl, scheme, existing...)
			rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, syncStatus)
			if tc.expectApply {
				rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply)).Return(resource.CreatedApplyResult, nil)
			} else {
				rt.expectUnchangedLeaseRenewTime = true
			}
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_NewSyncSetApplied(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	scheme := newScheme()
	existingResource := testConfigMap("dest-namespace", "dest-name")
	existingSyncSet := testsyncset.FullBuilder(testNamespace, "existing-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		func(ss *hivev1.SyncSet) {
			ss.Generation = 1
			ss.Spec.Resources = []runtime.RawExtension{{
				Object: existingResource,
			}}
		},
	)
	existingSyncStatus := hiveintv1alpha1.SyncStatus{
		Name:               "existing-syncset",
		ObservedGeneration: 1,
		Result:             hiveintv1alpha1.SuccessSyncSetResult,
	}
	newResource := testConfigMap("other-namespace", "other-name")
	newSyncSet := testsyncset.FullBuilder(testNamespace, "new-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		func(ss *hivev1.SyncSet) {
			ss.Generation = 1
			ss.Spec.Resources = []runtime.RawExtension{{
				Object: newResource,
			}}
		},
	)
	newSyncStatus := hiveintv1alpha1.SyncStatus{
		Name:               "new-syncset",
		ObservedGeneration: 1,
		Result:             hiveintv1alpha1.SuccessSyncSetResult,
	}
	clusterSync := clusterSyncBuilder(scheme).Build(testcs.WithSyncSetStatus(existingSyncStatus))
	lease := buildSyncLease(time.Now().Add(-1 * time.Hour))
	rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), existingSyncSet, newSyncSet, clusterSync, lease)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(newResource)).Return(resource.CreatedApplyResult, nil)
	rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, existingSyncStatus, newSyncStatus)
	rt.expectUnchangedLeaseRenewTime = true
	rt.run(t)
}

func TestReconcileClusterSync_SyncSetDeleted(t *testing.T) {
	cases := []struct {
		name                     string
		includeResourcesToDelete bool
		expectDelete             bool
	}{
		{
			name:                     "upsert",
			includeResourcesToDelete: false,
			expectDelete:             false,
		},
		{
			name:                     "sync",
			includeResourcesToDelete: true,
			expectDelete:             true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			existingSyncStatus := hiveintv1alpha1.SyncStatus{
				Name:               "test-syncset",
				ObservedGeneration: 1,
				Result:             hiveintv1alpha1.SuccessSyncSetResult,
			}
			if tc.includeResourcesToDelete {
				existingSyncStatus.ResourcesToDelete = []hiveintv1alpha1.SyncResourceReference{
					testConfigMapRef("dest-namespace", "dest-name"),
				}
			}
			clusterSync := clusterSyncBuilder(scheme).Build(testcs.WithSyncSetStatus(existingSyncStatus))
			lease := buildSyncLease(time.Now().Add(-1 * time.Hour))
			rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), clusterSync, lease)
			if tc.expectDelete {
				rt.mockResourceHelper.EXPECT().
					Delete("v1", "ConfigMap", "dest-namespace", "dest-name").
					Return(nil)
			}
			rt.expectUnchangedLeaseRenewTime = true
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_ResourceRemovedFromSyncSet(t *testing.T) {
	cases := []struct {
		name                     string
		includeResourcesToDelete bool
		expectDelete             bool
	}{
		{
			name:                     "upsert",
			includeResourcesToDelete: false,
			expectDelete:             false,
		},
		{
			name:                     "sync",
			includeResourcesToDelete: true,
			expectDelete:             true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			resourceToApply := testConfigMap("dest-namespace", "retained-resource")
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				func(ss *hivev1.SyncSet) {
					ss.Generation = 2
					ss.Spec.Resources = []runtime.RawExtension{{
						Object: resourceToApply,
					}}
				},
			)
			existingSyncStatus := hiveintv1alpha1.SyncStatus{
				Name:               "test-syncset",
				ObservedGeneration: 1,
				Result:             hiveintv1alpha1.SuccessSyncSetResult,
			}
			if tc.includeResourcesToDelete {
				existingSyncStatus.ResourcesToDelete = []hiveintv1alpha1.SyncResourceReference{
					testConfigMapRef("dest-namespace", "deleted-resource"),
					testConfigMapRef("dest-namespace", "retained-resource"),
				}
			}
			clusterSync := clusterSyncBuilder(scheme).Build(testcs.WithSyncSetStatus(existingSyncStatus))
			lease := buildSyncLease(time.Now().Add(-1 * time.Hour))
			rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), syncSet, clusterSync, lease)
			rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply)).Return(resource.CreatedApplyResult, nil)
			if tc.expectDelete {
				rt.mockResourceHelper.EXPECT().
					Delete("v1", "ConfigMap", "dest-namespace", "deleted-resource").
					Return(nil)
			}
			expectedSyncStatus := hiveintv1alpha1.SyncStatus{
				Name:               "test-syncset",
				ObservedGeneration: 2,
				Result:             hiveintv1alpha1.SuccessSyncSetResult,
			}
			if tc.includeResourcesToDelete {
				expectedSyncStatus.ResourcesToDelete = []hiveintv1alpha1.SyncResourceReference{
					testConfigMapRef("dest-namespace", "retained-resource"),
				}
			}
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{expectedSyncStatus}
			rt.expectUnchangedLeaseRenewTime = true
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_ErrorApplyingResource(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	scheme := newScheme()
	resourceToApply := testConfigMap("dest-namespace", "dest-name")
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		func(ss *hivev1.SyncSet) {
			ss.Generation = 1
			ss.Spec.Resources = []runtime.RawExtension{{
				Object: resourceToApply,
			}}
		},
	)
	rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), syncSet)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply)).
		Return(resource.ApplyResult(""), errors.New("test apply error"))
	rt.expectedFailedMessage = "SyncSet test-syncset is failing"
	syncSetStatus := hiveintv1alpha1.SyncStatus{
		Name:               "test-syncset",
		ObservedGeneration: 1,
		Result:             hiveintv1alpha1.FailureSyncSetResult,
		FailureMessage:     "Failed to apply resource 0: test apply error",
	}
	rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, syncSetStatus)
	rt.expectRequeue = true
	rt.run(t)
}

func TestReconcileClusterSync_ErrorApplyingSecret(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	scheme := newScheme()
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		func(ss *hivev1.SyncSet) {
			ss.Generation = 1
			ss.Spec.Secrets = []hivev1.SecretMapping{
				testSecretMapping("test-secret", "dest-namespace", "dest-name"),
			}
		},
	)
	srcSecret := testsecret.FullBuilder(testNamespace, "test-secret", scheme).Build(
		testsecret.WithDataKeyValue("test-key", []byte("test-data")),
	)
	rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), syncSet, srcSecret)
	secretToApply := testsecret.BasicBuilder().GenericOptions(
		testgeneric.WithNamespace("dest-namespace"),
		testgeneric.WithName("dest-name"),
		testgeneric.WithTypeMeta(scheme),
	).Build(
		testsecret.WithDataKeyValue("test-key", []byte("test-data")),
	)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(secretToApply)).
		Return(resource.ApplyResult(""), errors.New("test apply error"))
	rt.expectedFailedMessage = "SyncSet test-syncset is failing"
	syncSetStatus := hiveintv1alpha1.SyncStatus{
		Name:               "test-syncset",
		ObservedGeneration: 1,
		Result:             hiveintv1alpha1.FailureSyncSetResult,
		FailureMessage:     "Failed to apply secret 0: test apply error",
	}
	rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, syncSetStatus)
	rt.expectRequeue = true
	rt.run(t)
}

func TestReconcileClusterSync_ErrorApplyingPatch(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	scheme := newScheme()
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		func(ss *hivev1.SyncSet) {
			ss.Generation = 1
			ss.Spec.Patches = []hivev1.SyncObjectPatch{{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  "dest-namespace",
				Name:       "dest-name",
				PatchType:  "patch-type",
				Patch:      "test-patch",
			}}
		},
	)
	rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), syncSet)
	rt.mockResourceHelper.EXPECT().Patch(
		types.NamespacedName{Namespace: "dest-namespace", Name: "dest-name"},
		"ConfigMap",
		"v1",
		[]byte("test-patch"),
		"patch-type",
	).Return(errors.New("test patch error"))
	rt.expectedFailedMessage = "SyncSet test-syncset is failing"
	rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses,
		hiveintv1alpha1.SyncStatus{
			Name:               "test-syncset",
			ObservedGeneration: 1,
			Result:             hiveintv1alpha1.FailureSyncSetResult,
			FailureMessage:     "Failed to apply patch 0: test patch error",
		},
	)
	rt.expectRequeue = true
	rt.run(t)
}

func TestReconcileClusterSync_SkipAfterFailingResource(t *testing.T) {
	cases := []struct {
		name                string
		successfulResources int
		successfulSecrets   int
		successfulPatches   int
		failureMessage      string
	}{
		{
			name:                "resource 0 fails",
			successfulResources: 0,
			failureMessage:      "Failed to apply resource 0: test apply error",
		},
		{
			name:                "resource 1 fails",
			successfulResources: 1,
			failureMessage:      "Failed to apply resource 1: test apply error",
		},
		{
			name:                "resource 2 fails",
			successfulResources: 2,
			failureMessage:      "Failed to apply resource 2: test apply error",
		},
		{
			name:                "secret 0 fails",
			successfulResources: 3,
			successfulSecrets:   0,
			failureMessage:      "Failed to apply secret 0: test apply error",
		},
		{
			name:                "secret 1 fails",
			successfulResources: 3,
			successfulSecrets:   1,
			failureMessage:      "Failed to apply secret 1: test apply error",
		},
		{
			name:                "secret 2 fails",
			successfulResources: 3,
			successfulSecrets:   2,
			failureMessage:      "Failed to apply secret 2: test apply error",
		},
		{
			name:                "patch 0 fails",
			successfulResources: 3,
			successfulSecrets:   3,
			successfulPatches:   0,
			failureMessage:      "Failed to apply patch 0: test patch error",
		},
		{
			name:                "patch 1 fails",
			successfulResources: 3,
			successfulSecrets:   3,
			successfulPatches:   1,
			failureMessage:      "Failed to apply patch 1: test patch error",
		},
		{
			name:                "patch 2 fails",
			successfulResources: 3,
			successfulSecrets:   3,
			successfulPatches:   2,
			failureMessage:      "Failed to apply patch 2: test patch error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			resourcesToApply := make([]hivev1.MetaRuntimeObject, 3)
			for i := range resourcesToApply {
				resourcesToApply[i] = testConfigMap(
					fmt.Sprintf("resource-namespace-%d", i),
					fmt.Sprintf("resource-name-%d", i),
				)
			}
			srcSecrets := make([]*corev1.Secret, 3)
			for i := range srcSecrets {
				srcSecrets[i] = testsecret.FullBuilder(testNamespace, fmt.Sprintf("test-secret-%d", i), scheme).Build(
					testsecret.WithDataKeyValue(
						fmt.Sprintf("test-key-%d", i),
						[]byte(fmt.Sprintf("test-data-%d", i)),
					),
				)
			}
			secretsToApply := make([]*corev1.Secret, len(srcSecrets))
			for i := range secretsToApply {
				secretsToApply[i] = testsecret.BasicBuilder().GenericOptions(
					testgeneric.WithNamespace(fmt.Sprintf("secret-namespace-%d", i)),
					testgeneric.WithName(fmt.Sprintf("secret-name-%d", i)),
					testgeneric.WithTypeMeta(scheme),
				).Build(
					testsecret.WithDataKeyValue(
						fmt.Sprintf("test-key-%d", i),
						[]byte(fmt.Sprintf("test-data-%d", i)),
					),
				)
			}
			patchesToApply := make([]hivev1.SyncObjectPatch, 3)
			for i := range patchesToApply {
				patchesToApply[i] = hivev1.SyncObjectPatch{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Namespace:  fmt.Sprintf("patch-namespace-%d", i),
					Name:       fmt.Sprintf("patch-name-%d", i),
					PatchType:  fmt.Sprintf("patch-type-%d", i),
					Patch:      fmt.Sprintf("test-patch-%d", i),
				}
			}
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				func(ss *hivev1.SyncSet) {
					ss.Generation = 1
					ss.Spec.Resources = make([]runtime.RawExtension, len(resourcesToApply))
					for i, r := range resourcesToApply {
						ss.Spec.Resources[i].Object = r
					}
					ss.Spec.Secrets = make([]hivev1.SecretMapping, len(srcSecrets))
					for i := range srcSecrets {
						ss.Spec.Secrets[i] = testSecretMapping(
							fmt.Sprintf("test-secret-%d", i),
							fmt.Sprintf("secret-namespace-%d", i),
							fmt.Sprintf("secret-name-%d", i),
						)
					}
					ss.Spec.Patches = make([]hivev1.SyncObjectPatch, len(patchesToApply))
					for i, p := range patchesToApply {
						ss.Spec.Patches[i] = p
					}
				},
			)
			existing := []runtime.Object{cdBuilder(scheme).Build(), syncSet}
			for _, s := range srcSecrets {
				existing = append(existing, s)
			}
			rt := newReconcileTest(t, mockCtrl, scheme, existing...)
			var resourceHelperCalls []*gomock.Call
			for i := 0; i < tc.successfulResources; i++ {
				resourceHelperCalls = append(resourceHelperCalls,
					rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourcesToApply[i])).
						Return(resource.CreatedApplyResult, nil))
			}
			if tc.successfulResources < len(resourcesToApply) {
				resourceHelperCalls = append(resourceHelperCalls,
					rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourcesToApply[tc.successfulResources])).
						Return(resource.ApplyResult(""), errors.New("test apply error")))
			}
			for i := 0; i < tc.successfulSecrets; i++ {
				resourceHelperCalls = append(resourceHelperCalls,
					rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(secretsToApply[i])).
						Return(resource.CreatedApplyResult, nil))
			}
			if tc.successfulResources == len(resourcesToApply) && tc.successfulSecrets < len(srcSecrets) {
				resourceHelperCalls = append(resourceHelperCalls,
					rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(secretsToApply[tc.successfulSecrets])).
						Return(resource.ApplyResult(""), errors.New("test apply error")))
			}
			for i := 0; i < tc.successfulPatches; i++ {
				patch := patchesToApply[i]
				resourceHelperCalls = append(resourceHelperCalls,
					rt.mockResourceHelper.EXPECT().Patch(
						types.NamespacedName{Namespace: patch.Namespace, Name: patch.Name},
						patch.Kind,
						patch.APIVersion,
						[]byte(patch.Patch),
						patch.PatchType,
					).Return(nil))
			}
			if tc.successfulResources == len(resourcesToApply) && tc.successfulSecrets == len(secretsToApply) && tc.successfulPatches < len(patchesToApply) {
				patch := patchesToApply[tc.successfulPatches]
				resourceHelperCalls = append(resourceHelperCalls,
					rt.mockResourceHelper.EXPECT().Patch(
						types.NamespacedName{Namespace: patch.Namespace, Name: patch.Name},
						patch.Kind,
						patch.APIVersion,
						[]byte(patch.Patch),
						patch.PatchType,
					).Return(errors.New("test patch error")))
			}
			gomock.InOrder(resourceHelperCalls...)
			rt.expectedFailedMessage = "SyncSet test-syncset is failing"
			syncSetStatus := hiveintv1alpha1.SyncStatus{
				Name:               "test-syncset",
				ObservedGeneration: 1,
				Result:             hiveintv1alpha1.FailureSyncSetResult,
				FailureMessage:     tc.failureMessage,
			}
			rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, syncSetStatus)
			rt.expectRequeue = true
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_ResourcesToDeleteAreOrdered(t *testing.T) {
	scheme := newScheme()
	resourcesToApply := []hivev1.MetaRuntimeObject{
		testConfigMap("namespace-A", "name-A"),
		testConfigMap("namespace-A", "name-B"),
		testConfigMap("namespace-B", "name-A"),
		&corev1.Service{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Service",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "namespace-A",
				Name:      "name-A",
			},
		},
		&hivev1.ClusterDeployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "hive.openshift.io/v1",
				Kind:       "ClusterDeployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "namespace-A",
				Name:      "name-A",
			},
		},
	}
	srcSecret := testsecret.FullBuilder(testNamespace, "test-secret", scheme).Build(
		testsecret.WithDataKeyValue("test-key", []byte("test-data")),
	)
	secretMappings := []hivev1.SecretMapping{
		testSecretMapping("test-secret", "namespace-A", "name-A"),
		testSecretMapping("test-secret", "namespace-A", "name-B"),
		testSecretMapping("test-secret", "namespace-B", "name-A"),
	}
	permutation := 0
	roa := make([]interface{}, len(resourcesToApply))
	for i, r := range resourcesToApply {
		roa[i] = r
	}
	sm := make([]interface{}, len(secretMappings))
	for i, m := range secretMappings {
		sm[i] = m
	}
	permute(roa, func(roa []interface{}) {
		permute(sm, func(sm []interface{}) {
			resourcesToApply = make([]hivev1.MetaRuntimeObject, len(roa))
			for i, r := range roa {
				resourcesToApply[i] = r.(hivev1.MetaRuntimeObject)
			}
			secretMappings = make([]hivev1.SecretMapping, len(sm))
			for i, m := range sm {
				secretMappings[i] = m.(hivev1.SecretMapping)
			}
			t.Run(fmt.Sprintf("permutation %03d", permutation), func(t *testing.T) {
				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()
				syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
					testsyncset.ForClusterDeployments(testCDName),
					func(ss *hivev1.SyncSet) {
						ss.Generation = 1
						ss.Spec.ResourceApplyMode = hivev1.SyncResourceApplyMode
						ss.Spec.Resources = make([]runtime.RawExtension, len(resourcesToApply))
						for i, r := range resourcesToApply {
							ss.Spec.Resources[i].Object = r
						}
						ss.Spec.Secrets = secretMappings
					},
				)
				clusterSync := clusterSyncBuilder(scheme).Build(
					testcs.WithSyncSetStatus(hiveintv1alpha1.SyncStatus{
						Name:               "test-syncset",
						ObservedGeneration: 0,
						ResourcesToDelete: []hiveintv1alpha1.SyncResourceReference{
							testConfigMapRef("namespace-A", "resource-failing-to-delete-A"),
							testConfigMapRef("namespace-A", "resource-failing-to-delete-B"),
							testConfigMapRef("namespace-B", "resource-failing-to-delete-A"),
						},
						Result: hiveintv1alpha1.SuccessSyncSetResult,
					}),
				)
				rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), clusterSync, syncSet, srcSecret)
				var resourceHelperCalls []*gomock.Call
				for _, r := range resourcesToApply {
					resourceHelperCalls = append(resourceHelperCalls,
						rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(r)).
							Return(resource.CreatedApplyResult, nil))
				}
				for _, s := range secretMappings {
					secretToApply := testsecret.BasicBuilder().GenericOptions(
						testgeneric.WithNamespace(s.TargetRef.Namespace),
						testgeneric.WithName(s.TargetRef.Name),
						testgeneric.WithTypeMeta(scheme),
					).Build(
						testsecret.WithDataKeyValue("test-key", []byte("test-data")),
					)
					resourceHelperCalls = append(resourceHelperCalls,
						rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(secretToApply)).
							Return(resource.CreatedApplyResult, nil))
				}
				resourceHelperCalls = append(resourceHelperCalls,
					rt.mockResourceHelper.EXPECT().Delete("v1", "ConfigMap", "namespace-A", "resource-failing-to-delete-A").
						Return(errors.New("error deleting resource")),
					rt.mockResourceHelper.EXPECT().Delete("v1", "ConfigMap", "namespace-A", "resource-failing-to-delete-B").
						Return(errors.New("error deleting resource")),
					rt.mockResourceHelper.EXPECT().Delete("v1", "ConfigMap", "namespace-B", "resource-failing-to-delete-A").
						Return(errors.New("error deleting resource")),
				)
				gomock.InOrder(resourceHelperCalls...)
				rt.expectedFailedMessage = "SyncSet test-syncset is failing"
				syncSetStatus := hiveintv1alpha1.SyncStatus{
					Name:               "test-syncset",
					ObservedGeneration: 1,
					Result:             hiveintv1alpha1.FailureSyncSetResult,
					FailureMessage:     "[Failed to delete v1, Kind=ConfigMap namespace-A/resource-failing-to-delete-A: error deleting resource, Failed to delete v1, Kind=ConfigMap namespace-A/resource-failing-to-delete-B: error deleting resource, Failed to delete v1, Kind=ConfigMap namespace-B/resource-failing-to-delete-A: error deleting resource]",
					ResourcesToDelete: []hiveintv1alpha1.SyncResourceReference{
						{
							APIVersion: "hive.openshift.io/v1",
							Kind:       "ClusterDeployment",
							Namespace:  "namespace-A",
							Name:       "name-A",
						},
						testConfigMapRef("namespace-A", "name-A"),
						testConfigMapRef("namespace-A", "name-B"),
						testConfigMapRef("namespace-A", "resource-failing-to-delete-A"),
						testConfigMapRef("namespace-A", "resource-failing-to-delete-B"),
						testConfigMapRef("namespace-B", "name-A"),
						testConfigMapRef("namespace-B", "resource-failing-to-delete-A"),
						testSecretRef("namespace-A", "name-A"),
						testSecretRef("namespace-A", "name-B"),
						testSecretRef("namespace-B", "name-A"),
						{
							APIVersion: "v1",
							Kind:       "Service",
							Namespace:  "namespace-A",
							Name:       "name-A",
						},
					},
				}
				rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, syncSetStatus)
				rt.expectRequeue = true
				rt.run(t)
			})
			permutation++
		})
	})
}

func TestReconcileClusterSync_FailingSyncSetDoesNotBlockOtherSyncSets(t *testing.T) {
	cases := []struct {
		name           string
		failingSyncSet int
	}{
		{
			name:           "resource 0 fails",
			failingSyncSet: 0,
		},
		{
			name:           "resource 1 fails",
			failingSyncSet: 1,
		},
		{
			name:           "resource 2 fails",
			failingSyncSet: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			resourcesToApply := make([]hivev1.MetaRuntimeObject, 3)
			for i := range resourcesToApply {
				resourcesToApply[i] = testConfigMap(
					fmt.Sprintf("resource-namespace-%d", i),
					fmt.Sprintf("resource-name-%d", i),
				)
			}
			syncSets := make([]*hivev1.SyncSet, len(resourcesToApply))
			for i := range resourcesToApply {
				syncSets[i] = testsyncset.FullBuilder(testNamespace, fmt.Sprintf("test-syncset-%d", i), scheme).Build(
					testsyncset.ForClusterDeployments(testCDName),
					func(ss *hivev1.SyncSet) {
						ss.Generation = 1
						ss.Spec.Resources = []runtime.RawExtension{{
							Object: resourcesToApply[i],
						}}
					},
				)
			}
			existing := []runtime.Object{cdBuilder(scheme).Build()}
			for _, r := range resourcesToApply {
				existing = append(existing, r)
			}
			for _, s := range syncSets {
				existing = append(existing, s)
			}
			rt := newReconcileTest(t, mockCtrl, scheme, existing...)
			for i, r := range resourcesToApply {
				if i == tc.failingSyncSet {
					rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(r)).
						Return(resource.ApplyResult(""), errors.New("test apply error"))
				} else {
					rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(r)).
						Return(resource.CreatedApplyResult, nil)
				}
			}
			rt.expectedFailedMessage = fmt.Sprintf("SyncSet test-syncset-%d is failing", tc.failingSyncSet)
			for i, s := range syncSets {
				syncSetStatus := hiveintv1alpha1.SyncStatus{
					Name:               s.Name,
					ObservedGeneration: s.Generation,
				}
				if i == tc.failingSyncSet {
					syncSetStatus.Result = hiveintv1alpha1.FailureSyncSetResult
					syncSetStatus.FailureMessage = "Failed to apply resource 0: test apply error"
				} else {
					syncSetStatus.Result = hiveintv1alpha1.SuccessSyncSetResult
				}
				rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, syncSetStatus)
			}
			rt.expectRequeue = true
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_FailureMessage(t *testing.T) {
	cases := []struct {
		name                    string
		failingSyncSets         int
		failingSelectorSyncSets int
		expectedFailedMessage   string
	}{
		{
			name:                  "multiple failing syncsets",
			failingSyncSets:       3,
			expectedFailedMessage: "SyncSets test-syncset-0, test-syncset-1, test-syncset-2 are failing",
		},
		{
			name:                    "multiple failing selectorsyncsets",
			failingSelectorSyncSets: 3,
			expectedFailedMessage:   "SelectorSyncSets test-selectorsyncset-0, test-selectorsyncset-1, test-selectorsyncset-2 are failing",
		},
		{
			name:                    "one failing syncset and selectorsyncset",
			failingSyncSets:         1,
			failingSelectorSyncSets: 1,
			expectedFailedMessage:   "SyncSet test-syncset-0 and SelectorSyncSet test-selectorsyncset-0 are failing",
		},
		{
			name:                    "multiple failing syncsets and selectorsyncsets",
			failingSyncSets:         3,
			failingSelectorSyncSets: 3,
			expectedFailedMessage:   "SyncSets test-syncset-0, test-syncset-1, test-syncset-2 and SelectorSyncSets test-selectorsyncset-0, test-selectorsyncset-1, test-selectorsyncset-2 are failing",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			syncSets := make([]runtime.Object, tc.failingSyncSets)
			for i := range syncSets {
				syncSets[i] = testsyncset.FullBuilder(testNamespace, fmt.Sprintf("test-syncset-%d", i), scheme).Build(
					testsyncset.ForClusterDeployments(testCDName),
					func(ss *hivev1.SyncSet) {
						ss.Generation = 1
						ss.Spec.Resources = []runtime.RawExtension{{
							Object: testConfigMap(
								fmt.Sprintf("syncset-namespace-%d", i),
								fmt.Sprintf("syncset-name-%d", i),
							),
						}}
					},
				)
			}
			selectorSyncSets := make([]runtime.Object, tc.failingSelectorSyncSets)
			for i := range selectorSyncSets {
				selectorSyncSets[i] = testselectorsyncset.FullBuilder(fmt.Sprintf("test-selectorsyncset-%d", i), scheme).Build(
					testselectorsyncset.WithLabelSelector("test-label-key", "test-label-value"),
					func(ss *hivev1.SelectorSyncSet) {
						ss.Generation = 1
						ss.Spec.Resources = []runtime.RawExtension{{
							Object: testConfigMap(
								fmt.Sprintf("selectorsyncset-namespace-%d", i),
								fmt.Sprintf("selectorsyncset-name-%d", i),
							),
						}}
					},
				)
			}
			existing := []runtime.Object{cdBuilder(scheme).Build(testcd.WithLabel("test-label-key", "test-label-value"))}
			existing = append(existing, syncSets...)
			existing = append(existing, selectorSyncSets...)
			rt := newReconcileTest(t, mockCtrl, scheme, existing...)
			rt.mockResourceHelper.EXPECT().Apply(gomock.Any()).
				Return(resource.ApplyResult(""), errors.New("test apply error")).
				Times(tc.failingSyncSets + tc.failingSelectorSyncSets)
			rt.expectedFailedMessage = tc.expectedFailedMessage
			if tc.failingSyncSets > 0 {
				rt.expectedSyncSetStatuses = make([]hiveintv1alpha1.SyncStatus, tc.failingSyncSets)
				for i := range rt.expectedSyncSetStatuses {
					rt.expectedSyncSetStatuses[i] = hiveintv1alpha1.SyncStatus{
						Name:               fmt.Sprintf("test-syncset-%d", i),
						ObservedGeneration: 1,
						Result:             hiveintv1alpha1.FailureSyncSetResult,
						FailureMessage:     "Failed to apply resource 0: test apply error",
					}
				}
			}
			if tc.failingSelectorSyncSets > 0 {
				rt.expectedSelectorSyncSetStatuses = make([]hiveintv1alpha1.SyncStatus, tc.failingSelectorSyncSets)
				for i := range rt.expectedSelectorSyncSetStatuses {
					rt.expectedSelectorSyncSetStatuses[i] = hiveintv1alpha1.SyncStatus{
						Name:               fmt.Sprintf("test-selectorsyncset-%d", i),
						ObservedGeneration: 1,
						Result:             hiveintv1alpha1.FailureSyncSetResult,
						FailureMessage:     "Failed to apply resource 0: test apply error",
					}
				}
			}
			rt.expectRequeue = true
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_PartialApply(t *testing.T) {
	cases := []struct {
		name       string
		syncStatus hiveintv1alpha1.SyncStatus
	}{
		{
			name: "last apply failed",
			syncStatus: hiveintv1alpha1.SyncStatus{
				Name:               "test-syncset",
				ObservedGeneration: 2,
				Result:             hiveintv1alpha1.FailureSyncSetResult,
			},
		},
		{
			name: "syncset generation changed",
			syncStatus: hiveintv1alpha1.SyncStatus{
				Name:               "test-syncset",
				ObservedGeneration: 1,
				Result:             hiveintv1alpha1.SuccessSyncSetResult,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			resourceToApply := testConfigMap("dest-namespace", "dest-name")
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				func(ss *hivev1.SyncSet) {
					ss.Generation = 2
					ss.Spec.Resources = []runtime.RawExtension{{
						Object: resourceToApply,
					}}
				},
			)
			clusterSync := clusterSyncBuilder(scheme).Build(testcs.WithSyncSetStatus(tc.syncStatus))
			syncLease := buildSyncLease(time.Now())
			rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), syncSet, clusterSync, syncLease)
			rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply)).Return(resource.CreatedApplyResult, nil)
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{{
				Name:               "test-syncset",
				ObservedGeneration: 2,
				Result:             hiveintv1alpha1.SuccessSyncSetResult,
			}}
			rt.expectUnchangedLeaseRenewTime = true
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_ErrorDeleting(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	scheme := newScheme()
	existingSyncStatus := hiveintv1alpha1.SyncStatus{
		Name:               "test-syncset",
		ObservedGeneration: 1,
		Result:             hiveintv1alpha1.SuccessSyncSetResult,
		ResourcesToDelete: []hiveintv1alpha1.SyncResourceReference{
			testConfigMapRef("dest-namespace", "dest-name"),
		},
	}
	clusterSync := clusterSyncBuilder(scheme).Build(testcs.WithSyncSetStatus(existingSyncStatus))
	lease := buildSyncLease(time.Now().Add(-1 * time.Hour))
	rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), clusterSync, lease)
	rt.mockResourceHelper.EXPECT().
		Delete("v1", "ConfigMap", "dest-namespace", "dest-name").
		Return(errors.New("error deleting resource"))
	rt.expectedFailedMessage = "SyncSet test-syncset is failing"
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{{
		Name: "test-syncset",
		ResourcesToDelete: []hiveintv1alpha1.SyncResourceReference{
			testConfigMapRef("dest-namespace", "dest-name"),
		},
		Result:         hiveintv1alpha1.FailureSyncSetResult,
		FailureMessage: "Failed to delete v1, Kind=ConfigMap dest-namespace/dest-name: error deleting resource",
	}}
	rt.expectUnchangedLeaseRenewTime = true
	rt.expectRequeue = true
	rt.run(t)
}

func TestReconcileClusterSync_DeleteErrorDoesNotBlockOtherDeletes(t *testing.T) {
	cases := []struct {
		name           string
		syncSetRemoved bool
	}{
		{
			name:           "removed syncset",
			syncSetRemoved: true,
		},
		{
			name:           "removed resource",
			syncSetRemoved: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			existingSyncStatus := hiveintv1alpha1.SyncStatus{
				Name:               "test-syncset",
				ObservedGeneration: 1,
				Result:             hiveintv1alpha1.SuccessSyncSetResult,
				ResourcesToDelete: []hiveintv1alpha1.SyncResourceReference{
					testConfigMapRef("dest-namespace", "failing-resource"),
					testConfigMapRef("dest-namespace", "successful-resource"),
				},
			}
			clusterSync := clusterSyncBuilder(scheme).Build(testcs.WithSyncSetStatus(existingSyncStatus))
			lease := buildSyncLease(time.Now().Add(-1 * time.Hour))
			existing := []runtime.Object{cdBuilder(scheme).Build(), clusterSync, lease}
			if !tc.syncSetRemoved {
				existing = append(existing,
					testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
						testsyncset.ForClusterDeployments(testCDName),
						func(ss *hivev1.SyncSet) {
							ss.Generation = 2
						},
					),
				)
			}
			rt := newReconcileTest(t, mockCtrl, scheme, existing...)
			rt.mockResourceHelper.EXPECT().
				Delete("v1", "ConfigMap", "dest-namespace", "failing-resource").
				Return(errors.New("error deleting resource"))
			rt.mockResourceHelper.EXPECT().
				Delete("v1", "ConfigMap", "dest-namespace", "successful-resource").
				Return(nil)
			rt.expectedFailedMessage = "SyncSet test-syncset is failing"
			expectedSyncSetStatus := hiveintv1alpha1.SyncStatus{
				Name: "test-syncset",
				ResourcesToDelete: []hiveintv1alpha1.SyncResourceReference{
					testConfigMapRef("dest-namespace", "failing-resource"),
				},
				Result:         hiveintv1alpha1.FailureSyncSetResult,
				FailureMessage: "Failed to delete v1, Kind=ConfigMap dest-namespace/failing-resource: error deleting resource",
			}
			if !tc.syncSetRemoved {
				expectedSyncSetStatus.ObservedGeneration = 2
			}
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{expectedSyncSetStatus}
			rt.expectUnchangedLeaseRenewTime = true
			rt.expectRequeue = true
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_DoNotApplyNotReadySyncSet(t *testing.T) {
	scheme := newScheme()
	cases := []struct {
		name           string
		syncSet        *hivev1.SyncSet
		failureMessage string
	}{
		{
			name: "latest generation not observed",
			syncSet: testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				func(ss *hivev1.SyncSet) {
					ss.Generation = 2
					ss.Spec.ResourceApplyMode = hivev1.SyncResourceApplyMode
					ss.Spec.Resources = []runtime.RawExtension{{
						Object: testConfigMap("dest-namespace", "dest-name"),
					}}
				},
			),
			failureMessage: "Current generation has not been observed yet",
		},
		{
			name: "invalid resource",
			syncSet: testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				func(ss *hivev1.SyncSet) {
					ss.Generation = 1
					ss.Spec.ResourceApplyMode = hivev1.SyncResourceApplyMode
					ss.Spec.Resources = []runtime.RawExtension{{
						Object: testConfigMap("dest-namespace", "dest-name"),
					}}
				},
			),
			failureMessage: "Invalid resource",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			existing := []runtime.Object{cdBuilder(scheme).Build(), tc.syncSet}
			rt := newReconcileTest(t, mockCtrl, scheme, existing...)
			rt.expectedFailedMessage = "SyncSet test-syncset is failing"
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{{
				Name:               "test-syncset",
				ObservedGeneration: 1,
				Result:             hiveintv1alpha1.FailureSyncSetResult,
				FailureMessage:     tc.failureMessage,
			}}
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_ApplyBehavior(t *testing.T) {
	cases := []struct {
		applyBehavior hivev1.SyncSetApplyBehavior
	}{
		{
			applyBehavior: hivev1.ApplySyncSetApplyBehavior,
		},
		{
			applyBehavior: hivev1.CreateOnlySyncSetApplyBehavior,
		},
		{
			applyBehavior: hivev1.CreateOrUpdateSyncSetApplyBehavior,
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.applyBehavior), func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scheme := newScheme()
			resourceToApply := testConfigMap("resource-namespace", "resource-name")
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				func(ss *hivev1.SyncSet) {
					ss.Generation = 1
					ss.Spec.ApplyBehavior = tc.applyBehavior
					ss.Spec.Resources = []runtime.RawExtension{{
						Object: resourceToApply,
					}}
					ss.Spec.Secrets = []hivev1.SecretMapping{
						testSecretMapping("test-secret", "secret-namespace", "secret-name"),
					}
					ss.Spec.Patches = []hivev1.SyncObjectPatch{{
						APIVersion: "patch-api/v1",
						Kind:       "PatchKind",
						Namespace:  "patch-namespace",
						Name:       "patch-name",
						PatchType:  "patch-type",
						Patch:      "test-patch",
					}}
				},
			)
			srcSecret := testsecret.FullBuilder(testNamespace, "test-secret", scheme).Build(
				testsecret.WithDataKeyValue("test-key", []byte("test-data")),
			)
			rt := newReconcileTest(t, mockCtrl, scheme, cdBuilder(scheme).Build(), syncSet, srcSecret)
			secretToApply := testsecret.BasicBuilder().GenericOptions(
				testgeneric.WithNamespace("secret-namespace"),
				testgeneric.WithName("secret-name"),
				testgeneric.WithTypeMeta(scheme),
			).Build(
				testsecret.WithDataKeyValue("test-key", []byte("test-data")),
			)
			switch tc.applyBehavior {
			case hivev1.ApplySyncSetApplyBehavior:
				rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply)).Return(resource.CreatedApplyResult, nil)
				rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(secretToApply)).Return(resource.CreatedApplyResult, nil)
			case hivev1.CreateOnlySyncSetApplyBehavior:
				rt.mockResourceHelper.EXPECT().Create(newApplyMatcher(resourceToApply)).Return(resource.CreatedApplyResult, nil)
				rt.mockResourceHelper.EXPECT().Create(newApplyMatcher(secretToApply)).Return(resource.CreatedApplyResult, nil)
			case hivev1.CreateOrUpdateSyncSetApplyBehavior:
				rt.mockResourceHelper.EXPECT().CreateOrUpdate(newApplyMatcher(resourceToApply)).Return(resource.CreatedApplyResult, nil)
				rt.mockResourceHelper.EXPECT().CreateOrUpdate(newApplyMatcher(secretToApply)).Return(resource.CreatedApplyResult, nil)
			}
			rt.mockResourceHelper.EXPECT().Patch(
				types.NamespacedName{Namespace: "patch-namespace", Name: "patch-name"},
				"PatchKind",
				"patch-api/v1",
				[]byte("test-patch"),
				"patch-type",
			).Return(nil)
			syncSetStatus := hiveintv1alpha1.SyncStatus{
				Name:               "test-syncset",
				ObservedGeneration: 1,
				Result:             hiveintv1alpha1.SuccessSyncSetResult,
			}
			rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, syncSetStatus)
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_IgnoreNotApplicableSyncSets(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	scheme := newScheme()
	syncSetResourceToApply := testConfigMap("dest-namespace", "resource-from-applicable-syncset")
	applicableSyncSet := testsyncset.FullBuilder(testNamespace, "applicable-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		func(ss *hivev1.SyncSet) {
			ss.Generation = 1
			ss.Spec.Resources = []runtime.RawExtension{{
				Object: syncSetResourceToApply,
			}}
		},
	)
	nonApplicableSyncSet := testsyncset.FullBuilder(testNamespace, "non-applicable-syncset", scheme).Build(
		testsyncset.ForClusterDeployments("other-cd"),
		func(ss *hivev1.SyncSet) {
			ss.Generation = 1
			ss.Spec.Resources = []runtime.RawExtension{{
				Object: testConfigMap("dest-namespace", "resource-from-non-applicable-syncset"),
			}}
		},
	)
	selectorSyncSetResourceToApply := testConfigMap("dest-namespace", "resource-from-applicable-selectorsyncset")
	applicableSelectorSyncSet := testselectorsyncset.FullBuilder("applicable-selectorsyncset", scheme).Build(
		testselectorsyncset.WithLabelSelector("test-label-key", "test-label-value"),
		func(ss *hivev1.SelectorSyncSet) {
			ss.Generation = 1
			ss.Spec.Resources = []runtime.RawExtension{{
				Object: selectorSyncSetResourceToApply,
			}}
		},
	)
	nonApplicableSelectorSyncSet := testselectorsyncset.FullBuilder("non-applicable-selectorsyncset", scheme).Build(
		testselectorsyncset.WithLabelSelector("test-label-key", "other-label-value"),
		func(ss *hivev1.SelectorSyncSet) {
			ss.Generation = 1
			ss.Spec.Resources = []runtime.RawExtension{{
				Object: testConfigMap("dest-namespace", "resource-from-non-applicable-selectorsyncset"),
			}}
		},
	)
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(testcd.WithLabel("test-label-key", "test-label-value")),
		applicableSyncSet,
		nonApplicableSyncSet,
		applicableSelectorSyncSet,
		nonApplicableSelectorSyncSet,
	)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(syncSetResourceToApply)).Return(resource.CreatedApplyResult, nil)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(selectorSyncSetResourceToApply)).Return(resource.CreatedApplyResult, nil)
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{{
		Name:               "applicable-syncset",
		ObservedGeneration: 1,
		Result:             hiveintv1alpha1.SuccessSyncSetResult,
	}}
	rt.expectedSelectorSyncSetStatuses = []hiveintv1alpha1.SyncStatus{{
		Name:               "applicable-selectorsyncset",
		ObservedGeneration: 1,
		Result:             hiveintv1alpha1.SuccessSyncSetResult,
	}}
	rt.run(t)
}

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)
	hiveintv1alpha1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)
	return scheme
}

func cdBuilder(scheme *runtime.Scheme) testcd.Builder {
	return testcd.FullBuilder(testNamespace, testCDName, scheme).Options(
		testcd.Installed(),
		testcd.WithCondition(hivev1.ClusterDeploymentCondition{
			Type:   hivev1.UnreachableCondition,
			Status: corev1.ConditionFalse,
		}),
	)
}

func clusterSyncBuilder(scheme *runtime.Scheme) testcs.Builder {
	return testcs.FullBuilder(testNamespace, testClusterSyncName, scheme)
}

func buildSyncLease(t time.Time) *hiveintv1alpha1.ClusterSyncLease {
	return &hiveintv1alpha1.ClusterSyncLease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testLeaseName,
		},
		Spec: hiveintv1alpha1.ClusterSyncLeaseSpec{
			RenewTime: metav1.NewMicroTime(t),
		},
	}
}

func testConfigMap(namespace, name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func testConfigMapRef(namespace, name string) hiveintv1alpha1.SyncResourceReference {
	return hiveintv1alpha1.SyncResourceReference{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Namespace:  namespace,
		Name:       name,
	}
}

func testSecretMapping(srcName, destNamespace, destName string) hivev1.SecretMapping {
	return hivev1.SecretMapping{
		SourceRef: hivev1.SecretReference{
			Name: srcName,
		},
		TargetRef: hivev1.SecretReference{
			Namespace: destNamespace,
			Name:      destName,
		},
	}
}

func testSecretRef(namespace, name string) hiveintv1alpha1.SyncResourceReference {
	return hiveintv1alpha1.SyncResourceReference{
		APIVersion: "v1",
		Kind:       "Secret",
		Namespace:  namespace,
		Name:       name,
	}
}

type applyMatcher struct {
	resource *unstructured.Unstructured
}

func newApplyMatcher(resource hivev1.MetaRuntimeObject) gomock.Matcher {
	resourceAsJSON, err := json.Marshal(resource)
	if err != nil {
		panic(errors.Wrap(err, "could not marshal resource to JSON"))
	}
	u := &unstructured.Unstructured{}
	if err := json.Unmarshal(resourceAsJSON, u); err != nil {
		panic(errors.Wrap(err, "could not unmarshal as unstructured"))
	}
	labels := u.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[constants.HiveManagedLabel] = "true"
	u.SetLabels(labels)
	return &applyMatcher{resource: u}
}

func (m *applyMatcher) Matches(x interface{}) bool {
	rawData, ok := x.([]byte)
	if !ok {
		return false
	}
	u := &unstructured.Unstructured{}
	if err := json.Unmarshal(rawData, u); err != nil {
		return false
	}
	return reflect.DeepEqual(u, m.resource)
}

func (m *applyMatcher) String() string {
	return fmt.Sprintf(
		"is %s %s in %s",
		m.resource.GetObjectKind().GroupVersionKind(),
		m.resource.GetName(),
		m.resource.GetNamespace(),
	)
}

func (m *applyMatcher) Got(got interface{}) string {
	switch t := got.(type) {
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func permute(x []interface{}, foo func([]interface{})) {
	switch l := len(x); l {
	case 0:
	case 1:
		foo(x)
	default:
		for i := 0; i < l; i++ {
			x[0], x[i] = x[i], x[0]
			permute(x[1:], func(y []interface{}) {
				foo(append(x[0:1], y...))
			})
			x[0], x[i] = x[i], x[0]
		}
	}
}
