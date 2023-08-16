package hibernation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	certsv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakekubeclient "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1 "github.com/openshift/api/config/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/hibernation/mock"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testcs "github.com/openshift/hive/pkg/test/clustersync"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	namespace = "test-namespace"
	cdName    = "test-cluster-deployment"
	finalizer = "test-finalizer"
)

func TestReconcile(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	scheme := scheme.GetScheme()

	cdBuilder := testcd.FullBuilder(namespace, cdName, scheme).Options(
		testcd.Installed(),
		testcd.WithClusterVersion("4.4.9"),
	)
	o := clusterDeploymentOptions{}
	csBuilder := testcs.FullBuilder(namespace, cdName, scheme).Options(
		testcs.WithFirstSuccessTime(time.Now().Add(-10 * time.Hour)),
	)

	tests := []struct {
		name                   string
		cd                     *hivev1.ClusterDeployment
		cs                     *hiveintv1alpha1.ClusterSync
		hibernationUnsupported bool
		setupActuator          func(actuator *mock.MockHibernationActuator)
		setupCSRHelper         func(helper *mock.MockcsrHelper)
		setupRemote            func(builder *remoteclientmock.MockBuilder)
		validate               func(t *testing.T, cd *hivev1.ClusterDeployment)
		expectError            bool
		expectRequeueAfter     time.Duration
	}{
		{
			name: "cluster deleted with finalizer",
			// Note: For the time being, tests which add a deletion timestamp require a finalizer in order to behave as expected,
			// due to the following issue: https://github.com/kubernetes-sigs/controller-runtime/issues/2184
			cd: cdBuilder.GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(finalizer)).Options(o.shouldHibernate).Build(),
			cs: csBuilder.Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				if assert.NotNil(t, cond) {
					assert.Equal(t, corev1.ConditionUnknown, cond.Status)
					assert.Equal(t, hivev1.HibernatingReasonClusterDeploymentDeleted, cond.Reason)
				}
				if assert.NotNil(t, runCond) {
					assert.Equal(t, corev1.ConditionUnknown, runCond.Status)
					assert.Equal(t, hivev1.ReadyReasonClusterDeploymentDeleted, runCond.Reason)
				}
				assert.Equal(t, hivev1.ClusterPowerStateUnknown, string(cd.Status.PowerState))
			},
		},
		{
			name: "cluster deleted with finalizer, conditions initialized",
			cd: cdBuilder.GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(finalizer)).Options(o.shouldHibernate,
				testcd.WithPowerState(hivev1.ClusterPowerStateRunning),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ClusterHibernatingCondition,
					Status: corev1.ConditionFalse,
				}),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ClusterDeploymentConditionType(hivev1.ClusterRunningCondition),
					Status: corev1.ConditionTrue,
				})).Build(),
			cs: csBuilder.Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				if assert.NotNil(t, cond) {
					assert.Equal(t, corev1.ConditionUnknown, cond.Status)
					assert.Equal(t, hivev1.HibernatingReasonClusterDeploymentDeleted, cond.Reason)
				}
				if assert.NotNil(t, runCond) {
					assert.Equal(t, corev1.ConditionUnknown, runCond.Status)
					assert.Equal(t, hivev1.ReadyReasonClusterDeploymentDeleted, runCond.Reason)
				}
				assert.Equal(t, hivev1.ClusterPowerStateUnknown, string(cd.Status.PowerState))
			},
		},
		{
			name: "hibernation and running condition initialized",
			cd:   cdBuilder.Options(o.notInstalled, o.shouldHibernate).Build(),
			cs:   csBuilder.Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionUnknown, cond.Status)
				assert.Equal(t, hivev1.InitializedConditionReason, cond.Reason)
				assert.Equal(t, corev1.ConditionUnknown, runCond.Status)
				assert.Equal(t, hivev1.InitializedConditionReason, runCond.Reason)
			},
		},
		{
			name:                   "do not hibernate unsupported clusters",
			cd:                     cdBuilder.Build(),
			hibernationUnsupported: true,
			cs:                     csBuilder.Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, _ := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonUnsupported, cond.Reason)
			},
		},
		{
			name: "set powerstate for unsupported clusters",
			cd: cdBuilder.Options(
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:    hivev1.ClusterHibernatingCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.HibernatingReasonUnsupported,
					Message: "Unsupported platform: no actuator to handle it"})).Build(),
			hibernationUnsupported: true,
			cs:                     csBuilder.Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, hivev1.HibernatingReasonUnsupported, cond.Reason)
				require.NotNil(t, runCond)
				assert.Equal(t, hivev1.ReadyReasonRunning, runCond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateRunning, cd.Status.PowerState)
			},
		},
		{
			name:                   "start hibernating, unsupported cluster",
			cd:                     cdBuilder.Options(o.shouldHibernate).Build(),
			hibernationUnsupported: true,
			cs:                     csBuilder.Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, _ := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonUnsupported, cond.Reason)
			},
		},
		{
			name: "powerstate-paused annotation won't hibernate",
			cd:   cdBuilder.Options(o.shouldHibernate, testcd.WithAnnotation(constants.PowerStatePauseAnnotation, "true")).Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				assert.Equal(t, hivev1.ClusterPowerStateUnknown, string(cd.Status.PowerState), "unexpected Status.PowerState")
				hc, rc := getHibernatingAndRunningConditions(cd)
				for _, cond := range []*hivev1.ClusterDeploymentCondition{hc, rc} {
					assert.Equal(t, corev1.ConditionUnknown, cond.Status, "unexpected status for the %s condition", cond.Type)
					assert.Equal(t, "PowerStatePaused", cond.Reason, "unexpected reason for the %s condition", cond.Type)
					assert.Equal(t, "the powerstate-pause annotation is set", cond.Message, "unexpected message for the %s condition", cond.Type)
				}
			},
		},
		{
			name: "powerstate-paused annotation won't run",
			cd:   cdBuilder.Options(o.shouldRun, testcd.WithAnnotation(constants.PowerStatePauseAnnotation, "true")).Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				assert.Equal(t, hivev1.ClusterPowerStateUnknown, string(cd.Status.PowerState), "unexpected Status.PowerState")
				hc, rc := getHibernatingAndRunningConditions(cd)
				for _, cond := range []*hivev1.ClusterDeploymentCondition{hc, rc} {
					assert.Equal(t, corev1.ConditionUnknown, cond.Status, "unexpected status for the %s condition", cond.Type)
					assert.Equal(t, "PowerStatePaused", cond.Reason, "unexpected reason for the %s condition", cond.Type)
					assert.Equal(t, "the powerstate-pause annotation is set", cond.Message, "unexpected message for the %s condition", cond.Type)
				}
			},
		},
		{
			name: "powerstate-paused takes precedence over unsupported",
			cd: cdBuilder.Options(
				o.shouldRun,
				testcd.WithAnnotation(constants.PowerStatePauseAnnotation, "true"),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:    hivev1.ClusterHibernatingCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.HibernatingReasonUnsupported,
					Message: "Unsupported platform: no actuator to handle it",
				})).Build(),
			hibernationUnsupported: true,
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				assert.Equal(t, hivev1.ClusterPowerStateUnknown, string(cd.Status.PowerState), "unexpected Status.PowerState")
				hc, rc := getHibernatingAndRunningConditions(cd)
				for _, cond := range []*hivev1.ClusterDeploymentCondition{hc, rc} {
					assert.Equal(t, corev1.ConditionUnknown, cond.Status, "unexpected status for the %s condition", cond.Type)
					assert.Equal(t, "PowerStatePaused", cond.Reason, "unexpected reason for the %s condition", cond.Type)
					assert.Equal(t, "the powerstate-pause annotation is set", cond.Message, "unexpected message for the %s condition", cond.Type)
				}
			},
		},
		{
			name: "unpause start hibernating",
			cd: cdBuilder.Options(
				o.shouldHibernate,
				// Simulate a previous reconcile having set the status for powerstate-paused,
				// but now the annotation is non-true-ish.
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateUnknown),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:    hivev1.ClusterHibernatingCondition,
					Status:  corev1.ConditionUnknown,
					Reason:  hivev1.HibernatingReasonPowerStatePaused,
					Message: "the powerstate-pause annotation is set",
				}),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:    hivev1.ClusterReadyCondition,
					Status:  corev1.ConditionUnknown,
					Reason:  hivev1.ReadyReasonPowerStatePaused,
					Message: "the powerstate-pause annotation is set",
				}),
				testcd.WithAnnotation(constants.PowerStatePauseAnnotation, "false"),
			).Build(),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().StopMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonStopping, cond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateStopping, cd.Status.PowerState)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonStoppingOrHibernating, runCond.Reason)
			},
		},
		{
			name: "unpause start resuming",
			cd: cdBuilder.Options(
				o.hibernating,
				o.shouldRun,
				// Simulate a previous reconcile having set the status for powerstate-paused,
				// but now the annotation is non-true-ish.
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateUnknown),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:    hivev1.ClusterHibernatingCondition,
					Status:  corev1.ConditionUnknown,
					Reason:  hivev1.HibernatingReasonPowerStatePaused,
					Message: "the powerstate-pause annotation is set",
				}),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:    hivev1.ClusterReadyCondition,
					Status:  corev1.ConditionUnknown,
					Reason:  hivev1.ReadyReasonPowerStatePaused,
					Message: "the powerstate-pause annotation is set",
				}),
				testcd.WithAnnotation(constants.PowerStatePauseAnnotation, "false"),
			).Build(),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().StartMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonStartingMachines, runCond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateStartingMachines, cd.Status.PowerState)
			},
		},
		{
			name:        "clustersync not yet created",
			cd:          cdBuilder.Options(o.shouldHibernate).Build(),
			expectError: true,
		},
		{
			name: "start hibernating, no syncsets",
			cd:   cdBuilder.Options(o.shouldHibernate).Build(),
			// The clustersync controller creates a ClusterSync even when there are no syncsets
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().StopMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonStopping, cond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateStopping, cd.Status.PowerState)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonStoppingOrHibernating, runCond.Reason)
			},
		},
		{
			name: "start hibernating, syncsets not applied",
			cd:   cdBuilder.Options(o.shouldHibernate, testcd.InstalledTimestamp(time.Now())).Build(),
			cs:   csBuilder.Options(testcs.WithNoFirstSuccessTime()).Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, _ := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonSyncSetsNotApplied, cond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateSyncSetsNotApplied, cd.Status.PowerState)
			},
			expectError:        false,
			expectRequeueAfter: time.Minute * 10,
		},
		{
			name: "clear SyncSetsNotApplied",
			cd: cdBuilder.Options(
				testcd.InstalledTimestamp(time.Now()),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:    hivev1.ClusterHibernatingCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.HibernatingReasonSyncSetsNotApplied,
					Message: "Cluster SyncSets have not been applied",
				}),
			).Build(),
			cs: csBuilder.Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, _ := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, hivev1.HibernatingReasonSyncSetsApplied, cond.Reason)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
			},
		},
		{
			name: "set powerstate for clusters with syncsets just applied",
			cd: cdBuilder.Options(o.shouldRun,
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:    hivev1.ClusterHibernatingCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.HibernatingReasonSyncSetsApplied,
					Message: "SyncSets have been applied"}),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ClusterReadyCondition,
					Status: corev1.ConditionFalse,
					Reason: hivev1.ReadyReasonStartingMachines,
				})).Build(),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil, nil)
			},
			setupRemote: func(builder *remoteclientmock.MockBuilder) {
				objs := []runtime.Object{}
				objs = append(objs, readyNodes()...)
				objs = append(objs, readyClusterOperators()...)
				c := testfake.NewFakeClientBuilder().WithRuntimeObjects(objs...).Build()
				builder.EXPECT().Build().Times(1).Return(c, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				assert.Equal(t, hivev1.ClusterPowerStatePausingForClusterOperatorsToSettle, cd.Status.PowerState)
			},
			expectRequeueAfter: time.Minute * 2,
		},
		{
			name: "proceed to pausing for cluster operators from starting machines",
			cd: cdBuilder.Options(o.shouldRun).Build(
				testcd.WithCondition(hibernatingCondition(corev1.ConditionFalse,
					hivev1.HibernatingReasonResumingOrRunning, 6*time.Hour)),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse,
					hivev1.ReadyReasonStartingMachines, 1*time.Hour))),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil, nil)
			},
			setupRemote: func(builder *remoteclientmock.MockBuilder) {
				objs := []runtime.Object{}
				objs = append(objs, readyNodes()...)
				c := testfake.NewFakeClientBuilder().WithRuntimeObjects(objs...).Build()
				builder.EXPECT().Build().Times(1).Return(c, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				assert.Equal(t, hivev1.ClusterPowerStatePausingForClusterOperatorsToSettle, cd.Status.PowerState)
			},
			expectRequeueAfter: time.Minute * 2,
		},
		{
			name: "proceed to pausing for cluster operators from waiting for machines",
			cd: cdBuilder.Options(o.shouldRun).Build(
				testcd.WithCondition(hibernatingCondition(corev1.ConditionFalse,
					hivev1.HibernatingReasonResumingOrRunning, 6*time.Hour)),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse,
					hivev1.ReadyReasonWaitingForMachines, 1*time.Hour))),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil, nil)
			},
			setupRemote: func(builder *remoteclientmock.MockBuilder) {
				objs := []runtime.Object{}
				objs = append(objs, readyNodes()...)
				c := testfake.NewFakeClientBuilder().WithRuntimeObjects(objs...).Build()
				builder.EXPECT().Build().Times(1).Return(c, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				assert.Equal(t, hivev1.ClusterPowerStatePausingForClusterOperatorsToSettle, cd.Status.PowerState)
			},
			expectRequeueAfter: time.Minute * 2,
		},
		{
			name: "proceed to pausing for cluster operators from waiting for nodes",
			cd: cdBuilder.Options(o.shouldRun).Build(
				testcd.WithCondition(hibernatingCondition(corev1.ConditionFalse,
					hivev1.HibernatingReasonResumingOrRunning, 6*time.Hour)),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse,
					hivev1.ReadyReasonWaitingForNodes, 1*time.Hour))),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil, nil)
			},
			setupRemote: func(builder *remoteclientmock.MockBuilder) {
				objs := []runtime.Object{}
				objs = append(objs, readyNodes()...)
				c := testfake.NewFakeClientBuilder().WithRuntimeObjects(objs...).Build()
				builder.EXPECT().Build().Times(1).Return(c, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				assert.Equal(t, hivev1.ClusterPowerStatePausingForClusterOperatorsToSettle, cd.Status.PowerState)
			},
			expectRequeueAfter: time.Minute * 2,
		},
		{
			name: "start hibernating, syncsets not applied but 10 minutes have passed since cd install",
			cd:   cdBuilder.Options(o.shouldHibernate, testcd.InstalledTimestamp(time.Now().Add(-15*time.Minute))).Build(),
			cs:   csBuilder.Options(testcs.WithNoFirstSuccessTime()).Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().StopMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonStopping, cond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateStopping, cd.Status.PowerState)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonStoppingOrHibernating, runCond.Reason)
			},
		},
		{
			name: "start hibernating",
			cd:   cdBuilder.Options(o.shouldHibernate).Build(),
			cs:   csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().StopMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonStopping, cond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateStopping, cd.Status.PowerState)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonStoppingOrHibernating, runCond.Reason)
			},
		},
		{
			name: "fail to stop machines",
			cd:   cdBuilder.Options(o.shouldHibernate).Build(),
			cs:   csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().StopMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(fmt.Errorf("error"))
			},
			expectError: true,
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonFailedToStop, cond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateFailedToStop, cd.Status.PowerState)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonStoppingOrHibernating, runCond.Reason)
			},
		},
		{
			name: "stopping, machines have stopped",
			cd: cdBuilder.Options(o.shouldHibernate, o.stopping,
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ClusterReadyCondition,
					Status: corev1.ConditionFalse,
					Reason: hivev1.ReadyReasonStoppingOrHibernating,
				})).Build(),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().MachinesStopped(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionTrue, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonHibernating, cond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateHibernating, cd.Status.PowerState)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonStoppingOrHibernating, runCond.Reason)
			},
		},
		{
			name: "stopping, machines have not stopped",
			cd:   cdBuilder.Options(o.shouldHibernate, o.stopping).Build(),
			cs:   csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().StopMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
				actuator.EXPECT().MachinesStopped(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
					Return(false, []string{"running-1", "pending-1", "stopping-1"}, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, _ := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonWaitingForMachinesToStop, cond.Reason)
				assert.Equal(t, "Stopping cluster machines. Some machines have not yet stopped: pending-1,running-1,stopping-1", cond.Message)
				assert.Equal(t, hivev1.ClusterPowerStateWaitingForMachinesToStop, cd.Status.PowerState)
			},
			expectRequeueAfter: time.Minute * 1,
		},
		{
			name: "stopping after MachinesFailedToStart",
			cd: cdBuilder.Options(o.shouldHibernate).Build(
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ClusterReadyCondition,
					Status: corev1.ConditionFalse,
					Reason: hivev1.ReadyReasonFailedToStartMachines,
				},
				)),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				// Ensure we try to stop machines in this state (bugfix)
				actuator.EXPECT().StopMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonStopping, cond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateStopping, cd.Status.PowerState)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonStoppingOrHibernating, runCond.Reason)
			},
		},
		{
			name: "start resuming",
			cd:   cdBuilder.Options(o.hibernating, o.shouldRun).Build(),
			cs:   csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().StartMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonStartingMachines, runCond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateStartingMachines, cd.Status.PowerState)
			},
		},
		{
			name: "resuming machines failed to start",
			cd:   cdBuilder.Options(o.hibernating).Build(),
			cs:   csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().StartMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(fmt.Errorf("error"))
			},
			expectError: true,
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonFailedToStartMachines, runCond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateFailedToStartMachines, cd.Status.PowerState)
			},
		},
		{
			name: "attempt to hibernate after previous failure",
			cd: cdBuilder.Options(o.shouldHibernate).Build(
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ClusterReadyCondition,
					Status: corev1.ConditionFalse,
					Reason: hivev1.ReadyReasonFailedToStartMachines,
				}),
				testcd.WithCondition(readyCondition(corev1.ConditionTrue, hivev1.ReadyReasonRunning, 6*time.Hour))),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().StopMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonStopping, cond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateStopping, cd.Status.PowerState)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonStoppingOrHibernating, runCond.Reason)
			},
		},
		{
			name: "resuming, machines have not started",
			cd: cdBuilder.Options(o.shouldRun).Build(
				testcd.WithCondition(hibernatingCondition(corev1.ConditionFalse, hivev1.HibernatingReasonResumingOrRunning, 6*time.Hour)),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse, hivev1.ReadyReasonStartingMachines, 1*time.Hour))),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().StartMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
					Return(false, []string{"stopped-1", "pending-1"}, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonWaitingForMachines, runCond.Reason)
				assert.Equal(t, "Waiting for cluster machines to start. Some machines are not yet running: pending-1,stopped-1 (step 1/4)", runCond.Message)
				assert.Equal(t, hivev1.ClusterPowerStateWaitingForMachines, cd.Status.PowerState)
			},
			expectRequeueAfter: time.Minute * 1,
		},
		{
			name: "resuming unready node",
			cd: cdBuilder.Options().Build(
				testcd.WithCondition(hibernatingCondition(corev1.ConditionFalse, hivev1.HibernatingReasonResumingOrRunning, 6*time.Hour)),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse, "unused", 6*time.Hour))),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil, nil)
			},
			setupRemote: func(builder *remoteclientmock.MockBuilder) {
				fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(unreadyNode()...).Build()
				fakeKubeClient := fakekubeclient.NewSimpleClientset()
				builder.EXPECT().Build().Times(1).Return(fakeClient, nil)
				builder.EXPECT().BuildKubeClient().Times(1).Return(fakeKubeClient, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonWaitingForNodes, runCond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateWaitingForNodes, cd.Status.PowerState)
			},
			expectRequeueAfter: time.Second * 30,
		},
		{
			name: "resuming pending csrs",
			cd: cdBuilder.Options().Build(
				testcd.WithCondition(hibernatingCondition(corev1.ConditionFalse, hivev1.HibernatingReasonResumingOrRunning, 6*time.Hour)),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse, "unused", 6*time.Hour))),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil, nil)
			},
			setupRemote: func(builder *remoteclientmock.MockBuilder) {
				fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(unreadyNode()...).Build()
				fakeKubeClient := fakekubeclient.NewSimpleClientset(csrs()...)
				builder.EXPECT().Build().Times(1).Return(fakeClient, nil)
				builder.EXPECT().BuildKubeClient().Times(1).Return(fakeKubeClient, nil)
			},
			setupCSRHelper: func(helper *mock.MockcsrHelper) {
				count := len(csrs())
				helper.EXPECT().IsApproved(gomock.Any()).Times(count).Return(false)
				helper.EXPECT().Parse(gomock.Any()).Times(count).Return(nil, nil)
				helper.EXPECT().Authorize(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(count).Return(nil)
				helper.EXPECT().Approve(gomock.Any(), gomock.Any()).Times(count).Return(nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonWaitingForNodes, runCond.Reason)
			},
			expectRequeueAfter: time.Second * 30,
		},
		{
			name: "resuming nodes ready pause for clusteroperators to start and settle",
			cd: cdBuilder.Options().Build(
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:               hivev1.ClusterHibernatingCondition,
					Status:             corev1.ConditionFalse,
					Reason:             hivev1.HibernatingReasonResumingOrRunning,
					LastProbeTime:      metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					LastTransitionTime: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				}),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:               hivev1.ClusterReadyCondition,
					Status:             corev1.ConditionFalse,
					Reason:             hivev1.ReadyReasonWaitingForNodes,
					LastProbeTime:      metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
					LastTransitionTime: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				}),
			),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil, nil)
			},
			setupRemote: func(builder *remoteclientmock.MockBuilder) {
				objs := []runtime.Object{}
				objs = append(objs, readyNodes()...)
				objs = append(objs, readyClusterOperators()...)
				c := testfake.NewFakeClientBuilder().WithRuntimeObjects(objs...).Build()
				builder.EXPECT().Build().Times(1).Return(c, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonPausingForClusterOperatorsToSettle, runCond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStatePausingForClusterOperatorsToSettle, cd.Status.PowerState)
			},
			expectRequeueAfter: time.Minute * 2,
		},
		{
			name: "resuming continue to pause for clusteroperators to start and settle",
			cd: cdBuilder.Options().Build(
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:               hivev1.ClusterHibernatingCondition,
					Status:             corev1.ConditionFalse,
					Reason:             hivev1.HibernatingReasonResumingOrRunning,
					LastProbeTime:      metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					LastTransitionTime: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				}),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:               hivev1.ClusterReadyCondition,
					Status:             corev1.ConditionFalse,
					Reason:             hivev1.ReadyReasonPausingForClusterOperatorsToSettle,
					LastProbeTime:      metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
					LastTransitionTime: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				}),
			),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil, nil)
			},
			setupRemote: func(builder *remoteclientmock.MockBuilder) {
				objs := []runtime.Object{}
				objs = append(objs, readyNodes()...)
				objs = append(objs, degradedClusterOperators()...)
				c := testfake.NewFakeClientBuilder().WithRuntimeObjects(objs...).Build()
				builder.EXPECT().Build().Times(1).Return(c, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonWaitingForClusterOperators, runCond.Reason)
			},
			expectRequeueAfter: time.Second * 30,
		},
		{
			name: "resuming clusteroperators not ready",
			cd: cdBuilder.Options().Build(
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:               hivev1.ClusterHibernatingCondition,
					Status:             corev1.ConditionFalse,
					Reason:             hivev1.HibernatingReasonResumingOrRunning,
					LastProbeTime:      metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					LastTransitionTime: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				}),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:               hivev1.ClusterReadyCondition,
					Status:             corev1.ConditionFalse,
					Reason:             hivev1.ReadyReasonPausingForClusterOperatorsToSettle,
					LastProbeTime:      metav1.Time{Time: time.Now().Add(-6 * time.Minute)},
					LastTransitionTime: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				}),
			),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil, nil)
			},
			setupRemote: func(builder *remoteclientmock.MockBuilder) {
				objs := []runtime.Object{}
				objs = append(objs, readyNodes()...)
				objs = append(objs, degradedClusterOperators()...)
				c := testfake.NewFakeClientBuilder().WithRuntimeObjects(objs...).Build()
				builder.EXPECT().Build().Times(1).Return(c, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonWaitingForClusterOperators, runCond.Reason)
			},
			expectRequeueAfter: time.Second * 30,
		},
		{
			name: "resume skips cluster operators",
			cd: cdBuilder.Options().Build(
				testcd.WithLabel(constants.ResumeSkipsClusterOperatorsLabel, "true"),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:               hivev1.ClusterHibernatingCondition,
					Status:             corev1.ConditionFalse,
					Reason:             hivev1.HibernatingReasonResumingOrRunning,
					LastProbeTime:      metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					LastTransitionTime: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				}),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:               hivev1.ClusterReadyCondition,
					Status:             corev1.ConditionFalse,
					Reason:             hivev1.ReadyReasonWaitingForNodes,
					LastProbeTime:      metav1.Time{Time: time.Now().Add(-6 * time.Minute)},
					LastTransitionTime: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				}),
			),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil, nil)
			},
			setupRemote: func(builder *remoteclientmock.MockBuilder) {
				objs := []runtime.Object{}
				objs = append(objs, readyNodes()...)
				// Prove we're skipping these
				objs = append(objs, degradedClusterOperators()...)
				c := testfake.NewFakeClientBuilder().WithRuntimeObjects(objs...).Build()
				builder.EXPECT().Build().Times(1).Return(c, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionTrue, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonRunning, runCond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateRunning, cd.Status.PowerState)
			},
		},
		{
			name: "resuming everything ready",
			cd: cdBuilder.Options().Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:               hivev1.ClusterHibernatingCondition,
				Status:             corev1.ConditionFalse,
				Reason:             hivev1.HibernatingReasonResumingOrRunning,
				LastProbeTime:      metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				LastTransitionTime: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
			}),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:               hivev1.ClusterReadyCondition,
					Status:             corev1.ConditionFalse,
					Reason:             hivev1.ReadyReasonPausingForClusterOperatorsToSettle,
					LastProbeTime:      metav1.Time{Time: time.Now().Add(-6 * time.Minute)},
					LastTransitionTime: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				}),
			),
			cs: csBuilder.Build(),
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil, nil)
			},
			setupRemote: func(builder *remoteclientmock.MockBuilder) {
				objs := []runtime.Object{}
				objs = append(objs, readyNodes()...)
				objs = append(objs, readyClusterOperators()...)
				c := testfake.NewFakeClientBuilder().WithRuntimeObjects(objs...).Build()
				builder.EXPECT().Build().Times(1).Return(c, nil)
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionTrue, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonRunning, runCond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateRunning, cd.Status.PowerState)
			},
		},
		{
			name: "previously unsupported hibernation, now supported",
			cd: cdBuilder.Options(o.unsupported,
				testcd.WithHibernateAfter(8*time.Hour)).Build(),
			cs: csBuilder.Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, _ := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				assert.Equal(t, "Hibernation capable", cond.Message)
			},
		},
		{
			name: "hibernate fake cluster",
			cd: cdBuilder.Build(
				o.shouldHibernate,
				testcd.InstalledTimestamp(time.Now().Add(-1*time.Hour)),
				testcd.WithAnnotation(constants.HiveFakeClusterAnnotation, "true")),
			cs: csBuilder.Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, hivev1.HibernatingReasonHibernating, cond.Reason)
				assert.Equal(t, corev1.ConditionTrue, cond.Status)
				assert.Equal(t, "Fake cluster is stopped", cond.Message)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionFalse, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonStoppingOrHibernating, runCond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateHibernating, cd.Status.PowerState)
			},
		},
		{
			name: "start hibernated fake cluster",
			cd: cdBuilder.Options(o.hibernating,
				testcd.WithPowerState(hivev1.ClusterPowerStateRunning),
				testcd.WithAnnotation(constants.HiveFakeClusterAnnotation, "true")).Build(),
			cs: csBuilder.Build(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond, runCond := getHibernatingAndRunningConditions(cd)
				require.NotNil(t, cond)
				assert.Equal(t, hivev1.HibernatingReasonResumingOrRunning, cond.Reason)
				assert.Equal(t, corev1.ConditionFalse, cond.Status)
				require.NotNil(t, runCond)
				assert.Equal(t, corev1.ConditionTrue, runCond.Status)
				assert.Equal(t, hivev1.ReadyReasonRunning, runCond.Reason)
				assert.Equal(t, hivev1.ClusterPowerStateRunning, cd.Status.PowerState)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockActuator := mock.NewMockHibernationActuator(ctrl)
			mockActuator.EXPECT().CanHandle(gomock.Any()).AnyTimes().Return(!test.hibernationUnsupported)
			if test.setupActuator != nil {
				test.setupActuator(mockActuator)
			}
			mockBuilder := remoteclientmock.NewMockBuilder(ctrl)
			if test.setupRemote != nil {
				test.setupRemote(mockBuilder)
			}
			mockCSRHelper := mock.NewMockcsrHelper(ctrl)
			if test.setupCSRHelper != nil {
				test.setupCSRHelper(mockCSRHelper)
			}
			actuators = []HibernationActuator{mockActuator}
			var c client.Client
			if test.cs != nil {
				c = testfake.NewFakeClientBuilder().WithRuntimeObjects(test.cd, test.cs).Build()
			} else {
				c = testfake.NewFakeClientBuilder().WithRuntimeObjects(test.cd).Build()
			}

			reconciler := hibernationReconciler{
				Client: c,
				logger: log.WithField("controller", "hibernation"),
				remoteClientBuilder: func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
					return mockBuilder
				},
				csrUtil: mockCSRHelper,
			}
			result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: namespace, Name: cdName},
			})

			// Need to do fuzzy requeue after matching
			if test.expectRequeueAfter == 0 {
				assert.Zero(t, result.RequeueAfter)
			} else {
				assert.GreaterOrEqual(t, result.RequeueAfter.Seconds(), (test.expectRequeueAfter - 10*time.Second).Seconds(), "requeue after too small")
				assert.LessOrEqual(t, result.RequeueAfter.Seconds(), (test.expectRequeueAfter + 10*time.Second).Seconds(), "request after too large")
			}

			if test.expectError {
				assert.Error(t, err, "expected error from reconcile")
			} else {
				assert.NoError(t, err, "expected no error from reconcile")
			}
			if test.validate != nil {
				cd := &hivev1.ClusterDeployment{}
				err := c.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: cdName}, cd)
				require.Nil(t, err)
				test.validate(t, cd)
			}
		})
	}

}

