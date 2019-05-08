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

package syncset

import (
	"context"
	"crypto/md5"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/resource"
)

const (
	testName                 = "foo"
	testNamespace            = "default"
	adminKubeconfigSecret    = "foo-admin-kubeconfig"
	adminKubeconfigSecretKey = "kubeconfig"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestSyncSetReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name              string
		clusterDeployment *hivev1.ClusterDeployment
		syncSets          []*hivev1.SyncSet
		selectorSyncSets  []*hivev1.SelectorSyncSet
		validate          func(*testing.T, *hivev1.ClusterDeployment)
		expectDeleted     []deletedItemInfo
		expectErr         bool
	}{
		{
			name:              "Create single resource successfully",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithResources("ss1", testCM("cm1", "foo", "bar")),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulResourceStatus("ss1", []runtime.Object{testCM("cm1", "foo", "bar")}),
				})
			},
		},
		{
			name:              "Create multiple resources successfully",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithResources("foo", testCMs("bar", 5)...),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulResourceStatus("foo", testCMs("bar", 5)),
				})
			},
		},
		{
			name:              "Create resources from multiple syncsets",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithResources("foo1", testCMs("foo1", 4)...),
				testSyncSetWithResources("foo2", testCMs("foo2", 3)...),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulResourceStatus("foo1", testCMs("foo1", 4)),
					successfulResourceStatus("foo2", testCMs("foo2", 3)),
				})
			},
		},
		{
			name: "Update single resource",
			clusterDeployment: testClusterDeployment([]hivev1.SyncSetObjectStatus{
				successfulResourceStatus("ss1", []runtime.Object{testCM("cm1", "key1", "value1")}),
			}, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithResources("ss1", testCM("cm1", "key1", "value***changed")),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulResourceStatus("ss1", []runtime.Object{testCM("cm1", "key1", "value***changed")}),
				})
			},
		},
		{
			name: "Update only resources that have changed",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				fiveMinutesAgo := time.Unix(metav1.NewTime(time.Now().Add(-5*time.Minute)).Unix(), 0)
				cd := testClusterDeployment([]hivev1.SyncSetObjectStatus{
					successfulResourceStatusWithTime("aaa", []runtime.Object{
						testCM("cm1", "key1", "value1"),
						testCM("cm2", "key2", "value2"),
						testCM("cm3", "key3", "value3"),
					}, metav1.NewTime(fiveMinutesAgo)),
				}, nil)
				// let's store the time as an annotation for later verification
				cd.Annotations = map[string]string{
					"five-minutes-ago": fmt.Sprintf("%d", fiveMinutesAgo.Unix()),
				}
				return cd
			}(),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithResources("aaa",
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value***changed"),
					testCM("cm3", "key3", "value3"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulResourceStatus("aaa", []runtime.Object{
						testCM("cm1", "key1", "value1"),
						testCM("cm2", "key2", "value***changed"),
						testCM("cm3", "key3", "value3"),
					}),
				})
				unixFiveMinutesAgo, _ := strconv.ParseInt(cd.Annotations["five-minutes-ago"], 0, 64)
				unchanged := []hivev1.SyncStatus{
					cd.Status.SyncSetStatus[0].Resources[0],
					cd.Status.SyncSetStatus[0].Resources[2],
				}
				for _, ss := range unchanged {
					if ss.Conditions[0].LastProbeTime.Time.Unix() != unixFiveMinutesAgo {
						t.Errorf("unexpected condition last probe time for resource %s/%s. Got: %v, Expected: %v", ss.Namespace, ss.Name, ss.Conditions[0].LastProbeTime.Time, time.Unix(unixFiveMinutesAgo, 0))
					}
					if ss.Conditions[0].LastTransitionTime.Time.Unix() != unixFiveMinutesAgo {
						t.Errorf("unexpected condition last transition time for resource %s/%s. Got: %v, Expected: %v", ss.Namespace, ss.Name, ss.Conditions[0].LastTransitionTime.Time, time.Unix(unixFiveMinutesAgo, 0))
					}
				}
				changed := cd.Status.SyncSetStatus[0].Resources[1]
				if changed.Conditions[0].LastProbeTime.Time.Unix() <= unixFiveMinutesAgo {
					t.Errorf("unexpected condition last probe time for resource %s/%s. Got: %v, Expected a more recent time", changed.Namespace, changed.Name, changed.Conditions[0].LastProbeTime.Time)
				}
				// The last transition time should not have changed because we went from successful to successful
				if changed.Conditions[0].LastTransitionTime.Time.Unix() != unixFiveMinutesAgo {
					t.Errorf("unexpected condition last transition time for resource %s/%s. Got: %v, Expected: %v", changed.Namespace, changed.Name, changed.Conditions[0].LastTransitionTime.Time, time.Unix(unixFiveMinutesAgo, 0))
				}
			},
		},
		{
			name:              "Check for failed info call, set condition",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithResources("foo",
					testCM("cm1", "key1", "value1"),
					testCM("info-error", "key2", "value2"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateUnknownObjectCondition(t, cd.Status.SyncSetStatus[0])
			},
			expectErr: true,
		},
		{
			name:              "Check for failed info call, set condition and process other syncsets",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithResources("foo", testCM("info-error", "key1", "value1")),
				testSyncSetWithResources("bar", testCM("valid", "key2", "value2")),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				for _, ss := range cd.Status.SyncSetStatus {
					if ss.Name == "foo" {
						validateUnknownObjectCondition(t, ss)
					} else if ss.Name == "bar" {
						validateSyncSetStatus(t, ss, successfulResourceStatus("bar", []runtime.Object{testCM("valid", "key2", "value2")}))
					} else {
						t.Errorf("unexpected syncset status: %s", ss.Name)
					}
				}
			},
			expectErr: true,
		},
		{
			name:              "Stop applying resources when error occurs",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithResources("foo",
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value2"),
					testCM("apply-error", "key3", "value3"),
					testCM("cm4", "key4", "value4"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				status := successfulResourceStatus("foo", []runtime.Object{
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value2"),
				})
				status.Resources = append(status.Resources, failedResourceStatus("foo", []runtime.Object{
					testCM("apply-error", "key3", "value3"),
				}).Resources...)
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{status})
			},
			expectErr: true,
		},
		{
			name:              "selectorsyncset: apply single resource",
			clusterDeployment: testClusterDeployment(nil, nil),
			selectorSyncSets: []*hivev1.SelectorSyncSet{
				testMatchingSelectorSyncSetWithResources("foo",
					testCM("cm1", "key1", "value1"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SelectorSyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulResourceStatus("foo", []runtime.Object{
						testCM("cm1", "key1", "value1"),
					}),
				})
			},
		},
		{
			name:              "selectorsyncset: apply resources for only matching",
			clusterDeployment: testClusterDeployment(nil, nil),
			selectorSyncSets: []*hivev1.SelectorSyncSet{
				testMatchingSelectorSyncSetWithResources("foo",
					testCM("cm1", "key1", "value1"),
				),
				testNonMatchingSelectorSyncSetWithResources("bar",
					testCM("cm2", "key2", "value2"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SelectorSyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulResourceStatus("foo", []runtime.Object{
						testCM("cm1", "key1", "value1"),
					}),
				})
			},
		},
		{
			name:              "Apply single patch successfully",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithPatches("ss1", testSyncObjectPatch("foo", "bar", "baz", "v1", "AlwaysApply", "value1")),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulPatchStatus("ss1", []hivev1.SyncObjectPatch{testSyncObjectPatch("foo", "bar", "baz", "v1", "AlwaysApply", "value1")}),
				})
			},
		},
		{
			name:              "Apply multiple patches successfully",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithPatches("ss1",
					testSyncObjectPatch("foo", "bar", "baz", "v1", "AlwaysApply", "value1"),
					testSyncObjectPatch("chicken", "potato", "stew", "v1", "AlwaysApply", "value2"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulPatchStatus("ss1",
						[]hivev1.SyncObjectPatch{
							testSyncObjectPatch("foo", "bar", "baz", "v1", "AlwaysApply", "value1"),
							testSyncObjectPatch("chicken", "potato", "stew", "v1", "AlwaysApply", "value2"),
						},
					),
				})
			},
		},
		{
			name: "Reapply single patch",
			clusterDeployment: testClusterDeployment([]hivev1.SyncSetObjectStatus{
				successfulPatchStatus("ss1", []hivev1.SyncObjectPatch{
					testSyncObjectPatch("foo", "bar", "baz", "v1", "ApplyOnce", "value1"),
				}),
			}, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithPatches("ss1",
					testSyncObjectPatch("foo", "bar", "baz", "v1", "ApplyOnce", "value1***changed"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulPatchStatus("ss1",
						[]hivev1.SyncObjectPatch{
							testSyncObjectPatch("foo", "bar", "baz", "v1", "ApplyOnce", "value1***changed"),
						},
					),
				})
			},
		},
		{
			name:              "Apply patches from multiple syncsets",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithPatches("ss1", testSyncObjectPatch("foo", "bar", "baz", "v1", "AlwaysApply", "value1")),
				testSyncSetWithPatches("ss2", testSyncObjectPatch("chicken", "potato", "stew", "v1", "AlwaysApply", "value1")),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulPatchStatus("ss1", []hivev1.SyncObjectPatch{testSyncObjectPatch("foo", "bar", "baz", "v1", "AlwaysApply", "value1")}),
					successfulPatchStatus("ss2", []hivev1.SyncObjectPatch{testSyncObjectPatch("chicken", "potato", "stew", "v1", "AlwaysApply", "value1")}),
				})
			},
		},
		{
			name:              "Stop applying patches when error occurs",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSetWithPatches("ss1",
					testSyncObjectPatch("thing1", "bar", "baz", "v1", "AlwaysApply", "value1"),
					testSyncObjectPatch("thing2", "bar", "baz", "v1", "AlwaysApply", "value1"),
					testSyncObjectPatch("thing3", "bar", "baz", "v1", "AlwaysApply", "patch-error"),
					testSyncObjectPatch("thing4", "bar", "baz", "v1", "AlwaysApply", "value1"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				status := successfulPatchStatus("ss1", []hivev1.SyncObjectPatch{
					testSyncObjectPatch("thing1", "bar", "baz", "v1", "AlwaysApply", "value1"),
					testSyncObjectPatch("thing2", "bar", "baz", "v1", "AlwaysApply", "value1"),
				})
				status.Patches = append(status.Patches, failedPatchStatus("ss1", []hivev1.SyncObjectPatch{
					testSyncObjectPatch("thing3", "bar", "baz", "v1", "AlwaysApply", "patch-error"),
				}).Patches...)
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{status})
			},
			expectErr: true,
		},
		{
			name:              "selectorsyncset: apply single resource",
			clusterDeployment: testClusterDeployment(nil, nil),
			selectorSyncSets: []*hivev1.SelectorSyncSet{
				testMatchingSelectorSyncSetWithPatches("ss1",
					testSyncObjectPatch("foo", "bar", "baz", "v1", "ApplyOnce", "value1"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SelectorSyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulPatchStatus("ss1", []hivev1.SyncObjectPatch{
						testSyncObjectPatch("foo", "bar", "baz", "v1", "ApplyOnce", "value1"),
					}),
				})
			},
		},
		{
			name:              "selectorsyncset: apply patches for only matching",
			clusterDeployment: testClusterDeployment(nil, nil),
			selectorSyncSets: []*hivev1.SelectorSyncSet{
				testMatchingSelectorSyncSetWithPatches("ss1",
					testSyncObjectPatch("foo1", "bar1", "baz1", "v1", "ApplyOnce", "value1"),
				),
				testNonMatchingSelectorSyncSetWithPatches("ss2",
					testSyncObjectPatch("foo2", "bar2", "baz2", "v1", "ApplyOnce", "value2"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SelectorSyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulPatchStatus("ss1", []hivev1.SyncObjectPatch{
						testSyncObjectPatch("foo1", "bar1", "baz1", "v1", "ApplyOnce", "value1"),
					}),
				})
			},
		},
		{
			name:              "No patches applied when resource error occurs",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSet("ss1",
					[]runtime.Object{
						testCM("cm1", "key1", "value1"),
						testCM("cm2", "key2", "value2"),
						testCM("apply-error", "key3", "value3"),
						testCM("cm4", "key4", "value4"),
					},
					[]hivev1.SyncObjectPatch{
						testSyncObjectPatch("thing1", "bar", "baz", "v1", "AlwaysApply", "value1"),
						testSyncObjectPatch("thing2", "bar", "baz", "v1", "AlwaysApply", "value2"),
					},
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				status := successfulResourceStatus("ss1", []runtime.Object{
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value2"),
				})
				status.Resources = append(status.Resources, failedResourceStatus("ss1", []runtime.Object{
					testCM("apply-error", "key3", "value3"),
				}).Resources...)
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{status})
			},
			expectErr: true,
		},
		{
			name: "resource sync mode, remove resources",
			clusterDeployment: testClusterDeployment([]hivev1.SyncSetObjectStatus{
				successfulResourceStatus("aaa", []runtime.Object{
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value2"),
					testCM("cm3", "key3", "value3"),
					testCM("cm4", "key4", "value4"),
				}),
			}, nil),
			syncSets: []*hivev1.SyncSet{
				func() *hivev1.SyncSet {
					ss := testSyncSetWithResources("aaa",
						testCM("cm1", "key1", "value1"),
						testCM("cm3", "key3", "value3"),
					)
					ss.Spec.ResourceApplyMode = hivev1.SyncResourceApplyMode
					return ss
				}(),
			},
			expectDeleted: []deletedItemInfo{
				deletedCM("cm2"),
				deletedCM("cm4"),
			},
		},
	}

	for _, test := range tests {
		apis.AddToScheme(scheme.Scheme)
		t.Run(test.name, func(t *testing.T) {
			runtimeObjs := []runtime.Object{test.clusterDeployment, kubeconfigSecret()}
			for _, s := range test.syncSets {
				runtimeObjs = append(runtimeObjs, s)
			}
			for _, s := range test.selectorSyncSets {
				runtimeObjs = append(runtimeObjs, s)
			}
			fakeClient := fake.NewFakeClient(runtimeObjs...)
			dynamicClient := &fakeDynamicClient{}

			helper := &fakeHelper{t: t}
			rcd := &ReconcileSyncSet{
				Client:         fakeClient,
				scheme:         scheme.Scheme,
				logger:         log.WithField("controller", "syncset"),
				applierBuilder: helper.newHelper,
				hash:           fakeHashFunc(t),
				dynamicClientBuilder: func(string) (dynamic.Interface, error) {
					return dynamicClient, nil
				},
			}
			_, err := rcd.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})
			if !test.expectErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			} else if test.expectErr && err == nil {
				t.Fatal("expected error not returned")
			}
			validateDeletedItems(t, dynamicClient.deletedItems, test.expectDeleted)
			cd := &hivev1.ClusterDeployment{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: testName, Namespace: testNamespace}, cd)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.validate != nil {
				test.validate(t, cd)
			}
		})
	}
}

