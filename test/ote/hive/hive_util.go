package hive

import (
	"bufio"
	"context"
	"crypto"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"

	"github.com/3th1nk/cidr"

	"gopkg.in/yaml.v3"

	legoroute53 "github.com/go-acme/lego/v4/providers/dns/route53"
	"github.com/go-acme/lego/v4/registration"

	o "github.com/onsi/gomega"

	exutil "github.com/openshift/origin/test/extended/util"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type clusterMonitoringConfig struct {
	enableUserWorkload bool
	namespace          string
	template           string
}

type hiveNameSpace struct {
	name     string
	template string
}

type operatorGroup struct {
	name      string
	namespace string
	template  string
}

type subscription struct {
	name            string
	namespace       string
	channel         string
	approval        string
	operatorName    string
	sourceName      string
	sourceNamespace string
	startingCSV     string
	currentCSV      string
	installedCSV    string
	template        string
}

type hiveconfig struct {
	logLevel        string
	targetNamespace string
	template        string
}

type clusterImageSet struct {
	name         string
	releaseImage string
	template     string
}

type clusterPool struct {
	name           string
	namespace      string
	fake           string
	baseDomain     string
	imageSetRef    string
	platformType   string
	credRef        string
	region         string
	pullSecretRef  string
	size           int
	maxSize        int
	runningCount   int
	maxConcurrent  int
	hibernateAfter string
	template       string
}

type clusterPoolCDC struct {
	name           string
	namespace      string
	fake           string
	baseDomain     string
	imageSetRef    string
	platformType   string
	credRef        string
	region         string
	pullSecretRef  string
	size           int
	maxSize        int
	runningCount   int
	maxConcurrent  int
	hibernateAfter string
	inventoryCDC1  string
	inventoryCDC2  string
	inventoryCDC3  string
	template       string
}

type clusterClaim struct {
	name            string
	namespace       string
	clusterPoolName string
	template        string
}

type installConfig struct {
	name1              string
	namespace          string
	baseDomain         string
	name2              string
	region             string
	template           string
	publish            string
	vmType             string
	arch               string
	credentialsMode    string
	internalJoinSubnet string
	clusterNetworkMtu  int
}

type installConfigPrivateLink struct {
	name1              string
	namespace          string
	baseDomain         string
	name2              string
	region             string
	template           string
	publish            string
	vmType             string
	arch               string
	credentialsMode    string
	internalJoinSubnet string
	privateSubnetId1   string
	privateSubnetId2   string
	privateSubnetId3   string
	machineNetworkCidr string
}

type clusterDeployment struct {
	fake                 string
	installerType        string
	name                 string
	namespace            string
	baseDomain           string
	clusterName          string
	manageDNS            bool
	platformType         string
	credRef              string
	region               string
	imageSetRef          string
	installConfigSecret  string
	pullSecretRef        string
	installAttemptsLimit int
	customizedTag        string
	template             string
}

type clusterDeploymentAdopt struct {
	name               string
	namespace          string
	baseDomain         string
	adminKubeconfigRef string
	clusterID          string
	infraID            string
	clusterName        string
	manageDNS          bool
	platformType       string
	credRef            string
	region             string
	pullSecretRef      string
	preserveOnDelete   bool
	template           string
}

type clusterDeploymentAssumeRole struct {
	fake                                   string
	installerType                          string
	name                                   string
	namespace                              string
	baseDomain                             string
	boundServiceAccountSigningKeySecretRef string
	roleARN                                string
	externalID                             string
	clusterName                            string
	manageDNS                              bool
	platformType                           string
	region                                 string
	manifestsSecretRef                     string
	imageSetRef                            string
	installConfigSecret                    string
	pullSecretRef                          string
	installAttemptsLimit                   int
	template                               string
}

type clusterDeploymentPrivateLink struct {
	fake                 string
	name                 string
	namespace            string
	baseDomain           string
	clusterName          string
	manageDNS            bool
	credRef              string
	region               string
	imageSetRef          string
	installConfigSecret  string
	pullSecretRef        string
	installAttemptsLimit int
	template             string
}

type machinepool struct {
	clusterName      string
	namespace        string
	iops             int
	template         string
	authentication   string
	gcpSecureBoot    string
	networkProjectID string
	customizedTag    string
}

type syncSetResource struct {
	name          string
	namespace     string
	namespace2    string
	cdrefname     string
	ramode        string
	applybehavior string
	cmname        string
	cmnamespace   string
	template      string
}

type syncSetPatch struct {
	name        string
	namespace   string
	cdrefname   string
	cmname      string
	cmnamespace string
	pcontent    string
	patchType   string
	template    string
}

type syncSetSecret struct {
	name       string
	namespace  string
	cdrefname  string
	sname      string
	snamespace string
	tname      string
	tnamespace string
	template   string
}

type objectTableRef struct {
	kind      string
	namespace string
	name      string
}

// Azure
type azureInstallConfig struct {
	name1      string
	namespace  string
	baseDomain string
	name2      string
	resGroup   string
	azureType  string
	region     string
	template   string
}

type azureClusterDeployment struct {
	fake                   string
	copyCliDomain          string
	name                   string
	namespace              string
	baseDomain             string
	clusterName            string
	platformType           string
	credRef                string
	region                 string
	resGroup               string
	azureType              string
	imageSetRef            string
	installConfigSecret    string
	installerImageOverride string
	pullSecretRef          string
	template               string
}

type azureClusterPool struct {
	name           string
	namespace      string
	fake           string
	baseDomain     string
	imageSetRef    string
	platformType   string
	credRef        string
	region         string
	resGroup       string
	pullSecretRef  string
	size           int
	maxSize        int
	runningCount   int
	maxConcurrent  int
	hibernateAfter string
	template       string
}

// GCP
type gcpInstallConfig struct {
	name1              string
	namespace          string
	baseDomain         string
	name2              string
	region             string
	projectid          string
	template           string
	secureBoot         string
	computeSubnet      string
	controlPlaneSubnet string
	network            string
	networkProjectId   string
}

type gcpClusterDeployment struct {
	fake                        string
	name                        string
	namespace                   string
	baseDomain                  string
	clusterName                 string
	platformType                string
	credRef                     string
	region                      string
	imageSetRef                 string
	installConfigSecret         string
	pullSecretRef               string
	installerImageOverride      string
	installAttemptsLimit        int
	template                    string
	serviceAttachmentSubnetCidr string
}

type gcpClusterPool struct {
	name           string
	namespace      string
	fake           string
	baseDomain     string
	imageSetRef    string
	platformType   string
	credRef        string
	region         string
	pullSecretRef  string
	size           int
	maxSize        int
	runningCount   int
	maxConcurrent  int
	hibernateAfter string
	template       string
}

// vSphere
type vSphereInstallConfig struct {
	secretName     string
	secretNs       string
	baseDomain     string
	icName         string
	machineNetwork string
	apiVip         string
	cluster        string
	datacenter     string
	datastore      string
	ingressVip     string
	network        string
	password       string
	username       string
	vCenter        string
	template       string
}

type vSphereClusterDeployment struct {
	fake                 bool
	name                 string
	namespace            string
	baseDomain           string
	manageDns            bool
	clusterName          string
	certRef              string
	cluster              string
	credRef              string
	datacenter           string
	datastore            string
	network              string
	vCenter              string
	imageSetRef          string
	installConfigSecret  string
	pullSecretRef        string
	installAttemptsLimit int
	template             string
}

type ClusterDeploymentCustomization struct {
	name      string
	namespace string
	region    string
	template  string
}

type prometheusQueryResult struct {
	Data struct {
		Result []struct {
			Metric struct {
				Name                 string `json:"__name__"`
				ClusterpoolName      string `json:"clusterpool_name"`
				ClusterpoolNamespace string `json:"clusterpool_namespace"`
				ClusterDeployment    string `json:"cluster_deployment"`
				ExportedNamespace    string `json:"exported_namespace"`
				ClusterType          string `json:"cluster_type"`
				ClusterVersion       string `json:"cluster_version"`
				InstallAttempt       string `json:"install_attempt"`
				Platform             string `json:"platform"`
				Region               string `json:"region"`
				Prometheus           string `json:"prometheus"`
				Condition            string `json:"condition"`
				Reason               string `json:"reason"`
				Endpoint             string `json:"endpoint"`
				Instance             string `json:"instance"`
				Job                  string `json:"job"`
				Namespace            string `json:"namespace"`
				Pod                  string `json:"pod"`
				Workers              string `json:"workers"`
				Service              string `json:"service"`
			} `json:"metric"`
			Value []interface{} `json:"value"`
		} `json:"result"`
		ResultType string `json:"resultType"`
	} `json:"data"`
	Status string `json:"status"`
}

// This type is defined to avoid requiring openshift/installer types
// which bring in quite a few dependencies, making this repo unnecessarily
// difficult to maintain.
type minimalInstallConfig struct {
	Networking struct {
		MachineNetwork []struct {
			CIDR string `yaml:"cidr"`
		} `yaml:"machineNetwork"`
	} `yaml:"networking"`
}

type legoUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

type testEnv string

// General configurations
const (
	pemX509CertPattern = "-----BEGIN CERTIFICATE-----\\n([A-Za-z0-9+/=\\n]+)\\n-----END CERTIFICATE-----"
)

// Installer Configurations
const (
	PublishExternal              = "External"
	PublishInternal              = "Internal"
	AWSVmTypeARM64               = "m6g.xlarge"
	AWSVmTypeAMD64               = "m6i.xlarge"
	archARM64                    = "arm64"
	archAMD64                    = "amd64"
	defaultAWSInternalJoinSubnet = "100.64.0.0/16"
	defaultAWSMachineNetworkCidr = "10.0.0.0/16"
	defaultAWSClusterNetworkMtu  = 8901
	customAWSClusterNetworkMtu   = 1200
)

// Monitoring configurations
const (
	PrometheusURL    = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query="
	thanosQuerierURL = "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query?query="
)

// Hive Configurations
const (
	HiveNamespace                     = "hive" //Hive Namespace
	PullSecret                        = "pull-secret"
	hiveAdditionalCASecret            = "hive-additional-ca"
	HiveImgRepoOnQuay                 = "redhat-user-workloads/crt-redhat-acm-tenant/hive-operator"
	ClusterInstallTimeout             = 3600
	DefaultTimeout                    = 120
	WaitingForClusterOperatorsTimeout = 600
	FakeClusterInstallTimeout         = 600
	ClusterResumeTimeout              = 1200
	ClusterUninstallTimeout           = 1800
	HibernateAfterTimer               = 300
	ClusterSuffixLen                  = 4
	LogsLimitLen                      = 1024
	HiveManagedDNS                    = "hivemanageddns" //for all manage DNS Domain
)

// Test environments
const (
	testEnvLocal   testEnv = "Local"
	testEnvJenkins testEnv = "Jenkins"
	testEnvCI      testEnv = "CI"
)

// AWS Configurations
const (
	AWSBaseDomain   = "qe.devcluster.openshift.com" //AWS BaseDomain
	AWSRegion       = "us-east-2"
	AWSRegion2      = "us-east-1"
	AWSCreds        = "aws-creds"
	AWSCredsPattern = `\[default\]
aws_access_key_id = ([a-zA-Z0-9+/]+)
aws_secret_access_key = ([a-zA-Z0-9+/]+)`
	AWSDefaultCDTag = "hive-qe-cd-tag" //Default customized userTag defined ClusterDeployment's spec
	AWSDefaultMPTag = "hive-qe-mp-tag" //Default customized userTag defined MachinePool's spec
)

// Azure Configurations
const (
	AzureClusterInstallTimeout = 4500
	AzureBaseDomain            = "qe.azure.devcluster.openshift.com" //Azure BaseDomain
	AzureRegion                = "centralus"
	AzureRegion2               = "eastus"
	AzureCreds                 = "azure-credentials"
	AzureRESGroup              = "os4-common"
	AzurePublic                = "AzurePublicCloud"
	AzureGov                   = "AzureUSGovernmentCloud"
)

// GCP Configurations
const (
	GCPBaseDomain      = "qe.gcp.devcluster.openshift.com" //GCP BaseDomain
	GCPBaseDomain2     = "qe1.gcp.devcluster.openshift.com"
	GCPRegion          = "us-central1"
	GCPRegion2         = "us-east1"
	GCPCreds           = "gcp-credentials"
	GCPCredentialsPath = "/var/run/secrets/ci.openshift.io/cluster-profile/gce.json"
)

// VSphere configurations
const (
	VSphereCreds              = "vsphere-creds"
	VSphereCerts              = "vsphere-certs"
	VSphereAWSCredsFilePathCI = "/var/run/vault/aws/.awscred"
	VSphereNetworkPattern     = "[a-zA-Z]+-[a-zA-Z]+-([\\d]+)"
	VSphereLastCidrOctetMin   = 3
	VSphereLastCidrOctetMax   = 49
)

func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var cfgFileJSON string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "-hive-resource-cfg.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		cfgFileJSON = output
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, "fail to create config file")

	e2e.Logf("the file of resource is %s", cfgFileJSON)
	defer os.Remove(cfgFileJSON)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", cfgFileJSON).Execute()
}

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

func (u *legoUser) GetEmail() string {
	return u.Email
}

func (u *legoUser) GetRegistration() *registration.Resource {
	return u.Registration
}

