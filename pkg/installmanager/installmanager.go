package installmanager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	yamlpatch "github.com/krishicks/yaml-patch"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	clientwatch "k8s.io/client-go/tools/watch"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/openshift/installer/pkg/destroy/alibabacloud"
	"github.com/openshift/installer/pkg/destroy/aws"
	"github.com/openshift/installer/pkg/destroy/azure"
	"github.com/openshift/installer/pkg/destroy/gcp"
	"github.com/openshift/installer/pkg/destroy/ibmcloud"
	"github.com/openshift/installer/pkg/destroy/openstack"
	"github.com/openshift/installer/pkg/destroy/ovirt"
	"github.com/openshift/installer/pkg/destroy/providers"
	"github.com/openshift/installer/pkg/destroy/vsphere"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesalibabacloud "github.com/openshift/installer/pkg/types/alibabacloud"
	installertypesazure "github.com/openshift/installer/pkg/types/azure"
	installertypesgcp "github.com/openshift/installer/pkg/types/gcp"
	installertypesibmcloud "github.com/openshift/installer/pkg/types/ibmcloud"
	installertypesopenstack "github.com/openshift/installer/pkg/types/openstack"
	installertypesovirt "github.com/openshift/installer/pkg/types/ovirt"
	installertypesvsphere "github.com/openshift/installer/pkg/types/vsphere"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	contributils "github.com/openshift/hive/contrib/pkg/utils"
	aliutils "github.com/openshift/hive/contrib/pkg/utils/alibabacloud"
	awsutils "github.com/openshift/hive/contrib/pkg/utils/aws"
	azureutils "github.com/openshift/hive/contrib/pkg/utils/azure"
	gcputils "github.com/openshift/hive/contrib/pkg/utils/gcp"
	ibmutils "github.com/openshift/hive/contrib/pkg/utils/ibmcloud"
	openstackutils "github.com/openshift/hive/contrib/pkg/utils/openstack"
	ovirtutils "github.com/openshift/hive/contrib/pkg/utils/ovirt"
	vsphereutils "github.com/openshift/hive/contrib/pkg/utils/vsphere"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/machinepool"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/gcpclient"
	"github.com/openshift/hive/pkg/ibmclient"
	"github.com/openshift/hive/pkg/resource"
	k8slabels "github.com/openshift/hive/pkg/util/labels"
	yamlutils "github.com/openshift/hive/pkg/util/yaml"
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
	installerFullLogFile                = ".openshift_install.log"
	installerConsoleLogFilePath         = "/tmp/openshift-install-console.log"
	provisioningTransitionTimeout       = 5 * time.Minute
	sshCopyTempFile                     = "/tmp/ssh-privatekey"
	defaultInstallConfigMountPath       = "/installconfig/install-config.yaml"
	defaultPullSecretMountPath          = "/pullsecret/" + corev1.DockerConfigJsonKey
	defaultManifestsMountPath           = "/manifests"
	defaultHomeDir                      = "/home/hive" // Used if no HOME env var set.
	installConfigKeyName                = "install-config"
	clusterConfigYAML                   = "cluster-config.yaml"
	overrideCredsYAML                   = "99_cloud-creds-secret.yaml"
	clusterInfraConfigYAML              = "cluster-infrastructure-02-config.yml"
)

var (
	// multi-line mode regex that allows removing/mutating any line containing 'password' case-insensitive
	multiLineRedactLinesWithPassword = regexp.MustCompile(`(?mi)^.*password.*$`)
)

// InstallManager coordinates executing the openshift-install binary, modifying
// generated assets, and uploading artifacts to the kube API after completion.
type InstallManager struct {
	log                              log.FieldLogger
	LogLevel                         string
	WorkDir                          string
	LogsDir                          string
	ClusterID                        string
	ClusterName                      string
	ClusterProvisionName             string
	ClusterProvision                 *hivev1.ClusterProvision
	Namespace                        string
	InstallConfigMountPath           string
	PullSecretMountPath              string
	ManifestsMountPath               string
	DynamicClient                    client.Client
	loadSecrets                      func(*InstallManager, *hivev1.ClusterDeployment)
	cleanupFailedProvision           func(dynamicClient client.Client, cd *hivev1.ClusterDeployment, infraID string, logger log.FieldLogger) error
	updateClusterProvision           func(*InstallManager, provisionMutation) error
	readClusterMetadata              func(*InstallManager) ([]byte, *installertypes.ClusterMetadata, error)
	uploadAdminKubeconfig            func(*InstallManager) (*corev1.Secret, error)
	uploadAdminPassword              func(*InstallManager) (*corev1.Secret, error)
	loadAdminPassword                func(*InstallManager) (string, error)
	provisionCluster                 func(*InstallManager) error
	readInstallerLog                 func(*InstallManager, bool) (string, error)
	waitForProvisioningStage         func(*InstallManager) error
	waitForInstallCompleteExecutions int
	binaryDir                        string
	actuator                         LogUploaderActuator
}

