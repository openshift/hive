package syncsets_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	kclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/fields"
	clientwatch "k8s.io/client-go/tools/watch"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/util/scheme"
	"github.com/openshift/hive/test/e2e/common"
)

func TestSyncsets(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Syncsets Suite")
}

var (
	clusterName      = os.Getenv("CLUSTER_NAME")
	clusterNamespace = os.Getenv("CLUSTER_NAMESPACE")
)

const (
	// secretMappingSourceSecretName is the name of the synthetic source Secret used by the
	// secretMapping refresh tests. These tests use a dedicated Secret -- rather than the
	// ClusterDeployment's pull secret, as the other secretMapping test does -- so that they can
	// freely mutate its contents without affecting the cluster under test.
	secretMappingSourceSecretName = "test-secretmapping-source"
	// secretMappingTargetSecretName is the name of the Secret the SecretMapping is expected to
	// create on the target cluster.
	secretMappingTargetSecretName = "test-secretmapping-copy"
	// secretMappingTargetNamespace is the namespace of the mapped Secret on the target cluster.
	secretMappingTargetNamespace = "default"

	// secretMappingApplyTimeout bounds the wait for the first apply of a newly created SyncSet to
	// land the contents of the source Secret on the target cluster.
	secretMappingApplyTimeout = 3 * time.Minute
	// secretMappingLeaseTimeout bounds the wait for the ClusterSyncLease to appear, both before it
	// is deleted and afterwards, when the clustersync controller recreates it having re-applied
	// every SyncSet.
	secretMappingLeaseTimeout = 5 * time.Minute
	// secretMappingConfirmTimeout bounds the wait for the mapped Secret once a full re-apply has
	// completed. The re-apply writes the Secret before it writes the lease, so this only absorbs
	// the round trip to the target cluster.
	secretMappingConfirmTimeout = 30 * time.Second
	// secretMappingPollInterval is the polling interval used while waiting for the target cluster
	// and the ClusterSyncLease to reach an expected state.
	secretMappingPollInterval = 5 * time.Second
	// secretMappingMaxReapplyAttempts bounds the number of full re-applies forced by
	// reapplyUntilTargetSecretMatches.
	secretMappingMaxReapplyAttempts = 3

	// secretMappingArtifactTimeout bounds the collection of diagnostics, so that a cluster which
	// has become unreachable -- a plausible reason for the spec to have failed in the first place
	// -- cannot leave the suite blocked in its capture path.
	secretMappingArtifactTimeout = 2 * time.Minute
	// secretMappingLogLimitBytes caps the clustersync log captured per pod, so that a long job
	// cannot produce an unbounded artifact. An oversized log is truncated at the end.
	secretMappingLogLimitBytes = 5 << 20
	// secretMappingLogSkewMargin extends the window of captured logs a little further back than
	// the start of the spec, because the log is selected by the kubelet using node clocks.
	secretMappingLogSkewMargin = 30 * time.Second

	// Stages at which a spec captures the state of the scenario.
	artifactStageBefore = "before"
	artifactStageAfter  = "after"
)

