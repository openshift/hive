package installmanager

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// metadataRelativePath is the location of the installers cluster metadata file
	// relative to our WorkDir.
	metadataRelativePath        = "metadata.json"
	adminKubeConfigRelativePath = "auth/kubeconfig"
)

// InstallManager coordinates executing the openshift-install binary, modifying
// generated assets, and uploading artifacts to the kube API after completion.
type InstallManager struct {
	log         log.FieldLogger
	LogLevel    string
	WorkDir     string
	ClusterName string
	Namespace   string
	KubeClient  clientset.Interface
}

// NewInstallManagerCommand is the entrypoint to create the 'install-manager' subcommand
func NewInstallManagerCommand() *cobra.Command {
	opts := &InstallManager{}
	cmd := &cobra.Command{
		Use:   "install-manager NAMESPACE CLUSTER_NAME",
		Short: "Executes and oversees the openshift-installer.",
		Long:  "The Hive Install Manager runs the phases of the openshift-installer, edits generated assets before completing install, and monitors for artifacts that need to be uploaded back to Hive.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := opts.Complete(args); err != nil {
				log.WithError(err).Error("cannot complete command")
				return
			}

			if len(args) != 2 {
				cmd.Help()
				opts.log.Fatal("invalid command arguments")
			}
			// Parse the namespace/name for our cluster deployment:
			opts.Namespace, opts.ClusterName = args[0], args[1]

			if err := opts.Validate(); err != nil {
				log.WithError(err).Error("invalid command options")
				return
			}
			if err := opts.Run(); err != nil {
				log.WithError(err).Error("runtime error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.LogLevel, "log-level", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.StringVar(&opts.WorkDir, "work-dir", "/output", "directory to use for all input and output")
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
	return nil
}

// Run is the entrypoint to start the install process
func (m *InstallManager) Run() error {
	if err := m.getKubeClient(); err != nil {
		m.log.WithError(err).Fatal("error creating kube client")
	}

	m.waitForInstallerBinaries()

	installErr := m.runInstaller()
	if installErr != nil {
		m.log.WithError(installErr).Error("error running openshift-install")
	}

	// TODO: if we successfully installed, but fail after this point, that would be
	// pretty bad.

	// Try to upload any artifacts that exist regardless if the install errored or not.
	// We may need to cleanup.
	if err := m.uploadClusterMetadata(); err != nil {
		m.log.WithError(err).Fatal("error uploading cluster metadata.json")
	}

	if err := m.uploadAdminKubeconfig(); err != nil {
		m.log.WithError(err).Fatal("error uploading admin kubeconfig")
	}

	if installErr != nil {
		m.log.WithError(installErr).Fatal("failed due to install error")
	}

	return nil
}

func (m *InstallManager) getKubeClient() error {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	cfg, err := kubeconfig.ClientConfig()
	if err != nil {
		m.log.WithError(err).Error("cannot obtain client config")
		return err
	}
	m.KubeClient, err = clientset.NewForConfig(cfg)
	if err != nil {
		m.log.WithError(err).Error("cannot create kubernetes client from client config")
		return err
	}
	m.log.Debug("created kube client")
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

func (m *InstallManager) runInstaller() error {
	m.log.Info("running openshift-install")
	cmd := exec.Command(filepath.Join(m.WorkDir, "openshift-install"), []string{"cluster", "--dir", m.WorkDir, "--log-level", "debug"}...)

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

func (m *InstallManager) uploadClusterMetadata() error {
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
	_, err = m.KubeClient.CoreV1().ConfigMaps(m.Namespace).Create(metadataCfgMap)
	if err != nil {
		// TODO: what should happen if the configmap already exists?
		m.log.WithError(err).Error("error creating metadata configmap")
		return err
	}
	m.log.WithField("configMapName", metadataCfgMap.Name).Info("uploaded cluster metadata configmap")

	return nil
}

func (m *InstallManager) uploadAdminKubeconfig() error {
	m.log.Infoln("uploading admin kubeconfig")
	fullPath := filepath.Join(m.WorkDir, adminKubeConfigRelativePath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		m.log.WithField("path", fullPath).Error("admin kubeconfig file does not exist")
		return err
	}

	kubeconfigBytes, err := ioutil.ReadFile(fullPath)
	if err != nil {
		m.log.WithError(err).WithField("path", fullPath).Error("error reading admin kubeconfig file")
		return err
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
	_, err = m.KubeClient.CoreV1().Secrets(m.Namespace).Create(kubeconfigSecret)
	if err != nil {
		// TODO: what should happen if it already exists?
		m.log.WithError(err).Error("error creating admin kubeconfig secret")
		return err
	}
	m.log.WithField("secretName", kubeconfigSecret.Name).Info("uploaded admin kubeconfig secret")

	return nil
}
