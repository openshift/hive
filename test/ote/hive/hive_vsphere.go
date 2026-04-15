package hive

import (
	"context"
	"fmt"
	"net"
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	"github.com/3th1nk/cidr"

	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	"github.com/openshift/origin/test/extended/util/compat_otp/architecture"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[OTP][sig-hive] Cluster_Operator hive should", func() {
	defer g.GinkgoRecover()

	var (
		// Clients
		oc = compat_otp.NewCLI("hive", compat_otp.KubeConfigPath())

		// Test-specific
		testDataDir  string
		testOCPImage string
		randStr      string

		// Platform-specific
		datacenter       string
		datastore        string
		network          string
		networkCIDR      *cidr.CIDR
		minIp            net.IP
		maxIp            net.IP
		machineIPs       []string
		vCenter          string
		cluster          string
		basedomain       string
		awsCredsFilePath string
		tEnv             testEnv
	)

	// Under the hood, "extended-platform-tests run" calls "extended-platform-tests run-test" on each test
	// case separately. This means that all necessary initializations need to be done before every single
	// test case, either globally or in a Ginkgo node like BeforeEach.
	g.BeforeEach(func() {
		// Skip incompatible platforms
		compat_otp.SkipIfPlatformTypeNot(oc, "vsphere")
		architecture.SkipNonAmd64SingleArch(oc)

		// Get test-specific info
		testDataDir = FixturePath("testdata")
		testOCPImage = getTestOCPImage(oc)
		randStr = getRandomString()[:ClusterSuffixLen]

		// Get platform-specific info
		tEnv = getTestEnv()
		awsCredsFilePath = getAWSCredsFilePath4VSphere(tEnv)
		basedomain = getBasedomain(oc)
		networkCIDR, minIp, maxIp = getVSphereCIDR(oc)
		machineIPs = getVMInternalIPs(oc)
		infrastructure, err := oc.
			AdminConfigClient().
			ConfigV1().
			Infrastructures().
			Get(context.Background(), "cluster", metav1.GetOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		failureDomains := infrastructure.Spec.PlatformSpec.VSphere.FailureDomains
		datacenter = failureDomains[0].Topology.Datacenter
		datastore = failureDomains[0].Topology.Datastore
		network = failureDomains[0].Topology.Networks[0]
		vCenter = failureDomains[0].Server
		cluster = failureDomains[0].Topology.ComputeCluster
		e2e.Logf(`Found platform-specific info:
- Datacenter: %s
- Datastore: %s
- Network: %s
- Machine IPs: %s, 
- vCenter Server: %s
- Cluster: %s
- Base domain: %s
- Test environment: %s
- AWS creds file path: %s`, datacenter, datastore, network, machineIPs, vCenter, cluster, basedomain, tEnv, awsCredsFilePath)

		// Install Hive operator if necessary
		_, _ = installHiveOperator(oc, &hiveNameSpace{}, &operatorGroup{}, &subscription{}, &hiveconfig{}, testDataDir)
	})

	// Author: fxie@redhat.com
	// Timeout: 60min
	g.It("[Level0] NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:fxie-High-32026-Add hive api for vsphere provisioning [Serial]", func() {
		var (
			testCaseID      = "32026"
			cdName          = fmt.Sprintf("cd-%s-%s", testCaseID, randStr)
			icSecretName    = fmt.Sprintf("%s-install-config", cdName)
			imageSetName    = fmt.Sprintf("%s-imageset", cdName)
			apiDomain       = fmt.Sprintf("api.%v.%v", cdName, basedomain)
			ingressDomain   = fmt.Sprintf("*.apps.%v.%v", cdName, basedomain)
			domains2Reserve = []string{apiDomain, ingressDomain}
		)

		compat_otp.By("Extracting root credentials")
		username, password := getVSphereCredentials(oc, vCenter)

		compat_otp.By(fmt.Sprintf("Reserving API/ingress IPs for domains %v", domains2Reserve))
		fReserve, fRelease, domain2Ip := getIps2ReserveFromAWSHostedZone(oc, basedomain,
			networkCIDR, minIp, maxIp, machineIPs, awsCredsFilePath, domains2Reserve)
		defer fRelease()
		fReserve()

		compat_otp.By("Creating ClusterDeployment and related resources")
		installConfigSecret := vSphereInstallConfig{
			secretName:     icSecretName,
			secretNs:       oc.Namespace(),
			baseDomain:     basedomain,
			icName:         cdName,
			cluster:        cluster,
			machineNetwork: networkCIDR.CIDR().String(),
			apiVip:         domain2Ip[apiDomain],
			datacenter:     datacenter,
			datastore:      datastore,
			ingressVip:     domain2Ip[ingressDomain],
			network:        network,
			password:       password,
			username:       username,
			vCenter:        vCenter,
			template:       filepath.Join(testDataDir, "vsphere-install-config.yaml"),
		}
		cd := vSphereClusterDeployment{
			fake:                 false,
			name:                 cdName,
			namespace:            oc.Namespace(),
			baseDomain:           basedomain,
			manageDns:            false,
			clusterName:          cdName,
			certRef:              VSphereCerts,
			cluster:              cluster,
			credRef:              VSphereCreds,
			datacenter:           datacenter,
			datastore:            datastore,
			network:              network,
			vCenter:              vCenter,
			imageSetRef:          imageSetName,
			installConfigSecret:  icSecretName,
			pullSecretRef:        PullSecret,
			installAttemptsLimit: 1,
			template:             filepath.Join(testDataDir, "clusterdeployment-vsphere.yaml"),
		}
		defer cleanCD(oc, imageSetName, oc.Namespace(), installConfigSecret.secretName, cd.name)
		createCD(testDataDir, testOCPImage, oc, oc.Namespace(), installConfigSecret, cd)

		compat_otp.By("Create worker MachinePool ...")
		workermachinepoolVSphereTemp := filepath.Join(testDataDir, "machinepool-worker-vsphere.yaml")
		workermp := machinepool{
			namespace:   oc.Namespace(),
			clusterName: cdName,
			template:    workermachinepoolVSphereTemp,
		}

		defer cleanupObjects(oc,
			objectTableRef{"MachinePool", oc.Namespace(), cdName + "-worker"},
		)
		workermp.create(oc)

		compat_otp.By("Waiting for the CD to be installed")
		// TODO(fxie): fail early in case of ProvisionStopped
		newCheck("expect", "get", asAdmin, requireNS, compare, "true", ok,
			ClusterInstallTimeout, []string{"ClusterDeployment", cdName, "-o=jsonpath={.spec.installed}"}).check(oc)
		// Check the worker MP in good conditions
		newCheck("expect", "get", asAdmin, requireNS, contain, "3", ok,
			WaitingForClusterOperatorsTimeout, []string{"MachinePool", cdName + "-worker", "-o=jsonpath={.status.replicas}"}).check(oc)
	})
})