func (u *legoUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

func (cmc *clusterMonitoringConfig) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cmc.template, "-p", "ENABLEUSERWORKLOAD="+strconv.FormatBool(cmc.enableUserWorkload), "NAMESPACE="+cmc.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create hive namespace if not exist
func (ns *hiveNameSpace) createIfNotExist(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ns.template, "-p", "NAME="+ns.name)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create operatorGroup for Hive if not exist
func (og *operatorGroup) createIfNotExist(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", og.template, "-p", "NAME="+og.name, "NAMESPACE="+og.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (sub *subscription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sub.template, "-p", "NAME="+sub.name, "NAMESPACE="+sub.namespace, "CHANNEL="+sub.channel,
		"APPROVAL="+sub.approval, "OPERATORNAME="+sub.operatorName, "SOURCENAME="+sub.sourceName, "SOURCENAMESPACE="+sub.sourceNamespace, "STARTINGCSV="+sub.startingCSV)
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Compare(sub.approval, "Automatic") == 0 {
		sub.findInstalledCSV(oc)
	} else {
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "UpgradePending", ok, DefaultTimeout, []string{"sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)
	}
}

// Create subscription for Hive if not exist and wait for resource is ready
func (sub *subscription) createIfNotExist(oc *exutil.CLI) {

	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", sub.namespace).Output()
	if strings.Contains(output, "NotFound") || strings.Contains(output, "No resources") || err != nil {
		e2e.Logf("No hive subscription, Create it.")
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sub.template, "-p", "NAME="+sub.name, "NAMESPACE="+sub.namespace, "CHANNEL="+sub.channel,
			"APPROVAL="+sub.approval, "OPERATORNAME="+sub.operatorName, "SOURCENAME="+sub.sourceName, "SOURCENAMESPACE="+sub.sourceNamespace, "STARTINGCSV="+sub.startingCSV)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(sub.approval, "Automatic") == 0 {
			sub.findInstalledCSV(oc)
		} else {
			newCheck("expect", "get", asAdmin, withoutNamespace, compare, "UpgradePending", ok, DefaultTimeout, []string{"sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)
		}
		//wait for pod running
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, 5*DefaultTimeout, []string{"pod", "--selector=control-plane=hive-operator", "-n",
			sub.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		//Check if need to replace with the latest Hive image
		hiveDeployedImg, _, err := oc.
			AsAdmin().
			WithoutNamespace().
			Run("get").
			Args("csv", sub.installedCSV, "-n", sub.namespace,
				"-o", "jsonpath={.spec.install.spec.deployments[0].spec.template.spec.containers[0].image}").
			Outputs()
		if err != nil {
			e2e.Logf("Failed to get Hive image: %v", err)
		} else {
			e2e.Logf("Found Hive deployed image = %v", hiveDeployedImg)
			latestHiveVer := getLatestHiveVersion()
			if strings.Contains(hiveDeployedImg, latestHiveVer) {
				e2e.Logf("The deployed Hive image is already the lastest.")
			} else {
				e2e.Logf("The deployed Hive image is NOT the lastest, patched to the latest version: %v", latestHiveVer)
				patchYaml := `[{"op": "replace", "path": "/spec/install/spec/deployments/0/spec/template/spec/containers/0/image", "value": "quay.io/redhat-user-workloads/crt-redhat-acm-tenant/hive-operator/hive:versiontobepatched"}]`
				patchYaml = strings.Replace(patchYaml, "versiontobepatched", latestHiveVer, 1)
				err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("-n", sub.namespace, "csv", sub.installedCSV, "--type=json", "-p", patchYaml).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}
	} else {
		e2e.Logf("hive subscription already exists.")
	}

}

func (sub *subscription) findInstalledCSV(oc *exutil.CLI) {
	newCheck("expect", "get", asAdmin, withoutNamespace, compare, "AtLatestKnown", ok, 5*DefaultTimeout, []string{"sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)
	installedCSV := getResource(oc, asAdmin, withoutNamespace, "sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.installedCSV}")
	o.Expect(installedCSV).NotTo(o.BeEmpty())
	if strings.Compare(sub.installedCSV, installedCSV) != 0 {
		sub.installedCSV = installedCSV
	}
	e2e.Logf("the installed CSV name is %s", sub.installedCSV)
}

func (hc *hiveconfig) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", hc.template, "-p", "LOGLEVEL="+hc.logLevel, "TARGETNAMESPACE="+hc.targetNamespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create hivconfig if not exist and wait for resource is ready
func (hc *hiveconfig) createIfNotExist(oc *exutil.CLI) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HiveConfig", "hive").Output()
	if strings.Contains(output, "have a resource type") || err != nil {
		e2e.Logf("No hivconfig, Create it.")
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", hc.template, "-p", "LOGLEVEL="+hc.logLevel, "TARGETNAMESPACE="+hc.targetNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		//wait for pods running
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-clustersync", ok, WaitingForClusterOperatorsTimeout, []string{"pod", "--selector=control-plane=clustersync",
			"-n", HiveNamespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=clustersync", "-n",
			HiveNamespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-controllers", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=controller-manager",
			"-n", HiveNamespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DefaultTimeout, []string{"pod", "--selector=control-plane=controller-manager", "-n",
			HiveNamespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hiveadmission", ok, DefaultTimeout, []string{"pod", "--selector=app=hiveadmission",
			"-n", HiveNamespace, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DefaultTimeout, []string{"pod", "--selector=app=hiveadmission", "-n",
			HiveNamespace, "-o=jsonpath={.items[*].status.phase}"}).check(oc)
	} else {
		e2e.Logf("hivconfig already exists.")
	}

}

func (imageset *clusterImageSet) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", imageset.template, "-p", "NAME="+imageset.name, "RELEASEIMAGE="+imageset.releaseImage)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pool *clusterPool) create(oc *exutil.CLI) {
	parameters := []string{"--ignore-unknown-parameters=true", "-f", pool.template, "-p", "NAME=" + pool.name, "NAMESPACE=" + pool.namespace, "FAKE=" + pool.fake, "BASEDOMAIN=" + pool.baseDomain, "IMAGESETREF=" + pool.imageSetRef, "PLATFORMTYPE=" + pool.platformType, "CREDREF=" + pool.credRef, "PULLSECRETREF=" + pool.pullSecretRef, "SIZE=" + strconv.Itoa(pool.size), "MAXSIZE=" + strconv.Itoa(pool.maxSize), "RUNNINGCOUNT=" + strconv.Itoa(pool.runningCount), "MAXCONCURRENT=" + strconv.Itoa(pool.maxConcurrent), "HIBERNATEAFTER=" + pool.hibernateAfter, "REGION=" + pool.region}
	err := applyResourceFromTemplate(oc, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pool *clusterPoolCDC) create(oc *exutil.CLI) {
	parameters := []string{"--ignore-unknown-parameters=true", "-f", pool.template, "-p", "NAME=" + pool.name, "NAMESPACE=" + pool.namespace, "FAKE=" + pool.fake, "BASEDOMAIN=" + pool.baseDomain, "IMAGESETREF=" + pool.imageSetRef, "PLATFORMTYPE=" + pool.platformType, "CREDREF=" + pool.credRef, "PULLSECRETREF=" + pool.pullSecretRef, "SIZE=" + strconv.Itoa(pool.size), "MAXSIZE=" + strconv.Itoa(pool.maxSize), "RUNNINGCOUNT=" + strconv.Itoa(pool.runningCount), "MAXCONCURRENT=" + strconv.Itoa(pool.maxConcurrent), "HIBERNATEAFTER=" + pool.hibernateAfter}
	if len(pool.region) > 0 {
		parameters = append(parameters, "REGION="+pool.region)
	}
	if len(pool.inventoryCDC1) > 0 {
		parameters = append(parameters, "INVENTORYCDC1="+pool.inventoryCDC1)
	}
	if len(pool.inventoryCDC2) > 0 {
		parameters = append(parameters, "INVENTORYCDC2="+pool.inventoryCDC2)
	}
	if len(pool.inventoryCDC3) > 0 {
		parameters = append(parameters, "INVENTORYCDC3="+pool.inventoryCDC3)
	}
	err := applyResourceFromTemplate(oc, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (claim *clusterClaim) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", claim.template, "-p", "NAME="+claim.name, "NAMESPACE="+claim.namespace, "CLUSTERPOOLNAME="+claim.clusterPoolName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (config *installConfig) create(oc *exutil.CLI) {
	// Set default values
	if config.publish == "" {
		config.publish = PublishExternal
	}
	if config.vmType == "" {
		config.vmType = AWSVmTypeAMD64
	}
	if config.arch == "" {
		config.arch = archAMD64
	}

	parameters := []string{"--ignore-unknown-parameters=true", "-f", config.template, "-p", "NAME1=" + config.name1, "NAMESPACE=" + config.namespace, "BASEDOMAIN=" + config.baseDomain, "NAME2=" + config.name2, "REGION=" + config.region, "PUBLISH=" + config.publish, "VMTYPE=" + config.vmType, "ARCH=" + config.arch}
	if len(config.credentialsMode) > 0 {
		parameters = append(parameters, "CREDENTIALSMODE="+config.credentialsMode)
	}
	if len(config.internalJoinSubnet) == 0 {
		parameters = append(parameters, "INTERNALJOINSUBNET="+defaultAWSInternalJoinSubnet)
	} else {
		parameters = append(parameters, "INTERNALJOINSUBNET="+config.internalJoinSubnet)
	}
	if config.clusterNetworkMtu == 0 {
		//use default
		parameters = append(parameters, "CLUSTERNETWORKMTU="+strconv.Itoa(defaultAWSClusterNetworkMtu))
	} else {
		parameters = append(parameters, "CLUSTERNETWORKMTU="+strconv.Itoa(config.clusterNetworkMtu))
	}
	err := applyResourceFromTemplate(oc, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (config *installConfigPrivateLink) create(oc *exutil.CLI) {
	// Set default values
	if config.publish == "" {
		config.publish = PublishExternal
	}
	if config.vmType == "" {
		config.vmType = AWSVmTypeAMD64
	}
	if config.arch == "" {
		config.arch = archAMD64
	}

	parameters := []string{"--ignore-unknown-parameters=true", "-f", config.template, "-p", "NAME1=" + config.name1, "NAMESPACE=" + config.namespace, "BASEDOMAIN=" + config.baseDomain, "NAME2=" + config.name2, "REGION=" + config.region, "PUBLISH=" + config.publish, "VMTYPE=" + config.vmType, "ARCH=" + config.arch}
	if len(config.credentialsMode) > 0 {
		parameters = append(parameters, "CREDENTIALSMODE="+config.credentialsMode)
	}
	if len(config.internalJoinSubnet) == 0 {
		parameters = append(parameters, "INTERNALJOINSUBNET="+defaultAWSInternalJoinSubnet)
	} else {
		parameters = append(parameters, "INTERNALJOINSUBNET="+config.internalJoinSubnet)
	}
	if len(config.privateSubnetId1) > 0 {
		parameters = append(parameters, "PRIVATESUBNETID1="+config.privateSubnetId1)
	}
	if len(config.privateSubnetId2) > 0 {
		parameters = append(parameters, "PRIVATESUBNETID2="+config.privateSubnetId2)
	}
	if len(config.privateSubnetId3) > 0 {
		parameters = append(parameters, "PRIVATESUBNETID3="+config.privateSubnetId3)
	}
	if len(config.machineNetworkCidr) == 0 {
		parameters = append(parameters, "MACHINENETWORKCIDR="+defaultAWSMachineNetworkCidr)
	} else {
		parameters = append(parameters, "MACHINENETWORKCIDR="+config.machineNetworkCidr)
	}
	err := applyResourceFromTemplate(oc, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cluster *clusterDeployment) create(oc *exutil.CLI) {
	parameters := []string{"--ignore-unknown-parameters=true", "-f", cluster.template, "-p", "FAKE=" + cluster.fake, "NAME=" + cluster.name, "NAMESPACE=" + cluster.namespace, "BASEDOMAIN=" + cluster.baseDomain, "CLUSTERNAME=" + cluster.clusterName, "MANAGEDNS=" + strconv.FormatBool(cluster.manageDNS), "PLATFORMTYPE=" + cluster.platformType, "CREDREF=" + cluster.credRef, "REGION=" + cluster.region, "IMAGESETREF=" + cluster.imageSetRef, "INSTALLCONFIGSECRET=" + cluster.installConfigSecret, "PULLSECRETREF=" + cluster.pullSecretRef, "INSTALLATTEMPTSLIMIT=" + strconv.Itoa(cluster.installAttemptsLimit)}
	if len(cluster.installerType) > 0 {
		parameters = append(parameters, "INSTALLERTYPE="+cluster.installerType)
	} else {
		parameters = append(parameters, "INSTALLERTYPE=installer")
	}
	if len(cluster.customizedTag) > 0 {
		parameters = append(parameters, "CUSTOMIZEDTAG="+cluster.customizedTag)
	} else {
		parameters = append(parameters, "CUSTOMIZEDTAG="+AWSDefaultCDTag)
	}
	err := applyResourceFromTemplate(oc, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cluster *clusterDeploymentAssumeRole) create(oc *exutil.CLI) {
	parameters := []string{"--ignore-unknown-parameters=true", "-f", cluster.template, "-p", "FAKE=" + cluster.fake, "INSTALLERTYPE=" + cluster.installerType, "NAME=" + cluster.name, "NAMESPACE=" + cluster.namespace, "BASEDOMAIN=" + cluster.baseDomain, "BOUND_SERVICE_ACCOUNT_SIGNING_KEY_SECRET_REF=" + cluster.boundServiceAccountSigningKeySecretRef, "ROLEARN=" + cluster.roleARN, "EXTERNALID=" + cluster.externalID, "CLUSTERNAME=" + cluster.clusterName, "MANAGEDNS=" + strconv.FormatBool(cluster.manageDNS), "PLATFORMTYPE=" + cluster.platformType, "REGION=" + cluster.region, "MANIFESTS_SECRET_REF=" + cluster.manifestsSecretRef, "IMAGESETREF=" + cluster.imageSetRef, "INSTALLCONFIGSECRET=" + cluster.installConfigSecret, "PULLSECRETREF=" + cluster.pullSecretRef, "INSTALLATTEMPTSLIMIT=" + strconv.Itoa(cluster.installAttemptsLimit)}
	err := applyResourceFromTemplate(oc, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cluster *clusterDeploymentAdopt) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cluster.template, "-p", "NAME="+cluster.name, "NAMESPACE="+cluster.namespace, "BASEDOMAIN="+cluster.baseDomain, "ADMINKUBECONFIGREF="+cluster.adminKubeconfigRef, "CLUSTERID="+cluster.clusterID, "INFRAID="+cluster.infraID, "CLUSTERNAME="+cluster.clusterName, "MANAGEDNS="+strconv.FormatBool(cluster.manageDNS), "PLATFORMTYPE="+cluster.platformType, "CREDREF="+cluster.credRef, "REGION="+cluster.region, "PULLSECRETREF="+cluster.pullSecretRef, "PRESERVEONDELETE="+strconv.FormatBool(cluster.preserveOnDelete))
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cluster *clusterDeploymentPrivateLink) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cluster.template, "-p", "FAKE="+cluster.fake, "NAME="+cluster.name, "NAMESPACE="+cluster.namespace, "BASEDOMAIN="+cluster.baseDomain, "CLUSTERNAME="+cluster.clusterName, "MANAGEDNS="+strconv.FormatBool(cluster.manageDNS), "CREDREF="+cluster.credRef, "REGION="+cluster.region, "IMAGESETREF="+cluster.imageSetRef, "INSTALLCONFIGSECRET="+cluster.installConfigSecret, "PULLSECRETREF="+cluster.pullSecretRef, "INSTALLATTEMPTSLIMIT="+strconv.Itoa(cluster.installAttemptsLimit))
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (machine *machinepool) create(oc *exutil.CLI) {
	// Set default values
	if machine.gcpSecureBoot == "" {
		machine.gcpSecureBoot = "Disabled"
	}
	if machine.customizedTag == "" {
		machine.customizedTag = AWSDefaultMPTag
	}
	parameters := []string{"--ignore-unknown-parameters=true", "-f", machine.template, "-p", "CLUSTERNAME=" + machine.clusterName, "NAMESPACE=" + machine.namespace, "IOPS=" + strconv.Itoa(machine.iops), "AUTHENTICATION=" + machine.authentication, "SECUREBOOT=" + machine.gcpSecureBoot, "CUSTOMIZEDTAG=" + machine.customizedTag}
	if len(machine.networkProjectID) > 0 {
		parameters = append(parameters, "NETWORKPROJECTID="+machine.networkProjectID)
	}
	err := applyResourceFromTemplate(oc, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (syncresource *syncSetResource) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", syncresource.template, "-p", "NAME="+syncresource.name, "NAMESPACE="+syncresource.namespace, "CDREFNAME="+syncresource.cdrefname, "NAMESPACE2="+syncresource.namespace2, "RAMODE="+syncresource.ramode, "APPLYBEHAVIOR="+syncresource.applybehavior, "CMNAME="+syncresource.cmname, "CMNAMESPACE="+syncresource.cmnamespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (syncpatch *syncSetPatch) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", syncpatch.template, "-p", "NAME="+syncpatch.name, "NAMESPACE="+syncpatch.namespace, "CDREFNAME="+syncpatch.cdrefname, "CMNAME="+syncpatch.cmname, "CMNAMESPACE="+syncpatch.cmnamespace, "PCONTENT="+syncpatch.pcontent, "PATCHTYPE="+syncpatch.patchType)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (syncsecret *syncSetSecret) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", syncsecret.template, "-p", "NAME="+syncsecret.name, "NAMESPACE="+syncsecret.namespace, "CDREFNAME="+syncsecret.cdrefname, "SNAME="+syncsecret.sname, "SNAMESPACE="+syncsecret.snamespace, "TNAME="+syncsecret.tname, "TNAMESPACE="+syncsecret.tnamespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Azure
func (config *azureInstallConfig) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", config.template, "-p", "NAME1="+config.name1, "NAMESPACE="+config.namespace, "BASEDOMAIN="+config.baseDomain, "NAME2="+config.name2, "RESGROUP="+config.resGroup, "AZURETYPE="+config.azureType, "REGION="+config.region)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cluster *azureClusterDeployment) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cluster.template, "-p", "FAKE="+cluster.fake, "COPYCLIDOMAIN="+cluster.copyCliDomain, "NAME="+cluster.name, "NAMESPACE="+cluster.namespace, "BASEDOMAIN="+cluster.baseDomain, "CLUSTERNAME="+cluster.clusterName, "PLATFORMTYPE="+cluster.platformType, "CREDREF="+cluster.credRef, "REGION="+cluster.region, "RESGROUP="+cluster.resGroup, "AZURETYPE="+cluster.azureType, "IMAGESETREF="+cluster.imageSetRef, "INSTALLCONFIGSECRET="+cluster.installConfigSecret, "INSTALLERIMAGEOVERRIDE="+cluster.installerImageOverride, "PULLSECRETREF="+cluster.pullSecretRef)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pool *azureClusterPool) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pool.template, "-p", "NAME="+pool.name, "NAMESPACE="+pool.namespace, "FAKE="+pool.fake, "BASEDOMAIN="+pool.baseDomain, "IMAGESETREF="+pool.imageSetRef, "PLATFORMTYPE="+pool.platformType, "CREDREF="+pool.credRef, "REGION="+pool.region, "RESGROUP="+pool.resGroup, "PULLSECRETREF="+pool.pullSecretRef, "SIZE="+strconv.Itoa(pool.size), "MAXSIZE="+strconv.Itoa(pool.maxSize), "RUNNINGCOUNT="+strconv.Itoa(pool.runningCount), "MAXCONCURRENT="+strconv.Itoa(pool.maxConcurrent), "HIBERNATEAFTER="+pool.hibernateAfter)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// GCP
func (config *gcpInstallConfig) create(oc *exutil.CLI) {
	// Set default values
	if config.secureBoot == "" {
		config.secureBoot = "Disabled"
	}
	parameters := []string{"--ignore-unknown-parameters=true", "-f", config.template, "-p", "NAME1=" + config.name1, "NAMESPACE=" + config.namespace, "BASEDOMAIN=" + config.baseDomain, "NAME2=" + config.name2, "REGION=" + config.region, "PROJECTID=" + config.projectid, "SECUREBOOT=" + config.secureBoot}
	if len(config.computeSubnet) > 0 {
		parameters = append(parameters, "COMPUTESUBNET="+config.computeSubnet)
	}
	if len(config.controlPlaneSubnet) > 0 {
		parameters = append(parameters, "CONTROLPLANESUBNET="+config.controlPlaneSubnet)
	}
	if len(config.network) > 0 {
		parameters = append(parameters, "NETWORK="+config.network)
	}
	if len(config.networkProjectId) > 0 {
		parameters = append(parameters, "NETWORKPROJECTID="+config.networkProjectId)
	}
	err := applyResourceFromTemplate(oc, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cluster *gcpClusterDeployment) create(oc *exutil.CLI) {
	parameters := []string{"--ignore-unknown-parameters=true", "-f", cluster.template, "-p", "FAKE=" + cluster.fake, "NAME=" + cluster.name, "NAMESPACE=" + cluster.namespace, "BASEDOMAIN=" + cluster.baseDomain, "CLUSTERNAME=" + cluster.clusterName, "PLATFORMTYPE=" + cluster.platformType, "CREDREF=" + cluster.credRef, "REGION=" + cluster.region, "IMAGESETREF=" + cluster.imageSetRef, "INSTALLCONFIGSECRET=" + cluster.installConfigSecret, "PULLSECRETREF=" + cluster.pullSecretRef, "INSTALLERIMAGEOVERRIDE=" + cluster.installerImageOverride, "INSTALLATTEMPTSLIMIT=" + strconv.Itoa(cluster.installAttemptsLimit)}
	if len(cluster.serviceAttachmentSubnetCidr) > 0 {
		parameters = append(parameters, "SERVICEATTACHMENTSUBNETCIDR="+cluster.serviceAttachmentSubnetCidr)
	}
	err := applyResourceFromTemplate(oc, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pool *gcpClusterPool) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pool.template, "-p", "NAME="+pool.name, "NAMESPACE="+pool.namespace, "FAKE="+pool.fake, "BASEDOMAIN="+pool.baseDomain, "IMAGESETREF="+pool.imageSetRef, "PLATFORMTYPE="+pool.platformType, "CREDREF="+pool.credRef, "REGION="+pool.region, "PULLSECRETREF="+pool.pullSecretRef, "SIZE="+strconv.Itoa(pool.size), "MAXSIZE="+strconv.Itoa(pool.maxSize), "RUNNINGCOUNT="+strconv.Itoa(pool.runningCount), "MAXCONCURRENT="+strconv.Itoa(pool.maxConcurrent), "HIBERNATEAFTER="+pool.hibernateAfter)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// vSphere
func (ic *vSphereInstallConfig) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ic.template,
		"-p", "SECRETNAME="+ic.secretName, "SECRETNS="+ic.secretNs, "BASEDOMAIN="+ic.baseDomain,
		"ICNAME="+ic.icName, "MACHINENETWORK="+ic.machineNetwork, "APIVIP="+ic.apiVip, "CLUSTER="+ic.cluster,
		"DATACENTER="+ic.datacenter, "DATASTORE="+ic.datastore, "INGRESSVIP="+ic.ingressVip, "NETWORK="+ic.network,
		"PASSWORD="+ic.password, "USERNAME="+ic.username, "VCENTER="+ic.vCenter)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cluster *vSphereClusterDeployment) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cluster.template,
		"-p", "FAKE="+strconv.FormatBool(cluster.fake), "NAME="+cluster.name, "NAMESPACE="+cluster.namespace,
		"BASEDOMAIN="+cluster.baseDomain, "MANAGEDNS="+strconv.FormatBool(cluster.manageDns),
		"CLUSTERNAME="+cluster.clusterName, "CERTREF="+cluster.certRef, "CLUSTER="+cluster.cluster,
		"CREDREF="+cluster.credRef, "DATACENTER="+cluster.datacenter, "DATASTORE="+cluster.datastore,
		"NETWORK="+cluster.network, "VCENTER="+cluster.vCenter, "IMAGESETREF="+cluster.imageSetRef,
		"INSTALLCONFIGSECRET="+cluster.installConfigSecret, "PULLSECRETREF="+cluster.pullSecretRef,
		"INSTALLATTEMPTSLIMIT="+strconv.Itoa(cluster.installAttemptsLimit))
	o.Expect(err).NotTo(o.HaveOccurred())
}

// clusterDeploymentCustomize
func (cdc *ClusterDeploymentCustomization) create(oc *exutil.CLI) {
	parameters := []string{"--ignore-unknown-parameters=true", "-f", cdc.template, "-p", "NAME=" + cdc.name, "NAMESPACE=" + cdc.namespace}
	if len(cdc.region) > 0 {
		parameters = append(parameters, "REGION="+cdc.region)
	}
	err := applyResourceFromTemplate(oc, parameters...)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getResource(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) string {
	var result string
	err := wait.Poll(3*time.Second, 120*time.Second, func() (bool, error) {
		output, err := doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("the get error is %v, and try next", err)
			return false, nil
		}
		result = output
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("cat not get %v without empty", parameters))
	e2e.Logf("the result of queried resource:%v", result)
	return result
}

func doAction(oc *exutil.CLI, action string, asAdmin bool, withoutNamespace bool, parameters ...string) (string, error) {
	if asAdmin && withoutNamespace {
		return oc.AsAdmin().WithoutNamespace().Run(action).Args(parameters...).Output()
	}
	if asAdmin && !withoutNamespace {
		return oc.AsAdmin().Run(action).Args(parameters...).Output()
	}
	if !asAdmin && withoutNamespace {
		return oc.WithoutNamespace().Run(action).Args(parameters...).Output()
	}
	if !asAdmin && !withoutNamespace {
		return oc.Run(action).Args(parameters...).Output()
	}
	return "", nil
}

// Check the resource meets the expect
// parameter method: expect or present
// parameter action: get, patch, delete, ...
// parameter executor: asAdmin or not
// parameter inlineNamespace: withoutNamespace or not
// parameter expectAction: Compare or not
// parameter expectContent: expected string
// parameter expect: ok, expected to have expectContent; nok, not expected to have expectContent
// parameter timeout: use CLUSTER_INSTALL_TIMEOUT de default, and CLUSTER_INSTALL_TIMEOUT, CLUSTER_RESUME_TIMEOUT etc in different scenarios
// parameter resource: resource
func newCheck(method string, action string, executor bool, inlineNamespace bool, expectAction bool,
	expectContent string, expect bool, timeout int, resource []string) checkDescription {
	return checkDescription{
		method:          method,
		action:          action,
		executor:        executor,
		inlineNamespace: inlineNamespace,
		expectAction:    expectAction,
		expectContent:   expectContent,
		expect:          expect,
		timeout:         timeout,
		resource:        resource,
	}
}

type checkDescription struct {
	method          string
	action          string
	executor        bool
	inlineNamespace bool
	expectAction    bool
	expectContent   string
	expect          bool
	timeout         int
	resource        []string
}

const (
	asAdmin          = true
	withoutNamespace = true
	requireNS        = false
	compare          = true
	contain          = false
	present          = true
	notPresent       = false
	ok               = true
	nok              = false
)

func (ck checkDescription) check(oc *exutil.CLI) {
	switch ck.method {
	case "present":
		ok := isPresentResource(oc, ck.action, ck.executor, ck.inlineNamespace, ck.expectAction, ck.resource...)
		o.Expect(ok).To(o.BeTrue())
	case "expect":
		err := expectedResource(oc, ck.action, ck.executor, ck.inlineNamespace, ck.expectAction, ck.expectContent, ck.expect, ck.timeout, ck.resource...)
		compat_otp.AssertWaitPollNoErr(err, "can not get expected result")
	default:
		err := fmt.Errorf("unknown method")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func isPresentResource(oc *exutil.CLI, action string, asAdmin bool, withoutNamespace bool, present bool, parameters ...string) bool {
	parameters = append(parameters, "--ignore-not-found")
	err := wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
		output, err := doAction(oc, action, asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("the get error is %v, and try next", err)
			return false, nil
		}
		if !present && strings.Compare(output, "") == 0 {
			return true, nil
		}
		if present && strings.Compare(output, "") != 0 {
			return true, nil
		}
		return false, nil
	})
	return err == nil
}

func expectedResource(oc *exutil.CLI, action string, asAdmin bool, withoutNamespace bool, isCompare bool, content string, expect bool, timeout int, parameters ...string) error {
	cc := func(a, b string, ic bool) bool {
		bs := strings.Split(b, "+2+")
		ret := false
		for _, s := range bs {
			if (ic && strings.Compare(a, s) == 0) || (!ic && strings.Contains(a, s)) {
				ret = true
			}
		}
		return ret
	}
	var interval, inputTimeout time.Duration
	if timeout >= ClusterInstallTimeout {
		inputTimeout = time.Duration(timeout/60) * time.Minute
		interval = 3 * time.Minute
	} else {
		inputTimeout = time.Duration(timeout) * time.Second
		interval = time.Duration(timeout/60) * time.Second
	}
	return wait.Poll(interval, inputTimeout, func() (bool, error) {
		output, err := doAction(oc, action, asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("the get error is %v, and try next", err)
			return false, nil
		}
		e2e.Logf("the queried resource:%s", output)
		if isCompare && expect && cc(output, content, isCompare) {
			e2e.Logf("the output %s matches one of the content %s, expected", output, content)
			return true, nil
		}
		if isCompare && !expect && !cc(output, content, isCompare) {
			e2e.Logf("the output %s does not match the content %s, expected", output, content)
			return true, nil
		}
		if !isCompare && expect && cc(output, content, isCompare) {
			e2e.Logf("the output %s contains one of the content %s, expected", output, content)
			return true, nil
		}
		if !isCompare && !expect && !cc(output, content, isCompare) {
			e2e.Logf("the output %s does not contain the content %s, expected", output, content)
			return true, nil
		}
		return false, nil
	})
}

// clean up the object resource
func cleanupObjects(oc *exutil.CLI, objs ...objectTableRef) {
	for _, v := range objs {
		e2e.Logf("Start to remove: %v", v)
		//Print out debugging info if CD installed is false
		var provisionPodOutput, installedFlag string
		if v.kind == "ClusterPool" {
			if v.namespace != "" {
				cdListStr := getCDlistfromPool(oc, v.name)
				var cdArray []string
				cdArray = strings.Split(strings.TrimSpace(cdListStr), "\n")
				for i := range cdArray {
					installedFlag, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterDeployment", "-n", cdArray[i], cdArray[i], "-o=jsonpath={.spec.installed}").Output()
					if installedFlag == "false" {
						failedCdName := cdArray[i]
						e2e.Logf("failedCdName is %s", failedCdName)
						//At present, the maximum size of clusterpool in auto test is 2, we can print them all to get more information if cd installed is false
						printStatusConditions(oc, "ClusterDeployment", failedCdName, failedCdName)
						printProvisionPodLogs(oc, provisionPodOutput, failedCdName)
					}
				}
			}
		} else if v.kind == "ClusterDeployment" {
			installedFlag, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(v.kind, "-n", v.namespace, v.name, "-o=jsonpath={.spec.installed}").Output()
			if installedFlag == "false" {
				printStatusConditions(oc, v.kind, v.namespace, v.name)
				printProvisionPodLogs(oc, provisionPodOutput, v.namespace)
			}
		}
		if v.namespace != "" {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args(v.kind, "-n", v.namespace, v.name, "--ignore-not-found").Output()
		} else {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args(v.kind, v.name, "--ignore-not-found").Output()
		}
		//For ClusterPool or ClusterDeployment, need to wait ClusterDeployment delete done
		if v.kind == "ClusterPool" || v.kind == "ClusterDeployment" {
			e2e.Logf("Wait ClusterDeployment delete done for %s", v.name)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, v.name, nok, ClusterUninstallTimeout, []string{"ClusterDeployment", "-A"}).check(oc)
		}
	}
}

// print out the status conditions
func printStatusConditions(oc *exutil.CLI, kind, namespace, name string) {
	statusConditions, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(kind, "-n", namespace, name, "-o=jsonpath={.status.conditions}").Output()
	if len(statusConditions) <= LogsLimitLen {
		e2e.Logf("statusConditions is %s", statusConditions)
	} else {
		e2e.Logf("statusConditions is %s", statusConditions[:LogsLimitLen])
	}
}

// print out provision pod logs
func printProvisionPodLogs(oc *exutil.CLI, provisionPodOutput, namespace string) {
	provisionPodOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "hive.openshift.io/job-type=provision", "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	e2e.Logf("provisionPodOutput is %s", provisionPodOutput)
	//if err == nil , print out provision pod logs
	if err == nil && len(strings.TrimSpace(provisionPodOutput)) > 0 {
		var provisionPod []string
		provisionPod = strings.Split(strings.TrimSpace(provisionPodOutput), " ")
		e2e.Logf("provisionPod is %s", provisionPod)
		if len(provisionPod) > 0 {
			e2e.Logf("provisionPod len is %d. provisionPod[0] is %s", len(provisionPod), provisionPod[0])
			provisionPodLogsFile := "logs_output_" + getRandomString()[:ClusterSuffixLen] + ".txt"
			provisionPodLogs, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args(provisionPod[0], "-c", "hive", "-n", namespace).OutputToFile(provisionPodLogsFile)
			defer os.Remove(provisionPodLogs)
			failLogs, _ := exec.Command("bash", "-c", "grep -E 'level=error|level=fatal' "+provisionPodLogs).Output()
			if len(failLogs) <= LogsLimitLen {
				e2e.Logf("provisionPodLogs is %s", failLogs)
			} else {
				e2e.Logf("provisionPodLogs is %s", failLogs[len(failLogs)-LogsLimitLen:])
			}
		}
	}
}

// check if the target string is in a string slice
func ContainsInStringSlice(items []string, item string) bool {
	for _, eachItem := range items {
		if eachItem == item {
			return true
		}
	}
	return false
}

func getInfraIDFromCDName(oc *exutil.CLI, cdName string) string {
	var (
		infraID string
		err     error
	)

	getInfraIDFromCD := func() bool {
		infraID, _, err = oc.AsAdmin().Run("get").Args("cd", cdName, "-o=jsonpath={.spec.clusterMetadata.infraID}").Outputs()
		return err == nil && strings.HasPrefix(infraID, cdName)
	}
	o.Eventually(getInfraIDFromCD).WithTimeout(10 * time.Minute).WithPolling(5 * time.Second).Should(o.BeTrue())
	e2e.Logf("Found infraID = %v", infraID)
	return infraID
}

func getClusterprovisionName(oc *exutil.CLI, cdName, namespace string) string {
	var ClusterprovisionName string
	var err error
	waitForClusterprovision := func() bool {
		ClusterprovisionName, _, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterDeployment", cdName, "-n", namespace, "-o=jsonpath={.status.provisionRef.name}").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(ClusterprovisionName, cdName) {
			return true
		} else {
			return false
		}
	}
	o.Eventually(waitForClusterprovision).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())

	return ClusterprovisionName
}

func getProvisionPodNames(oc *exutil.CLI, cdName, namespace string) (provisionPodNames []string) {
	// For "kubectl get", the default sorting order is alphabetical
	stdout, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "hive.openshift.io/job-type=provision", "-l", "hive.openshift.io/cluster-deployment-name="+cdName, "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Outputs()
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, provisionPodName := range strings.Split(stdout, " ") {
		o.Expect(provisionPodName).To(o.ContainSubstring("provision"))
		o.Expect(provisionPodName).To(o.ContainSubstring(cdName))
		provisionPodNames = append(provisionPodNames, provisionPodName)
	}

	return
}

func getDeprovisionPodName(oc *exutil.CLI, cdName, namespace string) string {
	var DeprovisionPodName string
	var err error
	waitForDeprovisionPod := func() bool {
		DeprovisionPodName, _, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-l", "hive.openshift.io/job-type=deprovision", "-l", "hive.openshift.io/cluster-deployment-name="+cdName, "-n", namespace, "-o=jsonpath={.items[0].metadata.name}").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Contains(DeprovisionPodName, cdName) && strings.Contains(DeprovisionPodName, "uninstall") {
			return true
		} else {
			return false
		}
	}
	o.Eventually(waitForDeprovisionPod).WithTimeout(DefaultTimeout * time.Second).WithPolling(3 * time.Second).Should(o.BeTrue())

	return DeprovisionPodName
}

/*
Looks for targetLines in the transformed provision log stream with a timeout.
Default lineTransformation is the identity function.
Suitable for test cases for which logs can be checked before the provision is finished.

Example:

Provision logs (logStream.r's underlying data) = "foo\nbar\nbaz\nquux";
targetLines = []string{"ar", "baz", "qu"};
lineTransformation = nil;
targetLines found in provision logs -> returns true
*/
func assertLogs(logStream *os.File, targetLines []string, lineTransformation func(line string) string, timeout time.Duration) bool {
	// Set timeout (applies to future AND currently-blocked Read calls)
	endTime := time.Now().Add(timeout)
	err := logStream.SetReadDeadline(endTime)
	o.Expect(err).NotTo(o.HaveOccurred())

	// Default line transformation: the identity function
	if lineTransformation == nil {
		e2e.Logf("Using default line transformation (the identity function)")
		lineTransformation = func(line string) string { return line }
	}

	// Line scanning
	scanner := bufio.NewScanner(logStream)
	targetIdx := 0
	// In case of timeout, current & subsequent Read calls error out, resulting in scanner.Scan() returning false immediately
	for scanner.Scan() {
		switch tranformedLine, targetLine := lineTransformation(scanner.Text()), targetLines[targetIdx]; {
		// We have a match, proceed to the next target line
		case targetIdx == 0 && strings.HasSuffix(tranformedLine, targetLine) ||
			targetIdx == len(targetLines)-1 && strings.HasPrefix(tranformedLine, targetLine) ||
			tranformedLine == targetLine:
			if targetIdx++; targetIdx == len(targetLines) {
				e2e.Logf("Found substring [%v] in the logs", strings.Join(targetLines, "\n"))
				return true
			}
		// Restart from target line 0
		default:
			targetIdx = 0
		}
	}

	return false
}

func removeResource(oc *exutil.CLI, parameters ...string) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(parameters...).Output()
	if err != nil && (strings.Contains(output, "NotFound") || strings.Contains(output, "No resources found")) {
		e2e.Logf("No resource found!")
		return
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (hc *hiveconfig) delete(oc *exutil.CLI) {
	removeResource(oc, "hiveconfig", "hive")
}

// Create pull-secret in current project namespace
func createPullSecret(oc *exutil.CLI, namespace string) {
	dirname := "/tmp/" + oc.Namespace() + "-pull"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)

	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.Run("create").Args("secret", "generic", "pull-secret", "--from-file="+dirname+"/.dockerconfigjson", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create AWS credentials in current project namespace
func createAWSCreds(oc *exutil.CLI, namespace string) {
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)
	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/aws-creds", "-n", "kube-system", "--to="+dirname, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.Run("create").Args("secret", "generic", "aws-creds", "--from-file="+dirname+"/aws_access_key_id", "--from-file="+dirname+"/aws_secret_access_key", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create Route53 AWS credentials in hive namespace
func createRoute53AWSCreds(oc *exutil.CLI, secretName, namespace string) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", secretName, "-n", namespace).Output()
	if strings.Contains(output, "NotFound") || err != nil {
		e2e.Logf("No route53-aws-creds, Create it.")
		dirname := "/tmp/" + oc.Namespace() + "-route53-creds"
		err = os.MkdirAll(dirname, 0777)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/aws-creds", "-n", "kube-system", "--to="+dirname, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", secretName, "--from-file="+dirname+"/aws_access_key_id", "--from-file="+dirname+"/aws_secret_access_key", "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		e2e.Logf("route53-aws-creds already exists.")
	}
}

// Create Azure credentials in current project namespace
func createAzureCreds(oc *exutil.CLI, namespace string) {
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)

	var azureClientID, azureClientSecret, azureSubscriptionID, azureTenantID string
	azureClientID, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "--template='{{.data.azure_client_id | base64decode}}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	azureClientSecret, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "--template='{{.data.azure_client_secret | base64decode}}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	azureSubscriptionID, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "--template='{{.data.azure_subscription_id | base64decode}}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	azureTenantID, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "--template='{{.data.azure_tenant_id | base64decode}}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	//Convert credentials to osServicePrincipal.json format
	output := fmt.Sprintf("{\"subscriptionId\":\"%s\",\"clientId\":\"%s\",\"clientSecret\":\"%s\",\"tenantId\":\"%s\"}", azureSubscriptionID[1:len(azureSubscriptionID)-1], azureClientID[1:len(azureClientID)-1], azureClientSecret[1:len(azureClientSecret)-1], azureTenantID[1:len(azureTenantID)-1])
	outputFile, outputErr := os.OpenFile(dirname+"/osServicePrincipal.json", os.O_CREATE|os.O_WRONLY, 0666)
	o.Expect(outputErr).NotTo(o.HaveOccurred())
	defer outputFile.Close()
	outputWriter := bufio.NewWriter(outputFile)
	writeByte, writeError := outputWriter.WriteString(output)
	o.Expect(writeError).NotTo(o.HaveOccurred())
	writeError = outputWriter.Flush()
	o.Expect(writeError).NotTo(o.HaveOccurred())
	e2e.Logf("%d byte written to osServicePrincipal.json", writeByte)
	err = oc.Run("create").Args("secret", "generic", AzureCreds, "--from-file="+dirname+"/osServicePrincipal.json", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Create GCP credentials in current project namespace
func createGCPCreds(oc *exutil.CLI, namespace string) {
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)

	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/gcp-credentials", "-n", "kube-system", "--to="+dirname, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.Run("create").Args("secret", "generic", GCPCreds, "--from-file=osServiceAccount.json="+dirname+"/service_account.json", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func createVSphereCreds(oc *exutil.CLI, namespace, vCenter string) {
	username, password := getVSphereCredentials(oc, vCenter)
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      VSphereCreds,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"username": username,
			"password": password,
		},
	}
	_, err := oc.AdminKubeClient().CoreV1().Secrets(namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Return release version from Image
func extractRelFromImg(image string) string {
	index := strings.Index(image, ":")
	if index != -1 {
		tempStr := image[index+1:]
		index = strings.Index(tempStr, "-")
		if index != -1 {
			e2e.Logf("Extracted OCP release: %s", tempStr[:index])
			return tempStr[:index]
		}
	}
	e2e.Logf("Failed to extract OCP release from Image.")
	return ""
}

// Get CD list from Pool
// Return string CD list such as "pool-44945-2bbln5m47s\n pool-44945-f8xlv6m6s"
func getCDlistfromPool(oc *exutil.CLI, pool string) string {
	fileName := "cd_output_" + getRandomString() + ".txt"
	cdOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cd", "-A").OutputToFile(fileName)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.Remove(cdOutput)
	poolCdList, err := exec.Command("bash", "-c", "cat "+cdOutput+" | grep "+pool+" | awk '{print $1}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("CD list is %s for pool %s", poolCdList, pool)
	return string(poolCdList)
}

// Extract the kubeconfig for CD/clustername, return its path
func getClusterKubeconfig(oc *exutil.CLI, clustername, namespace, dir string) string {
	kubeconfigsecretname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cd", clustername, "-n", namespace, "-o=jsonpath={.spec.clusterMetadata.adminKubeconfigSecretRef.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Extract cluster %s kubeconfig to %s", clustername, dir)
	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/"+kubeconfigsecretname, "-n", namespace, "--to="+dir, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	kubeConfigPath := dir + "/kubeconfig"
	return kubeConfigPath
}

// Check resource number after filtering
func checkResourceNumber(oc *exutil.CLI, filterName string, resource []string) int {
	resourceOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource...).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Count(resourceOutput, filterName)
}

func getPullSecret(oc *exutil.CLI) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/pull-secret", "-n", "openshift-config", `--template={{index .data ".dockerconfigjson" | base64decode}}`).OutputToFile("auth.dockerconfigjson")
}

func getCommitID(oc *exutil.CLI, component string, clusterVersion string) (string, error) {
	secretFile, secretErr := getPullSecret(oc)
	defer os.Remove(secretFile)
	if secretErr != nil {
		return "", secretErr
	}
	outFilePath, ocErr := oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", "--registry-config="+secretFile, "--commits", clusterVersion, "--insecure=true").OutputToFile("commitIdLogs.txt")
	defer os.Remove(outFilePath)
	if ocErr != nil {
		return "", ocErr
	}
	commitID, cmdErr := exec.Command("bash", "-c", "cat "+outFilePath+" | grep "+component+" | awk '{print $3}'").Output()
	return strings.TrimSuffix(string(commitID), "\n"), cmdErr
}

func getPullSpec(oc *exutil.CLI, component string, clusterVersion string) (string, error) {
	secretFile, secretErr := getPullSecret(oc)
	defer os.Remove(secretFile)
	if secretErr != nil {
		return "", secretErr
	}
	pullSpec, ocErr := oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", "--registry-config="+secretFile, "--image-for="+component, clusterVersion, "--insecure=true").Output()
	if ocErr != nil {
		return "", ocErr
	}
	return pullSpec, nil
}

const (
	enable  = true
	disable = false
)

// Expose Hive metrics as a user-defined project
// The cluster's status of monitoring before running this function is stored for recoverability.
// *needRecoverPtr: whether recovering is needed
// *prevConfigPtr: data stored in ConfigMap/cluster-monitoring-config before running this function
func exposeMetrics(oc *exutil.CLI, testDataDir string, needRecoverPtr *bool, prevConfigPtr *string) {
	// Look for cluster-level monitoring configuration
	getOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ConfigMap", "cluster-monitoring-config", "-n", "openshift-monitoring", "--ignore-not-found").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	// Enable user workload monitoring
	if len(getOutput) > 0 {
		e2e.Logf("ConfigMap cluster-monitoring-config exists, extracting cluster-monitoring-config ...")
		extractOutput, _, _ := oc.AsAdmin().WithoutNamespace().Run("extract").Args("ConfigMap/cluster-monitoring-config", "-n", "openshift-monitoring", "--to=-").Outputs()

		if strings.Contains(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(extractOutput, "'", ""), "\"", ""), " ", ""), "enableUserWorkload:true") {
			e2e.Logf("User workload is enabled, doing nothing ... ")
			*needRecoverPtr, *prevConfigPtr = false, ""
		} else {
			e2e.Logf("User workload is not enabled, enabling ...")
			*needRecoverPtr, *prevConfigPtr = true, extractOutput

			extractOutputParts := strings.Split(extractOutput, "\n")
			containKeyword := false
			for idx, part := range extractOutputParts {
				if strings.Contains(part, "enableUserWorkload") {
					e2e.Logf("Keyword \"enableUserWorkload\" found in cluster-monitoring-config, setting enableUserWorkload to true ...")
					extractOutputParts[idx] = "enableUserWorkload: true"
					containKeyword = true
					break
				}
			}
			if !containKeyword {
				e2e.Logf("Keyword \"enableUserWorkload\" not found in cluster-monitoring-config, adding ...")
				extractOutputParts = append(extractOutputParts, "enableUserWorkload: true")
			}
			modifiedExtractOutput := strings.ReplaceAll(strings.Join(extractOutputParts, "\\n"), "\"", "\\\"")

			e2e.Logf("Patching ConfigMap cluster-monitoring-config to enable user workload monitoring ...")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("ConfigMap", "cluster-monitoring-config", "-n", "openshift-monitoring", "--type", "merge", "-p", fmt.Sprintf("{\"data\":{\"config.yaml\": \"%s\"}}", modifiedExtractOutput)).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	} else {
		e2e.Logf("ConfigMap cluster-monitoring-config does not exist, creating ...")
		*needRecoverPtr, *prevConfigPtr = true, ""

		clusterMonitoringConfigTemp := clusterMonitoringConfig{
			enableUserWorkload: true,
			namespace:          "openshift-monitoring",
			template:           filepath.Join(testDataDir, "cluster-monitoring-config.yaml"),
		}
		clusterMonitoringConfigTemp.create(oc)
	}

	// Check monitoring-related pods are created in the openshift-user-workload-monitoring namespace
	newCheck("expect", "get", asAdmin, withoutNamespace, contain, "prometheus-operator", ok, DefaultTimeout, []string{"pod", "-n", "openshift-user-workload-monitoring"}).check(oc)
	newCheck("expect", "get", asAdmin, withoutNamespace, contain, "prometheus-user-workload", ok, DefaultTimeout, []string{"pod", "-n", "openshift-user-workload-monitoring"}).check(oc)
	newCheck("expect", "get", asAdmin, withoutNamespace, contain, "thanos-ruler-user-workload", ok, DefaultTimeout, []string{"pod", "-n", "openshift-user-workload-monitoring"}).check(oc)

	// Check if ServiceMonitors and PodMonitors are created
	e2e.Logf("Checking if ServiceMonitors and PodMonitors exist ...")
	getOutput, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ServiceMonitor", "hive-clustersync", "-n", HiveNamespace, "--ignore-not-found").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	if len(getOutput) == 0 {
		e2e.Logf("Creating PodMonitor for hive-operator ...")
		podMonitorYaml := filepath.Join(testDataDir, "hive-operator-podmonitor.yaml")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", podMonitorYaml).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Creating ServiceMonitor for hive-controllers ...")
		serviceMonitorControllers := filepath.Join(testDataDir, "hive-controllers-servicemonitor.yaml")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", serviceMonitorControllers).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Creating ServiceMonitor for hive-clustersync ...")
		serviceMonitorClustersync := filepath.Join(testDataDir, "hive-clustersync-servicemonitor.yaml")
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", serviceMonitorClustersync).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// Recover cluster monitoring state, neutralizing the effect of exposeMetrics.
func recoverClusterMonitoring(oc *exutil.CLI, needRecoverPtr *bool, prevConfigPtr *string) {
	if *needRecoverPtr {
		e2e.Logf("Recovering cluster monitoring configurations ...")
		if len(*prevConfigPtr) == 0 {
			e2e.Logf("ConfigMap/cluster-monitoring-config did not exist before calling exposeMetrics, deleting ...")
			err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ConfigMap", "cluster-monitoring-config", "-n", "openshift-monitoring", "--ignore-not-found").Execute()
			if err != nil {
				e2e.Logf("Error occurred when deleting ConfigMap/cluster-monitoring-config: %v", err)
			}
		} else {
			e2e.Logf("Reverting changes made to ConfigMap/cluster-monitoring-config ...")
			*prevConfigPtr = strings.ReplaceAll(strings.ReplaceAll(*prevConfigPtr, "\n", "\\n"), "\"", "\\\"")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("ConfigMap", "cluster-monitoring-config", "-n", "openshift-monitoring", "--type", "merge", "-p", fmt.Sprintf("{\"data\":{\"config.yaml\": \"%s\"}}", *prevConfigPtr)).Execute()
			if err != nil {
				e2e.Logf("Error occurred when patching ConfigMap/cluster-monitoring-config: %v", err)
			}
		}

		e2e.Logf("Deleting ServiceMonitors and PodMonitors in the hive namespace ...")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ServiceMonitor", "hive-clustersync", "-n", HiveNamespace, "--ignore-not-found").Execute()
		if err != nil {
			e2e.Logf("Error occurred when deleting ServiceMonitor/hive-clustersync: %v", err)
		}
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("ServiceMonitor", "hive-controllers", "-n", HiveNamespace, "--ignore-not-found").Execute()
		if err != nil {
			e2e.Logf("Error occurred when deleting ServiceMonitor/hive-controllers: %v", err)
		}
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("PodMonitor", "hive-operator", "-n", HiveNamespace, "--ignore-not-found").Execute()
		if err != nil {
			e2e.Logf("Error occurred when deleting PodMonitor/hive-operator: %v", err)
		}

		return
	}

	e2e.Logf("No recovering needed for cluster monitoring configurations. ")
}

// If enable hive exportMetric
func exportMetric(oc *exutil.CLI, action bool) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HiveConfig", "hive", "-o=jsonpath={.spec.exportMetrics}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if action {
		if strings.Contains(output, "true") {
			e2e.Logf("The exportMetrics has been enabled in hiveconfig, won't change")
		} else {
			e2e.Logf("Enable hive exportMetric in Hiveconfig.")
			newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"HiveConfig", "hive", "--type", "merge", "-p", `{"spec":{"exportMetrics": true}}`}).check(oc)
			hiveNS, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("Hiveconfig", "hive", "-o=jsonpath={.spec.targetNamespace}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(hiveNS).NotTo(o.BeEmpty())
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "prometheus-k8s", ok, DefaultTimeout, []string{"role", "-n", hiveNS}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "prometheus-k8s", ok, DefaultTimeout, []string{"rolebinding", "-n", hiveNS}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-clustersync", ok, DefaultTimeout, []string{"servicemonitor", "-n", hiveNS, "-o=name"}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-controllers", ok, DefaultTimeout, []string{"servicemonitor", "-n", hiveNS, "-o=name"}).check(oc)
		}
	}
	if !action {
		if !strings.Contains(output, "true") {
			e2e.Logf("The exportMetrics has been disabled in hiveconfig, won't change")
		} else {
			e2e.Logf("Disable hive exportMetric in Hiveconfig.")
			newCheck("expect", "patch", asAdmin, withoutNamespace, contain, "patched", ok, DefaultTimeout, []string{"HiveConfig", "hive", "--type", "merge", "-p", `{"spec":{"exportMetrics": false}}`}).check(oc)
			hiveNS, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("Hiveconfig", "hive", "-o=jsonpath={.spec.targetNamespace}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(hiveNS).NotTo(o.BeEmpty())
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "prometheus-k8s", nok, DefaultTimeout, []string{"role", "-n", hiveNS}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "prometheus-k8s", nok, DefaultTimeout, []string{"rolebinding", "-n", hiveNS}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-clustersync", nok, DefaultTimeout, []string{"servicemonitor", "-n", hiveNS, "-o=name"}).check(oc)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-controllers", nok, DefaultTimeout, []string{"servicemonitor", "-n", hiveNS, "-o=name"}).check(oc)
		}
	}

}

