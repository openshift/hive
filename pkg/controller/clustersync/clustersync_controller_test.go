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
	"k8s.io/utils/pointer"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
	"github.com/openshift/hive/pkg/resource"
	resourcemock "github.com/openshift/hive/pkg/resource/mock"
	hiveassert "github.com/openshift/hive/pkg/test/assert"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testcs "github.com/openshift/hive/pkg/test/clustersync"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	testsecret "github.com/openshift/hive/pkg/test/secret"
	testselectorsyncset "github.com/openshift/hive/pkg/test/selectorsyncset"
	teststatefulset "github.com/openshift/hive/pkg/test/statefulset"
	testsyncset "github.com/openshift/hive/pkg/test/syncset"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	testNamespace       = "test-namespace"
	testCDName          = "test-cluster-deployment"
	testCDUID           = "test-cluster-deployment-uid"
	testClusterSyncName = testCDName
	testClusterSyncUID  = "test-cluster-sync-uid"
	testLeaseName       = testCDName
)

var (
	timeInThePast = metav1.NewTime(time.Date(2020, 1, 2, 3, 4, 5, 0, time.Local))
)

type reconcileTest struct {
	logger                  log.FieldLogger
	c                       client.Client
	r                       *ReconcileClusterSync
	mockCtrl                *gomock.Controller
	mockResourceHelper      *resourcemock.MockHelper
	mockRemoteClientBuilder *remoteclientmock.MockBuilder
	expectedFailedMessage   string

	// A zero LastTransitionTime indicates that the time should be set to now.
	// A FirstSuccessTime that points to a zero time indicates that the time should be set to now.
	expectedSyncSetStatuses         []hiveintv1alpha1.SyncStatus
	expectedSelectorSyncSetStatuses []hiveintv1alpha1.SyncStatus

	expectUnchangedLeaseRenewTime bool
	expectRequeue                 bool
	expectNoWorkDone              bool
}

func newReconcileTest(t *testing.T, mockCtrl *gomock.Controller, scheme *runtime.Scheme, existing ...runtime.Object) *reconcileTest {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	c := testfake.NewFakeClientBuilder().WithRuntimeObjects(existing...).Build()

	mockResourceHelper := resourcemock.NewMockHelper(mockCtrl)
	mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)

	r := &ReconcileClusterSync{
		ordinalID:       0,
		Client:          c,
		logger:          logger,
		reapplyInterval: defaultReapplyInterval,
		resourceHelperBuilder: func(
			cd *hivev1.ClusterDeployment,
			remoteClusterAPIClientBuilderFunc func(cd *hivev1.ClusterDeployment) remoteclient.Builder,
			_ log.FieldLogger,
		) (resource.Helper, error) {
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
	result, err := rt.r.Reconcile(context.TODO(), reconcileRequest)
	require.NoError(t, err, "unexpected error from Reconcile")
	endTime := time.Now()
	startTime = startTime.Truncate(time.Second)
	endTime = endTime.Add(time.Second).Truncate(time.Second)

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
		// Due to the vagaries of floating point math, these numbers are still "right" when within 0.01s.
		assert.InDelta(t, result.RequeueAfter.Seconds(), minRequeueAfter, 0.01*float64(time.Second), "Minimum RequeueAfter out of range")
		assert.InDelta(t, result.RequeueAfter.Seconds(), maxRequeueAfter, 0.01*float64(time.Second), "Maximum RequeueAfter out of range")
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

	expectedOwnerReferenceFromClusterSync := metav1.OwnerReference{
		APIVersion:         hivev1.SchemeGroupVersion.String(),
		Kind:               "ClusterDeployment",
		Name:               testCDName,
		UID:                testCDUID,
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}
	assert.Contains(t, clusterSync.OwnerReferences, expectedOwnerReferenceFromClusterSync, "expected owner reference from ClusterSync to ClusterDeployment")

	expectedOwnerReferenceFromLease := metav1.OwnerReference{
		APIVersion:         hiveintv1alpha1.SchemeGroupVersion.String(),
		Kind:               "ClusterSync",
		Name:               testClusterSyncName,
		UID:                testClusterSyncUID,
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}
	assert.Contains(t, lease.OwnerReferences, expectedOwnerReferenceFromLease, "expected owner reference from ClusterSyncLease to ClusterSync")

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

	areSyncStatusesEqual(t, "syncset", rt.expectedSyncSetStatuses, clusterSync.Status.SyncSets, startTime, endTime)
	areSyncStatusesEqual(t, "selectorsyncset", rt.expectedSelectorSyncSetStatuses, clusterSync.Status.SelectorSyncSets, startTime, endTime)
}

func areSyncStatusesEqual(t *testing.T, syncSetType string, expectedStatuses, actualStatuses []hiveintv1alpha1.SyncStatus, startTime, endTime time.Time) {
	if !assert.Equalf(t, len(expectedStatuses), len(actualStatuses), "unexpected number of %s statuses", syncSetType) {
		return
	}
	for i, expectedStatus := range expectedStatuses {
		if expectedStatus.LastTransitionTime.IsZero() {
			actual := actualStatuses[i].LastTransitionTime
			hiveassert.BetweenTimes(t, actual.Time, startTime, endTime, "expected %s status %d to have LastTransitionTime of now", syncSetType, i)
			expectedStatuses[i].LastTransitionTime = actual
		}
		if expectedStatus.FirstSuccessTime != nil && expectedStatus.FirstSuccessTime.IsZero() {
			if actualStatuses[i].FirstSuccessTime != nil {
				actual := actualStatuses[i].FirstSuccessTime
				hiveassert.BetweenTimes(t, actual.Time, startTime, endTime, "expected %s status %d to have FirstSuccessTime of now", syncSetType, i)
				*expectedStatuses[i].FirstSuccessTime = *actualStatuses[i].FirstSuccessTime
			}
		}
	}
	assert.Equalf(t, expectedStatuses, actualStatuses, "unexpected %s statuses", syncSetType)
}

func TestReconcileClusterSync_NewClusterDeployment(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
	)
	reconcileRequest := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      testCDName,
		},
	}
	result, err := rt.r.Reconcile(context.TODO(), reconcileRequest)
	require.NoError(t, err, "unexpected error from Reconcile")
	assert.Equal(t, result, reconcile.Result{Requeue: true}, "unexpected result from reconcile")
	err = rt.c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testLeaseName}, &hiveintv1alpha1.ClusterSyncLease{})
	assert.True(t, apierrors.IsNotFound(err), "expected no lease")
	err = rt.c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testClusterSyncName}, &hiveintv1alpha1.ClusterSync{})
	assert.Nil(t, err, "expected there to be a ClusterSync")
}

func TestReconcileClusterSync_NoWorkToDo(t *testing.T) {
	scheme := scheme.GetScheme()
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
			cd:   cdBuilder(scheme).GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer("test-finalizer")).Build(),
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
		{
			name: "uninstalled and unreachable unknown",
			cd: testcd.FullBuilder(testNamespace, testCDName, scheme).
				GenericOptions(
					testgeneric.WithUID(testCDUID),
				).
				Options(
					testcd.WithCondition(hivev1.ClusterDeploymentCondition{
						Type:   hivev1.UnreachableCondition,
						Status: corev1.ConditionUnknown,
					})).Build(),
		},
		{
			name: "syncset pause",
			cd:   cdBuilder(scheme).GenericOptions(testgeneric.WithAnnotation(constants.SyncsetPauseAnnotation, "true")).Build(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			var existing []runtime.Object
			if tc.cd != nil {
				existing = append(existing,
					tc.cd,
					teststatefulset.FullBuilder("hive", stsName, scheme).Build(
						teststatefulset.WithCurrentReplicas(3),
						teststatefulset.WithReplicas(3),
					))
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
			scheme := scheme.GetScheme()
			resourceToApply := testConfigMap("dest-namespace", "dest-name")
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				testsyncset.WithGeneration(1),
				testsyncset.WithApplyMode(tc.applyMode),
				testsyncset.WithResources(resourceToApply),
			)
			rt := newReconcileTest(t, mockCtrl, scheme,
				cdBuilder(scheme).Build(),
				clusterSyncBuilder(scheme).Build(),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
				syncSet)
			rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply)).Return(resource.CreatedApplyResult, nil)
			expectedSyncStatusBuilder := newSyncStatusBuilder("test-syncset")
			if tc.includeResourcesToDelete {
				expectedSyncStatusBuilder = expectedSyncStatusBuilder.Options(
					withResourcesToDelete(testConfigMapRef("dest-namespace", "dest-name")),
				)
			}
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{expectedSyncStatusBuilder.Build()}
			rt.run(t)
		})
	}
}

