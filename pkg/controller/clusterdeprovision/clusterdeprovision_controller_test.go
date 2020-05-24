package clusterdeprovision

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
)

const (
	testName      = "deprovision-request"
	testNamespace = "default"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestClusterDeprovisionReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                 string
		deployment           *hivev1.ClusterDeployment
		deprovision          *hivev1.ClusterDeprovision
		existing             []runtime.Object
		validate             func(t *testing.T, c client.Client)
		expectErr            bool
		deprovisionsDisabled bool
	}{
		{
			name: "no-op deleting",
			deprovision: func() *hivev1.ClusterDeprovision {
				req := testClusterDeprovision()
				now := metav1.Now()
				req.DeletionTimestamp = &now
				return req
			}(),
			deployment: testDeletedClusterDeployment(),
			validate: func(t *testing.T, c client.Client) {
				validateNoJobExists(t, c)
			},
		},
		{
			name: "no-op completed",
			deprovision: func() *hivev1.ClusterDeprovision {
				req := testClusterDeprovision()
				req.Status.Completed = true
				return req
			}(),
			deployment: testDeletedClusterDeployment(),
			validate: func(t *testing.T, c client.Client) {
				validateNoJobExists(t, c)
			},
		},
		{
			name:        "no-op if cluster deployment not deleted",
			deprovision: testClusterDeprovision(),
			deployment:  testClusterDeployment(),
			validate: func(t *testing.T, c client.Client) {
				validateNoJobExists(t, c)
			},
			expectErr: true,
		},
		{
			name:        "create uninstall job",
			deprovision: testClusterDeprovision(),
			deployment:  testDeletedClusterDeployment(),
			validate: func(t *testing.T, c client.Client) {
				validateJobExists(t, c)
			},
		},
		{
			name:        "do not create uninstall job when deprovisions are disabled",
			deprovision: testClusterDeprovision(),
			deployment:  testDeletedClusterDeployment(),
			validate: func(t *testing.T, c client.Client) {
				validateNoJobExists(t, c)
			},
			deprovisionsDisabled: true,
		},
		{
			name:        "no-op when job in progress",
			deprovision: testClusterDeprovision(),
			deployment:  testDeletedClusterDeployment(),
			existing: []runtime.Object{
				testUninstallJob(),
			},
			validate: func(t *testing.T, c client.Client) {
				validateNotCompleted(t, c)
			},
		},
		{
			name:        "completed when job is successful",
			deprovision: testClusterDeprovision(),
			deployment:  testDeletedClusterDeployment(),
			existing: []runtime.Object{
				func() runtime.Object {
					job := testUninstallJob()
					job.Status.Conditions = []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					}
					now := metav1.Now()
					job.Status.CompletionTime = &now
					job.Status.StartTime = &now
					return job
				}(),
			},
			validate: func(t *testing.T, c client.Client) {
				validateCompleted(t, c)
			},
		},
		{
			name:        "regenerate job when hash missing",
			deprovision: testClusterDeprovision(),
			deployment:  testDeletedClusterDeployment(),
			existing: []runtime.Object{
				func() *batchv1.Job {
					job := testUninstallJob()
					delete(job.Annotations, jobHashAnnotation)
					return job
				}(),
			},
			validate: func(t *testing.T, c client.Client) {
				validateNoJobExists(t, c)
			},
		},
		{
			name:        "regenerate job when hash changed",
			deprovision: testClusterDeprovision(),
			deployment:  testDeletedClusterDeployment(),
			existing: []runtime.Object{
				func() *batchv1.Job {
					job := testUninstallJob()
					job.Annotations[jobHashAnnotation] = "DIFFERENTHASH"
					return job
				}(),
			},
			validate: func(t *testing.T, c client.Client) {
				validateNoJobExists(t, c)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			if test.deployment != nil {
				// Associate the cluster deployment as the owner of the provision to match real world:
				err := controllerutil.SetControllerReference(test.deployment, test.deprovision, scheme.Scheme)
				if err != nil {
					t.Errorf("unable to set owner reference on deprovision: %v", err)
					return
				}
			}
			existing := append(test.existing, test.deprovision, test.deployment)

			fakeClient := fake.NewFakeClient(existing...)
			r := &ReconcileClusterDeprovision{
				Client:               fakeClient,
				scheme:               scheme.Scheme,
				deprovisionsDisabled: test.deprovisionsDisabled,
			}

			_, err := r.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})

			if test.validate != nil {
				test.validate(t, fakeClient)
			}

			if !test.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func testClusterDeprovision() *hivev1.ClusterDeprovision {
	return &hivev1.ClusterDeprovision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		},
		Spec: hivev1.ClusterDeprovisionSpec{
			InfraID:   "test-infra-id",
			ClusterID: "test-cluster-id",
			Platform: hivev1.ClusterDeprovisionPlatform{
				AWS: &hivev1.AWSClusterDeprovision{
					Region: "us-east-1",
					CredentialsSecretRef: &corev1.LocalObjectReference{
						Name: "aws-creds",
					},
				},
			},
		},
	}
}

func testDeletedClusterDeployment() *hivev1.ClusterDeployment {
	now := metav1.Now()
	cd := testClusterDeployment()
	cd.ObjectMeta.DeletionTimestamp = &now
	return cd
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testName,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			Installed: true,
		},
	}
}

func testUninstallJob() *batchv1.Job {
	uninstallJob, _ := install.GenerateUninstallerJobForDeprovision(testClusterDeprovision())
	hash, err := controllerutils.CalculateJobSpecHash(uninstallJob)
	if err != nil {
		panic("should never get error calculating job spec hash")
	}

	uninstallJob.Annotations[jobHashAnnotation] = hash
	return uninstallJob
}

func validateNoJobExists(t *testing.T, c client.Client) {
	job := &batchv1.Job{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testName + "-uninstall"}, job)
	if err == nil {
		t.Errorf("uninstall job should not exist")
		return
	}
	if !errors.IsNotFound(err) {
		t.Errorf("unexpected error")
	}
}

func validateJobExists(t *testing.T, c client.Client) {
	job := &batchv1.Job{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testName + "-uninstall"}, job)
	if err != nil {
		t.Errorf("cannot get expected job")
	}

	require.NotNil(t, job, "expected job")
	assert.Equal(t, testClusterDeprovision().Name, job.Labels[constants.ClusterDeprovisionNameLabel], "incorrect cluster deprovision name label")
	assert.Equal(t, constants.JobTypeDeprovision, job.Labels[constants.JobTypeLabel], "incorrect job type label")
}

func validateNotCompleted(t *testing.T, c client.Client) {
	req := &hivev1.ClusterDeprovision{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testName}, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if req.Status.Completed {
		t.Errorf("request is not expected to be in completed state")
	}
}

func validateCompleted(t *testing.T, c client.Client) {
	req := &hivev1.ClusterDeprovision{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testName}, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !req.Status.Completed {
		t.Errorf("request is expected to be in completed state")
	}
}