func doPrometheusQuery(oc *exutil.CLI, token string, url string, query string) prometheusQueryResult {
	var data prometheusQueryResult
	msg, _, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(
		"-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "-i", "--",
		"curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token),
		fmt.Sprintf("%s%s", url, query)).Outputs()
	if err != nil {
		e2e.Failf("Failed Prometheus query, error: %v", err)
	}
	o.Expect(msg).NotTo(o.BeEmpty())
	json.Unmarshal([]byte(msg), &data)
	return data
}

// parameter expect: ok, expected to have expectContent; nok, not expected to have expectContent
func checkMetricExist(oc *exutil.CLI, expect bool, token string, url string, query []string) {
	for _, v := range query {
		e2e.Logf("Check metric %s", v)
		err := wait.Poll(1*time.Minute, (ClusterResumeTimeout/60)*time.Minute, func() (bool, error) {
			data := doPrometheusQuery(oc, token, url, v)
			if expect && len(data.Data.Result) > 0 {
				e2e.Logf("Metric %s exist, expected", v)
				return true, nil
			}
			if !expect && len(data.Data.Result) == 0 {
				e2e.Logf("Metric %s doesn't exist, expected", v)
				return true, nil
			}
			return false, nil

		})
		compat_otp.AssertWaitPollNoErr(err, "\"checkMetricExist\" fail, can not get expected result")
	}

}

