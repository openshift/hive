/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clusterdeployment

import (
	"fmt"
	"testing"
	"time"

	"github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"

	kbatch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/controller/clusterdeployment"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}

var jobKey = types.NamespacedName{Name: "foo-install", Namespace: "default"}

const (
	timeout             = time.Second * 10
	fakeClusterUUID     = "fe953108-f64c-4166-bb8e-20da7665ba00"
	fakeClusterMetadata = `{"clusterName":"foo","aws":{"region":"us-east-1","identifier":{"openshiftClusterID":"fe953108-f64c-4166-bb8e-20da7665ba00"}}}`
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: hivev1.ClusterDeploymentSpec{
			Config: hivev1.InstallConfig{
				Admin: hivev1.Admin{
					Email: "user@example.com",
					Password: corev1.LocalObjectReference{
						Name: "admin-password",
					},
					SSHKey: &corev1.LocalObjectReference{
						Name: "ssh-key",
					},
				},
				Machines: []hivev1.MachinePool{},
				PullSecret: corev1.LocalObjectReference{
					Name: "pull-secret",
				},
				Platform: hivev1.Platform{
					AWS: &hivev1.AWSPlatform{
						Region: "us-east-1",
					},
				},
			},
			PlatformSecrets: hivev1.PlatformSecrets{
				AWS: &hivev1.AWSPlatformSecrets{
					Credentials: corev1.LocalObjectReference{
						Name: "aws-credentials",
					},
				},
			},
		},
	}
}

func TestReconcileNewClusterDeployment(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(clusterdeployment.NewReconciler(mgr))
	g.Expect(clusterdeployment.AddToManager(mgr, recFn)).NotTo(gomega.HaveOccurred())
	defer close(StartTestManager(mgr, g))

	instance := testClusterDeployment()
	// Create the ClusterDeployment object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Errorf("failed to create object, got an invalid object error: %v", err)
		t.Fail()
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	job := &kbatch.Job{}
	g.Eventually(func() error { return c.Get(context.TODO(), jobKey, job) }, timeout).
		Should(gomega.Succeed())

	err = fakeInstallJobSuccess(c, instance, job)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// Test that our cluster deployment is updated as we would expect:
	g.Eventually(func() error {
		updatedCD := &hivev1.ClusterDeployment{}
		err := c.Get(context.TODO(), expectedRequest.NamespacedName, updatedCD)
		if err != nil {
			return err
		}
		// All of these conditions should eventually be true:
		if !updatedCD.Status.Installed {
			return fmt.Errorf("cluster deployment status not marked installed")
		}
		if updatedCD.Status.ClusterUUID != fakeClusterUUID {
			return fmt.Errorf("cluster deployment status does not have cluster UUID")
		}
		if !clusterdeployment.HasFinalizer(updatedCD, hivev1.FinalizerDeprovision) {
			return fmt.Errorf("cluster deployment does not have expected finalizer")
		}
		return nil
	}, timeout).Should(gomega.Succeed())

	// Delete the Job and expect Reconcile to be called for Job deletion
	g.Expect(c.Delete(context.TODO(), job)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return c.Get(context.TODO(), jobKey, job) }, timeout).
		Should(gomega.Succeed())

	// Manually delete Job since GC isn't enabled in the test control plane
	g.Expect(c.Delete(context.TODO(), job)).To(gomega.Succeed())
}

// TODO: how to mimic objects already existing?

func fakeInstallJobSuccess(c client.Client, cd *hivev1.ClusterDeployment, job *kbatch.Job) error {

	// Create a fake cluster metadata configmap:
	metadataCfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-metadata", cd.Name),
			Namespace: cd.Namespace,
		},
		Data: map[string]string{
			"metadata.json": fakeClusterMetadata,
		},
	}
	err := c.Create(context.TODO(), metadataCfgMap)
	if err != nil {
		return err
	}

	// Fake that the install job was successful:
	job.Status.Conditions = []kbatch.JobCondition{
		{
			Type:   kbatch.JobComplete,
			Status: corev1.ConditionTrue,
		},
	}
	return c.Status().Update(context.TODO(), job)
}
