/*
Copyright 2019 The Kubernetes Authors.

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

package remoteingress

import (
	"context"
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ingresscontroller "github.com/openshift/api/operator/v1"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

const (
	testClusterName  = "foo"
	testNamespace    = "default"
	testClusterID    = "foo-12345-uuid"
	testInfraID      = "foo-12345"
	sshKeySecret     = "foo-ssh-key"
	pullSecretSecret = "foo-pull-secret"

	testSyncSetName        = "foo-clusteringress"
	testDefaultIngressName = "default"
	testIngressDomain      = "testapps.example.com"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

type SyncSetEntry struct {
	name              string
	domain            string
	routeSelector     *metav1.LabelSelector
	namespaceSelector *metav1.LabelSelector
}

func TestRemoteClusterIngressReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                   string
		localObjects           []runtime.Object
		expectedSyncSetEntries []SyncSetEntry
	}{
		{
			name: "Test no ingress defined (no expected syncset)",
			localObjects: []runtime.Object{
				testClusterDeploymentWithoutIngress(),
			},
		},
		{
			name: "Test single ingress (only default)",
			localObjects: []runtime.Object{
				testClusterDeployment(),
			},
			expectedSyncSetEntries: []SyncSetEntry{
				{
					name:   testDefaultIngressName,
					domain: testIngressDomain,
				},
			},
		},
		{
			name: "Test multiple ingress",
			localObjects: []runtime.Object{
				addIngressToClusterDeployment(testClusterDeployment(), SyncSetEntry{
					name:   "secondingress",
					domain: "moreingress.example.com",
				}),
			},
			expectedSyncSetEntries: []SyncSetEntry{
				{
					name:   testDefaultIngressName,
					domain: testIngressDomain,
				},
				{
					name:   "secondingress",
					domain: "moreingress.example.com",
				},
			},
		},
		{
			name: "Test updating existing syncset",
			localObjects: []runtime.Object{
				addIngressToClusterDeployment(testClusterDeployment(), SyncSetEntry{
					name:   "secondingress",
					domain: "moreingress.example.com",
				}),
				syncSetFromClusterDeployment(testClusterDeployment()),
			},
			expectedSyncSetEntries: []SyncSetEntry{
				{
					name:   testDefaultIngressName,
					domain: testIngressDomain,
				},
				{
					name:   "secondingress",
					domain: "moreingress.example.com",
				},
			},
		},
		{
			name: "Test removing an ingress from existing syncset",
			localObjects: func() []runtime.Object {
				objects := []runtime.Object{}
				// create a temp cluster deployment with extra ingress
				cd := testClusterDeployment()
				cd.Spec.Ingress = append(cd.Spec.Ingress, hivev1.ClusterIngress{
					Name:   "secondingress",
					Domain: "moreingress.example.com",
				})

				// create a syncset that has the extra ingress
				ss := syncSetFromClusterDeployment(cd)
				objects = append(objects, ss)

				// create the current cluster deployment with only a single ingress
				cd = testClusterDeployment()
				objects = append(objects, cd)

				return objects
			}(),
			expectedSyncSetEntries: []SyncSetEntry{
				{
					name:   testDefaultIngressName,
					domain: testIngressDomain,
				},
			},
		},
		{
			name: "Test setting routeSelector",
			localObjects: []runtime.Object{
				addIngressToClusterDeployment(testClusterDeployment(), SyncSetEntry{
					name:          "secondingress",
					domain:        "moreingress.example.com",
					routeSelector: testRouteSelector(),
				}),
			},
			expectedSyncSetEntries: []SyncSetEntry{
				{
					name:   testDefaultIngressName,
					domain: testIngressDomain,
				},
				{
					name:          "secondingress",
					domain:        "moreingress.example.com",
					routeSelector: testRouteSelector(),
				},
			},
		},
		{
			name: "Test setting namespaceSelector",
			localObjects: []runtime.Object{
				addIngressToClusterDeployment(testClusterDeployment(), SyncSetEntry{
					name:              "secondingress",
					domain:            "moreingress.example.com",
					namespaceSelector: testNamespaceSelector(),
				}),
			},
			expectedSyncSetEntries: []SyncSetEntry{
				{
					name:   testDefaultIngressName,
					domain: testIngressDomain,
				},
				{
					name:              "secondingress",
					domain:            "moreingress.example.com",
					namespaceSelector: testNamespaceSelector(),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.localObjects...)

			rcd := &ReconcileRemoteClusterIngress{
				Client: fakeClient,
				scheme: scheme.Scheme,
				logger: log.WithField("controller", controllerName),
			}
			_, err := rcd.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testClusterName,
					Namespace: testNamespace,
				},
			})

			if err != nil {
				t.Errorf("unexpexted error: %v", err)
			}

			ss := &hivev1.SyncSet{}
			ssNamespacedName := types.NamespacedName{Name: testSyncSetName, Namespace: testNamespace}
			if len(test.expectedSyncSetEntries) == 0 {
				// should get an IsNotFound
				assert.Error(t, fakeClient.Get(context.TODO(), ssNamespacedName, ss))
			} else {
				// validate the syncset data looks correct
				if err := fakeClient.Get(context.TODO(), ssNamespacedName, ss); err != nil {
					t.Errorf("failed fetching SyncSet to check whether it was properly created/updated: %v", err)
				}
				ingressControllers := rawToIngressControllers(ss.Spec.Resources)

				// We should have the expected number of ingress objects
				assert.Equal(t, len(test.expectedSyncSetEntries), len(ingressControllers))

				// And the configuration of each ingress object should match what we expect
				for _, entry := range test.expectedSyncSetEntries {
					foundIngress := false
					for _, ingressController := range ingressControllers {
						if entry.name == ingressController.Name {
							foundIngress = true

							// check domain looks right
							assert.Equal(t, entry.domain, ingressController.Spec.Domain)

							// check routeSelector
							assert.Equal(t, entry.routeSelector, ingressController.Spec.RouteSelector)

							// check namespaceSelector
							assert.Equal(t, entry.namespaceSelector, ingressController.Spec.NamespaceSelector)
						}
					}
					assert.Equal(t, true, foundIngress)
				}
			}
		})
	}
}

func testNamespaceSelector() *metav1.LabelSelector {
	return testRouteSelector()
}

func testRouteSelector() *metav1.LabelSelector {
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"shard": "secretshard",
		},
	}
	return selector
}

func addIngressToClusterDeployment(cd *hivev1.ClusterDeployment, ingress SyncSetEntry) *hivev1.ClusterDeployment {
	cd.Spec.Ingress = append(cd.Spec.Ingress, hivev1.ClusterIngress{
		Name:              ingress.name,
		Domain:            ingress.domain,
		RouteSelector:     ingress.routeSelector,
		NamespaceSelector: ingress.namespaceSelector,
	})

	return cd
}

func syncSetFromClusterDeployment(cd *hivev1.ClusterDeployment) *hivev1.SyncSet {
	rawExtensions := rawExtensionsFromClusterDeployment(cd)
	ssSpec := newSyncSetSpec(cd, rawExtensions)
	return &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cd.Name + "clusteringress",
			Namespace: cd.Namespace,
		},
		Spec: *ssSpec,
	}
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeploymentWithoutIngress()
	cd = addIngressToClusterDeployment(cd, SyncSetEntry{
		name:   testDefaultIngressName,
		domain: testIngressDomain,
	})

	return cd
}

func testClusterDeploymentWithoutIngress() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testClusterName,
			Namespace:  testNamespace,
			Finalizers: []string{hivev1.FinalizerDeprovision},
			UID:        types.UID("1234"),
		},
		Spec: hivev1.ClusterDeploymentSpec{
			SSHKey: &corev1.LocalObjectReference{
				Name: sshKeySecret,
			},
			ClusterName:  testClusterName,
			ControlPlane: hivev1.MachinePool{},
			PullSecret: corev1.LocalObjectReference{
				Name: pullSecretSecret,
			},
			Platform: hivev1.Platform{
				AWS: &hivev1.AWSPlatform{
					Region: "us-east-1",
				},
			},
			Networking: hivev1.Networking{
				Type: hivev1.NetworkTypeOpenshiftSDN,
			},
			PlatformSecrets: hivev1.PlatformSecrets{
				AWS: &hivev1.AWSPlatformSecrets{
					Credentials: corev1.LocalObjectReference{
						Name: "aws-credentials",
					},
				},
			},
		},
		Status: hivev1.ClusterDeploymentStatus{
			Installed:             true,
			AdminKubeconfigSecret: corev1.LocalObjectReference{Name: fmt.Sprintf("%s-admin-kubeconfig", testClusterName)},
			ClusterID:             testClusterID,
			InfraID:               testInfraID,
		},
	}
}

func rawToIngressControllers(rawList []runtime.RawExtension) []*ingresscontroller.IngressController {
	decoder := newIngressControllerDecoder()
	ingressControllers := []*ingresscontroller.IngressController{}

	for _, raw := range rawList {
		obj, _, err := decoder.Decode(raw.Raw, nil, &ingresscontroller.IngressController{})
		if err != nil {
			panic("error decoding to ingresscontroller object")
		}
		ic, ok := obj.(*ingresscontroller.IngressController)
		if !ok {
			panic("error casting to IngressController")
		}
		ingressControllers = append(ingressControllers, ic)
	}
	return ingressControllers
}

func newIngressControllerDecoder() runtime.Decoder {
	scheme, err := hivev1.SchemeBuilder.Build()
	if err != nil {
		panic("error building ingresscontroller scheme")
	}
	codecFactory := serializer.NewCodecFactory(scheme)
	decoder := codecFactory.UniversalDecoder(hivev1.SchemeGroupVersion)

	return decoder
}
