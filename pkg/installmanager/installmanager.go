package installmanager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/retry"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift/installer/pkg/destroy/aws"
	installertypes "github.com/openshift/installer/pkg/types"
)

const (
	// metadataRelativePath is the location of the installers cluster metadata file
	// relative to our WorkDir.
	metadataRelativePath                = "metadata.json"
	adminKubeConfigRelativePath         = "auth/kubeconfig"
	adminPasswordRelativePath           = "auth/kubeadmin-password"
	kubernetesKeyPrefix                 = "kubernetes.io/cluster/"
	kubeadminUsername                   = "kubeadmin"
	metadataConfigmapStringTemplate     = "%s-metadata"
	adminKubeConfigSecretStringTemplate = "%s-admin-kubeconfig"
	adminPasswordSecretStringTemplate   = "%s-admin-password"
	adminSSHKeySecretKey                = "ssh-publickey"
	installerFullLogFile                = ".openshift_install.log"
	installerConsoleLogFilePath         = "/tmp/openshift-install-console.log"
	sleepSecondsFilename                = "sleep-seconds.txt"
	hiveInstallFailureTestAnnotation    = "hive.openshift.io/install-failure-test"

	fetchLogsScriptTemplate = `#!/bin/bash
USER_ID=$(id -u)
GROUP_ID=$(id -g)

if [ x"$USER_ID" != x"0" ]; then

    echo "default:x:${USER_ID}:${GROUP_ID}:Default Application User:${HOME}:/sbin/nologin" >> /etc/passwd

fi

ssh -o "StrictHostKeyChecking=no" -A core@%s '/usr/local/bin/installer-gather.sh'
scp -o "StrictHostKeyChecking=no" core@%s:~/log-bundle.tar.gz %s`

	testFailureManifest = `apiVersion: v1
kind: NotARealSecret
metadata:
  name: foo
  namespace: bar
type: TestFailResource
`
)

var (
	// multi-line mode regex that allows removing/mutating any line containing 'password' case-insensitive
	multiLineRedactLinesWithPassword = regexp.MustCompile(`(?mi)^.*password.*$`)
)

// InstallManager coordinates executing the openshift-install binary, modifying
// generated assets, and uploading artifacts to the kube API after completion.
type InstallManager struct {
	log                           log.FieldLogger
	LogLevel                      string
	WorkDir                       string
	LogsDir                       string
	ClusterID                     string
	ClusterName                   string
	Region                        string
	ClusterDeploymentName         string
	Namespace                     string
	DynamicClient                 client.Client
	runUninstaller                func(clusterName, region, clusterID string, logger log.FieldLogger) error
	uploadClusterMetadata         func(*hivev1.ClusterDeployment, *InstallManager) error
	updateClusterDeploymentStatus func(*hivev1.ClusterDeployment, string, string, *InstallManager) error
	uploadAdminKubeconfig         func(*hivev1.ClusterDeployment, *InstallManager) (*corev1.Secret, error)
	uploadAdminPassword           func(*hivev1.ClusterDeployment, *InstallManager) (*corev1.Secret, error)
	uploadInstallerLog            func(*hivev1.ClusterDeployment, *InstallManager, error) error
	isGatherLogsEnabled           func() bool
}

