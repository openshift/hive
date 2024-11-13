package privatelink

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	mockactuator "github.com/openshift/hive/pkg/controller/privatelink/actuator/mock"
	"github.com/openshift/hive/pkg/controller/privatelink/conditions"
	testassert "github.com/openshift/hive/pkg/test/assert"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testcp "github.com/openshift/hive/pkg/test/clusterprovision"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/openshift/hive/pkg/test/generic"
	testlogger "github.com/openshift/hive/pkg/test/logger"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	testNameSpace         = "test-namespace"
	testClusterDeployment = "test-clusterdeployment"
	previousInfraID       = "test-infraid"
	testClusterProvision  = "test-clusterprovision"
)

func Test_Reconcile(t *testing.T) {
	scheme := scheme.GetScheme()
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme)
	cpBuilder := testcp.FullBuilder(testNameSpace, testClusterProvision)

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment
		cp   *hivev1.ClusterProvision

		hubActuatorConfig  func(*mockactuator.MockActuator)
		linkActuatorConfig func(*mockactuator.MockActuator)

		privateLinkEnabled    bool
		conditionsInitialized bool

		expect           bool
		expectConditions []hivev1.ClusterDeploymentCondition
		expectError      string
		expectLogs       []string
	}{{ // do not reconcile when Pause annotation is configured
		name:       "ClusterDeployment pause annotation",
		cd:         cdBuilder.GenericOptions(generic.WithAnnotation(constants.ReconcilePauseAnnotation, "true")).Build(),
		expectLogs: []string{"skipping reconcile due to ClusterDeployment pause annotation"},
	}, { // there should be an error on failure to update the clusterdeployment conditions in the cluster api
		name:        "failure to update clusterdeployment status after initializing conditions",
		expectError: "failed to update cluster deployment status: clusterdeployments.hive.openshift.io \"" + testClusterDeployment + "\" not found",
	}, { // do not reconcile immediately after conditions are initialized
		name:       "conditions not initialized",
		cd:         cdBuilder.Build(),
		expectLogs: []string{"initializing private link controller conditions"},
	}, { // privatelink not enabled, there should be an error on failure to cleanup cluster deployment
		name: "privatelink not enabled, cleanup required, failure to clean up",
		cd: cdBuilder.GenericOptions(
			generic.WithFinalizer(finalizer),
		).Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{}),
		),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(true)
			m.EXPECT().CleanupRequired(gomock.Any()).Return(true)
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("hub actuator cleanup failed"))
		},
		conditionsInitialized: true,
		expectError:           "hub actuator cleanup failed",
	}, { // privatelink not enabled, do not reconcile when there is no cleanup required
		name: "privatelink not enabled, cleanup not required",
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(false)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(false)
		},
		conditionsInitialized: true,
		expectLogs:            []string{"cluster deployment does not have private link enabled, so skipping"},
	}, { // there should be an error on failure to cleanup cluster deployment when the clusterdeployment is deleted
		name: "clusterDeployment deleted, error on cleanup",
		cd: cdBuilder.GenericOptions(
			generic.WithFinalizer(finalizer),
			generic.Deleted(),
		).Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{}),
		),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(true)
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("hub actuator cleanup failed"))
		},
		conditionsInitialized: true,
		privateLinkEnabled:    true,
		expectError:           "hub actuator cleanup failed",
	}, { // there should be an error on failure to add a finalizer to the clusterDeployment
		name:                  "error on adding finalizer",
		conditionsInitialized: true,
		privateLinkEnabled:    true,
		expectError:           "error adding finalizer to ClusterDeployment: clusterdeployments.hive.openshift.io \"" + testClusterDeployment + "\" not found",
	}, { // there should be a result returned when sync is not needed
		name: "sync not needed",
		cd: cdBuilder.Build(
			testcd.Installed(),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:          hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:        corev1.ConditionTrue,
				LastProbeTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
			}),
		),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().ShouldSync(gomock.Any()).Return(false)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().ShouldSync(gomock.Any()).Return(false)
		},
		conditionsInitialized: true,
		privateLinkEnabled:    true,
		expect:                true,
		expectLogs:            []string{"Sync not needed"},
	}, { // there should be an error on failure to reconcile privatelink
		name: "installed, sync required, failure to reconcile",
		cd: cdBuilder.Build(
			testcd.Installed(),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{}),
		),
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Reconcile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(reconcile.Result{}, errors.New("link actuator reconcile failed"))
		},
		conditionsInitialized: true,
		privateLinkEnabled:    true,
		expectError:           "link actuator reconcile failed",
	}, { // there should not be an error on privatelink reconcile success
		name: "installed, sync required, successful reconcile",
		cd: cdBuilder.Build(
			testcd.Installed(),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{}),
		),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Reconcile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(reconcile.Result{}, nil)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Reconcile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(reconcile.Result{}, nil)
		},
		conditionsInitialized: true,
		privateLinkEnabled:    true,
		expectLogs:            []string{"reconciling already installed cluster deployment"},
	}, { // do not sync while waiting on cluster deployment provision to start
		name: "sync required, waiting on provision to start",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{}),
		),
		conditionsInitialized: true,
		privateLinkEnabled:    true,
		expectLogs:            []string{"waiting for cluster deployment provision to start, will retry soon."},
	}, { // there should be an error when cluster provision not found
		name: "sync required, cluster provision not found",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{}),
			testcd.WithClusterProvision(testClusterProvision),
		),
		conditionsInitialized: true,
		privateLinkEnabled:    true,
		expectError:           "clusterprovisions.hive.openshift.io \"" + testClusterProvision + "\" not found",
	}, { // there should be an error on failure to cleanup previous privision
		name: "sync required, failure to cleanup previous provision",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{}),
			testcd.WithClusterProvision(testClusterProvision),
		),
		cp: cpBuilder.Options(testcp.WithPreviousInfraID(ptr.To(previousInfraID))).Build(),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(false)
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(true)
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("link actuator cleanup failed"))
		},
		conditionsInitialized: true,
		privateLinkEnabled:    true,
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Reason:  "CleanupForProvisionReattemptFailed",
			Message: "error cleaning up PrivateLink resources for ClusterDeployment: link actuator cleanup failed",
		}},
		expectError: "error cleaning up PrivateLink resources for ClusterDeployment: link actuator cleanup failed",
	}, { // there should be a result when successfully cleaned up previous provision
		name: "sync required, successful cleanup previous provision",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{}),
			testcd.WithClusterProvision(testClusterProvision),
		),
		cp: cpBuilder.Options(testcp.WithPreviousInfraID(ptr.To(previousInfraID))).Build(),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(false)
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(true)
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		},
		conditionsInitialized: true,
		privateLinkEnabled:    true,
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Reason:  "PreviousAttemptCleanupComplete",
			Message: "successfully cleaned up resources from previous provision attempt so that next attempt can start",
		}},
		expect:     true,
		expectLogs: []string{"cleaning up PrivateLink resources from previous attempt"},
	}, { // do not sync while waiting on cluster metadata
		name: "sync required, waiting on cluster metadata",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{}),
			testcd.WithClusterProvision(testClusterProvision),
		),
		cp:                    cpBuilder.Build(),
		conditionsInitialized: true,
		privateLinkEnabled:    true,
		expectLogs:            []string{"waiting for cluster deployment provision to provide ClusterMetadata, will retry soon."},
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			existingCD := &hivev1.ClusterDeployment{}
			if test.cd == nil {
				test.cd = cdBuilder.Build()
			} else {
				existingCD = test.cd
			}
			existingCP := &hivev1.ClusterProvision{}
			if test.cp != nil {
				existingCP = test.cp
			}
			fakeClient := client.Client(testfake.NewFakeClientBuilder().
				WithRuntimeObjects(existingCD).
				WithRuntimeObjects(existingCP).
				Build())

			hubActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.hubActuatorConfig != nil {
				test.hubActuatorConfig(hubActuator)
			}

			linkActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.linkActuatorConfig != nil {
				test.linkActuatorConfig(linkActuator)
			}

			logger, hook := testlogger.NewLoggerWithHook()

			privateLink := &PrivateLink{
				Client:       fakeClient,
				cd:           test.cd,
				hubActuator:  hubActuator,
				linkActuator: linkActuator,
				logger:       logger,
			}

			if test.conditionsInitialized {
				newConditions, _ := conditions.InitializeConditions(test.cd)
				test.cd.Status.Conditions = newConditions
			}

			result, err := privateLink.Reconcile(test.privateLinkEnabled)
			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			for _, log := range test.expectLogs {
				testlogger.AssertHookContainsMessage(t, hook, log)
			}

			if test.expectConditions != nil {
				curr := &hivev1.ClusterDeployment{}
				errGet := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: test.cd.Namespace, Name: test.cd.Name}, curr)
				assert.NoError(t, errGet)
				testassert.AssertConditions(t, curr, test.expectConditions)
			}

			if test.expect {
				assert.False(t, result.IsZero())
			}
		})
	}
}

