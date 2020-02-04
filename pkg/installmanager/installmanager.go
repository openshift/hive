package installmanager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openshift/hive/pkg/gcpclient"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/resource"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	clientwatch "k8s.io/client-go/tools/watch"
	k8slabels "k8s.io/kubernetes/pkg/util/labels"
	"k8s.io/utils/pointer"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/retry"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	azuresession "github.com/openshift/installer/pkg/asset/installconfig/azure"
	"github.com/openshift/installer/pkg/destroy/aws"
	"github.com/openshift/installer/pkg/destroy/azure"
	"github.com/openshift/installer/pkg/destroy/gcp"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesgcp "github.com/openshift/installer/pkg/types/gcp"
)

const (
	// metadataRelativePath is the location of the installers cluster metadata file
	// relative to our WorkDir.
	metadataRelativePath                = "metadata.json"
	adminKubeConfigRelativePath         = "auth/kubeconfig"
	adminPasswordRelativePath           = "auth/kubeadmin-password"
	kubernetesKeyPrefix                 = "kubernetes.io/cluster/"
	kubeadminUsername                   = "kubeadmin"
	adminKubeConfigSecretStringTemplate = "%s-admin-kubeconfig"
	adminPasswordSecretStringTemplate   = "%s-admin-password"
	adminSSHKeySecretKey                = "ssh-publickey"
	installerFullLogFile                = ".openshift_install.log"
	installerConsoleLogFilePath         = "/tmp/openshift-install-console.log"
	provisioningTransitionTimeout       = 5 * time.Minute
	sshCopyTempFile                     = "/tmp/ssh-privatekey"
	defaultInstallConfigMountPath       = "/installconfig/install-config.yaml"
	defaultPullSecretMountPath          = "/pullsecret/" + corev1.DockerConfigJsonKey
	defaultManifestsMountPath           = "/manifests"
	defaultHomeDir                      = "/home/hive" // Used if no HOME env var set.
)

var (
	// multi-line mode regex that allows removing/mutating any line containing 'password' case-insensitive
	multiLineRedactLinesWithPassword = regexp.MustCompile(`(?mi)^.*password.*$`)
)

// InstallManager coordinates executing the openshift-install binary, modifying
// generated assets, and uploading artifacts to the kube API after completion.
type InstallManager struct {
	log                      log.FieldLogger
	LogLevel                 string
	WorkDir                  string
	LogsDir                  string
	ClusterID                string
	ClusterName              string
	ClusterProvisionName     string
	Namespace                string
	InstallConfigMountPath   string
	PullSecretMountPath      string
	ManifestsMountPath       string
	DynamicClient            client.Client
	cleanupFailedProvision   func(dynamicClient client.Client, cd *hivev1.ClusterDeployment, infraID string, logger log.FieldLogger) error
	updateClusterProvision   func(*hivev1.ClusterProvision, *InstallManager, provisionMutation) error
	readClusterMetadata      func(*hivev1.ClusterProvision, *InstallManager) ([]byte, *installertypes.ClusterMetadata, error)
	uploadAdminKubeconfig    func(*hivev1.ClusterProvision, *InstallManager) (*corev1.Secret, error)
	uploadAdminPassword      func(*hivev1.ClusterProvision, *InstallManager) (*corev1.Secret, error)
	readInstallerLog         func(*hivev1.ClusterProvision, *InstallManager) (string, error)
	waitForProvisioningStage func(*hivev1.ClusterProvision, *InstallManager) error
	isGatherLogsEnabled      func() bool
}