// NewInstallManagerCommand is the entrypoint to create the 'install-manager' subcommand
func NewInstallManagerCommand() *cobra.Command {
	im := &InstallManager{}
	cmd := &cobra.Command{
		Use:   "install-manager NAMESPACE CLUSTER_DEPLOYMENT_NAME",
		Short: "Executes and oversees the openshift-installer.",
		Long:  "The Hive Install Manager runs the phases of the openshift-installer, edits generated assets before completing install, and monitors for artifacts that need to be uploaded back to Hive.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := im.Complete(args); err != nil {
				log.WithError(err).Error("cannot complete command")
				return
			}

			if len(args) != 2 {
				cmd.Help()
				im.log.Fatal("invalid command arguments")
			}
			// Parse the namespace/name for our cluster deployment:
			im.Namespace, im.ClusterDeploymentName = args[0], args[1]

			if err := im.Validate(); err != nil {
				log.WithError(err).Error("invalid command options")
				return
			}

			var err error
			im.DynamicClient, err = contributils.GetClient()
			if err != nil {
				im.log.WithError(err).Fatal("error creating kube clients")
			}

			if err := im.Run(); err != nil {
				log.WithError(err).Fatal("runtime error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&im.LogLevel, "log-level", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.StringVar(&im.WorkDir, "work-dir", "/output", "directory to use for all input and output")
	flags.StringVar(&im.LogsDir, "logs-dir", "/logs", "directory to use for all installer logs")
	flags.StringVar(&im.Region, "region", "us-east-1", "Region installing into")
	return cmd
}

// Complete sets remaining fields on the InstallManager based on command options and arguments.
func (m *InstallManager) Complete(args []string) error {
	// Connect up structure's function pointers
	m.uploadClusterMetadata = uploadClusterMetadata
	m.updateClusterDeploymentStatus = updateClusterDeploymentStatus
	m.uploadAdminKubeconfig = uploadAdminKubeconfig
	m.uploadAdminPassword = uploadAdminPassword
	m.uploadInstallerLog = uploadInstallerLog
	m.isGatherLogsEnabled = isGatherLogsEnabled
	m.runUninstaller = runUninstaller

	// Set log level
	level, err := log.ParseLevel(m.LogLevel)
	if err != nil {
		log.WithError(err).Error("cannot parse log level")
		return err
	}

	m.log = log.NewEntry(&log.Logger{
		Out: os.Stdout,
		Formatter: &log.TextFormatter{
			FullTimestamp: true,
		},
		Hooks: make(log.LevelHooks),
		Level: level,
	})
	// Add an installID logger field with a randomly generated string to help with debugging across multiple
	// install attempts in the namespace.
	m.log = m.log.WithField("installID", utilrand.String(8))

	absPath, err := filepath.Abs(m.WorkDir)
	if err != nil {
		m.log.WithError(err).Fatalf("error finding absolute workdir path")
	}
	m.WorkDir = absPath
	if _, err := os.Stat(m.WorkDir); os.IsNotExist(err) {
		m.log.WithField("workdir", m.WorkDir).Fatalf("workdir does not exist")
	}

	return nil
}

// Validate ensures the given options and arguments are valid.
func (m *InstallManager) Validate() error {
	return nil
}

// Run is the entrypoint to start the install process
func (m *InstallManager) Run() error {
	cd, err := m.loadClusterDeployment()
	if err != nil {
		m.log.WithError(err).Fatal("error looking up cluster deployment")
	}
	if cd.Status.Installed {
		// This should not be possible but just in-case we can somehow
		// run the install job for a cluster already installed, exit early,
		// and don't delete *anything*.
		m.log.Warn("cluster is already installed, exiting")
		os.Exit(0)
	}

	m.ClusterName = cd.Spec.ClusterName

	m.waitForInstallerBinaries()

	// Generate an install-config.yaml:
	sshKey := os.Getenv("SSH_PUB_KEY")
	pullSecret := os.Getenv("PULL_SECRET")
	m.log.Info("generating install config")
	ic, err := install.GenerateInstallConfig(cd, sshKey, pullSecret, true)
	if err != nil {
		m.log.WithError(err).Error("error generating install-config")
		return err
	}
	d, err := yaml.Marshal(ic)
	if err != nil {
		m.log.WithError(err).Error("error marshalling install-config.yaml")
		return err
	}
	err = ioutil.WriteFile(filepath.Join(m.WorkDir, "install-config.yaml"), d, 0644)
	if err != nil {
		m.log.WithError(err).Error("error writing install-config.yaml to disk")
		return err
	}

	// If the cluster deployment has a clusterID set, this implies we failed an install
	// and are re-trying. Cleanup any resources that may have been provisioned.
	m.log.Info("cleaning up from past install attempts")
	if err := m.cleanupFailedInstall(cd); err != nil {
		m.log.WithError(err).Error("error while trying to preemptively clean up")
		return err
	}

	// Generate installer assets we need to modify or upload.
	m.log.Info("generating assets")
	if err := m.generateAssets(cd); err != nil {
		if upErr := m.uploadInstallerLog(cd, m, err); upErr != nil {
			m.log.WithError(err).Error("error saving asset generation log")
		}
		m.writeSleepSecondsFile()
		return err
	}

	// We should now have cluster metadata.json we can parse for the clusterID. If we fail
	// to extract the ID and upload it, this is a critical failure and we should restart.
	// No cloud resources have been provisioned at this point.
	m.log.Info("uploading cluster metadata")
	if err := m.uploadClusterMetadata(cd, m); err != nil {
		return err
	}

	m.log.Info("provisioning cluster")
	installErr := m.provisionCluster(cd)
	if installErr != nil {
		m.log.WithError(installErr).Error("error running openshift-install, running deprovision to clean up")

		// Fetch logs from all cluster machines:
		if m.isGatherLogsEnabled() {
			_, err := m.gatherLogs(cd)
			if err != nil {
				// This is not a terminal error, we still want to cleanup and retry normally.
				m.log.WithError(err).Error("error fetching logs from cluster, proceeding with cleanup")
			}
		}

		// TODO: should we timebox this deprovision attempt in the event it gets stuck?
		if err := m.cleanupFailedInstall(cd); err != nil {
			// Log the error but continue. It is possible we were not able to clear the infraID
			// here, but we will attempt this again anyhow when the next job retries. The
			// goal here is just to minimize running resources in the event of a long wait
			// until the next retry.
			m.log.WithError(err).Error("error while trying to deprovision after failed install")
		}

		// If we failed to install, write a sleep-seconds.txt file that will be watched for and read
		// by a separate script which will do the sleep. This is done to slow down kube restarts
		// and consume as little memory as possible while doing so.
		m.log.Info("writing retries sleep seconds file")
		m.writeSleepSecondsFile()
	}

	if err := m.uploadInstallerLog(cd, m, installErr); err != nil {
		m.log.WithError(err).Error("error saving installer log")
	}

	adminKubeconfigSecret, err := m.uploadAdminKubeconfig(cd, m)
	if err != nil {
		// fatal. without admin kubeconfig we have no cluster
		m.log.WithError(err).Error("error uploading admin kubeconfig")
		return fmt.Errorf("error trying to save admin kubeconfig: %v", err)
	}

	adminPasswordSecret, err := m.uploadAdminPassword(cd, m)
	if err != nil {
		// also fatal. without admin password we have no cluster
		m.log.WithError(err).Error("error uploading admin password")
		return fmt.Errorf("error trying to save admin password: %v", err)
	}

	if err := m.updateClusterDeploymentStatus(cd, adminKubeconfigSecret.Name, adminPasswordSecret.Name, m); err != nil {
		// non-fatal. log and continue.
		// will fix up any updates to the clusterdeployment in the periodic controller
		m.log.WithError(err).Warning("error updating cluster deployment status")
	}

	if installErr != nil {
		m.log.WithError(installErr).Error("failed due to install error")
		return installErr
	}

	m.log.Info("install completed successfully")

	return nil
}

func (m *InstallManager) waitForFiles(files []string) {
	m.log.Infof("waiting for files to be available: %v", files)

	// Infinitely wait, we'll let the job terminate if we run over deadline:
	for _, p := range files {
		found := false
		for !found {
			if _, err := os.Stat(p); !os.IsNotExist(err) {
				found = true
			} else {
				time.Sleep(500 * time.Millisecond)
			}
		}
		m.log.WithField("path", p).Info("found file")
	}
	m.log.Infof("all files found, ready to proceed")
}

// writeSleepSecondsFile writes a file with a number of seconds to sleep using backoff
// and the current retry count on the cluster deployment status.
func (m *InstallManager) writeSleepSecondsFile() {
	// Refresh the cluster deployment to get accurate restart count, which can lag a little if
	// this pod restarted before the controller updated the value. Used when we write sleep seconds below.
	cd, err := m.loadClusterDeployment()
	currentRetries := cd.Status.InstallRestarts
	if err != nil {
		m.log.WithError(err).Error("ignoring error looking up cluster deployment for restart count")
		return
	}
	sleepSeconds := calcSleepSeconds(currentRetries)
	m.log.WithField("seconds", sleepSeconds).Info("calculated seconds to sleep for")
	d := []byte(strconv.Itoa(sleepSeconds))
	err = ioutil.WriteFile(filepath.Join(m.WorkDir, sleepSecondsFilename), d, 0644)
	if err != nil {
		m.log.WithError(err).Error("ignoring error writing sleep-seconds.txt")
	}
}

func calcSleepSeconds(currentRetries int) int {
	// (2^currentRetries) * 60 seconds up to a max of 24 hours.
	// In theory this could int overflow with absurd numbers of retries, but this
	// should not be a problem in the real world.
	return int(math.Min(math.Exp2(float64(currentRetries))*60, float64(60*60*24))) // max sleep of 24 hours
}

func (m *InstallManager) waitForInstallerBinaries() {
	fileList := []string{
		filepath.Join(m.WorkDir, "openshift-install"),
		filepath.Join(m.WorkDir, "oc"),
	}
	m.waitForFiles(fileList)
}

// cleanupFailedInstall allows recovering from an installation error and allows retries
func (m *InstallManager) cleanupFailedInstall(cd *hivev1.ClusterDeployment) error {
	if err := m.cleanupMetadataConfigmap(); err != nil {
		return err
	}

	if err := m.cleanupAdminKubeconfigSecret(); err != nil {
		return err
	}

	if err := m.cleanupAdminPasswordSecret(); err != nil {
		return err
	}

	if err := m.cleanupTerraformFiles(); err != nil {
		return err
	}

	if cd.Status.InfraID != "" {
		m.log.Info("InfraID set from failed install, running deprovison")
		if err := m.runUninstaller(m.ClusterName, m.Region, cd.Status.InfraID, m.log); err != nil {
			return err
		}

		m.log.Info("clear out clusterID and infraID from clusterdeployment")
		cd.Status.ClusterID = ""
		cd.Status.InfraID = ""
		err := updateClusterDeploymentStatusWithRetries(m, func(cd *hivev1.ClusterDeployment) {
			cd.Status.ClusterID = ""
			cd.Status.InfraID = ""
		})
		if err != nil {
			m.log.WithError(err).Error("error trying to update clusterdeployment status")
		}

	} else {
		m.log.Warn("skipping cleanup as no cluster ID set")
	}

	return nil
}

func (m *InstallManager) cleanupTerraformFiles() error {
	// cleanup any remnant cache files from a previous installation attempt
	deleteFiles := []string{
		"auth",
		"metadata.json",
		"terraform.tfstate",
		"terraform.tfvars",
		".openshift_install_state.json",
	}
	for _, f := range deleteFiles {
		if _, err := os.Stat(path.Join([]string{m.WorkDir, f}...)); os.IsNotExist(err) {
			// skipping non-existent file
			continue
		}

		if err := os.RemoveAll(path.Join([]string{m.WorkDir, f}...)); err != nil {
			m.log.WithError(err).Error("error while cleaning up terraform remnants")
			return err
		}
	}

	return nil
}

func runUninstaller(clusterName, region, infraID string, logger log.FieldLogger) error {
	// run the uninstaller to clean up any cloud resources previously created
	filters := []aws.Filter{
		{kubernetesKeyPrefix + infraID: "owned"},
	}
	uninstaller := &aws.ClusterUninstaller{
		Filters: filters,
		Region:  region,
		Logger:  logger,
	}

	return uninstaller.Run()
}

// generateAssets runs openshift-install commands to generate on-disk assets we need to
// upload or modify prior to provisioning resources in the cloud.
func (m *InstallManager) generateAssets(cd *hivev1.ClusterDeployment) error {

	m.log.Info("running openshift-install create manifests")
	err := m.runOpenShiftInstallCommand([]string{"create", "manifests", "--dir", m.WorkDir})
	if err != nil {
		m.log.WithError(err).Error("error generating installer assets")
		return err
	}

	// If the failure test annotation is set, write a bogus resource into the manifests which will
	// cause a late failure in the install process, enough that we can gather logs from the cluster.
	if _, ok := cd.Annotations[hiveInstallFailureTestAnnotation]; ok {
		m.log.Warnf("generating a late installation failure for testing due to %s annotation on cluster deployment", hiveInstallFailureTestAnnotation)
		data := []byte(testFailureManifest)
		err = ioutil.WriteFile(filepath.Join(m.WorkDir, "manifests", "failure-test.yaml"), data, 0644)
		if err != nil {
			m.log.WithError(err).Error("error writing failure manifest to disk")
			return err
		}
	}

	m.log.Info("running openshift-install create ignition-configs")
	err = m.runOpenShiftInstallCommand([]string{"create", "ignition-configs", "--dir", m.WorkDir})
	if err != nil {
		m.log.WithError(err).Error("error generating installer assets")
		return err
	}
	m.log.Info("assets generated successfully")
	return nil
}

// provisionCluster invokes the openshift-install create cluster command to provision resources
// in the cloud.
func (m *InstallManager) provisionCluster(cd *hivev1.ClusterDeployment) error {

	m.log.Info("running openshift-install create cluster")

	err := m.runOpenShiftInstallCommand([]string{"create", "cluster", "--dir", m.WorkDir})
	if err != nil {
		m.log.WithError(err).Errorf("error provisioning cluster")
		return err
	}
	return nil
}

func (m *InstallManager) runOpenShiftInstallCommand(args []string) error {
	cmd := exec.Command(filepath.Join(m.WorkDir, "openshift-install"), args...)

	// save the commands' stdout/stderr to a file
	stdOutAndErrOutput, err := os.Create(installerConsoleLogFilePath)
	if err != nil {
		m.log.WithError(err).Error("error creating/truncating installer console log file")
		return err
	}
	defer stdOutAndErrOutput.Close()
	cmd.Stdout = stdOutAndErrOutput
	cmd.Stderr = stdOutAndErrOutput

	err = cmd.Start()
	if err != nil {
		m.log.WithError(err).Error("error starting installer")
		return err
	}

	// 'tail -f' on the installer log file so this binary's output
	// becomes the full log of the installer
	go func() {
		logfileName := filepath.Join(m.WorkDir, installerFullLogFile)
		m.waitForFiles([]string{logfileName})

		logfile, err := os.Open(logfileName)
		defer logfile.Close()
		if err != nil {
			// FIXME what is a better response to being unable to open the file
			m.log.WithError(err).Fatalf("unable to open installer log file to display to stdout")
			panic("unable to open log file")
		}

		r := bufio.NewReader(logfile)
		fullLine := ""
		fiveMS := time.Millisecond * 5

		// this loop will store up a full line worth of text into fullLine before
		// passing through regex and then out to stdout
		//
		// NOTE: this is *not* going to catch the unlikely case where the log file contains
		// 'some leading text, pass', which we get no prefix==true, and then later the
		// file is appended to with 'word: SECRETHERE'
		for {
			line, prefix, err := r.ReadLine()
			if err != nil && err != io.EOF {
				m.log.WithError(err).Error("error reading from log file")
			}
			// pause for EOF and any other error
			if err != nil {
				time.Sleep(fiveMS)
				continue
			}

			fullLine = fmt.Sprintf("%v%v", fullLine, string(line))

			if prefix {
				// need to do another read to get to end-of-line
				continue
			}

			cleanLine := cleanupLogOutput(fullLine)
			fmt.Println(cleanLine)
			// clear out the line buffer so we can start again
			fullLine = ""
		}
	}()

	err = cmd.Wait()
	// give goroutine above a chance to read through whole buffer
	time.Sleep(time.Second)
	if err != nil {
		m.log.WithError(err).Error("error after waiting for command completion")
		return err
	}

	m.log.Info("command completed successfully")
	return nil
}

func uploadClusterMetadata(cd *hivev1.ClusterDeployment, m *InstallManager) error {
	m.log.Infoln("extracting cluster ID and uploading cluster metadata")
	fullMetadataPath := filepath.Join(m.WorkDir, metadataRelativePath)
	if _, err := os.Stat(fullMetadataPath); os.IsNotExist(err) {
		m.log.WithField("path", fullMetadataPath).Error("cluster metadata file does not exist")
		return err
	}

	metadataBytes, err := ioutil.ReadFile(fullMetadataPath)
	if err != nil {
		m.log.WithError(err).WithField("metadata", fullMetadataPath).Error("error reading cluster metadata file")
		return err
	}

	// Extract and save the cluster ID, this step is critical and a failure here
	// should abort the install. Note that this is run *before* we begin provisioning cloud
	// resources.
	var md installertypes.ClusterMetadata
	if err := json.Unmarshal(metadataBytes, &md); err != nil {
		m.log.WithError(err).Error("error unmarshalling cluster metadata")
		return err
	}
	if md.InfraID == "" {
		m.log.Error("cluster metadata did not contain infraID")
		return fmt.Errorf("cluster metadata did not contain infraID")
	}

	// Set the clusterID and infraID
	m.log.Info("setting clusterID and infraID on clusterdeployment")
	cd.Status.ClusterID = md.ClusterID
	cd.Status.InfraID = md.InfraID
	err = updateClusterDeploymentStatusWithRetries(m, func(cd *hivev1.ClusterDeployment) {
		cd.Status.ClusterID = md.ClusterID
		cd.Status.InfraID = md.InfraID

		controllerutils.FixupEmptyClusterVersionFields(&cd.Status.ClusterVersionStatus)
	})
	if err != nil {
		m.log.WithError(err).Error("error trying to update clusterdeployment status")
		return err
	}

	metadataCfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(metadataConfigmapStringTemplate, m.ClusterDeploymentName),
			Namespace: m.Namespace,
		},
		Data: map[string]string{
			"metadata.json": string(metadataBytes),
		},
	}

	if err := controllerutil.SetControllerReference(cd, metadataCfgMap, scheme.Scheme); err != nil {
		m.log.WithError(err).Error("error setting controller reference on configmap")
		return err
	}

	err = m.DynamicClient.Create(context.Background(), metadataCfgMap)
	if err != nil {
		// TODO: what should happen if the configmap already exists?
		m.log.WithError(err).Error("error creating metadata configmap")
		return err
	}
	m.log.WithField("configMapName", metadataCfgMap.Name).Info("uploaded cluster metadata configmap")

	return nil
}

