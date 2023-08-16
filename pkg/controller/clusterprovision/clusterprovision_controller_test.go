package clusterprovision

import (
	"context"
	"fmt"
	"testing"
	"time"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/metricsconfig"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
	tcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	tcp "github.com/openshift/hive/pkg/test/clusterprovision"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	testjob "github.com/openshift/hive/pkg/test/job"
	"github.com/openshift/hive/pkg/util/scheme"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testDeploymentName    = "test-deployment-name"
	testProvisionName     = "test-provision-name"
	installJobName        = "test-provision-name-provision"
	testNamespace         = "test-namespace"
	controllerUidLabelKey = "controller-uid"
	testControllerUid     = "test-controller-uid"
)

func init() {
	log.SetLevel(log.DebugLevel)
	// While the metrics need not be registered for this test suite, they still need to be defined to avoid panics
	// during the tests
	registerMetrics(&metricsconfig.MetricsConfig{}, log.WithField("controller", "clusterProvision"))
}

func TestClusterProvisionReconcile(t *testing.T) {

	tests := []struct {
		name                  string
		existing              []runtime.Object
		pendingCreation       bool
		expectErr             bool
		expectedStage         hivev1.ClusterProvisionStage
		expectedFailReason    string
		expectNoJob           bool
		expectNoJobReference  bool
		expectPendingCreation bool
		validateRequeueAfter  func(time.Duration, client.Client, *testing.T)
		validate              func(client.Client, *testing.T)
	}{
		{
			name: "create job",
			existing: []runtime.Object{
				testProvision(),
			},
			expectedStage:         hivev1.ClusterProvisionStageInitializing,
			expectNoJobReference:  true,
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				job := getJob(c)

				require.NotNil(t, job, "expected job")
				assert.Equal(t, testProvision().Name, job.Labels[constants.ClusterProvisionNameLabel], "incorrect cluster provision name label")
				assert.Equal(t, constants.JobTypeProvision, job.Labels[constants.JobTypeLabel], "incorrect job type label")
			},
		},
		{
			name: "job not created when pending create",
			existing: []runtime.Object{
				testProvision(),
			},
			pendingCreation:       true,
			expectedStage:         hivev1.ClusterProvisionStageInitializing,
			expectNoJob:           true,
			expectNoJobReference:  true,
			expectPendingCreation: true,
		},
		{
			name: "adopt job",
			existing: []runtime.Object{
				testProvision(),
				testJob(),
			},
			expectedStage: hivev1.ClusterProvisionStageInitializing,
		},
		{
			name: "running job",
			existing: []runtime.Object{
				testProvision(tcp.WithJob(installJobName)),
				testJob(),
				testPod("foo", running()),
			},
			expectedStage: hivev1.ClusterProvisionStageInitializing,
		},
		{
			name: "completed job",
			existing: []runtime.Object{
				testProvision(
					tcp.WithJob(installJobName),
					tcp.WithStage(hivev1.ClusterProvisionStageProvisioning)),
				testJob(completed()),
				testPod("foo", success()),
			},
			expectedStage: hivev1.ClusterProvisionStageComplete,
		},
		{
			name: "completed job while initializing",
			existing: []runtime.Object{
				testProvision(tcp.WithJob(installJobName)),
				testJob(completed()),
				testPod("foo", success()),
			},
			expectedStage:      hivev1.ClusterProvisionStageFailed,
			expectedFailReason: "InitializationNotComplete",
		},
		{
			name: "failed job",
			existing: []runtime.Object{
				testProvision(tcp.WithJob(installJobName)),
				testJob(failedJob()),
				testPod("foo"),
			},
			expectedStage:      hivev1.ClusterProvisionStageFailed,
			expectedFailReason: unknownReason,
		},
		{
			name: "deadline exceeded job",
			existing: []runtime.Object{
				testProvision(tcp.WithJob(installJobName)),
				testJob(deadlineExceededJob()),
				testPod("foo"),
			},
			expectedStage:      hivev1.ClusterProvisionStageFailed,
			expectedFailReason: "AttemptDeadlineExceeded",
		},
		{
			name: "keep job for 24 hours after success",
			existing: []runtime.Object{
				testProvision(
					tcp.WithStage(hivev1.ClusterProvisionStageComplete),
					tcp.WithJob(installJobName),
					tcp.WithCreationTimestamp(time.Now())),
				testJob(),
			},
			validateRequeueAfter: func(requeueAfter time.Duration, c client.Client, t *testing.T) {
				testProvisionCreationTime := getProvision(c).CreationTimestamp.Time
				assert.Less(t, requeueAfter.Nanoseconds(), 24*time.Hour.Nanoseconds(), "unexpected requeue after duration")
				assert.Greater(t, requeueAfter.Nanoseconds(), time.Until(testProvisionCreationTime.Add(24*time.Hour)).Nanoseconds(),
					"unexpected requeue after duration")
			},
			expectedStage: hivev1.ClusterProvisionStageComplete,
		},
		{
			name: "removed job 24 hours after success",
			existing: []runtime.Object{
				testProvision(
					tcp.WithStage(hivev1.ClusterProvisionStageComplete),
					tcp.WithJob(installJobName),
					tcp.WithCreationTimestamp(time.Now().Add(-24*time.Hour))),
				testJob(),
			},
			expectedStage:        hivev1.ClusterProvisionStageComplete,
			expectNoJob:          true,
			expectNoJobReference: true,
		},
		{
			name: "keep job after failure",
			existing: []runtime.Object{
				testProvision(tcp.Failed(), tcp.WithJob(installJobName)),
				testJob(),
			},
			expectedStage: hivev1.ClusterProvisionStageFailed,
		},
		{
			name: "lost job",
			existing: []runtime.Object{
				testProvision(tcp.WithJob(installJobName)),
			},
			expectedStage:      hivev1.ClusterProvisionStageFailed,
			expectedFailReason: "JobNotFound",
			expectNoJob:        true,
		},
		{
			name: "removed job while provisioning",
			existing: []runtime.Object{
				testProvision(tcp.WithStage(hivev1.ClusterProvisionStageProvisioning)),
			},
			expectedStage:        hivev1.ClusterProvisionStageFailed,
			expectedFailReason:   "NoJobReference",
			expectNoJob:          true,
			expectNoJobReference: true,
		},
		{
			name: "removed job after abort",
			existing: []runtime.Object{
				testProvision(tcp.WithJob(installJobName), tcp.WithFailureReason("test-reason")),
			},
			expectedStage:      hivev1.ClusterProvisionStageFailed,
			expectedFailReason: "test-reason",
			expectNoJob:        true,
		},
		{
			name: "no install pod running after starting install job",
			existing: []runtime.Object{
				testProvision(tcp.WithJob(installJobName)),
				testJob(withCreationTimestamp(time.Now().Add(-podStatusCheckDelay))),
			},
			expectedStage: hivev1.ClusterProvisionStageInitializing,
			validate: func(c client.Client, t *testing.T) {
				provision := getProvision(c)
				require.NotNil(t, provision, "could not get ClusterProvision")
				assertConditionStatus(t, provision, hivev1.InstallPodStuckCondition, corev1.ConditionTrue)
				assertConditionReason(t, provision, hivev1.InstallPodStuckCondition, "InstallPodMissing")
			},
			expectErr: true,
		},
		{
			name: "multiple install pods running after starting install job",
			existing: []runtime.Object{
				testProvision(tcp.WithJob(installJobName)),
				testJob(withCreationTimestamp(time.Now().Add(-podStatusCheckDelay))),
				testPod("foo", running()),
				testPod("bar", running()),
			},
			expectedStage: hivev1.ClusterProvisionStageInitializing,
			validate: func(c client.Client, t *testing.T) {
				provision := getProvision(c)
				require.NotNil(t, provision, "could not get ClusterProvision")
				assertConditionStatus(t, provision, hivev1.InstallPodStuckCondition, corev1.ConditionTrue)
				assertConditionReason(t, provision, hivev1.InstallPodStuckCondition, "InstallPodMissing")
			},
			expectErr: true,
		},
		{
			name: "install pod is stuck in pending phase",
			existing: []runtime.Object{
				testProvision(tcp.WithJob(installJobName)),
				testJob(withCreationTimestamp(time.Now().Add(-podStatusCheckDelay))),
				testPod("foo", pending()),
			},
			expectedStage: hivev1.ClusterProvisionStageInitializing,
			validate: func(c client.Client, t *testing.T) {
				provision := getProvision(c)
				require.NotNil(t, provision, "could not get ClusterProvision")
				assertConditionStatus(t, provision, hivev1.InstallPodStuckCondition, corev1.ConditionTrue)
				assertConditionReason(t, provision, hivev1.InstallPodStuckCondition, "PodInPendingPhase")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("controller", "clusterProvision")
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()
			controllerExpectations := controllerutils.NewExpectations(logger)
			rcp := &ReconcileClusterProvision{
				Client:       fakeClient,
				scheme:       scheme,
				logger:       logger,
				expectations: controllerExpectations,
			}

			reconcileRequest := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testProvisionName,
					Namespace: testNamespace,
				},
			}

			if test.pendingCreation {
				controllerExpectations.ExpectCreations(reconcileRequest.String(), 1)
			}

			result, err := rcp.Reconcile(context.TODO(), reconcileRequest)

			if test.validateRequeueAfter != nil {
				test.validateRequeueAfter(result.RequeueAfter, fakeClient, t)
			}

			if test.expectErr {
				assert.Error(t, err, "expected error from reconcile")
			} else {
				assert.NoError(t, err, "expected no error from reconcile")
			}

			provision := getProvision(fakeClient)
			if assert.NotNil(t, provision, "provision lost") {
				assert.Equal(t, string(test.expectedStage), string(provision.Spec.Stage), "unexpected provision stage")
				failedCond := controllerutils.FindCondition(provision.Status.Conditions, hivev1.ClusterProvisionFailedCondition)
				if test.expectedFailReason != "" {
					if assert.NotNil(t, failedCond, "expected to find a Failed condition") {
						assert.Equal(t, test.expectedFailReason, failedCond.Reason, "unexpected fail reason")
					}
				} else {
					assert.Nil(t, failedCond, "expected not to find a Failed condition")
				}
				if test.expectNoJobReference {
					assert.Nil(t, provision.Status.JobRef, "expected no job reference from provision")
				} else {
					if assert.NotNil(t, provision.Status.JobRef, "expected job reference from provision") {
						assert.Equal(t, installJobName, provision.Status.JobRef.Name, "unexpected job name referenced from provision")
					}
				}
			}

			job := getJob(fakeClient)
			if test.expectNoJob {
				assert.Nil(t, job, "expected no job")
			} else {
				assert.NotNil(t, job, "expected job")
			}

			actualPendingCreation := !controllerExpectations.SatisfiedExpectations(reconcileRequest.String())
			assert.Equal(t, test.expectPendingCreation, actualPendingCreation, "unexpected pending creation")

			if test.validate != nil {
				test.validate(fakeClient, t)
			}
		})
	}
}