var _ = Describe("Test Syncset and SelectorSyncSet func", func() {

	hiveClient := common.MustGetClient()
	targetClusterClient := common.MustGetClientFromConfig(common.MustGetClusterDeploymentClientConfig())
	// Used to collect clustersync controller logs as test artifacts.
	kubeClient := common.MustGetKubernetesClient()

	// release resource after test
	AfterEach(func() {
		deleteAllSyncSets(hiveClient, clusterNamespace)
		deleteAllSelectorSyncSets(hiveClient, clusterNamespace)
		deleteConfigMap(targetClusterClient, "default", "foo")
		deleteSecret(targetClusterClient, "default", "test-aws-creds")
		deleteSecret(targetClusterClient, secretMappingTargetNamespace, secretMappingTargetSecretName)
		deleteSecret(hiveClient, clusterNamespace, secretMappingSourceSecretName)
	})
	Describe("Test SyncSet", func() {
		Context("Test SynSet of resources,patches,and secretMappings", func() {
			It("Test SynSet resources", func() {
				ctx := context.TODO()
				testSyncSet := func(applyMode hivev1.SyncSetResourceApplyMode) *hivev1.SyncSet {
					return &hivev1.SyncSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-syncresource",
							Namespace: clusterNamespace,
						},
						Spec: hivev1.SyncSetSpec{
							ClusterDeploymentRefs: []corev1.LocalObjectReference{
								{
									Name: clusterName,
								},
							},
							SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
								ResourceApplyMode: applyMode,
								Resources: []runtime.RawExtension{
									{
										Object: &corev1.ConfigMap{
											TypeMeta: metav1.TypeMeta{
												Kind:       "ConfigMap",
												APIVersion: "v1",
											},
											ObjectMeta: metav1.ObjectMeta{
												Name:      "foo",
												Namespace: "default",
											},
											Data: map[string]string{
												"foo": "bar",
											},
										},
									},
								},
							},
						},
					}
				}

				By("Create a syncset resources using 'Sync' apply mode and verify syncset applied successfully")
				syncSetWithSyncApplyMode := testSyncSet(hivev1.SyncResourceApplyMode)
				err := hiveClient.Create(ctx, syncSetWithSyncApplyMode)
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetApplied(clusterNamespace, clusterName, "test-syncresource", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the resource is synced on target cluster")
				resultConfigMap := &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

				By("Delete the syncset and verify syncset deleted and syncset disassociated with cluster")
				deleteSyncSets(hiveClient, clusterNamespace, "test-syncresource")
				err = waitForSyncSetDeleted(clusterNamespace, "test-syncresource")
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetDisassociated(clusterNamespace, clusterName, "test-syncresource", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the ConfigMap is deleted on the target cluster")
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(errors.IsNotFound(err)).Should(BeTrue())

				By("Create a syncset resource using 'Upsert' apply mode, verify syncset applied successfully")
				syncSetWithUpsertApplyMode := testSyncSet(hivev1.UpsertResourceApplyMode)
				err = hiveClient.Create(ctx, syncSetWithUpsertApplyMode)
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetApplied(clusterNamespace, clusterName, "test-syncresource", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the resource is synced on target cluster")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

				By("Verify the managed-by-Hive label was injected automatically")
				Ω(resultConfigMap.Labels[constants.HiveManagedLabel]).Should(Equal("true"))

				By("Delete the syncset and verify syncset deleted and syncset disassociated with cluster")
				deleteSyncSets(hiveClient, clusterNamespace, "test-syncresource")
				err = waitForSyncSetDeleted(clusterNamespace, "test-syncresource")
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetDisassociated(clusterNamespace, clusterName, "test-syncresource", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the ConfigMap won't delete on the target cluster")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

			})

			It("Test SyncSet patches", func() {
				ctx := context.TODO()
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "default",
					},
					Data: map[string]string{
						"foo": "bar",
					},
				}
				testSyncSetPatch := func(patch, patchtype string) *hivev1.SyncSet {
					return &hivev1.SyncSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-syncpatch",
							Namespace: clusterNamespace,
						},
						Spec: hivev1.SyncSetSpec{
							ClusterDeploymentRefs: []corev1.LocalObjectReference{
								{
									Name: clusterName,
								},
							},
							SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
								ResourceApplyMode: "Sync",
								Patches: []hivev1.SyncObjectPatch{
									{
										APIVersion: "v1",
										Kind:       "ConfigMap",
										Name:       "foo",
										Namespace:  "default",
										Patch:      patch,
										PatchType:  patchtype,
									},
								},
							},
						},
					}
				}
				By("Create a resource ConfigMap on target cluster and verify create successfully")
				err := targetClusterClient.Create(ctx, configMap)
				Ω(err).ShouldNot(HaveOccurred())
				resultConfigMap := &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

				By("Create a syncpatch with merge patchType and verify syncpatch created successfully")
				patch := `{ "data": { "foo": "baz-merge" } }`
				patchType := "merge"
				syncPatchWithMergeType := testSyncSetPatch(patch, patchType)
				err = hiveClient.Create(ctx, syncPatchWithMergeType)
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetApplied(clusterNamespace, clusterName, "test-syncpatch", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the resource is patched on target cluster")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "baz-merge"}))

				By("Delete the SyncSetPatch and verify syncset deleted and syncset disassociated with cluster")
				deleteSyncSets(hiveClient, clusterNamespace, "test-syncpatch")
				err = waitForSyncSetDeleted(clusterNamespace, "test-syncpatch")
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetDisassociated(clusterNamespace, clusterName, "test-syncpatch", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Create a syncpatch with strategic patchType and verify syncpatch create successfully")
				patch = `{ "data": { "foo": "baz-strategic" } }`
				patchType = "strategic"
				syncPatchWithstrategicType := testSyncSetPatch(patch, patchType)
				err = hiveClient.Create(ctx, syncPatchWithstrategicType)
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetApplied(clusterNamespace, clusterName, "test-syncpatch", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the resource is patched on target cluster")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "baz-strategic"}))

				By("Delete the SyncSetPatch and verify syncset deleted and syncset disassociated with cluster")
				deleteSyncSets(hiveClient, clusterNamespace, "test-syncpatch")
				err = waitForSyncSetDeleted(clusterNamespace, "test-syncpatch")
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetDisassociated(clusterNamespace, clusterName, "test-syncpatch", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Create a syncpatch with json patchType and verify syncpatch created successfully")
				patch = `[ { "op": "replace", "path": "/data/foo", "value": "baz-json" } ]`
				patchType = "json"
				syncPatchWithPatchType := testSyncSetPatch(patch, patchType)
				err = hiveClient.Create(ctx, syncPatchWithPatchType)
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetApplied(clusterNamespace, clusterName, "test-syncpatch", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the resource is patched on target cluster")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "baz-json"}))
			})

			It("Test syncSet secretMappings", func() {
				ctx := context.TODO()
				By("Getting the pull secret name of clusterdeployment")
				cd := &hivev1.ClusterDeployment{}
				err := hiveClient.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, cd)
				Ω(err).ShouldNot(HaveOccurred())
				pullSecret := cd.Spec.PullSecretRef.Name

				syncSetSecretMappings := &hivev1.SyncSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-syncsecret",
						Namespace: clusterNamespace,
					},
					Spec: hivev1.SyncSetSpec{
						ClusterDeploymentRefs: []corev1.LocalObjectReference{
							{
								Name: clusterName,
							},
						},
						SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
							ResourceApplyMode: hivev1.SyncResourceApplyMode,
							Secrets: []hivev1.SecretMapping{
								{
									SourceRef: hivev1.SecretReference{
										Name:      pullSecret,
										Namespace: clusterNamespace,
									},
									TargetRef: hivev1.SecretReference{
										Name:      "test-pull-secret-copy",
										Namespace: "default",
									},
								},
							},
						},
					},
				}
				By("Create a syncSet SecretMappings and verify syncset is created successfully")
				err = hiveClient.Create(ctx, syncSetSecretMappings)
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetApplied(clusterNamespace, clusterName, "test-syncsecret", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the secret is copied to target cluster")
				resultSecret := &corev1.Secret{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "test-pull-secret-copy", Namespace: "default"}, resultSecret)
				Ω(err).ShouldNot(HaveOccurred())

			})

			// A SyncSet whose only change is to the contents of a Secret it maps is not reapplied
			// when the source Secret changes: the clustersync controller does not watch source
			// Secrets, and the SyncSet generation does not change. The mapped Secret is refreshed
			// by a full re-apply, which happens periodically (every reapplyInterval, two hours by
			// default) or as soon as the ClusterSyncLease for the cluster is deleted. This test
			// covers that refresh path and asserts the contents that actually land on the target
			// cluster, not merely that the SyncSet is reported as applied.
			//
			// One source secret update is followed by exactly one lease deletion, and the spec
			// only reads from then on. A refresh the single re-apply does not deliver therefore
			// fails the spec. That is the intent, and it has a consequence worth stating: the
			// controller reads the source secret through a cache, so a re-apply which starts
			// before the update has propagated to that cache would also fail this spec, and the
			// assertions cannot tell that apart from a refresh the controller genuinely missed.
			// Neither can the artifacts captured below, on their own: they record the state of
			// the objects before and after, the times at which the spec observed them and what
			// the controller logged, but not the revision of the source secret the controller
			// actually read. They are material for an investigation, not proof of which of the
			// two happened. Deleting the lease again until the contents match would hide both.
			It("Test syncSet secretMappings refresh the target secret after a single source secret update and ClusterSyncLease deletion", func() {
				ctx := context.TODO()
				scenarioStart := time.Now()
				syncSetName := "test-syncsecret-refresh"
				initialData := secretMappingData("value-a")
				updatedData := secretMappingData("value-b")

				// Capture the state of the scenario on the way out, on success as well as on
				// failure, and before AfterEach removes the objects.
				defer captureSecretMappingArtifacts(hiveClient, targetClusterClient, kubeClient, syncSetName, artifactStageAfter, scenarioStart)

				createSecretMappingSyncSet(hiveClient, targetClusterClient, kubeClient, syncSetName, initialData, scenarioStart)

				By("Record the syncSet so we can show the syncSet itself is not modified")
				syncSetBefore := &hivev1.SyncSet{}
				err := hiveClient.Get(ctx, client.ObjectKey{Name: syncSetName, Namespace: clusterNamespace}, syncSetBefore)
				Ω(err).ShouldNot(HaveOccurred())

				By("Update the contents of the source secret only")
				err = updateSourceSecretData(hiveClient, updatedData)
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the syncSet generation is unchanged by the source secret update")
				syncSetAfterUpdate := &hivev1.SyncSet{}
				err = hiveClient.Get(ctx, client.ObjectKey{Name: syncSetName, Namespace: clusterNamespace}, syncSetAfterUpdate)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(syncSetAfterUpdate.Generation).Should(Equal(syncSetBefore.Generation))

				// Note: we deliberately do not assert that the target cluster still holds the old
				// contents at this point. A periodic full re-apply can legitimately happen at any
				// time, so such an assertion would make the test depend on timing rather than on
				// the behavior under test.

				By("Delete the ClusterSyncLease exactly once to trigger a single full re-apply")
				deletedUID, err := deleteClusterSyncLease(hiveClient)
				Ω(err).ShouldNot(HaveOccurred())

				// The two waits below only read. Waiting for the re-apply pass to finish
				// separates "no re-apply ran" from "a re-apply ran but did not carry the
				// update". It says nothing about whether the applications in that pass
				// succeeded -- the controller writes the lease either way -- so the contents of
				// the mapped secret, checked next, remain the result that counts.
				By("Wait for the single re-apply pass to finish")
				err = waitForFullSyncSetReapply(hiveClient, deletedUID)
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify that single re-apply landed the updated source contents on the target cluster")
				err = waitForTargetSecretData(targetClusterClient, updatedData, secretMappingConfirmTimeout)
				Ω(err).ShouldNot(HaveOccurred())
				verifyMappedSecret(hiveClient, targetClusterClient, updatedData)

				By("Verify the syncSet spec and generation are unchanged by the scenario")
				syncSetAfter := &hivev1.SyncSet{}
				err = hiveClient.Get(ctx, client.ObjectKey{Name: syncSetName, Namespace: clusterNamespace}, syncSetAfter)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(syncSetAfter.Generation).Should(Equal(syncSetBefore.Generation))
				Ω(syncSetAfter.Spec).Should(Equal(syncSetBefore.Spec))

				By("Verify the refresh is reported as a successful apply of the same syncSet generation")
				verifySyncSetStatus(hiveClient, syncSetName, syncSetBefore.Generation)
			})

			// Eventual-convergence coverage, deliberately weaker than the single-trigger spec
			// above: the source secret is updated several times -- twice with no re-apply in
			// between, then once more right after a re-apply was triggered -- and full re-applies
			// are forced repeatedly until the mapped secret holds the contents the source secret
			// ends up with.
			//
			// Limits of this spec, so its coverage is not mistaken for more than it is:
			//   - Because it retriggers, it cannot detect a refresh missed by an individual
			//     re-apply. That is what the single-trigger spec above covers; this one must not
			//     be treated as a substitute for it.
			//   - It does not control when a re-apply reads the source secret relative to an
			//     update, so the interleaving in which the controller reads the source secret in
			//     between two updates is not covered.
			It("Test syncSet secretMappings eventually converge on the final source secret contents across successive updates and re-applies", func() {
				scenarioStart := time.Now()
				syncSetName := "test-syncsecret-converge"

				defer captureSecretMappingArtifacts(hiveClient, targetClusterClient, kubeClient, syncSetName, artifactStageAfter, scenarioStart)

				createSecretMappingSyncSet(hiveClient, targetClusterClient, kubeClient, syncSetName, secretMappingData("value-1"), scenarioStart)

				By("Update the source secret twice with no re-apply forced in between")
				err := updateSourceSecretData(hiveClient, secretMappingData("value-2"))
				Ω(err).ShouldNot(HaveOccurred())
				err = updateSourceSecretData(hiveClient, secretMappingData("value-3"))
				Ω(err).ShouldNot(HaveOccurred())

				By("Force full re-applies until the mapped secret holds the contents of the last of those updates")
				err = reapplyUntilTargetSecretMatches(hiveClient, targetClusterClient, secretMappingData("value-3"))
				Ω(err).ShouldNot(HaveOccurred())
				verifyMappedSecret(hiveClient, targetClusterClient, secretMappingData("value-3"))

				By("Update the source secret, trigger a re-apply without waiting for it to finish, then update the source secret again")
				err = updateSourceSecretData(hiveClient, secretMappingData("value-4"))
				Ω(err).ShouldNot(HaveOccurred())
				_, err = deleteClusterSyncLease(hiveClient)
				Ω(err).ShouldNot(HaveOccurred())
				err = updateSourceSecretData(hiveClient, secretMappingData("value-5"))
				Ω(err).ShouldNot(HaveOccurred())

				By("Force full re-applies until the mapped secret holds the contents of the final update")
				err = reapplyUntilTargetSecretMatches(hiveClient, targetClusterClient, secretMappingData("value-5"))
				Ω(err).ShouldNot(HaveOccurred())
				verifyMappedSecret(hiveClient, targetClusterClient, secretMappingData("value-5"))
			})
		})

		Context("Test selectorSynSet", func() {
			It("Test selectorSynSet of resources,patches,and secretMappings", func() {
				ctx := context.TODO()
				By("Get the pull secret name of clusterdeployment")
				cd := &hivev1.ClusterDeployment{}
				var cdLabels map[string]string
				err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
					err := hiveClient.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, cd)
					Ω(err).ShouldNot(HaveOccurred())

					By(`Set a label "cluster-group: hivecluster" to clusterdeployment`)
					cdLabels = cd.ObjectMeta.GetLabels()
					cdLabels["cluster-group"] = "hivecluster"
					cd.ObjectMeta.SetLabels(cdLabels)
					return hiveClient.Update(ctx, cd)
				})
				Ω(err).ShouldNot(HaveOccurred())
				Ω(cd.ObjectMeta.Labels).Should(Equal(cdLabels))

				resourceConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "default",
					},
					Data: map[string]string{
						"foo": "bar",
					},
				}

				selectorSyncSet := &hivev1.SelectorSyncSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-selectorsyncset",
					},
					Spec: hivev1.SelectorSyncSetSpec{
						ClusterDeploymentSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"cluster-group": "hivecluster"},
						},
						SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
							ResourceApplyMode: hivev1.SyncResourceApplyMode,
							Resources: []runtime.RawExtension{
								{
									Object: &corev1.ConfigMap{
										TypeMeta: metav1.TypeMeta{
											Kind:       "ConfigMap",
											APIVersion: "v1",
										},
										ObjectMeta: metav1.ObjectMeta{
											Name:      "foo-selectorsyncset",
											Namespace: "default",
										},
										Data: map[string]string{
											"foo-sSS": "bar-sSS",
										},
									},
								},
							},
							Patches: []hivev1.SyncObjectPatch{
								{
									APIVersion: "v1",
									Kind:       "ConfigMap",
									Name:       "foo",
									Namespace:  "default",
									Patch:      `{ "data": { "foo": "new-bar" } }`,
									PatchType:  "merge",
								},
							},
							Secrets: []hivev1.SecretMapping{
								{
									SourceRef: hivev1.SecretReference{
										Name:      cd.Spec.PullSecretRef.Name,
										Namespace: clusterNamespace,
									},
									TargetRef: hivev1.SecretReference{
										Name:      "test-pull-secret-copy",
										Namespace: "default",
									},
								},
							},
						},
					},
				}

				By("Create a ConfigMap on target cluster and verify ConfigMap created successfully")
				err = targetClusterClient.Create(ctx, resourceConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				resultConfigMap := &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

				By("Create a selectorSyncSet  including resources, patches and secretMappings and verify create successfully")
				err = hiveClient.Create(ctx, selectorSyncSet)
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetApplied(clusterNamespace, clusterName, "test-selectorsyncset", "selectorsyncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the resource is synced on target cluster")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo-selectorsyncset", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo-sSS": "bar-sSS"}))

				By("Verify the resource is patched")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "new-bar"}))

				By("Verify the secret is copied")
				resultSecret := &corev1.Secret{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "test-pull-secret-copy", Namespace: "default"}, resultSecret)
				Ω(err).ShouldNot(HaveOccurred())

				By(`Delete the label "cluster-group: hivecluster" of clusterdeployment`)
				err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
					err := hiveClient.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, cd)
					Ω(err).ShouldNot(HaveOccurred())
					cdLabels = cd.ObjectMeta.GetLabels()
					delete(cdLabels, "cluster-group")
					cd.ObjectMeta.SetLabels(cdLabels)
					return hiveClient.Update(ctx, cd)
				})
				Ω(err).ShouldNot(HaveOccurred())
				Ω(cd.ObjectMeta.Labels).Should(Equal(cdLabels))

				By("Verify syncset disassociated with cluster")
				err = waitForSyncSetDisassociated(clusterNamespace, clusterName, "test-selectorsyncset", "selectorsyncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the resource is deleted on target cluster")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo-selectorsyncset", Namespace: "default"}, resultConfigMap)
				Ω(errors.IsNotFound(err)).Should(BeTrue())

				By("Verify the secret is deleted on target cluster")
				resultSecret = &corev1.Secret{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "test-pull-secret-copy", Namespace: "default"}, resultSecret)
				Ω(errors.IsNotFound(err)).Should(BeTrue())
			})
		})
	})
})

