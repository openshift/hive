package clusterprovision

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
)

const (
	testDeploymentName = "test-deployment-name"
	testProvisionName  = "test-provision-name"
	installJobName     = "test-provision-name-provision"
	testNamespace      = "test-namespace"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestClusterProvisionReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	openshiftapiv1.Install(scheme.Scheme)
	routev1.Install(scheme.Scheme)

	tests := []struct {
		name                    string
		existing                []runtime.Object
		pendingCreation         bool
		expectedReconcileResult reconcile.Result
		expectErr               bool
		expectedStage           hivev1.ClusterProvisionStage
		expectedFailReason      string
		expectNoJob             bool
		expectNoJobReference    bool
		expectPendingCreation   bool
		validate                func(client.Client, *testing.T)
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
				testProvision(withJob()),
				testJob(),
			},
			expectedStage: hivev1.ClusterProvisionStageInitializing,
		},
		{
			name: "completed job",
			existing: []runtime.Object{
				testProvision(withJob(), provisioning()),
				testJob(completed()),
			},
			expectedStage: hivev1.ClusterProvisionStageComplete,
		},
		{
			name: "completed job while initializing",
			existing: []runtime.Object{
				testProvision(withJob()),
				testJob(completed()),
			},
			expectedStage:      hivev1.ClusterProvisionStageFailed,
			expectedFailReason: "InitializationNotComplete",
		},
		{
			name: "failed job",
			existing: []runtime.Object{
				testProvision(withJob()),
				testJob(failedJob()),
			},
			expectedStage:      hivev1.ClusterProvisionStageFailed,
			expectedFailReason: unknownReason,
		},
		{
			name: "keep job after success",
			existing: []runtime.Object{
				testProvision(succeeded(), withJob()),
				testJob(),
			},
			expectedStage: hivev1.ClusterProvisionStageComplete,
		},
		{
			name: "keep job after failure",
			existing: []runtime.Object{
				testProvision(failed(), withJob()),
				testJob(),
			},
			expectedStage: hivev1.ClusterProvisionStageFailed,
		},
		{
			name: "lost job",
			existing: []runtime.Object{
				testProvision(withJob()),
			},
			expectedStage:      hivev1.ClusterProvisionStageFailed,
			expectedFailReason: "JobNotFound",
			expectNoJob:        true,
		},
		{
			name: "removed job while provisioning",
			existing: []runtime.Object{
				testProvision(provisioning()),
			},
			expectedStage:        hivev1.ClusterProvisionStageFailed,
			expectedFailReason:   "NoJobReference",
			expectNoJob:          true,
			expectNoJobReference: true,
		},
		{
			name: "removed job after abort",
			existing: []runtime.Object{
				testProvision(withJob(), withFailedCondition("test-reason")),
			},
			expectedStage:      hivev1.ClusterProvisionStageFailed,
			expectedFailReason: "test-reason",
			expectNoJob:        true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("controller", "clusterProvision")
			fakeClient := fake.NewFakeClient(test.existing...)
			controllerExpectations := controllerutils.NewExpectations(logger)
			rcp := &ReconcileClusterProvision{
				Client:       fakeClient,
				scheme:       scheme.Scheme,
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

			result, err := rcp.Reconcile(reconcileRequest)

			assert.Equal(t, test.expectedReconcileResult, result, "unexpected reconcile result")

			if test.expectErr {
				assert.Error(t, err, "expected error from reconcile")
			} else {
				assert.NoError(t, err, "expected no error from reconcile")
			}

			provision := getProvision(fakeClient)
			if assert.NotNil(t, provision, "provision lost") {
				assert.Equal(t, string(test.expectedStage), string(provision.Spec.Stage), "unexpected provision stage")
				failedCond := controllerutils.FindClusterProvisionCondition(provision.Status.Conditions, hivev1.ClusterProvisionFailedCondition)
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

type provisionOption func(*hivev1.ClusterProvision)

func testProvision(opts ...provisionOption) *hivev1.ClusterProvision {
	provision := &hivev1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testProvisionName,
			Namespace: testNamespace,
			Labels: map[string]string{
				constants.ClusterDeploymentNameLabel: testDeploymentName,
			},
		},
		Spec: hivev1.ClusterProvisionSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: testDeploymentName,
			},
			Stage: hivev1.ClusterProvisionStageInitializing,
		},
	}

	for _, o := range opts {
		o(provision)
	}

	return provision
}

func provisioning() provisionOption {
	return func(p *hivev1.ClusterProvision) {
		p.Spec.Stage = hivev1.ClusterProvisionStageProvisioning
	}
}

func succeeded() provisionOption {
	return func(p *hivev1.ClusterProvision) {
		p.Spec.Stage = hivev1.ClusterProvisionStageComplete
	}
}

func failed() provisionOption {
	return func(p *hivev1.ClusterProvision) {
		p.Spec.Stage = hivev1.ClusterProvisionStageFailed
	}
}

func withJob() provisionOption {
	return func(p *hivev1.ClusterProvision) {
		p.Status.JobRef = &corev1.LocalObjectReference{
			Name: installJobName,
		}
	}
}

func withFailedCondition(reason string) provisionOption {
	return func(p *hivev1.ClusterProvision) {
		p.Status.Conditions = append(
			p.Status.Conditions,
			hivev1.ClusterProvisionCondition{
				Type:   hivev1.ClusterProvisionFailedCondition,
				Status: corev1.ConditionTrue,
				Reason: reason,
			},
		)
	}
}

type jobOption func(*batchv1.Job)

func testJob(opts ...jobOption) *batchv1.Job {
	provision := testProvision()
	job, err := install.GenerateInstallerJob(provision)
	if err != nil {
		panic("should not error while generating test install job")
	}
	job.Labels[clusterProvisionLabelKey] = provision.Name

	controllerutil.SetControllerReference(provision, job, scheme.Scheme)

	for _, o := range opts {
		o(job)
	}

	return job
}

func completed() jobOption {
	return func(job *batchv1.Job) {
		job.Status.Conditions = append(job.Status.Conditions,
			batchv1.JobCondition{
				Type:   batchv1.JobComplete,
				Status: corev1.ConditionTrue,
			},
		)
	}
}

func failedJob() jobOption {
	return func(job *batchv1.Job) {
		job.Status.Conditions = append(job.Status.Conditions,
			batchv1.JobCondition{
				Type:   batchv1.JobFailed,
				Status: corev1.ConditionTrue,
			},
		)
	}
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