func testProvision(opts ...tcp.Option) *hivev1.ClusterProvision {
	provision := tcp.
		FullBuilder(testNamespace, testProvisionName).
		Build(tcp.WithClusterDeploymentRef(testDeploymentName))

	for _, o := range opts {
		o(provision)
	}

	return provision
}

func testJob(opts ...testjob.Option) *batchv1.Job {
	provision := testProvision()
	job, err := install.GenerateInstallerJob(provision)
	if err != nil {
		panic("should not error while generating test install job")
	}
	job.Labels[clusterProvisionLabelKey] = provision.Name
	job.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{controllerUidLabelKey: testControllerUid},
	}
	scheme := scheme.GetScheme()
	controllerutil.SetControllerReference(provision, job, scheme)

	for _, o := range opts {
		o(job)
	}

	return job
}

func completed() testjob.Option {
	return func(job *batchv1.Job) {
		job.Status.Conditions = append(job.Status.Conditions,
			batchv1.JobCondition{
				Type:   batchv1.JobComplete,
				Status: corev1.ConditionTrue,
			},
		)
	}
}

func failedJob() testjob.Option {
	return func(job *batchv1.Job) {
		job.Status.Conditions = append(job.Status.Conditions,
			batchv1.JobCondition{
				Type:   batchv1.JobFailed,
				Status: corev1.ConditionTrue,
			},
		)
	}
}
func deadlineExceededJob() testjob.Option {
	return func(job *batchv1.Job) {
		job.Status.Conditions = append(job.Status.Conditions,
			batchv1.JobCondition{
				Type:   batchv1.JobFailed,
				Status: corev1.ConditionTrue,
				Reason: "DeadlineExceeded",
			},
		)
	}
}