func TestGetAndCheckClustersyncStatefulSet(t *testing.T) {
	scheme := scheme.GetScheme()

	cases := []struct {
		name                string
		existingObjects     []runtime.Object
		expectedStatefulSet *appsv1.StatefulSet
		expectedErr         bool
	}{
		{
			name:            "missing sts",
			expectedErr:     true,
			existingObjects: []runtime.Object{},
		},
		{
			name:        "sts replicas not set",
			expectedErr: true,
			existingObjects: []runtime.Object{
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(),
			},
		},
		{
			name:        "sts replicas not same as currentReplicas",
			expectedErr: true,
			existingObjects: []runtime.Object{
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(1),
					teststatefulset.WithReplicas(3),
				),
			},
		},
		{
			name:        "correct sts",
			expectedErr: false,
			expectedStatefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(1),
				teststatefulset.WithReplicas(1)),
			existingObjects: []runtime.Object{
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(1),
					teststatefulset.WithReplicas(1),
				),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mockCtrl := gomock.NewController(t)
			rt := newReconcileTest(t, mockCtrl, scheme, tc.existingObjects...)

			// Act
			actualStatefulset, actualErr := rt.r.getAndCheckClusterSyncStatefulSet(rt.logger)

			// Assert
			assert.Equal(t, tc.expectedStatefulSet, actualStatefulset)
			if tc.expectedErr {
				assert.Error(t, actualErr, "Didn't error when was expected to.")
			} else {
				assert.NoError(t, actualErr, "Got error when not expected.")
			}
		})
	}
}

func TestIsSyncAssignedToMe(t *testing.T) {
	scheme := scheme.GetScheme()

	cases := []struct {
		name              string
		ordinalID         int64
		expectedOrdinalID int64
		statefulSet       *appsv1.StatefulSet
		clusterDeployment *hivev1.ClusterDeployment
		expectedErr       bool
	}{
		{
			name:      "assigned to me - ordinal 0",
			ordinalID: 0,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(3),
				teststatefulset.WithReplicas(3),
			),
			clusterDeployment: testcd.FullBuilder(testNamespace, testCDName, scheme).Build(
				testcd.Generic(testgeneric.WithUID("1138528c-c36e-11e9-a1a7-42010a800195")),
			),
			expectedOrdinalID: 0,
			expectedErr:       false,
		},
		{
			name:              "not assigned to me - ordinal 0",
			ordinalID:         0,
			expectedOrdinalID: 1,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(3),
				teststatefulset.WithReplicas(3),
			),
			clusterDeployment: testcd.FullBuilder(testNamespace, testCDName, scheme).Build(
				testcd.Generic(testgeneric.WithUID("1138528c-c36e-11e9-a1a7-42010a800196")),
			),
			expectedErr: false,
		},
		{
			name:              "assigned to me - ordinal 1",
			ordinalID:         1,
			expectedOrdinalID: 1,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(3),
				teststatefulset.WithReplicas(3),
			),
			clusterDeployment: testcd.FullBuilder(testNamespace, testCDName, scheme).Build(
				testcd.Generic(testgeneric.WithUID("1138528c-c36e-11e9-a1a7-42010a800196")),
			),
			expectedErr: false,
		},
		{
			name:              "not assigned to me - ordinal 1",
			ordinalID:         1,
			expectedOrdinalID: 2,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(3),
				teststatefulset.WithReplicas(3),
			),
			clusterDeployment: testcd.FullBuilder(testNamespace, testCDName, scheme).Build(
				testcd.Generic(testgeneric.WithUID("1138528c-c36e-11e9-a1a7-42010a800197")),
			),
			expectedErr: false,
		},
		{
			name:              "assigned to me - ordinal 2",
			ordinalID:         2,
			expectedOrdinalID: 2,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(3),
				teststatefulset.WithReplicas(3),
			),
			clusterDeployment: testcd.FullBuilder(testNamespace, testCDName, scheme).Build(
				testcd.Generic(testgeneric.WithUID("1138528c-c36e-11e9-a1a7-42010a800197")),
			),
			expectedErr: false,
		},
		{
			name:              "not assigned to me - ordinal 2",
			ordinalID:         2,
			expectedOrdinalID: 0,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(3),
				teststatefulset.WithReplicas(3),
			),
			clusterDeployment: testcd.FullBuilder(testNamespace, testCDName, scheme).Build(
				testcd.Generic(testgeneric.WithUID("1138528c-c36e-11e9-a1a7-42010a800198")),
			),
			expectedErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mockCtrl := gomock.NewController(t)
			rt := newReconcileTest(t, mockCtrl, scheme)
			rt.r.ordinalID = tc.ordinalID

			// Act
			actualAssignedToMe, actualOrdinalID, actualErr := rt.r.isSyncAssignedToMe(tc.statefulSet, tc.clusterDeployment, rt.logger)

			// Assert
			assert.Equal(t, tc.ordinalID == tc.expectedOrdinalID, actualAssignedToMe, "Unexpected assigned-to-me")
			assert.Equal(t, tc.expectedOrdinalID, actualOrdinalID)
			if tc.expectedErr {
				assert.Error(t, actualErr, "Didn't error when was expected to.")
			} else {
				assert.NoError(t, actualErr, "Got error when not expected.")
			}
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
			scheme := scheme.GetScheme()
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				testsyncset.WithGeneration(1),
				testsyncset.WithApplyMode(tc.applyMode),
				testsyncset.WithSecrets(
					testSecretMapping("test-secret", "dest-namespace", "dest-name"),
				),
			)
			srcSecret := testsecret.FullBuilder(testNamespace, "test-secret", scheme).Build(
				testsecret.WithDataKeyValue("test-key", []byte("test-data")),
			)
			rt := newReconcileTest(t, mockCtrl, scheme,
				cdBuilder(scheme).Build(),
				clusterSyncBuilder(scheme).Build(),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
				syncSet,
				srcSecret)
			secretToApply := testsecret.BasicBuilder().GenericOptions(
				testgeneric.WithNamespace("dest-namespace"),
				testgeneric.WithName("dest-name"),
				testgeneric.WithTypeMeta(scheme),
			).Build(
				testsecret.WithDataKeyValue("test-key", []byte("test-data")),
			)
			rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(secretToApply)).Return(resource.CreatedApplyResult, nil)
			expectedSyncStatusBuilder := newSyncStatusBuilder("test-syncset")
			if tc.includeResourcesToDelete {
				expectedSyncStatusBuilder = expectedSyncStatusBuilder.Options(
					withResourcesToDelete(testSecretRef("dest-namespace", "dest-name")),
				)
			}
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{expectedSyncStatusBuilder.Build()}
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
			scheme := scheme.GetScheme()
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				testsyncset.WithGeneration(1),
				testsyncset.WithApplyMode(tc.applyMode),
				testsyncset.WithPatches(hivev1.SyncObjectPatch{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Namespace:  "dest-namespace",
					Name:       "dest-name",
					PatchType:  "patch-type",
					Patch:      "test-patch",
				}),
			)
			rt := newReconcileTest(t, mockCtrl, scheme,
				cdBuilder(scheme).Build(),
				clusterSyncBuilder(scheme).Build(),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
				syncSet)
			rt.mockResourceHelper.EXPECT().Patch(
				types.NamespacedName{Namespace: "dest-namespace", Name: "dest-name"},
				"ConfigMap",
				"v1",
				[]byte("test-patch"),
				"patch-type",
			).Return(nil)
			rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, buildSyncStatus("test-syncset"))
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
			scheme := scheme.GetScheme()
			resourceToApply := testConfigMap("resource-namespace", "resource-name")
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				testsyncset.WithGeneration(1),
				testsyncset.WithApplyMode(tc.applyMode),
				testsyncset.WithResources(resourceToApply),
				testsyncset.WithSecrets(
					testSecretMapping("test-secret", "secret-namespace", "secret-name"),
				),
				testsyncset.WithPatches(hivev1.SyncObjectPatch{
					APIVersion: "patch-api/v1",
					Kind:       "PatchKind",
					Namespace:  "patch-namespace",
					Name:       "patch-name",
					PatchType:  "patch-type",
					Patch:      "test-patch",
				}),
			)
			srcSecret := testsecret.FullBuilder(testNamespace, "test-secret", scheme).Build(
				testsecret.WithDataKeyValue("test-key", []byte("test-data")),
			)
			rt := newReconcileTest(t, mockCtrl, scheme,
				cdBuilder(scheme).Build(),
				clusterSyncBuilder(scheme).Build(),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
				syncSet,
				srcSecret)
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
			expectedSyncStatusBuilder := newSyncStatusBuilder("test-syncset")
			if tc.includeResourcesToDelete {
				expectedSyncStatusBuilder = expectedSyncStatusBuilder.Options(
					withResourcesToDelete(
						testConfigMapRef("resource-namespace", "resource-name"),
						testSecretRef("secret-namespace", "secret-name"),
					),
				)
			}
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{expectedSyncStatusBuilder.Build()}
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_Reapply(t *testing.T) {
	cases := []struct {
		name        string
		noSyncLease bool
		renewTime   time.Time
		expectApply bool
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
			scheme := scheme.GetScheme()
			resourceToApply := testConfigMap("dest-namespace", "dest-name")
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				testsyncset.WithGeneration(1),
				testsyncset.WithResources(resourceToApply),
			)
			existing := []runtime.Object{
				cdBuilder(scheme).Build(),
				clusterSyncBuilder(scheme).Build(
					testcs.WithSyncSetStatus(buildSyncStatus("test-syncset",
						withTransitionInThePast(),
						withFirstSuccessTimeInThePast(),
					),
					)),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
				syncSet,
			}
			if !tc.noSyncLease {
				existing = append(existing, buildSyncLease(tc.renewTime))
			}
			rt := newReconcileTest(t, mockCtrl, scheme, existing...)
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{
				buildSyncStatus("test-syncset", withTransitionInThePast(), withFirstSuccessTimeInThePast()),
			}
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
	scheme := scheme.GetScheme()
	existingResource := testConfigMap("dest-namespace", "dest-name")
	existingSyncSet := testsyncset.FullBuilder(testNamespace, "existing-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithResources(existingResource),
	)
	newResource := testConfigMap("other-namespace", "other-name")
	newSyncSet := testsyncset.FullBuilder(testNamespace, "new-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithResources(newResource),
	)
	clusterSync := clusterSyncBuilder(scheme).Build(testcs.WithSyncSetStatus(
		buildSyncStatus("existing-syncset", withTransitionInThePast(), withFirstSuccessTimeInThePast()),
	))
	lease := buildSyncLease(time.Now().Add(-1 * time.Hour))
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		existingSyncSet,
		newSyncSet,
		clusterSync,
		lease)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(newResource)).Return(resource.CreatedApplyResult, nil)
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{
		buildSyncStatus("existing-syncset", withTransitionInThePast(), withFirstSuccessTimeInThePast()),
		buildSyncStatus("new-syncset"),
	}
	rt.expectUnchangedLeaseRenewTime = true
	rt.run(t)
}

