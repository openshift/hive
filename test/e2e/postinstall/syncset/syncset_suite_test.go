package syncset_test

import (
	"context"
	// "fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/test/e2e/common"
)

func TestSyncset(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Syncset Suite")
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
		// common.RunShellCmd(fmt.Sprintf(`oc delete syncset --all -n %s`, clusterNamespace))
		deleteAllSyncSets(hiveClient, clusterNamespace)
		// common.RunShellCmd(fmt.Sprintf(`oc delete SelectorSyncSet --all -n hive`))
		deleteAllSelectorSyncSets(hiveClient, clusterNamespace)

		// common.RunShellCmdWithEnv(newEnv, assetDir, fmt.Sprintf(`oc delete configmap foo`))
		deleteConfigMap(targetClusterClient, "default", "foo")
		// common.RunShellCmdWithEnv(newEnv, assetDir, fmt.Sprintf(`oc delete configmap test-foo`))
		deleteConfigMap(targetClusterClient, "default", "test-foo")
		// common.RunShellCmdWithEnv(newEnv, assetDir, fmt.Sprintf(`oc delete secret test-aws-creds`))
		deleteSecret(targetClusterClient, "default", "test-aws-creds")
		// os.RemoveAll(tempDir)
	})
	Describe("Test Syncset", func() {
		Context("Test managing target cluster resource via syncset", func() {
			It("OCP-23040:Create SyncSet resource, the resource that allows you to create resources in remote clusters", func() {
				// change2clusterENV(clusterName)
				/*
									syncSet := `echo "apiVersion: hive.openshift.io/v1
					kind: SyncSet
					metadata:
					  name: test-syncresource
					spec:
					  clusterDeploymentRefs:
					  - name: %s
					  resourceApplyMode: Sync
					  resources:
					  - kind: ConfigMap
					    apiVersion: v1
					    metadata:
					      name: foo
					      namespace: default
					    data:
					      foo: bar" | kubectl -n %s apply -f -
					`
				*/
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

				// TODO: This should be replaced with code that waits for the SyncSetInstance to report that the SyncSet was synced
				time.Sleep(5 * time.Second)

				By("Verifying the resource was synced")
				resultConfigMap := &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

				By("Deleting the syncset resource with sync apply mode")
				err = hiveClient.Delete(ctx, syncSetWithSyncApplyMode)
				Ω(err).ShouldNot(HaveOccurred())

				// TODO: This should be replaced with code that waits for the SyncSetInstance to report that the SyncSet was synced
				time.Sleep(5 * time.Second)

				By("Verifying the ConfigMap was deleted on the target cluster")
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(errors.IsNotFound(err)).Should(BeTrue())

				By("Creating a syncset with a resource using 'Upsert' apply mode")
				syncSetWithUpsertApplyMode := testSyncSet(hivev1.UpsertResourceApplyMode)
				err = hiveClient.Create(ctx, syncSetWithUpsertApplyMode)
				Ω(err).ShouldNot(HaveOccurred())

				// TODO: This should be replaced with code that waits for the SyncSetInstance to report that the SyncSet was synced
				time.Sleep(5 * time.Second)

				By("Verifying the resource was synced")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))

				By("Deleting the syncset resource with upsert apply mode")
				err = hiveClient.Delete(ctx, syncSetWithUpsertApplyMode)
				Ω(err).ShouldNot(HaveOccurred())

				// TODO: This should be replaced with code that waits for the SyncSetInstance to report that the SyncSet was synced
				time.Sleep(5 * time.Second)

				By("Verifying the ConfigMap was not deleted on the target cluster")
				resultConfigMap = &corev1.ConfigMap{}
				err = targetClusterClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "default"}, resultConfigMap)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resultConfigMap.Data).Should(Equal(map[string]string{"foo": "bar"}))
			})
			/*
			   			It("OCP-23144:SyncSet patching", func() {
			   				//Create a ConfigMap resource "foo" on target cluster
			   				configMapIsCreated := createConfigmapOnRemoteCluster(clusterName)
			   				Ω(configMapIsCreated).Should(Equal(true))
			   				syncSetPatch := `echo "apiVersion: hive.openshift.io/v1
			   kind: SyncSet
			   metadata:
			     name: test-syncpatch
			   spec:
			     clusterDeploymentRefs:
			     - name: %s
			     patches:
			     - kind: ConfigMap
			       apiVersion: v1
			       name: foo
			       namespace: default
			       patch: |-
			         { \"data\": { \"foo\": \"new-bar\" } }
			       patchType: merge" | kubectl -n %s apply -f -
			   `
			   				data := fmt.Sprintf(syncSetPatch, clusterName, clusterNamespace)
			   				cmd := `oc get configmap foo -o yaml`
			   				//Patch data from "foo: bar" to "foo: new-bar" and check
			   				syncSetResourceIsApplied := applySyncSetAndCheck(data, "foo: new-bar", cmd)
			   				Ω(syncSetResourceIsApplied).Should(Equal(true))

			   			})

			   			It("OCP-25334:SyncSet controllers support SecretReference syncing", func() {
			   				change2clusterENV(clusterName)
			   				syncSetSecretRef := `echo "apiVersion: hive.openshift.io/v1
			   kind: SyncSet
			   metadata:
			     name: test-syncsetsecretref
			   spec:
			     clusterDeploymentRefs:
			     - name: %s
			     resourceApplyMode: Sync
			     secretMappings:
			     - sourceRef:
			         name: %s-pull-secret
			         namespace: %s
			       targetRef:
			         name: test-copy-pull-secret
			         namespace: default" | kubectl -n %s apply -f -
			   `
			   				data := fmt.Sprintf(syncSetSecretRef, clusterName, clusterName, clusterNamespace, clusterNamespace)
			   				cmd := `oc get secret -n default`
			   				//Copy secret from local cluster to target cluster
			   				syncSetResourceIsApplied := applySyncSetAndCheck(data, "test-copy-pull-secret", cmd)
			   				Ω(syncSetResourceIsApplied).Should(Equal(true))

			   			})
			   		})
			   	})

			   	Context("Test managing target cluster resource via SelectorSyncSet", func() {
			   		It("test create/patch resource and copy secretRef via SelectorSyncSet", func() {
			   			//Firstly,create a ComfigMap resource "foo" with "data: {foo:bar}" on target cluster
			   			configMapIsCreated := createConfigmapOnRemoteCluster(clusterName)
			   			Ω(configMapIsCreated).Should(Equal(true))
			   			selectSyncSet := `echo "apiVersion: hive.openshift.io/v1
			   kind: SelectorSyncSet
			   metadata:
			     name: test-selectorsyncset
			   spec:
			     clusterDeploymentSelector:
			       matchLabels:
			         cluster-group: hivecluster
			     resourceApplyMode: Sync
			     resources:
			     - kind: ConfigMap
			       apiVersion: v1
			       metadata:
			         name: test-foo
			         namespace: default
			       data:
			         test-foo: test-bar
			     patches:
			     - kind: ConfigMap
			       apiVersion: v1
			       name: foo
			       namespace: default
			       patch: |-
			         { \"data\": { \"foo\": \"new-bar\" } }
			       patchType: merge
			     secretMappings:
			     - sourceRef:
			         name: %s-aws-creds
			         namespace: %s
			       targetRef:
			         name: test-copy-aws-creds
			         namespace: default" | kubectl -n hive apply -f -
			   `
			   			//Set label "cluster-group: hivecluster" on test clusterdeployment
			   			setLabelOnCluster(clusterName, clusterNamespace)
			   			data := fmt.Sprintf(selectSyncSet, clusterName, clusterNamespace)
			   			cmd := `oc get configmap test-foo -o yaml`
			   			//create SelectorSyncSet resources in namespace(namespace：hive) which is different from clusterdeployment namespace
			   			syncSetResourceIsApplied := applySyncSetAndCheck(data, "test-foo: test-bar", cmd)
			   			cmd = `oc get configmap foo -o yaml`
			   			syncSetResourceIsApplied = applySyncSetAndCheck("", "foo: new-bar", cmd)
			   			Ω(syncSetResourceIsApplied).Should(Equal(true))
			   			cmd = `oc get secret`
			   			syncSetResourceIsApplied = applySyncSetAndCheck("", "test-copy-aws-creds", cmd)

			*/
		})
	})
})