func (m *InstallManager) loadClusterDeployment() (*hivev1.ClusterDeployment, error) {
	cd := &hivev1.ClusterDeployment{}
	err := m.DynamicClient.Get(context.Background(), types.NamespacedName{Namespace: m.Namespace, Name: m.ClusterDeploymentName}, cd)
	if err != nil {
		m.log.WithError(err).Error("error getting cluster deployment")
		return nil, err
	}
	return cd, nil
}

func isGatherLogsEnabled() bool {
	return os.Getenv(constants.GatherLogsEnvVar) == "true"
}

func (m *InstallManager) gatherLogs(cd *hivev1.ClusterDeployment) (string, error) {

	m.log.Info("gathering logs from cluster")

	m.log.Debug("checking for SSH private key")
	sshPrivKeyPath := os.Getenv("SSH_PRIV_KEY_PATH")
	if sshPrivKeyPath == "" {
		m.log.Warn("cannot gather logs as SSH_PRIV_KEY_PATH is unset or empty")
		return "", fmt.Errorf("cannot gather logs as SSH_PRIV_KEY_PATH is unset or empty")
	}
	fileInfo, err := os.Stat(sshPrivKeyPath)
	if err != nil && os.IsNotExist(err) {
		m.log.Warn("cannot gather logs/tarball as no ssh private key file found")
		return "", err
	} else if err != nil {
		m.log.WithError(err).Error("error stating file containing private key")
		return "", err
	}

	if fileInfo.Size() == 0 {
		m.log.Warn("cannot gather logs/tarball as ssh private key file is empty")
		return "", err
	}

	// set up ssh private key, and run the log gathering script
	cleanup, err := initSSHAgent(sshPrivKeyPath, m)
	defer cleanup()
	if err != nil {
		m.log.WithError(err).Error("failed to setup SSH agent")
		return "", err
	}

	bootstrapIP, err := getBootstrapIP(m)
	if err != nil {
		return "", err
	}

	logTarball, err := m.runGatherScript(bootstrapIP, fetchLogsScriptTemplate, m.WorkDir)
	if err != nil {
		m.log.Error("failed to run script for gathering logs")
		return "", err
	}

	return logTarball, nil
}