func TestReconcileClusterSync_SyncSetRenamed(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()

	// clustersync exists for old syncset
	clusterSync := clusterSyncBuilder(scheme).Build(
		testcs.WithSyncSetStatus(
			newSyncStatusBuilder("test-syncset").Options(
				withTransitionInThePast(),
				withFirstSuccessTimeInThePast(),
				withResourcesToDelete(testConfigMapRef("dest-namespace", "dest-name")),
			).Build(),
		),
	)

	// syncset name does not match the existing clustersync
	syncSet := testsyncset.FullBuilder(testNamespace, "renamed-test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithApplyMode(hivev1.SyncResourceApplyMode),
		testsyncset.WithGeneration(2),
		testsyncset.WithResources(testConfigMap("dest-namespace", "dest-name")),
	)

	lease := buildSyncLease(time.Now().Add(-1 * time.Hour))
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		syncSet,
		clusterSync,
		lease)

	// Configmap managed by original syncset is deleted
	deleteCall := rt.mockResourceHelper.EXPECT().
		Delete("v1", "ConfigMap", "dest-namespace", "dest-name").
		Return(nil)

	// Configmap managed by renamed syncset is applied
	rt.mockResourceHelper.EXPECT().
		Apply(newApplyMatcher(testConfigMap("dest-namespace", "dest-name"))).After(deleteCall)

	rt.expectUnchangedLeaseRenewTime = true

	// Expected syncsetstatus contains syncstatus for renamed syncset
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("renamed-test-syncset",
		withResourcesToDelete(testConfigMapRef("dest-namespace", "dest-name")),
		withObservedGeneration(2),
	)}

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
			scheme := scheme.GetScheme()
			existingSyncStatusBuilder := newSyncStatusBuilder("test-syncset").Options(
				withTransitionInThePast(),
				withFirstSuccessTimeInThePast(),
			)
			if tc.includeResourcesToDelete {
				existingSyncStatusBuilder = existingSyncStatusBuilder.Options(
					withResourcesToDelete(testConfigMapRef("dest-namespace", "dest-name")),
				)
			}
			clusterSync := clusterSyncBuilder(scheme).Build(testcs.WithSyncSetStatus(existingSyncStatusBuilder.Build()))
			lease := buildSyncLease(time.Now().Add(-1 * time.Hour))
			rt := newReconcileTest(t, mockCtrl, scheme,
				cdBuilder(scheme).Build(),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
				clusterSync,
				lease)
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
		resourceApplyMode        hivev1.SyncSetResourceApplyMode
	}{
		{
			name:                     "upsert",
			includeResourcesToDelete: false,
			expectDelete:             false,
			resourceApplyMode:        hivev1.UpsertResourceApplyMode,
		},
		{
			name:                     "sync",
			includeResourcesToDelete: true,
			expectDelete:             true,
			resourceApplyMode:        hivev1.SyncResourceApplyMode,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			scheme := scheme.GetScheme()
			resourceToApply := testConfigMap("dest-namespace", "retained-resource")
			resourceToApply2 := testConfigMap("another-namespace", "another-resource")
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				testsyncset.WithGeneration(2),
				testsyncset.WithResources(resourceToApply),
				testsyncset.WithApplyMode(tc.resourceApplyMode),
			)
			syncSet2 := testsyncset.FullBuilder(testNamespace, "test-syncset2", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				testsyncset.WithGeneration(2),
				testsyncset.WithResources(resourceToApply2),
				testsyncset.WithApplyMode(tc.resourceApplyMode),
			)
			existingSyncStatusBuilder := newSyncStatusBuilder("test-syncset").Options(
				withTransitionInThePast(),
				withFirstSuccessTimeInThePast(),
			)
			existingSyncStatusBuilder2 := newSyncStatusBuilder("test-syncset2").Options(
				withTransitionInThePast(),
				withFirstSuccessTimeInThePast(),
			)
			if tc.includeResourcesToDelete {
				existingSyncStatusBuilder = existingSyncStatusBuilder.Options(withResourcesToDelete(
					testConfigMapRef("dest-namespace", "deleted-resource"),
					testConfigMapRef("dest-namespace", "retained-resource"),
				))
				existingSyncStatusBuilder2 = existingSyncStatusBuilder2.Options(withResourcesToDelete(
					testConfigMapRef("another-namespace", "another-resource"),
				))
			}
			clusterSync := clusterSyncBuilder(scheme).Build(
				testcs.WithSyncSetStatus(existingSyncStatusBuilder.Build()),
				testcs.WithSyncSetStatus(existingSyncStatusBuilder2.Build()),
			)
			lease := buildSyncLease(time.Now().Add(-1 * time.Hour))
			rt := newReconcileTest(t, mockCtrl, scheme,
				cdBuilder(scheme).Build(),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
				syncSet,
				syncSet2,
				clusterSync,
				lease)
			rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply)).Return(resource.CreatedApplyResult, nil)
			rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply2)).Return(resource.CreatedApplyResult, nil)
			if tc.expectDelete {
				rt.mockResourceHelper.EXPECT().
					Delete("v1", "ConfigMap", "dest-namespace", "deleted-resource").
					Return(nil)
			}
			expectedSyncStatusBuilder := newSyncStatusBuilder("test-syncset").Options(
				withObservedGeneration(2),
				withFirstSuccessTimeInThePast(),
			)
			expectedSyncStatusBuilder2 := newSyncStatusBuilder("test-syncset2").Options(
				withObservedGeneration(2),
				withFirstSuccessTimeInThePast(),
			)
			if tc.includeResourcesToDelete {
				expectedSyncStatusBuilder = expectedSyncStatusBuilder.Options(withResourcesToDelete(
					testConfigMapRef("dest-namespace", "retained-resource"),
				))
				expectedSyncStatusBuilder2 = expectedSyncStatusBuilder2.Options(withResourcesToDelete(
					testConfigMapRef("another-namespace", "another-resource"),
				))
			}
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{
				expectedSyncStatusBuilder.Build(),
				expectedSyncStatusBuilder2.Build(),
			}
			rt.expectUnchangedLeaseRenewTime = true
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_ErrorApplyingResource(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	resourceToApply := testConfigMap("dest-namespace", "dest-name")
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithResources(resourceToApply),
	)
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		clusterSyncBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		syncSet)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply)).
		Return(resource.ApplyResult(""), errors.New("test apply error"))
	rt.expectedFailedMessage = "SyncSet test-syncset is failing"
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-syncset",
		withFailureResult("failed to apply resource 0: test apply error"),
		withNoFirstSuccessTime(),
	)}
	rt.expectRequeue = true
	rt.run(t)
}