// NewInstallManagerCommand is the entrypoint to create the 'install-manager' subcommand
func NewInstallManagerCommand() *cobra.Command {
	im := &InstallManager{}
	cmd := &cobra.Command{
		Use:   "install-manager NAMESPACE CLUSTER_PROVISION_NAME",
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
			// Parse the namespace/name for our cluster provision:
			im.Namespace, im.ClusterProvisionName = args[0], args[1]
			im.InstallConfigMountPath = defaultInstallConfigMountPath
			im.PullSecretMountPath = defaultPullSecretMountPath
			im.ManifestsMountPath = defaultManifestsMountPath

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
	return cmd
}

// Complete sets remaining fields on the InstallManager based on command options and arguments.
func (m *InstallManager) Complete(args []string) error {
	// Connect up structure's function pointers
	m.updateClusterProvision = updateClusterProvisionWithRetries
	m.readClusterMetadata = readClusterMetadata
	m.uploadAdminKubeconfig = uploadAdminKubeconfig
	m.uploadAdminPassword = uploadAdminPassword
	m.readInstallerLog = readInstallerLog
	m.isGatherLogsEnabled = isGatherLogsEnabled
	m.cleanupFailedProvision = cleanupFailedProvision
	m.waitForProvisioningStage = waitForProvisioningStage

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
	provision := &hivev1.ClusterProvision{}
	if err := m.loadClusterProvision(provision); err != nil {
		m.log.WithError(err).Fatal("error looking up cluster provision")
	}
	switch provision.Spec.Stage {
	case hivev1.ClusterProvisionStageInitializing, hivev1.ClusterProvisionStageProvisioning:
	default:
		// This should not be possible but just in-case we can somehow
		// run the install job for a cluster provision that is already complete, exit early,
		// and don't delete *anything*.
		m.log.Warnf("provision is at stage %q, exiting", provision.Spec.Stage)
		os.Exit(0)
	}
	cd, err := m.loadClusterDeployment(provision)
	if err != nil {
		m.log.WithError(err).Fatal("error looking up cluster deployment")
	}
	if cd.Spec.Installed {
		// This should not be possible but just in-case we can somehow
		// run the install job for a cluster already installed, exit early,
		// and don't delete *anything*.
		m.log.Warn("cluster is already installed, exiting")
		os.Exit(0)
	}

	// will be non-nil if we've been unable to setup SSH, which is a non-fatal error:
	var sshAgentSetupErr error

	var sshKeyPath string
	sshKeyPath, sshAgentSetupErr = m.initSSHKey()
	if sshAgentSetupErr != nil {
		// Not a fatal error.
		m.log.WithError(sshAgentSetupErr).Info("unable to initialize ssh agent")
	} else {
		sshCleanupFunc, sshAgentSetupErr := m.initSSHAgent(sshKeyPath)
		defer sshCleanupFunc()
		if sshAgentSetupErr != nil {
			// Not a fatal error.
			m.log.WithError(sshAgentSetupErr).Info("ssh agent is not initialized")
		}
	}

	m.ClusterName = cd.Spec.ClusterName

	m.waitForInstallerBinaries()

	m.log.Info("copying install-config.yaml")
	icData, err := ioutil.ReadFile(m.InstallConfigMountPath)
	if err != nil {
		m.log.WithError(err).Error("error reading install-config.yaml")
		return err
	}
	icData, err = pasteInPullSecret(icData, m.PullSecretMountPath)
	if err != nil {
		m.log.WithError(err).Error("error adding pull secret to install-config.yaml")
		return err
	}
	destInstallConfigPath := filepath.Join(m.WorkDir, "install-config.yaml")
	if err := ioutil.WriteFile(destInstallConfigPath, icData, 0644); err != nil {
		m.log.WithError(err).Error("error writing install-config.yaml")
	}
	m.log.Infof("copied %s to %s", m.InstallConfigMountPath, destInstallConfigPath)

	if cd.Spec.Provisioning != nil && len(cd.Spec.Provisioning.SSHKnownHosts) > 0 {
		err = m.setupSSHUser()
		if err != nil {
			m.log.WithError(err).Error("error setting up SSH known_hosts")
			return err
		}
		err = m.writeSSHKnownHosts(getHomeDir(), cd.Spec.Provisioning.SSHKnownHosts)
		if err != nil {
			m.log.WithError(err).Error("error setting up SSH known_hosts")
			return err
		}
	}

	// If the cluster provision has an infraID set, this implies we failed an install
	// and are re-trying. Cleanup any resources that may have been provisioned.
	m.log.Info("cleaning up from past install attempts")
	if err := m.cleanupFailedInstall(cd, provision); err != nil {
		m.log.WithError(err).Error("error while trying to preemptively clean up")
		return err
	}

	// Generate installer assets we need to modify or upload.
	m.log.Info("generating assets")
	if err := m.generateAssets(provision); err != nil {
		m.log.Info("reading installer log")
		installLog, readErr := m.readInstallerLog(provision, m)
		if readErr != nil {
			m.log.WithError(readErr).Error("error reading asset generation log")
			return err
		}
		m.log.Info("updating clusterprovision")
		if err := m.updateClusterProvision(
			provision,
			m,
			func(provision *hivev1.ClusterProvision) {
				provision.Spec.InstallLog = pointer.StringPtr(installLog)
			},
		); err != nil {
			m.log.WithError(err).Error("error updating cluster provision with asset generation log")
			return err
		}
		return err
	}

	// We should now have cluster metadata.json we can parse for the infra ID,
	// the kubeconfig, and the admin password. If we fail to read any of these or
	// to extract the infra ID and upload it, this is a critical failure and we
	// should restart. No cloud resources have been provisioned at this point.
	m.log.Info("setting cluster metadata")
	metadataBytes, metadata, err := m.readClusterMetadata(provision, m)
	if err != nil {
		m.log.WithError(err).Error("error reading cluster metadata")
		return errors.Wrap(err, "error reading cluster metadata")
	}
	kubeconfigSecret, err := m.uploadAdminKubeconfig(provision, m)
	if err != nil {
		m.log.WithError(err).Error("error uploading admin kubeconfig")
		return errors.Wrap(err, "error trying to save admin kubeconfig")
	}

	passwordSecret, err := m.uploadAdminPassword(provision, m)
	if err != nil {
		m.log.WithError(err).Error("error uploading admin password")
		return errors.Wrap(err, "error trying to save admin password")
	}
	if err := m.updateClusterProvision(
		provision,
		m,
		func(provision *hivev1.ClusterProvision) {
			provision.Spec.Metadata = &runtime.RawExtension{Raw: metadataBytes}
			provision.Spec.InfraID = pointer.StringPtr(metadata.InfraID)
			provision.Spec.ClusterID = pointer.StringPtr(metadata.ClusterID)
			provision.Spec.AdminKubeconfigSecretRef = &corev1.LocalObjectReference{
				Name: kubeconfigSecret.Name,
			}
			provision.Spec.AdminPasswordSecretRef = &corev1.LocalObjectReference{
				Name: passwordSecret.Name,
			}
		},
	); err != nil {
		m.log.WithError(err).Error("error updating cluster provision with cluster metadata")
		return errors.Wrap(err, "error updating cluster provision with cluster metadata")
	}

	m.log.Info("waiting for ClusterProvision to transition to provisioning")
	if err := m.waitForProvisioningStage(provision, m); err != nil {
		m.log.WithError(err).Error("ClusterProvision failed to transition to provisioning")
		return errors.Wrap(err, "failed to transition to provisioning")
	}

	m.log.Info("provisioning cluster")
	installErr := m.provisionCluster()
	if installErr != nil {
		m.log.WithError(installErr).Error("error running openshift-install, running deprovision to clean up")

		// Fetch logs from all cluster machines:
		if m.isGatherLogsEnabled() {
			m.gatherLogs(cd, sshKeyPath, sshAgentSetupErr)
		}

		// TODO: should we timebox this deprovision attempt in the event it gets stuck?
		if err := m.cleanupFailedInstall(cd, provision); err != nil {
			// Log the error but continue. It is possible we were not able to clear the infraID
			// here, but we will attempt this again anyhow when the next job retries. The
			// goal here is just to minimize running resources in the event of a long wait
			// until the next retry.
			m.log.WithError(err).Error("error while trying to deprovision after failed install")
		}
	}

	if installLog, err := m.readInstallerLog(provision, m); err == nil {
		if err := m.updateClusterProvision(
			provision,
			m,
			func(provision *hivev1.ClusterProvision) {
				provision.Spec.InstallLog = pointer.StringPtr(installLog)
			},
		); err != nil {
			m.log.WithError(err).Warning("error updating cluster provision with installer log")
		}
	} else {
		m.log.WithError(err).Error("error reading installer log")
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

func (m *InstallManager) waitForInstallerBinaries() {
	fileList := []string{
		filepath.Join(m.WorkDir, "openshift-install"),
		filepath.Join(m.WorkDir, "oc"),
	}
	m.waitForFiles(fileList)
}

// cleanupFailedInstall allows recovering from an installation error and allows retries
func (m *InstallManager) cleanupFailedInstall(cd *hivev1.ClusterDeployment, provision *hivev1.ClusterProvision) error {
	if err := m.cleanupAdminKubeconfigSecret(); err != nil {
		return err
	}

	if err := m.cleanupAdminPasswordSecret(); err != nil {
		return err
	}

	infraID := provision.Spec.InfraID
	if infraID == nil {
		infraID = provision.Spec.PrevInfraID
	}
	if infraID != nil {
		m.log.Info("InfraID set from failed install, running deprovison")
		if err := m.cleanupFailedProvision(m.DynamicClient, cd, *infraID, m.log); err != nil {
			return err
		}
	} else {
		m.log.Warn("skipping cleanup as no infra ID set")
	}

	return nil
}

func cleanupFailedProvision(dynClient client.Client, cd *hivev1.ClusterDeployment, infraID string, logger log.FieldLogger) error {
	switch {
	case cd.Spec.Platform.AWS != nil:
		// run the uninstaller to clean up any cloud resources previously created
		filters := []aws.Filter{
			{kubernetesKeyPrefix + infraID: "owned"},
		}
		uninstaller := &aws.ClusterUninstaller{
			Filters: filters,
			Region:  cd.Spec.Platform.AWS.Region,
			Logger:  logger,
		}

		if err := uninstaller.Run(); err != nil {
			return err
		}

		// If we're managing DNS for this cluster, lookup the DNSZone and cleanup
		// any leftover A records that may have leaked due to
		// https://jira.coreos.com/browse/CORS-1195.
		if cd.Spec.ManageDNS {
			dnsZone := &hivev1.DNSZone{}
			dnsZoneNamespacedName := types.NamespacedName{Namespace: cd.Namespace, Name: controllerutils.DNSZoneName(cd.Name)}
			err := dynClient.Get(context.TODO(), dnsZoneNamespacedName, dnsZone)
			if err != nil {
				logger.WithError(err).Error("error looking up managed dnszone")
				return err
			}
			if dnsZone.Status.AWS == nil {
				return fmt.Errorf("found non-AWS DNSZone for AWS ClusterDeployment")
			}
			if dnsZone.Status.AWS.ZoneID == nil {
				// Shouldn't really be possible as we block install until DNS is ready:
				return fmt.Errorf("DNSZone %s has no ZoneID set", dnsZone.Name)
			}
			return cleanupDNSZone(*dnsZone.Status.AWS.ZoneID, cd.Spec.Platform.AWS.Region, logger)
		}
		return nil
	case cd.Spec.Platform.Azure != nil:
		uninstaller := &azure.ClusterUninstaller{}
		uninstaller.Logger = logger
		session, err := azuresession.GetSession()
		if err != nil {
			return err
		}

		uninstaller.InfraID = infraID
		uninstaller.SubscriptionID = session.Credentials.SubscriptionID
		uninstaller.TenantID = session.Credentials.TenantID
		uninstaller.GraphAuthorizer = session.GraphAuthorizer
		uninstaller.Authorizer = session.Authorizer

		return uninstaller.Run()
	case cd.Spec.Platform.GCP != nil:
		credsFile := os.Getenv("GOOGLE_CREDENTIALS")
		projectID, err := gcpclient.ProjectIDFromFile(credsFile)
		if err != nil {
			return errors.Wrap(err, "could not get GCP project ID")
		}
		metadata := &installertypes.ClusterMetadata{
			InfraID: infraID,
			ClusterPlatformMetadata: installertypes.ClusterPlatformMetadata{
				GCP: &installertypesgcp.Metadata{
					Region:    cd.Spec.Platform.GCP.Region,
					ProjectID: projectID,
				},
			},
		}
		uninstaller, err := gcp.New(logger, metadata)
		if err != nil {
			return err
		}
		return uninstaller.Run()
	default:
		logger.Warn("unknown platform for re-try cleanup")
		return errors.New("unknown platform for re-try cleanup")
	}
}

// generateAssets runs openshift-install commands to generate on-disk assets we need to
// upload or modify prior to provisioning resources in the cloud.
func (m *InstallManager) generateAssets(provision *hivev1.ClusterProvision) error {

	m.log.Info("running openshift-install create manifests")
	err := m.runOpenShiftInstallCommand("create", "manifests")
	if err != nil {
		m.log.WithError(err).Error("error generating installer assets")
		return err
	}

	if src := m.ManifestsMountPath; isDirNonEmpty(src) {
		m.log.Info("copying user-provided manifests")
		dest := filepath.Join(m.WorkDir, "manifests")
		out, err := exec.Command("bash", "-c", fmt.Sprintf("cp %s %s", filepath.Join(src, "*"), dest)).CombinedOutput()
		fmt.Printf("%s\n", out)
		if err != nil {
			log.WithError(err).Errorf("error copying manifests from %s to %s", src, dest)
			return err
		}
		m.log.Infof("copied %s to %s", src, dest)
	}

	m.log.Info("running openshift-install create ignition-configs")
	if err := m.runOpenShiftInstallCommand("create", "ignition-configs"); err != nil {
		m.log.WithError(err).Error("error generating installer assets")
		return err
	}
	m.log.Info("assets generated successfully")
	return nil
}

// provisionCluster invokes the openshift-install create cluster command to provision resources
// in the cloud.
func (m *InstallManager) provisionCluster() error {

	m.log.Info("running openshift-install create cluster")

	if err := m.runOpenShiftInstallCommand("create", "cluster"); err != nil {
		if m.isBootstrapComplete() {
			m.log.WithError(err).Warn("provisioning cluster failed after completing bootstrapping, waiting longer for install to complete")
			err = m.runOpenShiftInstallCommand("wait-for", "install-complete")
		}
		if err != nil {
			m.log.WithError(err).Error("error provisioning cluster")
			return err
		}
	}
	return nil
}

func (m *InstallManager) runOpenShiftInstallCommand(args ...string) error {
	m.log.WithField("args", args).Info("running openshift-install binary")
	cmd := exec.Command("./openshift-install", args...)
	cmd.Dir = m.WorkDir

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

func readClusterMetadata(provision *hivev1.ClusterProvision, m *InstallManager) ([]byte, *installertypes.ClusterMetadata, error) {
	m.log.Infoln("extracting cluster ID and uploading cluster metadata")
	fullMetadataPath := filepath.Join(m.WorkDir, metadataRelativePath)
	if _, err := os.Stat(fullMetadataPath); os.IsNotExist(err) {
		m.log.WithField("path", fullMetadataPath).Error("cluster metadata file does not exist")
		return nil, nil, err
	}

	metadataBytes, err := ioutil.ReadFile(fullMetadataPath)
	if err != nil {
		m.log.WithError(err).WithField("metadata", fullMetadataPath).Error("error reading cluster metadata file")
		return nil, nil, err
	}

	// Extract and save the cluster ID, this step is critical and a failure here
	// should abort the install. Note that this is run *before* we begin provisioning cloud
	// resources.
	md := &installertypes.ClusterMetadata{}
	if err := json.Unmarshal(metadataBytes, md); err != nil {
		m.log.WithError(err).Error("error unmarshalling cluster metadata")
		return nil, nil, err
	}
	if md.InfraID == "" {
		m.log.Error("cluster metadata did not contain infraID")
		return nil, nil, errors.New("cluster metadata did not contain infraID")
	}

	return metadataBytes, md, nil
}

func (m *InstallManager) loadClusterProvision(provision *hivev1.ClusterProvision) error {
	if err := m.DynamicClient.Get(context.TODO(), types.NamespacedName{Namespace: m.Namespace, Name: m.ClusterProvisionName}, provision); err != nil {
		m.log.WithError(err).Error("error getting cluster provision")
		return err
	}
	return nil
}

func (m *InstallManager) loadClusterDeployment(provision *hivev1.ClusterProvision) (*hivev1.ClusterDeployment, error) {
	cd := &hivev1.ClusterDeployment{}
	if err := m.DynamicClient.Get(context.Background(), types.NamespacedName{Namespace: m.Namespace, Name: provision.Spec.ClusterDeploymentRef.Name}, cd); err != nil {
		m.log.WithError(err).Error("error getting cluster deployment")
		return nil, err
	}
	return cd, nil
}

func isGatherLogsEnabled() bool {
	// By default we assume to gather logs, only disable if explicitly told to via HiveConfig.
	envVarValue := os.Getenv(constants.SkipGatherLogsEnvVar)
	return envVarValue != "true"
}

// gatherLogs will attempt to gather logs after a failed install. First we attempt
// to gather logs from the bootstrap node. If this fails, we may have made it far enough
// to teardown the bootstrap node, in which case we then attempt to gather with
// 'oc adm must-gather', which would gather logs from the cluster's API itself.
// If neither succeeds we do not consider this a fatal error,
// we're just gathering as much information as we can and then proceeding with cleanup
// so we can re-try.
func (m *InstallManager) gatherLogs(cd *hivev1.ClusterDeployment, sshPrivKeyPath string, sshAgentSetupErr error) {
	if !m.isBootstrapComplete() {
		if sshAgentSetupErr != nil {
			m.log.Warn("unable to fetch logs from bootstrap node as SSH agent was not configured")
			return
		}
		if err := m.gatherBootstrapNodeLogs(cd, sshPrivKeyPath); err != nil {
			m.log.WithError(err).Warn("error fetching logs from bootstrap node")
			return
		}
		m.log.Info("successfully gathered logs from bootstrap node")
	} else {
		if err := m.gatherClusterLogs(cd); err != nil {
			m.log.WithError(err).Warn("error fetching logs with oc adm must-gather")
			return
		}
		m.log.Info("successfully ran oc adm must-gather")
	}
}

func (m *InstallManager) gatherClusterLogs(cd *hivev1.ClusterDeployment) error {
	m.log.Info("attempting to gather logs with oc adm must-gather")
	destDir := filepath.Join(m.LogsDir, fmt.Sprintf("%s-must-gather", time.Now().Format("20060102150405")))
	cmd := exec.Command(filepath.Join(m.WorkDir, "oc"), "adm", "must-gather", "--dest-dir", destDir)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("KUBECONFIG=%s", filepath.Join(m.WorkDir, "auth", "kubeconfig")))
	stdout, err := cmd.Output()
	m.log.Infof("must-gather output: %s", stdout)
	return err
}

func (m *InstallManager) initSSHKey() (string, error) {
	m.log.Debug("checking for SSH private key")
	sshPrivKeyPath := os.Getenv("SSH_PRIV_KEY_PATH")
	if sshPrivKeyPath == "" {
		m.log.Warn("cannot configure SSH agent as SSH_PRIV_KEY_PATH is unset or empty")
		return "", fmt.Errorf("cannot configure SSH agent as SSH_PRIV_KEY_PATH is unset or empty")
	}
	fileInfo, err := os.Stat(sshPrivKeyPath)
	if err != nil && os.IsNotExist(err) {
		m.log.WithField("path", sshPrivKeyPath).Warn("SSH_PRIV_KEY_PATH defined but file does not exist")
		return "", err
	} else if err != nil {
		m.log.WithError(err).Error("error stat'ing file containing private key")
		return "", err
	} else if fileInfo.Size() == 0 {
		m.log.Warn("cannot initialize SSH as the private key file is empty")
		return "", errors.New("cannot initialize SSH as the private key file is empty")
	}

	// copy the mounted volume with the ssh private key to
	// a temporary file, which allows us to chmod it 0600 to appease ssh-add.
	input, err := ioutil.ReadFile(sshPrivKeyPath)
	if err != nil {
		m.log.WithError(err).Error("error reading ssh private key to copy")
		return "", err
	}

	if err := ioutil.WriteFile(sshCopyTempFile, input, 0600); err != nil {
		m.log.WithError(err).Error("error writing copy of ssh private key")
		return "", err
	}

	return sshCopyTempFile, nil

}

func (m *InstallManager) initSSHAgent(sshKeyPath string) (func(), error) {
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

	cmd := exec.Command(bin, sshKeyPath)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		m.log.WithError(err).Errorf("failed to add private key: %v", sshKeyPath)
		return sshAgentCleanup, err
	}

	return sshAgentCleanup, nil
}

func readInstallerLog(provision *hivev1.ClusterProvision, m *InstallManager) (string, error) {
	m.log.Infoln("saving installer output")

	if _, err := os.Stat(installerConsoleLogFilePath); os.IsNotExist(err) {
		m.log.WithField("path", installerConsoleLogFilePath).Error("installer log file does not exist")
		return "", err
	}

	logBytes, err := ioutil.ReadFile(installerConsoleLogFilePath)
	if err != nil {
		m.log.WithError(err).WithField("path", installerConsoleLogFilePath).Error("error reading log file")
		return "", err
	}

	logWithoutSensitiveData := cleanupLogOutput(string(logBytes))

	m.log.Debugf("installer console log: %v", logWithoutSensitiveData)

	return logWithoutSensitiveData, nil
}

func (m *InstallManager) gatherBootstrapNodeLogs(cd *hivev1.ClusterDeployment, newSSHPrivKeyPath string) error {

	m.log.Info("attempting to gather logs with 'openshift-install gather bootstrap'")
	err := m.runOpenShiftInstallCommand("gather", "bootstrap", "--key", newSSHPrivKeyPath)
	if err != nil {
		m.log.WithError(err).Error("failed to gather logs from bootstrap node")
		return err
	}

	m.log.Infof("copying log bundles from %s to %s", m.WorkDir, m.LogsDir)
	logBundles, err := filepath.Glob(filepath.Join(m.WorkDir, "log-bundle-*.tar.gz"))
	if err != nil {
		m.log.WithError(err).Error("erroring globbing log bundles")
		return err
	}
	for _, lb := range logBundles {
		// Using mv here rather than reading them into memory to write them out again.
		cmd := exec.Command("mv", lb, m.LogsDir)
		err = cmd.Run()
		if err != nil {
			log.WithError(err).Errorf("error moving file %s", lb)
			return err
		}
		m.log.Infof("moved %s to %s", lb, m.LogsDir)
	}
	m.log.Info("bootstrap node log gathering complete")

	return nil
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

func (m *InstallManager) isBootstrapComplete() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "./openshift-install", "wait-for", "bootstrap-complete")
	cmd.Dir = m.WorkDir
	return cmd.Run() == nil
}

func uploadAdminKubeconfig(provision *hivev1.ClusterProvision, m *InstallManager) (*corev1.Secret, error) {
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
			Name:      fmt.Sprintf(adminKubeConfigSecretStringTemplate, m.ClusterProvisionName),
			Namespace: m.Namespace,
		},
		Data: map[string][]byte{
			"kubeconfig": kubeconfigBytes,
		},
	}

	m.log.WithField("derivedObject", kubeconfigSecret.Name).Debug("Setting labels on derived object")
	kubeconfigSecret.Labels = k8slabels.AddLabel(kubeconfigSecret.Labels, constants.ClusterProvisionNameLabel, provision.Name)
	kubeconfigSecret.Labels = k8slabels.AddLabel(kubeconfigSecret.Labels, constants.SecretTypeLabel, constants.SecretTypeKubeConfig)
	if err := controllerutil.SetControllerReference(provision, kubeconfigSecret, scheme.Scheme); err != nil {
		m.log.WithError(err).Error("error setting controller reference on kubeconfig secret")
		return nil, err
	}

	if err := createWithRetries(kubeconfigSecret, m); err != nil {
		return nil, err
	}

	return kubeconfigSecret, nil
}