func waitForSyncSetApplied(namespace, cdName, syncsetname, syncsettype string) error {
	scheme := scheme.GetScheme()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := common.MustGetConfig()
	gvk, err := apiutil.GVKForObject(&hiveintv1alpha1.ClusterSync{}, scheme)
	if err != nil {
		return err
	}
	hc, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return err
	}
	restClient, err := apiutil.RESTClientForGVK(gvk, false, false, cfg, serializer.NewCodecFactory(scheme), hc)
	if err != nil {
		return err
	}
	listWatcher := cache.NewFilteredListWatchFromClient(
		restClient,
		"clustersyncs",
		namespace,
		func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", cdName).String()
			options.AllowWatchBookmarks = true
		},
	)
	syncSetApplied := func(event watch.Event) (bool, error) {
		if event.Type != watch.Added && event.Type != watch.Modified {
			return false, nil
		}
		clusterSync, ok := event.Object.(*hiveintv1alpha1.ClusterSync)
		if !ok {
			// Object is not of type syncssetinstance
			return false, nil
		}
		var syncStatuses []hiveintv1alpha1.SyncStatus
		switch syncsettype {
		case "syncset":
			syncStatuses = clusterSync.Status.SyncSets
		case "selectorsyncset":
			syncStatuses = clusterSync.Status.SelectorSyncSets
		default:
			return false, fmt.Errorf("unknown syncset type")
		}
		for _, status := range syncStatuses {
			if status.Name == syncsetname {
				return status.Result == hiveintv1alpha1.SuccessSyncSetResult, nil
			}
		}
		return false, nil
	}
	_, err = clientwatch.UntilWithSync(ctx, listWatcher, &hiveintv1alpha1.ClusterSync{}, nil, syncSetApplied)
	return err
}