func TestReconcileClusterSync_ErrorDecodingResource(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
	)
	syncSet.Spec.Resources = []runtime.RawExtension{{Raw: []byte("{}")}}
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		clusterSyncBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		syncSet)
	rt.expectedFailedMessage = "SyncSet test-syncset is failing"
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-syncset",
		withFailureResult("failed to decode resource 0: error unmarshaling JSON: while decoding JSON: Object 'Kind' is missing in '{}'"),
		withNoFirstSuccessTime(),
	)}
	rt.run(t)
}

func TestReconcileClusterSync_ErrorApplyingSecret(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithSecrets(
			testSecretMapping("test-secret", "dest-namespace", "dest-name"),
		),
	)
	srcSecret := testsecret.FullBuilder(testNamespace, "test-secret", scheme).Build(
		testsecret.WithDataKeyValue("test-key", []byte("test-data")),
	)
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		clusterSyncBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		syncSet,
		srcSecret)
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
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-syncset",
		withFailureResult("failed to apply secret 0: test apply error"),
		withNoFirstSuccessTime(),
	)}
	rt.expectRequeue = true
	rt.run(t)
}

func TestReconcileClusterSync_ErrorApplyingPatch(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithPatches(hivev1.SyncObjectPatch{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Namespace:  "dest-namespace",
			Name:       "dest-name",
			PatchType:  "patch-type",
			Patch:      "test-patch",
		}),
	)
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		clusterSyncBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		syncSet)
	rt.mockResourceHelper.EXPECT().Patch(
		types.NamespacedName{Namespace: "dest-namespace", Name: "dest-name"},
		"ConfigMap",
		"v1",
		[]byte("test-patch"),
		"patch-type",
	).Return(errors.New("test patch error"))
	rt.expectedFailedMessage = "SyncSet test-syncset is failing"
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-syncset",
		withFailureResult("failed to apply patch 0: test patch error"),
		withNoFirstSuccessTime(),
	)}
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
			failureMessage:      "failed to apply resource 0: test apply error",
		},
		{
			name:                "resource 1 fails",
			successfulResources: 1,
			failureMessage:      "failed to apply resource 1: test apply error",
		},
		{
			name:                "resource 2 fails",
			successfulResources: 2,
			failureMessage:      "failed to apply resource 2: test apply error",
		},
		{
			name:                "secret 0 fails",
			successfulResources: 3,
			successfulSecrets:   0,
			failureMessage:      "failed to apply secret 0: test apply error",
		},
		{
			name:                "secret 1 fails",
			successfulResources: 3,
			successfulSecrets:   1,
			failureMessage:      "failed to apply secret 1: test apply error",
		},
		{
			name:                "secret 2 fails",
			successfulResources: 3,
			successfulSecrets:   2,
			failureMessage:      "failed to apply secret 2: test apply error",
		},
		{
			name:                "patch 0 fails",
			successfulResources: 3,
			successfulSecrets:   3,
			successfulPatches:   0,
			failureMessage:      "failed to apply patch 0: test patch error",
		},
		{
			name:                "patch 1 fails",
			successfulResources: 3,
			successfulSecrets:   3,
			successfulPatches:   1,
			failureMessage:      "failed to apply patch 1: test patch error",
		},
		{
			name:                "patch 2 fails",
			successfulResources: 3,
			successfulSecrets:   3,
			successfulPatches:   2,
			failureMessage:      "failed to apply patch 2: test patch error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			scheme := scheme.GetScheme()
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
				testsyncset.WithGeneration(1),
				testsyncset.WithResources(resourcesToApply...),
				testsyncset.WithSecrets(
					testSecretMapping("test-secret-0", "secret-namespace-0", "secret-name-0"),
					testSecretMapping("test-secret-1", "secret-namespace-1", "secret-name-1"),
					testSecretMapping("test-secret-2", "secret-namespace-2", "secret-name-2"),
				),
				testsyncset.WithPatches(patchesToApply...),
			)
			existing := []runtime.Object{
				cdBuilder(scheme).Build(),
				clusterSyncBuilder(scheme).Build(),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
				syncSet}
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
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-syncset",
				withFailureResult(tc.failureMessage),
				withNoFirstSuccessTime(),
			)}
			rt.expectRequeue = true
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_ResourcesToDeleteAreOrdered(t *testing.T) {
	scheme := scheme.GetScheme()
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
				syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
					testsyncset.ForClusterDeployments(testCDName),
					testsyncset.WithGeneration(2),
					testsyncset.WithApplyMode(hivev1.SyncResourceApplyMode),
					testsyncset.WithResources(resourcesToApply...),
					testsyncset.WithSecrets(secretMappings...),
				)
				clusterSync := clusterSyncBuilder(scheme).Build(
					testcs.WithSyncSetStatus(buildSyncStatus("test-syncset",
						withResourcesToDelete(
							testConfigMapRef("namespace-A", "resource-failing-to-delete-A"),
							testConfigMapRef("namespace-A", "resource-failing-to-delete-B"),
							testConfigMapRef("namespace-B", "resource-failing-to-delete-A"),
						),
						withTransitionInThePast(),
						withFirstSuccessTimeInThePast(),
					)),
				)
				rt := newReconcileTest(t, mockCtrl, scheme,
					cdBuilder(scheme).Build(),
					teststatefulset.FullBuilder("hive", stsName, scheme).Build(
						teststatefulset.WithCurrentReplicas(3),
						teststatefulset.WithReplicas(3),
					),
					clusterSync,
					syncSet,
					srcSecret)
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
				rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-syncset",
					withObservedGeneration(2),
					withFailureResult("[failed to delete v1, Kind=ConfigMap namespace-A/resource-failing-to-delete-A: error deleting resource, failed to delete v1, Kind=ConfigMap namespace-A/resource-failing-to-delete-B: error deleting resource, failed to delete v1, Kind=ConfigMap namespace-B/resource-failing-to-delete-A: error deleting resource]"),
					withFirstSuccessTimeInThePast(),
					withResourcesToDelete(
						hiveintv1alpha1.SyncResourceReference{
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
						hiveintv1alpha1.SyncResourceReference{
							APIVersion: "v1",
							Kind:       "Service",
							Namespace:  "namespace-A",
							Name:       "name-A",
						},
					),
				)}
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
			scheme := scheme.GetScheme()
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
					testsyncset.WithGeneration(1),
					testsyncset.WithResources(resourcesToApply[i]),
				)
			}
			existing := []runtime.Object{
				cdBuilder(scheme).Build(),
				clusterSyncBuilder(scheme).Build(),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
			}
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
				expectedSyncSetStatusBuilder := newSyncStatusBuilder(s.Name)
				if i == tc.failingSyncSet {
					expectedSyncSetStatusBuilder = expectedSyncSetStatusBuilder.Options(
						withFailureResult("failed to apply resource 0: test apply error"),
						withNoFirstSuccessTime(),
					)
				}
				rt.expectedSyncSetStatuses = append(rt.expectedSyncSetStatuses, expectedSyncSetStatusBuilder.Build())
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
			scheme := scheme.GetScheme()
			syncSets := make([]runtime.Object, tc.failingSyncSets)
			for i := range syncSets {
				syncSets[i] = testsyncset.FullBuilder(testNamespace, fmt.Sprintf("test-syncset-%d", i), scheme).Build(
					testsyncset.ForClusterDeployments(testCDName),
					testsyncset.WithGeneration(1),
					testsyncset.WithResources(
						testConfigMap(fmt.Sprintf("syncset-namespace-%d", i), fmt.Sprintf("syncset-name-%d", i)),
					),
				)
			}
			selectorSyncSets := make([]runtime.Object, tc.failingSelectorSyncSets)
			for i := range selectorSyncSets {
				selectorSyncSets[i] = testselectorsyncset.FullBuilder(fmt.Sprintf("test-selectorsyncset-%d", i), scheme).Build(
					testselectorsyncset.WithLabelSelector("test-label-key", "test-label-value"),
					testselectorsyncset.WithGeneration(1),
					testselectorsyncset.WithResources(
						testConfigMap(fmt.Sprintf("selectorsyncset-namespace-%d", i), fmt.Sprintf("selectorsyncset-name-%d", i)),
					),
				)
			}
			existing := []runtime.Object{
				cdBuilder(scheme).Build(testcd.WithLabel("test-label-key", "test-label-value")),
				clusterSyncBuilder(scheme).Build(),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
			}
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
					rt.expectedSyncSetStatuses[i] = buildSyncStatus(fmt.Sprintf("test-syncset-%d", i),
						withFailureResult("failed to apply resource 0: test apply error"),
						withNoFirstSuccessTime(),
					)
				}
			}
			if tc.failingSelectorSyncSets > 0 {
				rt.expectedSelectorSyncSetStatuses = make([]hiveintv1alpha1.SyncStatus, tc.failingSelectorSyncSets)
				for i := range rt.expectedSelectorSyncSetStatuses {
					rt.expectedSelectorSyncSetStatuses[i] = buildSyncStatus(fmt.Sprintf("test-selectorsyncset-%d", i),
						withFailureResult("failed to apply resource 0: test apply error"),
						withNoFirstSuccessTime(),
					)
				}
			}
			rt.expectRequeue = true
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_PartialApply(t *testing.T) {
	cases := []struct {
		name               string
		existingSyncStatus hiveintv1alpha1.SyncStatus
		expectedSyncStatus hiveintv1alpha1.SyncStatus
	}{
		{
			name: "last apply failed",
			existingSyncStatus: buildSyncStatus("test-syncset",
				withObservedGeneration(2),
				withFailureResult("existing failure"),
				withTransitionInThePast(),
			),
			expectedSyncStatus: buildSyncStatus("test-syncset",
				withObservedGeneration(2),
			),
		},
		{
			name: "syncset generation changed",
			existingSyncStatus: buildSyncStatus("test-syncset",
				withObservedGeneration(1),
				withTransitionInThePast(),
				withFirstSuccessTimeInThePast(),
			),
			expectedSyncStatus: buildSyncStatus("test-syncset",
				withObservedGeneration(2),
				withFirstSuccessTimeInThePast(),
			),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			scheme := scheme.GetScheme()
			resourceToApply := testConfigMap("dest-namespace", "dest-name")
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				testsyncset.WithGeneration(2),
				testsyncset.WithResources(resourceToApply),
			)
			clusterSync := clusterSyncBuilder(scheme).Build(testcs.WithSyncSetStatus(tc.existingSyncStatus))
			syncLease := buildSyncLease(time.Now())
			rt := newReconcileTest(t, mockCtrl, scheme,
				cdBuilder(scheme).Build(),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				), syncSet,
				clusterSync,
				syncLease)
			rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply)).Return(resource.CreatedApplyResult, nil)
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{tc.expectedSyncStatus}
			rt.expectUnchangedLeaseRenewTime = true
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_ErrorDeleting(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	existingSyncStatus := buildSyncStatus("test-syncset",
		withResourcesToDelete(testConfigMapRef("dest-namespace", "dest-name")),
		withTransitionInThePast(),
		withFirstSuccessTimeInThePast(),
	)
	clusterSync := clusterSyncBuilder(scheme).Build(testcs.WithSyncSetStatus(existingSyncStatus))
	lease := buildSyncLease(time.Now().Add(-1 * time.Hour))
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		clusterSync,
		lease)
	rt.mockResourceHelper.EXPECT().
		Delete("v1", "ConfigMap", "dest-namespace", "dest-name").
		Return(errors.New("error deleting resource"))
	rt.expectedFailedMessage = "SyncSet test-syncset is failing"
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-syncset",
		withObservedGeneration(0),
		withFailureResult("failed to delete v1, Kind=ConfigMap dest-namespace/dest-name: error deleting resource"),
		withResourcesToDelete(testConfigMapRef("dest-namespace", "dest-name")),
		withFirstSuccessTimeInThePast(),
	)}
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
			scheme := scheme.GetScheme()
			existingSyncStatus := buildSyncStatus("test-syncset",
				withResourcesToDelete(
					testConfigMapRef("dest-namespace", "failing-resource"),
					testConfigMapRef("dest-namespace", "successful-resource"),
				),
				withTransitionInThePast(),
				withFirstSuccessTimeInThePast(),
			)
			clusterSync := clusterSyncBuilder(scheme).Build(testcs.WithSyncSetStatus(existingSyncStatus))
			lease := buildSyncLease(time.Now().Add(-1 * time.Hour))
			existing := []runtime.Object{
				cdBuilder(scheme).Build(),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
				clusterSync,
				lease}
			if !tc.syncSetRemoved {
				existing = append(existing,
					testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
						testsyncset.ForClusterDeployments(testCDName),
						testsyncset.WithGeneration(2),
						testsyncset.WithApplyMode(hivev1.SyncResourceApplyMode),
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
			expectedSyncSetStatusBuilder := newSyncStatusBuilder("test-syncset").Options(
				withFailureResult("failed to delete v1, Kind=ConfigMap dest-namespace/failing-resource: error deleting resource"),
				withResourcesToDelete(
					testConfigMapRef("dest-namespace", "failing-resource"),
				),
				withFirstSuccessTimeInThePast(),
			)
			if tc.syncSetRemoved {
				expectedSyncSetStatusBuilder = expectedSyncSetStatusBuilder.Options(withObservedGeneration(0))
			} else {
				expectedSyncSetStatusBuilder = expectedSyncSetStatusBuilder.Options(withObservedGeneration(2))
			}
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{expectedSyncSetStatusBuilder.Build()}
			rt.expectUnchangedLeaseRenewTime = true
			rt.expectRequeue = true
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
			scheme := scheme.GetScheme()
			resourceToApply := testConfigMap("resource-namespace", "resource-name")
			syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
				testsyncset.ForClusterDeployments(testCDName),
				testsyncset.WithGeneration(1),
				testsyncset.WithApplyBehavior(tc.applyBehavior),
				testsyncset.WithResources(resourceToApply),
				testsyncset.WithSecrets(
					testSecretMapping("test-secret", "secret-namespace", "secret-name"),
				),
				testsyncset.WithPatches(hivev1.SyncObjectPatch{
					APIVersion: "patch-api/v1",
					Kind:       "PatchKind",
					Namespace:  "patch-namespace",
					Name:       "patch-name",
					PatchType:  "patch-type",
					Patch:      "test-patch",
				}),
			)
			srcSecret := testsecret.FullBuilder(testNamespace, "test-secret", scheme).Build(
				testsecret.WithDataKeyValue("test-key", []byte("test-data")),
			)
			rt := newReconcileTest(t, mockCtrl, scheme,
				cdBuilder(scheme).Build(),
				clusterSyncBuilder(scheme).Build(),
				teststatefulset.FullBuilder("hive", stsName, scheme).Build(
					teststatefulset.WithCurrentReplicas(3),
					teststatefulset.WithReplicas(3),
				),
				syncSet,
				srcSecret)
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
			rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-syncset")}
			rt.run(t)
		})
	}
}

