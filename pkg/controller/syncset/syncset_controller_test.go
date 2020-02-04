package syncset

import (
	"context"
	"reflect"
	"testing"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

const (
	testName      = "test-clusterdeployment"
	testNamespace = "test-namespace"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestReconcileSyncSet(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	ss := testMatchingSyncSet
	sx := testNonMatchingSyncSet
	sss := testMatchingSelectorSyncSet
	ssx := testNonMatchingSelectorSyncSet

	si := testSyncSetInstanceForSyncSet
	ssi := testSyncSetInstanceForSelectorSyncSet

	up := hivev1.UpsertResourceApplyMode
	sy := hivev1.SyncResourceApplyMode

	tests := []struct {
		name     string
		existing []runtime.Object
		expected []*hivev1.SyncSetInstance
	}{
		{
			name: "add syncsetinstances",
			existing: []runtime.Object{
				ss("a"), ss("b"), ss("c"),
				sss("aa"), sss("bb"), sss("cc"),
			},
			expected: []*hivev1.SyncSetInstance{
				si("a"), si("b"), si("c"),
				ssi("aa"), ssi("bb"), ssi("cc"),
			},
		},
		{
			name: "exclude non-matching syncsets",
			existing: []runtime.Object{
				ss("a"), sx("b"), ss("c"),
				sss("aa"), sss("bb"), ssx("c"),
			},
			expected: []*hivev1.SyncSetInstance{
				si("a"), si("c"), ssi("aa"), ssi("bb"),
			},
		},
		{
			name: "delete outdated syncset instances",
			existing: []runtime.Object{
				ss("a"), ss("b"), ss("c"),
				si("a"), si("b"), si("c"), si("e"), si("f"),
			},
			expected: []*hivev1.SyncSetInstance{
				si("a"), si("b"), si("c"),
			},
		},
		{
			name: "delete and create syncset instances",
			existing: []runtime.Object{
				sss("a"), sss("b"), sss("c"),
				ssi("a"), ssi("c"), ssi("e"), ssi("f"),
			},
			expected: []*hivev1.SyncSetInstance{
				ssi("a"), ssi("b"), ssi("c"),
			},
		},
		{
			name: "update syncset instances",
			existing: []runtime.Object{
				ss("a", up), ss("b", sy),
				si("a", sy), si("b", up),
			},
			expected: []*hivev1.SyncSetInstance{
				si("a", up), si("b", sy),
			},
		},
		{
			name: "multiple changes",
			existing: []runtime.Object{
				ss("a", up), ss("b", sy),
				sss("aa", sy), sss("bb", up), sss("cc", up),

				si("a", up), si("b", up), si("c", up),
				ssi("bb", sy), ssi("cc", up),
			},
			expected: []*hivev1.SyncSetInstance{
				si("a", up), si("b", sy),
				ssi("aa", sy), ssi("bb", up), ssi("cc", up),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cd := testClusterDeployment()
			objs := append(test.existing, cd)
			fakeClient := fake.NewFakeClient(objs...)
			rss := &ReconcileSyncSet{
				Client:      fakeClient,
				scheme:      scheme.Scheme,
				logger:      log.WithField("controller", "syncset"),
				computeHash: testHashCompute,
			}
			_, err := rss.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			validateExpected(t, fakeClient, test.expected)
		})
	}
}

func validateExpected(t *testing.T, c client.Client, expected []*hivev1.SyncSetInstance) {
	list := &hivev1.SyncSetInstanceList{}
	err := c.List(context.TODO(), list, client.InNamespace(testNamespace))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedInstances := []hivev1.SyncSetInstance{}

	for _, instance := range expected {
		expectedInstances = append(expectedInstances, *instance)
		if !containsMatchingInstance(*instance, list.Items) {
			t.Errorf("did not get expected syncset instance: %s", instance.Name)
		}
	}

	for _, instance := range list.Items {
		if !containsMatchingInstance(instance, expectedInstances) {
			t.Errorf("found unexpected syncset instance: %s", instance.Name)
		}
	}
}

func containsMatchingInstance(instance hivev1.SyncSetInstance, list []hivev1.SyncSetInstance) bool {
	for _, i := range list {
		if instance.Name == i.Name {
			if reflect.DeepEqual(instance.Spec, i.Spec) &&
				reflect.DeepEqual(instance.Labels, i.Labels) {
				return true
			}
		}
	}
	return false
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
			Labels:    map[string]string{"foo": "bar"},
		},
		Spec: hivev1.ClusterDeploymentSpec{
			Installed: true,
		},
	}
	return cd
}

