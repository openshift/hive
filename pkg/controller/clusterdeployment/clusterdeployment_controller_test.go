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

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	"github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"

	kbatch "k8s.io/api/batch/v1"
	kapi "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}

var cfgMapKey = types.NamespacedName{Name: "foo-install", Namespace: "default"}
var jobKey = types.NamespacedName{Name: "foo-install", Namespace: "default"}

const timeout = time.Second * 5

func init() {
	log.SetLevel(log.DebugLevel)
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: hivev1.ClusterDeploymentSpec{
			Config: hivev1.InstallConfig{
				Machines: []hivev1.MachinePool{},
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

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())
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

	cfgMap := &kapi.ConfigMap{}
	g.Eventually(func() error { return c.Get(context.TODO(), cfgMapKey, cfgMap) }, timeout).
		Should(gomega.Succeed())

	job := &kbatch.Job{}
	g.Eventually(func() error { return c.Get(context.TODO(), jobKey, job) }, timeout).
		Should(gomega.Succeed())

	// Fake that the install job was successful:
	job.Status.Conditions = []kbatch.JobCondition{
		{
			Type:   kbatch.JobComplete,
			Status: kapi.ConditionTrue,
		},
	}
	g.Expect(c.Status().Update(context.TODO(), job)).NotTo(gomega.HaveOccurred())

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
		if !HasFinalizer(updatedCD, hivev1.FinalizerDeprovision) {
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