func TestReconcileClusterSync_IgnoreNotApplicableSyncSets(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	syncSetResourceToApply := testConfigMap("dest-namespace", "resource-from-applicable-syncset")
	applicableSyncSet := testsyncset.FullBuilder(testNamespace, "applicable-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithResources(syncSetResourceToApply),
	)
	nonApplicableSyncSet := testsyncset.FullBuilder(testNamespace, "non-applicable-syncset", scheme).Build(
		testsyncset.ForClusterDeployments("other-cd"),
		testsyncset.WithGeneration(1),
		testsyncset.WithResources(
			testConfigMap("dest-namespace", "resource-from-non-applicable-syncset"),
		),
	)
	selectorSyncSetResourceToApply := testConfigMap("dest-namespace", "resource-from-applicable-selectorsyncset")
	applicableSelectorSyncSet := testselectorsyncset.FullBuilder("applicable-selectorsyncset", scheme).Build(
		testselectorsyncset.WithLabelSelector("test-label-key", "test-label-value"),
		testselectorsyncset.WithGeneration(1),
		testselectorsyncset.WithResources(selectorSyncSetResourceToApply),
	)
	nonApplicableSelectorSyncSet := testselectorsyncset.FullBuilder("non-applicable-selectorsyncset", scheme).Build(
		testselectorsyncset.WithLabelSelector("test-label-key", "other-label-value"),
		testselectorsyncset.WithGeneration(1),
		testselectorsyncset.WithResources(
			testConfigMap("dest-namespace", "resource-from-non-applicable-selectorsyncset"),
		),
	)
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(testcd.WithLabel("test-label-key", "test-label-value")),
		clusterSyncBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		applicableSyncSet,
		nonApplicableSyncSet,
		applicableSelectorSyncSet,
		nonApplicableSelectorSyncSet,
	)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(syncSetResourceToApply)).Return(resource.CreatedApplyResult, nil)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(selectorSyncSetResourceToApply)).Return(resource.CreatedApplyResult, nil)
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("applicable-syncset")}
	rt.expectedSelectorSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("applicable-selectorsyncset")}
	rt.run(t)
}

