package syncsets_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/fields"
	clientwatch "k8s.io/client-go/tools/watch"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
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

var _ = Describe("Test Syncset and SelectorSyncSet func", func() {

	hiveClient := common.MustGetClient()
	targetClusterClient := common.MustGetClientFromConfig(common.MustGetClusterDeploymentClientConfig())

	// release resource after test
	AfterEach(func() {
		deleteAllSyncSets(hiveClient, clusterNamespace)
		deleteAllSelectorSyncSets(hiveClient, clusterNamespace)
		deleteConfigMap(targetClusterClient, "default", "foo")
		deleteSecret(targetClusterClient, "default", "test-aws-creds")
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
		})

		Context("Test selectorSynSet", func() {
			It("Test selectorSynSet of resources,patches,and secretMappings", func() {
				ctx := context.TODO()
				By("Get the pull secret name of clusterdeployment")
				cd := &hivev1.ClusterDeployment{}
				err := hiveClient.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, cd)
				Ω(err).ShouldNot(HaveOccurred())
				pullSecret := cd.Spec.PullSecretRef.Name

				By(`Set a label "cluster-group: hivecluster" to clusterdeployment`)
				cdLabels := cd.ObjectMeta.GetLabels()
				cdLabels["cluster-group"] = "hivecluster"
				cd.ObjectMeta.SetLabels(cdLabels)
				err = hiveClient.Update(ctx, cd)
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
				cdLabels = cd.ObjectMeta.GetLabels()
				delete(cdLabels, "cluster-group")
				cd.ObjectMeta.SetLabels(cdLabels)
				err = hiveClient.Update(ctx, cd)
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
	restClient, err := apiutil.RESTClientForGVK(gvk, false, cfg, serializer.NewCodecFactory(scheme), hc)
	if err != nil {
		return err
	}
	listWatcher := cache.NewListWatchFromClient(restClient, "clustersyncs", namespace, fields.OneTermEqualSelector("metadata.name", cdName))
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
	restClient, err := apiutil.RESTClientForGVK(gvk, false, cfg, serializer.NewCodecFactory(scheme), hc)
	if err != nil {
		return err
	}
	listWatcher := cache.NewListWatchFromClient(restClient, "syncsets", namespace, fields.OneTermEqualSelector("metadata.name", syncsetname))
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
	restClient, err := apiutil.RESTClientForGVK(gvk, false, cfg, serializer.NewCodecFactory(scheme), hc)
	if err != nil {
		return err
	}
	listWatcher := cache.NewListWatchFromClient(restClient, "clustersyncs", namespace, fields.OneTermEqualSelector("metadata.name", cdName))
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