func TestHibernateAfter(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	scheme := scheme.GetScheme()

	cdBuilder := testcd.FullBuilder(namespace, cdName, scheme).Options(
		testcd.Installed(),
		testcd.WithClusterVersion("4.4.9"),
	)
	o := clusterDeploymentOptions{}
	csBuilder := testcs.FullBuilder(namespace, cdName, scheme).Options(
		testcs.WithFirstSuccessTime(time.Now().Add(-10 * time.Hour)),
	)

	tests := []struct {
		name                   string
		setupActuator          func(actuator *mock.MockHibernationActuator)
		cd                     *hivev1.ClusterDeployment
		cs                     *hiveintv1alpha1.ClusterSync
		hibernationUnsupported bool

		expectError             bool
		expectRequeueAfter      time.Duration
		expectedPowerState      hivev1.ClusterPowerState
		expectedConditionReason string
	}{
		{
			name: "cluster due for hibernate no condition", // cluster that has never been hibernated and thus has no running condition
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(8*time.Hour),
				testcd.InstalledTimestamp(time.Now().Add(-10*time.Hour))),
			cs:                 csBuilder.Build(),
			expectedPowerState: hivev1.ClusterPowerStateHibernating,
		},
		{
			name: "cluster due for hibernate but unsupported",
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(8*time.Hour),
				testcd.InstalledTimestamp(time.Now().Add(-10*time.Hour))),
			hibernationUnsupported:  true,
			cs:                      csBuilder.Build(),
			expectedPowerState:      "",
			expectedConditionReason: hivev1.HibernatingReasonUnsupported,
		},
		{
			name: "cluster not yet due for hibernate older version", // cluster that has never been hibernated and thus has no running condition
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(8*time.Hour),
				testcd.InstalledTimestamp(time.Now().Add(-3*time.Hour))),
			hibernationUnsupported: true,
			cs: csBuilder.Options(
				testcs.WithFirstSuccessTime(time.Now().Add(-3 * time.Hour)),
			).Build(),
			expectedPowerState:      "",
			expectedConditionReason: hivev1.HibernatingReasonUnsupported,
		},
		{
			name: "cluster not yet due for hibernate no running condition", // cluster that has never been hibernated
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(12*time.Hour),
				testcd.WithPowerState(hivev1.ClusterPowerStateRunning),
				testcd.InstalledTimestamp(time.Now().Add(-10*time.Hour))),
			cs:                 csBuilder.Build(),
			expectRequeueAfter: 2 * time.Hour,
			expectedPowerState: hivev1.ClusterPowerStateRunning,
		},
		{
			name: "cluster with running condition due for hibernate",
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(8*time.Hour),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse, hivev1.ReadyReasonRunning, 9*time.Hour)),
				testcd.InstalledTimestamp(time.Now().Add(-10*time.Hour))),
			cs:                 csBuilder.Build(),
			expectedPowerState: hivev1.ClusterPowerStateHibernating,
		},
		{
			name: "cluster with running condition not due for hibernate",
			cd: cdBuilder.Build(
				testcd.WithCondition(hibernatingCondition(corev1.ConditionFalse, hivev1.HibernatingReasonResumingOrRunning, 6*time.Hour)),
				testcd.WithCondition(readyCondition(corev1.ConditionTrue, hivev1.ReadyReasonRunning, 6*time.Hour)),
				testcd.WithHibernateAfter(20*time.Hour),
				testcd.InstalledTimestamp(time.Now().Add(-10*time.Hour))),
			cs:                 csBuilder.Build(),
			expectRequeueAfter: 14 * time.Hour,
			expectedPowerState: "",
		},
		{
			name: "cluster waking from hibernate",
			setupActuator: func(actuator *mock.MockHibernationActuator) {
				actuator.EXPECT().StartMachines(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
				actuator.EXPECT().MachinesRunning(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).
					Return(false, []string{"no-running-1", "no-running-2"}, nil)
			},
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(8*time.Hour),
				testcd.InstalledTimestamp(time.Now().Add(-10*time.Hour)),
				testcd.WithCondition(hibernatingCondition(corev1.ConditionFalse, hivev1.HibernatingReasonResumingOrRunning, 8*time.Hour)),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse, hivev1.ReadyReasonWaitingForNodes, 8*time.Hour)),
				o.shouldRun),
			cs:                 csBuilder.Build(),
			expectedPowerState: hivev1.ClusterPowerStateRunning,
			expectRequeueAfter: stateCheckInterval,
		},
		{
			name: "cluster due for hibernate, no syncsets",
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(8*time.Minute),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse, hivev1.ReadyReasonRunning, 8*time.Minute)),
				testcd.InstalledTimestamp(time.Now().Add(-8*time.Minute))),
			// The clustersync controller creates an empty ClusterSync even when there are no syncsets.
			cs:                 csBuilder.Build(),
			expectedPowerState: hivev1.ClusterPowerStateHibernating,
		},
		{
			name: "cluster due for hibernate but syncsets not applied",
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(8*time.Minute),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse, hivev1.ReadyReasonRunning, 8*time.Minute)),
				testcd.InstalledTimestamp(time.Now().Add(-8*time.Minute))),
			cs: csBuilder.Options(
				testcs.WithNoFirstSuccessTime(),
			).Build(),
			expectError:        false,
			expectedPowerState: "",
			expectRequeueAfter: time.Minute * 2,
		},
		{
			name: "cluster due for hibernate, syncsets not applied but 10 minutes have passed since cd install",
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(8*time.Hour),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse, hivev1.ReadyReasonRunning, 9*time.Hour)),
				testcd.InstalledTimestamp(time.Now().Add(-10*time.Hour))),
			cs: csBuilder.Options(
				testcs.WithNoFirstSuccessTime(),
			).Build(),
			expectedPowerState: hivev1.ClusterPowerStateHibernating,
		},
		{
			name: "cluster due for hibernate, syncsets successfully applied",
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(8*time.Hour),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse, hivev1.ReadyReasonRunning, 9*time.Hour)),
				testcd.InstalledTimestamp(time.Now().Add(-10*time.Hour))),
			cs: csBuilder.Options(
				testcs.WithFirstSuccessTime(time.Now()),
			).Build(),
			expectedPowerState: hivev1.ClusterPowerStateHibernating,
		},
		{
			name: "fake cluster due for hibernate but syncsets not applied",
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(8*time.Minute),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse, hivev1.ReadyReasonRunning, 8*time.Minute)),
				testcd.WithAnnotation(constants.HiveFakeClusterAnnotation, "true"),
				testcd.InstalledTimestamp(time.Now().Add(-8*time.Minute))),
			cs: csBuilder.Options(
				testcs.WithNoFirstSuccessTime(),
			).Build(),
			expectError:        false,
			expectedPowerState: "",
			expectRequeueAfter: time.Minute * 2,
		},
		{
			name: "fake cluster due for hibernate, syncsets successfully applied",
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(8*time.Hour),
				testcd.WithCondition(readyCondition(corev1.ConditionFalse, hivev1.ReadyReasonRunning, 9*time.Hour)),
				testcd.WithAnnotation(constants.HiveFakeClusterAnnotation, "true"),
				testcd.InstalledTimestamp(time.Now().Add(-10*time.Hour))),
			cs: csBuilder.Options(
				testcs.WithFirstSuccessTime(time.Now()),
			).Build(),
			expectedPowerState: hivev1.ClusterPowerStateHibernating,
		},
		{
			name: "hibernate fake cluster",
			cd: cdBuilder.Build(
				testcd.WithHibernateAfter(1*time.Hour),
				testcd.InstalledTimestamp(time.Now().Add(-1*time.Hour)),
				testcd.WithAnnotation(constants.HiveFakeClusterAnnotation, "true")),
			cs:                 csBuilder.Build(),
			expectedPowerState: hivev1.ClusterPowerStateHibernating,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockActuator := mock.NewMockHibernationActuator(ctrl)
			mockActuator.EXPECT().CanHandle(gomock.Any()).AnyTimes().Return(!test.hibernationUnsupported)
			if test.setupActuator != nil {
				test.setupActuator(mockActuator)
			}
			mockBuilder := remoteclientmock.NewMockBuilder(ctrl)
			mockCSRHelper := mock.NewMockcsrHelper(ctrl)
			actuators = []HibernationActuator{mockActuator}
			var c client.Client
			if test.cs != nil {
				c = testfake.NewFakeClientBuilder().WithRuntimeObjects(test.cd, test.cs).Build()
			} else {
				c = testfake.NewFakeClientBuilder().WithRuntimeObjects(test.cd).Build()
			}

			reconciler := hibernationReconciler{
				Client: c,
				logger: log.WithField("controller", "hibernation"),
				remoteClientBuilder: func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
					return mockBuilder
				},
				csrUtil: mockCSRHelper,
			}
			result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: namespace, Name: cdName},
			})

			if test.expectError {
				assert.Error(t, err, "expected error from reconcile")
			} else {
				assert.NoError(t, err, "expected no error from reconcile")
			}

			// Need to do fuzzy requeue after matching
			if test.expectRequeueAfter == 0 {
				assert.Zero(t, result.RequeueAfter)
			} else {
				assert.GreaterOrEqual(t, result.RequeueAfter.Seconds(), (test.expectRequeueAfter - 10*time.Second).Seconds(), "requeue after too small")
				assert.LessOrEqual(t, result.RequeueAfter.Seconds(), (test.expectRequeueAfter + 10*time.Second).Seconds(), "request after too large")
			}

			cd := &hivev1.ClusterDeployment{}
			err = c.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: cdName}, cd)
			require.NoError(t, err, "error looking up ClusterDeployment")
			assert.Equal(t, test.expectedPowerState, cd.Spec.PowerState, "unexpected PowerState")

		})
	}
}