func withCreationTimestamp(time time.Time) testjob.Option {
	return testjob.Generic(testgeneric.WithCreationTimestamp(time))
}

func getJob(c client.Client) *batchv1.Job {
	job := &batchv1.Job{}
	err := c.Get(context.TODO(), client.ObjectKey{Name: installJobName, Namespace: testNamespace}, job)
	if err == nil {
		return job
	}
	return nil
}

func getProvision(c client.Client) *hivev1.ClusterProvision {
	provision := &hivev1.ClusterProvision{}
	if err := c.Get(context.TODO(), client.ObjectKey{Name: testProvisionName, Namespace: testNamespace}, provision); err != nil {
		return nil
	}
	return provision
}

type podOption func(*corev1.Pod)

func testPod(nameSuffix string, opts ...podOption) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", testJob().Name, nameSuffix),
			Namespace: testNamespace,
			Labels: map[string]string{
				controllerUidLabelKey: testControllerUid,
			},
		},
	}

	for _, o := range opts {
		o(pod)
	}

	return pod
}

func pending() podOption {
	return func(pod *corev1.Pod) {
		pod.Status.Phase = "Pending"
	}
}

func running() podOption {
	return func(pod *corev1.Pod) {
		pod.Status.Phase = "Running"
	}
}

func success() podOption {
	return func(pod *corev1.Pod) {
		pod.Status.Phase = "Succeeded"
	}
}

