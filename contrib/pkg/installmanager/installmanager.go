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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	metadataRelativePath        = "metadata.json"
	adminKubeConfigRelativePath = "auth/kubeconfig"
	uuidKey                     = "tectonicClusterID"
	kubernetesKeyPrefix         = "kubernetes.io/cluster/"
)

// InstallManager coordinates executing the openshift-install binary, modifying
// generated assets, and uploading artifacts to the kube API after completion.
type InstallManager struct {
	log                   log.FieldLogger
	LogLevel              string
	WorkDir               string
	InstallConfig         string
	ClusterUUID           string
	Region                string
	ClusterName           string
	Namespace             string
	DynamicClient         client.Client
	SkipPreInstallCleanup bool
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
	m.waitForInstallerBinaries()

	dstInstallConfig := filepath.Join(m.WorkDir, "install-config.yml")
	m.log.Debugf("copying %s to %s", m.InstallConfig, dstInstallConfig)
	err := m.copyFile(m.InstallConfig, dstInstallConfig)
	if err != nil {
		m.log.WithError(err).Fatalf("error copying install config from %s to %s",
			m.InstallConfig, dstInstallConfig)
	}

	installErr := m.runInstaller()
	if installErr != nil {
		m.log.WithError(installErr).Error("error running openshift-install")
	}

	cd, err := m.loadClusterDeployment()
	if err != nil {
		m.log.WithError(err).Fatal("error looking up cluster deployment")
	}

	// Try to upload any artifacts that exist regardless if the install errored or not.
	// We may need to cleanup.
	if err := m.uploadClusterMetadata(cd); err != nil {
		m.log.WithError(err).Fatal("error uploading cluster metadata.json")
	}

	adminKubeconfigSecret, err := m.uploadAdminKubeconfig(cd)
	if err != nil {
		m.log.WithError(err).Fatal("error uploading admin kubeconfig")
	}

	if err := m.updateClusterDeploymentStatus(cd, adminKubeconfigSecret.Name); err != nil {
		m.log.WithError(err).Fatal("error updating cluster deployment status")
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
	// first cleanup any remnant cache files from a previous installation attempt
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

	// now run the uninstaller to clean up any cloud resources previously created
	filters := []awstagdeprovision.AWSFilter{
		{uuidKey: m.ClusterUUID},
		{kubernetesKeyPrefix + m.ClusterName: "owned"},
	}
	uninstaller := &awstagdeprovision.ClusterUninstaller{
		Filters:     filters,
		Region:      m.Region,
		ClusterName: m.ClusterName,
		Logger:      m.log,
	}

	err := uninstaller.Run()

	return err
}

func (m *InstallManager) runInstaller() error {

	if !m.SkipPreInstallCleanup {
		err := m.cleanupBeforeInstall()
		if err != nil {
			m.log.WithError(err).Error("error while trying to preemptively clean up")
			return err
		}
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

func (m *InstallManager) uploadClusterMetadata(cd *hivev1.ClusterDeployment) error {
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
			Name:      fmt.Sprintf("%s-metadata", m.ClusterName),
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

func (m *InstallManager) uploadAdminKubeconfig(cd *hivev1.ClusterDeployment) (*corev1.Secret, error) {
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
			Name:      fmt.Sprintf("%s-admin-kubeconfig", m.ClusterName),
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

	err = m.DynamicClient.Create(context.Background(), kubeconfigSecret)
	if err != nil {
		// TODO: what should happen if it already exists?
		m.log.WithError(err).Error("error creating admin kubeconfig secret")
		return nil, err
	}
	m.log.WithField("secretName", kubeconfigSecret.Name).Info("uploaded admin kubeconfig secret")

	return kubeconfigSecret, nil
}

func (m *InstallManager) updateClusterDeploymentStatus(cd *hivev1.ClusterDeployment, adminKubeconfigSecretName string) error {
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