func (m *InstallManager) runGatherScript(bootstrapIP, scriptTemplate, workDir string) (string, error) {

	tmpFile, err := ioutil.TempFile(workDir, "gatherlog")
	if err != nil {
		m.log.WithError(err).Error("failed to create temp log gathering file")
		return "", err
	}
	defer os.Remove(tmpFile.Name())

	destTarball := filepath.Join(m.LogsDir, fmt.Sprintf("%s-log-bundle.tar.gz", time.Now().Format("20060102150405")))
	script := fmt.Sprintf(scriptTemplate, bootstrapIP, bootstrapIP, destTarball)
	m.log.Debugf("generated script: %s", script)

	if _, err := tmpFile.Write([]byte(script)); err != nil {
		m.log.WithError(err).Error("failed to write to log gathering file")
		return "", err
	}
	if err := tmpFile.Chmod(0555); err != nil {
		m.log.WithError(err).Error("failed to set script as executable")
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		m.log.WithError(err).Error("failed to close script")
		return "", err
	}

	m.log.Info("Gathering logs from bootstrap node")
	gatherCmd := exec.Command(tmpFile.Name())
	if err := gatherCmd.Run(); err != nil {
		m.log.WithError(err).Error("failed while running gather script")
		return "", err
	}

	_, err = os.Stat(destTarball)
	if err != nil {
		m.log.WithError(err).Error("error while stat-ing log tarball")
		return "", err
	}
	m.log.Infof("cluster logs gathered: %s", destTarball)

	return destTarball, nil
}

