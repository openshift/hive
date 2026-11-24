package syncsets_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
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

	// burstConvergeTimeout bounds the wait for the target cluster to reach the contents the
	// source secrets end the burst with. A re-apply that carries them lands within seconds; if
	// none does, the next one is a periodic re-apply two hours later, so waiting longer than
	// this only delays the failure.
	burstConvergeTimeout = 3 * time.Minute
	// burstTriggerTimeout bounds a single trigger attempt. It is short on purpose: a trigger
	// that blocks would serialize the burst it is meant to keep going.
	burstTriggerTimeout = 30 * time.Second
)

// burstConfigMapData builds the contents of the ConfigMap a burst scenario adds to its SyncSet.
func burstConfigMapData() map[string]string {
	return map[string]string{"burst": "value-configmap"}
}

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
		for _, scenario := range burstScenarios {
			for _, pair := range scenario.pairs {
				deleteSecret(targetClusterClient, secretMappingTargetNamespace, pair.targetName)
				deleteSecret(hiveClient, clusterNamespace, pair.sourceName)
			}
			if scenario.changesSyncSet() {
				deleteConfigMap(targetClusterClient, secretMappingTargetNamespace, scenario.configMapName)
			}
		}
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
				defer captureSecretMappingArtifacts(hiveClient, targetClusterClient, kubeClient, syncSetName, artifactStageAfter, scenarioStart, defaultSecretMappingPairs)

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

				defer captureSecretMappingArtifacts(hiveClient, targetClusterClient, kubeClient, syncSetName, artifactStageAfter, scenarioStart, defaultSecretMappingPairs)

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

			// Burst coverage: a SyncSet carrying two SecretMappings is created and the contents
			// of its source secrets are changed several times in a row, with no wait for the
			// target cluster in between and none after the SyncSet is created. What is required
			// at the end is that the target cluster holds exactly what the source secrets hold.
			//
			// Each mutation is followed by a single attempt at deleting the ClusterSyncLease.
			// The attempt never waits for the lease to exist and is never repeated, so no
			// mutation waits for its own re-apply to settle and the burst stays a burst. Every
			// outcome is recorded in the timeline artifact: a lease that was deleted, one that
			// definitely was not (absent, or replaced since it was read), one that could not be
			// read, or a deletion whose effect cannot be known. The last two fail the scenario
			// before it waits for anything, because the trigger either never reached the
			// deletion call or left no way to tell what it did; the first two are legitimate
			// results of firing triggers faster than the controller recreates the lease.
			//
			// Limits, so this is not read as more than it is:
			//   - Only the final contents are asserted. A refresh missed by one of the earlier
			//     re-applies is covered by a later one and does not fail these specs; catching
			//     a single missed refresh is the single-trigger spec's job.
			//   - Nothing orders these updates against the controller. An update may reach the
			//     hub while an apply is running or between two applies; the specs do not force
			//     either and do not assert that either happened. An update that lands while an
			//     earlier apply is in flight is covered deterministically by the clustersync
			//     controller unit tests, which place it there directly.
			//   - No attempt is required to have deleted anything. Finding no lease, or finding
			//     that the one it read has just been replaced, is a legitimate result of
			//     firing an attempt into a burst: an earlier attempt may have removed a lease
			//     the controller has not recreated yet, or has recreated since. A burst in
			//     which every attempt ends that way is a run of the scenario like any other.
			//   - The controller reads source secrets through a cache and does not watch them,
			//     so a re-apply that ran before the last writes reached that cache leaves the
			//     target holding earlier contents until the periodic re-apply hours later. A
			//     scenario whose re-applies all fell that way reaches its convergence timeout
			//     and fails. That failure is the result of the scenario, with the artifacts to
			//     show what the triggers and the controller did; nothing is issued afterwards
			//     to bring the contents in line. How often real runs land there is not known:
			//     these scenarios have not been run against a cluster yet.
			It("Test syncSet secretMappings converge when only the source secrets are updated in a burst", func() {
				// The SyncSet is created once and never touched again, and the only thing this
				// scenario does after a source secret changes is the single lease deletion
				// attempt that accompanies it -- which may well find no lease. Nothing is done
				// afterwards to make a re-apply happen. What the apply caused by the creation
				// of the SyncSet does, and what the controller retries of its own accord, is
				// the controller's business and is not driven from here.
				runSecretMappingBurst(hiveClient, targetClusterClient, kubeClient, sourcesOnlyBurst)
			})

			It("Test syncSet secretMappings converge when a burst of source updates is followed by a syncSet change", func() {
				// The burst ends by adding a resource to the SyncSet, so a SyncSet and the
				// secrets it maps change together. The generation change is a trigger of its
				// own, independent of the lease: it can cover a refresh an earlier re-apply
				// missed, which is why the scenario above exists alongside this one.
				runSecretMappingBurst(hiveClient, targetClusterClient, kubeClient, syncSetChangeBurst)
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

// secretMappingPair names a source secret on the hub and the secret a SecretMapping copies it to
// on the target cluster. Captures are taken for these pairs only, so a spec can never snapshot a
// secret it did not create.
type secretMappingPair struct {
	sourceName string
	targetName string
}

var (
	defaultSecretMappingPair = secretMappingPair{
		sourceName: secretMappingSourceSecretName,
		targetName: secretMappingTargetSecretName,
	}
	defaultSecretMappingPairs = []secretMappingPair{defaultSecretMappingPair}

	// sourcesOnlyBurst changes nothing but the contents of the source secrets: the SyncSet is
	// created once and left alone, so the only triggers the scenario has are its one-shot lease
	// deletions.
	sourcesOnlyBurst = burstScenario{
		syncSetName: "test-syncsecret-burst-sources",
		pairs: []secretMappingPair{
			{sourceName: "test-secretmapping-burst-sources-source-one", targetName: "test-secretmapping-burst-sources-copy-one"},
			{sourceName: "test-secretmapping-burst-sources-source-two", targetName: "test-secretmapping-burst-sources-copy-two"},
		},
	}
	// syncSetChangeBurst ends its burst by adding a resource to the SyncSet, which covers a
	// SyncSet and the secrets it maps changing together.
	syncSetChangeBurst = burstScenario{
		syncSetName: "test-syncsecret-burst-syncset-change",
		pairs: []secretMappingPair{
			{sourceName: "test-secretmapping-burst-change-source-one", targetName: "test-secretmapping-burst-change-copy-one"},
			{sourceName: "test-secretmapping-burst-change-source-two", targetName: "test-secretmapping-burst-change-copy-two"},
		},
		configMapName: "test-secretmapping-burst-change-configmap",
	}
	burstScenarios = []burstScenario{sourcesOnlyBurst, syncSetChangeBurst}
)

// burstScenario is one burst variant. Each has its own SyncSet, its own source and target
// secrets and its own timeline artifact. The ClusterSync and the ClusterSyncLease are per
// ClusterDeployment and so are necessarily shared, which is harmless: the suite is serial, so
// only one scenario drives the lease at a time.
type burstScenario struct {
	syncSetName string
	pairs       []secretMappingPair
	// configMapName, when set, is a resource added to the SyncSet at the end of the burst. That
	// changes the SyncSet spec, and so its generation, which is a trigger in its own right.
	configMapName string
}

// changesSyncSet reports whether the scenario modifies the SyncSet after creating it.
func (s burstScenario) changesSyncSet() bool { return s.configMapName != "" }

// leaseDeleteOutcome is what a single attempt at deleting the ClusterSyncLease achieved.
type leaseDeleteOutcome string

const (
	// leaseDeleted: the attempt removed the lease that was read, so it triggered a re-apply.
	leaseDeleted leaseDeleteOutcome = "Deleted"
	// leaseNotDeleted: the attempt definitely removed nothing, because the lease was not there
	// or had already been replaced by another one. No re-apply was triggered by it.
	leaseNotDeleted leaseDeleteOutcome = "NotDeleted"
	// leaseNotDeletedReadFailed: the lease could not be read, so no deletion was ever issued.
	// Nothing was removed, and the reason is kept separate from the ordinary case so that a
	// flaky hub does not read as a lease that was simply absent.
	leaseNotDeletedReadFailed leaseDeleteOutcome = "NotDeletedReadFailed"
	// leaseDeleteUnknown: the attempt failed in a way that leaves its effect unknown -- the API
	// server may or may not have applied the deletion. It is recorded as such and never counted
	// as either of the above.
	leaseDeleteUnknown leaseDeleteOutcome = "Unknown"
)

// burstMutation records one mutation of a burst scenario and the outcome of the single trigger
// attempt that followed it. Only synthetic identifiers, resource versions, generations and times
// are recorded; no secret contents.
type burstMutation struct {
	Name                   string             `json:"name"`
	AtUTC                  string             `json:"atUTC"`
	SourceResourceVersions map[string]string  `json:"sourceResourceVersions,omitempty"`
	SyncSetGeneration      string             `json:"syncSetGeneration"`
	LeaseDeleteOutcome     leaseDeleteOutcome `json:"leaseDeleteOutcome"`
	LeaseDeleteDetail      string             `json:"leaseDeleteDetail,omitempty"`
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

// burstSecretMappingSyncSet builds the SyncSet of a burst scenario: one SecretMapping per pair,
// so that the burst covers a SyncSet carrying more than one mapped secret. When withResource is
// set the SyncSet also carries a ConfigMap, which is how a scenario changes the SyncSet itself at
// the end of its burst.
func burstSecretMappingSyncSet(scenario burstScenario, withResource bool) *hivev1.SyncSet {
	syncSet := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scenario.syncSetName,
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
			},
		},
	}
	for _, pair := range scenario.pairs {
		syncSet.Spec.Secrets = append(syncSet.Spec.Secrets, hivev1.SecretMapping{
			SourceRef: hivev1.SecretReference{Name: pair.sourceName, Namespace: clusterNamespace},
			TargetRef: hivev1.SecretReference{Name: pair.targetName, Namespace: secretMappingTargetNamespace},
		})
	}
	if withResource && scenario.changesSyncSet() {
		syncSet.Spec.Resources = []runtime.RawExtension{
			{
				Object: &corev1.ConfigMap{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      scenario.configMapName,
						Namespace: secretMappingTargetNamespace,
					},
					Data: burstConfigMapData(),
				},
			},
		}
	}
	return syncSet
}