func hibernatingCondition(status corev1.ConditionStatus, reason string, lastTransitionAgo time.Duration) hivev1.ClusterDeploymentCondition {
	return hivev1.ClusterDeploymentCondition{
		Type:               hivev1.ClusterHibernatingCondition,
		Status:             status,
		Message:            "unused",
		Reason:             reason,
		LastTransitionTime: metav1.NewTime(time.Now().Add(-lastTransitionAgo)),
	}
}

func readyCondition(status corev1.ConditionStatus, reason string, lastTransitionAgo time.Duration) hivev1.ClusterDeploymentCondition {
	return hivev1.ClusterDeploymentCondition{
		Type:               hivev1.ClusterReadyCondition,
		Status:             status,
		Message:            "unused",
		Reason:             reason,
		LastTransitionTime: metav1.NewTime(time.Now().Add(-lastTransitionAgo)),
	}
}

type clusterDeploymentOptions struct{}

func (*clusterDeploymentOptions) notInstalled(cd *hivev1.ClusterDeployment) {
	cd.Spec.Installed = false
}
func (*clusterDeploymentOptions) shouldHibernate(cd *hivev1.ClusterDeployment) {
	cd.Spec.PowerState = hivev1.ClusterPowerStateHibernating
}
func (*clusterDeploymentOptions) shouldRun(cd *hivev1.ClusterDeployment) {
	cd.Spec.PowerState = hivev1.ClusterPowerStateRunning
}
func (*clusterDeploymentOptions) stopping(cd *hivev1.ClusterDeployment) {
	cd.Status.Conditions = append(cd.Status.Conditions, hivev1.ClusterDeploymentCondition{
		Type:   hivev1.ClusterHibernatingCondition,
		Reason: hivev1.HibernatingReasonStopping,
		Status: corev1.ConditionFalse,
	})
}
func (*clusterDeploymentOptions) hibernating(cd *hivev1.ClusterDeployment) {
	cd.Status.Conditions = append(cd.Status.Conditions, hivev1.ClusterDeploymentCondition{
		Type:   hivev1.ClusterHibernatingCondition,
		Reason: hivev1.HibernatingReasonHibernating,
		Status: corev1.ConditionTrue,
	})
}
func (*clusterDeploymentOptions) unsupported(cd *hivev1.ClusterDeployment) {
	cd.Status.Conditions = append(cd.Status.Conditions, hivev1.ClusterDeploymentCondition{
		Type:   hivev1.ClusterHibernatingCondition,
		Status: corev1.ConditionFalse,
		Reason: hivev1.HibernatingReasonUnsupported,
	})
}