func waitForSyncSetDeleted(namespace, syncsetname string) error {
	scheme := scheme.GetScheme()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := common.MustGetConfig()
	gvk, err := apiutil.GVKForObject(&hivev1.SyncSet{}, scheme)
	if err != nil {
		return err
	}
	hc, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return err
	}
	restClient, err := apiutil.RESTClientForGVK(gvk, false, false, cfg, serializer.NewCodecFactory(scheme), hc)
	if err != nil {
		return err
	}
	listWatcher := cache.NewFilteredListWatchFromClient(
		restClient,
		"syncsets",
		namespace,
		func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", syncsetname).String()
			options.AllowWatchBookmarks = true
		},
	)
	_, err = clientwatch.UntilWithSync(
		ctx,
		listWatcher,
		&hivev1.SyncSet{},
		func(store cache.Store) (bool, error) {
			return len(store.List()) == 0, nil
		},
		func(event watch.Event) (bool, error) {
			return event.Type == watch.Deleted, nil
		})
	return err
}

func waitForSyncSetDisassociated(namespace, cdName, syncsetname, syncsettype string) error {
	scheme := scheme.GetScheme()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := common.MustGetConfig()
	gvk, err := apiutil.GVKForObject(&hiveintv1alpha1.ClusterSync{}, scheme)
	if err != nil {
		return err
	}
	hc, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return err
	}
	restClient, err := apiutil.RESTClientForGVK(gvk, false, false, cfg, serializer.NewCodecFactory(scheme), hc)
	if err != nil {
		return err
	}
	listWatcher := cache.NewFilteredListWatchFromClient(
		restClient,
		"clustersyncs",
		namespace,
		func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", cdName).String()
			options.AllowWatchBookmarks = true
		},
	)
	syncSetDisassociated := func(event watch.Event) (bool, error) {
		if event.Type != watch.Added && event.Type != watch.Modified {
			return false, nil
		}
		clusterSync, ok := event.Object.(*hiveintv1alpha1.ClusterSync)
		if !ok {
			// Object is not of type syncssetinstance
			return false, nil
		}
		var syncStatuses []hiveintv1alpha1.SyncStatus
		switch syncsettype {
		case "syncset":
			syncStatuses = clusterSync.Status.SyncSets
		case "selectorsyncset":
			syncStatuses = clusterSync.Status.SelectorSyncSets
		default:
			return false, fmt.Errorf("unknown syncset type")
		}
		for _, status := range syncStatuses {
			if status.Name == syncsetname {
				return false, nil
			}
		}
		return true, nil
	}
	_, err = clientwatch.UntilWithSync(ctx, listWatcher, &hiveintv1alpha1.ClusterSync{}, nil, syncSetDisassociated)
	return err
}