func Test_cleanupRequired(t *testing.T) {
	cases := []struct {
		name string

		hubActuatorConfig  func(*mockactuator.MockActuator)
		linkActuatorConfig func(*mockactuator.MockActuator)

		expect bool
	}{{ // cleanup is required when the hub actuator needs to clean up
		name: "hub cleanup required",
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(true)
		},
		expect: true,
	}, { // cleanup is required when the link actuator needs to clean up
		name: "link cleanup required",
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(false)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(true)
		},
		expect: true,
	}, { // there is no cleanup required if neither the hub actuator or link actuator needs to clean up
		name: "no cleanup required",
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(false)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(false)
		},
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			hubActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.hubActuatorConfig != nil {
				test.hubActuatorConfig(hubActuator)
			}

			linkActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.linkActuatorConfig != nil {
				test.linkActuatorConfig(linkActuator)
			}

			privateLink := &PrivateLink{
				hubActuator:  hubActuator,
				linkActuator: linkActuator,
			}

			result := privateLink.cleanupRequired()
			assert.Equal(t, result, test.expect)
		})
	}
}

func Test_cleanupClusterDeployment(t *testing.T) {
	scheme := scheme.GetScheme()
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme)

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		hubActuatorConfig  func(*mockactuator.MockActuator)
		linkActuatorConfig func(*mockactuator.MockActuator)

		expectConditions []hivev1.ClusterDeploymentCondition
		expectError      string
		expectFinalizer  bool
	}{{ // there is no cleanup to do if there is no finalizer
		name: "no finalizer",
	}, { // there should be an error when there is a failure to cleanupPrivateLink
		name: "cleanup required with error on cleanupPrivateLink",
		cd: cdBuilder.GenericOptions(
			generic.WithFinalizer(finalizer),
		).Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{}),
		),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(false)
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(true)
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("link actuator cleanup failed"))
		},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Reason:  "CleanupForDeprovisionFailed",
			Message: "link actuator cleanup failed",
		}},
		expectError:     "link actuator cleanup failed",
		expectFinalizer: true,
	}, { // there should be no errors on success
		name: "no errors",
		cd: cdBuilder.GenericOptions(
			generic.WithFinalizer(finalizer),
		).Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{}),
		),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(false)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().CleanupRequired(gomock.Any()).Return(false)
		},
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

			hubActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.hubActuatorConfig != nil {
				test.hubActuatorConfig(hubActuator)
			}

			linkActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.linkActuatorConfig != nil {
				test.linkActuatorConfig(linkActuator)
			}

			logger, _ := testlogger.NewLoggerWithHook()

			privateLink := &PrivateLink{
				Client:       fakeClient,
				cd:           test.cd,
				hubActuator:  hubActuator,
				linkActuator: linkActuator,
				logger:       logger,
			}

			_, err := privateLink.cleanupClusterDeployment()
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

			if test.expectFinalizer {
				assert.Contains(t, privateLink.cd.ObjectMeta.Finalizers, finalizer)
			} else {
				assert.NotContains(t, privateLink.cd.ObjectMeta.Finalizers, finalizer)
			}
		})
	}
}