// burstSourceData builds the contents a source secret holds at a given revision of a burst.
func burstSourceData(sourceIndex, revision int) map[string][]byte {
	return secretMappingData(fmt.Sprintf("burst-source-%d-value-%d", sourceIndex+1, revision))
}

// runSecretMappingBurst creates the fixtures of a burst scenario and runs it: the source secrets
// are updated several times in a row with no wait for the target cluster in between, each
// mutation followed by a single attempt at deleting the ClusterSyncLease, and the final contents
// are then observed by reading only.
func runSecretMappingBurst(hiveClient, targetClusterClient client.Client, kubeClient kclient.Interface, scenario burstScenario) {
	ctx := context.TODO()
	// The sequence of updates below is written for two mapped secrets.
	Ω(scenario.pairs).Should(HaveLen(2))
	scenarioStart := time.Now()
	var mutations []burstMutation

	defer captureSecretMappingArtifacts(hiveClient, targetClusterClient, kubeClient, scenario.syncSetName, artifactStageAfter, scenarioStart, scenario.pairs)
	defer func() { writeBurstTimeline(scenario.syncSetName, mutations) }()

	By("Create the dedicated source secrets of the scenario in the cluster namespace")
	for i, pair := range scenario.pairs {
		err := createNamedSourceSecret(hiveClient, pair.sourceName, burstSourceData(i, 1))
		Ω(err).ShouldNot(HaveOccurred())
	}

	// Everything that talks to a cluster before the burst happens here, while the SyncSet does
	// not exist yet. Capturing after the SyncSet was created would put several API calls, and a
	// controller log fetch, between its creation and the first update.
	By("Snapshot the state of the scenario before the syncSet is created")
	captureSecretMappingArtifacts(hiveClient, targetClusterClient, kubeClient, scenario.syncSetName, artifactStageBefore, scenarioStart, scenario.pairs)

	By("Create the syncSet mapping the source secrets to the target cluster")
	createdSyncSet := burstSecretMappingSyncSet(scenario, false)
	err := hiveClient.Create(ctx, createdSyncSet)
	Ω(err).ShouldNot(HaveOccurred())
	// The object the API server returned, written from memory so that nothing stands between the
	// creation of the SyncSet and the first update of a source secret.
	writeArtifact(fmt.Sprintf("secretmapping-%s-%s-syncset-create-response.yaml", scenario.syncSetName, artifactStageBefore), createdSyncSet)

	// The updates below are issued without waiting for the mapping to converge on the target
	// cluster. Whether any of them reaches the hub while an apply is actually running is left to
	// the runtime: this spec does not order them against the controller, and does not assert
	// that it did. writeSource keeps the contents each source secret last received, so that the
	// final assertions cannot drift away from what the burst actually wrote.
	latest := make([]map[string][]byte, len(scenario.pairs))
	writeSource := func(index, revision int) {
		data := burstSourceData(index, revision)
		Ω(updateNamedSourceSecretData(hiveClient, scenario.pairs[index].sourceName, data)).ShouldNot(HaveOccurred())
		latest[index] = data
	}

	By("Update one source secret without waiting for the initial convergence, and trigger once")
	writeSource(0, 2)
	mutations = append(mutations, recordBurstMutation(hiveClient, "update-one-without-waiting-for-initial-convergence", scenario.pairs, scenario.syncSetName))

	By("Update both source secrets without waiting for the target cluster, and trigger once")
	writeSource(0, 3)
	writeSource(1, 2)
	mutations = append(mutations, recordBurstMutation(hiveClient, "update-both", scenario.pairs, scenario.syncSetName))

	By("Update both source secrets to their final contents, and trigger once")
	writeSource(0, 4)
	writeSource(1, 3)
	mutations = append(mutations, recordBurstMutation(hiveClient, "update-both-final", scenario.pairs, scenario.syncSetName))

	if scenario.changesSyncSet() {
		By("Change the syncSet itself at the end of the burst, and trigger once")
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			syncSet := &hivev1.SyncSet{}
			if err := hiveClient.Get(ctx, client.ObjectKey{Name: scenario.syncSetName, Namespace: clusterNamespace}, syncSet); err != nil {
				return err
			}
			syncSet.Spec.Resources = burstSecretMappingSyncSet(scenario, true).Spec.Resources
			return hiveClient.Update(ctx, syncSet)
		})
		Ω(err).ShouldNot(HaveOccurred())
		mutations = append(mutations, recordBurstMutation(hiveClient, "add-resource-to-syncset", scenario.pairs, scenario.syncSetName))
	}

	// Nothing else is written to the hub or the target cluster from here on, beyond the
	// diagnostics and the cleanup. In particular the burst is not followed by another
	// lease deletion, by a wait for one to become possible, or by any other way of making a
	// re-apply happen: what the target cluster ends up with is what the mutations above, and the
	// attempts that accompanied them, brought about.

	// Both checks below run before the spec waits for anything, so that an attempt which could
	// not do its work is reported as itself rather than as the propagation timeout that would
	// follow it. Neither requires an attempt to have deleted a lease: an attempt that found
	// none did exactly what a one-shot trigger does in a burst.
	By("Verify no trigger attempt of the burst ended with an unknown effect")
	Ω(triggerOutcomes(mutations, leaseDeleteUnknown)).Should(BeEmpty(),
		"a lease deletion failed leaving its effect unknown, so the triggers this burst fired cannot be established")
	By("Verify every trigger attempt of the burst got as far as the deletion call")
	Ω(triggerOutcomes(mutations, leaseNotDeletedReadFailed)).Should(BeEmpty(),
		"a lease could not be read, so that trigger never attempted a deletion")

	// Reads only from here on. No further trigger is issued, in particular not to repair
	// contents that have not arrived.
	By("Verify the mapped secrets end up holding exactly what their source secrets hold")
	for i, pair := range scenario.pairs {
		err = waitForNamedTargetSecretData(targetClusterClient, pair.targetName, latest[i], burstConvergeTimeout)
		Ω(err).ShouldNot(HaveOccurred())
		verifyNamedMappedSecret(hiveClient, targetClusterClient, pair, latest[i])
	}

	finalSyncSet := &hivev1.SyncSet{}
	err = hiveClient.Get(ctx, client.ObjectKey{Name: scenario.syncSetName, Namespace: clusterNamespace}, finalSyncSet)
	Ω(err).ShouldNot(HaveOccurred())
	if scenario.changesSyncSet() {
		By("Verify the resource added to the syncSet during the burst is applied as well")
		err = waitForTargetConfigMapData(targetClusterClient, scenario.configMapName, burstConfigMapData(), burstConvergeTimeout)
		Ω(err).ShouldNot(HaveOccurred())
	} else {
		By("Verify the syncSet was not touched after it was created")
		Ω(finalSyncSet.Generation).Should(Equal(createdSyncSet.Generation))
		Ω(finalSyncSet.Spec).Should(Equal(createdSyncSet.Spec))
	}

	By("Verify the syncSet is reported as applied at the generation it ends the burst with")
	// The status is written at the end of a re-apply pass, after the objects it applied, so it
	// is waited for rather than read straight after the contents arrived.
	err = waitForSyncSetApplied(clusterNamespace, clusterName, scenario.syncSetName, "syncset")
	Ω(err).ShouldNot(HaveOccurred())
	verifySyncSetStatus(hiveClient, scenario.syncSetName, finalSyncSet.Generation)
}