func checkResourcesMetricValue(oc *exutil.CLI, resourceName, resourceNamespace string, expectedResult string, token string, url string, query string) {
	err := wait.Poll(1*time.Minute, (ClusterResumeTimeout/60)*time.Minute, func() (bool, error) {
		data := doPrometheusQuery(oc, token, url, query)
		for _, v := range data.Data.Result {
			switch query {
			case "hive_clusterclaim_assignment_delay_seconds_count", "hive_clusterpool_stale_clusterdeployments_deleted":
				if v.Metric.ClusterpoolName == resourceName && v.Metric.ClusterpoolNamespace == resourceNamespace {
					e2e.Logf("Found metric for pool %s in namespace %s", resourceName, resourceNamespace)
					if v.Value[1].(string) == expectedResult {
						e2e.Logf("The metric Value %s matches expected %s", v.Value[1].(string), expectedResult)
						return true, nil
					}
					e2e.Logf("The metric Value %s didn't match expected %s, try next round", v.Value[1].(string), expectedResult)
					return false, nil
				}
			case "hive_cluster_deployment_provision_underway_install_restarts":
				if v.Metric.ClusterDeployment == resourceName && v.Metric.ExportedNamespace == resourceNamespace {
					e2e.Logf("Found metric for ClusterDeployment %s in namespace %s", resourceName, resourceNamespace)
					if v.Value[1].(string) == expectedResult {
						e2e.Logf("The metric Value %s matches expected %s", v.Value[1].(string), expectedResult)
						return true, nil
					}
					e2e.Logf("The metric Value %s didn't match expected %s, try next round", v.Value[1].(string), expectedResult)
					return false, nil
				}
			case "hive_cluster_deployment_install_success_total_count":
				if v.Metric.Region == resourceName && v.Metric.Namespace == resourceNamespace {
					if data.Data.Result[0].Metric.InstallAttempt == expectedResult {
						e2e.Logf("The region %s has %s install attempts", v.Metric.Region, data.Data.Result[0].Metric.InstallAttempt)
						return true, nil
					}
					e2e.Logf("The metric InstallAttempt label %s didn't match expected %s, try next round", data.Data.Result[0].Metric.InstallAttempt, expectedResult)
					return false, nil
				}
			case "hive_cluster_deployment_install_failure_total_count":
				if v.Metric.Region == resourceName && v.Metric.Namespace == resourceNamespace {
					if data.Data.Result[2].Metric.InstallAttempt == expectedResult {
						e2e.Logf("The region %s has %s install attempts", v.Metric.Region, data.Data.Result[2].Metric.InstallAttempt)
						return true, nil
					}
					e2e.Logf("The metric InstallAttempt label %s didn't match expected %s, try next round", data.Data.Result[2].Metric.InstallAttempt, expectedResult)
					return false, nil
				}
			}
		}
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(err, "\"checkResourcesMetricValue\" fail, can not get expected result")
}

func checkHiveConfigMetric(oc *exutil.CLI, field string, expectedResult string, token string, url string, query string) {
	err := wait.Poll(1*time.Minute, (ClusterResumeTimeout/60)*time.Minute, func() (bool, error) {
		data := doPrometheusQuery(oc, token, url, query)
		switch field {
		case "condition":
			if data.Data.Result[0].Metric.Condition == expectedResult {
				e2e.Logf("the Metric %s field \"%s\" matched the expected result \"%s\"", query, field, expectedResult)
				return true, nil
			}
		case "reason":
			if data.Data.Result[0].Metric.Reason == expectedResult {
				e2e.Logf("the Metric %s field \"%s\" matched the expected result \"%s\"", query, field, expectedResult)
				return true, nil
			}
		default:
			e2e.Logf("the Metric %s doesn't contain field %s", query, field)
			return false, nil
		}
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(err, "\"checkHiveConfigMetric\" fail, can not get expected result")
}

func createCD(testDataDir string, testOCPImage string, oc *exutil.CLI, ns string, installConfigSecret interface{}, cd interface{}) {
	switch x := cd.(type) {
	case clusterDeployment:
		compat_otp.By("Create AWS ClusterDeployment..." + ns)
		imageSet := clusterImageSet{
			name:         x.name + "-imageset",
			releaseImage: testOCPImage,
			template:     filepath.Join(testDataDir, "clusterimageset.yaml"),
		}
		compat_otp.By("Create ClusterImageSet...")
		imageSet.create(oc)
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and aws-creds to target namespace for the pool
		compat_otp.By("Copy AWS platform credentials...")
		createAWSCreds(oc, ns)
		compat_otp.By("Copy pull-secret...")
		createPullSecret(oc, ns)
		compat_otp.By("Create AWS Install-Config Secret...")
		switch ic := installConfigSecret.(type) {
		case installConfig:
			ic.create(oc)
		default:
			e2e.Failf("Incorrect install-config type")
		}
		x.create(oc)
	case gcpClusterDeployment:
		compat_otp.By("Create gcp ClusterDeployment..." + ns)
		imageSet := clusterImageSet{
			name:         x.name + "-imageset",
			releaseImage: testOCPImage,
			template:     filepath.Join(testDataDir, "clusterimageset.yaml"),
		}
		compat_otp.By("Create ClusterImageSet...")
		imageSet.create(oc)
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and aws-creds to target namespace for the pool
		compat_otp.By("Copy GCP platform credentials...")
		createGCPCreds(oc, ns)
		compat_otp.By("Copy pull-secret...")
		createPullSecret(oc, ns)
		compat_otp.By("Create GCP Install-Config Secret...")
		switch ic := installConfigSecret.(type) {
		case gcpInstallConfig:
			ic.create(oc)
		default:
			e2e.Failf("Incorrect install-config type")
		}
		x.create(oc)
	case azureClusterDeployment:
		compat_otp.By("Create azure ClusterDeployment..." + ns)
		imageSet := clusterImageSet{
			name:         x.name + "-imageset",
			releaseImage: testOCPImage,
			template:     filepath.Join(testDataDir, "clusterimageset.yaml"),
		}
		compat_otp.By("Create ClusterImageSet...")
		imageSet.create(oc)
		//secrets can be accessed by pod in the same namespace, so copy pull-secret and aws-creds to target namespace for the pool
		compat_otp.By("Copy Azure platform credentials...")
		createAzureCreds(oc, ns)
		compat_otp.By("Copy pull-secret...")
		createPullSecret(oc, ns)
		compat_otp.By("Create Azure Install-Config Secret...")
		switch ic := installConfigSecret.(type) {
		case azureInstallConfig:
			ic.create(oc)
		default:
			e2e.Failf("Incorrect install-config type")
		}
		x.create(oc)
	case vSphereClusterDeployment:
		compat_otp.By("Creating vSphere ClusterDeployment in namespace: " + ns)
		imageSet := clusterImageSet{
			name:         x.name + "-imageset",
			releaseImage: testOCPImage,
			template:     filepath.Join(testDataDir, "clusterimageset.yaml"),
		}
		compat_otp.By("Creating ClusterImageSet")
		imageSet.create(oc)
		compat_otp.By("Copying vSphere platform credentials")
		createVSphereCreds(oc, ns, x.vCenter)
		compat_otp.By("Copying pull-secret")
		createPullSecret(oc, ns)
		compat_otp.By("Creating vCenter certificates Secret")
		createVsphereCertsSecret(oc, ns, x.vCenter)
		compat_otp.By("Creating vSphere Install-Config Secret")
		switch ic := installConfigSecret.(type) {
		case vSphereInstallConfig:
			ic.create(oc)
		default:
			e2e.Failf("Incorrect install-config type")
		}
		x.create(oc)
	default:
		compat_otp.By("Unknown ClusterDeployment type")
	}
}

func cleanCD(oc *exutil.CLI, clusterImageSetName string, ns string, secretName string, cdName string) {
	defer cleanupObjects(oc, objectTableRef{"ClusterImageSet", "", clusterImageSetName})
	defer cleanupObjects(oc, objectTableRef{"Secret", ns, secretName})
	defer cleanupObjects(oc, objectTableRef{"ClusterDeployment", ns, cdName})
}

// Install Hive Operator if not existent
func installHiveOperator(oc *exutil.CLI, ns *hiveNameSpace, og *operatorGroup, sub *subscription, hc *hiveconfig, testDataDir string) (string, error) {
	nsTemp := filepath.Join(testDataDir, "namespace.yaml")
	ogTemp := filepath.Join(testDataDir, "operatorgroup.yaml")
	subTemp := filepath.Join(testDataDir, "subscription.yaml")
	hcTemp := filepath.Join(testDataDir, "hiveconfig.yaml")

	*ns = hiveNameSpace{
		name:     HiveNamespace,
		template: nsTemp,
	}

	*og = operatorGroup{
		name:      "hive-og",
		namespace: HiveNamespace,
		template:  ogTemp,
	}

	*sub = subscription{
		name:            "hive-sub",
		namespace:       HiveNamespace,
		channel:         "alpha",
		approval:        "Automatic",
		operatorName:    "hive-operator",
		sourceName:      "community-operators",
		sourceNamespace: "openshift-marketplace",
		startingCSV:     "",
		currentCSV:      "",
		installedCSV:    "",
		template:        subTemp,
	}

	*hc = hiveconfig{
		logLevel:        "debug",
		targetNamespace: HiveNamespace,
		template:        hcTemp,
	}

	// Create Hive Resources if not exist
	ns.createIfNotExist(oc)
	og.createIfNotExist(oc)
	sub.createIfNotExist(oc)
	hc.createIfNotExist(oc)

	return "success", nil
}

// Get hiveadmission pod name
func getHiveadmissionPod(oc *exutil.CLI, namespace string) string {
	hiveadmissionOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector=app=hiveadmission", "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podArray := strings.Split(strings.TrimSpace(hiveadmissionOutput), " ")
	o.Expect(len(podArray)).To(o.BeNumerically(">", 0))
	e2e.Logf("Hiveadmission pod list is %s,first pod name is %s", podArray, podArray[0])
	return podArray[0]
}

// Get hivecontrollers pod name
func getHivecontrollersPod(oc *exutil.CLI, namespace string) string {
	hivecontrollersOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "--selector=control-plane=controller-manager", "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podArray := strings.Split(strings.TrimSpace(hivecontrollersOutput), " ")
	o.Expect(len(podArray)).To(o.BeNumerically(">", 0))
	e2e.Logf("Hivecontrollers pod list is %s,first pod name is %s", podArray, podArray[0])
	return podArray[0]
}

// Get OCP image for spoke cluster
// To simplify, use the OCP version same to the hive hub cluster
func getTestOCPImage(oc *exutil.CLI) string {
	testOCPImage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={..status.desired.image}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if testOCPImage == "" {
		e2e.Failf("Failed to get OCP image for the hive hub cluster")
	}
	e2e.Logf("Spoke cluster uses OCP image = %s", testOCPImage)
	return testOCPImage
}

func getCondition(oc *exutil.CLI, kind, resourceName, namespace, conditionType string) map[string]string {
	e2e.Logf("Extracting the %v condition from %v/%v in namespace %v", conditionType, kind, resourceName, namespace)
	stdout, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(kind, resourceName, "-n", namespace, fmt.Sprintf("-o=jsonpath={.status.conditions[?(@.type==\"%s\")]}", conditionType)).Outputs()
	o.Expect(err).NotTo(o.HaveOccurred())

	var condition map[string]string
	// Avoid Unmarshal failure when stdout is empty
	if len(stdout) == 0 {
		e2e.Logf("Condition %v not found on %v/%v in namespace %v", conditionType, kind, resourceName, namespace)
		return condition
	}
	err = json.Unmarshal([]byte(stdout), &condition)
	o.Expect(err).NotTo(o.HaveOccurred())

	return condition
}

func checkCondition(oc *exutil.CLI, kind, resourceName, namespace, conditionType string, expectKeyValue map[string]string, hint string) func() bool {
	e2e.Logf(hint)
	return func() bool {
		condition := getCondition(oc, kind, resourceName, namespace, conditionType)
		for key, expectValue := range expectKeyValue {
			if actualValue, ok := condition[key]; !ok || actualValue != expectValue {
				e2e.Logf("For condition %s's %s, expected value is %s, actual value is %v, retrying ...", conditionType, key, expectValue, actualValue)
				return false
			}
		}
		e2e.Logf("For condition %s, all fields checked are expected, proceeding to the next step ...", conditionType)
		return true
	}
}

// Get AWS credentials from root credentials, mount paths, and then from external configurations (in that order)
func getAWSCredentials(oc *exutil.CLI, mountPaths ...string) (AWSAccessKeyID string, AWSSecretAccessKey string) {
	err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n=kube-system").Execute()
	switch {
	// Try root credentials
	case err == nil:
		e2e.Logf("Extracting AWS credentials from root credentials")
		AWSAccessKeyID, _, err = oc.
			AsAdmin().
			WithoutNamespace().
			Run("extract").
			Args("secret/aws-creds", "-n=kube-system", "--keys=aws_access_key_id", "--to=-").
			Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		AWSSecretAccessKey, _, err = oc.
			AsAdmin().
			WithoutNamespace().
			Run("extract").
			Args("secret/aws-creds", "-n=kube-system", "--keys=aws_secret_access_key", "--to=-").
			Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
	// Try mount paths
	case len(mountPaths) > 0:
		e2e.Logf("Extracting AWS creds from credential mounts")
		e2e.Logf("Is the test running in the CI environment, targeting a non-AWS platform  ?")
		re, err := regexp.Compile(AWSCredsPattern)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, mountPath := range mountPaths {
			e2e.Logf("Extracting AWS creds from path %s", mountPath)
			fileBs, err := os.ReadFile(mountPath)
			if err != nil {
				e2e.Logf("Failed to read file: %v", err)
				continue
			}
			matches := re.FindStringSubmatch(string(fileBs))
			if len(matches) != 3 {
				e2e.Logf("Incorrect credential format")
				continue
			}
			AWSAccessKeyID = matches[1]
			AWSSecretAccessKey = matches[2]
			break
		}
	// Fall back to external configurations
	default:
		e2e.Logf("Extracting AWS creds from external configurations")
		e2e.Logf("Is the test running locally, targeting a non-AWS platform ?")
		if cfg, err := config.LoadDefaultConfig(context.Background()); err == nil {
			creds, retrieveErr := cfg.Credentials.Retrieve(context.Background())
			o.Expect(retrieveErr).NotTo(o.HaveOccurred())
			AWSAccessKeyID = creds.AccessKeyID
			AWSSecretAccessKey = creds.SecretAccessKey
		}
	}

	o.Expect(AWSAccessKeyID).NotTo(o.BeEmpty())
	o.Expect(AWSSecretAccessKey).NotTo(o.BeEmpty())
	return
}

// Extract vSphere root credentials
func getVSphereCredentials(oc *exutil.CLI, vCenter string) (username string, password string) {
	var err error
	username, _, err = oc.
		AsAdmin().
		WithoutNamespace().
		Run("extract").
		Args("secret/vsphere-creds", "-n=kube-system", fmt.Sprintf("--keys=%v.username", vCenter), "--to=-").
		Outputs()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(username).NotTo(o.BeEmpty())
	password, _, err = oc.
		AsAdmin().
		WithoutNamespace().
		Run("extract").
		Args("secret/vsphere-creds", "-n=kube-system", fmt.Sprintf("--keys=%v.password", vCenter), "--to=-").
		Outputs()
	o.Expect(err).NotTo(o.HaveOccurred())
	// This assertion fails only when the password is an empty string, so the password is never logged out.
	o.Expect(password).NotTo(o.BeEmpty())

	return
}

// getAWSConfig gets AWS-SDK-V2 configurations with static credentials for the provided region
func getAWSConfig(oc *exutil.CLI, region string, secretMountPaths ...string) aws.Config {
	AWSAccessKeyID, AWSSecretAccessKey := getAWSCredentials(oc, secretMountPaths...)
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(AWSAccessKeyID, AWSSecretAccessKey, "")),
		config.WithRegion(region),
	)
	o.Expect(err).NotTo(o.HaveOccurred())
	return cfg
}

// Customize the function to obtain a Lego DNS provider to avoid setting environment variables
// as the default implementation read from them
func newLegoDNSProvider(
	maxRetries, TTL int,
	propagationTimeout, pollingInterval time.Duration,
	accessKeyID, secretAccessKey, region string,
) (*legoroute53.DNSProvider, error) {
	legoRoute53Config := &legoroute53.Config{
		Region:             region,
		MaxRetries:         maxRetries,
		TTL:                TTL,
		PropagationTimeout: propagationTimeout,
		PollingInterval:    pollingInterval,
		AccessKeyID:        accessKeyID,
		SecretAccessKey:    secretAccessKey,
	}
	return legoroute53.NewDNSProviderConfig(legoRoute53Config)
}

// Extract hiveutil (from the latest Hive image) into dir and return the executable's path
func extractHiveutil(oc *exutil.CLI, dir string) string {
	latestImgTagStr := getLatestHiveVersion()

	e2e.Logf("Extracting hiveutil from image %v (latest) ...", latestImgTagStr)
	err := oc.
		AsAdmin().
		WithoutNamespace().
		Run("image", "extract").
		Args(fmt.Sprintf("quay.io/%s/hive:%s", HiveImgRepoOnQuay, latestImgTagStr), "--path", "/usr/bin/hiveutil:"+dir).
		Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	hiveutilPath := dir + "/hiveutil"

	e2e.Logf("Making hiveutil executable ...")
	cmd := exec.Command("chmod", "+x", hiveutilPath)
	_, err = cmd.CombinedOutput()
	o.Expect(err).NotTo(o.HaveOccurred())

	e2e.Logf("Making sure hiveutil is functional ...")
	cmd = exec.Command(hiveutilPath)
	out, err := cmd.CombinedOutput()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(string(out)).To(o.ContainSubstring("Available Commands"))
	o.Expect(string(out)).To(o.ContainSubstring("awsprivatelink"))
	return hiveutilPath
}

func getNodeNames(oc *exutil.CLI, labels map[string]string) []string {
	e2e.Logf("Extracting Node names")
	args := []string{"node"}
	for k, v := range labels {
		args = append(args, fmt.Sprintf("--selector=%s=%s", k, v))
	}
	args = append(args, "-o=jsonpath={.items[*].metadata.name}")
	stdout, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(args...).Outputs()
	o.Expect(err).NotTo(o.HaveOccurred())
	nodeNames := strings.Split(stdout, " ")
	e2e.Logf("Nodes extracted = %v", nodeNames)
	return nodeNames
}

// machinePoolName is MachinePool.spec.name
func getMachinePoolInstancesIds(oc *exutil.CLI, machinePoolName string, kubeconfigPath string) []string {
	// The command below does not error out if the selector does not have a match
	stdout, _, err := oc.AsAdmin().WithoutNamespace().Run("get").
		Args(
			"machine",
			fmt.Sprintf("--selector=machine.openshift.io/cluster-api-machine-role=%s", machinePoolName),
			"-n", "openshift-machine-api", "-o=jsonpath={.items[*].status.providerStatus.instanceId}",
			"--kubeconfig", kubeconfigPath,
		).
		Outputs()
	// When stdout is an empty string, strings.Split(stdout, " ") = []string{""}
	// We do not want this, so return an empty slice
	if len(stdout) == 0 {
		return []string{}
	}

	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.Split(stdout, " ")
}

func getBasedomain(oc *exutil.CLI) string {
	stdout, _, err := oc.
		AsAdmin().
		WithoutNamespace().
		Run("get").
		Args("dns/cluster", "-o=jsonpath={.spec.baseDomain}").
		Outputs()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(stdout).To(o.ContainSubstring("."))

	basedomain := stdout[strings.Index(stdout, ".")+1:]
	e2e.Logf("Found base domain = %v", basedomain)
	return basedomain
}

func getRegion(oc *exutil.CLI) string {
	infrastructure, err := oc.
		AdminConfigClient().
		ConfigV1().
		Infrastructures().
		Get(context.Background(), "cluster", metav1.GetOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())

	var region string
	switch platform := strings.ToLower(string(infrastructure.Status.PlatformStatus.Type)); platform {
	case "aws":
		region = infrastructure.Status.PlatformStatus.AWS.Region
	case "azure":
		region, _, err = oc.
			AsAdmin().
			WithoutNamespace().
			Run("get").
			Args("nodes", "-o=jsonpath={.items[0].metadata.labels['topology\\.kubernetes\\.io/region']}").
			Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
	case "gcp":
		region = infrastructure.Status.PlatformStatus.GCP.Region
	default:
		e2e.Failf("Unknown platform: %s", platform)
	}

	e2e.Logf("Found region = %v", region)
	return region
}

// Download root certificates for vCenter, then creates a Secret for them.
func createVsphereCertsSecret(oc *exutil.CLI, ns, vCenter string) {
	// Notes:
	// 1) As we do not necessarily have access to the vCenter URL, we'd better run commands on the ephemeral cluster.
	// 2) For some reason, /certs/download.zip might contain root certificates for a co-hosted (alias) domain.
	//    Provision will fail when this happens. As a result, we need to get an additional set of certificates
	//    with openssl, and merge those certificates into the ones obtained with wget.
	// TODO: is certificates obtained though openssl sufficient themselves (probably yes) ?
	e2e.Logf("Getting certificates from the ephemeral cluster")
	commands := fmt.Sprintf("yum install -y unzip && "+
		"wget https://%v/certs/download.zip --no-check-certificate && "+
		"unzip download.zip && "+
		"cat certs/lin/*.0 && "+
		"openssl s_client -host %v -port 443 -showcerts", vCenter, vCenter)
	// No need to recover labels set on oc.Namespace()
	err := compat_otp.SetNamespacePrivileged(oc, oc.Namespace())
	o.Expect(err).NotTo(o.HaveOccurred())
	// --to-namespace is required for the CI environment, otherwise
	// the API server will throw a "namespace XXX not found" error.
	stdout, _, err := oc.
		AsAdmin().
		WithoutNamespace().
		Run("debug").
		Args("--to-namespace", oc.Namespace(), "--", "bash", "-c", commands).
		Outputs()
	o.Expect(err).NotTo(o.HaveOccurred())
	re, err := regexp.Compile(pemX509CertPattern)
	o.Expect(err).NotTo(o.HaveOccurred())
	matches := re.FindAllStringSubmatch(stdout, -1)
	var certsSlice []string
	for _, match := range matches {
		certsSlice = append(certsSlice, match[0])
	}
	certs := strings.Join(certsSlice, "\n")

	e2e.Logf("Creating Secret containing root certificates of vCenter %v", vCenter)
	certSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      VSphereCerts,
			Namespace: ns,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			".cacert": certs,
		},
	}
	_, err = oc.AdminKubeClient().CoreV1().Secrets(ns).Create(context.Background(), certSecret, metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
}

/*
fReserve: when called, reserves an available IP for each domain from the hostedZoneName hosted zone.
IPs with the following properties can be reserved:

a) Be in the cidr block defined by cidrObj
b) Be in the IP range defined by minIp and maxIp

fRelease: when called, releases the IPs reserved in fReserve().

domain2Ip: maps domain to the IP reserved for it.
*/
func getIps2ReserveFromAWSHostedZone(oc *exutil.CLI, hostedZoneName string, cidrBlock *cidr.CIDR, minIp net.IP,
	maxIp net.IP, unavailableIps []string, awsCredsFilePath string, domains2Reserve []string) (fReserve func(),
	fRelease func(), domain2Ip map[string]string) {
	// Route 53 is global so any region will do
	var cfg aws.Config
	if awsCredsFilePath == "" {
		cfg = getAWSConfig(oc, AWSRegion)
	} else {
		cfg = getAWSConfig(oc, AWSRegion, awsCredsFilePath)
	}
	route53Client := route53.NewFromConfig(cfg)

	// Get hosted zone ID
	var hostedZoneId *string
	listHostedZonesByNameOutput, err := route53Client.ListHostedZonesByName(context.Background(),
		&route53.ListHostedZonesByNameInput{
			DNSName: aws.String(hostedZoneName),
		},
	)
	o.Expect(err).NotTo(o.HaveOccurred())
	hostedZoneFound := false
	for _, hostedZone := range listHostedZonesByNameOutput.HostedZones {
		if strings.TrimSuffix(aws.ToString(hostedZone.Name), ".") == hostedZoneName {
			hostedZoneFound = true
			hostedZoneId = hostedZone.Id
			break
		}
	}
	o.Expect(hostedZoneFound).To(o.BeTrue())
	e2e.Logf("Found hosted zone id = %v", aws.ToString(hostedZoneId))

	// Get reserved IPs in cidr
	reservedIps := sets.New[string](unavailableIps...)
	listResourceRecordSetsPaginator := route53.NewListResourceRecordSetsPaginator(
		route53Client,
		&route53.ListResourceRecordSetsInput{
			HostedZoneId: hostedZoneId,
		},
	)
	for listResourceRecordSetsPaginator.HasMorePages() {
		// Get a page of record sets
		listResourceRecordSetsOutput, listResourceRecordSetsErr := listResourceRecordSetsPaginator.NextPage(context.Background())
		o.Expect(listResourceRecordSetsErr).NotTo(o.HaveOccurred())

		// Iterate records, mark IPs which belong to the cidr block as reservedIps
		for _, recordSet := range listResourceRecordSetsOutput.ResourceRecordSets {
			for _, resourceRecord := range recordSet.ResourceRecords {
				if ip := aws.ToString(resourceRecord.Value); cidrBlock.Contains(ip) {
					reservedIps.Insert(ip)
				}
			}
		}
	}
	e2e.Logf("Found reserved IPs = %v", reservedIps.UnsortedList())

	// Get available IPs in cidr which do not exceed the range defined by minIp and maxIp
	var ips2Reserve []string
	err = cidrBlock.EachFrom(minIp.String(), func(ip string) bool {
		// Stop if IP exceeds maxIp or no more IPs to reserve
		if cidr.IPCompare(net.ParseIP(ip), maxIp) == 1 || len(ips2Reserve) == len(domains2Reserve) {
			return false
		}
		// Reserve available IP
		if !reservedIps.Has(ip) {
			ips2Reserve = append(ips2Reserve, ip)
		}
		return true
	})
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(len(domains2Reserve)).To(o.Equal(len(ips2Reserve)), "Not enough available IPs to reserve")
	e2e.Logf("IPs to reserve = %v", ips2Reserve)
	e2e.Logf("Domains to reserve = %v", domains2Reserve)

	// Get functions to reserve/release IPs
	var recordSetChanges4Reservation, recordSetChanges4Release []types.Change
	domain2Ip = make(map[string]string)
	for i, domain2Reserve := range domains2Reserve {
		ip2Reserve := ips2Reserve[i]
		domain2Ip[domain2Reserve] = ip2Reserve
		e2e.Logf("Will reserve IP %v for domain %v", ip2Reserve, domain2Reserve)
		recordSetChanges4Reservation = append(recordSetChanges4Reservation, types.Change{
			Action: types.ChangeActionCreate,
			ResourceRecordSet: &types.ResourceRecordSet{
				Name:            aws.String(domain2Reserve),
				Type:            types.RRTypeA,
				TTL:             aws.Int64(60),
				ResourceRecords: []types.ResourceRecord{{Value: aws.String(ip2Reserve)}},
			},
		})
		recordSetChanges4Release = append(recordSetChanges4Release, types.Change{
			Action: types.ChangeActionDelete,
			ResourceRecordSet: &types.ResourceRecordSet{
				Name:            aws.String(domain2Reserve),
				Type:            types.RRTypeA,
				TTL:             aws.Int64(60),
				ResourceRecords: []types.ResourceRecord{{Value: aws.String(ip2Reserve)}},
			},
		})
	}
	fReserve = func() {
		e2e.Logf("Reserving IP addresses with domain to IP injection %v", domain2Ip)
		_, reserveErr := route53Client.ChangeResourceRecordSets(
			context.Background(),
			&route53.ChangeResourceRecordSetsInput{
				HostedZoneId: hostedZoneId,
				ChangeBatch: &types.ChangeBatch{
					Changes: recordSetChanges4Reservation,
				},
			},
		)
		o.Expect(reserveErr).NotTo(o.HaveOccurred())
	}
	fRelease = func() {
		e2e.Logf("Releasing IP addresses for domains %v", domains2Reserve)
		_, releaseErr := route53Client.ChangeResourceRecordSets(
			context.Background(),
			&route53.ChangeResourceRecordSetsInput{
				HostedZoneId: hostedZoneId,
				ChangeBatch: &types.ChangeBatch{
					Changes: recordSetChanges4Release,
				},
			},
		)
		o.Expect(releaseErr).NotTo(o.HaveOccurred())
	}
	return
}

// getVSphereCIDR gets vSphere CIDR block, minimum and maximum IPs to be used for API/ingress VIPs.
func getVSphereCIDR(oc *exutil.CLI) (*cidr.CIDR, net.IP, net.IP) {
	// Extracting machine network CIDR from install-config works for different network segments,
	// including ci-vlan and devqe.
	stdout, _, err := oc.
		AsAdmin().
		WithoutNamespace().
		Run("extract").
		Args("cm/cluster-config-v1", "-n", "kube-system", "--keys", "install-config", "--to", "-").
		Outputs()
	var ic minimalInstallConfig
	o.Expect(err).NotTo(o.HaveOccurred())
	err = yaml.Unmarshal([]byte(stdout), &ic)
	o.Expect(err).NotTo(o.HaveOccurred())
	machineNetwork := ic.Networking.MachineNetwork[0].CIDR
	e2e.Logf("Found machine network segment = %v", machineNetwork)

	cidrObj, err := cidr.Parse(machineNetwork)
	o.Expect(err).NotTo(o.HaveOccurred())
	// We need another (temporary) CIDR object which will change with begin.
	cidrObjTemp, err := cidr.Parse(machineNetwork)
	o.Expect(err).NotTo(o.HaveOccurred())
	begin, end := cidrObjTemp.IPRange()
	// The first 2 IPs should not be used
	// The next 2 IPs are reserved for the Hive cluster
	// We thus skip the first 4 IPs
	minIpOffset := 4
	for i := 0; i < minIpOffset; i++ {
		cidr.IPIncr(begin)
	}
	e2e.Logf("Min IP = %v, max IP = %v", begin, end)
	return cidrObj, begin, end
}

// getVMInternalIPs gets private IPs of cloud VMs
func getVMInternalIPs(oc *exutil.CLI) []string {
	stdout, _, err := oc.
		AsAdmin().
		WithoutNamespace().
		Run("get").
		Args("node", "-o=jsonpath={.items[*].status.addresses[?(@.type==\"InternalIP\")].address}").
		Outputs()
	o.Expect(err).NotTo(o.HaveOccurred())

	return strings.Fields(stdout)
}

// Get the environment in which the test runs
func getTestEnv() (tEnv testEnv) {
	if val, ok := os.LookupEnv("OPENSHIFT_CI"); ok && val == "true" {
		tEnv = testEnvCI
	} else if _, ok := os.LookupEnv("JENKINS_HOME"); ok {
		tEnv = testEnvJenkins
	} else {
		tEnv = testEnvLocal
	}
	return
}

func getAWSCredsFilePath4VSphere(tEnv testEnv) (credsFilePath string) {
	switch tEnv {
	case testEnvCI:
		credsFilePath = VSphereAWSCredsFilePathCI
	case testEnvJenkins:
		e2e.Failf(`
VSphere test cases are meant to be tested locally (instead of on Jenkins).
In fact, an additional set of AWS credentials are required for DNS setup,
and those credentials are loaded using external AWS configurations (which
are only available locally) when running in non-CI environments.`)
	case testEnvLocal:
		// Credentials will be retrieved from external configurations using AWS tool chains when running locally.
		credsFilePath = ""
	default:
		e2e.Failf("Unknown test environment")
	}
	return credsFilePath
}

func createAssumeRolePolicyDocument(principalARN, uuid string) (string, error) {
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Principal": map[string]string{
					"AWS": principalARN,
				},
				"Action": "sts:AssumeRole",
			},
		},
	}

	if uuid != "" {
		policyStatements := policy["Statement"].([]map[string]interface{})
		policyStatements[0]["Condition"] = map[string]interface{}{
			"StringEquals": map[string]string{
				"sts:ExternalId": uuid,
			},
		}
	}

	policyJSON, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal policy: %v", err)
	}

	return string(policyJSON), nil
}

