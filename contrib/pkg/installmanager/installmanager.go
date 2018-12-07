package installmanager

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift/hive/contrib/pkg/awstagdeprovision"
)

const (
	// metadataRelativePath is the location of the installers cluster metadata file
	// relative to our WorkDir.
	metadataRelativePath                = "metadata.json"
	adminKubeConfigRelativePath         = "auth/kubeconfig"
	uuidKey                             = "openshiftClusterID"
	kubernetesKeyPrefix                 = "kubernetes.io/cluster/"
	metadataConfigmapStringTemplate     = "%s-metadata"
	adminKubeConfigSecretStringTemplate = "%s-admin-kubeconfig"
	sleepBetweenRetries                 = time.Second * 5
)

// InstallManager coordinates executing the openshift-install binary, modifying
// generated assets, and uploading artifacts to the kube API after completion.
type InstallManager struct {
	log                           log.FieldLogger
	LogLevel                      string
	WorkDir                       string
	InstallConfig                 string
	ClusterUUID                   string
	Region                        string
	ClusterName                   string
	Namespace                     string
	DynamicClient                 client.Client
	runUninstaller                func(clusterName, region, uuid string, logger log.FieldLogger) error
	uploadClusterMetadata         func(*hivev1.ClusterDeployment, *InstallManager) error
	updateClusterDeploymentStatus func(*hivev1.ClusterDeployment, string, *InstallManager) error
	uploadAdminKubeconfig         func(*hivev1.ClusterDeployment, *InstallManager) (*corev1.Secret, error)
}