// createSecretMappingSyncSet creates the synthetic source secret along with a SyncSet mapping it
// to the target cluster, waits until the mapped secret holds the contents of the source, and
// snapshots the state the scenario starts from.
func createSecretMappingSyncSet(hiveClient, targetClusterClient client.Client, kubeClient kclient.Interface, syncSetName string, data map[string][]byte, scenarioStart time.Time) {
	By("Create a dedicated source secret in the cluster namespace")
	err := createNamedSourceSecret(hiveClient, secretMappingSourceSecretName, data)
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
	captureSecretMappingArtifacts(hiveClient, targetClusterClient, kubeClient, syncSetName, artifactStageBefore, scenarioStart, defaultSecretMappingPairs)
}

// createNamedSourceSecret creates a synthetic source secret in the cluster namespace. A secret
// left behind by an earlier spec whose cleanup did not complete is updated instead, so that one
// failing spec does not cascade into the next.
func createNamedSourceSecret(c client.Client, name string, data map[string][]byte) error {
	err := c.Create(context.TODO(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: clusterNamespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	})
	if errors.IsAlreadyExists(err) {
		return updateNamedSourceSecretData(c, name, data)
	}
	return err
}

// updateSourceSecretData changes the contents of the source secret, leaving every other object --
// in particular the SyncSet referencing it -- untouched.
func updateSourceSecretData(c client.Client, data map[string][]byte) error {
	return updateNamedSourceSecretData(c, secretMappingSourceSecretName, data)
}

