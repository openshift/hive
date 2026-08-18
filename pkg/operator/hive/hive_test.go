package hive

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakekubeclient "k8s.io/client-go/kubernetes/fake"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/resource/mock"
)

func testLogger() log.FieldLogger {
	l := log.New()
	l.SetLevel(log.FatalLevel)
	return l
}

// componentPod builds a Pod carrying the hive.openshift.io/component label that
// Hive's workload pods (controllers/clustersync/machinepool/hiveadmission) all bear.
func componentPod(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels:    map[string]string{hiveComponentLabelKey: "hive-controllers"},
		},
	}
}

// unlabeledPod builds a Pod with no hive.openshift.io/component label (e.g. some
// unrelated pod that happens to live in the namespace).
func unlabeledPod(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
	}
}

func TestHivePodsGone(t *testing.T) {
	cases := []struct {
		name      string
		existing  []runtime.Object
		namespace string
		wantGone  bool
	}{
		{
			name:      "no pods at all",
			existing:  nil,
			namespace: "old",
			wantGone:  true,
		},
		{
			name:      "hive workload pod present",
			existing:  []runtime.Object{componentPod("old", "hive-controllers-abc")},
			namespace: "old",
			wantGone:  false,
		},
		{
			name:      "only unlabeled pods present",
			existing:  []runtime.Object{unlabeledPod("old", "something-else")},
			namespace: "old",
			wantGone:  true,
		},
		{
			name:      "hive workload pod only in a different namespace",
			existing:  []runtime.Object{componentPod("other", "hive-controllers-abc")},
			namespace: "old",
			wantGone:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &ReconcileHiveConfig{kubeClient: fakekubeclient.NewSimpleClientset(tc.existing...)}
			gone, err := r.hivePodsGone(tc.namespace, testLogger())
			require.NoError(t, err)
			assert.Equal(t, tc.wantGone, gone)
		})
	}
}

// TestScrubOldNamespaceNetworkPolicies_PodsPresent is the assertion that guards the
// fix: while a former target namespace still has Hive workload pods, its allow-all
// NetworkPolicies must NOT be deleted (deleting them would strand the terminating
// pods behind an admin's baseline deny-all). The strict mock has no Delete
// expectations, so any NetworkPolicy deletion fails the test.
func TestScrubOldNamespaceNetworkPolicies_PodsPresent(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mock.NewMockHelper(ctrl)

	r := &ReconcileHiveConfig{
		kubeClient: fakekubeclient.NewSimpleClientset(componentPod("old", "hive-controllers-abc")),
	}

	scrubbed, err := r.scrubOldNamespaceNetworkPolicies(h, &hivev1.HiveConfig{}, []string{"old"}, testLogger())
	require.NoError(t, err)
	assert.Empty(t, scrubbed, "namespace with live pods must not be reported as scrubbed")
}

// TestScrubOldNamespaceNetworkPolicies_PodsGone verifies that once the workload pods
// are gone, both allow-all NetworkPolicies are deleted from the namespace and the
// namespace is reported as fully scrubbed.
func TestScrubOldNamespaceNetworkPolicies_PodsGone(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mock.NewMockHelper(ctrl)
	// apiVersion/kind come from the decoded asset; match on namespace + name, which
	// are the values that actually matter here.
	h.EXPECT().Delete(gomock.Any(), gomock.Any(), "old", "hive-controllers").Return(nil)
	h.EXPECT().Delete(gomock.Any(), gomock.Any(), "old", "hiveadmission").Return(nil)

	r := &ReconcileHiveConfig{kubeClient: fakekubeclient.NewSimpleClientset()}

	scrubbed, err := r.scrubOldNamespaceNetworkPolicies(h, &hivev1.HiveConfig{}, []string{"old"}, testLogger())
	require.NoError(t, err)
	assert.Equal(t, []string{"old"}, scrubbed)
}

// TestScrubOldNamespaceNetworkPolicies_Mixed verifies per-namespace gating: a drained
// namespace is scrubbed while a namespace still hosting a workload pod is left for a
// later reconcile (and its NetworkPolicies are not touched).
func TestScrubOldNamespaceNetworkPolicies_Mixed(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mock.NewMockHelper(ctrl)
	// Only the drained namespace ("gone") gets NetworkPolicy deletions.
	h.EXPECT().Delete(gomock.Any(), gomock.Any(), "gone", "hive-controllers").Return(nil)
	h.EXPECT().Delete(gomock.Any(), gomock.Any(), "gone", "hiveadmission").Return(nil)

	r := &ReconcileHiveConfig{
		kubeClient: fakekubeclient.NewSimpleClientset(componentPod("busy", "hive-controllers-abc")),
	}

	scrubbed, err := r.scrubOldNamespaceNetworkPolicies(h, &hivev1.HiveConfig{}, []string{"gone", "busy"}, testLogger())
	require.NoError(t, err)
	assert.Equal(t, []string{"gone"}, scrubbed)
}
