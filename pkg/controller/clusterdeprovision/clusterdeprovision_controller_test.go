package clusterdeprovision

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	awsclient "github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	testName      = "deprovision-request"
	testNamespace = "default"
	testFinalizer = "test-finalizer"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestClusterDeprovisionReconcile(t *testing.T) {

	tests := []struct {
		name                  string
		deployment            *hivev1.ClusterDeployment
		deprovision           *hivev1.ClusterDeprovision
		mockGetCallerIdentity bool
		// The reconcile flow deletes the existing deprovision job if it failed, or if its spec
		// needs to be changed. That's the only delete in the flow. Setting this field to `true`
		// causes the test driver to mock out that deletion to return an error, allowing coverage
		// of related error paths.
		mockDeleteFailure              bool
		expectedGetCallerIdentityError error
		existing                       []runtime.Object
		validate                       func(t *testing.T, c client.Client)
		expectErr                      bool
		deprovisionsDisabled           bool
	}{
		{
			name: "no-op deleting",
			deprovision: func() *hivev1.ClusterDeprovision {
				req := testClusterDeprovision()
				now := metav1.Now()
				req.DeletionTimestamp = &now
				req.Finalizers = []string{testFinalizer}
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
		},
		{
			name:        "no-op if cluster deployment has delete protection on",
			deprovision: testClusterDeprovision(),
			deployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				if cd.Annotations == nil {
					cd.Annotations = make(map[string]string, 1)
				}
				cd.Annotations[constants.ProtectedDeleteAnnotation] = "true"
				return cd
			}(),
			validate: func(t *testing.T, c client.Client) {
				validateNoJobExists(t, c)
			},
		},
		{
			name:                  "create uninstall job",
			deprovision:           testClusterDeprovision(),
			deployment:            testDeletedClusterDeployment(),
			mockGetCallerIdentity: true,
			validate: func(t *testing.T, c client.Client) {
				validateJobExists(t, c)
			},
		},
		{
			name:                 "do not create uninstall job when deprovisions are disabled",
			deprovision:          testClusterDeprovision(),
			deployment:           testDeletedClusterDeployment(),
			deprovisionsDisabled: true,
			validate: func(t *testing.T, c client.Client) {
				validateNoJobExists(t, c)
			},
		},
		{
			name:        "no-op when job in progress",
			deprovision: testClusterDeprovision(),
			deployment:  testDeletedClusterDeployment(),
			existing: []runtime.Object{
				testUninstallJob(),
			},
			mockGetCallerIdentity: true,
			validate: func(t *testing.T, c client.Client) {
				validateNotCompleted(t, c)
			},
		},
		{
			name:        "completed when job is successful",
			deprovision: testClusterDeprovision(),
			deployment:  testDeletedClusterDeployment(),
			existing: []runtime.Object{
				testUninstallJob(batchv1.JobCondition{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				}),
			},
			mockGetCallerIdentity: true,
			validate: func(t *testing.T, c client.Client) {
				validateCompleted(t, c)
			},
		},
		{
			name:        "error on failed delete",
			deprovision: testClusterDeprovision(),
			deployment:  testDeletedClusterDeployment(),
			existing: []runtime.Object{
				testUninstallJob(batchv1.JobCondition{
					Type:   batchv1.JobFailed,
					Status: corev1.ConditionTrue,
				}),
			},
			mockGetCallerIdentity: true,
			mockDeleteFailure:     true,
			expectErr:             true,
		},
		{
			name:        "job failed",
			deprovision: testClusterDeprovision(),
			deployment:  testDeletedClusterDeployment(),
			existing: []runtime.Object{
				testUninstallJob(batchv1.JobCondition{
					Type:   batchv1.JobFailed,
					Status: corev1.ConditionTrue,
				}),
			},
			mockGetCallerIdentity: true,
			validate: func(t *testing.T, c client.Client) {
				validateCondition(t, c, []hivev1.ClusterDeprovisionCondition{
					{
						Type:    hivev1.DeprovisionFailedClusterDeprovisionCondition,
						Reason:  "UnknownError",
						Message: "Deprovision attempt failed for unknown reason",
						Status:  corev1.ConditionTrue,
					},
				})
			},
		},
		{
			name:        "job deadline exceeded",
			deprovision: testClusterDeprovision(),
			deployment:  testDeletedClusterDeployment(),
			existing: []runtime.Object{
				testUninstallJob(batchv1.JobCondition{
					Type:   batchv1.JobFailed,
					Status: corev1.ConditionTrue,
					Reason: "DeadlineExceeded",
				}),
			},
			mockGetCallerIdentity: true,
			validate: func(t *testing.T, c client.Client) {
				validateCondition(t, c, []hivev1.ClusterDeprovisionCondition{
					{
						Type:    hivev1.DeprovisionFailedClusterDeprovisionCondition,
						Reason:  "AttemptDeadlineExceeded",
						Message: "Deprovision attempt failed because the deadline was exceeded",
						Status:  corev1.ConditionTrue,
					},
				})
			},
		},
		{
			name:        "credentials test fails",
			deprovision: testClusterDeprovision(),
			deployment:  testDeletedClusterDeployment(),
			existing: []runtime.Object{
				testUninstallJob(batchv1.JobCondition{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				}),
			},
			mockGetCallerIdentity:          true,
			expectedGetCallerIdentityError: awserr.New("InvalidClientTokenId", "", fmt.Errorf("")),
			validate: func(t *testing.T, c client.Client) {
				validateCondition(t, c, []hivev1.ClusterDeprovisionCondition{
					{
						Type:   hivev1.AuthenticationFailureClusterDeprovisionCondition,
						Reason: "AuthenticationFailed",
						Status: corev1.ConditionTrue,
					},
				})
			},
			expectErr: true,
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
			mockGetCallerIdentity: true,
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
			mockGetCallerIdentity: true,
			validate: func(t *testing.T, c client.Client) {
				validateNoJobExists(t, c)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			scheme := scheme.GetScheme()
			if test.deployment != nil {
				// Associate the cluster deployment as the owner of the provision to match real world:
				err := controllerutil.SetControllerReference(test.deployment, test.deprovision, scheme)
				if err != nil {
					t.Errorf("unable to set owner reference on deprovision: %v", err)
					return
				}
			}
			existing := append(test.existing, test.deprovision, test.deployment)

			mocks := setupDefaultMocks(t, test.mockDeleteFailure, existing...)

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.

			if test.mockGetCallerIdentity {
				mocks.mockAWSClient.EXPECT().
					GetCallerIdentity(gomock.Any()).
					Return(nil, test.expectedGetCallerIdentityError)
			}

			r := &ReconcileClusterDeprovision{
				Client:               mocks.fakeKubeClient,
				scheme:               scheme,
				deprovisionsDisabled: test.deprovisionsDisabled,
			}

			// Save the list of actuators so that it can be restored at the end of this test
			actuatorsSaved := actuators
			actuators = []Actuator{&awsActuator{awsClientFn: func(clusterDeprovision *hivev1.ClusterDeprovision, c client.Client, logger log.FieldLogger) (awsclient.Client, error) {
				return mocks.mockAWSClient, nil
			}}}

			_, err := r.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})

			// Restore the actuators list back to what it was before this test
			actuators = actuatorsSaved

			if test.validate != nil {
				test.validate(t, mocks.fakeKubeClient)
			}

			if test.expectErr {
				assert.NotNil(t, err, "Expected error but didn't get one")
			} else {
				assert.Nil(t, err, "Unexpected error: %v", err)
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
	cd.ObjectMeta.Finalizers = []string{testFinalizer}
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

// testUninstallJob returns a Job for the deprovision generated by testClusterDeprovision().
// By default, the start and completion timestamps are unset.
// If one or more `conditions` are specified, we will set the time stamps in addition to adding the
// specified conditions.
func testUninstallJob(conditions ...batchv1.JobCondition) *batchv1.Job {
	uninstallJob, _ := install.GenerateUninstallerJobForDeprovision(testClusterDeprovision(),
		"someserviceaccount", "", "", "", nil)
	hash, err := controllerutils.CalculateJobSpecHash(uninstallJob)
	if err != nil {
		panic("should never get error calculating job spec hash")
	}

	uninstallJob.Annotations[jobHashAnnotation] = hash

	if len(conditions) != 0 {
		uninstallJob.Status.Conditions = conditions
		now := metav1.Now()
		uninstallJob.Status.CompletionTime = &now
		uninstallJob.Status.StartTime = &now
	}

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

func validateCondition(t *testing.T, c client.Client, expectedConditions []hivev1.ClusterDeprovisionCondition) {
	req := &hivev1.ClusterDeprovision{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testName}, req)
	assert.NoError(t, err, "unexpected error getting ClusterDeprovision")

	if len(req.Status.Conditions) != len(expectedConditions) {
		t.Errorf("request condition length %v does not match expected condition length %v", len(req.Status.Conditions), len(expectedConditions))
	}

	for i, expectedCondition := range expectedConditions {
		actualCondition := req.Status.Conditions[i]

		if actualCondition.Status != expectedCondition.Status ||
			actualCondition.Type != expectedCondition.Type ||
			actualCondition.Reason != expectedCondition.Reason {
			t.Errorf("request is expected to have specific condition %v, got %v", expectedCondition, actualCondition)
		}
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