// updateNamedSourceSecretData changes the contents of the named source secret only.
func updateNamedSourceSecretData(c client.Client, name string, data map[string][]byte) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		secret := &corev1.Secret{}
		if err := c.Get(context.TODO(), client.ObjectKey{Name: name, Namespace: clusterNamespace}, secret); err != nil {
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

// tryDeleteClusterSyncLeaseOnce makes a single attempt at deleting the ClusterSyncLease of the
// cluster under test. Unlike deleteClusterSyncLease it does not wait for the lease to exist and
// never retries, which is what lets a burst scenario fire one trigger per mutation without the
// trigger itself turning into a wait for the previous re-apply.
//
// The outcome is returned rather than asserted: a lease that was not there is a legitimate result
// of a burst, since an earlier attempt may have deleted one the controller has not recreated yet.
// An attempt whose effect cannot be known is reported as such and never rounded to either of the
// definite outcomes.
func tryDeleteClusterSyncLeaseOnce(c client.Client) (leaseDeleteOutcome, string) {
	ctx, cancel := context.WithTimeout(context.Background(), burstTriggerTimeout)
	defer cancel()

	lease := &hiveintv1alpha1.ClusterSyncLease{}
	switch err := c.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, lease); {
	case errors.IsNotFound(err):
		return leaseNotDeleted, "lease did not exist"
	case err != nil:
		// No deletion was issued, so this attempt certainly removed nothing.
		return leaseNotDeletedReadFailed, fmt.Sprintf("could not read the lease: %v", err)
	}
	switch err := c.Delete(ctx, lease, client.Preconditions{UID: &lease.UID}); {
	case err == nil:
		return leaseDeleted, fmt.Sprintf("deleted lease with uid %s", lease.UID)
	case errors.IsNotFound(err):
		return leaseNotDeleted, "lease was gone by the time it was deleted"
	case errors.IsConflict(err):
		return leaseNotDeleted, "lease had been replaced by another one"
	default:
		return leaseDeleteUnknown, fmt.Sprintf("delete failed with an unknown effect: %v", err)
	}
}