func uploadAdminPassword(provision *hivev1.ClusterProvision, m *InstallManager) (*corev1.Secret, error) {
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
			Name:      fmt.Sprintf(adminPasswordSecretStringTemplate, m.ClusterProvisionName),
			Namespace: m.Namespace,
		},
		Data: map[string][]byte{
			"username": []byte(kubeadminUsername),
			"password": []byte(password),
		},
	}

	m.log.WithField("derivedObject", s.Name).Debug("Setting labels on derived object")
	s.Labels = k8slabels.AddLabel(s.Labels, constants.ClusterProvisionNameLabel, provision.Name)
	s.Labels = k8slabels.AddLabel(s.Labels, constants.SecretTypeLabel, constants.SecretTypeKubeAdminCreds)
	if err := controllerutil.SetControllerReference(provision, s, scheme.Scheme); err != nil {
		m.log.WithError(err).Error("error setting controller reference on kubeconfig secret")
		return nil, err
	}

	if err := createWithRetries(s, m); err != nil {
		return nil, err
	}

	return s, nil
}

func createWithRetries(obj runtime.Object, m *InstallManager) error {
	logger := m.log.WithField("kind", obj.GetObjectKind().GroupVersionKind().Kind)

	metaObj, err := meta.Accessor(obj)
	if err != nil {
		logger.WithError(err).Error("object has no object metadata")
		return errors.Wrap(err, "object has no object metadata")
	}

	logger = logger.WithField("name", metaObj.GetName())

	backoff := retry.DefaultBackoff
	backoff.Steps = 10
	backoff.Duration = time.Second

	if err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		err := m.DynamicClient.Create(context.Background(), obj)
		if err != nil {
			logger.WithError(err).Warnf("error creating resource")
			return false, nil
		}
		logger.Info("created resource")
		return true, nil
	}); err != nil {
		logger.WithError(err).Error("failed to create resource")
		return err
	}

	return nil
}