func Test_cleanupPrivateLink(t *testing.T) {
	scheme := scheme.GetScheme()
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme)

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		hubActuatorConfig  func(*mockactuator.MockActuator)
		linkActuatorConfig func(*mockactuator.MockActuator)

		expectError string
	}{{ // there should be an error when the hub actuator fails to cleanup
		name: "hub actuator cleanup error",
		cd:   cdBuilder.Build(testcd.WithClusterMetadata(&hivev1.ClusterMetadata{})),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("hub actuator cleanup failed"))
		},
		expectError: "hub actuator cleanup failed",
	}, { // there should be an error when the link actuator fails to cleanup
		name: "link actuator cleanup error",
		cd:   cdBuilder.Build(testcd.WithClusterMetadata(&hivev1.ClusterMetadata{})),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("link actuator cleanup failed"))
		},
		expectError: "link actuator cleanup failed",
	}, { // there should be no errors on success
		name: "no errors",
		cd:   cdBuilder.Build(testcd.WithClusterMetadata(&hivev1.ClusterMetadata{})),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			hubActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.hubActuatorConfig != nil {
				test.hubActuatorConfig(hubActuator)
			}

			linkActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.linkActuatorConfig != nil {
				test.linkActuatorConfig(linkActuator)
			}

			privateLink := &PrivateLink{
				cd:           test.cd,
				hubActuator:  hubActuator,
				linkActuator: linkActuator,
			}

			err := privateLink.cleanupPrivateLink(test.cd.Spec.ClusterMetadata)
			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}
		})
	}
}