func sameDeletedItem(a, b deletedItemInfo) bool {
	return a.name == b.name &&
		a.namespace == b.namespace &&
		a.group == b.group &&
		a.version == b.version &&
		a.resource == b.resource
}

func validateDeletedItems(t *testing.T, actual, expected []deletedItemInfo) {
	if len(actual) != len(expected) {
		t.Errorf("unexpected number of deleted items, actual: %d, expected: %d", len(actual), len(expected))
		return
	}
	for _, item := range actual {
		index := -1
		for i, expectedItem := range expected {
			if sameDeletedItem(item, expectedItem) {
				index = i
				break
			}
		}
		if index == -1 {
			t.Errorf("unexpected deleted item: %#v", item)
			return
		}
		// remove the item from the expected array
		expected = append(expected[0:index], expected[index+1:]...)
	}
}

func testClusterDeployment(syncSetStatus []hivev1.SyncSetObjectStatus, selectorSyncSetStatus []hivev1.SyncSetObjectStatus) *hivev1.ClusterDeployment {
	cd := hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
			Labels: map[string]string{
				"region": "us-east-1",
			},
		},
		Spec: hivev1.ClusterDeploymentSpec{},
		Status: hivev1.ClusterDeploymentStatus{
			Installed:             true,
			AdminKubeconfigSecret: corev1.LocalObjectReference{Name: adminKubeconfigSecret},
			SyncSetStatus:         syncSetStatus,
			SelectorSyncSetStatus: selectorSyncSetStatus,
		},
	}

	return &cd
}