func (m *InstallManager) cleanupAdminKubeconfigSecret() error {
	// find/delete any previous admin kubeconfig secret
	namespacedName := types.NamespacedName{
		Name:      fmt.Sprintf(adminKubeConfigSecretStringTemplate, m.ClusterProvisionName),
		Namespace: m.Namespace,
	}
	if err := m.deleteAnyExistingObject(namespacedName, &corev1.Secret{}); err != nil {
		m.log.WithError(err).Error("failed to fetch/delete any pre-existing kubeconfig secret")
		return err
	}

	return nil
}

func (m *InstallManager) cleanupAdminPasswordSecret() error {
	// find/delete any previous admin password secret
	namespacedName := types.NamespacedName{
		Name:      fmt.Sprintf(adminPasswordSecretStringTemplate, m.ClusterProvisionName),
		Namespace: m.Namespace,
	}
	if err := m.deleteAnyExistingObject(namespacedName, &corev1.Secret{}); err != nil {
		m.log.WithError(err).Error("failed to fetch/delete any pre-existing admin password secret")
		return err
	}

	return nil
}

// deleteAnyExistingObject will look for any object that exists that matches the passed in 'obj' and will delete it if it exists
func (m *InstallManager) deleteAnyExistingObject(namespacedName types.NamespacedName, obj runtime.Object) error {
	return resource.DeleteAnyExistingObject(m.DynamicClient, namespacedName, obj, m.log)
}