func TestReconcileClusterSync_ApplySecretForSelectorSyncSet(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	cd := cdBuilder(scheme).Build(testcd.WithLabel("test-label-key", "test-label-value"))
	selectorSyncSet := testselectorsyncset.FullBuilder("test-selectorsyncset", scheme).Build(
		testselectorsyncset.WithLabelSelector("test-label-key", "test-label-value"),
		testselectorsyncset.WithGeneration(1),
		testselectorsyncset.WithSecrets(
			hivev1.SecretMapping{
				SourceRef: hivev1.SecretReference{Namespace: "src-namespace", Name: "src-name"},
				TargetRef: hivev1.SecretReference{Namespace: "dest-namespace", Name: "dest-name"},
			},
		),
	)
	srcSecret := testsecret.FullBuilder("src-namespace", "src-name", scheme).Build(
		testsecret.WithDataKeyValue("test-key", []byte("test-data")),
	)
	rt := newReconcileTest(t, mockCtrl, scheme,
		cd,
		clusterSyncBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		selectorSyncSet,
		srcSecret)
	secretToApply := testsecret.BasicBuilder().GenericOptions(
		testgeneric.WithNamespace("dest-namespace"),
		testgeneric.WithName("dest-name"),
		testgeneric.WithTypeMeta(scheme),
	).Build(
		testsecret.WithDataKeyValue("test-key", []byte("test-data")),
	)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(secretToApply)).Return(resource.CreatedApplyResult, nil)
	rt.expectedSelectorSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-selectorsyncset")}
	rt.run(t)
}

func TestReconcileClusterSync_MissingSecretNamespaceForSelectorSyncSet(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	cd := cdBuilder(scheme).Build(testcd.WithLabel("test-label-key", "test-label-value"))
	selectorSyncSet := testselectorsyncset.FullBuilder("test-selectorsyncset", scheme).Build(
		testselectorsyncset.WithLabelSelector("test-label-key", "test-label-value"),
		testselectorsyncset.WithGeneration(1),
		testselectorsyncset.WithSecrets(
			testSecretMapping("test-secret", "dest-namespace", "dest-name"),
		),
	)
	srcSecret := testsecret.FullBuilder(testNamespace, "test-secret", scheme).Build(
		testsecret.WithDataKeyValue("test-key", []byte("test-data")),
	)
	rt := newReconcileTest(t, mockCtrl, scheme,
		cd,
		clusterSyncBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		selectorSyncSet,
		srcSecret)
	rt.expectedFailedMessage = "SelectorSyncSet test-selectorsyncset is failing"
	rt.expectedSelectorSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-selectorsyncset",
		withFailureResult("source namespace missing for secret 0"),
		withNoFirstSuccessTime(),
	)}
	rt.run(t)
}

func TestReconcileClusterSync_ValidSecretNamespaceForSyncSet(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithSecrets(
			hivev1.SecretMapping{
				SourceRef: hivev1.SecretReference{Namespace: testNamespace, Name: "test-secret"},
				TargetRef: hivev1.SecretReference{Namespace: "dest-namespace", Name: "dest-name"},
			},
		),
	)
	srcSecret := testsecret.FullBuilder(testNamespace, "test-secret", scheme).Build(
		testsecret.WithDataKeyValue("test-key", []byte("test-data")),
	)
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		clusterSyncBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		syncSet,
		srcSecret)
	secretToApply := testsecret.BasicBuilder().GenericOptions(
		testgeneric.WithNamespace("dest-namespace"),
		testgeneric.WithName("dest-name"),
		testgeneric.WithTypeMeta(scheme),
	).Build(
		testsecret.WithDataKeyValue("test-key", []byte("test-data")),
	)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(secretToApply)).Return(resource.CreatedApplyResult, nil)
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-syncset")}
	rt.run(t)
}

func TestReconcileClusterSync_InvalidSecretNamespaceForSyncSet(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithSecrets(
			hivev1.SecretMapping{
				SourceRef: hivev1.SecretReference{Namespace: "src-namespace", Name: "src-name"},
				TargetRef: hivev1.SecretReference{Namespace: "dest-namespace", Name: "dest-name"},
			},
		),
	)
	srcSecret := testsecret.FullBuilder("src-namespace", "src-name", scheme).Build(
		testsecret.WithDataKeyValue("test-key", []byte("test-data")),
	)
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		clusterSyncBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		syncSet,
		srcSecret)
	rt.expectedFailedMessage = "SyncSet test-syncset is failing"
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-syncset",
		withFailureResult("source in wrong namespace for secret 0"),
		withNoFirstSuccessTime(),
	)}
	rt.run(t)
}