func testSyncSet(name string, resources []runtime.Object, patches []hivev1.SyncObjectPatch) *hivev1.SyncSet {
	ss := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: testName,
				},
			},
		},
	}
	for _, r := range resources {
		ss.Spec.Resources = append(ss.Spec.Resources, runtime.RawExtension{
			Object: r,
		})
	}
	for _, p := range patches {
		ss.Spec.Patches = append(ss.Spec.Patches, p)
	}
	return ss
}

func testSyncSetWithResources(name string, resources ...runtime.Object) *hivev1.SyncSet {
	ss := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: testName,
				},
			},
		},
	}
	for _, r := range resources {
		ss.Spec.Resources = append(ss.Spec.Resources, runtime.RawExtension{
			Object: r,
		})
	}
	return ss
}

func testSyncSetWithPatches(name string, patches ...hivev1.SyncObjectPatch) *hivev1.SyncSet {
	ss := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: testName,
				},
			},
		},
	}
	for _, p := range patches {
		ss.Spec.Patches = append(ss.Spec.Patches, p)
	}
	return ss
}

func testSyncObjectPatch(name, namespace, kind, apiVersion string, applyMode hivev1.SyncSetPatchApplyMode, value string) hivev1.SyncObjectPatch {
	patch := fmt.Sprintf("{'spec': {'key: '%v'}}", value)
	return hivev1.SyncObjectPatch{
		Name:       name,
		Namespace:  namespace,
		Kind:       kind,
		APIVersion: apiVersion,
		ApplyMode:  applyMode,
		Patch:      patch,
		PatchType:  "merge",
	}
}