func deleteAllSyncSets(c client.Client, namespace string) {
	ctx := context.TODO()
	list := &hivev1.SyncSetList{}
	err := c.List(ctx, list, client.InNamespace(namespace))
	Ω(err).ShouldNot(HaveOccurred())
	for i := range list.Items {
		err = c.Delete(ctx, &list.Items[i])
		Ω(err).ShouldNot(HaveOccurred())
	}
}

func deleteSyncSets(c client.Client, namespace, name string) {
	ctx := context.TODO()
	ss := &hivev1.SyncSet{}
	ss.Namespace = namespace
	ss.Name = name
	err := c.Delete(ctx, ss)
	Ω(err).ShouldNot(HaveOccurred())
}

func deleteAllSelectorSyncSets(c client.Client, namespace string) {
	ctx := context.TODO()
	list := &hivev1.SelectorSyncSetList{}
	err := c.List(ctx, list, client.InNamespace(namespace))
	Ω(err).ShouldNot(HaveOccurred())
	for i := range list.Items {
		err = c.Delete(ctx, &list.Items[i])
		Ω(err).ShouldNot(HaveOccurred())
	}
}

func deleteConfigMap(c client.Client, namespace, name string) {
	ctx := context.TODO()
	cm := &corev1.ConfigMap{}
	cm.Namespace = namespace
	cm.Name = name
	err := c.Delete(ctx, cm)
	if !errors.IsNotFound(err) {
		Ω(err).ShouldNot(HaveOccurred())
	}

}

// secretMappingData builds the contents of the synthetic source secret. The "rotating" key carries
// the value under test; the "static" key is never changed, so that asserting on the whole data map
// also catches a re-apply that drops or corrupts the keys which did not change.
func secretMappingData(rotatingValue string) map[string][]byte {
	return map[string][]byte{
		"rotating": []byte(rotatingValue),
		"static":   []byte("value-static"),
	}
}

// secretMappingSyncSet builds a SyncSet mapping the synthetic source secret to the target cluster.
// Sync apply mode is used so that deleting the SyncSet in AfterEach also removes the mapped secret
// from the target cluster; AfterEach deletes that secret directly as well, because the removal
// performed by Hive is asynchronous.
func secretMappingSyncSet(name string) *hivev1.SyncSet {
	return &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: clusterNamespace,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: clusterName,
				},
			},
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				ResourceApplyMode: hivev1.SyncResourceApplyMode,
				Secrets: []hivev1.SecretMapping{
					{
						SourceRef: hivev1.SecretReference{
							Name:      secretMappingSourceSecretName,
							Namespace: clusterNamespace,
						},
						TargetRef: hivev1.SecretReference{
							Name:      secretMappingTargetSecretName,
							Namespace: secretMappingTargetNamespace,
						},
					},
				},
			},
		},
	}
}