func getBootstrapIP(m *InstallManager) (string, error) {
	tfStatefile := filepath.Join(m.WorkDir, "terraform.tfstate")

	data, err := ioutil.ReadFile(tfStatefile)
	if err != nil {
		m.log.WithError(err).Error("error reading in terraform state file")
		return "", err
	}
	var tf terraformState
	if err := json.Unmarshal(data, &tf); err != nil {
		m.log.WithError(err).Error("failed to unmarshal terraform statefile json")
		return "", err
	}

	boot, found, err := unstructured.NestedString(tf.Modules["root/bootstrap"].Resources, "aws_instance.bootstrap", "primary", "attributes", "public_ip")
	if err != nil {
		m.log.WithError(err).Error("failed trying read in public IP for bootstrap instance")
		return "", err
	}
	if !found {
		msg := "failed to find public IP for bootstrap instance"
		m.log.Error(msg)
		return "", fmt.Errorf(msg)
	}
	m.log.Debugf("found bootstrap IP: %v", boot)
	return boot, nil
}

// These types and json unmarshaling are taken from the openshift installer
type terraformState struct {
	Modules map[string]terraformStateModule
}

type terraformStateModule struct {
	Resources map[string]interface{} `json:"resources"`
}

func (tfs *terraformState) UnmarshalJSON(raw []byte) error {
	var transform struct {
		Modules []struct {
			Path []string `json:"path"`
			terraformStateModule
		} `json:"modules"`
	}
	if err := json.Unmarshal(raw, &transform); err != nil {
		return err
	}
	if tfs == nil {
		tfs = &terraformState{}
	}
	if tfs.Modules == nil {
		tfs.Modules = make(map[string]terraformStateModule)
	}
	for _, m := range transform.Modules {
		tfs.Modules[strings.Join(m.Path, "/")] = terraformStateModule{Resources: m.Resources}
	}
	return nil
}

