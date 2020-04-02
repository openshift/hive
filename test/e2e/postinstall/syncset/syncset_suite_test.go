package syncset_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/hive/test/e2e/common"
)

func TestSyncset(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Syncset Suite")
}

var (
	assetDir                          = "/tmp"
	clusterName                       = os.Getenv("CLUSTER_NAME")
	clusterNamespace                  = os.Getenv("CLUSTER_NAMESPACE")
	clusterKubeconfigPath, tempDir, _ = common.Createkubeconfig(clusterName, clusterNamespace, assetDir)
	newEnv                            = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", clusterKubeconfigPath))
)

var _ = Describe("Test Syncset and SelectorSyncSet func", func() {
	// release resource after test
	AfterEach(func() {
		common.RunShellCmd(fmt.Sprintf(`oc delete syncset --all -n %s`, clusterNamespace))
		common.RunShellCmd(fmt.Sprintf(`oc delete SelectorSyncSet --all -n hive`))
		common.RunShellCmdWithEnv(newEnv, assetDir, fmt.Sprintf(`oc delete configmap foo`))
		common.RunShellCmdWithEnv(newEnv, assetDir, fmt.Sprintf(`oc delete configmap test-foo`))
		common.RunShellCmdWithEnv(newEnv, assetDir, fmt.Sprintf(`oc delete secret test-aws-creds`))
		os.RemoveAll(tempDir)
	})
	Describe("Test Syncset", func() {
		Context("Test managing target cluster resource via syncset", func() {
			It("OCP-23040:Create SyncSet resource, the resource that allows you to create resources in remote clusters", func() {
				change2clusterENV(clusterName)
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
				//Create ConfigMap using "Sync" resourceApplyMode and check
				data := fmt.Sprintf(syncSet, clusterName, clusterNamespace)
				cmd := `oc get configmap foo -o yaml`
				syncSetResourceIsApplied := applySyncSetAndCheck(data, "foo: bar", cmd)
				Ω(syncSetResourceIsApplied).Should(Equal(true))

				//Delete syncset resource and check ConfigMap was deleted on targat cluster.
				common.RunShellCmd(fmt.Sprintf(`oc delete syncset test-syncresource -n %s`, clusterNamespace))
				syncSetResourceIsApplied = applySyncSetAndCheck("", "foo: bar", cmd)
				Ω(syncSetResourceIsApplied).Should(Equal(false))
				syncSetU := `echo "apiVersion: hive.openshift.io/v1
kind: SyncSet
metadata:
  name: test-syncresource
spec:
  clusterDeploymentRefs:
  - name: %s
  resourceApplyMode: Upsert
  resources:
  - kind: ConfigMap
    apiVersion: v1
    metadata:
      name: foo
      namespace: default
    data:
      foo: bar" | kubectl -n %s apply -f -
`
				//Create ConfigMap using "Upsert" resourceApplyMode and check
				data = fmt.Sprintf(syncSetU, clusterName, clusterNamespace)
				syncSetResourceIsApplied = applySyncSetAndCheck(data, "foo: bar", cmd)
				Ω(syncSetResourceIsApplied).Should(Equal(true))
				//Delete syncset resource and check ConfigMap is still on targat cluster.
				common.RunShellCmd(fmt.Sprintf(`oc delete syncset test-syncresource -n %s`, clusterNamespace))
				syncSetResourceIsApplied = applySyncSetAndCheck("", "foo: bar", cmd)
				Ω(syncSetResourceIsApplied).Should(Equal(true))
			})

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

		})
	})
})

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