func Test_reconcilePrivateLink(t *testing.T) {
	scheme := scheme.GetScheme()
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme)

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		hubActuatorConfig  func(*mockactuator.MockActuator)
		linkActuatorConfig func(*mockactuator.MockActuator)

		expect           bool
		expectConditions []hivev1.ClusterDeploymentCondition
		expectError      string
	}{{ // there should be an error when the link actuator fails to reconcile
		name: "link actuator reconcile error",
		cd:   cdBuilder.Build(testcd.WithClusterMetadata(&hivev1.ClusterMetadata{})),
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Reconcile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(reconcile.Result{}, errors.New("link actuator reconcile failed"))
		},
		expectError: "link actuator reconcile failed",
	}, { // there should be a result when the link actuator reconcile returns a result
		name: "link actuator reconcile result",
		cd:   cdBuilder.Build(testcd.WithClusterMetadata(&hivev1.ClusterMetadata{})),
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Reconcile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(reconcile.Result{Requeue: true}, nil)
		},
		expect: true,
	}, { // there should be an error when the hub actuator fails to reconcile
		name: "hub actuator reconcile error",
		cd:   cdBuilder.Build(testcd.WithClusterMetadata(&hivev1.ClusterMetadata{})),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Reconcile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(reconcile.Result{}, errors.New("hub actuator reconcile failed"))
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Reconcile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(reconcile.Result{}, nil)
		},
		expectError: "hub actuator reconcile failed",
	}, { // there should be a result when the hub actuator reconcile returns a result
		name: "hub actuator reconcile result",
		cd:   cdBuilder.Build(testcd.WithClusterMetadata(&hivev1.ClusterMetadata{})),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Reconcile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(reconcile.Result{Requeue: true}, nil)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Reconcile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(reconcile.Result{}, nil)
		},
		expect: true,
	}, { // there should bre no errors on success
		name: "no errors",
		cd:   cdBuilder.Build(testcd.WithClusterMetadata(&hivev1.ClusterMetadata{})),
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Reconcile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(reconcile.Result{}, nil)
		},
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Reconcile(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(reconcile.Result{}, nil)
		},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Reason:  "PrivateLinkAccessReady",
			Message: "private link access is ready for use",
		}},
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := client.Client(testfake.NewFakeClientBuilder().WithRuntimeObjects(test.cd).Build())

			hubActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.hubActuatorConfig != nil {
				test.hubActuatorConfig(hubActuator)
			}

			linkActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.linkActuatorConfig != nil {
				test.linkActuatorConfig(linkActuator)
			}

			logger, _ := testlogger.NewLoggerWithHook()

			privateLink := &PrivateLink{
				Client:       fakeClient,
				cd:           test.cd,
				hubActuator:  hubActuator,
				linkActuator: linkActuator,
				logger:       logger,
			}

			result, err := privateLink.reconcilePrivateLink(test.cd.Spec.ClusterMetadata)

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

			if test.expect {
				assert.False(t, result.IsZero())
			}
		})
	}
}