func initSSHAgent(privKeyPath string, m *InstallManager) (func(), error) {
	sshAgentCleanup := func() {}

	sock := os.Getenv("SSH_AUTH_SOCK")

	if sock == "" {
		m.log.Debug("no SSH_AUTH_SOCK defined. starting ssh-agent")
		bin, err := exec.LookPath("ssh-agent")
		if err != nil {
			m.log.WithError(err).Error("failed to find ssh-agent binary")
			return sshAgentCleanup, err
		}
		cmd := exec.Command(bin, "-s")
		out, err := cmd.Output()
		if err != nil {
			m.log.WithError(err).Error("failed to start ssh-agent")
			return sshAgentCleanup, err
		}

		fields := bytes.Split(out, []byte(";"))
		line := bytes.SplitN(fields[0], []byte("="), 2)
		line[0] = bytes.TrimLeft(line[0], "\n")
		if string(line[0]) != "SSH_AUTH_SOCK" {
			errMsg := "no SSH_AUTH_SOCK in ssh-agent output"
			m.log.Error(errMsg)
			return sshAgentCleanup, fmt.Errorf(errMsg)
		}
		sock = string(line[1])

		line = bytes.SplitN(fields[2], []byte("="), 2)
		line[0] = bytes.TrimLeft(line[0], "\n")
		if string(line[0]) != "SSH_AGENT_PID" {
			errMsg := "no SSH_AGENT_PID in ssh-agent output"
			m.log.Error(errMsg)
			return sshAgentCleanup, fmt.Errorf(errMsg)
		}
		pidStr := line[1]
		pid, err := strconv.Atoi(string(pidStr))
		if err != nil {
			errMsg := "couldn't convert SSH_AGENT_PID to string"
			m.log.Error(errMsg)
			return sshAgentCleanup, fmt.Errorf("errMsg")
		}

		os.Setenv("SSH_AUTH_SOCK", sock)
		os.Setenv("SSH_AGENT_PID", string(pidStr))

		sshAgentCleanup = func() {
			proc, _ := os.FindProcess(pid)
			if proc != nil {
				proc.Kill()
			}
			os.RemoveAll(filepath.Dir(sock))
			os.Unsetenv("SSH_AUTH_SOCK")
			os.Unsetenv("SSH_AGENT_PID")
		}
	}

	// re-adding private key if it already exists is harmless
	bin, err := exec.LookPath("ssh-add")
	if err != nil {
		m.log.WithError(err).Error("failed to find ssh-add binary")
		return sshAgentCleanup, err
	}

	cmd := exec.Command(bin, privKeyPath)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		m.log.WithError(err).Errorf("failed to add private key: %v", privKeyPath)
		return sshAgentCleanup, err
	}

	return sshAgentCleanup, nil
}