// Check if MCE is enabled
func isMCEEnabled(oc *exutil.CLI) bool {
	e2e.Logf("Checking if MCE is enabled in the cluster")
	checkMCEOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("MultiClusterEngine", "multiclusterengine-sample").Output()
	if err != nil {
		if strings.Contains(checkMCEOutput, "the server doesn't have a resource type \"MultiClusterEngine\"") {
			return false
		} else {
			e2e.Failf("Failed to check if MCE is enabled in the cluster: %v", err)
		}
	}
	return strings.Contains(checkMCEOutput, "multiclusterengine-sample")
}

// Get the latest Hive Version at HiveImgRepoOnQuay
func getLatestHiveVersion() string {
	e2e.Logf("Getting tag of the latest Hive image")

	for page := 1; page <= 20; page++ {
		e2e.Logf("Searching for hive-on-push image tag on page %d", page)

		cmd := exec.Command(
			"bash",
			"-c",
			fmt.Sprintf(
				"curl -sk 'https://quay.io/api/v1/repository/%s/hive/tag/?page=%d&limit=100' "+
					"| jq -r '{tag: (.tags | map(select(.name | startswith(\"hive-on-push\") and endswith(\"-index\"))) | sort_by(.start_ts) | reverse | .[0].name // empty), has_additional: .has_additional}'",
				HiveImgRepoOnQuay, page,
			),
		)
		output, err := cmd.CombinedOutput()
		o.Expect(err).NotTo(o.HaveOccurred())
		//If the filtered list is blank, just try to search the next page since further Unmarshal will fail
		if len(output) == 0 {
			continue
		}

		var result struct {
			Tag           string `json:"tag"`
			HasAdditional bool   `json:"has_additional"`
		}
		err = json.Unmarshal(output, &result)
		o.Expect(err).NotTo(o.HaveOccurred())

		latestImgTagStr := strings.Trim(result.Tag, "\"")
		if latestImgTagStr != "" {
			e2e.Logf("The latest Hive image version is %v", latestImgTagStr)
			return latestImgTagStr
		}

		if !result.HasAdditional {
			e2e.Logf("Reached last page, no more tags available")
			break
		}
	}

	o.Expect(fmt.Errorf("no matching hive-on-push image tag found")).NotTo(o.HaveOccurred())
	return ""
}