// NewInstallManagerCommand is the entrypoint to create the 'install-manager' subcommand
func NewInstallManagerCommand() *cobra.Command {
	im := &InstallManager{
		actuator: getActuator(),
	}
	cmd := &cobra.Command{
		Use:   "install-manager NAMESPACE CLUSTER_PROVISION_NAME",
		Short: "Executes and oversees the openshift-installer.",
		Long: `
The Hive Install Manager runs the phases of the openshift-installer, edits generated assets before
completing install, and monitors for artifacts that need to be uploaded back to Hive.

The following environment variables, if present, configure the Install Manager to upload logs for
failed provisions:

HIVE_INSTALL_LOGS_UPLOAD_PROVIDER: The cloud provider hosting the object store. At this time only
	"aws" is supported.
HIVE_INSTALL_LOGS_CREDENTIALS_SECRET: The name of a secret in the current namespace containing
	credentials sufficient to write data to the specified bucket. For example, for AWS, the secret
	data could contain base64-encoded values for "aws_access_key_id" and "aws_secret_access_key".
HIVE_INSTALL_LOGS_AWS_REGION: The region containing the specified bucket.
HIVE_INSTALL_LOGS_AWS_S3_BUCKET: The name of the S3 bucket to which to upload the logs. The bucket
	must exist and be writable using the specified credentials.
SSH_PRIV_KEY_PATH: File system path of a file containing the SSH private key corresponding to the
	public key in the install config.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := im.Complete(args); err != nil {
				log.WithError(err).Error("cannot complete command")
				return
			}

			if len(args) != 2 {
				cmd.Help()
				im.log.WithField("args", args).Fatal("invalid command arguments")
			}
			// Parse the namespace/name for our cluster provision:
			im.Namespace, im.ClusterProvisionName = args[0], args[1]
			im.InstallConfigMountPath = defaultInstallConfigMountPath
			im.PullSecretMountPath = defaultPullSecretMountPath
			im.ManifestsMountPath = defaultManifestsMountPath
			im.binaryDir = getHomeDir()

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

	cmd.AddCommand(NewInstallManagerAWSCredentials())
	return cmd
}

// Complete sets remaining fields on the InstallManager based on command options and arguments.
// ...except for the clusterProvision field. That's loaded up by Run(), since it involves a REST call.
func (m *InstallManager) Complete(args []string) error {
	// Connect up structure's function pointers
	m.loadSecrets = loadSecrets
	m.updateClusterProvision = updateClusterProvisionWithRetries
	m.readClusterMetadata = readClusterMetadata
	m.uploadAdminKubeconfig = uploadAdminKubeconfig
	m.uploadAdminPassword = uploadAdminPassword
	m.loadAdminPassword = loadAdminPassword
	m.readInstallerLog = readInstallerLog
	m.cleanupFailedProvision = cleanupFailedProvision
	m.provisionCluster = provisionCluster
	m.waitForProvisioningStage = waitForProvisioningStage

	var err error
	m.log, err = contributils.NewLogger(m.LogLevel)
	if err != nil {
		return err
	}

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

	// Swap some function implementations if this is flagged to be a fake install:
	fakeInstall, _ := strconv.ParseBool(os.Getenv(constants.FakeClusterInstallEnvVar))
	if fakeInstall {
		m.log.Warnf("%s set to true, swapping function implementations to fake an installation",
			constants.FakeClusterInstallEnvVar)
		m.loadAdminPassword = fakeLoadAdminPassword
		m.readClusterMetadata = fakeReadClusterMetadata
		m.provisionCluster = fakeProvisionCluster
	}

	return nil
}

// Validate ensures the given options and arguments are valid.
func (m *InstallManager) Validate() error {
	return nil
}

// Run is the entrypoint to start the install process
func (m *InstallManager) Run() error {
	m.ClusterProvision = &hivev1.ClusterProvision{}
	if err := m.loadClusterProvision(); err != nil {
		m.log.WithError(err).Fatal("error looking up cluster provision")
	}
	switch m.ClusterProvision.Spec.Stage {
	case hivev1.ClusterProvisionStageInitializing, hivev1.ClusterProvisionStageProvisioning:
	default:
		// This should not be possible but just in-case we can somehow
		// run the install job for a cluster provision that is already complete, exit early,
		// and don't delete *anything*.
		m.log.Warnf("provision is at stage %q, exiting", m.ClusterProvision.Spec.Stage)
		os.Exit(0)
	}
	cd, err := m.loadClusterDeployment()
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

	m.loadSecrets(m, cd)

	// sshKeyPaths will contain paths to all ssh keys in use
	var sshKeyPaths []string

	// setup the host ssh key if possible
	sshKeyPath, sshAgentSetupErr := m.initSSHKey(constants.SSHPrivKeyPathEnvVar)
	if sshAgentSetupErr != nil {
		// Not a fatal error.
		m.log.WithError(sshAgentSetupErr).Info("unable to initialize host ssh key")
	} else {
		sshKeyPaths = append(sshKeyPaths, sshKeyPath)
	}

	// By default we scrub passwords and sensitive looking text from the install log before printing to the pod log.
	// In rare cases during debugging, a developer may want to disable this via the annotation.
	scrubInstallLog := true
	if disable, err := strconv.ParseBool(cd.Annotations[constants.DisableInstallLogPasswordRedactionAnnotation]); err == nil {
		scrubInstallLog = !disable
	}

	// setup the bare metal provisioning libvirt ssh key if possible
	if cd.Spec.Platform.BareMetal != nil {
		libvirtSSHKeyPath, libvirtSSHAgentSetupErr := m.initSSHKey(constants.LibvirtSSHPrivKeyPathEnvVar)
		if libvirtSSHAgentSetupErr != nil {
			// Not a fatal error.
			m.log.WithError(libvirtSSHAgentSetupErr).Info("unable to initialize libvirt ssh key")
		} else {
			sshKeyPaths = append(sshKeyPaths, libvirtSSHKeyPath)
		}
	}

	if len(sshKeyPaths) > 0 {
		m.log.Infof("initializing ssh agent with %d keys", len(sshKeyPaths))
		sshCleanupFunc, sshAgentSetupErr := m.initSSHAgent(sshKeyPaths)
		defer sshCleanupFunc()
		if sshAgentSetupErr != nil {
			// Not a fatal error. If this is bare metal, it could be, but the problem will surface soon enough
			// and we're not sure how implementations of this will evolve, we may not be doing libvirt over ssh
			// long term.
			m.log.WithError(sshAgentSetupErr).Info("ssh agent is not initialized")
		}
	}

	m.ClusterName = cd.Spec.ClusterName

	if err := m.copyInstallerBinaries(); err != nil {
		m.log.WithError(err).Error("error waiting for/copying binaries")
		return err
	}

	go m.tailFullInstallLog(scrubInstallLog)

	m.log.Info("copying install-config.yaml")
	icData, err := os.ReadFile(m.InstallConfigMountPath)
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
	if err := os.WriteFile(destInstallConfigPath, icData, 0644); err != nil {
		m.log.WithError(err).Error("error writing install-config.yaml")
	}
	m.log.Infof("copied %s to %s", m.InstallConfigMountPath, destInstallConfigPath)

	if cd.Spec.Provisioning != nil && len(cd.Spec.Provisioning.SSHKnownHosts) > 0 {
		err = m.writeSSHKnownHosts(getHomeDir(), cd.Spec.Provisioning.SSHKnownHosts)
		if err != nil {
			m.log.WithError(err).Error("error setting up SSH known_hosts")
			return err
		}
	}

	// If the cluster provision has a prevInfraID set, this implies we failed a previous
	// cluster provision attempt. Cleanup any resources that may have been provisioned.
	if m.ClusterProvision.Spec.PrevInfraID != nil {
		m.log.Info("cleaning up resources from previous provision attempt")
		if err := m.cleanupFailedInstall(cd, *m.ClusterProvision.Spec.PrevInfraID, *m.ClusterProvision.Spec.PrevProvisionName, m.Namespace); err != nil {
			m.log.WithError(err).Error("error while trying to preemptively clean up")
			return err
		}
	}

	// If the cluster provision has an infraID set, this implies that previous install
	// pod for the current cluster provision got unexpectedly killed or evicted. If we
	// proceed, the current install will error out updating the infraID on the cluster
	// provision. The infraID is an immutable write-once field, as we need it to clean
	// up provisioned resources from a failed provision. So if infraID is already set,
	// we do not proceed further and fail the provision.
	if m.ClusterProvision.Spec.InfraID != nil {
		// This error won't be displayed as condition message on the cluster provision
		// because install log is not generated until we run installer binary. We do
		// have the option of faking an install log that can then be regexed.
		m.log.Error("infraID is already set on the ClusterProvision. Unexpected install pod restart detected. Cleaning up resources from previous install attempt")
		if err := m.cleanupFailedInstall(cd, *m.ClusterProvision.Spec.InfraID, m.ClusterProvisionName, m.Namespace); err != nil {
			m.log.WithError(err).Error("error while trying to preemptively clean up")
			return err
		}
		return fmt.Errorf("infraID is already set on the ClusterProvision. Unexpected install pod restart detected")
	}

	// Get worker MachinePool for the ClusterDeployment if one exists.
	workerMachinePool, err := m.loadWorkerMachinePool(cd)
	if err != nil {
		// Error indicates we weren't able to check if there was a worker MachinePool
		// associated with the ClusterDeployment.
		m.log.WithError(err).Error("error looking up worker machine pool")
		return err
	}

	// Generate installer assets we need to modify or upload.
	m.log.Info("generating assets")
	if err := m.generateAssets(cd, workerMachinePool); err != nil {
		m.log.Info("reading installer log")
		installLog, readErr := m.readInstallerLog(m, scrubInstallLog)
		if readErr != nil {
			m.log.WithError(readErr).Error("error reading asset generation log")
			return err
		}

		m.log.Info("updating clusterprovision")
		if err := m.updateClusterProvision(
			m,
			func(provision *hivev1.ClusterProvision) {
				provision.Spec.InstallLog = pointer.String(installLog)
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
	metadataBytes, metadata, err := m.readClusterMetadata(m)
	if err != nil {
		m.log.WithError(err).Error("error reading cluster metadata")
		return errors.Wrap(err, "error reading cluster metadata")
	}
	kubeconfigSecret, err := m.uploadAdminKubeconfig(m)
	if err != nil {
		m.log.WithError(err).Error("error uploading admin kubeconfig")
		return errors.Wrap(err, "error trying to save admin kubeconfig")
	}

	passwordSecret, err := m.uploadAdminPassword(m)
	if err != nil {
		m.log.WithError(err).Error("error uploading admin password")
		return errors.Wrap(err, "error trying to save admin password")
	}
	if err := m.updateClusterProvision(
		m,
		func(provision *hivev1.ClusterProvision) {
			provision.Spec.MetadataJSON = metadataBytes
			provision.Spec.InfraID = pointer.String(metadata.InfraID)
			provision.Spec.ClusterID = pointer.String(metadata.ClusterID)

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
	if err := m.waitForProvisioningStage(m); err != nil {
		m.log.WithError(err).Error("ClusterProvision failed to transition to provisioning")
		return errors.Wrap(err, "failed to transition to provisioning")
	}

	m.log.Info("provisioning cluster")

	if waitForInstallCompleteExecutions, ok := cd.Annotations[constants.WaitForInstallCompleteExecutionsAnnotation]; ok {
		m.waitForInstallCompleteExecutions, err = strconv.Atoi(waitForInstallCompleteExecutions)
		if err != nil {
			m.log.WithField("value", waitForInstallCompleteExecutions).WithError(err).
				Errorf("error parsing integer from %s annotation", constants.WaitForInstallCompleteExecutionsAnnotation)
		}
	}

	installErr := m.provisionCluster(m)
	if installErr != nil {
		m.log.WithError(installErr).Error("error running openshift-install, running deprovision to clean up")

		if pauseDur, ok := cd.Annotations[constants.PauseOnInstallFailureAnnotation]; ok {
			m.log.Infof("pausing on failure due to annotation %s=%s", constants.PauseOnInstallFailureAnnotation,
				pauseDur)
			dur, err := time.ParseDuration(pauseDur)
			if err != nil {
				// Not a fatal error.
				m.log.WithError(err).WithField("pauseDuration", pauseDur).Warn("error parsing pause duration, skipping pause")
			} else {
				time.Sleep(dur)
			}
		}

		// Fetch logs from all cluster machines:
		if m.actuator == nil {
			m.log.Debug("Unable to find log storage actuator. Disabling gathering logs.")
		} else {
			m.gatherLogs(cd, sshKeyPath, sshAgentSetupErr)
		}
	}

	if installLog, err := m.readInstallerLog(m, scrubInstallLog); err == nil {
		if err := m.updateClusterProvision(
			m,
			func(provision *hivev1.ClusterProvision) {
				provision.Spec.InstallLog = pointer.String(installLog)
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

func loadSecrets(m *InstallManager, cd *hivev1.ClusterDeployment) {
	// Configure credentials (including certs) appropriately according to the cloud provider
	switch {
	case cd.Spec.Platform.AlibabaCloud != nil:
		aliutils.ConfigureCreds(m.DynamicClient)
	case cd.Spec.Platform.AWS != nil:
		awsutils.ConfigureCreds(m.DynamicClient)
	case cd.Spec.Platform.Azure != nil:
		azureutils.ConfigureCreds(m.DynamicClient)
	case cd.Spec.Platform.GCP != nil:
		gcputils.ConfigureCreds(m.DynamicClient)
	case cd.Spec.Platform.OpenStack != nil:
		openstackutils.ConfigureCreds(m.DynamicClient)
	case cd.Spec.Platform.VSphere != nil:
		vsphereutils.ConfigureCreds(m.DynamicClient)
	case cd.Spec.Platform.Ovirt != nil:
		ovirtutils.ConfigureCreds(m.DynamicClient)
	case cd.Spec.Platform.IBMCloud != nil:
		ibmutils.ConfigureCreds(m.DynamicClient)
	}

	// Load up the install config and pull secret. These env vars are required; else we'll panic.
	contributils.ProjectToDir(contributils.LoadSecretOrDie(m.DynamicClient, "INSTALLCONFIG_SECRET_NAME"), "/installconfig")
	contributils.ProjectToDir(contributils.LoadSecretOrDie(m.DynamicClient, "PULLSECRET_SECRET_NAME"), "/pullsecret")

	// Additional manifests? Could come in on a Secret or a ConfigMap
	if manSecret := contributils.LoadSecretOrDie(m.DynamicClient, "MANIFESTS_SECRET_NAME"); manSecret != nil {
		contributils.ProjectToDir(manSecret, "/manifests")
	} else if manCM := contributils.LoadConfigMapOrDie(m.DynamicClient, "MANIFESTS_CONFIGMAP_NAME"); manCM != nil {
		contributils.ProjectToDir(manCM, "/manifests")
	}

	// Custom BoundServiceAccountSigningKey
	if bsask := contributils.LoadSecretOrDie(m.DynamicClient, "BOUND_TOKEN_SIGNING_KEY_SECRET_NAME"); bsask != nil {
		contributils.ProjectToDir(bsask, constants.BoundServiceAccountSigningKeyDir, constants.BoundServiceAccountSigningKeyFile)
		os.Setenv(constants.BoundServiceAccountSigningKeyEnvVar,
			constants.BoundServiceAccountSigningKeyDir+"/"+constants.BoundServiceAccountSigningKeyFile)
	}

	// SSH private key
	if sshkey := contributils.LoadSecretOrDie(m.DynamicClient, "SSH_PRIVATE_KEY_SECRET_PATH"); sshkey != nil {
		contributils.ProjectToDir(sshkey, constants.SSHPrivateKeyDir)
		// TODO: Collapse this in initSSHKey
		os.Setenv(constants.SSHPrivKeyPathEnvVar,
			constants.SSHPrivateKeyDir+"/"+constants.SSHPrivateKeySecretKey)
	}

	// BareMetal Libvirt SSH private key
	if sshkey := contributils.LoadSecretOrDie(m.DynamicClient, "LIBVIRT_SSH_KEYS_SECRET_NAME"); sshkey != nil {
		contributils.ProjectToDir(sshkey, constants.LibvirtSSHPrivateKeyDir)
		os.Setenv(constants.LibvirtSSHPrivKeyPathEnvVar,
			constants.LibvirtSSHPrivateKeyDir+"/"+constants.SSHPrivateKeySecretKey)
	}
}

func getActuator() LogUploaderActuator {
	// As we add more LogUploaderActuators, add them here
	actuators := []LogUploaderActuator{
		&s3LogUploaderActuator{awsClientFn: getAWSClient},
	}

	for _, a := range actuators {
		if a.IsConfigured() {
			return a
		}
	}
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

func (m *InstallManager) copyInstallerBinaries() error {
	fileList := []string{
		filepath.Join(m.WorkDir, "openshift-install"),
		filepath.Join(m.WorkDir, "oc"),
	}

	// copy each binary to our container user's home dir to avoid situations
	// where the /output workdir may be mounted with noexec. (surfaced when using kind
	// 0.8.x with podman but will likely be fixed in future versions)
	for _, src := range fileList {
		dest := filepath.Join(m.binaryDir, filepath.Base(src))
		if err := m.copyFile(src, dest); err != nil {
			return err
		}
		m.log.Infof("copied %s to %s", src, dest)
	}
	return nil
}

func (m *InstallManager) copyFile(src, dst string) error {
	cmd := exec.Command("cp", "-p", src, dst)
	if err := cmd.Run(); err != nil {
		log.WithError(err).Errorf("error copying file %s to %s", src, dst)
		return err
	}
	return nil
}

// cleanupFailedInstall allows recovering from an installation error and allows retries
func (m *InstallManager) cleanupFailedInstall(cd *hivev1.ClusterDeployment, infraID, provisionName, provisionNamespace string) error {
	if err := m.cleanupFailedProvision(m.DynamicClient, cd, infraID, m.log); err != nil {
		return err
	}

	if err := m.cleanupAdminKubeconfigSecret(provisionName, provisionNamespace); err != nil {
		return err
	}

	if err := m.cleanupAdminPasswordSecret(provisionName, provisionNamespace); err != nil {
		return err
	}

	return nil
}

func cleanupFailedProvision(dynClient client.Client, cd *hivev1.ClusterDeployment, infraID string, logger log.FieldLogger) error {
	var uninstaller providers.Destroyer
	switch {
	case cd.Spec.Platform.AlibabaCloud != nil:
		metadata := &installertypes.ClusterMetadata{
			ClusterName: cd.Spec.ClusterName,
			InfraID:     infraID,
			ClusterPlatformMetadata: installertypes.ClusterPlatformMetadata{
				AlibabaCloud: &installertypesalibabacloud.Metadata{
					ClusterDomain: fmt.Sprintf("%s.%s", cd.Spec.ClusterName, cd.Spec.BaseDomain),
					Region:        cd.Spec.Platform.AlibabaCloud.Region,
				},
			},
		}
		var err error
		uninstaller, err = alibabacloud.New(logger, metadata)
		if err != nil {
			return err
		}
	case cd.Spec.Platform.AWS != nil:
		// run the uninstaller to clean up any cloud resources previously created
		filters := []aws.Filter{
			{kubernetesKeyPrefix + infraID: "owned"},
		}
		uninstaller = &aws.ClusterUninstaller{
			Filters: filters,
			Region:  cd.Spec.Platform.AWS.Region,
			Logger:  logger,
		}
	case cd.Spec.Platform.Azure != nil:
		cloudName := installertypesazure.PublicCloud.Name()
		if cd.Spec.Platform.Azure.CloudName != "" {
			cloudName = cd.Spec.Platform.Azure.CloudName.Name()
		}
		metadata := &installertypes.ClusterMetadata{
			InfraID: infraID,
			ClusterPlatformMetadata: installertypes.ClusterPlatformMetadata{
				Azure: &installertypesazure.Metadata{
					CloudName: installertypesazure.CloudEnvironment(cloudName),
				},
			},
		}
		var err error
		uninstaller, err = azure.New(logger, metadata)
		if err != nil {
			return err
		}
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
		uninstaller, err = gcp.New(logger, metadata)
		if err != nil {
			return err
		}
	case cd.Spec.Platform.OpenStack != nil:
		metadata := &installertypes.ClusterMetadata{
			InfraID: infraID,
			ClusterPlatformMetadata: installertypes.ClusterPlatformMetadata{
				OpenStack: &installertypesopenstack.Metadata{
					Cloud: cd.Spec.Platform.OpenStack.Cloud,
					Identifier: map[string]string{
						"openshiftClusterID": infraID,
					},
				},
			},
		}
		var err error
		uninstaller, err = openstack.New(logger, metadata)
		if err != nil {
			return err
		}
	case cd.Spec.Platform.VSphere != nil:
		vSphereUsername := os.Getenv(constants.VSphereUsernameEnvVar)
		if vSphereUsername == "" {
			return fmt.Errorf("no %s env var set, cannot proceed", constants.VSphereUsernameEnvVar)
		}
		vSpherePassword := os.Getenv(constants.VSpherePasswordEnvVar)
		if vSpherePassword == "" {
			return fmt.Errorf("no %s env var set, cannot proceed", constants.VSpherePasswordEnvVar)
		}
		metadata := &installertypes.ClusterMetadata{
			InfraID: infraID,
			ClusterPlatformMetadata: installertypes.ClusterPlatformMetadata{
				VSphere: &installertypesvsphere.Metadata{
					VCenter:  cd.Spec.Platform.VSphere.VCenter,
					Username: vSphereUsername,
					Password: vSpherePassword,
				},
			},
		}
		var err error
		uninstaller, err = vsphere.New(logger, metadata)
		if err != nil {
			return err
		}
	case cd.Spec.Platform.Ovirt != nil:
		metadata := &installertypes.ClusterMetadata{
			InfraID: infraID,
			ClusterPlatformMetadata: installertypes.ClusterPlatformMetadata{
				Ovirt: &installertypesovirt.Metadata{
					ClusterID:      cd.Spec.Platform.Ovirt.ClusterID,
					RemoveTemplate: true,
				},
			},
		}
		var err error
		uninstaller, err = ovirt.New(logger, metadata)
		if err != nil {
			return err
		}
	case cd.Spec.Platform.IBMCloud != nil:
		// Create IBMCloud Client
		ibmCloudAPIKey := os.Getenv(constants.IBMCloudAPIKeyEnvVar)
		if ibmCloudAPIKey == "" {
			return fmt.Errorf("no %s env var set, cannot proceed", constants.IBMCloudAPIKeyEnvVar)
		}
		ibmClient, err := ibmclient.NewClient(ibmCloudAPIKey)
		if err != nil {
			return errors.Wrap(err, "Unable to create IBM Cloud client")
		}
		// Retrieve CISInstanceCRN
		cisInstanceCRN, err := ibmclient.GetCISInstanceCRN(ibmClient, context.TODO(), cd.Spec.BaseDomain)
		if err != nil {
			return err
		}
		// Retrieve AccountID
		accountID, err := ibmclient.GetAccountID(ibmClient, context.TODO())
		if err != nil {
			return err
		}
		metadata := &installertypes.ClusterMetadata{
			InfraID:     infraID,
			ClusterName: cd.Spec.ClusterName,
			ClusterPlatformMetadata: installertypes.ClusterPlatformMetadata{
				IBMCloud: &installertypesibmcloud.Metadata{
					AccountID:         accountID,
					BaseDomain:        cd.Spec.BaseDomain,
					CISInstanceCRN:    cisInstanceCRN,
					Region:            cd.Spec.Platform.IBMCloud.Region,
					ResourceGroupName: cd.Spec.ClusterMetadata.InfraID,
				},
			},
		}
		var ibmCloudDestroyerErr error
		uninstaller, ibmCloudDestroyerErr = ibmcloud.New(logger, metadata)
		if ibmCloudDestroyerErr != nil {
			return ibmCloudDestroyerErr
		}
	default:
		logger.Warn("unknown platform for re-try cleanup")
		return errors.New("unknown platform for re-try cleanup")
	}

	// ClusterQuota stomped in return
	if _, err := uninstaller.Run(); err != nil {
		return err
	}

	return cleanupDNSZone(dynClient, cd, logger)
}

// generateAssets runs openshift-install commands to generate on-disk assets we need to
// upload or modify prior to provisioning resources in the cloud.
func (m *InstallManager) generateAssets(cd *hivev1.ClusterDeployment, workerMachinePool *hivev1.MachinePool) error {
	m.log.Info("running openshift-install create manifests")
	err := m.runOpenShiftInstallCommand("create", "manifests")
	if err != nil {
		m.log.WithError(err).Error("error generating installer assets")
		return err
	}

	// Configure for install of an STS cluster if specified:
	boundSASigningKey := os.Getenv(constants.BoundServiceAccountSigningKeyEnvVar)
	if boundSASigningKey != "" {
		tlsDir := filepath.Join(m.WorkDir, "tls")
		dest := filepath.Join(tlsDir, constants.BoundServiceAccountSigningKeyFile)
		m.log.WithField("src", boundSASigningKey).WithField("dest", dest).
			Info("copying bound service account signing key")
		if err := os.MkdirAll(tlsDir, 0700); err != nil {
			m.log.WithError(err).Errorf("error creating %s directory", tlsDir)
			return err
		}
		log.WithField("dest", dest).Info("copying bound SA signing key")
		if err := m.copyFile(boundSASigningKey, dest); err != nil {
			return err
		}
	}

	// Day 0: Configure Hive worker MachinePool manifests with additional security group when
	// ExtraWorkerSecurityGroupAnnotation has been set in annotations for the worker MachinePool.
	// For details, see HIVE-1802.
	if workerMachinePool != nil && metav1.HasAnnotation(workerMachinePool.ObjectMeta, constants.ExtraWorkerSecurityGroupAnnotation) {
		if cd.Spec.Platform.AWS == nil {
			return errors.New("ExtraWorkerSecurityGroup annotation cannot be configured for non-AWS MachinePool")
		}
		m.log.Info("retrieving VPC ID of configured worker machinepool subnets")
		awsClient, err := awsclient.NewClientFromSecret(nil, cd.Spec.Platform.AWS.Region)
		if err != nil {
			return err
		}
		vpcID, err := machinepool.GetVPCIDForMachinePool(awsClient, workerMachinePool)
		if err != nil {
			return err
		}

		m.log.Info("modifying worker machineset manifests")

		// TODO: Handle error arg to WalkDirFunc
		err = filepath.WalkDir(filepath.Join(m.WorkDir, "openshift"), func(path string, d fs.DirEntry, _ error) error {
			if d.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
				return nil
			}

			manifestBytes, err := os.ReadFile(path)
			if err != nil {
				m.log.WithError(err).Error("error reading manifest file")
				return err
			}

			modifiedManifestBytes, err := patchWorkerMachineSetManifest(manifestBytes, workerMachinePool, vpcID, m.log)
			if err != nil {
				m.log.WithError(err).Error("error patching worker machineset manifest")
				return err
			}
			if modifiedManifestBytes != nil {
				info, err := d.Info()
				if err != nil {
					m.log.WithError(err).Error("error retrieving file info")
					return err
				}
				err = os.WriteFile(path, *modifiedManifestBytes, info.Mode())
				if err != nil {
					m.log.WithError(err).Error("error writing manifest")
					return err
				}
			}

			return nil
		})
		if err != nil {
			m.log.WithError(err).Error("error finding machine pool manifests")
			return err
		}
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

	if effectiveMode, ok := cd.Annotations[constants.OverrideInClusterCredentialsModeAnnotation]; ok {
		if err := m.overrideCredentialsModeOnInstallConfig(effectiveMode, cd); err != nil {
			m.log.WithError(err).Error("error overriding credentials mode on install config")
			return err
		}
	}

	m.log.Info("running openshift-install create ignition-configs")
	if err := m.runOpenShiftInstallCommand("create", "ignition-configs"); err != nil {
		m.log.WithError(err).Error("error generating installer assets")
		return err
	}

	m.log.Info("assets generated successfully")
	return nil
}

// patchWorkerMachineSetManifest accepts a yaml manifest as []byte and patches the manifest to include an additional
// security group filter if that manifest is the definition of a worker MachineSet. The security group is obtained from
// an annotation (constants.ExtraWorkerSecurityGroupAnnotation) on the provided MachinePool and is the value of the
// annotation (singular).
// The first return value points to the modified manifest. It is `nil` if the manifest was not modified.
// The second return value indicates an error from which we should not proceed.
// Of note, it is not an error if `manifestBytes` represents valid yaml that is not a worker MachineSet.
func patchWorkerMachineSetManifest(manifestBytes []byte, pool *hivev1.MachinePool, vpcID string, logger log.FieldLogger) (*[]byte, error) {
	c, err := yamlutils.Decode(manifestBytes)
	// Decoding the yaml here shouldn't produce an error. An error indicates that the
	// installer produced yaml that could not be decoded.
	if err != nil {
		logger.WithError(err).Error("unable to decode manifest bytes")
		return nil, err
	}

	// Return if file is not a worker MachineSet manifest
	if isMachineSet, _ := yamlutils.Test(c, "/kind", "MachineSet"); !isMachineSet {
		return nil, nil
	}
	// Note: We have to escape the `/` in the label key as `~1` per https://datatracker.ietf.org/doc/html/rfc6901#section-3
	if isWorkerMachineSet, _ := yamlutils.Test(c, "/spec/template/metadata/labels/machine.openshift.io~1cluster-api-machine-type", "worker"); !isWorkerMachineSet {
		return nil, nil
	}

	securityGroupFilterValue := interface{}(pool.Annotations[constants.ExtraWorkerSecurityGroupAnnotation])
	var vpcIDFilterValue map[string]interface{} = map[string]interface{}{
		"name":   "vpc-id",
		"values": []string{vpcID},
	}
	vpcIDFilterValueInterface := interface{}(vpcIDFilterValue)
	ops := yamlpatch.Patch{
		// Add the security group name obtained from the ExtraWorkerSecurityGroupAnnotation
		// annotation found on the MachinePool to the existing tag:Name filter in the worker
		// MachineSet manifest
		yamlpatch.Operation{
			Op:    "add",
			Path:  yamlpatch.OpPath("/spec/template/spec/providerSpec/value/securityGroups/0/filters/0/values/-"),
			Value: yamlpatch.NewNode(&securityGroupFilterValue),
		},
		// Add a filter for the vpcID since the names of security groups may be the same within a
		// given AWS account. HIVE-1874
		yamlpatch.Operation{
			Op:    "add",
			Path:  yamlpatch.OpPath("/spec/template/spec/providerSpec/value/securityGroups/0/filters/-"),
			Value: yamlpatch.NewNode(&vpcIDFilterValueInterface),
		},
	}

	// Apply patch to manifest
	modifiedBytes, err := ops.Apply(manifestBytes)
	if err != nil {
		logger.WithError(err).Error("error applying patch")
		return nil, err
	}
	return &modifiedBytes, nil
}

// provisionCluster invokes the openshift-install create cluster command to provision resources
// in the cloud.
func provisionCluster(m *InstallManager) error {

	m.log.Info("running openshift-install create cluster")

	if err := m.runOpenShiftInstallCommand("create", "cluster"); err != nil {
		if (m.waitForInstallCompleteExecutions > 0) && m.isBootstrapComplete() {
			for i := 0; i < m.waitForInstallCompleteExecutions; i++ {
				m.log.WithField("waitIteration", i).WithError(err).
					Warn("provisioning cluster failed after completing bootstrapping, waiting longer for install to complete")
				err = m.runOpenShiftInstallCommand("wait-for", "install-complete")
			}
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
	cmd := exec.Command(filepath.Join(m.binaryDir, "openshift-install"), args...)
	cmd.Dir = m.WorkDir

	// save the commands' stdout/stderr to a file
	stdOutAndErrOutput, err := os.OpenFile(installerConsoleLogFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		m.log.WithError(err).Error("error creating/appending installer console log file")
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

// tailFullInstallLog streams the full install log to standard out so that
// the log can be seen from the pods logs.
func (m *InstallManager) tailFullInstallLog(scrubInstallLog bool) {
	logfileName := filepath.Join(m.WorkDir, installerFullLogFile)
	m.waitForFiles([]string{logfileName})

	logfile, err := os.Open(logfileName)
	if err != nil {
		// FIXME what is a better response to being unable to open the file
		m.log.WithError(err).Fatalf("unable to open installer log file to display to stdout")
		panic("unable to open log file")
	}
	defer logfile.Close()

	r := bufio.NewReader(logfile)
	fullLine := ""

	// Set up additional log fields
	suffix := ""
	switch fields, err := utils.ExtractLogFields(utils.MetaObjectLogTagger{Object: m.ClusterProvision}); {
	case err != nil:
		m.log.WithError(err).Warning("failed to extract additional log fields -- ignoring")
	case fields == nil:
		m.log.Debug("no additional log fields found")
	default:
		// We should get this with component=hive; override to indicate that the component
		// being tagged now is the installer
		fields["component"] = "installer"
		for k, v := range fields {
			suffix = suffix + fmt.Sprintf(" %s=%v", k, v)
		}
	}

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
			time.Sleep(time.Millisecond * 5)
			continue
		}

		fullLine = fmt.Sprintf("%v%v", fullLine, string(line))

		if prefix {
			// need to do another read to get to end-of-line
			continue
		}

		if scrubInstallLog {
			cleanLine := cleanupLogOutput(fullLine)
			fmt.Println(cleanLine + suffix)
		} else {
			fmt.Println(fullLine + suffix)
		}
		// clear out the line buffer so we can start again
		fullLine = ""
	}
}

func readClusterMetadata(m *InstallManager) ([]byte, *installertypes.ClusterMetadata, error) {
	m.log.Infoln("extracting cluster ID and uploading cluster metadata")
	fullMetadataPath := filepath.Join(m.WorkDir, metadataRelativePath)
	if _, err := os.Stat(fullMetadataPath); os.IsNotExist(err) {
		m.log.WithField("path", fullMetadataPath).Error("cluster metadata file does not exist")
		return nil, nil, err
	}

	metadataBytes, err := os.ReadFile(fullMetadataPath)
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

func (m *InstallManager) loadClusterProvision() error {
	if err := m.DynamicClient.Get(context.TODO(), types.NamespacedName{Namespace: m.Namespace, Name: m.ClusterProvisionName}, m.ClusterProvision); err != nil {
		m.log.WithError(err).Error("error getting cluster provision")
		return err
	}
	m.log = utils.AddLogFields(utils.MetaObjectLogTagger{Object: m.ClusterProvision}, (m.log).(*log.Entry))
	return nil
}

func (m *InstallManager) loadClusterDeployment() (*hivev1.ClusterDeployment, error) {
	cd := &hivev1.ClusterDeployment{}
	if err := m.DynamicClient.Get(context.Background(), types.NamespacedName{Namespace: m.Namespace, Name: m.ClusterProvision.Spec.ClusterDeploymentRef.Name}, cd); err != nil {
		m.log.WithError(err).Error("error getting cluster deployment")
		return nil, err
	}
	return cd, nil
}

func (m *InstallManager) loadWorkerMachinePool(cd *hivev1.ClusterDeployment) (*hivev1.MachinePool, error) {
	mpL := &hivev1.MachinePoolList{}
	if err := m.DynamicClient.List(context.Background(), mpL, &client.ListOptions{Namespace: m.Namespace}); err != nil {
		return nil, err
	}
	for _, pool := range mpL.Items {
		if pool.Spec.ClusterDeploymentRef.Name == cd.Name && pool.Spec.Name == "worker" {
			return &pool, nil
		}
	}
	// No worker pool is associated with the ClusterDeployment (no error)
	return nil, nil
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

	// At this point, all log files are in m.LogsDir
	// Gather the filenames
	files, err := os.ReadDir(m.LogsDir)
	if err != nil {
		m.log.WithError(err).WithField("clusterprovision", types.NamespacedName{Name: m.ClusterProvision.Name, Namespace: m.ClusterProvision.Namespace}).Error("error reading Logsdir")
		return
	}

	filepaths := []string{}
	for _, file := range files {
		if file.IsDir() {
			m.log.WithField("directory", file.Name()).Debug("not uploading directory")
			continue
		}

		filepaths = append(filepaths, filepath.Join(m.LogsDir, file.Name()))
	}

	uploadErr := m.actuator.UploadLogs(cd.Spec.ClusterName, m.ClusterProvision, m.DynamicClient, m.log, filepaths...)
	if uploadErr != nil {
		m.log.WithError(uploadErr).Error("error uploading logs")
	}
}

func (m *InstallManager) gatherClusterLogs(cd *hivev1.ClusterDeployment) error {
	m.log.Info("attempting to gather logs with oc adm must-gather")
	destDir := filepath.Join(m.LogsDir, fmt.Sprintf("%s-must-gather", time.Now().Format("20060102150405")))
	cmd := exec.Command(filepath.Join(m.binaryDir, "oc"), "adm", "must-gather", "--dest-dir", destDir)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("KUBECONFIG=%s", filepath.Join(m.WorkDir, "auth", "kubeconfig")))
	stdout, err := cmd.Output()
	m.log.Infof("must-gather output: %s", stdout)
	if err != nil {
		return err
	}
	// Creating a compressed file of the must-gather directory in m.LogsDir
	tarCmd := exec.Command("tar", "cvaf", filepath.Join(m.LogsDir, "must-gather.tar.gz"), destDir)
	err = tarCmd.Run()
	return err
}

func (m *InstallManager) initSSHKey(envVar string) (string, error) {
	m.log.Debug("checking for SSH private key")
	sshPrivKeyPath := os.Getenv(envVar)
	if sshPrivKeyPath == "" {
		return "", fmt.Errorf("cannot configure SSH agent as %s is unset or empty", envVar)
	}
	fileInfo, err := os.Stat(sshPrivKeyPath)
	if err != nil && os.IsNotExist(err) {
		m.log.WithField("path", sshPrivKeyPath).Warnf("%s defined but file does not exist", envVar)
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
	input, err := os.ReadFile(sshPrivKeyPath)
	if err != nil {
		m.log.WithError(err).Error("error reading ssh private key to copy")
		return "", err
	}

	if err := os.WriteFile(sshCopyTempFile, input, 0600); err != nil {
		m.log.WithError(err).Error("error writing copy of ssh private key")
		return "", err
	}

	return sshCopyTempFile, nil

}

func (m *InstallManager) initSSHAgent(sshKeyPaths []string) (func(), error) {
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

	for _, sshKeyPath := range sshKeyPaths {
		cmd := exec.Command(bin, sshKeyPath)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			m.log.WithField("key", sshKeyPath).WithError(err).Errorf("failed to add private key: %v", sshKeyPath)
			return sshAgentCleanup, err
		}
		m.log.WithField("key", sshKeyPath).Info("added ssh private key to agent")
	}

	return sshAgentCleanup, nil
}

func readInstallerLog(m *InstallManager, scrubInstallLog bool) (string, error) {
	m.log.Infoln("saving installer output")

	if _, err := os.Stat(installerConsoleLogFilePath); os.IsNotExist(err) {
		m.log.WithField("path", installerConsoleLogFilePath).Error("installer log file does not exist")
		return "", err
	}

	logBytes, err := os.ReadFile(installerConsoleLogFilePath)
	if err != nil {
		m.log.WithError(err).WithField("path", installerConsoleLogFilePath).Error("error reading log file")
		return "", err
	}

	consoleLog := string(logBytes)
	if scrubInstallLog {
		consoleLog = cleanupLogOutput(consoleLog)
	}

	m.log.Debugf("installer console log: %v", consoleLog)

	return consoleLog, nil
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

func (m *InstallManager) isBootstrapComplete() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "./openshift-install", "wait-for", "bootstrap-complete")
	cmd.Dir = m.WorkDir
	return cmd.Run() == nil
}

func uploadAdminKubeconfig(m *InstallManager) (*corev1.Secret, error) {
	m.log.Infoln("uploading admin kubeconfig")

	var kubeconfigSecret *corev1.Secret

	fullPath := filepath.Join(m.WorkDir, adminKubeConfigRelativePath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		m.log.WithField("path", fullPath).Error("admin kubeconfig file does not exist")
		return nil, err
	}

	kubeconfigBytes, err := os.ReadFile(fullPath)
	if err != nil {
		m.log.WithError(err).WithField("path", fullPath).Error("error reading admin kubeconfig file")
		return nil, err
	}

	kubeconfigSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(adminKubeConfigSecretStringTemplate, m.ClusterProvisionName),
			Namespace: m.Namespace,
		},
		Data: map[string][]byte{
			"kubeconfig": kubeconfigBytes,
		},
	}

	m.log.WithField("derivedObject", kubeconfigSecret.Name).Debug("Setting labels on derived object")
	kubeconfigSecret.Labels = k8slabels.AddLabel(kubeconfigSecret.Labels, constants.ClusterProvisionNameLabel, m.ClusterProvision.Name)
	kubeconfigSecret.Labels = k8slabels.AddLabel(kubeconfigSecret.Labels, constants.SecretTypeLabel, constants.SecretTypeKubeConfig)

	provisionGVK, err := apiutil.GVKForObject(m.ClusterProvision, scheme.Scheme)
	if err != nil {
		m.log.WithError(err).Errorf("error getting GVK for provision")
		return nil, err
	}

	kubeconfigSecret.OwnerReferences = []metav1.OwnerReference{{
		APIVersion:         provisionGVK.GroupVersion().String(),
		Kind:               provisionGVK.Kind,
		Name:               m.ClusterProvision.Name,
		UID:                m.ClusterProvision.UID,
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}

	if err := createWithRetries(kubeconfigSecret, m); err != nil {
		return nil, err
	}

	return kubeconfigSecret, nil
}

func loadAdminPassword(m *InstallManager) (string, error) {
	m.log.Infoln("loading admin password")

	var password string
	fullPath := filepath.Join(m.WorkDir, adminPasswordRelativePath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		m.log.WithField("path", fullPath).Error("admin password file does not exist")
		return "", err
	}

	passwordBytes, err := os.ReadFile(fullPath)
	if err != nil {
		m.log.WithError(err).WithField("path", fullPath).Error("error reading admin password file")
		return "", err
	}

	// Need to trim trailing newlines from the password
	password = strings.TrimSpace(string(passwordBytes))

	return password, nil
}

func uploadAdminPassword(m *InstallManager) (*corev1.Secret, error) {
	m.log.Infoln("uploading admin username/password")

	// Need to trim trailing newlines from the password
	password, err := m.loadAdminPassword(m)
	if err != nil {
		return nil, err
	}

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
	s.Labels = k8slabels.AddLabel(s.Labels, constants.ClusterProvisionNameLabel, m.ClusterProvision.Name)
	s.Labels = k8slabels.AddLabel(s.Labels, constants.SecretTypeLabel, constants.SecretTypeKubeAdminCreds)

	provisionGVK, err := apiutil.GVKForObject(m.ClusterProvision, scheme.Scheme)
	if err != nil {
		m.log.WithError(err).Errorf("error getting GVK for provision")
		return nil, err
	}

	s.OwnerReferences = []metav1.OwnerReference{{
		APIVersion:         provisionGVK.GroupVersion().String(),
		Kind:               provisionGVK.Kind,
		Name:               m.ClusterProvision.Name,
		UID:                m.ClusterProvision.UID,
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}

	if err := createWithRetries(s, m); err != nil {
		return nil, err
	}

	return s, nil
}

func createWithRetries(obj client.Object, m *InstallManager) error {
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

func (m *InstallManager) cleanupAdminKubeconfigSecret(provisionName, provisionNamespace string) error {
	// find/delete any previous admin kubeconfig secret
	namespacedName := types.NamespacedName{
		Name:      fmt.Sprintf(adminKubeConfigSecretStringTemplate, provisionName),
		Namespace: provisionNamespace,
	}
	if err := m.deleteAnyExistingObject(namespacedName, &corev1.Secret{}); err != nil {
		m.log.WithError(err).Error("failed to fetch/delete any pre-existing kubeconfig secret")
		return err
	}

	return nil
}

func (m *InstallManager) cleanupAdminPasswordSecret(provisionName, provisionNamespace string) error {
	// find/delete any previous admin password secret
	namespacedName := types.NamespacedName{
		Name:      fmt.Sprintf(adminPasswordSecretStringTemplate, provisionName),
		Namespace: provisionNamespace,
	}
	if err := m.deleteAnyExistingObject(namespacedName, &corev1.Secret{}); err != nil {
		m.log.WithError(err).Error("failed to fetch/delete any pre-existing admin password secret")
		return err
	}

	return nil
}

// deleteAnyExistingObject will look for any object that exists that matches the passed in 'obj' and will delete it if it exists
func (m *InstallManager) deleteAnyExistingObject(namespacedName types.NamespacedName, obj hivev1.MetaRuntimeObject) error {
	_, err := resource.DeleteAnyExistingObject(m.DynamicClient, namespacedName, obj, m.log)
	return err
}

func waitForProvisioningStage(m *InstallManager) error {
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
	hc, err := rest.HTTPClientFor(config)
	if err != nil {
		return errors.Wrap(err, "could not generate http client for config")
	}
	restClient, err := apiutil.RESTClientForGVK(gvk, false, config, scheme.Codecs, hc)
	if err != nil {
		return errors.Wrap(err, "could not create REST client")
	}

	_, err = clientwatch.UntilWithSync(
		waitContext,
		cache.NewListWatchFromClient(
			restClient,
			"clusterprovisions",
			m.ClusterProvision.Namespace,
			fields.OneTermEqualSelector("metadata.name", m.ClusterProvision.Name),
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

func (m *InstallManager) writeSSHKnownHosts(homeDir string, knownHosts []string) error {
	sshDir := filepath.Join(homeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		m.log.WithError(err).Errorf("error creating %s directory", sshDir)
		return err
	}
	allKnownHosts := strings.Join(knownHosts, "\n")
	if err := os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte(allKnownHosts), 0644); err != nil {
		m.log.WithError(err).Error("error writing ssh known_hosts")
		return err
	}
	m.log.WithField("knownHosts", knownHosts).Infof("Wrote known hosts to %s/known_hosts", sshDir)
	return nil
}

type provisionMutation func(provision *hivev1.ClusterProvision)

func updateClusterProvisionWithRetries(m *InstallManager, mutation provisionMutation) error {
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// read in a fresh clusterprovision
		if err := m.loadClusterProvision(); err != nil {
			m.log.WithError(err).Warn("error reading in fresh clusterprovision")
			return err
		}

		// make the needed modifications to the clusterprovision
		mutation(m.ClusterProvision)

		if err := m.DynamicClient.Update(context.Background(), m.ClusterProvision); err != nil {
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
	// The console log may have carriage returns as well as newlines,
	// and they may be escaped (this especially happens with the
	// baremetal IPI installer when libvirt.so emits errors). Wrap the
	// string in quotes, then use Unquote() to interpret it as a go
	// literal to unescape any escaped characters. If that process
	// fails because the reformatted string doesn't match the expected
	// syntax, ignore the error and apply the later cleanning steps to
	// the original string.
	cleanedString, err := strconv.Unquote(fmt.Sprintf("\"%s\"", fullLog))
	if err != nil {
		cleanedString = fullLog
	}

	cleanedString = multiLineRedactLinesWithPassword.ReplaceAllString(cleanedString, "REDACTED LINE OF OUTPUT")

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
	pullSecretData, err := os.ReadFile(pullSecretFile)
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

func (m *InstallManager) overrideCredentialsModeOnInstallConfig(credentialsMode string, cd *hivev1.ClusterDeployment) error {
	clusterConfigPath := filepath.Join(m.WorkDir, "manifests", clusterConfigYAML)
	clusterConfigBytes, err := os.ReadFile(clusterConfigPath)
	if err != nil {
		return errors.Wrap(err, "error reading cluster config")
	}

	clusterConfig := &corev1.ConfigMap{}
	if err := yaml.Unmarshal(clusterConfigBytes, clusterConfig); err != nil {
		return errors.Wrap(err, "failed to decode cluster config")
	}

	data, ok := clusterConfig.Data[installConfigKeyName]
	if !ok {
		return fmt.Errorf("did not find key %s in data of cluster-config configmap", installConfigKeyName)
	}

	installConfig := &installertypes.InstallConfig{}
	if err := yaml.Unmarshal([]byte(data), installConfig); err != nil {
		return errors.Wrap(err, "failed to unmarshal install config")
	}

	installConfig.CredentialsMode = installertypes.CredentialsMode(credentialsMode)

	installConfigBytes, err := yaml.Marshal(installConfig)
	if err != nil {
		return errors.Wrap(err, "error marshalling install config")
	}

	clusterConfig.Data[installConfigKeyName] = string(installConfigBytes)

	clusterConfigBytes, err = yaml.Marshal(clusterConfig)
	if err != nil {
		return errors.Wrap(err, "error marshalling cluster config")
	}

	if err := os.WriteFile(clusterConfigPath, clusterConfigBytes, 0644); err != nil {
		return errors.Wrap(err, "error writing cluster-config.yaml")
	}

	m.log.WithField("newMode", credentialsMode).Info("overrode effective in-cluster credentials mode")

	if cd.Spec.Platform.Azure == nil {
		return nil
	}

	//If Azure, need fill up azure_resource_prefix and azure_resourcegroup if 99_cloud-creds-secret.yaml exists
	m.log.Info("patch 99_cloud-creds-secret.yaml when platform is azure")

	//Read 99_cloud-creds-secret.yaml
	overrideCredsPath := filepath.Join(m.WorkDir, "manifests", overrideCredsYAML)
	overrideCredsBytes, err := os.ReadFile(overrideCredsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			m.log.Info("99_cloud-creds-secret.yaml doesn't exist, continue installation")
			return nil
		} else {
			return errors.Wrap(err, "error reading 99_cloud-creds-secret.yaml")
		}
	}

	//Read infrastructureName/resourceGroupName from manifests cluster-infrastructure-02-config.yml
	clusterInfraConfigPath := filepath.Join(m.WorkDir, "manifests", clusterInfraConfigYAML)
	clusterInfraConfigBytes, err := os.ReadFile(clusterInfraConfigPath)
	if err != nil {
		return errors.Wrap(err, "error reading manifests cluster-infrastructure-02-config.yml")
	}

	modifiedBytes, err := patchAzureOverrideCreds(overrideCredsBytes, clusterInfraConfigBytes, installConfig.Platform.Azure.Region)
	if err != nil {
		return errors.Wrap(err, "error patching 99_cloud-creds-secret.yaml")
	}

	if modifiedBytes != nil {
		err = os.WriteFile(overrideCredsPath, *modifiedBytes, 0644)
		if err != nil {
			return errors.Wrap(err, "error writing 99_cloud-creds-secret.yaml")
		}
		m.log.Info("patched 99_cloud-creds-secret.yaml successfully")
	}

	return nil
}

func patchAzureOverrideCreds(overrideCredsBytes, clusterInfraConfigBytes []byte, region string) (*[]byte, error) {
	overrideCredsJson, err := yaml.YAMLToJSON(overrideCredsBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error converting 99_cloud-creds-secret.yaml to json")
	}

	regionInput := gjson.Get(string(overrideCredsJson), `data.azure_region`).String()
	infraNameInput := gjson.Get(string(overrideCredsJson), `data.azure_resource_prefix`).String()
	resourceGroupNameInput := gjson.Get(string(overrideCredsJson), `data.azure_resourcegroup`).String()

	//Return error if any of infraName/resourceGroupName already exist
	if infraNameInput != "" || resourceGroupNameInput != "" {
		return nil, fmt.Errorf("azure_resource_prefix/azure_resourcegroup already exists in %s", overrideCredsYAML)
	}

	//Return error if region exists but different to the one in cd.spec
	if regionInput != "" {
		regionDecode, err := base64.StdEncoding.DecodeString(regionInput)
		if err != nil {
			return nil, errors.Wrap(err, "error decoding data.azure_region")
		}
		if region != string(regionDecode) {
			return nil, fmt.Errorf("azure_region=%s already exists in 99_cloud-creds-secret.yaml but different to region=%s in ClusterDeployment.spec", string(regionDecode), region)
		}
	}

	clusterInfraConfigJson, err := yaml.YAMLToJSON(clusterInfraConfigBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error converting cluster-infrastructure-02-config.yml to json")
	}

	infraName := gjson.Get(string(clusterInfraConfigJson), `status.infrastructureName`).String()
	resourceGroupName := gjson.Get(string(clusterInfraConfigJson), `status.platformStatus.azure.resourceGroupName`).String()
	if region == "" || infraName == "" || resourceGroupName == "" {
		return nil, fmt.Errorf("error reading from manifests, region=%s, clusterInfraConfig=%s", region, string(clusterInfraConfigJson))
	}

	regionBase64 := interface{}(base64.StdEncoding.EncodeToString([]byte(region)))
	infraNameBase64 := interface{}(base64.StdEncoding.EncodeToString([]byte(infraName)))
	resourceGroupNameBase64 := interface{}(base64.StdEncoding.EncodeToString([]byte(resourceGroupName)))

	ops := yamlpatch.Patch{
		yamlpatch.Operation{
			Op:    "add", //ok even if /data/azure_region already exists
			Path:  yamlpatch.OpPath("/data/azure_region"),
			Value: yamlpatch.NewNode(&regionBase64),
		},
		yamlpatch.Operation{
			Op:    "add",
			Path:  yamlpatch.OpPath("/data/azure_resource_prefix"),
			Value: yamlpatch.NewNode(&infraNameBase64),
		},
		yamlpatch.Operation{
			Op:    "add",
			Path:  yamlpatch.OpPath("/data/azure_resourcegroup"),
			Value: yamlpatch.NewNode(&resourceGroupNameBase64),
		},
	}

	// Apply patch to 99_cloud-creds-secret.yaml
	modifiedBytes, err := ops.Apply(overrideCredsBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error applying patch on 99_cloud-creds-secret.yaml")
	}
	return &modifiedBytes, nil
}
