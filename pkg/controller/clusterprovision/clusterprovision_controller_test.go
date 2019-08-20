package clusterprovision

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

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
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
)

const (
	testDeploymentName = "test-deployment-name"
	testClusterName    = "test-cluster-name"
	testClusterID      = "test-cluster-id"
	testInfraID        = "test-infra-id"
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
		expectedReconcileResult reconcile.Result
		expectErr               bool
		expectedStage           hivev1.ClusterProvisionStage
		expectedFailReason      string
		expectNoJob             bool
		expectNoJobReference    bool
		validate                func(client.Client, *testing.T)
	}{
		{
			name: "create job",
			existing: []runtime.Object{
				testProvision(),
			},
			expectedStage:        hivev1.ClusterProvisionStageProvisioning,
			expectNoJobReference: true,
		},
		{
			name: "adopt job",
			existing: []runtime.Object{
				testProvision(),
				testJob(),
			},
			expectedStage: hivev1.ClusterProvisionStageProvisioning,
		},
		{
			name: "running job",
			existing: []runtime.Object{
				testProvision(withJob()),
				testJob(),
			},
			expectedStage: hivev1.ClusterProvisionStageProvisioning,
		},
		{
			name: "completed job",
			existing: []runtime.Object{
				testProvision(withJob()),
				testJob(completed()),
			},
			expectedStage: hivev1.ClusterProvisionStageComplete,
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
			name: "lost job while provisioning",
			existing: []runtime.Object{
				testProvision(withJob()),
			},
			expectedStage:      hivev1.ClusterProvisionStageFailed,
			expectedFailReason: "JobNotFound",
			expectNoJob:        true,
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
			fakeClient := fake.NewFakeClient(test.existing...)
			rcp := &ReconcileClusterProvision{
				Client: fakeClient,
				scheme: scheme.Scheme,
			}

			result, err := rcp.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testProvisionName,
					Namespace: testNamespace,
				},
			})

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
					assert.Nil(t, provision.Status.Job, "expected no job reference from provision")
				} else {
					if assert.NotNil(t, provision.Status.Job, "expected job reference from provision") {
						assert.Equal(t, installJobName, provision.Status.Job.Name, "unexpected job name referenced from provision")
					}
				}
			}

			job := getJob(fakeClient)
			if test.expectNoJob {
				assert.Nil(t, job, "expected no job")
			} else {
				assert.NotNil(t, job, "expected job")
			}

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
			ClusterDeployment: corev1.LocalObjectReference{
				Name: testDeploymentName,
			},
			Stage: hivev1.ClusterProvisionStageProvisioning,
		},
	}

	for _, o := range opts {
		o(provision)
	}

	return provision
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
		p.Status.Job = &corev1.LocalObjectReference{
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

func assertConditionStatus(t *testing.T, provision *hivev1.ClusterProvision, condType hivev1.ClusterProvisionConditionType, status corev1.ConditionStatus) {
	found := false
	for _, cond := range provision.Status.Conditions {
		if cond.Type == condType {
			found = true
			assert.Equal(t, status, cond.Status, "condition found with unexpected status")
		}
	}
	assert.True(t, found, "did not find expected condition type: %v", condType)
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