// recordBurstMutation reads the resource versions the mutations produced, fires a single trigger
// attempt and returns what the attempt achieved, as one entry of the burst timeline.
func recordBurstMutation(c client.Client, name string, pairs []secretMappingPair, syncSetName string) burstMutation {
	ctx := context.TODO()
	mutation := burstMutation{
		Name:                   name,
		AtUTC:                  time.Now().UTC().Format(time.RFC3339Nano),
		SourceResourceVersions: map[string]string{},
	}
	for _, pair := range pairs {
		secret := &corev1.Secret{}
		if err := c.Get(ctx, client.ObjectKey{Name: pair.sourceName, Namespace: clusterNamespace}, secret); err != nil {
			mutation.SourceResourceVersions[pair.sourceName] = fmt.Sprintf("unavailable: %v", err)
			continue
		}
		mutation.SourceResourceVersions[pair.sourceName] = secret.ResourceVersion
	}
	syncSet := &hivev1.SyncSet{}
	if err := c.Get(ctx, client.ObjectKey{Name: syncSetName, Namespace: clusterNamespace}, syncSet); err != nil {
		mutation.SyncSetGeneration = fmt.Sprintf("unavailable: %v", err)
	} else {
		mutation.SyncSetGeneration = strconv.FormatInt(syncSet.Generation, 10)
	}

	outcome, detail := tryDeleteClusterSyncLeaseOnce(c)
	mutation.LeaseDeleteOutcome = outcome
	mutation.LeaseDeleteDetail = detail
	fmt.Fprintf(GinkgoWriter, "burst mutation %s at %s: trigger %s (%s)\n", name, mutation.AtUTC, outcome, detail)
	return mutation
}