func testMatchingSelectorSyncSetWithResources(name string, resources ...runtime.Object) *hivev1.SelectorSyncSet {
	return testSelectorSyncSetWithResources(name, map[string]string{"region": "us-east-1"}, resources...)
}

func testNonMatchingSelectorSyncSetWithResources(name string, resources ...runtime.Object) *hivev1.SelectorSyncSet {
	return testSelectorSyncSetWithResources(name, map[string]string{"region": "us-west-2"}, resources...)
}

func testSelectorSyncSetWithResources(name string, matchLabels map[string]string, resources ...runtime.Object) *hivev1.SelectorSyncSet {
	ss := &hivev1.SelectorSyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: hivev1.SelectorSyncSetSpec{
			ClusterDeploymentSelector: metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}
	for _, r := range resources {
		ss.Spec.Resources = append(ss.Spec.Resources, runtime.RawExtension{
			Object: r,
		})
	}
	return ss
}

func testMatchingSelectorSyncSetWithPatches(name string, patches ...hivev1.SyncObjectPatch) *hivev1.SelectorSyncSet {
	return testSelectorSyncSetWithPatches(name, map[string]string{"region": "us-east-1"}, patches...)
}

func testNonMatchingSelectorSyncSetWithPatches(name string, patches ...hivev1.SyncObjectPatch) *hivev1.SelectorSyncSet {
	return testSelectorSyncSetWithPatches(name, map[string]string{"region": "us-west-2"}, patches...)
}

