package hive

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/api/iterator"

	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	"github.com/openshift/origin/test/extended/util/compat_otp/architecture"

	e2e "k8s.io/kubernetes/test/e2e/framework"
)

//
// Hive test case suite for GCP
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
	)
	g.BeforeEach(func() {
		// Skip ARM64 arch
		architecture.SkipNonAmd64SingleArch(oc)

		// Skip if running on a non-GCP platform
		compat_otp.SkipIfPlatformTypeNot(oc, "gcp")

		// Install Hive operator if not
		testDataDir = FixturePath("testdata")
		_, _ = installHiveOperator(oc, &ns, &og, &sub, &hc, testDataDir)

		// Get OCP Image for Hive testing
		testOCPImage = getTestOCPImage(oc)

		// Get platform configurations
		region = getRegion(oc)
		basedomain = getBasedomain(oc)
	})

	// Author: fxie@redhat.com
	// Timeout: 60min
	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:fxie-Critical-68240-Enable UEFISecureBoot for day 2 VMs on GCP [Serial]", func() {
		var (
			testCaseID   = "68240"
			cdName       = "cd-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
			cdTemplate   = filepath.Join(testDataDir, "clusterdeployment-gcp.yaml")
			icName       = cdName + "-install-config"
			icTemplate   = filepath.Join(testDataDir, "gcp-install-config.yaml")
			imageSetName = cdName + "-imageset"
			mpTemplate   = filepath.Join(testDataDir, "machinepool-infra-gcp.yaml")
		)

		var (
			// Count the number of VMs in a project, after filtering with the passed-in filter
			countVMs = func(client *compute.InstancesClient, projectID, filter string) (vmCount int) {
				instancesIterator := client.AggregatedList(context.Background(), &computepb.AggregatedListInstancesRequest{
					Filter:  &filter,
					Project: projectID,
				})
				for {
					resp, err := instancesIterator.Next()
					if err == iterator.Done {
						break
					}
					o.Expect(err).NotTo(o.HaveOccurred())
					vmCount += len(resp.Value.Instances)
				}
				e2e.Logf("Found VM count = %v", vmCount)
				return vmCount
			}
		)

		compat_otp.By("Getting project ID from the Hive cd")
		projectID, err := compat_otp.GetGcpProjectID(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		e2e.Logf("Found project ID = %v", projectID)

		compat_otp.By("Creating a spoke cluster with shielded VM enabled")
		installConfigSecret := gcpInstallConfig{
			name1:      icName,
			namespace:  oc.Namespace(),
			baseDomain: basedomain,
			name2:      cdName,
			region:     region,
			projectid:  projectID,
			template:   icTemplate,
			secureBoot: "Enabled",
		}
		cd := gcpClusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           basedomain,
			clusterName:          cdName,
			platformType:         "gcp",
			credRef:              GCPCreds,
			region:               region,
			imageSetRef:          imageSetName,
			installConfigSecret:  icName,
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 1,
			template:             cdTemplate,
		}
		defer cleanCD(oc, imageSetName, oc.Namespace(), icName, cdName)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cd)

		compat_otp.By("Waiting for the CD to be installed")
		newCheck("expect", "get", asAdmin, requireNS, compare, "true", ok,
			ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-o=jsonpath={.spec.installed}"}).check(oc)

		// The Google cloud SDK must be able to locate Application Default Credentials (ADC).
		// To this end, we should point the GOOGLE_APPLICATION_CREDENTIALS environment
		// variable to a Google cloud credential file.
		instancesClient, err := compute.NewInstancesRESTClient(context.Background())
		o.Expect(err).NotTo(o.HaveOccurred())
		filter := fmt.Sprintf("(name=%s*) AND (shieldedInstanceConfig.enableSecureBoot = true)", cdName)
		o.Expect(countVMs(instancesClient, projectID, filter)).To(o.Equal(6))

		compat_otp.By("Create an infra MachinePool with secureboot enabled")
		inframp := machinepool{
			namespace:     oc.Namespace(),
			clusterName:   cdName,
			template:      mpTemplate,
			gcpSecureBoot: "Enabled",
		}
		// The inframp will be deprovisioned along with the CD, so no need to defer a deletion here.
		inframp.create(oc)

		compat_otp.By("Make sure all infraVMs have secureboot enabled")
		infraId := getInfraIDFromCDName(oc, cdName)
		filterInfra := fmt.Sprintf("(name=%s*) AND (shieldedInstanceConfig.enableSecureBoot = true)", infraId+"-infra")
		o.Eventually(func() bool {
			return countVMs(instancesClient, projectID, filterInfra) == 1
		}).WithTimeout(15 * time.Minute).WithPolling(30 * time.Second).Should(o.BeTrue())
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "41777"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:lwan-High-41777-High-28636-Hive API support for GCP[Serial]", func() {
		testCaseID := "41777"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		compat_otp.By("Config GCP Install-Config Secret...")
		projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		installConfigSecret := gcpInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: GCPBaseDomain,
			name2:      cdName,
			region:     GCPRegion,
			projectid:  projectID,
			template:   filepath.Join(testDataDir, "gcp-install-config.yaml"),
		}
		compat_otp.By("Config GCP ClusterDeployment...")
		cluster := gcpClusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           GCPBaseDomain,
			clusterName:          cdName,
			platformType:         "gcp",
			credRef:              GCPCreds,
			region:               GCPRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 3,
			template:             filepath.Join(testDataDir, "clusterdeployment-gcp.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		compat_otp.By("Create worker and infra MachinePool ...")
		workermachinepoolGCPTemp := filepath.Join(testDataDir, "machinepool-worker-gcp.yaml")
		inframachinepoolGCPTemp := filepath.Join(testDataDir, "machinepool-infra-gcp.yaml")
		workermp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    workermachinepoolGCPTemp,
		}
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    inframachinepoolGCPTemp,
		}

		defer cleanupObjects(oc,
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-worker"},
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"},
		)
		workermp.create(oc)
		inframp.create(oc)

		compat_otp.By("Check GCP ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		compat_otp.By("OCP-28636: Hive supports remote Machine Set Management for GCP")
		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err = os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"
		e2e.Logf("Check worker machinepool .status.replicas = 3")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "3", ok, DefaultTimeout, []string{"MachinePool", cdName + "-worker", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		e2e.Logf("Check infra machinepool .status.replicas = 1 ")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
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

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "33872"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:lwan-Medium-33872-[gcp]Hive supports ClusterPool [Serial]", func() {
		testCaseID := "33872"
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
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and gcp-credentials to target namespace for the pool
		compat_otp.By("Copy GCP platform credentials...")
		createGCPCreds(oc, oc.Namespace())

		compat_otp.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		compat_otp.By("Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool-gcp.yaml")
		pool := gcpClusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "false",
			baseDomain:     GCPBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "gcp",
			credRef:        GCPCreds,
			region:         GCPRegion,
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
		compat_otp.By("Check if GCP ClusterPool created successfully and become ready")
		//runningCount is 0 so pool status should be standby: 1, ready: 0
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, ClusterInstallTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)

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

	//author: liangli@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "44475"|./bin/extended-platform-tests run --timeout 90m -f -
	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:liangli-Medium-44475-Medium-45158-[gcp]Hive Change BaseDomain field right after creating pool and all clusters finish install firstly then recreated [Serial]", func() {
		testCaseID := "44475"
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
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and gcp-credentials to target namespace for the clusterdeployment
		compat_otp.By("Copy GCP platform credentials...")
		createGCPCreds(oc, oc.Namespace())

		compat_otp.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		compat_otp.By("Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool-gcp.yaml")
		pool := gcpClusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "false",
			baseDomain:     GCPBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "gcp",
			credRef:        GCPCreds,
			region:         GCPRegion,
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
		e2e.Logf("Check ClusterDeployment in pool created")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, poolName, ok, DefaultTimeout, []string{"ClusterDeployment", "-A", "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		e2e.Logf("get old ClusterDeployment Name")
		cdListStr := getCDlistfromPool(oc, poolName)
		oldClusterDeploymentName := strings.Split(strings.TrimSpace(cdListStr), "\n")
		o.Expect(len(oldClusterDeploymentName) > 0).Should(o.BeTrue())
		e2e.Logf("old cd name: %s", oldClusterDeploymentName[0])

		compat_otp.By("OCP-45158: Check Provisioned condition")
		e2e.Logf("Check ClusterDeployment is provisioning")
		expectedResult := "message:Cluster is provisioning,reason:Provisioning,status:False"
		jsonPath := "-o=jsonpath={\"message:\"}{.status.conditions[?(@.type==\"Provisioned\")].message}{\",reason:\"}{.status.conditions[?(@.type==\"Provisioned\")].reason}{\",status:\"}{.status.conditions[?(@.type==\"Provisioned\")].status}"
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, expectedResult, ok, DefaultTimeout, []string{"ClusterDeployment", oldClusterDeploymentName[0], "-n", oldClusterDeploymentName[0], jsonPath}).check(oc)
		e2e.Logf("Check ClusterDeployment Provisioned finish")
		expectedResult = "message:Cluster is provisioned,reason:Provisioned,status:True"
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, expectedResult, ok, ClusterInstallTimeout, []string{"ClusterDeployment", oldClusterDeploymentName[0], "-n", oldClusterDeploymentName[0], jsonPath}).check(oc)

		compat_otp.By("Check if GCP ClusterPool created successfully and become ready")
		//runningCount is 0 so pool status should be standby: 1, ready: 0
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.standby}"}).check(oc)

		compat_otp.By("test OCP-44475")
		e2e.Logf("oc patch ClusterPool 'spec.baseDomain'")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("ClusterPool", poolName, "-n", oc.Namespace(), "-p", `{"spec":{"baseDomain":"`+GCPBaseDomain2+`"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check ClusterDeployment is Deprovisioning")
		expectedResult = "message:Cluster is deprovisioning,reason:Deprovisioning,status:False"
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, expectedResult, ok, DefaultTimeout, []string{"ClusterDeployment", oldClusterDeploymentName[0], "-n", oldClusterDeploymentName[0], jsonPath}).check(oc)
		e2e.Logf("Check ClusterDeployment is Deprovisioned")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, oldClusterDeploymentName[0], nok, ClusterUninstallTimeout, []string{"ClusterDeployment", "-A", "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		e2e.Logf("Check if ClusterPool re-create the CD")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, poolName, ok, DefaultTimeout, []string{"ClusterDeployment", "-A", "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		e2e.Logf("get new ClusterDeployment name")
		cdListStr = getCDlistfromPool(oc, poolName)
		newClusterDeploymentName := strings.Split(strings.TrimSpace(cdListStr), "\n")
		o.Expect(len(newClusterDeploymentName) > 0).Should(o.BeTrue())
		e2e.Logf("new cd name: %s", newClusterDeploymentName[0])

		newCheck("expect", "get", asAdmin, withoutNamespace, contain, GCPBaseDomain2, ok, DefaultTimeout, []string{"ClusterDeployment", newClusterDeploymentName[0], "-n", newClusterDeploymentName[0], "-o=jsonpath={.spec.baseDomain}"}).check(oc)
		o.Expect(strings.Compare(oldClusterDeploymentName[0], newClusterDeploymentName[0]) != 0).Should(o.BeTrue())
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "41499"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:lwan-High-41499-High-34404-High-25333-Hive syncset test for paused and multi-modes[Serial]", func() {
		testCaseID := "41499"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		compat_otp.By("Config GCP Install-Config Secret...")
		projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		installConfigSecret := gcpInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: GCPBaseDomain,
			name2:      cdName,
			region:     GCPRegion,
			projectid:  projectID,
			template:   filepath.Join(testDataDir, "gcp-install-config.yaml"),
		}
		compat_otp.By("Config GCP ClusterDeployment...")
		cluster := gcpClusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           GCPBaseDomain,
			clusterName:          cdName,
			platformType:         "gcp",
			credRef:              GCPCreds,
			region:               GCPRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 3,
			template:             filepath.Join(testDataDir, "clusterdeployment-gcp.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		compat_otp.By("Check GCP ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err = os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"

		compat_otp.By("OCP-41499: Add condition in ClusterDeployment status for paused syncset")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace()}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "False", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"SyncSetFailed\")].status}"}).check(oc)
		e2e.Logf("Add \"hive.openshift.io/syncset-pause\" annotation in ClusterDeployment, and delete ClusterSync CR")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"metadata": {"annotations": {"hive.openshift.io/syncset-pause": "true"}}}`}).check(oc)
		newCheck("expect", "delete", asAdmin, withoutNamespace, contain, "delete", ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace()}).check(oc)
		e2e.Logf("Check ClusterDeployment condition=SyncSetFailed")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "True", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"SyncSetFailed\")].status}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "SyncSetPaused", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"SyncSetFailed\")].reason}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "SyncSet is paused. ClusterSync will not be created", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"SyncSetFailed\")].message}"}).check(oc)
		e2e.Logf("Check ClusterSync won't be created.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, nok, DefaultTimeout, []string{"ClusterSync", "-n", oc.Namespace()}).check(oc)
		e2e.Logf("Remove annotation, check ClusterSync will be created again.")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"metadata": {"annotations": {"hive.openshift.io/syncset-pause": "false"}}}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "False", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"SyncSetFailed\")].status}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, cdName, ok, DefaultTimeout, []string{"ClusterSync", cdName, "-n", oc.Namespace()}).check(oc)

		compat_otp.By("OCP-34404: Hive adds muti-modes for syncset to handle applying resources too large")
		e2e.Logf("Create SyncSet with default applyBehavior.")
		syncSetName := testCaseID + "-syncset-1"
		configMapName := testCaseID + "-configmap-1"
		configMapNamespace := testCaseID + "-" + getRandomString() + "-hive-1"
		resourceMode := "Sync"
		syncTemp := filepath.Join(testDataDir, "syncset-resource.yaml")
		syncResource := syncSetResource{
			name:        syncSetName,
			namespace:   oc.Namespace(),
			namespace2:  configMapNamespace,
			cdrefname:   cdName,
			cmname:      configMapName,
			cmnamespace: configMapNamespace,
			ramode:      resourceMode,
			template:    syncTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetName})
		syncResource.create(oc)
		e2e.Logf("Check ConfigMap is created on target cluster and have a last-applied-config annotation.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo":"bar"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName, "-n", configMapNamespace, "-o=jsonpath={.data}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "kubectl.kubernetes.io/last-applied-configuration", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName, "-n", configMapNamespace, "-o=jsonpath={.metadata.annotations}"}).check(oc)
		e2e.Logf("Patch syncset resource.")
		patchYaml := `
spec:
  resources:
  - apiVersion: v1
    kind: Namespace
    metadata:
      name: ` + configMapNamespace + `
  - apiVersion: v1
    data:
      foo1: bar1
    kind: ConfigMap
    metadata:
      name: ` + configMapName + `
      namespace: ` + configMapNamespace
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetName, "-n", oc.Namespace(), "--type", "merge", "-p", patchYaml}).check(oc)
		e2e.Logf("Check data field in ConfigMap on target cluster should update.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo1":"bar1"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName, "-n", configMapNamespace, "-o=jsonpath={.data}"}).check(oc)

		e2e.Logf("Create SyncSet with applyBehavior=CreateOnly.")
		syncSetName2 := testCaseID + "-syncset-2"
		configMapName2 := testCaseID + "-configmap-2"
		configMapNamespace2 := testCaseID + "-" + getRandomString() + "-hive-2"
		applyBehavior := "CreateOnly"
		syncTemp2 := filepath.Join(testDataDir, "syncset-resource.yaml")
		syncResource2 := syncSetResource{
			name:          syncSetName2,
			namespace:     oc.Namespace(),
			namespace2:    configMapNamespace2,
			cdrefname:     cdName,
			cmname:        configMapName2,
			cmnamespace:   configMapNamespace2,
			ramode:        resourceMode,
			applybehavior: applyBehavior,
			template:      syncTemp2,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetName2})
		syncResource2.create(oc)
		e2e.Logf("Check ConfigMap is created on target cluster and should not have the last-applied-config annotation.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo":"bar"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName2, "-n", configMapNamespace2, "-o=jsonpath={.data}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "kubectl.kubernetes.io/last-applied-configuration", nok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName2, "-n", configMapNamespace2, "-o=jsonpath={.metadata.annotations}"}).check(oc)
		e2e.Logf("Patch syncset resource.")
		patchYaml = `
spec:
  resources:
  - apiVersion: v1
    kind: Namespace
    metadata:
      name: ` + configMapNamespace2 + `
  - apiVersion: v1
    data:
      foo1: bar1
    kind: ConfigMap
    metadata:
      name: ` + configMapName2 + `
      namespace: ` + configMapNamespace2
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetName2, "-n", oc.Namespace(), "--type", "merge", "-p", patchYaml}).check(oc)
		e2e.Logf("Check data field in ConfigMap on target cluster should not update.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo":"bar"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName2, "-n", configMapNamespace2, "-o=jsonpath={.data}"}).check(oc)

		e2e.Logf("Create SyncSet with applyBehavior=CreateOrUpdate.")
		syncSetName3 := testCaseID + "-syncset-3"
		configMapName3 := testCaseID + "-configmap-3"
		configMapNamespace3 := testCaseID + "-" + getRandomString() + "-hive-3"
		applyBehavior = "CreateOrUpdate"
		syncTemp3 := filepath.Join(testDataDir, "syncset-resource.yaml")
		syncResource3 := syncSetResource{
			name:          syncSetName3,
			namespace:     oc.Namespace(),
			namespace2:    configMapNamespace3,
			cdrefname:     cdName,
			cmname:        configMapName3,
			cmnamespace:   configMapNamespace3,
			ramode:        resourceMode,
			applybehavior: applyBehavior,
			template:      syncTemp3,
		}
		defer cleanupObjects(oc, objectTableRef{"SyncSet", oc.Namespace(), syncSetName3})
		syncResource3.create(oc)
		e2e.Logf("Check ConfigMap is created on target cluster and should not have the last-applied-config annotation.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo":"bar"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName3, "-n", configMapNamespace3, "-o=jsonpath={.data}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "kubectl.kubernetes.io/last-applied-configuration", nok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName3, "-n", configMapNamespace3, "-o=jsonpath={.metadata.annotations}"}).check(oc)
		e2e.Logf("Patch syncset resource.")
		patchYaml = `
spec:
  resources:
  - apiVersion: v1
    kind: Namespace
    metadata:
      name: ` + configMapNamespace3 + `
  - apiVersion: v1
    data:
      foo2: bar2
    kind: ConfigMap
    metadata:
      name: ` + configMapName3 + `
      namespace: ` + configMapNamespace3
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetName3, "-n", oc.Namespace(), "--type", "merge", "-p", patchYaml}).check(oc)
		e2e.Logf("Check data field in ConfigMap on target cluster should update and contain both foo and foo2.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo":"bar","foo2":"bar2"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName3, "-n", configMapNamespace3, "-o=jsonpath={.data}"}).check(oc)
		e2e.Logf("Patch syncset resource.")
		patchYaml = `
spec:
  resources:
  - apiVersion: v1
    kind: Namespace
    metadata:
      name: ` + configMapNamespace3 + `
  - apiVersion: v1
    data:
      foo: bar-test
      foo3: bar3
    kind: ConfigMap
    metadata:
      name: ` + configMapName3 + `
      namespace: ` + configMapNamespace3
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"SyncSet", syncSetName3, "-n", oc.Namespace(), "--type", "merge", "-p", patchYaml}).check(oc)
		e2e.Logf("Check data field in ConfigMap on target cluster should update, patch foo and add foo3.")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, `{"foo":"bar-test","foo2":"bar2","foo3":"bar3"}`, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "ConfigMap", configMapName3, "-n", configMapNamespace3, "-o=jsonpath={.data}"}).check(oc)

		compat_otp.By("OCP-25333: Changing apiGroup for ClusterRoleBinding in SyncSet doesn't delete the CRB")
		e2e.Logf("Create SyncSet with invalid apiGroup in resource CR.")
		syncSetName4 := testCaseID + "-syncset-4"
		syncsetYaml := `
apiVersion: hive.openshift.io/v1
kind: SyncSet
metadata:
  name: ` + syncSetName4 + `
spec:
  clusterDeploymentRefs:
  - name: ` + cdName + `
  - namespace: ` + oc.Namespace() + `
  resourceApplyMode: Sync
  resources:
  - apiVersion: authorization.openshift.io/v1
    kind: ClusterRoleBinding
    metadata:
      name: dedicated-admins-cluster
    subjects:
    - kind: Group
      name: dedicated-admins
    - kind: Group
      name: system:serviceaccounts:dedicated-admin
    roleRef:
      name: dedicated-admins-cluster`
		var filename = testCaseID + "-syncset-crb.yaml"
		err = ioutil.WriteFile(filename, []byte(syncsetYaml), 0644)
		defer os.Remove(filename)
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", filename, "-n", oc.Namespace()).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`Invalid value: "authorization.openshift.io/v1": must use kubernetes group for this resource kind`))
		e2e.Logf("oc create syncset failed, this is expected.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, syncSetName4, nok, DefaultTimeout, []string{"SyncSet", "-n", oc.Namespace()}).check(oc)
	})

	//author: mihuang@redhat.com
	//The case OCP-78499 is supported starting from version 4.19.
	g.It("[Level0] Author:mihuang-NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Medium-35069-High-78499-High-87168-Hive supports cluster hibernation for gcp[Serial]", func() {
		testCaseID := "35069"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		compat_otp.By("Config GCP Install-Config Secret...")
		projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		installConfigSecret := gcpInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: GCPBaseDomain,
			name2:      cdName,
			region:     GCPRegion,
			projectid:  projectID,
			template:   filepath.Join(testDataDir, "gcp-install-config.yaml"),
		}
		compat_otp.By("Config GCP ClusterDeployment...")
		cluster := gcpClusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           GCPBaseDomain,
			clusterName:          cdName,
			platformType:         "gcp",
			credRef:              GCPCreds,
			region:               GCPRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 3,
			template:             filepath.Join(testDataDir, "clusterdeployment-gcp.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		compat_otp.By("OCP-87168: Create infra MachinePool ...")
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    filepath.Join(testDataDir, "machinepool-infra-gcp.yaml"),
		}
		defer cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"})
		inframp.create(oc)

		compat_otp.By("Check GCP ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
		compat_otp.By("OCP-78499: Verify whether the discardLocalSsdOnHibernate field exists")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "false", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.platform.gcp.discardLocalSsdOnHibernate}"}).check(oc)
		compat_otp.By("Check CD has Hibernating condition")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Hibernating")].status}`}).check(oc)
		compat_otp.By("patch the CD to Hibernating...")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"powerState": "Hibernating"}}`}).check(oc)
		e2e.Logf("OCP-78499: Wait until the CD successfully reaches the Hibernating state.")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Hibernating", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.powerState}"}).check(oc)
		e2e.Logf("Check cd's condition")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "True", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Hibernating")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Ready")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "True", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Unreachable")].status}`}).check(oc)

		compat_otp.By("patch the CD to Running...")
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "--type", "merge", "-p", `{"spec":{"powerState": "Running"}}`}).check(oc)
		e2e.Logf("Wait for CD to be Running")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.powerState}"}).check(oc)
		e2e.Logf("Check cd's condition")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Hibernating")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "True", ok, ClusterResumeTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Ready")].status}`}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "False", ok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), `-o=jsonpath={.status.conditions[?(@.type=="Unreachable")].status}`}).check(oc)

		compat_otp.By("OCP-87168: Login to spoke cluster and check the network tags")
		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err = os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"

		e2e.Logf("Check infra machinepool .status.replicas = 1 ")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)
		machinesetsname := getResource(oc, asAdmin, withoutNamespace, "MachinePool", cdName+"-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.machineSets[?(@.replicas==1)].name}")
		o.Expect(machinesetsname).NotTo(o.BeEmpty())
		e2e.Logf("OCP-87168: Check machineset %s created on remote cluster", machinesetsname)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, machinesetsname, ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].metadata.name}"}).check(oc)
		e2e.Logf("Check only 1 machineset up")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[?(@.spec.replicas==1)].status.availableReplicas}"}).check(oc)
		e2e.Logf("Check only one machines in Running status")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "Machine", "-n", "openshift-machine-api", "-o=jsonpath={.items[?(@.spec.metadata.labels.node-role\\.kubernetes\\.io==\"infra\")].status.phase}"}).check(oc)
		e2e.Logf("OCP-87168: Check network tags in MachineSet")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-qe-gcp-tag1", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", machinesetsname, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.tags}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-qe-gcp-tag2", ok, DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", machinesetsname, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.tags}"}).check(oc)
		e2e.Logf("OCP-87168: Verify GCP VM Instances Have Network Tags")
		machineName := getResource(oc, asAdmin, withoutNamespace, "--kubeconfig="+kubeconfig, "Machine", "-n", "openshift-machine-api", "-l", fmt.Sprintf("machine.openshift.io/cluster-api-machineset=%s", machinesetsname), "-o=jsonpath={.items[0].metadata.name}")
		o.Expect(machineName).NotTo(o.BeEmpty())
		e2e.Logf("OCP-87168: Found infra machine name: %s", machineName)
		providerID := getResource(oc, asAdmin, withoutNamespace, "Machine", machineName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.providerID}", "--kubeconfig="+kubeconfig)
		o.Expect(providerID).NotTo(o.BeEmpty())
		e2e.Logf("OCP-87168: Machine providerID: %s", providerID)
		instanceName := machineName
		if strings.Contains(providerID, "/") {
			parts := strings.Split(providerID, "/")
			if len(parts) > 0 {
				instanceName = parts[len(parts)-1]
			}
		}
		e2e.Logf("OCP-87168: Looking for GCP VM instance: %s", instanceName)
		instancesClient, err := compute.NewInstancesRESTClient(context.Background())
		o.Expect(err).NotTo(o.HaveOccurred())
		defer instancesClient.Close()

		filterInstance := fmt.Sprintf("name=%s", instanceName)
		instancesIterator := instancesClient.AggregatedList(context.Background(), &computepb.AggregatedListInstancesRequest{
			Filter: &filterInstance, Project: projectID,
		})
		var foundInstance *computepb.Instance
		for resp, err := instancesIterator.Next(); err != iterator.Done; resp, err = instancesIterator.Next() {
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, instance := range resp.Value.Instances {
				if instance.GetName() == instanceName {
					foundInstance = instance
					break
				}
			}
			if foundInstance != nil {
				break
			}
		}

		o.Expect(foundInstance).NotTo(o.BeNil(), "Expected to find GCP VM instance with name: %s", instanceName)
		e2e.Logf("OCP-87168: Found GCP VM instance: %s", foundInstance.GetName())

		instanceTags := foundInstance.GetTags().GetItems()
		e2e.Logf("OCP-87168: VM instance tags: %v", instanceTags)
		o.Expect(instanceTags).To(o.ContainElement("hive-qe-gcp-tag1"), "Expected to find tag 'hive-qe-gcp-tag1' in GCP VM instance %s", instanceName)
		o.Expect(instanceTags).To(o.ContainElement("hive-qe-gcp-tag2"), "Expected to find tag 'hive-qe-gcp-tag2' in GCP VM instance %s", instanceName)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "52411"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:lwan-Medium-52411-[GCP]Hive Machinepool test for autoscale [Serial]", func() {
		testCaseID := "52411"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		oc.SetupProject()

		compat_otp.By("Config GCP Install-Config Secret...")
		projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		installConfigSecret := gcpInstallConfig{
			name1:      cdName + "-install-config",
			namespace:  oc.Namespace(),
			baseDomain: GCPBaseDomain,
			name2:      cdName,
			region:     GCPRegion,
			projectid:  projectID,
			template:   filepath.Join(testDataDir, "gcp-install-config.yaml"),
		}
		compat_otp.By("Config GCP ClusterDeployment...")
		cluster := gcpClusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           GCPBaseDomain,
			clusterName:          cdName,
			platformType:         "gcp",
			credRef:              GCPCreds,
			region:               GCPRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 3,
			template:             filepath.Join(testDataDir, "clusterdeployment-gcp.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		compat_otp.By("Create infra MachinePool ...")
		inframachinepoolGCPTemp := filepath.Join(testDataDir, "machinepool-infra-gcp.yaml")
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    inframachinepoolGCPTemp,
		}

		defer cleanupObjects(oc, objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"})
		inframp.create(oc)

		compat_otp.By("Check if ClusterDeployment created successfully and become Provisioned")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)

		tmpDir := "/tmp/" + cdName + "-" + getRandomString()
		err = os.MkdirAll(tmpDir, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(tmpDir)
		getClusterKubeconfig(oc, cdName, oc.Namespace(), tmpDir)
		kubeconfig := tmpDir + "/kubeconfig"
		e2e.Logf("Patch static replicas to autoscaler")

		compat_otp.By("OCP-52411: [GCP]Allow minReplicas autoscaling of MachinePools to be 0")
		e2e.Logf("Check hive allow set minReplicas=0 without zone setting")
		autoScalingMax := "4"
		autoScalingMin := "0"
		removeConfig := "[{\"op\": \"remove\", \"path\": \"/spec/replicas\"}]"
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "json", "-p", removeConfig}).check(oc)
		autoscalConfig := fmt.Sprintf("{\"spec\": {\"autoscaling\": {\"maxReplicas\": %s, \"minReplicas\": %s}}}", autoScalingMax, autoScalingMin)
		newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "--type", "merge", "-p", autoscalConfig}).check(oc)
		e2e.Logf("Check replicas is 0")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0 0", ok, 2*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
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
    maxReplicas: 4
    minReplicas: 0
  clusterDeploymentRef:
    name: ` + cdName + `
  labels:
    node-role.kubernetes.io: infra2
    node-role.kubernetes.io/infra2: ""
  name: infra2
  platform:
    gcp:
      osDisk: {}
      type: n1-standard-4
      zones:
      - ` + GCPRegion + `-a
      - ` + GCPRegion + `-b
      - ` + GCPRegion + `-c
      - ` + GCPRegion + `-f`
		var filename = testCaseID + "-machinepool-infra2.yaml"
		err = ioutil.WriteFile(filename, []byte(infra2MachinepoolYaml), 0644)
		defer os.Remove(filename)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", filename, "--ignore-not-found").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", filename).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check replicas is 0")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0 0", ok, 2*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)

		compat_otp.By("Check Hive supports autoscale for GCP")
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
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "1 1 1 1", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
		e2e.Logf("Delete busybox in remote cluster and check machines will scale down to minReplicas %s", autoScalingMin)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("--kubeconfig="+kubeconfig, "-f", workloadYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check replicas will scale down to minimum value")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "0 0 0 0", ok, 5*DefaultTimeout, []string{"--kubeconfig=" + kubeconfig, "MachineSet", "-n", "openshift-machine-api", "-l", "hive.openshift.io/machine-pool=infra2", "-o=jsonpath={.items[*].status.replicas}"}).check(oc)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "46729"|./bin/extended-platform-tests run --timeout 60m -f -
	g.It("[Level0] NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:lwan-Medium-46729-[HIVE]Support overriding installer image [Serial]", func() {
		testCaseID := "46729"
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

		oc.SetupProject()
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and gcp-credentials to target namespace for the clusterdeployment
		compat_otp.By("Copy GCP platform credentials...")
		createGCPCreds(oc, oc.Namespace())

		compat_otp.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		compat_otp.By("Create GCP Install-Config Secret...")
		installConfigTemp := filepath.Join(testDataDir, "gcp-install-config.yaml")
		installConfigSecretName := cdName + "-install-config"
		projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		installConfigSecret := gcpInstallConfig{
			name1:      installConfigSecretName,
			namespace:  oc.Namespace(),
			baseDomain: GCPBaseDomain,
			name2:      cdName,
			region:     GCPRegion,
			projectid:  projectID,
			template:   installConfigTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		compat_otp.By("Create GCP ClusterDeployment...")
		clusterTemp := filepath.Join(testDataDir, "clusterdeployment-gcp.yaml")
		clusterVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion/version", "-o=jsonpath={.status.desired.version}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(clusterVersion).NotTo(o.BeEmpty())
		installerImageForOverride, err := getPullSpec(oc, "installer", clusterVersion)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(installerImageForOverride).NotTo(o.BeEmpty())
		e2e.Logf("ClusterVersion is %s, installerImageForOverride is %s", clusterVersion, installerImageForOverride)
		cluster := gcpClusterDeployment{
			fake:                   "false",
			name:                   cdName,
			namespace:              oc.Namespace(),
			baseDomain:             GCPBaseDomain,
			clusterName:            cdName,
			platformType:           "gcp",
			credRef:                GCPCreds,
			region:                 GCPRegion,
			imageSetRef:            imageSetName,
			installConfigSecret:    installConfigSecretName,
			pullSecretRef:          PullSecret,
			installerImageOverride: installerImageForOverride,
			installAttemptsLimit:   3,
			template:               clusterTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
		cluster.create(oc)

		compat_otp.By("Check installer image is overrided via \"installerImageOverride\" field")
		e2e.Logf("Check cd .status.installerImage")
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, installerImageForOverride, ok, 2*DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.installerImage}"}).check(oc)
		e2e.Logf("Check Installer commitID in provision pod log matches commitID from overrided Installer image")
		commitID, err := getCommitID(oc, "\" installer \"", clusterVersion)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(commitID).NotTo(o.BeEmpty())
		e2e.Logf("Installer commitID is %s", commitID)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "", nok, DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.provisionRef.name}"}).check(oc)
		provisionName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.provisionRef.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", "logs", asAdmin, withoutNamespace, contain, commitID, ok, DefaultTimeout, []string{"-n", oc.Namespace(), fmt.Sprintf("jobs/%s-provision", provisionName), "-c", "hive"}).check(oc)
	})

	//author: lwan@redhat.com
	//default duration is 15m for extended-platform-tests and 35m for jenkins job, need to reset for ClusterPool and ClusterDeployment cases
	//example: ./bin/extended-platform-tests run all --dry-run|grep "45279"|./bin/extended-platform-tests run --timeout 15m -f -
	g.It("[Level0] NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:lwan-Medium-45279-Test Metric for ClusterClaim[Serial]", func() {
		// Expose Hive metrics, and neutralize the effect after finishing the test case
		needRecover, prevConfig := false, ""
		defer recoverClusterMonitoring(oc, &needRecover, &prevConfig)
		exposeMetrics(oc, testDataDir, &needRecover, &prevConfig)

		testCaseID := "45279"
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
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and gcp-credentials to target namespace for the pool
		compat_otp.By("Copy GCP platform credentials...")
		createGCPCreds(oc, oc.Namespace())

		compat_otp.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		compat_otp.By("Create ClusterPool...")
		poolTemp := filepath.Join(testDataDir, "clusterpool-gcp.yaml")
		pool := gcpClusterPool{
			name:           poolName,
			namespace:      oc.Namespace(),
			fake:           "true",
			baseDomain:     GCPBaseDomain,
			imageSetRef:    imageSetName,
			platformType:   "gcp",
			credRef:        GCPCreds,
			region:         GCPRegion,
			pullSecretRef:  PullSecret,
			size:           2,
			maxSize:        2,
			runningCount:   2,
			maxConcurrent:  2,
			hibernateAfter: "360m",
			template:       poolTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterPool", oc.Namespace(), poolName})
		pool.create(oc)

		compat_otp.By("Check if GCP ClusterPool created successfully and become ready")
		//runningCount is 2 so pool status should be standby: 0, ready: 2
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "2", ok, DefaultTimeout, []string{"ClusterPool", poolName, "-n", oc.Namespace(), "-o=jsonpath={.status.ready}"}).check(oc)

		compat_otp.By("Check if CD is Running")
		cdListStr := getCDlistfromPool(oc, poolName)
		var cdArray []string
		cdArray = strings.Split(strings.TrimSpace(cdListStr), "\n")
		for i := range cdArray {
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "Running", ok, DefaultTimeout, []string{"ClusterDeployment", cdArray[i], "-n", cdArray[i]}).check(oc)
		}

		compat_otp.By("Create ClusterClaim...")
		claimTemp := filepath.Join(testDataDir, "clusterclaim.yaml")
		claimName1 := poolName + "-claim-1"
		claim1 := clusterClaim{
			name:            claimName1,
			namespace:       oc.Namespace(),
			clusterPoolName: poolName,
			template:        claimTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterClaim", oc.Namespace(), claimName1})
		claim1.create(oc)
		e2e.Logf("Check if ClusterClaim %s created successfully", claimName1)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, claimName1, ok, DefaultTimeout, []string{"ClusterClaim", "-n", oc.Namespace(), "-o=jsonpath={.items[*].metadata.name}"}).check(oc)

		compat_otp.By("Check Metrics for ClusterClaim...")
		token, err := compat_otp.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		query1 := "hive_clusterclaim_assignment_delay_seconds_sum"
		query2 := "hive_clusterclaim_assignment_delay_seconds_count"
		query3 := "hive_clusterclaim_assignment_delay_seconds_bucket"
		query := []string{query1, query2, query3}
		compat_otp.By("Check hive metrics for clusterclaim exist")
		checkMetricExist(oc, ok, token, thanosQuerierURL, query)
		e2e.Logf("Check metric %s Value is 1", query2)
		checkResourcesMetricValue(oc, poolName, oc.Namespace(), "1", token, thanosQuerierURL, query2)

		compat_otp.By("Create another ClusterClaim...")
		claimName2 := poolName + "-claim-2"
		claim2 := clusterClaim{
			name:            claimName2,
			namespace:       oc.Namespace(),
			clusterPoolName: poolName,
			template:        claimTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"ClusterClaim", oc.Namespace(), claimName2})
		claim2.create(oc)
		e2e.Logf("Check if ClusterClaim %s created successfully", claimName2)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, claimName2, ok, DefaultTimeout, []string{"ClusterClaim", "-n", oc.Namespace(), "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		e2e.Logf("Check metric %s Value change to 2", query2)
		checkResourcesMetricValue(oc, poolName, oc.Namespace(), "2", token, thanosQuerierURL, query2)
	})

	//author: mihuang@redhat.com
	//example: ./bin/extended-platform-tests run all --dry-run|grep "54463"|./bin/extended-platform-tests run --timeout 35m -f -
	g.It("[Level0] NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:mihuang-Medium-54463-Add cluster install success/fail metrics[Serial]", func() {
		// Expose Hive metrics, and neutralize the effect after finishing the test case
		needRecover, prevConfig := false, ""
		defer recoverClusterMonitoring(oc, &needRecover, &prevConfig)
		exposeMetrics(oc, testDataDir, &needRecover, &prevConfig)

		testCaseID := "54463"
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

		oc.SetupProject()
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and gcp-credentials to target namespace for the clusterdeployment
		compat_otp.By("Don't copy GCP platform credentials make install fail...")
		//createGCPCreds(oc, oc.Namespace())

		compat_otp.By("Copy pull-secret...")
		createPullSecret(oc, oc.Namespace())

		compat_otp.By("Create GCP Install-Config Secret...")
		installConfigTemp := filepath.Join(testDataDir, "gcp-install-config.yaml")
		installConfigSecretName := cdName + "-install-config"
		projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		installConfigSecret := gcpInstallConfig{
			name1:      installConfigSecretName,
			namespace:  oc.Namespace(),
			baseDomain: GCPBaseDomain,
			name2:      cdName,
			region:     GCPRegion,
			projectid:  projectID,
			template:   installConfigTemp,
		}
		defer cleanupObjects(oc, objectTableRef{"secret", oc.Namespace(), installConfigSecretName})
		installConfigSecret.create(oc)

		compat_otp.By("Get SA token to check Metrics...")
		token, err := compat_otp.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())

		var installAttemptsLimit = []int{3, 1}
		for i := 0; i < len(installAttemptsLimit); i++ {
			func() {
				if installAttemptsLimit[i] == 3 {
					compat_otp.By("Config GCP ClusterDeployment with installAttemptsLimit=3 and make install fail..")
				} else {
					compat_otp.By("Config GCP ClusterDeployment with installAttemptsLimit=1 and make install success..")
					compat_otp.By("Copy GCP platform credentials make install success...")
					createGCPCreds(oc, oc.Namespace())
				}
				cluster := gcpClusterDeployment{
					fake:                 "true",
					name:                 cdName,
					namespace:            oc.Namespace(),
					baseDomain:           GCPBaseDomain,
					clusterName:          cdName,
					platformType:         "gcp",
					credRef:              GCPCreds,
					region:               GCPRegion,
					imageSetRef:          cdName + "-imageset",
					installConfigSecret:  cdName + "-install-config",
					pullSecretRef:        PullSecret,
					installAttemptsLimit: installAttemptsLimit[i],
					template:             filepath.Join(testDataDir, "clusterdeployment-gcp.yaml"),
				}
				defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", oc.Namespace(), cdName})
				cluster.create(oc)

				if installAttemptsLimit[i] == 3 {
					newCheck("expect", "get", asAdmin, withoutNamespace, contain, "InstallAttemptsLimitReached", ok, 5*DefaultTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[?(@.type==\"ProvisionStopped\")].reason}"}).check(oc)
					o.Expect(checkResourceNumber(oc, cdName, []string{"pods", "-A"})).To(o.Equal(3))
					queryFailSum := "hive_cluster_deployment_install_failure_total_sum"
					queryFailCount := "hive_cluster_deployment_install_failure_total_count"
					queryFailBucket := "hive_cluster_deployment_install_failure_total_bucket"
					queryFail := []string{queryFailSum, queryFailCount, queryFailBucket}
					compat_otp.By("Check hive metrics for cd install fail")
					checkMetricExist(oc, ok, token, thanosQuerierURL, queryFail)
					e2e.Logf("Check metric %s with install_attempt = 2", queryFailCount)
					checkResourcesMetricValue(oc, GCPRegion, HiveNamespace, "2", token, thanosQuerierURL, queryFailCount)
					e2e.Logf("delete cd and create a success case")
				} else {
					compat_otp.By("Check GCP ClusterDeployment installed flag is true")
					newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
					querySuccSum := "hive_cluster_deployment_install_success_total_sum"
					querySuccCount := "hive_cluster_deployment_install_success_total_count"
					querySuccBucket := "hive_cluster_deployment_install_success_total_bucket"
					querySuccess := []string{querySuccSum, querySuccCount, querySuccBucket}
					compat_otp.By("Check hive metrics for cd installed successfully")
					checkMetricExist(oc, ok, token, thanosQuerierURL, querySuccess)
					e2e.Logf("Check metric %s with with install_attempt = 0", querySuccCount)
					checkResourcesMetricValue(oc, GCPRegion, HiveNamespace, "0", token, thanosQuerierURL, querySuccCount)
				}
			}()
		}
	})

	// Timeout: 60min
	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jshu-High-68294-GCP Shared VPC support for MachinePool[Serial]", func() {
		testCaseID := "68294"
		cdName := "cluster-" + testCaseID + "-" + getRandomString()[:ClusterSuffixLen]
		//oc.SetupProject()

		compat_otp.By("Config GCP Install-Config Secret...")
		projectID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure/cluster", "-o=jsonpath={.status.platformStatus.gcp.projectID}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(projectID).NotTo(o.BeEmpty())
		installConfigSecret := gcpInstallConfig{
			name1:              cdName + "-install-config",
			namespace:          oc.Namespace(),
			baseDomain:         GCPBaseDomain,
			name2:              cdName,
			region:             GCPRegion,
			projectid:          projectID,
			computeSubnet:      "installer-shared-vpc-subnet-2",
			controlPlaneSubnet: "installer-shared-vpc-subnet-1",
			network:            "installer-shared-vpc",
			networkProjectId:   "openshift-qe-shared-vpc",
			template:           filepath.Join(testDataDir, "gcp-install-config-sharedvpc.yaml"),
		}
		compat_otp.By("Config GCP ClusterDeployment...")
		cluster := gcpClusterDeployment{
			fake:                 "false",
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           GCPBaseDomain,
			clusterName:          cdName,
			platformType:         "gcp",
			credRef:              GCPCreds,
			region:               GCPRegion,
			imageSetRef:          cdName + "-imageset",
			installConfigSecret:  cdName + "-install-config",
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 3,
			template:             filepath.Join(testDataDir, "clusterdeployment-gcp.yaml"),
		}
		defer cleanCD(oc, cluster.name+"-imageset", oc.Namespace(), installConfigSecret.name1, cluster.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cluster)

		compat_otp.By("Create the infra MachinePool with the shared vpc...")
		inframachinepoolGCPTemp := filepath.Join(testDataDir, "machinepool-infra-gcp-sharedvpc.yaml")
		inframp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    inframachinepoolGCPTemp,
		}
		defer cleanupObjects(oc,
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-infra"},
		)
		inframp.create(oc)

		compat_otp.By("Check GCP ClusterDeployment installed flag is true")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "true", ok, ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-n", oc.Namespace(), "-o=jsonpath={.spec.installed}"}).check(oc)
		compat_otp.By("Check the infra MachinePool .status.replicas = 1")
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "1", ok, DefaultTimeout, []string{"MachinePool", cdName + "-infra", "-n", oc.Namespace(), "-o=jsonpath={.status.replicas}"}).check(oc)

	})

})
