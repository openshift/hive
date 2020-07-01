package syncsets_test

import (
	"context"

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
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/fields"
	clientwatch "k8s.io/client-go/tools/watch"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
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

				By("Create a syncset resources using 'Sync' apply mode and verify syncsetinstance created successfully")
				syncSetWithSyncApplyMode := testSyncSet(hivev1.SyncResourceApplyMode)
				err := hiveClient.Create(ctx, syncSetWithSyncApplyMode)
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetInstanceApplied(clusterNamespace, "test-syncresource", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the resource is synced on taret cluster")
				resultConfigMap := &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

				By("Delete the syncset and verify syncset and syncsetinstance are deleted")
				deleteSyncSets(hiveClient, clusterNamespace, "test-syncresource")
				err = waitForSyncSetDeleted(clusterNamespace, "test-syncresource")
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetInstanceDeleted(clusterNamespace, "test-syncresource", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the ConfigMap is deleted on the target cluster")
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(errors.IsNotFound(err)).Should(BeTrue())

				By("Create a syncset resource using 'Upsert' apply mode, verify syncsetinstance created successfully")
				syncSetWithUpsertApplyMode := testSyncSet(hivev1.UpsertResourceApplyMode)
				err = hiveClient.Create(ctx, syncSetWithUpsertApplyMode)
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetInstanceApplied(clusterNamespace, "test-syncresource", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the resource is synced on target cluster")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

				By("Verify the managed-by-Hive label was injected automatically")
				Ω(resultConfigMap.Labels[constants.HiveManagedLabel]).Should(Equal("true"))

				By("Delete the syncset and verify syncset and syncsetinstance are deleted")
				deleteSyncSets(hiveClient, clusterNamespace, "test-syncresource")
				err = waitForSyncSetDeleted(clusterNamespace, "test-syncresource")
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetInstanceDeleted(clusterNamespace, "test-syncresource", "syncset")
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
				err = waitForSyncSetInstanceApplied(clusterNamespace, "test-syncpatch", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the resource is patched on target cluster")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "baz-merge"}))

				By("Delete the SyncSetPatch and verify syncset and syncsetinstance are deleted")
				deleteSyncSets(hiveClient, clusterNamespace, "test-syncpatch")
				err = waitForSyncSetDeleted(clusterNamespace, "test-syncpatch")
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetInstanceDeleted(clusterNamespace, "test-syncpatch", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Create a syncpatch with strategic patchType and verify syncpatch create successfully")
				patch = `{ "data": { "foo": "baz-strategic" } }`
				patchType = "strategic"
				syncPatchWithstrategicType := testSyncSetPatch(patch, patchType)
				err = hiveClient.Create(ctx, syncPatchWithstrategicType)
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetInstanceApplied(clusterNamespace, "test-syncpatch", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Verify the resource is patched on target cluster")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "baz-strategic"}))

				By("Delete the SyncSetPatch and verify syncset and syncsetinstance are deleted")
				deleteSyncSets(hiveClient, clusterNamespace, "test-syncpatch")
				err = waitForSyncSetDeleted(clusterNamespace, "test-syncpatch")
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetInstanceDeleted(clusterNamespace, "test-syncpatch", "syncset")
				Ω(err).ShouldNot(HaveOccurred())

				By("Create a syncpatch with json patchType and verify syncpatch created successfully")
				patch = `[ { "op": "replace", "path": "/data/foo", "value": "baz-json" } ]`
				patchType = "json"
				syncPatchWithPatchType := testSyncSetPatch(patch, patchType)
				err = hiveClient.Create(ctx, syncPatchWithPatchType)
				Ω(err).ShouldNot(HaveOccurred())
				err = waitForSyncSetInstanceApplied(clusterNamespace, "test-syncpatch", "syncset")
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
				err = waitForSyncSetInstanceApplied(clusterNamespace, "test-syncsecret", "syncset")
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
				err = waitForSyncSetInstanceApplied(clusterNamespace, "test-selectorsyncset", "selectorsyncset")
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

				By("Verify syncset and syncsetinstance are deleted")
				err = waitForSyncSetInstanceDeleted(clusterNamespace, "test-selectorsyncset", "selectorsyncset")
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

func waitForSyncSetInstanceApplied(namespace, syncsetname, syncsettype string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := common.MustGetConfig()
	gvk, err := apiutil.GVKForObject(&hivev1.SyncSetInstance{}, scheme.Scheme)
	if err != nil {
		return err
	}
	restClient, err := apiutil.RESTClientForGVK(gvk, cfg, serializer.NewCodecFactory(scheme.Scheme))
	if err != nil {
		return err
	}
	labelSelectorFilter := func(options *metav1.ListOptions) {
		switch {
		case syncsettype == "syncset":
			options.LabelSelector = "hive.openshift.io/syncset-name=" + syncsetname
		case syncsettype == "selectorsyncset":
			options.LabelSelector = "hive.openshift.io/selector-syncset-name=" + syncsetname
		}
	}
	listWatcher := cache.NewFilteredListWatchFromClient(restClient, "syncsetinstances", namespace, labelSelectorFilter)
	syncSetInstanceApplied := func(event watch.Event) (bool, error) {
		if event.Type != watch.Added && event.Type != watch.Modified {
			return false, nil
		}
		syncSetInstance, ok := event.Object.(*hivev1.SyncSetInstance)
		if !ok {
			// Object is not of type syncssetinstance
			return false, nil
		}
		return syncSetInstance.Status.Applied, nil
	}
	_, err = clientwatch.UntilWithSync(ctx, listWatcher, &hivev1.SyncSetInstance{}, nil, syncSetInstanceApplied)
	return err
}

func waitForSyncSetDeleted(namespace, syncsetname string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := common.MustGetConfig()
	gvk, err := apiutil.GVKForObject(&hivev1.SyncSet{}, scheme.Scheme)
	if err != nil {
		return err
	}
	restClient, err := apiutil.RESTClientForGVK(gvk, cfg, serializer.NewCodecFactory(scheme.Scheme))
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

func waitForSyncSetInstanceDeleted(namespace, syncsetname, syncsettype string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg := common.MustGetConfig()
	gvk, err := apiutil.GVKForObject(&hivev1.SyncSetInstance{}, scheme.Scheme)
	if err != nil {
		return err
	}
	restClient, err := apiutil.RESTClientForGVK(gvk, cfg, serializer.NewCodecFactory(scheme.Scheme))
	if err != nil {
		return err
	}
	labelSelectorFilter := func(options *metav1.ListOptions) {
		switch {
		case syncsettype == "syncset":
			options.LabelSelector = "hive.openshift.io/syncset-name=" + syncsetname
		case syncsettype == "selectorsyncset":
			options.LabelSelector = "hive.openshift.io/selector-syncset-name=" + syncsetname
		}
	}
	listWatcher := cache.NewFilteredListWatchFromClient(restClient, "syncsetinstances", namespace, labelSelectorFilter)
	_, err = clientwatch.UntilWithSync(
		ctx,
		listWatcher,
		&hivev1.SyncSetInstance{},
		func(store cache.Store) (bool, error) {
			return len(store.List()) == 0, nil
		},
		func(event watch.Event) (bool, error) {
			return event.Type == watch.Deleted, nil
		})
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
