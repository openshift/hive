package clusterstate

import (
	"context"
	"sort"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	testName                 = "cluster1"
	testNamespace            = "cluster1namespace"
	testKubeconfigSecretName = "kubeconfig-secret"
)

func TestClusterStateReconcile(t *testing.T) {

	log.SetLevel(log.DebugLevel)

	cs := func(t *testing.T, c client.Client) *hivev1.ClusterState {
		st := &hivev1.ClusterState{}
		err := c.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: testName}, st)
		switch {
		case errors.IsNotFound(err):
			return nil
		case err != nil:
			t.Fatalf("unexpected: %v", err)
		}
		return st
	}
	co := clusterOperator
	uco := unavailableClusterOperator

	tests := []struct {
		name         string
		existing     []runtime.Object
		remote       []runtime.Object
		noRemoteCall bool
		validate     func(*testing.T, client.Client, reconcile.Result)
		noUpdate     bool
	}{
		{
			name: "create cluster state",
			existing: []runtime.Object{
				testClusterDeployment(),
			},
			noRemoteCall: true,
			validate: func(t *testing.T, c client.Client, result reconcile.Result) {
				st := cs(t, c)
				require.NotNil(t, st, "clusterstate should have been created")
				assert.Equal(t, testClusterDeployment().Name, st.Labels[constants.ClusterDeploymentNameLabel], "incorrect cluster deployment name label")
			},
		},
		{
			name: "update empty clusterstate",
			existing: []runtime.Object{
				testClusterState(),
				testClusterDeployment(),
				testKubeconfigSecret(),
			},
			remote: []runtime.Object{co("a"), co("b"), co("c")},
			validate: func(t *testing.T, c client.Client, result reconcile.Result) {
				st := cs(t, c)
				validateStatus(t, st.Status, co("a"), co("b"), co("c"))
			},
		},
		{
			name: "steady state",
			existing: []runtime.Object{
				testClusterStateWithStatus(co("d"), co("e")),
				testClusterDeployment(),
				testKubeconfigSecret(),
			},
			remote: []runtime.Object{co("d"), co("e")},
			validate: func(t *testing.T, c client.Client, result reconcile.Result) {
				st := cs(t, c)
				validateStatus(t, st.Status, co("d"), co("e"))
				assert.Equal(t, result.RequeueAfter, statusUpdateInterval)
			},
			noUpdate: true,
		},
		{
			name: "changed state",
			existing: []runtime.Object{
				testClusterStateWithStatus(co("a"), co("b"), co("c")),
				testClusterDeployment(),
				testKubeconfigSecret(),
			},
			remote: []runtime.Object{co("a"), co("b"), uco("c")},
			validate: func(t *testing.T, c client.Client, result reconcile.Result) {
				st := cs(t, c)
				validateStatus(t, st.Status, co("a"), co("b"), uco("c"))
			},
		},
		{
			name: "removed remote co",
			existing: []runtime.Object{
				testClusterStateWithStatus(co("a"), co("b"), co("c")),
				testClusterDeployment(),
				testKubeconfigSecret(),
			},
			remote: []runtime.Object{co("a"), co("c")},
			validate: func(t *testing.T, c client.Client, result reconcile.Result) {
				st := cs(t, c)
				validateStatus(t, st.Status, co("a"), co("c"))
			},
		},
		{
			name: "added remote condition",
			existing: []runtime.Object{
				testClusterStateWithStatus(co("a"), co("b")),
				testClusterDeployment(),
				testKubeconfigSecret(),
			},
			remote: []runtime.Object{addCond(co("a")), co("b")},
			validate: func(t *testing.T, c client.Client, result reconcile.Result) {
				st := cs(t, c)
				validateStatus(t, st.Status, addCond(co("a")), co("b"))
			},
		},
		{
			name: "removed remote condition",
			existing: []runtime.Object{
				testClusterStateWithStatus(co("a"), co("b")),
				testClusterDeployment(),
				testKubeconfigSecret(),
			},
			remote: []runtime.Object{co("a"), removeCond(co("b"))},
			validate: func(t *testing.T, c client.Client, result reconcile.Result) {
				st := cs(t, c)
				validateStatus(t, st.Status, co("a"), removeCond(co("b")))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()
			mockCtrl := gomock.NewController(t)
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			if !test.noRemoteCall {
				mockRemoteClientBuilder.EXPECT().Build().Return(testfake.NewFakeClientBuilder().WithRuntimeObjects(test.remote...).Build(), nil)
			}
			updateCalled := false
			rcd := &ReconcileClusterState{
				Client:                        fakeClient,
				scheme:                        scheme,
				logger:                        log.WithField("controller", "clusterState"),
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
				updateStatus: func(c client.Client, st *hivev1.ClusterState) error {
					updateCalled = true
					return updateClusterStateStatus(c, st)
				},
			}

			result, err := rcd.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if test.noUpdate && updateCalled {
				t.Errorf("Update was called unexpectedly")
				return
			}
			if test.validate != nil {
				test.validate(t, fakeClient, result)
			}
		})
	}
}