func getAWSCredsPathByEnv(oc *exutil.CLI) (string, string) {
	var credsDirNameForLocalTest string

	//set AWS_SHARED_CREDENTIALS_FILE from CLUSTER_PROFILE_DIR as the first priority"
	prowConfigDir, present := os.LookupEnv("CLUSTER_PROFILE_DIR")
	if present {
		awsCredFile := filepath.Join(prowConfigDir, ".awscred")
		if _, err := os.Stat(awsCredFile); err == nil {
			err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", awsCredFile)
			if err == nil {
				e2e.Logf("use CLUSTER_PROFILE_DIR/.awscred")
				awsCredsFilePath := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
				return awsCredsFilePath, credsDirNameForLocalTest
			}
		}
	}
	e2e.Logf("get AWS Creds from hub cluster")
	credsDirNameForLocalTest = "/tmp/" + oc.Namespace() + "-managed-creds"
	err := os.MkdirAll(credsDirNameForLocalTest, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/aws-creds", "-n", "kube-system", "--to="+credsDirNameForLocalTest, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	awsAccessKeyID, err := os.ReadFile(filepath.Join(credsDirNameForLocalTest, "aws_access_key_id"))
	o.Expect(err).NotTo(o.HaveOccurred())
	awsSecretAccessKey, err := os.ReadFile(filepath.Join(credsDirNameForLocalTest, "aws_secret_access_key"))
	o.Expect(err).NotTo(o.HaveOccurred())

	finalCreds := fmt.Sprintf("[default]\naws_access_key_id = %s\naws_secret_access_key = %s\n",
		strings.TrimSpace(string(awsAccessKeyID)),
		strings.TrimSpace(string(awsSecretAccessKey)),
	)

	finalCredsPath := filepath.Join(credsDirNameForLocalTest, "merged_aws_credentials")
	err = os.WriteFile(finalCredsPath, []byte(finalCreds), 0600)
	o.Expect(err).NotTo(o.HaveOccurred())

	return finalCredsPath, credsDirNameForLocalTest
}

func enableManagedDNS(oc *exutil.CLI, hiveutilPath string) {
	dirname := "/tmp/" + oc.Namespace() + "-managedns-creds"
	defer os.RemoveAll(dirname)
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	awsCredsFilePath, credsDirNameForLocalTest := getAWSCredsPathByEnv(oc)
	defer os.RemoveAll(credsDirNameForLocalTest)
	enableManageDNSCmd := fmt.Sprintf("%v adm manage-dns enable %v --cloud=aws --creds-file=%v", hiveutilPath, AWSBaseDomain, awsCredsFilePath)
	err = exec.Command("bash", "-c", enableManageDNSCmd).Run()
	o.Expect(err).NotTo(o.HaveOccurred())

	time.Sleep(5 * time.Second)
	checkManagedDNSOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hiveconfig", "hive", "-o=jsonpath={.spec.managedDomains}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(checkManagedDNSOutput).To(o.ContainSubstring(AWSBaseDomain))
}

func getGCPCredentialsOnAWSHub() string {
	info, err := os.Stat(GCPCredentialsPath)
	if err != nil {
		e2e.Failf("GCP credentials file does not exist: %v", err)
	}
	if info.IsDir() {
		e2e.Failf("GCP credentials path %s is a directory, not a file", GCPCredentialsPath)
	}

	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", GCPCredentialsPath)
	return GCPCredentialsPath
}

func getGcloudClient(oc *exutil.CLI) *compat_otp.Gcloud {
	gcpCredsFile := getGCPCredentialsOnAWSHub()
	e2e.Logf("gcp path is %v", gcpCredsFile)
	projectID := "openshift-qe"
	gcloud := compat_otp.Gcloud{ProjectID: projectID}
	return gcloud.Login()
}