// writeBurstTimeline stores the mutations of a burst as an artifact, so that the order of the
// mutations, the resource versions they produced and the outcome of each trigger attempt can be
// lined up with the manifests and the controller log captured alongside them.
func writeBurstTimeline(syncSetName string, mutations []burstMutation) {
	content, err := yaml.Marshal(map[string]any{"mutations": mutations})
	if err != nil {
		fmt.Fprintf(GinkgoWriter, "artifact timeline for %s: could not serialize: %v\n", syncSetName, err)
		return
	}
	writeArtifactBytes(fmt.Sprintf("secretmapping-%s-timeline.yaml", syncSetName), content)
}

// triggerOutcomes returns the mutations whose trigger attempt ended with the given outcome, named
// along with the detail recorded for them.
func triggerOutcomes(mutations []burstMutation, outcome leaseDeleteOutcome) []string {
	var matching []string
	for _, mutation := range mutations {
		if mutation.LeaseDeleteOutcome == outcome {
			matching = append(matching, fmt.Sprintf("%s: %s", mutation.Name, mutation.LeaseDeleteDetail))
		}
	}
	return matching
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
	return waitForNamedTargetSecretData(c, secretMappingTargetSecretName, want, timeout)
}

// waitForNamedTargetSecretData waits for the named mapped secret on the target cluster to hold
// exactly the expected contents.
func waitForNamedTargetSecretData(c client.Client, name string, want map[string][]byte, timeout time.Duration) error {
	var observed map[string][]byte
	var lastGetErr error
	if err := wait.PollUntilContextTimeout(context.Background(), secretMappingPollInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			secret := &corev1.Secret{}
			if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: secretMappingTargetNamespace}, secret); err != nil {
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
			secretMappingTargetNamespace, name, mismatchedKeys(want, observed), lastGetErr, err)
	}
	return nil
}

