package clusterdeprovisionrequest

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"

	controllerutils "github.com/openshift/hive/pkg/controller/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/install"
)

const (
	testName      = "deprovision-request"
	testNamespace = "default"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestClusterDeprovisionRequestReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name     string
		existing []runtime.Object
		validate func(t *testing.T, c client.Client)
	}{
		{
			name: "no-op deleting",
			existing: []runtime.Object{
				func() runtime.Object {
					req := testClusterDeprovisionRequest()
					now := metav1.Now()
					req.DeletionTimestamp = &now
					return req
				}(),
			},
			validate: func(t *testing.T, c client.Client) {
				validateNoJobExists(t, c)
			},
		},
		{
			name: "no-op completed",
			existing: []runtime.Object{
				func() runtime.Object {
					req := testClusterDeprovisionRequest()
					req.Status.Completed = true
					return req
				}(),
			},
			validate: func(t *testing.T, c client.Client) {
				validateNoJobExists(t, c)
			},
		},
		{
			name: "create uninstall job",
			existing: []runtime.Object{
				testClusterDeprovisionRequest(),
			},
			validate: func(t *testing.T, c client.Client) {
				validateJobExists(t, c)
			},
		},
		{
			name: "no-op when job in progress",
			existing: []runtime.Object{
				testClusterDeprovisionRequest(),
				testUninstallJob(),
			},
			validate: func(t *testing.T, c client.Client) {
				validateNotCompleted(t, c)
			},
		},
		{
			name: "completed when job is successful",
			existing: []runtime.Object{
				testClusterDeprovisionRequest(),
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
			name: "regenerate job when hash missing",
			existing: []runtime.Object{
				testClusterDeprovisionRequest(),
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
			name: "regenerate job when hash changed",
			existing: []runtime.Object{
				testClusterDeprovisionRequest(),
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
			fakeClient := fake.NewFakeClient(test.existing...)
			r := &ReconcileClusterDeprovisionRequest{
				Client: fakeClient,
				scheme: scheme.Scheme,
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

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func testClusterDeprovisionRequest() *hivev1.ClusterDeprovisionRequest {
	return &hivev1.ClusterDeprovisionRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		},
		Spec: hivev1.ClusterDeprovisionRequestSpec{
			InfraID:   "test-infra-id",
			ClusterID: "test-cluster-id",
			Platform: hivev1.ClusterDeprovisionRequestPlatform{
				AWS: &hivev1.AWSClusterDeprovisionRequest{
					Region: "us-east-1",
					CredentialsSecretRef: &corev1.LocalObjectReference{
						Name: "aws-creds",
					},
				},
			},
		},
	}
}

func testUninstallJob() *batchv1.Job {
	uninstallJob, _ := install.GenerateUninstallerJobForDeprovisionRequest(testClusterDeprovisionRequest())
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
}

func validateNotCompleted(t *testing.T, c client.Client) {
	req := &hivev1.ClusterDeprovisionRequest{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testName}, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if req.Status.Completed {
		t.Errorf("request is not expected to be in completed state")
	}
}

func validateCompleted(t *testing.T, c client.Client) {
	req := &hivev1.ClusterDeprovisionRequest{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testName}, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !req.Status.Completed {
		t.Errorf("request is expected to be in completed state")
	}
}