func Test_cleanupPreviousProvisionAttempt(t *testing.T) {
	scheme := scheme.GetScheme()
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme)

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment
		cp   *hivev1.ClusterProvision

		hubActuatorConfig  func(*mockactuator.MockActuator)
		linkActuatorConfig func(*mockactuator.MockActuator)

		expectAnnotation bool
		expectError      string
	}{{ // there should be an error when the kubeconfig is not available
		name:        "kubeconfig is not available",
		expectError: "cannot cleanup previous resources because the admin kubeconfig is not available",
	}, { // there should be an error when there link actuator cleanup fails
		name: "error cleaning up privatelink",
		cd:   cdBuilder.Build(testcd.WithClusterMetadata(&hivev1.ClusterMetadata{})),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("link actuator cleanup failed"))
		},
		cp: &hivev1.ClusterProvision{
			Spec: hivev1.ClusterProvisionSpec{
				PrevInfraID: ptr.To(previousInfraID),
			},
		},
		expectError: "error cleaning up PrivateLink resources for ClusterDeployment: link actuator cleanup failed",
	}, { // on success, the lastCleanupAnnotationKey annotations should be updated to be the last cleanup infra id
		name: "annotations updated",
		cd:   cdBuilder.Build(testcd.WithClusterMetadata(&hivev1.ClusterMetadata{})),
		cp: &hivev1.ClusterProvision{
			Spec: hivev1.ClusterProvisionSpec{
				PrevInfraID: ptr.To(previousInfraID),
			},
		},
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().Cleanup(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		},
		expectAnnotation: true,
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

			hubActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.hubActuatorConfig != nil {
				test.hubActuatorConfig(hubActuator)
			}

			linkActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.linkActuatorConfig != nil {
				test.linkActuatorConfig(linkActuator)
			}

			logger, _ := testlogger.NewLoggerWithHook()

			privateLink := &PrivateLink{
				Client:       fakeClient,
				cd:           test.cd,
				hubActuator:  hubActuator,
				linkActuator: linkActuator,
				logger:       logger,
			}

			err := privateLink.cleanupPreviousProvisionAttempt(test.cp)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			if test.expectAnnotation {
				assert.NotNil(t, test.cd.Annotations)
				assert.Equal(t, test.cd.Annotations[lastCleanupAnnotationKey], previousInfraID)
			}
		})
	}
}