// waitForTargetConfigMapData waits for a ConfigMap applied by a SyncSet to hold the expected
// contents on the target cluster.
func waitForTargetConfigMapData(c client.Client, name string, want map[string]string, timeout time.Duration) error {
	var lastErr error
	if err := wait.PollUntilContextTimeout(context.Background(), secretMappingPollInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			configMap := &corev1.ConfigMap{}
			if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: secretMappingTargetNamespace}, configMap); err != nil {
				lastErr = err
				return false, nil
			}
			lastErr = nil
			return reflect.DeepEqual(configMap.Data, want), nil
		}); err != nil {
		return fmt.Errorf("configMap %s/%s on the target cluster did not reach the expected contents (last get error: %v): %w",
			secretMappingTargetNamespace, name, lastErr, err)
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
	verifyNamedMappedSecret(hiveClient, targetClusterClient, defaultSecretMappingPair, want)
}

// verifyNamedMappedSecret asserts that the target of the given mapping is a faithful copy of its
// source.
func verifyNamedMappedSecret(hiveClient, targetClusterClient client.Client, pair secretMappingPair, want map[string][]byte) {
	ctx := context.TODO()

	sourceSecret := &corev1.Secret{}
	err := hiveClient.Get(ctx, client.ObjectKey{Name: pair.sourceName, Namespace: clusterNamespace}, sourceSecret)
	Ω(err).ShouldNot(HaveOccurred())
	Ω(sourceSecret.Data).Should(Equal(want))

	targetSecret := &corev1.Secret{}
	err = targetClusterClient.Get(ctx, client.ObjectKey{Name: pair.targetName, Namespace: secretMappingTargetNamespace}, targetSecret)
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
// Secrets are captured for the pairs the caller passes and for no others, and callers pass pairs
// this suite created: no secret of the hub or of the target cluster that the suite does not own
// is read, let alone written out.
func captureSecretMappingArtifacts(hiveClient, targetClusterClient client.Client, kubeClient kclient.Interface, syncSetName, stage string, since time.Time, pairs []secretMappingPair) {
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
	capture(hiveClient, clusterKey, &hiveintv1alpha1.ClusterSync{}, "clustersync")
	capture(hiveClient, clusterKey, &hiveintv1alpha1.ClusterSyncLease{}, "clustersynclease")
	for _, pair := range pairs {
		capture(hiveClient, client.ObjectKey{Name: pair.sourceName, Namespace: clusterNamespace},
			&corev1.Secret{}, fmt.Sprintf("source-secret-%s", pair.sourceName))
		capture(targetClusterClient, client.ObjectKey{Name: pair.targetName, Namespace: secretMappingTargetNamespace},
			&corev1.Secret{}, fmt.Sprintf("target-secret-%s", pair.targetName))
	}

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