func TestReconcileClusterSync_MissingSourceSecret(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithSecrets(
			testSecretMapping("test-secret", "dest-namespace", "dest-name"),
		),
	)
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		clusterSyncBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		syncSet)
	rt.expectedFailedMessage = "SyncSet test-syncset is failing"
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-syncset",
		withFailureResult(`failed to read secret 0: secrets "test-secret" not found`),
		withNoFirstSuccessTime(),
	)}
	rt.expectRequeue = true
	rt.run(t)
}

func TestReconcileClusterSync_ConditionNotMutatedWhenMessageNotChanged(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	resourceToApply := testConfigMap("dest-namespace", "dest-name")
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithResources(resourceToApply),
	)
	existingClusterSync := clusterSyncBuilder(scheme).Build(
		testcs.WithSyncSetStatus(buildSyncStatus("test-syncset",
			withFailureResult("failed to apply"),
			withTransitionInThePast(),
		)),
		testcs.WithCondition(hiveintv1alpha1.ClusterSyncCondition{
			Type:               hiveintv1alpha1.ClusterSyncFailed,
			Status:             corev1.ConditionTrue,
			Reason:             "Failure",
			Message:            "SyncSet test-syncset is failing",
			LastTransitionTime: timeInThePast,
			LastProbeTime:      timeInThePast,
		}),
	)
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		existingClusterSync,
		syncSet)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourceToApply)).
		Return(resource.ApplyResult(""), errors.New("test apply error"))
	rt.expectedFailedMessage = "SyncSet test-syncset is failing"
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{buildSyncStatus("test-syncset",
		withFailureResult("failed to apply resource 0: test apply error"),
		withNoFirstSuccessTime(),
	)}
	rt.expectRequeue = true
	rt.run(t)
	actualClusterSync := &hiveintv1alpha1.ClusterSync{}
	err := rt.c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: testClusterSyncName}, actualClusterSync)
	require.NoError(t, err, "unexpected error getting ClusterSync")
	require.Len(t, actualClusterSync.Status.Conditions, 1, "expected exactly 1 condition")
	cond := actualClusterSync.Status.Conditions[0]
	require.Equal(t, hiveintv1alpha1.ClusterSyncFailed, cond.Type, "expected Failed condition")
	require.Equal(t, string(corev1.ConditionTrue), string(cond.Status), "expected Failed condition to be true")
	assert.Equal(t, timeInThePast, cond.LastTransitionTime, "expected no change in last transition time")
	assert.Equal(t, timeInThePast, cond.LastProbeTime, "expected no change in last probe time")
}

func TestReconcileClusterSync_FirstSuccessTime(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	cd := cdBuilder(scheme).Options(testcd.InstalledTimestamp(timeInThePast.Time.Add(-time.Minute * 15).Truncate(time.Second))).Build()
	resourceToApply := testConfigMap("dest-namespace", "dest-name")
	syncSetNew := testsyncset.FullBuilder(testNamespace, "test-syncset-new", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithResources(resourceToApply),
	)
	syncSetOld := testsyncset.FullBuilder(testNamespace, "test-syncset-old", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithResources(resourceToApply),
	)
	oldFirstSuccessTime := metav1.NewTime(timeInThePast.Time.Add(time.Minute * 10).Truncate(time.Second))
	existingOldSyncStatus := buildSyncStatus("test-syncset-old",
		withTransitionInThePast(),
		withFirstSuccessTime(oldFirstSuccessTime),
	)
	existingNewSyncStatus := buildSyncStatus("test-syncset-new",
		withTransitionInThePast(),
		withFirstSuccessTimeInThePast(),
	)
	clusterSync := clusterSyncBuilder(scheme).Build(
		testcs.WithSyncSetStatus(existingOldSyncStatus),
		testcs.WithSyncSetStatus(existingNewSyncStatus),
	)
	syncLease := buildSyncLease(time.Now().Add(-time.Hour))
	rt := newReconcileTest(t, mockCtrl, scheme,
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		cd,
		syncSetOld,
		syncSetNew,
		clusterSync,
		syncLease)
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{
		buildSyncStatus("test-syncset-new", withTransitionInThePast(), withFirstSuccessTimeInThePast()),
		buildSyncStatus("test-syncset-old", withTransitionInThePast(), withFirstSuccessTime(oldFirstSuccessTime)),
	}
	rt.expectUnchangedLeaseRenewTime = true
	rt.run(t)
	updatedClusterSync := &hiveintv1alpha1.ClusterSync{}
	err := rt.c.Get(context.TODO(), client.ObjectKey{Name: testCDName, Namespace: testNamespace}, updatedClusterSync)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}
	// ClusterSync.Status.FirstSuccessTime is the oldest SyncStatus.FirstSuccessTime
	assert.Equal(t, oldFirstSuccessTime, *updatedClusterSync.Status.FirstSuccessTime)
}

func TestReconcileClusterSync_NoFirstSuccessTimeSet(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	cd := cdBuilder(scheme).Options(testcd.InstalledTimestamp(timeInThePast.Time.Add(-time.Minute * 15).Truncate(time.Second))).Build()
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		testsyncset.WithGeneration(1),
		testsyncset.WithResources(testConfigMap("dest-namespace", "dest-name")),
	)
	existingSyncStatus := buildSyncStatus("test-syncset",
		withTransitionInThePast(),
		withFailureResult("test apply error"),
		withNoFirstSuccessTime(),
	)
	clusterSync := clusterSyncBuilder(scheme).Build(
		testcs.WithSyncSetStatus(existingSyncStatus),
	)
	syncLease := buildSyncLease(time.Now().Add(-time.Hour))
	rt := newReconcileTest(t, mockCtrl, scheme,
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		cd,
		syncSet,
		clusterSync,
		syncLease)
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{
		buildSyncStatus("test-syncset",
			withNoFirstSuccessTime(),
			withFailureResult("failed to apply resource 0: test apply error")),
	}
	rt.mockResourceHelper.EXPECT().Apply(gomock.Any()).
		Return(resource.ApplyResult(""), errors.New("test apply error")).Times(1)
	rt.expectedFailedMessage = "SyncSet test-syncset is failing"
	rt.expectUnchangedLeaseRenewTime = true
	rt.expectRequeue = true
	rt.run(t)
	updatedClusterSync := &hiveintv1alpha1.ClusterSync{}
	err := rt.c.Get(context.TODO(), client.ObjectKey{Name: testCDName, Namespace: testNamespace}, updatedClusterSync)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}
	// ClusterSync.Status.FirstSuccessTime remains unset
	assert.Nil(t, updatedClusterSync.Status.FirstSuccessTime)
}

func TestReconcileClusterSync_FirstSuccessTimeSetWithNoSyncSets(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	cd := cdBuilder(scheme).Options(testcd.InstalledTimestamp(timeInThePast.Time.Add(-time.Minute * 15).Truncate(time.Second))).Build()
	clusterSync := clusterSyncBuilder(scheme).Build()
	syncLease := buildSyncLease(time.Now().Add(-time.Hour))
	rt := newReconcileTest(t, mockCtrl, scheme,
		cd,
		clusterSync,
		syncLease,
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
	)
	rt.expectUnchangedLeaseRenewTime = true
	rt.expectRequeue = false
	rt.run(t)
	updatedClusterSync := &hiveintv1alpha1.ClusterSync{}
	err := rt.c.Get(context.TODO(), client.ObjectKey{Name: testCDName, Namespace: testNamespace}, updatedClusterSync)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}
	// ClusterSync.Status.FirstSuccessTime set when there are no syncsets for clustersync
	assert.NotNil(t, updatedClusterSync.Status.FirstSuccessTime)
}