// createSecretMappingSyncSet creates the synthetic source secret along with a SyncSet mapping it
// to the target cluster, waits until the mapped secret holds the contents of the source, and
// snapshots the state the scenario starts from.
func createSecretMappingSyncSet(hiveClient, targetClusterClient client.Client, kubeClient kclient.Interface, syncSetName string, data map[string][]byte, scenarioStart time.Time) {
	By("Create a dedicated source secret in the cluster namespace")
	err := createSourceSecret(hiveClient, data)
	Ω(err).ShouldNot(HaveOccurred())

	By("Create a syncSet mapping the source secret to the target cluster and verify it is applied")
	err = hiveClient.Create(context.TODO(), secretMappingSyncSet(syncSetName))
	Ω(err).ShouldNot(HaveOccurred())
	err = waitForSyncSetApplied(clusterNamespace, clusterName, syncSetName, "syncset")
	Ω(err).ShouldNot(HaveOccurred())

	By("Verify the mapped secret on the target cluster holds the initial source contents")
	err = waitForTargetSecretData(targetClusterClient, data, secretMappingApplyTimeout)
	Ω(err).ShouldNot(HaveOccurred())
	verifyMappedSecret(hiveClient, targetClusterClient, data)

	By("Snapshot the syncSet, source secret, clusterSync, lease and target secret before the source secret is updated")
	captureSecretMappingArtifacts(hiveClient, targetClusterClient, kubeClient, syncSetName, artifactStageBefore, scenarioStart)
}

// createSourceSecret creates the synthetic source secret in the cluster namespace. A secret left
// behind by an earlier spec whose cleanup did not complete is updated instead, so that one failing
// spec does not cascade into the next.
func createSourceSecret(c client.Client, data map[string][]byte) error {
	err := c.Create(context.TODO(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretMappingSourceSecretName,
			Namespace: clusterNamespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	})
	if errors.IsAlreadyExists(err) {
		return updateSourceSecretData(c, data)
	}
	return err
}

// updateSourceSecretData changes the contents of the source secret, leaving every other object --
// in particular the SyncSet referencing it -- untouched.
func updateSourceSecretData(c client.Client, data map[string][]byte) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		secret := &corev1.Secret{}
		if err := c.Get(context.TODO(), client.ObjectKey{Name: secretMappingSourceSecretName, Namespace: clusterNamespace}, secret); err != nil {
			return err
		}
		secret.Data = data
		return c.Update(context.TODO(), secret)
	})
}

// deleteClusterSyncLease deletes the ClusterSyncLease of the cluster under test, which makes the
// clustersync controller re-apply every SyncSet and SelectorSyncSet associated with the cluster.
// The lease is deleted only once it has been observed to exist: deleting a lease that is not there
// produces no watch event and would therefore not trigger a re-apply. The UID of the deleted lease
// is returned so that the caller can recognize the lease the controller creates afterwards.
//
// Exactly one deletion is performed, so that callers can rely on having triggered exactly one
// re-apply. Reads are retried, and so is a deletion that is known to have deleted nothing -- the
// lease was gone, or carried a different UID -- but a deletion whose outcome is unknown, such as
// one that failed in transport after the API server may already have applied it, is reported as
// an error instead of being retried.
func deleteClusterSyncLease(c client.Client) (types.UID, error) {
	var deletedUID types.UID
	var lastErr error
	if err := wait.PollUntilContextTimeout(context.Background(), secretMappingPollInterval, secretMappingLeaseTimeout, true,
		func(ctx context.Context) (bool, error) {
			lease := &hiveintv1alpha1.ClusterSyncLease{}
			if err := c.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, lease); err != nil {
				// Either the controller has not (re)created the lease yet, or the API call failed
				// transiently. Keep polling in both cases.
				lastErr = err
				return false, nil
			}
			// The UID precondition makes sure we delete the lease we observed, and not one the
			// controller recreated in the meantime.
			switch err := c.Delete(ctx, lease, client.Preconditions{UID: &lease.UID}); {
			case err == nil:
				deletedUID = lease.UID
				return true, nil
			case errors.IsNotFound(err), errors.IsConflict(err):
				// Nothing was deleted: the lease had already gone, or it had been replaced by
				// one with another UID. Observing the lease afresh is safe.
				lastErr = err
				return false, nil
			default:
				return false, err
			}
		}); err != nil {
		return "", fmt.Errorf("could not delete clusterSyncLease %s/%s to force a full re-apply (last error: %v): %w",
			clusterNamespace, clusterName, lastErr, err)
	}
	return deletedUID, nil
}

// waitForFullSyncSetReapply waits for the clustersync controller to recreate a ClusterSyncLease
// that was deleted. The controller writes the lease at the end of a full re-apply pass, so a
// lease carrying a UID other than the deleted one means such a pass ran through to its end.
//
// It does not mean the applications in that pass succeeded: the lease is written whatever their
// outcome, with failures recorded in the ClusterSync status instead. Callers use this to tell a
// pass that never ran from one that ran, and then check what the pass actually produced.
func waitForFullSyncSetReapply(c client.Client, deletedUID types.UID) error {
	var lastErr error
	if err := wait.PollUntilContextTimeout(context.Background(), secretMappingPollInterval, secretMappingLeaseTimeout, true,
		func(ctx context.Context) (bool, error) {
			lease := &hiveintv1alpha1.ClusterSyncLease{}
			if err := c.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, lease); err != nil {
				lastErr = err
				return false, nil
			}
			lastErr = nil
			return lease.UID != deletedUID, nil
		}); err != nil {
		return fmt.Errorf("clusterSyncLease %s/%s was not recreated, so no full re-apply completed (last error: %v): %w",
			clusterNamespace, clusterName, lastErr, err)
	}
	return nil
}

// waitForTargetSecretData waits for the mapped secret on the target cluster to hold exactly the
// expected contents.
func waitForTargetSecretData(c client.Client, want map[string][]byte, timeout time.Duration) error {
	var observed map[string][]byte
	var lastGetErr error
	if err := wait.PollUntilContextTimeout(context.Background(), secretMappingPollInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			secret := &corev1.Secret{}
			if err := c.Get(ctx, client.ObjectKey{Name: secretMappingTargetSecretName, Namespace: secretMappingTargetNamespace}, secret); err != nil {
				// The secret may not have been created yet, and connectivity to the target
				// cluster can blip; keep polling until the timeout expires.
				lastGetErr = err
				observed = nil
				return false, nil
			}
			lastGetErr = nil
			observed = secret.Data
			return reflect.DeepEqual(secret.Data, want), nil
		}); err != nil {
		return fmt.Errorf("secret %s/%s on the target cluster did not reach the expected contents (keys not matching: %v, last get error: %v): %w",
			secretMappingTargetNamespace, secretMappingTargetSecretName, mismatchedKeys(want, observed), lastGetErr, err)
	}
	return nil
}