func getMode(mode []hivev1.SyncSetResourceApplyMode) hivev1.SyncSetResourceApplyMode {
	if len(mode) == 0 {
		return hivev1.UpsertResourceApplyMode
	}
	return mode[0]
}

func testMatchingSyncSet(name string, applyMode ...hivev1.SyncSetResourceApplyMode) *hivev1.SyncSet {
	return &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      name,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: testName,
				},
			},
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				ResourceApplyMode: getMode(applyMode),
			},
		},
	}
}

func testNonMatchingSyncSet(name string, applyMode ...hivev1.SyncSetResourceApplyMode) *hivev1.SyncSet {
	ss := testMatchingSyncSet(name, applyMode...)
	ss.Spec.ClusterDeploymentRefs[0].Name = "different-name"
	return ss
}

func testMatchingSelectorSyncSet(name string, applyMode ...hivev1.SyncSetResourceApplyMode) *hivev1.SelectorSyncSet {
	return &hivev1.SelectorSyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: hivev1.SelectorSyncSetSpec{
			ClusterDeploymentSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				ResourceApplyMode: getMode(applyMode),
			},
		},
	}
}

func testNonMatchingSelectorSyncSet(name string, applyMode ...hivev1.SyncSetResourceApplyMode) *hivev1.SelectorSyncSet {
	sss := testMatchingSelectorSyncSet(name, applyMode...)
	sss.Spec.ClusterDeploymentSelector.MatchLabels = map[string]string{"bar": "baz"}
	return sss
}

func testSyncSetInstanceForSyncSet(name string, applyMode ...hivev1.SyncSetResourceApplyMode) *hivev1.SyncSetInstance {
	return &hivev1.SyncSetInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name: syncSetInstanceNameForSyncSet(
				testClusterDeployment(), testMatchingSyncSet(name)),
			Labels: map[string]string{
				constants.SyncSetNameLabel:           name,
				constants.ClusterDeploymentNameLabel: testName,
			},
		},
		Spec: hivev1.SyncSetInstanceSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: testName,
			},
			SyncSetRef: &corev1.LocalObjectReference{
				Name: name,
			},
			ResourceApplyMode: getMode(applyMode),
			SyncSetHash:       string(getMode(applyMode)),
		},
	}
}

func testSyncSetInstanceForSelectorSyncSet(name string, applyMode ...hivev1.SyncSetResourceApplyMode) *hivev1.SyncSetInstance {
	return &hivev1.SyncSetInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name: syncSetInstanceNameForSelectorSyncSet(
				testClusterDeployment(), testMatchingSelectorSyncSet(name)),
			Labels: map[string]string{
				constants.SelectorSyncSetNameLabel:   name,
				constants.ClusterDeploymentNameLabel: testName,
			},
		},
		Spec: hivev1.SyncSetInstanceSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: testName,
			},
			SelectorSyncSetRef: &hivev1.SelectorSyncSetReference{
				Name: name,
			},
			ResourceApplyMode: getMode(applyMode),
			SyncSetHash:       string(getMode(applyMode)),
		},
	}
}

func testHashCompute(obj interface{}) (string, error) {
	switch spec := obj.(type) {
	case hivev1.SyncSetSpec:
		return string(spec.ResourceApplyMode), nil
	case hivev1.SelectorSyncSetSpec:
		return string(spec.ResourceApplyMode), nil
	default:
		return "unknown", nil
	}
}
