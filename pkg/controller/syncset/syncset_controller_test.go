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
	"fmt"
	"strconv"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	}{
		{
			name:              "Create single resource successfully",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets:          []*hivev1.SyncSet{testSyncSet("ss1", testCM("cm1", "foo", "bar"))},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulStatus("ss1", []runtime.Object{testCM("cm1", "foo", "bar")}),
				})
			},
		},
		{
			name:              "Create multiple resources successfully",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSet("foo", testCMs("bar", 5)...),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulStatus("foo", testCMs("bar", 5)),
				})
			},
		},
		{
			name:              "Create resources from multiple syncsets",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSet("foo1", testCMs("foo1", 4)...),
				testSyncSet("foo2", testCMs("foo2", 3)...),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulStatus("foo1", testCMs("foo1", 4)),
					successfulStatus("foo2", testCMs("foo2", 3)),
				})
			},
		},
		{
			name: "Update single resource",
			clusterDeployment: testClusterDeployment([]hivev1.SyncSetObjectStatus{
				successfulStatus("ss1", []runtime.Object{testCM("cm1", "key1", "value1")}),
			}, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSet("ss1", testCM("cm1", "key1", "value***changed")),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulStatus("ss1", []runtime.Object{testCM("cm1", "key1", "value***changed")}),
				})
			},
		},
		{
			name: "Update only resources that have changed",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				fiveMinutesAgo := time.Unix(metav1.NewTime(time.Now().Add(-5*time.Minute)).Unix(), 0)
				cd := testClusterDeployment([]hivev1.SyncSetObjectStatus{
					successfulStatusWithTime("aaa", []runtime.Object{
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
				testSyncSet("aaa",
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value***changed"),
					testCM("cm3", "key3", "value3"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulStatus("aaa", []runtime.Object{
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
				testSyncSet("foo",
					testCM("cm1", "key1", "value1"),
					testCM("info-error", "key2", "value2"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateUnknownObjectCondition(t, cd.Status.SyncSetStatus[0])
			},
		},
		{
			name:              "Check for failed info call, set condition and process other syncsets",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSet("foo", testCM("info-error", "key1", "value1")),
				testSyncSet("bar", testCM("valid", "key2", "value2")),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				for _, ss := range cd.Status.SyncSetStatus {
					if ss.Name == "foo" {
						validateUnknownObjectCondition(t, ss)
					} else if ss.Name == "bar" {
						validateSyncSetStatus(t, ss, successfulStatus("bar", []runtime.Object{testCM("valid", "key2", "value2")}))
					} else {
						t.Errorf("unexpected syncset status: %s", ss.Name)
					}
				}
			},
		},
		{
			name:              "Stop applying resources when error occurs",
			clusterDeployment: testClusterDeployment(nil, nil),
			syncSets: []*hivev1.SyncSet{
				testSyncSet("foo",
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value2"),
					testCM("apply-error", "key3", "value3"),
					testCM("cm4", "key4", "value4"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				status := successfulStatus("foo", []runtime.Object{
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value2"),
				})
				status.Resources = append(status.Resources, failedStatus("foo", []runtime.Object{
					testCM("apply-error", "key3", "value3"),
				}).Resources...)
				validateSyncSetObjectStatus(t, cd.Status.SyncSetStatus, []hivev1.SyncSetObjectStatus{status})
			},
		},
		{
			name:              "selectorsyncset: apply single resource",
			clusterDeployment: testClusterDeployment(nil, nil),
			selectorSyncSets: []*hivev1.SelectorSyncSet{
				testMatchingSelectorSyncSet("foo",
					testCM("cm1", "key1", "value1"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SelectorSyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulStatus("foo", []runtime.Object{
						testCM("cm1", "key1", "value1"),
					}),
				})
			},
		},
		{
			name:              "selectorsyncset: apply only matching",
			clusterDeployment: testClusterDeployment(nil, nil),
			selectorSyncSets: []*hivev1.SelectorSyncSet{
				testMatchingSelectorSyncSet("foo",
					testCM("cm1", "key1", "value1"),
				),
				testNonMatchingSelectorSyncSet("bar",
					testCM("cm2", "key2", "value2"),
				),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				validateSyncSetObjectStatus(t, cd.Status.SelectorSyncSetStatus, []hivev1.SyncSetObjectStatus{
					successfulStatus("foo", []runtime.Object{
						testCM("cm1", "key1", "value1"),
					}),
				})
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

			helper := &fakeHelper{t: t}
			rcd := &ReconcileSyncSet{
				Client:         fakeClient,
				scheme:         scheme.Scheme,
				logger:         log.WithField("controller", "syncset"),
				applierBuilder: helper.newHelper,
				hash:           fakeHashFunc(t),
			}
			_, err := rcd.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
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

func testSyncSet(name string, resources ...runtime.Object) *hivev1.SyncSet {
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

func testMatchingSelectorSyncSet(name string, resources ...runtime.Object) *hivev1.SelectorSyncSet {
	return testSelectorSyncSet(name, map[string]string{"region": "us-east-1"}, resources...)
}

func testNonMatchingSelectorSyncSet(name string, resources ...runtime.Object) *hivev1.SelectorSyncSet {
	return testSelectorSyncSet(name, map[string]string{"region": "us-west-2"}, resources...)
}

func testSelectorSyncSet(name string, matchLabels map[string]string, resources ...runtime.Object) *hivev1.SelectorSyncSet {
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

func failedStatus(name string, resources []runtime.Object) hivev1.SyncSetObjectStatus {
	conditionTime := metav1.Now()
	status := hivev1.SyncSetObjectStatus{
		Name: name,
	}
	for _, r := range resources {
		obj, _ := meta.Accessor(r)
		status.Resources = append(status.Resources, hivev1.SyncStatus{
			APIVersion: r.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       r.GetObjectKind().GroupVersionKind().Kind,
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

func successfulStatus(name string, resources []runtime.Object) hivev1.SyncSetObjectStatus {
	return successfulStatusWithTime(name, resources, metav1.Now())
}

func successfulStatusWithTime(name string, resources []runtime.Object, conditionTime metav1.Time) hivev1.SyncSetObjectStatus {
	status := hivev1.SyncSetObjectStatus{
		Name: name,
	}
	for _, r := range resources {
		obj, _ := meta.Accessor(r)
		status.Resources = append(status.Resources, hivev1.SyncStatus{
			APIVersion: r.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       r.GetObjectKind().GroupVersionKind().Kind,
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

type fakeHelper struct {
	t *testing.T
}

func (f *fakeHelper) newHelper(kubeconfig []byte, logger log.FieldLogger) Applier {
	return f
}

func (f *fakeHelper) Apply(data []byte) error {
	info, _ := f.Info(data)
	if info.Name == "apply-error" {
		return fmt.Errorf("cannot apply resource")
	}
	return nil
}

func (f *fakeHelper) Info(data []byte) (*resource.Info, error) {
	r, obj := decode(f.t, data)
	// Special case when the object's name is info-error
	if obj.GetName() == "info-error" {
		return nil, fmt.Errorf("cannot determine info")
	}
	return &resource.Info{
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		Kind:       r.GetObjectKind().GroupVersionKind().Kind,
		APIVersion: r.GetObjectKind().GroupVersionKind().GroupVersion().String(),
	}, nil
}

func (f *fakeHelper) Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType types.PatchType) error {
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
}

func matchesResourceStatus(a, b hivev1.SyncStatus) bool {
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

func decode(t *testing.T, data []byte) (runtime.Object, metav1.Object) {
	decoder := scheme.Codecs.UniversalDecoder(corev1.SchemeGroupVersion)
	r, _, err := decoder.Decode(data, nil, nil)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	obj, err := meta.Accessor(r)
	if err != nil {
		t.Fatalf("accessor error: %v", err)
	}
	return r, obj
}

func fakeHashFunc(t *testing.T) func([]byte) string {
	return func(data []byte) string {
		_, obj := decode(t, data)
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