// mismatchedKeys returns the sorted names of the keys whose values differ between want and
// observed, including keys missing from either side. Only the names are reported: the contents of
// a secret have no place in test output.
func mismatchedKeys(want, observed map[string][]byte) []string {
	var keys []string
	for key, wantValue := range want {
		if observedValue, ok := observed[key]; !ok || !bytes.Equal(observedValue, wantValue) {
			keys = append(keys, key)
		}
	}
	for key := range observed {
		if _, ok := want[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// reapplyUntilTargetSecretMatches forces full re-applies until the mapped secret on the target
// cluster holds the expected contents. Each attempt waits for the re-apply pass to finish before
// checking the secret, and at most secretMappingMaxReapplyAttempts attempts are made, so the wait
// is bounded.
//
// This is for the eventual-convergence spec only. It must not be used to observe the outcome of a
// single re-apply: repeating the trigger would turn a refresh the controller missed into a pass.
// A finished pass is never taken as success on its own -- the contents of the mapped secret are
// always checked.
func reapplyUntilTargetSecretMatches(hiveClient, targetClusterClient client.Client, want map[string][]byte) error {
	var lastErr error
	for attempt := 1; attempt <= secretMappingMaxReapplyAttempts; attempt++ {
		deletedUID, err := deleteClusterSyncLease(hiveClient)
		if err != nil {
			lastErr = err
			continue
		}
		if err := waitForFullSyncSetReapply(hiveClient, deletedUID); err != nil {
			lastErr = err
			continue
		}
		// The re-apply writes the mapped secret before it writes the lease, so the expected
		// contents are normally in place already.
		lastErr = waitForTargetSecretData(targetClusterClient, want, secretMappingConfirmTimeout)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("the mapped secret did not pick up the source secret contents after %d full re-applies: %w",
		secretMappingMaxReapplyAttempts, lastErr)
}

// verifyMappedSecret asserts that the secret the SecretMapping targetRef names on the target
// cluster is a faithful copy of the source secret.
func verifyMappedSecret(hiveClient, targetClusterClient client.Client, want map[string][]byte) {
	ctx := context.TODO()

	sourceSecret := &corev1.Secret{}
	err := hiveClient.Get(ctx, client.ObjectKey{Name: secretMappingSourceSecretName, Namespace: clusterNamespace}, sourceSecret)
	Ω(err).ShouldNot(HaveOccurred())
	Ω(sourceSecret.Data).Should(Equal(want))

	targetSecret := &corev1.Secret{}
	err = targetClusterClient.Get(ctx, client.ObjectKey{Name: secretMappingTargetSecretName, Namespace: secretMappingTargetNamespace}, targetSecret)
	Ω(err).ShouldNot(HaveOccurred())
	Ω(targetSecret.Data).Should(Equal(sourceSecret.Data))
	Ω(targetSecret.Type).Should(Equal(sourceSecret.Type))
}

// verifySyncSetStatus asserts that clustersync reports the SyncSet as successfully applied at the
// generation the spec observed, which shows the mapped secret was refreshed without any change to
// the SyncSet itself.
func verifySyncSetStatus(c client.Client, syncSetName string, generation int64) {
	clusterSync := &hiveintv1alpha1.ClusterSync{}
	err := c.Get(context.TODO(), client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, clusterSync)
	Ω(err).ShouldNot(HaveOccurred())

	var syncStatus *hiveintv1alpha1.SyncStatus
	for i, status := range clusterSync.Status.SyncSets {
		if status.Name == syncSetName {
			syncStatus = &clusterSync.Status.SyncSets[i]
			break
		}
	}
	Ω(syncStatus).ShouldNot(BeNil(), "clusterSync %s/%s has no status for syncSet %s", clusterNamespace, clusterName, syncSetName)
	Ω(syncStatus.Result).Should(Equal(hiveintv1alpha1.SuccessSyncSetResult))
	Ω(syncStatus.ObservedGeneration).Should(Equal(generation))
}

// artifactDir returns the directory test artifacts are written to, following the convention of
// the e2e shell helpers (hack/e2e-common.sh): $ARTIFACT_DIR, or /tmp when it is not set.
func artifactDir() string {
	if dir := os.Getenv("ARTIFACT_DIR"); dir != "" {
		return dir
	}
	return "/tmp"
}

// captureMetadata accompanies each stage of captured manifests with the times at which they were
// taken, in UTC, so that the stages can be ordered against each other and against the clustersync
// log without relying on the objects' own timestamps.
type captureMetadata struct {
	Stage            string `json:"stage"`
	ObservedAtUTC    string `json:"observedAtUTC"`
	SpecStartedAtUTC string `json:"specStartedAtUTC"`
	SyncSet          string `json:"syncSet"`
	ClusterName      string `json:"clusterName"`
	ClusterNamespace string `json:"clusterNamespace"`
}

// captureSecretMappingArtifacts writes the manifests that make up the secretMapping scenario --
// the SyncSet, the synthetic source secret, the ClusterSync, the ClusterSyncLease and the mapped
// secret on the target cluster -- to the artifact directory, and, at the end of a spec, the logs
// of the clustersync controller alongside them. Specs capture a snapshot before the source secret
// is updated and another on the way out, so that a successful and a failing run alike leave
// behind the before and after state. The manifests are written verbatim, keeping the generations,
// resource versions and object timestamps, and each stage is accompanied by the UTC time at which
// the spec took it. Together they are material for working out what the controller did and when;
// they do not record which revision of the source secret it read.
//
// Capture is best effort from end to end: problems are reported to the Ginkgo output, none of
// them fails the spec, a panic cannot escape, and the whole of it is bounded in time, so that a
// failure under investigation is neither masked nor left waiting on its own diagnostics. Callers
// run it before AfterEach deletes the objects.
//
// Both secrets written out are created by this suite and hold only its synthetic values; no other
// secret of the hub or of the target cluster is read.
func captureSecretMappingArtifacts(hiveClient, targetClusterClient client.Client, kubeClient kclient.Interface, syncSetName, stage string, since time.Time) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(GinkgoWriter, "artifact capture for %s panicked: %v\n", syncSetName, r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), secretMappingArtifactTimeout)
	defer cancel()
	prefix := fmt.Sprintf("secretmapping-%s-%s", syncSetName, stage)

	// When the snapshot was taken, which no field of the objects records: their timestamps say
	// when the API server last wrote them, not when this spec read them or updated the source
	// secret.
	metadata := captureMetadata{
		Stage:            stage,
		ObservedAtUTC:    time.Now().UTC().Format(time.RFC3339Nano),
		SpecStartedAtUTC: since.UTC().Format(time.RFC3339Nano),
		SyncSet:          syncSetName,
		ClusterName:      clusterName,
		ClusterNamespace: clusterNamespace,
	}
	if content, err := yaml.Marshal(metadata); err != nil {
		fmt.Fprintf(GinkgoWriter, "artifact %s-capture.yaml: could not serialize: %v\n", prefix, err)
	} else {
		writeArtifactBytes(prefix+"-capture.yaml", content)
	}

	capture := func(c client.Client, key client.ObjectKey, obj client.Object, kind string) {
		name := fmt.Sprintf("%s-%s.yaml", prefix, kind)
		if err := c.Get(ctx, key, obj); err != nil {
			fmt.Fprintf(GinkgoWriter, "artifact %s: could not read %s: %v\n", name, key, err)
			return
		}
		writeArtifact(name, obj)
	}

	clusterKey := client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}
	capture(hiveClient, client.ObjectKey{Name: syncSetName, Namespace: clusterNamespace},
		&hivev1.SyncSet{}, "syncset")
	capture(hiveClient, client.ObjectKey{Name: secretMappingSourceSecretName, Namespace: clusterNamespace},
		&corev1.Secret{}, "source-secret")
	capture(hiveClient, clusterKey, &hiveintv1alpha1.ClusterSync{}, "clustersync")
	capture(hiveClient, clusterKey, &hiveintv1alpha1.ClusterSyncLease{}, "clustersynclease")
	capture(targetClusterClient, client.ObjectKey{Name: secretMappingTargetSecretName, Namespace: secretMappingTargetNamespace},
		&corev1.Secret{}, "target-secret")

	// The log of a stage covers the whole spec up to that point, so collecting it once, at the
	// end, is enough.
	if stage == artifactStageAfter {
		captureClustersyncLogs(ctx, kubeClient, prefix, since)
	}
}

// captureClustersyncLogs writes the logs of the hive-clustersync pods to the artifact directory.
// Only the part of the log produced since the spec started is collected, so that it lines up with
// the manifests captured alongside it.
func captureClustersyncLogs(ctx context.Context, kubeClient kclient.Interface, prefix string, since time.Time) {
	hiveNamespace := controllerutils.GetHiveNamespace()
	controller := string(hivev1.ClustersyncControllerName)

	pods, err := kubeClient.CoreV1().Pods(hiveNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("control-plane=%s", controller),
	})
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "artifact %s: could not list %s pods in namespace %s: %v\n", prefix, controller, hiveNamespace, err)
		return
	}
	if len(pods.Items) == 0 {
		// Worth saying out loud: without these logs the artifacts cannot show what the controller
		// did. A likely cause is HIVE_NS not being set, leaving the namespace at its default.
		fmt.Fprintf(GinkgoWriter, "artifact %s: no %s pods found in namespace %s, no controller logs captured\n", prefix, controller, hiveNamespace)
		return
	}
	// The log window is selected by the kubelet against node clocks, so start it a little before
	// the spec did to allow for skew.
	sinceTime := metav1.NewTime(since.Add(-secretMappingLogSkewMargin))
	for _, pod := range pods.Items {
		logs, err := kubeClient.CoreV1().Pods(hiveNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container:  controller,
			SinceTime:  &sinceTime,
			LimitBytes: ptr.To(int64(secretMappingLogLimitBytes)),
		}).DoRaw(ctx)
		if err != nil {
			fmt.Fprintf(GinkgoWriter, "artifact %s: could not read logs of pod %s: %v\n", prefix, pod.Name, err)
			continue
		}
		writeArtifactBytes(fmt.Sprintf("%s-%s.log", prefix, pod.Name), logs)
	}
}

