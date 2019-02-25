package installmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"

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
	sleepBetweenRetries                 = time.Second * 5
)

// InstallManager coordinates executing the openshift-install binary, modifying
// generated assets, and uploading artifacts to the kube API after completion.
type InstallManager struct {
	log                           log.FieldLogger
	LogLevel                      string
	WorkDir                       string
	InstallConfig                 string
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
			im.DynamicClient, err = getClient()
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
	flags.StringVar(&im.Region, "region", "us-east-1", "Region installing into")
	// This is required due to how we have to share volume and mount in our install config. The installer also deletes the workdir copy.
	flags.StringVar(&im.InstallConfig, "install-config", "/installconfig/install-config.yaml", "location of install-config.yaml to copy into work-dir")
	return cmd
}

// Complete sets remaining fields on the InstallManager based on command options and arguments.
func (m *InstallManager) Complete(args []string) error {
	// Connect up structure's function pointers
	m.uploadClusterMetadata = uploadClusterMetadata
	m.updateClusterDeploymentStatus = updateClusterDeploymentStatus
	m.uploadAdminKubeconfig = uploadAdminKubeconfig
	m.uploadAdminPassword = uploadAdminPassword
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

	dstInstallConfig := filepath.Join(m.WorkDir, "install-config.yaml")
	m.log.Debugf("copying %s to %s", m.InstallConfig, dstInstallConfig)
	err = m.copyFile(m.InstallConfig, dstInstallConfig)
	if err != nil {
		m.log.WithError(err).Fatalf("error copying install config from %s to %s",
			m.InstallConfig, dstInstallConfig)
	}

	// If the cluster deployment has a clusterID set, this implies we failed an install
	// and are re-trying. Cleanup any resources that may have been provisioned.
	if err := m.cleanupBeforeInstall(cd); err != nil {
		m.log.WithError(err).Error("error while trying to preemptively clean up")
		return err
	}

	// Generate installer assets we need to modify or upload.
	if err := m.generateAssets(cd); err != nil {
		return err
	}

	// We should now have cluster metadata.json we can parse for the clusterID. If we fail
	// to extract the ID and upload it, this is a critical failure and we should restart.
	// No cloud resources have been provisioned at this point.
	if err := m.uploadClusterMetadata(cd, m); err != nil {
		return err
	}

	installErr := m.provisionCluster(cd)
	if installErr != nil {
		m.log.WithError(installErr).Error("error running openshift-install")
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

	return nil
}

func (m *InstallManager) waitForInstallerBinaries() {
	waitForFiles := []string{
		filepath.Join(m.WorkDir, "openshift-install"),
	}
	m.log.Infof("waiting for install binaries to be available: %v", waitForFiles)

	// Infinitely wait, we'll let the job terminate if we run over deadline:
	for _, p := range waitForFiles {
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
	m.log.Infof("all install binaries found, ready to proceed with install")
}

// cleanupBeforeInstall allows recovering from an installation error and allows retries
func (m *InstallManager) cleanupBeforeInstall(cd *hivev1.ClusterDeployment) error {
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
		if err := m.runUninstaller(m.ClusterName, m.Region, cd.Status.InfraID, m.log); err != nil {
			return err
		}
		// Cleanup successful, we must now clear the clusterID and infraID from status:
		cd.Status.ClusterID = ""
		cd.Status.InfraID = ""
		if err := m.DynamicClient.Status().Update(context.Background(), cd); err != nil {
			// This will cause a job re-try, which is fine as we'll just try to cleanup and
			// find nothing.
			m.log.WithError(err).Error("error clearing cluster ID in status")
			return err
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
	m.log.Info("running openshift-install create ignition-configs")
	err := m.runOpenShiftInstallCommand([]string{"create", "ignition-configs", "--dir", m.WorkDir, "--log-level", "debug"})
	if err != nil {
		m.log.WithError(err).Error("error generating installer assets")
		return err
	}
	return nil
}

// provisionCluster invokes the openshift-install create cluster command to provision resources
// in the cloud.
func (m *InstallManager) provisionCluster(cd *hivev1.ClusterDeployment) error {

	m.log.Info("running openshift-install create cluster")
	err := m.runOpenShiftInstallCommand([]string{"create", "cluster", "--dir", m.WorkDir, "--log-level", "debug"})
	if err != nil {
		m.log.WithError(err).Errorf("error provisioning cluster")
		return err
	}
	return nil
}

func (m *InstallManager) runOpenShiftInstallCommand(args []string) error {
	cmd := exec.Command(filepath.Join(m.WorkDir, "openshift-install"), args...)

	// Copy all stdout/stderr output from the child process:
	var stdoutBuf, stderrBuf bytes.Buffer
	var errStdout, errStderr error
	childStdout, _ := cmd.StdoutPipe()
	childStderr, _ := cmd.StderrPipe()
	stdout := io.MultiWriter(os.Stdout, &stdoutBuf)
	stderr := io.MultiWriter(os.Stderr, &stderrBuf)
	err := cmd.Start()
	if err != nil {
		log.WithError(err).Fatal("command start failed")
	}

	go func() {
		_, errStdout = io.Copy(stdout, childStdout)
	}()

	go func() {
		_, errStderr = io.Copy(stderr, childStderr)
	}()

	err = cmd.Wait()
	// Log errors capturing output but do not treat them as fatal.
	if errStdout != nil {
		log.WithError(errStdout).Error("error capturing openshift-install stdout")
	}
	if errStderr != nil {
		log.WithError(errStderr).Error("error capturing openshift-install stderr")
	}

	// Return any error from the command itself:
	return err
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

	cd.Status.ClusterID = md.ClusterID
	cd.Status.InfraID = md.InfraID
	if cd.Status.InfraID == "" {
		m.log.Error("cluster metadata did not contain infraID")
		return fmt.Errorf("cluster metadata did not contain infraID")
	}

	controllerutils.FixupEmptyClusterVersionFields(&cd.Status.ClusterVersionStatus)
	err = m.DynamicClient.Status().Update(context.Background(), cd)
	if err != nil {
		m.log.WithError(err).Error("error updating cluster ID in status")
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

func uploadSecretWithRetries(s *corev1.Secret, m *InstallManager) error {

	secretUploaded := false
	var err error
	// try 10 times to save the admin kubeconfig with some sleeps between attempts
	for i := 1; i < 11; i++ {
		err = m.DynamicClient.Create(context.Background(), s)
		if err != nil {
			m.log.WithError(err).WithField("secretName", s.Name).Warningf("error creating secret (attempt %d)", i)
			time.Sleep(sleepBetweenRetries)
			continue
		}
		m.log.WithField("secretName", s.Name).Info("uploaded secret")
		secretUploaded = true
		break
	}

	if !secretUploaded {
		m.log.WithError(err).WithField("secretName", s.Name).Error("failed to save secret")
		return err
	}

	return nil
}

func updateClusterDeploymentStatus(cd *hivev1.ClusterDeployment, adminKubeconfigSecretName, adminPasswordSecretName string, m *InstallManager) error {
	// Update the cluster deployment status with a reference to the admin kubeconfig secret:
	m.log.Info("updating cluster deployment status")
	if adminKubeconfigSecretName != "" {
		cd.Status.AdminKubeconfigSecret = corev1.LocalObjectReference{Name: adminKubeconfigSecretName}
	}
	if adminPasswordSecretName != "" {
		cd.Status.AdminPasswordSecret = corev1.LocalObjectReference{Name: adminPasswordSecretName}
	}
	controllerutils.FixupEmptyClusterVersionFields(&cd.Status.ClusterVersionStatus)
	return m.DynamicClient.Status().Update(context.Background(), cd)
}

func (m *InstallManager) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Sync()
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

func getClient() (client.Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	cfg, err := kubeconfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	apis.AddToScheme(scheme.Scheme)
	dynamicClient, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, err
	}

	return dynamicClient, nil
}