// NewInstallManagerCommand is the entrypoint to create the 'install-manager' subcommand
func NewInstallManagerCommand() *cobra.Command {
	im := &InstallManager{}
	cmd := &cobra.Command{
		Use:   "install-manager NAMESPACE CLUSTER_NAME",
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
			im.Namespace, im.ClusterName = args[0], args[1]

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
				log.WithError(err).Error("runtime error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&im.LogLevel, "log-level", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.StringVar(&im.WorkDir, "work-dir", "/output", "directory to use for all input and output")
	flags.StringVar(&im.ClusterUUID, "cluster-uuid", "", "UUID to tag cloud resources with")
	flags.StringVar(&im.Region, "region", "us-east-1", "Region installing into")
	// This is required due to how we have to share volume and mount in our install config. The installer also deletes the workdir copy.
	flags.StringVar(&im.InstallConfig, "install-config", "/installconfig/install-config.yml", "location of install-config.yml to copy into work-dir")
	return cmd
}

// Complete sets remaining fields on the InstallManager based on command options and arguments.
func (m *InstallManager) Complete(args []string) error {
	// Connect up structure's function pointers
	m.uploadClusterMetadata = uploadClusterMetadata
	m.updateClusterDeploymentStatus = updateClusterDeploymentStatus
	m.uploadAdminKubeconfig = uploadAdminKubeconfig
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
	if m.ClusterUUID == "" {
		m.log.Fatalf("cluster-uuid parameter must be provided")
	}
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

	m.waitForInstallerBinaries()

	dstInstallConfig := filepath.Join(m.WorkDir, "install-config.yml")
	m.log.Debugf("copying %s to %s", m.InstallConfig, dstInstallConfig)
	err = m.copyFile(m.InstallConfig, dstInstallConfig)
	if err != nil {
		m.log.WithError(err).Fatalf("error copying install config from %s to %s",
			m.InstallConfig, dstInstallConfig)
	}

	installErr := m.runInstaller()
	if installErr != nil {
		m.log.WithError(installErr).Error("error running openshift-install")
	}

	// Try to upload any artifacts that exist regardless if the install errored or not.
	// We may need to cleanup.
	if err := m.uploadClusterMetadata(cd, m); err != nil {
		// non-fatal. log and continue.
		m.log.WithError(err).Warning("error uploading cluster metadata.json")
	}

	adminKubeconfigSecret, err := m.uploadAdminKubeconfig(cd, m)
	if err != nil {
		// fatal. without admin kubeconfig we have no cluster
		m.log.WithError(err).Error("error uploading admin kubeconfig")
		return fmt.Errorf("error trying to save admin kubeconfig: %v", err)
	}

	if err := m.updateClusterDeploymentStatus(cd, adminKubeconfigSecret.Name, m); err != nil {
		// non-fatal. log and continue.
		// will fix up any updates to the clusterdeployment in the periodic controller
		m.log.WithError(err).Warning("error updating cluster deployment status")
	}

	if installErr != nil {
		m.log.WithError(installErr).Fatal("failed due to install error")
	}

	return nil
}

func (m *InstallManager) waitForInstallerBinaries() {
	waitForFiles := []string{
		filepath.Join(m.WorkDir, "openshift-install"),
		filepath.Join(m.WorkDir, "terraform"),
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
func (m *InstallManager) cleanupBeforeInstall() error {
	if err := m.cleanupMetadataConfigmap(); err != nil {
		return err
	}

	if err := m.cleanupAdminKubeconfigSecret(); err != nil {
		return err
	}

	if err := m.cleanupTerraformFiles(); err != nil {
		return err
	}

	if err := m.runUninstaller(m.ClusterName, m.Region, m.ClusterUUID, m.log); err != nil {
		return err
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

func runUninstaller(clusterName, region, uuid string, logger log.FieldLogger) error {
	// run the uninstaller to clean up any cloud resources previously created
	filters := []awstagdeprovision.AWSFilter{
		{uuidKey: uuid},
		{kubernetesKeyPrefix + clusterName: "owned"},
	}
	uninstaller := &awstagdeprovision.ClusterUninstaller{
		Filters:     filters,
		Region:      region,
		ClusterName: clusterName,
		Logger:      logger,
	}

	err := uninstaller.Run()
	return err
}

func (m *InstallManager) runInstaller() error {

	if err := m.cleanupBeforeInstall(); err != nil {
		m.log.WithError(err).Error("error while trying to preemptively clean up")
		return err
	}

	m.log.Info("running openshift-install")
	cmd := exec.Command(filepath.Join(m.WorkDir, "openshift-install"), []string{"create", "cluster", "--dir", m.WorkDir, "--log-level", "debug"}...)

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
	m.log.Infoln("uploading cluster metadata")
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

	metadataCfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(metadataConfigmapStringTemplate, m.ClusterName),
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
	err := m.DynamicClient.Get(context.Background(), types.NamespacedName{Namespace: m.Namespace, Name: m.ClusterName}, cd)
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
			Name:      fmt.Sprintf(adminKubeConfigSecretStringTemplate, m.ClusterName),
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

	secretUploaded := false
	// try 10 times to save the admin kubeconfig with some sleeps between attempts
	for i := 1; i < 11; i++ {
		err = m.DynamicClient.Create(context.Background(), kubeconfigSecret)
		if err != nil {
			m.log.WithError(err).Warningf("error creating admmin kubeconfig secret (attempt %d)", i)
			time.Sleep(sleepBetweenRetries)
			continue
		}
		m.log.WithField("secretName", kubeconfigSecret.Name).Info("uploaded admin kubeconfig secret")
		secretUploaded = true
		break
	}

	if !secretUploaded {
		m.log.WithError(err).Error("failed to save admin kubeconfig secret")
		return nil, err
	}

	return kubeconfigSecret, nil
}

func updateClusterDeploymentStatus(cd *hivev1.ClusterDeployment, adminKubeconfigSecretName string, m *InstallManager) error {
	// Update the cluster deployment status with a reference to the admin kubeconfig secret:
	m.log.Info("updating cluster deployment status")
	cd.Status.AdminKubeconfigSecret = corev1.LocalObjectReference{Name: adminKubeconfigSecretName}
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
		Name:      fmt.Sprintf(metadataConfigmapStringTemplate, m.ClusterName),
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
		Name:      fmt.Sprintf(adminKubeConfigSecretStringTemplate, m.ClusterName),
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