func waitForProvisioningStage(provision *hivev1.ClusterProvision, m *InstallManager) error {
	waitContext, cancel := context.WithTimeout(context.Background(), provisioningTransitionTimeout)
	defer cancel()

	config, err := rest.InClusterConfig()
	if err != nil {
		return errors.Wrap(err, "could not get in-cluster REST config")
	}
	gvk, err := apiutil.GVKForObject(&hivev1.ClusterProvision{}, scheme.Scheme)
	if err != nil {
		return errors.Wrap(err, "could not get the GVK for clusterprovisions")
	}
	restClient, err := apiutil.RESTClientForGVK(gvk, config, scheme.Codecs)
	if err != nil {
		return errors.Wrap(err, "could not create REST client")
	}

	_, err = clientwatch.UntilWithSync(
		waitContext,
		cache.NewListWatchFromClient(
			restClient,
			"clusterprovisions",
			provision.Namespace,
			fields.OneTermEqualSelector("metadata.name", provision.Name),
		),
		&hivev1.ClusterProvision{},
		nil,
		func(event watch.Event) (bool, error) {
			switch event.Type {
			case watch.Added, watch.Modified:
				provision, ok := event.Object.(*hivev1.ClusterProvision)
				if !ok {
					m.log.Warnf("Expected a ClusterProvision object but got a %q object instead", event.Object.GetObjectKind().GroupVersionKind())
					return false, nil
				}
				switch provision.Spec.Stage {
				case hivev1.ClusterProvisionStageInitializing:
					m.log.Info("Still waiting for transition to provisioning stage")
					return false, nil
				case hivev1.ClusterProvisionStageProvisioning:
					m.log.Info("ClusterProvisision has transitioned to provisioning stage")
					return true, nil
				default:
					m.log.Warnf("ClusterProvision has transitioned to %s stage while waiting for provisioning stage", provision.Spec.Stage)
					return false, fmt.Errorf("transition to %s", provision.Spec.Stage)
				}
			case watch.Deleted:
				m.log.Warnf("ClusterProvision was deleted")
				return false, errors.New("ClusterProvision deleted")
			default:
				return false, nil
			}
		},
	)
	return errors.Wrap(err, "ClusterProvision did not transition to provisioning stage")
}