func assertConditionStatus(t *testing.T, provision *hivev1.ClusterProvision, condType hivev1.ClusterProvisionConditionType, status corev1.ConditionStatus) {
	for _, cond := range provision.Status.Conditions {
		if cond.Type == condType {
			assert.Equal(t, string(status), string(cond.Status), "condition found with unexpected status")
			return
		}
	}
	t.Errorf("did not find expected condition type: %v", condType)
}

func assertConditionReason(t *testing.T, cd *hivev1.ClusterProvision, condType hivev1.ClusterProvisionConditionType, reason string) {
	for _, cond := range cd.Status.Conditions {
		if cond.Type == condType {
			assert.Equal(t, reason, cond.Reason, "condition found with unexpected reason")
			return
		}
	}
	t.Errorf("did not find expected condition type: %v", condType)
}

func Test_getWorkers(t *testing.T) {
	testScheme := scheme.GetScheme()
	icSecretName := "ic-secret"
	tests := []struct {
		name          string
		installConfig string
		want          string
	}{
		// TODO: Test the path where Get()ting the Secret fails
		{
			name:          "unmarshal failure",
			installConfig: "something that doesn't unmarshal",
			want:          "unknown",
		},
		{
			name: "green path",
			installConfig: `
controlPlane:
  name: master
  replicas: 5
compute:
  - name: worker
    replicas: 3
`,
			want: "3",
		},
		{
			name: "no compute section",
			installConfig: `
controlPlane:
  name: master
  replicas: 5
`,
			want: "0",
		},
		{
			name: "no compute section named 'worker'",
			installConfig: `
controlPlane:
  name: master
  replicas: 5
compute:
  - name: notworker
    replicas: 3
`,
			want: "0",
		},
		{
			name: "no replicas specified",
			installConfig: `
controlPlane:
  name: master
  replicas: 5
compute:
  - name: worker
`,
			want: "0",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("controller", "clusterProvision")
			icSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      icSecretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"install-config.yaml": []byte(test.installConfig),
				},
			}
			// A simple ClusterDeployment that's only used for its namespace and install-config reference
			cd := tcd.FullBuilder(testNamespace, testDeploymentName, testScheme).Build()
			cd.Spec.Provisioning = &hivev1.Provisioning{
				InstallConfigSecretRef: &corev1.LocalObjectReference{
					Name: icSecretName,
				},
			}
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(icSecret).Build()
			rcp := &ReconcileClusterProvision{
				Client: fakeClient,
				logger: logger,
			}

			if got := rcp.getWorkers(*cd); got != test.want {
				t.Errorf("ReconcileClusterProvision.getWorkers() = %v, want %v", got, test.want)
			}
		})
	}
}