func getHibernatingAndRunningConditions(cd *hivev1.ClusterDeployment) (*hivev1.ClusterDeploymentCondition, *hivev1.ClusterDeploymentCondition) {
	var hibCond *hivev1.ClusterDeploymentCondition
	var runCond *hivev1.ClusterDeploymentCondition
	for i := range cd.Status.Conditions {
		if cd.Status.Conditions[i].Type == hivev1.ClusterHibernatingCondition {
			hibCond = &cd.Status.Conditions[i]
		} else if cd.Status.Conditions[i].Type == hivev1.ClusterReadyCondition {
			runCond = &cd.Status.Conditions[i]
		}
	}
	return hibCond, runCond
}

func readyNodes() []runtime.Object {
	nodes := make([]runtime.Object, 5)
	for i := 0; i < len(nodes); i++ {
		node := &corev1.Node{}
		node.Name = fmt.Sprintf("node-%d", i)
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		}
		nodes[i] = node
	}
	return nodes
}

func unreadyNode() []runtime.Object {
	node := &corev1.Node{}
	node.Name = "unready"
	node.Status.Conditions = []corev1.NodeCondition{
		{
			Type:   corev1.NodeReady,
			Status: corev1.ConditionFalse,
		},
	}
	return append(readyNodes(), node)
}