func (m *InstallManager) setupSSHUser() error {

	// Add our potentially random UID to /etc/passwd so ssh works. Need to shell out here as
	// the go libraries for user info appear to rely on /etc/passwd, which our user is not in.
	out, err := exec.Command("id", "-u").Output()
	if err != nil {
		m.log.WithError(err).Error("error running id -u")
		return err
	}
	uid := strings.TrimSpace(string(out))
	m.log.Infof("Adding user ID to passwd file: %s", uid)
	f, err := os.OpenFile("/etc/passwd",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		m.log.WithError(err).Error("error opening /etc/passwd")
		return err
	}
	defer f.Close()
	passwdLine := fmt.Sprintf("default:x:%s:0:default user:%s:/sbin/nologin\n", uid, getHomeDir())
	m.log.Infof("Wrote passwd line: %s", passwdLine)
	if _, err := f.WriteString(passwdLine); err != nil {
		m.log.WithError(err).Error("error writing to /etc/passwd")
		return err
	}
	return nil
}

func (m *InstallManager) writeSSHKnownHosts(homeDir string, knownHosts []string) error {
	sshDir := filepath.Join(homeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		m.log.WithError(err).Errorf("error creating %s directory", sshDir)
		return err
	}
	allKnownHosts := strings.Join(knownHosts, "\n")
	if err := ioutil.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte(allKnownHosts), 0644); err != nil {
		m.log.WithError(err).Error("error writing ssh known_hosts")
		return err
	}
	m.log.WithField("knownHosts", knownHosts).Infof("Wrote known hosts to %s/known_hosts", sshDir)
	return nil
}