func Test_shouldSync(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		hubActuatorConfig  func(*mockactuator.MockActuator)
		linkActuatorConfig func(*mockactuator.MockActuator)

		expect         bool
		ExpectDuration time.Duration
	}{{ // no sync when the clusterDeployment is deleted and there is no finalizer
		name: "deleted and no finalizer",
		cd:   cdBuilder.GenericOptions(generic.Deleted()).Build(),
	}, { // sync when deleted and there is a finaliser
		name:   "deleted and finalizer",
		cd:     cdBuilder.GenericOptions(generic.Deleted(), generic.WithFinalizer(finalizer)).Build(),
		expect: true,
	}, { // sync when there is a failed condition
		name: "failed condition",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.PrivateLinkFailedClusterDeploymentCondition,
				Status: corev1.ConditionTrue,
			}),
		),
		expect: true,
	}, { // sync when there is not a ready condition
		name:   "no ready condition",
		cd:     cdBuilder.Build(),
		expect: true,
	}, { // sync when the ready condition is false
		name: "ready condition false",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status: corev1.ConditionFalse,
			}),
		),
		expect: true,
	}, { // sync during the installation phase when ready for more than 10 minutes
		name: "installing, ready for more than 10 minutes",
		cd: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:          hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:        corev1.ConditionTrue,
				LastProbeTime: metav1.Time{Time: time.Now().Add(-11 * time.Minute)},
			}),
		),
		expect: true,
	}, { // sync when ready for for more than 2 hours.
		name: "installed, ready for more than 2 hours",
		cd: cdBuilder.Build(
			testcd.Installed(),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:          hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:        corev1.ConditionTrue,
				LastProbeTime: metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
			}),
		),
		expect: true,
	}, { // sync in one minute when less than a minute is left
		name: "installed, ready for less than 2 hours, less than a minute left",
		cd: cdBuilder.Build(
			testcd.Installed(),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:          hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:        corev1.ConditionTrue,
				LastProbeTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour).Add(-59 * time.Minute).Add(-30 * time.Second)},
			}),
		),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().ShouldSync(gomock.Any()).Return(false)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().ShouldSync(gomock.Any()).Return(false)
		},
		ExpectDuration: 1 * time.Minute,
	}, { // sync after installed when link actuator requires sync
		name: "installed, ready for less than 2 hours, link actuator sync required",
		cd: cdBuilder.Build(
			testcd.Installed(),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:          hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:        corev1.ConditionTrue,
				LastProbeTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
			}),
		),
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().ShouldSync(gomock.Any()).Return(true)
		},

		expect: true,
	}, { // sync after install when hub actuator requires sync
		name: "installed, ready for less than 2 hours, hub actuator sync required",
		cd: cdBuilder.Build(
			testcd.Installed(),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:          hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:        corev1.ConditionTrue,
				LastProbeTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
			}),
		),
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().ShouldSync(gomock.Any()).Return(true)
		},
		expect: true,
	}, { // sync every 2 hours when there is no specific reason to sync
		name: "installed, ready for less than 2 hours, no actuator sync required",
		cd: cdBuilder.Build(
			testcd.Installed(),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:          hivev1.PrivateLinkReadyClusterDeploymentCondition,
				Status:        corev1.ConditionTrue,
				LastProbeTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
			}),
		),
		hubActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().ShouldSync(gomock.Any()).Return(false)
		},
		linkActuatorConfig: func(m *mockactuator.MockActuator) {
			m.EXPECT().ShouldSync(gomock.Any()).Return(false)
		},
		ExpectDuration: 1 * time.Hour,
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			hubActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.hubActuatorConfig != nil {
				test.hubActuatorConfig(hubActuator)
			}

			linkActuator := mockactuator.NewMockActuator(gomock.NewController(t))
			if test.linkActuatorConfig != nil {
				test.linkActuatorConfig(linkActuator)
			}

			privateLink := &PrivateLink{
				cd:           test.cd,
				hubActuator:  hubActuator,
				linkActuator: linkActuator,
			}

			shouldSync, syncAfter := privateLink.shouldSync()
			assert.Equal(t, test.expect, shouldSync)
			assert.Equal(t, test.ExpectDuration, syncAfter)
		})
	}
}