func testSelectorSyncSetWithPatches(name string, matchLabels map[string]string, patches ...hivev1.SyncObjectPatch) *hivev1.SelectorSyncSet {
	ss := &hivev1.SelectorSyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: hivev1.SelectorSyncSetSpec{
			ClusterDeploymentSelector: metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}
	for _, p := range patches {
		ss.Spec.Patches = append(ss.Spec.Patches, p)
	}
	return ss
}

func testCM(name, key, value string) runtime.Object {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Annotations: map[string]string{
				"hash": fmt.Sprintf("%s=%s", key, value),
			},
		},
		Data: map[string]string{
			key: value,
		},
	}
}

func deletedCM(name string) deletedItemInfo {
	return deletedItemInfo{
		name:      name,
		namespace: testNamespace,
		group:     "",
		version:   "v1",
		resource:  "ConfigMap",
	}
}

func testCMs(prefix string, count int) []runtime.Object {
	result := []runtime.Object{}
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("%s-key-%d", prefix, i)
		value := fmt.Sprintf("%s-value-%d", prefix, i)
		result = append(result, testCM(fmt.Sprintf("%s-%d", prefix, i), key, value))
	}
	return result
}

func kubeconfigSecret() *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      adminKubeconfigSecret,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			adminKubeconfigSecretKey: []byte("foo"),
		},
	}
	return s
}