/*
func applySyncSetAndCheck(data, expValue, cmd string) bool {
	if data != "" {
		r, e := common.RunShellCmd(data)
		common.PrintResultError(r, e)
		time.Sleep(5 * time.Second)
	}
	//Check whether resource was applied correctly
	result, err := common.RunShellCmdWithEnv(newEnv, assetDir, fmt.Sprintf(cmd))
	if common.InsenstiveContains(result, expValue) {
		return true
	}
	fmt.Println("result:", result, "err: ", err)
	return false
}

func createConfigmapOnRemoteCluster(clustername string) bool {
	configMapExample := `echo "apiVersion: v1
kind: ConfigMap
metadata:
  name: foo
  namespace: default
data:
  foo: bar" | kubectl apply -f -`
	result, err := common.RunShellCmdWithEnv(newEnv, assetDir, fmt.Sprintf(configMapExample))
	common.PrintResultError(result, err)
	// Check whether ConfigMap was created successfully
	result, err = common.RunShellCmdWithEnv(newEnv, assetDir, fmt.Sprintf(`oc get configmap foo -o yaml`))
	if common.InsenstiveContains(result, "foo: bar") {
		return true
	}
	common.PrintResultError(result, err)
	return false
}

func change2clusterENV(clustername string) {
	newEnv = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s/%s.kubeconfig", assetDir, clustername))
}

func setLabelOnCluster(clustername, namespace string) {
	cmd := `oc -n %s label cd %s cluster-group='hivecluster'`
	common.RunShellCmd(fmt.Sprintf(cmd, namespace, clustername))
}
*/

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
	err := c.Delete(ctx, cm)
	Ω(err).ShouldNot(HaveOccurred())
}

func deleteSecret(c client.Client, namespace, name string) {
	ctx := context.TODO()
	secret := &corev1.Secret{}
	secret.Namespace = namespace
	secret.Name = name
	err := c.Delete(ctx, secret)
	Ω(err).ShouldNot(HaveOccurred())
}
