package hive

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	"github.com/openshift/origin/test/extended/util/compat_otp/architecture"

	e2e "k8s.io/kubernetes/test/e2e/framework"
)

//
// Hive test case suite for Azure
//

var _ = g.Describe("[OTP][sig-hive] Cluster_Operator hive should", func() {
	defer g.GinkgoRecover()

	var (
		oc           = compat_otp.NewCLI("hive", compat_otp.KubeConfigPath())
		ns           hiveNameSpace
		og           operatorGroup
		sub          subscription
		hc           hiveconfig
		testDataDir  string
		testOCPImage string
		region       string
		basedomain   string
		cloudName    string
		isGovCloud   bool
	)
	g.BeforeEach(func() {
		// Skip ARM64 arch
		architecture.SkipNonAmd64SingleArch(oc)

		// Skip if running on a non-Azure platform
		compat_otp.SkipIfPlatformTypeNot(oc, "azure")

		// Install Hive operator if non-existent
		testDataDir = FixturePath("testdata")
		_, _ = installHiveOperator(oc, &ns, &og, &sub, &hc, testDataDir)

		// Get OCP Image for Hive testing
		testOCPImage = getTestOCPImage(oc)

		// Get platform configurations
		region = getRegion(oc)
		basedomain = getBasedomain(oc)
		isGovCloud = strings.Contains(region, "usgov")
		cloudName = AzurePublic
		if isGovCloud {
			e2e.Logf("Running on MAG")
			cloudName = AzureGov
		}
	})

	// Author: jshu@redhat.com fxie@redhat.com
	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jshu-High-25447-High-28657-High-45175-[Mag] Hive API support for Azure [Serial]", func() {
		testCaseID := "25447"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]

		compat_otp.By("Config Azure Install-Config Secret...")
		installConfigSecret := azureInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: basedomain,
			name2:      cdName,
			region:     region,
			resGroup:   AzureRESGroup,
			azureType:  cloudName,
			template:   filepath.Join(testDataDir, "azure-install-config.yaml"),
		}
		compat_otp.By("Config Azure ClusterDeployment...")
		cluster := azureClusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          basedomain,
			clusterName:         cdName,
			platformType:        "azure",
			credRef:             AzureCreds,
			region:              region,
			resGroup:            AzureRESGroup,
			azureType:           cloudName,
			imageSetRef:         cdName + "-imageset",
			installConfigSecret: cdName + "-install-config",
			pullSecretRef:       PullSecret,
			template:            filepath.Join(testDataDir, "clusterdeployment-azure.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		compat_otp.By("Create worker and infra MachinePool ...")
		workermachinepoolAzureTemp := filepath.Join(testDataDir, "machinepool-worker-azure.yaml")
		inframachinepoolAzureTemp := filepath.Join(testDataDir, "machinepool-infra-azure.yaml")
		workermp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    workermachinepoolAzureTemp,
		}
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    inframachinepoolAzureTemp,
		}

		defer cleanupObjects(oc,
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-worker"},
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"},
		)
		workermp.create(oc)
		inframp.create(oc)

		compat_otp.By("Check Azure ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, AzureClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		compat_otp.By("OCP-28657: Hive supports remote Machine Set Management for Azure")
		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err := os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"
		e2e.Logf("Check worker machinepool .status.replicas = 3")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "3", ok, 5*DefaultTimeout, []string{"MachinePool", cdName + "-worker", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		e2e.Logf("Check infra machinepool .status.replicas = 1 ")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, 5*DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		machinesetsname := getResource(oc, asAdmin, withoutNamespace, "MachinePool", cdName+"-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.machineSets[?(@.replicas==1)].name}")
		o.Expect(machinesetsname).NotTo(o.BeEmpty())
		e2e.Logf("Remote cluster machineset list: %s", machinesetsname)
		e2e.Logf("Check machineset %s created on remote cluster", machinesetsname)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, machinesetsname, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].metadata.name}"}).check(oc)
		e2e.Logf("Check only 1 machineset up")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].status.availableReplicas}"}).check(oc)
		e2e.Logf("Check only one machines in Running status")
		// Can't filter by infra label because of bug https://issues.redhat.com/browse/HIVE-1922
		//newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-o=jsonpath={.items[?(@.spec.metadata.labels.node-role\\.kubernetes\\.io==\"infra\")].status.phase}"}).check(oc)
		e2e.Logf("Patch infra machinepool .spec.replicas to 3")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"replicas": 3}}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "3", ok, 5*DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		machinesetsname = getResource(oc, asAdmin, withoutNamespace, "MachinePool", cdName+"-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.machineSets[?(@.replicas==1)].name}")
		o.Expect(machinesetsname).NotTo(o.BeEmpty())
		e2e.Logf("Remote cluster machineset list: %s", machinesetsname)
		e2e.Logf("Check machineset %s created on remote cluster", machinesetsname)
		machinesetsArray := strings.Fields(machinesetsname)
		o.Expect(len(machinesetsArray) == 3).Should(o.BeTrue())
		for _, machinesetName := range machinesetsArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, machinesetName, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].metadata.name}"}).check(oc)
		}
		e2e.Logf("Check machinesets scale up to 3")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1 1 1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].status.availableReplicas}"}).check(oc)
		e2e.Logf("Check 3 machines in Running status")
		// Can't filter by infra label because of bug https://issues.redhat.com/browse/HIVE-1922
		//newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-o=jsonpath={.items[?(@.spec.metadata.labels.node-role\\.kubernetes\\.io==\"infra\")].status.phase}"}).check(oc)
		e2e.Logf("Patch infra machinepool .spec.replicas to 2")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"replicas": 2}}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "2", ok, 5*DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		machinesetsname = getResource(oc, asAdmin, withoutNamespace, "MachinePool", cdName+"-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.machineSets[?(@.replicas==1)].name}")
		o.Expect(machinesetsname).NotTo(o.BeEmpty())
		e2e.Logf("Remote cluster machineset list: %s", machinesetsname)
		e2e.Logf("Check machineset %s created on remote cluster", machinesetsname)
		machinesetsArray = strings.Fields(machinesetsname)
		o.Expect(len(machinesetsArray) == 2).Should(o.BeTrue())
		for _, machinesetName := range machinesetsArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, machinesetName, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].metadata.name}"}).check(oc)
		}
		e2e.Logf("Check machinesets scale down to 2")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1 1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].status.availableReplicas}"}).check(oc)
		e2e.Logf("Check 2 machines in Running status")
		// Can't filter by infra label because of bug https://issues.redhat.com/browse/HIVE-1922
		//newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", "machine.openshift.io/cluster-api-machine-role=infra", "-o=jsonpath={.items[*].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-o=jsonpath={.items[?(@.spec.metadata.labels.node-role\\.kubernetes\\.io==\"infra\")].status.phase}"}).check(oc)
	})

	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jshu-Medium-33854-Hive supports Azure ClusterPool [Serial]", func() {
		testCaseID := "33854"
		poolName := "pool-" + testCaseID
		imageSetName := poolName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: testOCPImage,
			template:     imageSetTemp,
		}

		compat_otp.By("Create ClusterImageSet...")
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		imageSet.create(oc)

		compat_otp.By("Check if ClusterImageSet was created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, imageSetName, ok, DefaultTimeout, []string{"ClusterImageSet"}).check(oc)

		oc.SetupProject()
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and azure-credentials to target namespace for the cluster
		compat_otp.By("Copy Azure platform credentials...")
		createAzureCreds(oc, oc.Namespace())

		compat_otp.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		compat_otp.By("Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool-azure.yaml")
		pool := azureClusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "false",
			baseDomain:     AzureBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "azure",
			credRef:        AzureCreds,
			region:         AzureRegion,
			resGroup:       AzureRESGroup,
			pullSecretRef:  PullSecret,
			size:           1,
			maxSize:        1,
			runningCount:   0,
			maxConcurrent:  1,
			hibernateAfter: "360m",
			template:       poolTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterPool", oc.Namespace(), poolName})
		pool.create(oc)
		compat_otp.By("Check if Azure ClusterPool created successfully and become ready")
		//runningCount is 0 so pool status should be standby: 1, ready: 0
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, AzureClusterInstallTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)

		compat_otp.By("Check if CD is Hibernating")
		cdListStr := getCDlistfromPool(oc, poolName)
		var cdArray []string
		cdArray = strings.Split(strings.TrimSpace(cdListStr), "\n")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Hibernating", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i]}).check(oc)
		}

		compat_otp.By("Patch pool.spec.lables.test=test...")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"labels":{"test":"test"}}}`}).check(oc)

		compat_otp.By("The existing CD in the pool has no test label")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "test", nok, DefaultTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i], "-o=jsonpath={.metadata.labels}"}).check(oc)
		}

		compat_otp.By("The new CD in the pool should have the test label")
		e2e.Logf("Delete the old CD in the pool")
		newCheck("expect", "delete", asAdmin, withoutNamespace, contain, "delete", ok, ClusterUninstallTimeout, []string{"ClusterDeployment", cdArray[0], "-n", cdArray[0]}).check(oc)
		e2e.Logf("Get the CD list from the pool again.")
		cdListStr = getCDlistfromPool(oc, poolName)
		cdArray = strings.Split(strings.TrimSpace(cdListStr), "\n")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "test", ok, DefaultTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i], "-o=jsonpath={.metadata.labels}"}).check(oc)
		}
	})

	//author: mihuang@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "35297"|./bin/extended-platform-tests run --timeout 90m -f -
	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-Medium-35297-Hive supports cluster hibernation[Serial]", func() {
		testCaseID := "35297"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		compat_otp.By("Config Azure Install-Config Secret...")
		installConfigSecret := azureInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AzureBaseDomain,
			name2:      cdName,
			region:     AzureRegion,
			resGroup:   AzureRESGroup,
			azureType:  AzurePublic,
			template:   filepath.Join(testDataDir, "azure-install-config.yaml"),
		}
		compat_otp.By("Config Azure ClusterDeployment...")
		cluster := azureClusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          AzureBaseDomain,
			clusterName:         cdName,
			platformType:        "azure",
			credRef:             AzureCreds,
			region:              AzureRegion,
			resGroup:            AzureRESGroup,
			azureType:           AzurePublic,
			imageSetRef:         cdName + "-imageset",
			installConfigSecret: cdName + "-install-config",
			pullSecretRef:       PullSecret,
			template:            filepath.Join(testDataDir, "clusterdeployment-azure.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		compat_otp.By("Check Azure ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, AzureClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		compat_otp.By("Check CD has Hibernating condition")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Hibernating")].status}`}).check(oc)

		compat_otp.By("patch the CD to Hibernating...")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"powerState": "Hibernating"}}`}).check(oc)
		e2e.Logf("Wait for CD to be Hibernating")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Hibernating", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.powerState}"}).check(oc)
		e2e.Logf("Check cd's condition")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "True", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Hibernating")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Ready")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "True", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Unreachable")].status}`}).check(oc)

		compat_otp.By("patch the CD to Running...")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"powerState": "Running"}}`}).check(oc)
		e2e.Logf("Wait for CD to be Running")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.powerState}"}).check(oc)
		e2e.Logf("Check cd's condition")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Hibernating")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "True", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Ready")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Unreachable")].status}`}).check(oc)
	})

	//author: mihuang@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "44946"|./bin/extended-platform-tests run --timeout 90m -f -
	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:mihuang-Medium-44946-Keep it hot when HibernateAfter is setting [Serial]", func() {
		testCaseID := "44946"
		poolName := "pool-" + testCaseID
		imageSetName := poolName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: testOCPImage,
			template:     imageSetTemp,
		}

		compat_otp.By("Create ClusterImageSet...")
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		imageSet.create(oc)

		compat_otp.By("Check if ClusterImageSet was created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, imageSetName, ok, DefaultTimeout, []string{"ClusterImageSet"}).check(oc)

		oc.SetupProject()
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and azure-credentials to target namespace for the cluster
		compat_otp.By("Copy Azure platform credentials...")
		createAzureCreds(oc, oc.Namespace())

		compat_otp.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		compat_otp.By("Step1 : Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool-azure.yaml")
		pool := azureClusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "false",
			baseDomain:     AzureBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "azure",
			credRef:        AzureCreds,
			region:         AzureRegion,
			resGroup:       AzureRESGroup,
			pullSecretRef:  PullSecret,
			size:           1,
			maxSize:        1,
			runningCount:   1,
			maxConcurrent:  1,
			hibernateAfter: "5m",
			template:       poolTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterPool", oc.Namespace(), poolName})
		pool.create(oc)
		compat_otp.By("Step2 : Check if Azure ClusterPool created successfully and become ready")
		//runningCount is 1 so pool status should be ready: 1
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, AzureClusterInstallTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.ready}"}).check(oc)

		compat_otp.By("Get cd name from cdlist")
		cdListStr := getCDlistfromPool(oc, poolName)
		var cdArray []string
		cdArray = strings.Split(strings.TrimSpace(cdListStr), "\n")
		o.Expect(len(cdArray)).Should(o.BeNumerically(">=", 1))
		oldCdName := cdArray[0]

		compat_otp.By("Step4 : wait 7 mins")
		//hibernateAfter is 5 min, wait for > 5 min (Need wait hardcode timer instead of check any condition here so use Sleep function directly) to check the cluster is still running status.
		time.Sleep((HibernateAfterTimer + DefaultTimeout) * time.Second)

		compat_otp.By("Step6 and Step7 : Check unclaimed cluster is still in Running status after hibernateAfter time, because of runningcount=1")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, DefaultTimeout, []string{"ClusterDeployment", oldCdName, "-n", oldCdName}).check(oc)

		compat_otp.By("Step8 :Check installedTimestap and hibernating lastTransitionTime filed.")
		installedTimestap, err := time.Parse(time.RFC3339, getResource(oc, asAdmin, withoutNamespace, "ClusterDeployment", oldCdName, "-n", oldCdName, `-o=jsonpath={.status.installedTimestamp}`))
		o.Expect(err).NotTo(o.HaveOccurred())
		hibernateTransition, err := time.Parse(time.RFC3339, getResource(oc, asAdmin, withoutNamespace, "ClusterDeployment", oldCdName, "-n", oldCdName, `-o=jsonpath={.status.conditions[?(@.type=="Hibernating")].lastTransitionTime}`))
		o.Expect(err).NotTo(o.HaveOccurred())
		difference := hibernateTransition.Sub(installedTimestap)
		e2e.Logf("Timestamp difference is %v min", difference.Minutes())
		o.Expect(difference.Minutes()).Should(o.BeNumerically("<=", 1))

		compat_otp.By("Step4 : Create ClusterClaim...")
		claimTemp := filepath.Join(testDataDir, "clusterclaim.yaml")
		claimName := poolName + "-claim"
		claim := clusterClaim{
			name:            claimName,
			namespace:       oc.Namespace(),
			clusterPoolName: poolName,
			template:        claimTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterClaim", oc.Namespace(), claimName})
		claim.create(oc)

		claimedTimestamp, err := time.Parse(time.RFC3339, getResource(oc, asAdmin, withoutNamespace, "ClusterClaim", claimName, "-n", oc.Namespace(), "-o=jsonpath={.metadata.creationTimestamp}"))
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("get clusterclaim timestamp,creationTimestamp is %s", claimedTimestamp)

		compat_otp.By("Check if ClusterClaim created successfully and become running")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, DefaultTimeout, []string{"ClusterClaim", "-n", oc.Namespace(), claimName}).check(oc)

		compat_otp.By("Check if CD is Hibernating")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Hibernating", ok, 2*ClusterResumeTimeout, []string{"ClusterDeployment", oldCdName, "-n", oldCdName}).check(oc)

		hibernateTimestamp, err := time.Parse(time.RFC3339, getResource(oc, asAdmin, withoutNamespace, "ClusterDeployment", oldCdName, "-n", oldCdName, `-o=jsonpath={.status.conditions[?(@.type=="Hibernating")].lastProbeTime}`))
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("get hibernattimestaps, hibernateTimestamp is %s", hibernateTimestamp)

		compat_otp.By("Step5 : Get Timestamp difference")
		difference = hibernateTimestamp.Sub(claimedTimestamp)
		e2e.Logf("Timestamp difference is %v mins", difference.Minutes())
		o.Expect(difference.Minutes()).Should(o.BeNumerically(">=", 5))
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "52415"|./bin/extended-platform-tests run --timeout 90m -f -
	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:lwan-Medium-52415-[Azure]Hive Machinepool test for autoscale [Serial]", func() {
		testCaseID := "52415"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		compat_otp.By("Config Azure Install-Config Secret...")
		installConfigSecret := azureInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: AzureBaseDomain,
			name2:      cdName,
			region:     AzureRegion,
			resGroup:   AzureRESGroup,
			azureType:  AzurePublic,
			template:   filepath.Join(testDataDir, "azure-install-config.yaml"),
		}
		compat_otp.By("Config Azure ClusterDeployment...")
		cluster := azureClusterDeployment{
			fake:                "false",
			name:                cdName,
			namespace:           oc.Namespace(),
			baseDomain:          AzureBaseDomain,
			clusterName:         cdName,
			platformType:        "azure",
			credRef:             AzureCreds,
			region:              AzureRegion,
			resGroup:            AzureRESGroup,
			azureType:           AzurePublic,
			imageSetRef:         cdName + "-imageset",
			installConfigSecret: cdName + "-install-config",
			pullSecretRef:       PullSecret,
			template:            filepath.Join(testDataDir, "clusterdeployment-azure.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		compat_otp.By("Create infra MachinePool ...")
		inframachinepoolAzureTemp := filepath.Join(testDataDir, "machinepool-infra-azure.yaml")
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    inframachinepoolAzureTemp,
		}

		defer cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"})
		inframp.create(oc)

		compat_otp.By("Check if ClusterDeployment created successfully and become Provisioned")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, AzureClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err := os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"
		e2e.Logf("Patch static replicas to autoscaler")

		compat_otp.By("OCP-52415: [Azure]Allow minReplicas autoscaling of MachinePools to be 0")
		e2e.Logf("Check hive allow set minReplicas=0 without zone setting")
		autoScalingMax := "3"
		autoScalingMin := "0"
		removeConfig := "[{\"op\": \"remove\", \"path\": \"/spec/replicas\"}]"
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "json", "-p", removeConfig}).check(oc)
		autoscalConfig := fmt.Sprintf("{\"spec\": {\"autoscaling\": {\"maxReplicas\": %s, \"minReplicas\": %s}}}", autoScalingMax, autoScalingMin)
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", autoscalConfig}).check(oc)
		e2e.Logf("Check replicas is 0")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0", ok, 2*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
		e2e.Logf("Check hive allow set minReplicas=0 within zone setting")
		cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"})
		infra2MachinepoolYaml := `
apiVersion: hive.openshift.io/v1
kind: MachinePool
metadata:
  name: ` + cdName + `-infra2
  namespace: ` + oc.Namespace() + `
spec:
  autoscaling:
    maxReplicas: 3
    minReplicas: 0
  clusterDeploymentRef:
    name: ` + cdName + `
  labels:
    node-role.kubernetes.io: infra2
    node-role.kubernetes.io/infra2: ""
  name: infra2
  platform:
    azure:
      osDisk:
        diskSizeGB: 128
      type: Standard_D4s_v3
      zones:
      - "1"
      - "2"
      - "3"`
		var filename = testCaseID + "-machinepool-infra2.yaml"
		err = ioutil.WriteFile(filename, []byte(infra2MachinepoolYaml), 0644)
		defer os.Remove(filename)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", filename, "--ignore-not-found").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", filename).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check replicas is 0")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0", ok, 2*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)

		compat_otp.By("Check Hive supports autoscale for Azure")
		patchYaml := `
spec:
  scaleDown:
    enabled: true
    delayAfterAdd: 10s
    delayAfterDelete: 10s
    delayAfterFailure: 10s
    unneededTime: 10s`
		e2e.Logf("Add busybox in remote cluster and check machines will scale up to maxReplicas")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ClusterAutoscaler", "default", "--type", "merge", "-p", patchYaml}).check(oc)
		workloadYaml := filepath.Join(testDataDir, "workload.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml, "--ignore-not-found").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "busybox", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Deployment", "busybox", "-n", "default"}).check(oc)
		e2e.Logf("Check replicas will scale up to maximum value")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1 1 1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
		e2e.Logf("Delete busybox in remote cluster and check machines will scale down to minReplicas")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check replicas will scale down to minimum value")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0", ok, 10*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
	})

	//author: mihuang@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "54048"|./bin/extended-platform-tests run --timeout 10m -f -
	g.It("[Level0] NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:mihuang-Medium-54048-[Mag] Hive to support cli-domain-from-installer-image annotation [Serial]", func() {
		testCaseID := "54048"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		imageSetName := cdName + "-imageset"
		imageSetTemp := filepath.Join(testDataDir, "clusterimageset.yaml")
		imageSet := clusterImageSet{
			name:         imageSetName,
			releaseImage: testOCPImage,
			template:     imageSetTemp,
		}

		compat_otp.By("Create ClusterImageSet...")
		defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", imageSetName})
		imageSet.create(oc)

		compat_otp.By("Check if ClusterImageSet was created successfully")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, imageSetName, ok, DefaultTimeout, []string{"ClusterImageSet"}).check(oc)

		//secrets can be accessed by pod in the same namespace, so copy pull-secret and azure-credentials to target namespace for the cluster
		compat_otp.By("Copy Azure platform credentials...")
		createAzureCreds(oc, oc.Namespace())

		compat_otp.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		compat_otp.By("Create Install-Config Secret...")
		installConfigTemp := filepath.Join(testDataDir, "azure-install-config.yaml")
		installConfigSecretName := cdName + "-install-config"
		installConfigSecret := azureInstallConfig{
			name1:      installConfigSecretName,
			namespace:  oc.Namespace(),
			baseDomain: basedomain,
			name2:      cdName,
			region:     region,
			resGroup:   AzureRESGroup,
			azureType:  cloudName,
			template:   installConfigTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		compat_otp.By("Get domain from installerimage...")
		clusterVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-o=jsonpath={.status.desired.version}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterVersion).NotTo(o.BeEmpty())
		originalInstallerImage, err := getPullSpec(oc, "installer", clusterVersion)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(originalInstallerImage).NotTo(o.BeEmpty())
		e2e.Logf("ClusterVersion is %s, originalInstallerImage is %s", clusterVersion, originalInstallerImage)
		installerDomain := strings.SplitN(originalInstallerImage, "/", 2)[0]
		installerPath := strings.SplitN(originalInstallerImage, "/", 2)[1]
		overrideDomain := "mihuang.io"
		overrideInstallerImage := overrideDomain + "/" + installerPath

		type imageOverride struct {
			copyCliDomain          string
			installerImageOverride string
		}
		var scenarios = []imageOverride{
			{
				"true",
				overrideInstallerImage,
			},
			{
				"false",
				overrideInstallerImage,
			},
			{
				"true",
				"",
			},
		}
		for i := 0; i < len(scenarios); i++ {
			func() {
				//create cluster
				if scenarios[i].copyCliDomain == "true" && scenarios[i].installerImageOverride == overrideInstallerImage {
					compat_otp.By("Config Azure ClusterDeployment,with hive.openshift.io/cli-domain-from-installer-image=true and installerImageOverride set...")
				} else if scenarios[i].copyCliDomain == "false" && scenarios[i].installerImageOverride == overrideInstallerImage {
					compat_otp.By("Config Azure ClusterDeployment,with hive.openshift.io/cli-domain-from-installer-image=false and installerImageOverride set...")
				} else if scenarios[i].copyCliDomain == "true" && scenarios[i].installerImageOverride == "" {
					compat_otp.By("Config Azure ClusterDeployment,with hive.openshift.io/cli-domain-from-installer-image=true and installerImageOverride unset...")
				}
				cluster := azureClusterDeployment{
					fake:                   "false",
					copyCliDomain:          scenarios[i].copyCliDomain,
					name:                   cdName,
					namespace:              oc.Namespace(),
					baseDomain:             basedomain,
					clusterName:            cdName,
					platformType:           "azure",
					credRef:                AzureCreds,
					region:                 region,
					resGroup:               AzureRESGroup,
					azureType:              cloudName,
					imageSetRef:            cdName + "-imageset",
					installConfigSecret:    cdName + "-install-config",
					installerImageOverride: scenarios[i].installerImageOverride,
					pullSecretRef:          PullSecret,
					template:               filepath.Join(testDataDir, "clusterdeployment-azure.yaml"),
				}
				defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
				cluster.create(oc)

				//get conditions
				compat_otp.By("Check if provisioning pod create success")
				newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Provisioning", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace()}).check(oc)

				e2e.Logf("Check cd .status.installerImage")
				installerImage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.installerImage}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				overrideImageDomain := strings.SplitN(installerImage, "/", 2)[0]
				e2e.Logf("overrideImageDomain is %s", overrideImageDomain)

				e2e.Logf("Check cd .status.cliImage")
				cliImage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.cliImage}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				cliImageDomain := strings.SplitN(cliImage, "/", 2)[0]
				e2e.Logf("cliImageDomain is %s", cliImageDomain)
				//check conditions for scenarios
				if scenarios[i].copyCliDomain == "true" && scenarios[i].installerImageOverride == overrideInstallerImage {
					compat_otp.By("Check if both cliImage and installerImage use the new domain")
					o.Expect(overrideImageDomain).To(o.Equal(overrideDomain))
					o.Expect(cliImageDomain).To(o.Equal(overrideDomain))
				} else if scenarios[i].copyCliDomain == "false" && scenarios[i].installerImageOverride == overrideInstallerImage {
					compat_otp.By("Check if cliImage use offical domain and installerImage use the new domain")
					o.Expect(overrideImageDomain).To(o.Equal(overrideDomain))
					o.Expect(cliImageDomain).To(o.Equal(installerDomain))
				} else {
					compat_otp.By("Check if both cliImage and installerImage use the offical image")
					o.Expect(overrideImageDomain).To(o.Equal(installerDomain))
					o.Expect(cliImageDomain).To(o.Equal(installerDomain))
				}
			}()
		}
	})
})