func failedResourceStatus(name string, resources []runtime.Object) hivev1.SyncSetObjectStatus {
	conditionTime := metav1.Now()
	status := hivev1.SyncSetObjectStatus{
		Name: name,
	}
	for _, r := range resources {
		obj, _ := meta.Accessor(r)
		status.Resources = append(status.Resources, hivev1.SyncStatus{
			APIVersion: r.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       r.GetObjectKind().GroupVersionKind().Kind,
			Resource:   r.GetObjectKind().GroupVersionKind().Kind,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			Hash:       objectHash(obj),
			Conditions: []hivev1.SyncCondition{
				{
					Type:               hivev1.ApplyFailureSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
			},
		})
	}
	return status
}

func failedPatchStatus(name string, patches []hivev1.SyncObjectPatch) hivev1.SyncSetObjectStatus {
	conditionTime := metav1.Now()
	status := hivev1.SyncSetObjectStatus{
		Name: name,
	}
	for _, p := range patches {
		status.Patches = append(status.Patches, hivev1.SyncStatus{
			APIVersion: p.APIVersion,
			Kind:       p.Kind,
			Name:       p.Name,
			Namespace:  p.Namespace,
			Hash:       fmt.Sprintf("%x", md5.Sum([]byte(p.Patch))),
			Conditions: []hivev1.SyncCondition{
				{
					Type:               hivev1.ApplyFailureSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
			},
		})
	}
	return status
}

func successfulResourceStatus(name string, resources []runtime.Object) hivev1.SyncSetObjectStatus {
	return successfulResourceStatusWithTime(name, resources, metav1.Now())
}

func successfulResourceStatusWithTime(name string, resources []runtime.Object, conditionTime metav1.Time) hivev1.SyncSetObjectStatus {
	status := hivev1.SyncSetObjectStatus{
		Name: name,
	}
	for _, r := range resources {
		obj, _ := meta.Accessor(r)
		status.Resources = append(status.Resources, hivev1.SyncStatus{
			APIVersion: r.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       r.GetObjectKind().GroupVersionKind().Kind,
			Resource:   r.GetObjectKind().GroupVersionKind().Kind,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			Hash:       objectHash(obj),
			Conditions: []hivev1.SyncCondition{
				{
					Type:               hivev1.ApplySuccessSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
			},
		})
	}
	return status
}

func successfulPatchStatus(name string, patches []hivev1.SyncObjectPatch) hivev1.SyncSetObjectStatus {
	return successfulPatchStatusWithTime(name, patches, metav1.Now())
}

func successfulPatchStatusWithTime(name string, patches []hivev1.SyncObjectPatch, conditionTime metav1.Time) hivev1.SyncSetObjectStatus {
	status := hivev1.SyncSetObjectStatus{
		Name: name,
	}
	for _, p := range patches {
		status.Patches = append(status.Patches, hivev1.SyncStatus{
			APIVersion: p.APIVersion,
			Kind:       p.Kind,
			Name:       p.Name,
			Namespace:  p.Namespace,
			Hash:       fmt.Sprintf("%x", md5.Sum([]byte(p.Patch))),
			Conditions: []hivev1.SyncCondition{
				{
					Type:               hivev1.ApplySuccessSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
			},
		})
	}
	return status
}

type fakeHelper struct {
	t *testing.T
}

func (f *fakeHelper) newHelper(kubeconfig []byte, logger log.FieldLogger) Applier {
	return f
}

func (f *fakeHelper) Apply(data []byte) (resource.ApplyResult, error) {
	info, _ := f.Info(data)
	if info.Name == "apply-error" {
		return "", fmt.Errorf("cannot apply resource")
	}
	return resource.UnknownApplyResult, nil
}

func (f *fakeHelper) Info(data []byte) (*resource.Info, error) {
	r, obj, _ := decode(f.t, data)
	// Special case when the object's name is info-error
	if obj.GetName() == "info-error" {
		return nil, fmt.Errorf("cannot determine info")
	}

	return &resource.Info{
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		Kind:       r.GetObjectKind().GroupVersionKind().Kind,
		APIVersion: r.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Resource:   r.GetObjectKind().GroupVersionKind().Kind,
	}, nil
}

func (f *fakeHelper) Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType string) error {
	p := string(patch)
	if strings.Contains(p, "patch-error") {
		return fmt.Errorf("cannot apply patch")
	}
	return nil
}

func validateSyncSetObjectStatus(t *testing.T, actual, expected []hivev1.SyncSetObjectStatus) {
	if len(actual) != len(expected) {
		t.Errorf("actual status length does not match expected")
		return
	}
	for _, actualStatus := range actual {
		found := false
		for _, expectedStatus := range expected {
			if expectedStatus.Name == actualStatus.Name {
				found = true
				validateSyncSetStatus(t, actualStatus, expectedStatus)
				break
			}
		}
		if !found {
			t.Errorf("got unexpected syncset object status: %s", actualStatus.Name)
		}
	}
}

func validateSyncSetStatus(t *testing.T, actual, expected hivev1.SyncSetObjectStatus) {
	if len(actual.Resources) != len(expected.Resources) {
		t.Errorf("number of resource statuses does not match, actual: %d, expected: %d", len(actual.Resources), len(expected.Resources))
		return
	}
	if len(actual.Patches) != len(expected.Patches) {
		t.Errorf("number of patch statuses does not match, actual %d, expected: %d", len(actual.Patches), len(expected.Patches))
	}

	for _, actualResource := range actual.Resources {
		found := false
		for _, expectedResource := range expected.Resources {
			if matchesResourceStatus(actualResource, expectedResource) {
				found = true
				validateSyncStatus(t, actual.Name, actualResource, expectedResource)
				break
			}
		}
		if !found {
			t.Errorf("got unexpected syncset %s resource status: %s/%s (kind: %s, apiVersion: %s)",
				actual.Name, actualResource.Namespace, actualResource.Name, actualResource.Kind, actualResource.APIVersion)
		}
	}

	for _, actualPatch := range actual.Patches {
		found := false
		for _, expectedPatch := range expected.Patches {
			if matchesPatchStatus(actualPatch, expectedPatch) {
				found = true
				validateSyncStatus(t, actual.Name, actualPatch, expectedPatch)
				break
			}
		}
		if !found {
			t.Errorf("got unexpected syncset %s patch status: %s/%s (kind: %s, apiVersion: %s)",
				actual.Name, actualPatch.Namespace, actualPatch.Name, actualPatch.Kind, actualPatch.APIVersion)
		}
	}
}

func matchesResourceStatus(a, b hivev1.SyncStatus) bool {
	return a.Name == b.Name &&
		a.Namespace == b.Namespace &&
		a.Kind == b.Kind &&
		a.APIVersion == b.APIVersion
}

func matchesPatchStatus(a, b hivev1.SyncStatus) bool {
	return a.Name == b.Name &&
		a.Namespace == b.Namespace &&
		a.Kind == b.Kind &&
		a.APIVersion == b.APIVersion
}

func validateSyncStatus(t *testing.T, name string, actual, expected hivev1.SyncStatus) {
	if len(actual.Conditions) != len(expected.Conditions) {
		t.Errorf("number of conditions do not match for syncset %s resource %s/%s (kind: %s, apiVersion: %s). Expected: %d, Actual: %d",
			name, actual.Namespace, actual.Name, actual.Kind, actual.APIVersion, len(expected.Conditions), len(actual.Conditions))
		return
	}
	if actual.Hash != expected.Hash {
		t.Errorf("hashes don't match for syncset %s resource %s/%s (kind: %s, apiVersion: %s). Expected: %s, Actual: %s",
			name, actual.Namespace, actual.Name, actual.Kind, actual.APIVersion, expected.Hash, actual.Hash)
		return
	}
	for _, actualCondition := range actual.Conditions {
		found := false
		for _, expectedCondition := range expected.Conditions {
			if actualCondition.Type == expectedCondition.Type {
				found = true
				if actualCondition.Status != expectedCondition.Status {
					t.Errorf("condition does not match, syncset %s, resource %s/%s (kind: %s, apiVersion: %s), condition %s. Expected: %s, Actual: %s",
						name, actual.Namespace, actual.Name, actual.Kind, actual.APIVersion, actualCondition.Type, expectedCondition.Status, actualCondition.Status)
				}
			}
			if !found {
				t.Errorf("got unexpected condition %s in syncset %s resource %s/%s (kind: %s, apiVersion: %s)",
					actualCondition.Type, name, actual.Namespace, actual.Name, actual.Kind, actual.APIVersion)
			}
		}
	}
}

func validateUnknownObjectCondition(t *testing.T, status hivev1.SyncSetObjectStatus) {
	if len(status.Conditions) != 1 {
		t.Errorf("did not get the expected number of syncset level conditions (1)")
		return
	}
	condition := status.Conditions[0]
	if condition.Type != hivev1.UnknownObjectSyncCondition {
		t.Errorf("Unexpected type for syncset level condition: %s", condition.Type)
	}
	if condition.Status != corev1.ConditionTrue {
		t.Errorf("Unexpected condition status: %s", condition.Status)
	}
}

func decode(t *testing.T, data []byte) (runtime.Object, metav1.Object, error) {
	decoder := scheme.Codecs.UniversalDecoder(corev1.SchemeGroupVersion)
	r, _, err := decoder.Decode(data, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	obj, err := meta.Accessor(r)
	if err != nil {
		return nil, nil, err
	}
	return r, obj, nil
}

func fakeHashFunc(t *testing.T) func([]byte) string {
	return func(data []byte) string {
		_, obj, err := decode(t, data)
		if err != nil {
			return fmt.Sprintf("%x", md5.Sum(data))
		}
		return objectHash(obj)
	}
}

func objectHash(obj metav1.Object) string {
	if annotations := obj.GetAnnotations(); annotations != nil {
		if hash, ok := annotations["hash"]; ok {
			return hash
		}
	}
	return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
}

type deletedItemInfo struct {
	name      string
	namespace string
	resource  string
	group     string
	version   string
}

type fakeDynamicClient struct {
	deletedItems []deletedItemInfo
}

type fakeNamespaceableClient struct {
	client    *fakeDynamicClient
	resource  schema.GroupVersionResource
	namespace string
}

func (c *fakeDynamicClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &fakeNamespaceableClient{
		client:   c,
		resource: resource,
	}
}

func (c *fakeNamespaceableClient) Namespace(ns string) dynamic.ResourceInterface {
	return &fakeNamespaceableClient{
		client:    c.client,
		resource:  c.resource,
		namespace: ns,
	}
}

func (c *fakeNamespaceableClient) Create(obj *unstructured.Unstructured, options metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (c *fakeNamespaceableClient) Update(obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (c *fakeNamespaceableClient) UpdateStatus(obj *unstructured.Unstructured, options metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (c *fakeNamespaceableClient) Delete(name string, options *metav1.DeleteOptions, subresources ...string) error {
	if name == "delete-error" {
		return fmt.Errorf("cannot delete resource")
	}

	c.client.deletedItems = append(c.client.deletedItems, deletedItemInfo{
		name:      name,
		namespace: c.namespace,
		resource:  c.resource.Resource,
		group:     c.resource.Group,
		version:   c.resource.Version,
	})
	return nil
}

func (c *fakeNamespaceableClient) DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	return nil
}

func (c *fakeNamespaceableClient) Get(name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (c *fakeNamespaceableClient) List(opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return nil, nil
}

func (c *fakeNamespaceableClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (c *fakeNamespaceableClient) Patch(name string, pt types.PatchType, data []byte, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