func TestReconcileClusterSync_SyncToUpsertResourceApplyMode(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	resourcesToApply := testConfigMap("dest-namespace", "dest-name")
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		// ResourceApplyMode is Upsert
		testsyncset.WithApplyMode(hivev1.UpsertResourceApplyMode),
		testsyncset.WithGeneration(2),
		testsyncset.WithResources(resourcesToApply),
	)
	existingSyncStatus := buildSyncStatus("test-syncset",
		withTransitionInThePast(),
		withFirstSuccessTimeInThePast(),
		// Existing SyncStatus has ResourcesToDelete
		withResourcesToDelete(
			testConfigMapRef("dest-namespace", "dest-name"),
			// Status contains a resource no longer in SyncSet resources which will not be deleted on the remote cluster
			testConfigMapRef("deleted-namespace", "deleted-name"),
		),
	)
	clusterSync := clusterSyncBuilder(scheme).Build(
		testcs.WithSyncSetStatus(existingSyncStatus),
	)
	syncLease := buildSyncLease(time.Now().Add(-time.Hour))
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		syncSet,
		clusterSync,
		syncLease)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourcesToApply)).Return(resource.CreatedApplyResult, nil)
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{
		buildSyncStatus("test-syncset",
			withFirstSuccessTimeInThePast(),
			withObservedGeneration(2),
		),
	}
	rt.expectUnchangedLeaseRenewTime = true
	rt.run(t)

	updatedClusterSync := &hiveintv1alpha1.ClusterSync{}
	err := rt.c.Get(context.TODO(), client.ObjectKey{Name: testCDName, Namespace: testNamespace}, updatedClusterSync)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}
	// ClusterSync Status for SyncSet has no ResourcesToDelete
	assert.Empty(t, updatedClusterSync.Status.SyncSets[0].ResourcesToDelete, "expected no resources to delete")
}

func TestReconcileClusterSync_UpsertToSyncResourceApplyMode(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	scheme := scheme.GetScheme()
	resourcesToApply := testConfigMap("dest-namespace", "dest-name")
	syncSet := testsyncset.FullBuilder(testNamespace, "test-syncset", scheme).Build(
		testsyncset.ForClusterDeployments(testCDName),
		// ResourceApplyMode is Sync
		testsyncset.WithApplyMode(hivev1.SyncResourceApplyMode),
		testsyncset.WithGeneration(2),
		testsyncset.WithResources(resourcesToApply),
	)
	// Existing SyncStatus has no ResourcesToDelete
	existingSyncStatus := buildSyncStatus("test-syncset",
		withTransitionInThePast(),
		withFirstSuccessTimeInThePast(),
	)
	clusterSync := clusterSyncBuilder(scheme).Build(
		testcs.WithSyncSetStatus(existingSyncStatus),
	)
	syncLease := buildSyncLease(time.Now().Add(-time.Hour))
	rt := newReconcileTest(t, mockCtrl, scheme,
		cdBuilder(scheme).Build(),
		teststatefulset.FullBuilder("hive", stsName, scheme).Build(
			teststatefulset.WithCurrentReplicas(3),
			teststatefulset.WithReplicas(3),
		),
		syncSet,
		clusterSync,
		syncLease)
	rt.mockResourceHelper.EXPECT().Apply(newApplyMatcher(resourcesToApply)).Return(resource.CreatedApplyResult, nil)
	rt.expectedSyncSetStatuses = []hiveintv1alpha1.SyncStatus{
		buildSyncStatus("test-syncset",
			withFirstSuccessTimeInThePast(),
			withObservedGeneration(2),
			// Expected SyncStatus has ResourcesToDelete
			withResourcesToDelete(
				testConfigMapRef("dest-namespace", "dest-name"),
			),
		),
	}
	rt.expectUnchangedLeaseRenewTime = true
	rt.run(t)

	updatedClusterSync := &hiveintv1alpha1.ClusterSync{}
	err := rt.c.Get(context.TODO(), client.ObjectKey{Name: testCDName, Namespace: testNamespace}, updatedClusterSync)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}
}

func cdBuilder(scheme *runtime.Scheme) testcd.Builder {
	return testcd.FullBuilder(testNamespace, testCDName, scheme).
		GenericOptions(
			testgeneric.WithUID(testCDUID),
		).
		Options(
			testcd.Installed(),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.UnreachableCondition,
				Status: corev1.ConditionFalse,
			}),
			testcd.InstalledTimestamp(time.Now()),
		)
}

func clusterSyncBuilder(scheme *runtime.Scheme) testcs.Builder {
	return testcs.FullBuilder(testNamespace, testClusterSyncName, scheme).GenericOptions(
		testgeneric.WithUID(testClusterSyncUID),
		testgeneric.WithOwnerReference(cdBuilder(scheme).Build()),
	)
}

func buildSyncLease(t time.Time) *hiveintv1alpha1.ClusterSyncLease {
	return &hiveintv1alpha1.ClusterSyncLease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testLeaseName,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "hiveinternal.openshift.io/v1alpha1",
				Kind:               "ClusterSync",
				Name:               testClusterSyncName,
				UID:                testClusterSyncUID,
				BlockOwnerDeletion: pointer.BoolPtr(true),
			}},
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

type syncStatusOption func(syncStatus *hiveintv1alpha1.SyncStatus)

type syncStatusBuilder struct {
	name    string
	options []syncStatusOption
}

func newSyncStatusBuilder(name string) *syncStatusBuilder {
	return &syncStatusBuilder{name: name}
}

func (b *syncStatusBuilder) Build(opts ...syncStatusOption) hiveintv1alpha1.SyncStatus {
	return buildSyncStatus(b.name, append(b.options, opts...)...)
}

func (b *syncStatusBuilder) Options(opts ...syncStatusOption) *syncStatusBuilder {
	return &syncStatusBuilder{
		name:    b.name,
		options: append(b.options, opts...),
	}
}

func buildSyncStatus(name string, opts ...syncStatusOption) hiveintv1alpha1.SyncStatus {
	syncStatus := &hiveintv1alpha1.SyncStatus{
		Name:               name,
		ObservedGeneration: 1,
		Result:             hiveintv1alpha1.SuccessSyncSetResult,
		FirstSuccessTime:   &metav1.Time{},
	}
	for _, opt := range opts {
		opt(syncStatus)
	}
	return *syncStatus
}

func withObservedGeneration(observedGeneration int64) syncStatusOption {
	return func(syncStatus *hiveintv1alpha1.SyncStatus) {
		syncStatus.ObservedGeneration = observedGeneration
	}
}

func withFailureResult(message string) syncStatusOption {
	return func(syncStatus *hiveintv1alpha1.SyncStatus) {
		syncStatus.Result = hiveintv1alpha1.FailureSyncSetResult
		syncStatus.FailureMessage = message
	}
}

func withResourcesToDelete(resourcesToDelete ...hiveintv1alpha1.SyncResourceReference) syncStatusOption {
	return func(syncStatus *hiveintv1alpha1.SyncStatus) {
		syncStatus.ResourcesToDelete = resourcesToDelete
	}
}

func withTransitionInThePast() syncStatusOption {
	return func(syncStatus *hiveintv1alpha1.SyncStatus) {
		syncStatus.LastTransitionTime = timeInThePast
	}
}

func withNoFirstSuccessTime() syncStatusOption {
	return func(syncStatus *hiveintv1alpha1.SyncStatus) {
		syncStatus.FirstSuccessTime = nil
	}
}

func withFirstSuccessTimeInThePast() syncStatusOption {
	return func(syncStatus *hiveintv1alpha1.SyncStatus) {
		syncStatus.FirstSuccessTime = &timeInThePast
	}
}

func withFirstSuccessTime(firstSuccessTime metav1.Time) syncStatusOption {
	return func(syncStatus *hiveintv1alpha1.SyncStatus) {
		syncStatus.FirstSuccessTime = &firstSuccessTime
	}
}
