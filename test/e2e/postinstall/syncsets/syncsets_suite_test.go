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

	clientwatch "k8s.io/client-go/tools/watch"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/test/e2e/common"
)

func TestSyncsets(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Syncsets Suite")
}

var (
	assetDir         = "/tmp"
	clusterName      = os.Getenv("CLUSTER_NAME")
	clusterNamespace = os.Getenv("CLUSTER_NAMESPACE")
)

var _ = Describe("Test Syncset and SelectorSyncSet func", func() {

	hiveClient := common.MustGetClient()
	targetClusterClient := common.MustGetClientFromConfig(common.MustGetClusterDeploymentClientConfig())

	// release resource after test
	AfterEach(func() {
		// release all created resource
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

				By("Creating a syncset with a resource using 'Sync' apply mode")
				syncSetWithSyncApplyMode := testSyncSet(hivev1.SyncResourceApplyMode)
				err := hiveClient.Create(ctx, syncSetWithSyncApplyMode)
				Ω(err).ShouldNot(HaveOccurred())

				err = waitForSyncSetInstanceApplied("test-syncresource", "syncset", clusterNamespace)
				Ω(err).ShouldNot(HaveOccurred())

				By("Verifying the resource was synced")
				resultConfigMap := &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

				By("Deleting the syncset with sync apply mode")
				err = hiveClient.Delete(ctx, syncSetWithSyncApplyMode)
				Ω(err).ShouldNot(HaveOccurred())

				time.Sleep(5 * time.Second)

				By("Verifying the ConfigMap was deleted on the target cluster")
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(errors.IsNotFound(err)).Should(BeTrue())

				By("Creating a syncset with a resource using 'Upsert' apply mode")
				syncSetWithUpsertApplyMode := testSyncSet(hivev1.UpsertResourceApplyMode)
				err = hiveClient.Create(ctx, syncSetWithUpsertApplyMode)
				Ω(err).ShouldNot(HaveOccurred())

				err = waitForSyncSetInstanceApplied("test-syncresource", "syncset", clusterNamespace)
				Ω(err).ShouldNot(HaveOccurred())

				By("Verifying the resource was synced")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

				By("Deleting the syncset with upsert apply mode")
				err = hiveClient.Delete(ctx, syncSetWithUpsertApplyMode)
				Ω(err).ShouldNot(HaveOccurred())

				time.Sleep(5 * time.Second)

				By("Verifying the ConfigMap was not deleted on the target cluster")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

			})

			It("Test SyncSet patches", func() {
				ctx := context.TODO()
				configMap := &corev1.ConfigMap{
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
				}
				SyncSetPatch := &hivev1.SyncSet{
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
									Patch:      `{ "data": { "foo": "new-bar" } }`,
									PatchType:  "merge",
								},
							},
						},
					},
				}
				By("Creating a resource ConfigMap on target cluster")
				err := targetClusterClient.Create(ctx, configMap)
				Ω(err).ShouldNot(HaveOccurred())
				time.Sleep(5 * time.Second)

				By("Verify resource ConfigMap created on target cluster")
				resultConfigMap := &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

				By("Creating a patch with merge patchType")
				err = hiveClient.Create(ctx, SyncSetPatch)
				Ω(err).ShouldNot(HaveOccurred())

				err = waitForSyncSetInstanceApplied("test-syncpatch", "syncset", clusterNamespace)
				Ω(err).ShouldNot(HaveOccurred())

				By("Verifying the resource was patched")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "new-bar"}))

				By("Modifying the patch type to strategic")
				patch := `{ "data": { "foo": "baz-strategic" } }`
				patchType := "strategic"
				modifySynSetPatchType(hiveClient, SyncSetPatch, patch, patchType)

				err = waitForSyncSetInstanceApplied("test-syncpatch", "syncset", clusterNamespace)
				Ω(err).ShouldNot(HaveOccurred())

				By("Verifying the resource was patched with strategic patchType")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "baz-strategic"}))

				By("Modifying the patch type to json")
				patch = `[ { "op": "replace", "path": "/data/foo", "value": "baz-json" } ]`
				patchType = "json"
				modifySynSetPatchType(hiveClient, SyncSetPatch, patch, patchType)

				err = waitForSyncSetInstanceApplied("test-syncpatch", "syncset", clusterNamespace)
				Ω(err).ShouldNot(HaveOccurred())

				By("Verifying the resource was patched with json patchType")
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

				SyncSetSecretMappings := &hivev1.SyncSet{
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
							ResourceApplyMode: "Sync",
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
				By("Creating a syncSet SecretMappings")
				err = hiveClient.Create(ctx, SyncSetSecretMappings)
				Ω(err).ShouldNot(HaveOccurred())

				err = waitForSyncSetInstanceApplied("test-syncsecret", "syncset", clusterNamespace)
				Ω(err).ShouldNot(HaveOccurred())

				By("Verifying the secret was copied to target cluster")
				resultSecret := &corev1.Secret{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "test-pull-secret-copy", Namespace: "default"}, resultSecret)
				Ω(err).ShouldNot(HaveOccurred())

			})
		})
		Context("Test selectorSynSet", func() {
			It("Test selectorSynSet of resources,patches,and secretMappings", func() {
				ctx := context.TODO()
				By("Getting the pull secret name of clusterdeployment")
				cd := &hivev1.ClusterDeployment{}
				err := hiveClient.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, cd)
				Ω(err).ShouldNot(HaveOccurred())
				pullSecret := cd.Spec.PullSecretRef.Name

				By(`Set a label "cluster-group: hivecluster" to clusterdeployment`)
				cdLabels := cd.ObjectMeta.GetLabels()
				cdLabels["cluster-group"] = "hivecluster"
				cd.ObjectMeta.SetLabels(cdLabels)
				hiveClient.Update(ctx, cd)
				Ω(cd.ObjectMeta.Labels).Should(Equal(cdLabels))

				resourceConfigMap := &corev1.ConfigMap{
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
				}

				SelectorSyncSet := &hivev1.SelectorSyncSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-selectorsyncset",
						Namespace: "hive",
					},
					Spec: hivev1.SelectorSyncSetSpec{
						ClusterDeploymentSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"cluster-group": "hivecluster"},
						},
						SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
							ResourceApplyMode: "Sync",
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

				By("Creating a ConfigMap on target cluster")
				err = targetClusterClient.Create(ctx, resourceConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				time.Sleep(5 * time.Second)

				By("Verify resource ConfigMap created on target cluster")
				resultConfigMap := &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

				By("Creating a selectorSyncSet on target cluster including resources, patches and secretMappings")
				err = hiveClient.Create(ctx, SelectorSyncSet)

				err = waitForSyncSetInstanceApplied("test-selectorsyncset", "selectorsyncset", clusterNamespace)
				Ω(err).ShouldNot(HaveOccurred())

				By("Verifying the resource was synced")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo-selectorsyncset", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo-sSS": "bar-sSS"}))

				By("Verifying the resource was patched")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "new-bar"}))

				By("Verifying the secret was copied")
				resultSecret := &corev1.Secret{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "test-pull-secret-copy", Namespace: "default"}, resultSecret)
				Ω(err).ShouldNot(HaveOccurred())

				By(`Deleting the label "cluster-group: hivecluster" of clusterdeployment`)
				cdLabels = cd.ObjectMeta.GetLabels()
				delete(cdLabels, "cluster-group")
				cd.ObjectMeta.SetLabels(cdLabels)
				hiveClient.Update(ctx, cd)
				Ω(cd.ObjectMeta.Labels).Should(Equal(cdLabels))

				By("Verifying the resource was deleted")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo-selectorsyncset", Namespace: "default"}, resultConfigMap)
				Ω(errors.IsNotFound(err)).Should(BeTrue())

				By("Verifying the secret was deleted")
				resultSecret = &corev1.Secret{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "test-pull-secret-copy", Namespace: "default"}, resultSecret)
				Ω(errors.IsNotFound(err)).Should(BeTrue())

			})
		})
	})
})

func modifySynSetPatchType(c client.Client, s *hivev1.SyncSet, patch, patchtype string) {
	ctx := context.TODO()
	for i := range s.Spec.Patches {
		s.Spec.Patches[i].Patch = patch
		s.Spec.Patches[i].PatchType = patchtype
		err := c.Update(ctx, s)
		Ω(err).ShouldNot(HaveOccurred())
	}
}

func waitForSyncSetInstanceApplied(syncsetname, syncsettype, namespace string) error {
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
	c.Delete(ctx, cm)
	//Ω(err).ShouldNot(HaveOccurred())
}

func deleteSecret(c client.Client, namespace, name string) {
	ctx := context.TODO()
	secret := &corev1.Secret{}
	secret.Namespace = namespace
	secret.Name = name
	c.Delete(ctx, secret)
	//Ω(err).ShouldNot(HaveOccurred())
}
