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

// pod builds a Pod in the given namespace. If hiveComponent is true it carries the
// hive.openshift.io/component label that Hive's workload pods
// (controllers/clustersync/machinepool/hiveadmission) all bear; otherwise it is an
// unrelated pod that happens to live in the namespace.
func pod(namespace, name string, hiveComponent bool) *corev1.Pod {
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}}
	if hiveComponent {
		p.Labels = map[string]string{hiveComponentLabelKey: "hive-controllers"}
	}
	return p
}

// TestScrubOldNamespaceNetworkPolicies covers the gating contract that the fix
// depends on: a former target namespace's NetworkPolicies are deleted only once its
// Hive workload pods are gone. This also transitively exercises hivePodsGone,
// including its label selectivity (unrelated pods must not block scrubbing).
func TestScrubOldNamespaceNetworkPolicies(t *testing.T) {
	const ns = "old"
	cases := []struct {
		name string
		// existing pods seeded into the (former target) namespace.
		existing []runtime.Object
		// expectScrubbed is whether the namespace should be fully scrubbed -- i.e. its
		// NetworkPolicies deleted and the namespace reported back to the caller.
		expectScrubbed bool
	}{
		{
			name:           "workload pods present -> netpols retained, ns not scrubbed",
			existing:       []runtime.Object{pod(ns, "hive-controllers-abc", true)},
			expectScrubbed: false,
		},
		{
			name:           "no pods -> both netpols deleted, ns scrubbed",
			existing:       nil,
			expectScrubbed: true,
		},
		{
			name:           "only unrelated pods -> both netpols deleted, ns scrubbed",
			existing:       []runtime.Object{pod(ns, "something-else", false)},
			expectScrubbed: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			h := mock.NewMockHelper(ctrl)
			if tc.expectScrubbed {
				// apiVersion/kind come from the decoded asset; match on namespace + name.
				h.EXPECT().Delete(gomock.Any(), gomock.Any(), ns, "hive-controllers").Return(nil)
				h.EXPECT().Delete(gomock.Any(), gomock.Any(), ns, "hiveadmission").Return(nil)
			}
			// When not expectScrubbed, the strict mock has no Delete expectations, so any
			// NetworkPolicy deletion fails the test -- this is the assertion that guards
			// against deleting a NetworkPolicy while workload pods still exist.

			r := &ReconcileHiveConfig{kubeClient: fakekubeclient.NewSimpleClientset(tc.existing...)}
			scrubbed, err := r.scrubOldNamespaceNetworkPolicies(h, &hivev1.HiveConfig{}, []string{ns}, testLogger())
			require.NoError(t, err)
			if tc.expectScrubbed {
				assert.Equal(t, []string{ns}, scrubbed)
			} else {
				assert.Empty(t, scrubbed, "namespace with live pods must not be reported as scrubbed")
			}
		})
	}
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
		kubeClient: fakekubeclient.NewSimpleClientset(pod("busy", "hive-controllers-abc", true)),
	}

	scrubbed, err := r.scrubOldNamespaceNetworkPolicies(h, &hivev1.HiveConfig{}, []string{"gone", "busy"}, testLogger())
	require.NoError(t, err)
	assert.Equal(t, []string{"gone"}, scrubbed)
}