type provisionMutation func(provision *hivev1.ClusterProvision)

func updateClusterProvisionWithRetries(provision *hivev1.ClusterProvision, m *InstallManager, mutation provisionMutation) error {
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// read in a fresh clusterprovision
		if err := m.loadClusterProvision(provision); err != nil {
			m.log.WithError(err).Warn("error reading in fresh clusterprovision")
			return err
		}

		// make the needed modifications to the clusterprovision
		mutation(provision)

		if err := m.DynamicClient.Update(context.Background(), provision); err != nil {
			m.log.WithError(err).Warn("error updating clusterprovision")
			return err
		}

		return nil
	}); err != nil {
		m.log.WithError(err).Error("error trying to update clusterprovision")
		return err
	}
	return nil
}

func cleanupLogOutput(fullLog string) string {
	var cleanedString string

	cleanedString = multiLineRedactLinesWithPassword.ReplaceAllString(fullLog, "REDACTED LINE OF OUTPUT")

	return cleanedString
}

// isDirNonEmpty returns true if the directory exists and contains at least one file.
func isDirNonEmpty(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	return err == nil
}

func pasteInPullSecret(icData []byte, pullSecretFile string) ([]byte, error) {
	pullSecretData, err := ioutil.ReadFile(pullSecretFile)
	if err != nil {
		return nil, errors.Wrap(err, "could not read the pull secret file")
	}
	icRaw := map[string]interface{}{}
	if err := yaml.Unmarshal(icData, &icRaw); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal InstallConfig")
	}
	icRaw["pullSecret"] = string(pullSecretData)
	return yaml.Marshal(icRaw)
}

func getHomeDir() string {
	home := os.Getenv("HOME")
	if home != "" {
		return home
	}
	return defaultHomeDir
}