// writeArtifact serializes obj as YAML -- preserving generation, resourceVersion and timestamps --
// and writes it to the artifact directory. The kind is restored before serializing, because the
// typed client clears it on read.
func writeArtifact(name string, obj client.Object) {
	if gvk, err := apiutil.GVKForObject(obj, scheme.GetScheme()); err != nil {
		fmt.Fprintf(GinkgoWriter, "artifact %s: could not determine kind: %v\n", name, err)
	} else {
		obj.GetObjectKind().SetGroupVersionKind(gvk)
	}
	content, err := yaml.Marshal(obj)
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "artifact %s: could not serialize: %v\n", name, err)
		return
	}
	writeArtifactBytes(name, content)
}

func writeArtifactBytes(name string, content []byte) {
	dir := artifactDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(GinkgoWriter, "artifact %s: could not create %s: %v\n", name, dir, err)
		return
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0644); err != nil {
		fmt.Fprintf(GinkgoWriter, "artifact %s: could not write %s: %v\n", name, path, err)
	}
}

func deleteSecret(c client.Client, namespace, name string) {
	ctx := context.TODO()
	secret := &corev1.Secret{}
	secret.Namespace = namespace
	secret.Name = name
	err := c.Delete(ctx, secret)
	if !errors.IsNotFound(err) {
		Ω(err).ShouldNot(HaveOccurred())
	}
}