func testClusterState() *hivev1.ClusterState {
	return &hivev1.ClusterState{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testName,
		},
	}
}

func testClusterStateWithStatus(operators ...*configv1.ClusterOperator) *hivev1.ClusterState {
	cs := testClusterState()
	for _, op := range operators {
		cs.Status.ClusterOperators = append(cs.Status.ClusterOperators, hivev1.ClusterOperatorState{
			Name:       op.Name,
			Conditions: op.Status.Conditions,
		})
	}
	return cs
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testName,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterMetadata: &hivev1.ClusterMetadata{
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{
					Name: testKubeconfigSecretName,
				},
			},
			Installed: true,
		},
		Status: hivev1.ClusterDeploymentStatus{
			Conditions: []hivev1.ClusterDeploymentCondition{{
				Type:   hivev1.UnreachableCondition,
				Status: corev1.ConditionFalse,
			}},
		},
	}
}

func testKubeconfigSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testKubeconfigSecretName,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte("kubeconfig-data"),
		},
	}
}

func clusterOperator(name string) *configv1.ClusterOperator {
	return &configv1.ClusterOperator{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: configv1.ClusterOperatorStatus{
			Conditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:    configv1.OperatorAvailable,
					Status:  configv1.ConditionTrue,
					Reason:  "Available",
					Message: "Available",
				},
				{
					Type:    configv1.OperatorProgressing,
					Status:  configv1.ConditionFalse,
					Reason:  "Available",
					Message: "Available",
				},
				{
					Type:    configv1.OperatorDegraded,
					Status:  configv1.ConditionFalse,
					Reason:  "Available",
					Message: "Available",
				},
			},
		},
	}
}

func unavailableClusterOperator(name string) *configv1.ClusterOperator {
	op := clusterOperator(name)
	op.Status.Conditions[0].Status = configv1.ConditionFalse
	op.Status.Conditions[2].Status = configv1.ConditionTrue
	return op
}

func addCond(co *configv1.ClusterOperator) *configv1.ClusterOperator {
	co.Status.Conditions = append(co.Status.Conditions, configv1.ClusterOperatorStatusCondition{
		Type:    configv1.OperatorUpgradeable,
		Status:  configv1.ConditionTrue,
		Reason:  "Available",
		Message: "Available",
	})
	return co
}

func removeCond(co *configv1.ClusterOperator) *configv1.ClusterOperator {
	co.Status.Conditions = co.Status.Conditions[1:]
	return co
}

func validateStatus(t *testing.T, status hivev1.ClusterStateStatus, operators ...*configv1.ClusterOperator) {
	if !assert.Len(t, status.ClusterOperators, len(operators)) {
		return
	}
	sort.Slice(status.ClusterOperators, func(i, j int) bool {
		return status.ClusterOperators[i].Name < status.ClusterOperators[j].Name
	})
	sort.Slice(operators, func(i, j int) bool {
		return operators[i].Name < operators[j].Name
	})
	for i := range status.ClusterOperators {
		assert.Equal(t, status.ClusterOperators[i].Name, operators[i].Name, "operator names don't match")
		assert.ElementsMatch(t, status.ClusterOperators[i].Conditions, operators[i].Status.Conditions, "operator conditions don't match")
	}
}