func uploadInstallerLog(cd *hivev1.ClusterDeployment, m *InstallManager, installErr error) error {
	m.log.Infoln("uploading installer output")

	if _, err := os.Stat(installerConsoleLogFilePath); os.IsNotExist(err) {
		m.log.WithField("path", installerConsoleLogFilePath).Error("installer log file does not exist")
		return err
	}

	logBytes, err := ioutil.ReadFile(installerConsoleLogFilePath)
	if err != nil {
		m.log.WithError(err).WithField("path", installerConsoleLogFilePath).Error("error reading log file")
		return err
	}

	logWithoutSensitiveData := cleanupLogOutput(string(logBytes))

	m.log.Debugf("installer console log: %v", logWithoutSensitiveData)
	success := "false"
	if installErr == nil {
		success = "true"
	}

	kubeConfigmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-install-log", m.ClusterDeploymentName),
			Namespace: m.Namespace,
			Labels: map[string]string{
				hivev1.HiveClusterDeploymentNameLabel: m.ClusterDeploymentName,
				hivev1.HiveInstallLogLabel:            "true",
			},
		},
		Data: map[string]string{
			"log":     logWithoutSensitiveData,
			"success": success,
		},
	}

	if err := controllerutil.SetControllerReference(cd, kubeConfigmap, scheme.Scheme); err != nil {
		m.log.WithError(err).Error("error setting controller reference on configmap")
		return err
	}

	return uploadConfigMapWithRetries(kubeConfigmap, m)
}

func uploadAdminKubeconfig(cd *hivev1.ClusterDeployment, m *InstallManager) (*corev1.Secret, error) {
	m.log.Infoln("uploading admin kubeconfig")
	fullPath := filepath.Join(m.WorkDir, adminKubeConfigRelativePath)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		m.log.WithField("path", fullPath).Error("admin kubeconfig file does not exist")
		return nil, err
	}

	kubeconfigBytes, err := ioutil.ReadFile(fullPath)
	if err != nil {
		m.log.WithError(err).WithField("path", fullPath).Error("error reading admin kubeconfig file")
		return nil, err
	}

	kubeconfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(adminKubeConfigSecretStringTemplate, m.ClusterDeploymentName),
			Namespace: m.Namespace,
		},
		Data: map[string][]byte{
			"kubeconfig": kubeconfigBytes,
		},
	}

	if err := controllerutil.SetControllerReference(cd, kubeconfigSecret, scheme.Scheme); err != nil {
		m.log.WithError(err).Error("error setting controller reference on kubeconfig secret")
		return nil, err
	}

	return kubeconfigSecret, uploadSecretWithRetries(kubeconfigSecret, m)
}

func uploadAdminPassword(cd *hivev1.ClusterDeployment, m *InstallManager) (*corev1.Secret, error) {
	m.log.Infoln("uploading admin username/password")
	fullPath := filepath.Join(m.WorkDir, adminPasswordRelativePath)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		m.log.WithField("path", fullPath).Error("admin password file does not exist")
		return nil, err
	}

	passwordBytes, err := ioutil.ReadFile(fullPath)
	if err != nil {
		m.log.WithError(err).WithField("path", fullPath).Error("error reading admin password file")
		return nil, err
	}

	// Need to trim trailing newlines from the password
	password := strings.TrimSpace(string(passwordBytes))

	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(adminPasswordSecretStringTemplate, m.ClusterDeploymentName),
			Namespace: m.Namespace,
		},
		Data: map[string][]byte{
			"username": []byte(kubeadminUsername),
			"password": []byte(password),
		},
	}

	if err := controllerutil.SetControllerReference(cd, s, scheme.Scheme); err != nil {
		m.log.WithError(err).Error("error setting controller reference on kubeconfig secret")
		return nil, err
	}

	return s, uploadSecretWithRetries(s, m)
}

func uploadConfigMapWithRetries(c *corev1.ConfigMap, m *InstallManager) error {
	backoff := retry.DefaultBackoff
	backoff.Steps = 10
	backoff.Duration = time.Second

	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		if err := m.DynamicClient.Delete(context.Background(), c); err != nil && !errors.IsNotFound(err) {
			m.log.WithError(err).WithField("configmap", c.Name).Warning("error deleting existing configmap")
			return false, nil
		}

		if err := m.DynamicClient.Create(context.Background(), c); err != nil {
			m.log.WithError(err).WithField("configmap", c.Name).Warning("error creating configmap")
			return false, nil
		}
		m.log.WithField("configmap", c.Name).Info("uploaded configmap")
		return true, nil
	})
	if err != nil {
		m.log.WithError(err).WithField("configmap", c.Name).Error("failed to save configmap")
		return err
	}

	return nil
}

