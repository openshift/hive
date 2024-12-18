package conditions

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	testassert "github.com/openshift/hive/pkg/test/assert"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testlogger "github.com/openshift/hive/pkg/test/logger"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	testNameSpace         = "test-namespace"
	testClusterDeployment = "test-clusterdeployment"
	testReason            = "test-reason"
	testMessage           = "test-message"
)

func Test_SetErrCondition(t *testing.T) {
	scheme := scheme.GetScheme()
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme)

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		expectConditions []hivev1.ClusterDeploymentCondition
		expectError      string
	}{{ // There should be an error when the clusterdeployment is not found
		name:        "clusterdeployment not found",
		expectError: "failed to get cluster deployment: clusterdeployments.hive.openshift.io \"" + testClusterDeployment + "\" not found",
	}, { // A missing failed condition should be set to true
		name: "missing failed condition changed to true",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // A false failed condition should be changed to true
		name: "false failed condition changed to true",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // A true failed condition reason should be updated
		name: "true failed condition reason changed",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  "some other reason",
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // A missing ready condition should be changed to false
		name: "missing ready condition changed to false",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  testReason,
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // A true ready condition should be changed to false
		name: "true ready condition changed to false",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  testReason,
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // A false ready condition reason should be updated
		name: "false ready condition reason changed",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  testReason,
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  "some other reason",
				Message: testMessage,
			}),
		),
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // There should be no changes when failed is already true and ready is already false with the same reason(s)
		name: "no change",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  testReason,
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			existingCD := &hivev1.ClusterDeployment{}
			if test.cd == nil {
				test.cd = cdBuilder.Build()
			} else {
				existingCD = test.cd
			}
			fakeClient := client.Client(testfake.NewFakeClientBuilder().WithRuntimeObjects(existingCD).Build())

			logger, _ := testlogger.NewLoggerWithHook()

			err := SetErrCondition(fakeClient, test.cd, testReason, errors.New(testMessage), logger)
			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			if test.expectConditions != nil {
				curr := &hivev1.ClusterDeployment{}
				errGet := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: test.cd.Namespace, Name: test.cd.Name}, curr)
				assert.NoError(t, errGet)
				testassert.AssertConditions(t, curr, test.expectConditions)
			}
		})
	}
}

func Test_SetReadyCondition(t *testing.T) {
	scheme := scheme.GetScheme()
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme)

	cases := []struct {
		name      string
		cd        *hivev1.ClusterDeployment
		completed corev1.ConditionStatus

		expectConditions []hivev1.ClusterDeploymentCondition
		expectError      string
	}{{ // There should be an error when the clusterdeployment is not found
		name:        "clusterdeployment not found",
		expectError: "failed to get cluster deployment: clusterdeployments.hive.openshift.io \"" + testClusterDeployment + "\" not found",
	}, { // A missing failed condition should be set to false when completed is true
		name: "missing failed condition set to false",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		completed: corev1.ConditionTrue,
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // A true failed condition should be changed to false when completed is true
		name: "true failed condition changed to false",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  testReason,
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		completed: corev1.ConditionTrue,
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // A false failed condition reason should be updated when completed is true
		name: "false failed condition reason change",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  "some-other-reason",
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		completed: corev1.ConditionTrue,
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // A missing ready condition should be set to false when completed is false
		name: "missing ready condition set to false",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // A missing ready condition should be set to true when completed is true
		name: "missing ready condition set to true",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		completed: corev1.ConditionTrue,
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // A false ready condition reason should be updated when completed is false
		name: "false ready condition reason change",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  "some other reason",
				Message: testMessage,
			}),
		),
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // There should be no change when a false ready condition has the same reason and completed is false
		name: "false ready condition no change",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // A false ready condition should be changed to true when completed is true
		name: "completed, false ready condition changed to true",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		completed: corev1.ConditionTrue,
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // A true ready condition reason should be updated when completed is true
		name: "completed, true ready condition reason change",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  "some other reason",
				Message: testMessage,
			}),
		),
		completed: corev1.ConditionTrue,
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // There should be no change when a true ready condition has the same message and completed is true
		name: "completed, true ready condition no change",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  testReason,
				Message: testMessage,
			}),
		),
		completed: corev1.ConditionTrue,
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  testReason,
			Message: testMessage,
		}},
	}, { // There should be no change when the ready condition is true and completed is false
		name: "true ready condition no change",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status:  corev1.ConditionFalse,
				Reason:  testReason,
				Message: testMessage,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:  corev1.ConditionTrue,
				Reason:  "some-other-reason",
				Message: testMessage,
			}),
		),
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Status:  corev1.ConditionFalse,
			Reason:  testReason,
			Message: testMessage,
		}, {
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  "some-other-reason",
			Message: testMessage,
		}},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			existingCD := &hivev1.ClusterDeployment{}
			if test.cd == nil {
				test.cd = cdBuilder.Build()
			} else {
				existingCD = test.cd
			}
			fakeClient := client.Client(testfake.NewFakeClientBuilder().WithRuntimeObjects(existingCD).Build())

			if test.completed == "" {
				test.completed = corev1.ConditionFalse
			}

			logger, _ := testlogger.NewLoggerWithHook()

			err := SetReadyCondition(fakeClient, test.cd, test.completed, testReason, testMessage, logger)
			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			if test.expectConditions != nil {
				curr := &hivev1.ClusterDeployment{}
				errGet := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: test.cd.Namespace, Name: test.cd.Name}, curr)
				assert.NoError(t, errGet)
				testassert.AssertConditions(t, curr, test.expectConditions)
			}
		})
	}
}