func readyClusterOperators() []runtime.Object {
	cos := make([]runtime.Object, 5)
	for i := 0; i < len(cos); i++ {
		co := &configv1.ClusterOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("clusteroperator%d", i),
			},
			Status: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:   "Available",
						Status: configv1.ConditionTrue,
					},
					{
						Type:   "Progressing",
						Status: configv1.ConditionFalse,
					},
					{
						Type:   "Degraded",
						Status: configv1.ConditionFalse,
					},
				},
			},
		}
		cos[i] = co
	}
	return cos
}

func degradedClusterOperators() []runtime.Object {
	cos := make([]runtime.Object, 5)
	for i := 0; i < len(cos); i++ {
		co := &configv1.ClusterOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("clusteroperator%d", i),
			},
			Status: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:   "Available",
						Status: configv1.ConditionTrue,
					},
					{
						Type:   "Progressing",
						Status: configv1.ConditionFalse,
					},
					{
						Type:   "Degraded",
						Status: configv1.ConditionTrue,
					},
				},
			},
		}
		cos[i] = co
	}
	return cos
}

func csrs() []runtime.Object {
	result := make([]runtime.Object, 5)
	for i := 0; i < len(result); i++ {
		csr := &certsv1.CertificateSigningRequest{}
		csr.Name = fmt.Sprintf("csr-%d", i)
		result[i] = csr
	}
	return result
}