func uploadSecretWithRetries(s *corev1.Secret, m *InstallManager) error {
	backoff := retry.DefaultBackoff
	backoff.Steps = 10
	backoff.Duration = time.Second

	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		err := m.DynamicClient.Create(context.Background(), s)
		if err != nil {
			m.log.WithError(err).WithField("secretName", s.Name).Warning("error creating secret")
			return false, nil
		}
		m.log.WithField("secretName", s.Name).Info("uploaded secret")
		return true, nil
	})
	if err != nil {
		m.log.WithError(err).WithField("secretName", s.Name).Error("failed to save secret")
		return err
	}

	return nil
}

func updateClusterDeploymentStatus(cd *hivev1.ClusterDeployment, adminKubeconfigSecretName, adminPasswordSecretName string, m *InstallManager) error {
	// Update the cluster deployment status with a reference to the admin kubeconfig secret:
	m.log.Info("updating cluster deployment status")
	err := updateClusterDeploymentStatusWithRetries(m, func(cd *hivev1.ClusterDeployment) {
		if adminKubeconfigSecretName != "" {
			cd.Status.AdminKubeconfigSecret = corev1.LocalObjectReference{Name: adminKubeconfigSecretName}
		}
		if adminPasswordSecretName != "" {
			cd.Status.AdminPasswordSecret = corev1.LocalObjectReference{Name: adminPasswordSecretName}
		}
		controllerutils.FixupEmptyClusterVersionFields(&cd.Status.ClusterVersionStatus)
	})
	if err != nil {
		m.log.WithError(err).Warn("error updating clusterdeployment status")
	}

	return err
}

func (m *InstallManager) cleanupMetadataConfigmap() error {
	// find/delete any previous metadata configmap
	namespacedName := types.NamespacedName{
		Name:      fmt.Sprintf(metadataConfigmapStringTemplate, m.ClusterDeploymentName),
		Namespace: m.Namespace,
	}
	emptyCfgMap := &corev1.ConfigMap{}
	populatedCfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
	}
	err := m.deleteAnyExistingObject(namespacedName, populatedCfgMap, emptyCfgMap)
	if err != nil {
		m.log.WithError(err).Error("failed to fetch/delete any pre-existing metadata configmap")
		return err
	}

	return nil
}

func (m *InstallManager) cleanupAdminKubeconfigSecret() error {
	// find/delete any previous admin kubeconfig secret
	namespacedName := types.NamespacedName{
		Name:      fmt.Sprintf(adminKubeConfigSecretStringTemplate, m.ClusterDeploymentName),
		Namespace: m.Namespace,
	}
	emptySecret := &corev1.Secret{}
	populatedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
	}
	err := m.deleteAnyExistingObject(namespacedName, populatedSecret, emptySecret)
	if err != nil {
		m.log.WithError(err).Error("failed to fetch/delete any pre-existing kubeconfig secret")
		return err
	}

	return nil
}

func (m *InstallManager) cleanupAdminPasswordSecret() error {
	// find/delete any previous admin password secret
	namespacedName := types.NamespacedName{
		Name:      fmt.Sprintf(adminPasswordSecretStringTemplate, m.ClusterDeploymentName),
		Namespace: m.Namespace,
	}
	emptySecret := &corev1.Secret{}
	populatedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
	}
	err := m.deleteAnyExistingObject(namespacedName, populatedSecret, emptySecret)
	if err != nil {
		m.log.WithError(err).Error("failed to fetch/delete any pre-existing admin password secret")
		return err
	}

	return nil
}

// deleteAnyExistingObject will look for any object that exists that matches the passed in 'obj' and will delete it if it exists
func (m *InstallManager) deleteAnyExistingObject(namespacedName types.NamespacedName, obj runtime.Object, emptyObj runtime.Object) error {

	err := m.DynamicClient.Get(context.Background(), namespacedName, emptyObj)
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if errors.IsNotFound(err) {
		// object doesn't exist so ignore
	} else {
		// found existing object so delete it
		m.log.Infof("deleting existing object: %v/%v", namespacedName.Namespace, namespacedName.Name)
		err = m.DynamicClient.Delete(context.Background(), obj)
		if err != nil {
			m.log.WithError(err).Errorf("error deleting object: %v/%v", namespacedName.Namespace, namespacedName.Name)
			return err
		}
	}

	return nil
}

type clusterDeploymentEditorFunc func(*hivev1.ClusterDeployment)

func updateClusterDeploymentStatusWithRetries(m *InstallManager, f clusterDeploymentEditorFunc) error {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// read in a fresh clusterDeployment
		cd, err := m.loadClusterDeployment()
		if err != nil {
			m.log.WithError(err).Warn("error reading in fresh clusterdeployment")
			return err
		}

		// make the needed modifications to the clusterDeployment
		f(cd)

		if err := m.DynamicClient.Status().Update(context.Background(), cd); err != nil {
			m.log.WithError(err).Warn("error updating clusterdeployment status")
			return err
		}

		return nil
	})
	if err != nil {
		m.log.WithError(err).Error("error trying to update clusterdeployment status")
	}
	return err
}

func cleanupLogOutput(fullLog string) string {
	var cleanedString string

	cleanedString = multiLineRedactLinesWithPassword.ReplaceAllString(fullLog, "REDACTED LINE OF OUTPUT")

	return cleanedString
}
