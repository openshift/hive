package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/util/slice"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/resource"
)

const (
	caTrustDir = "/etc/pki/ca-trust/source/anchors/"
)

type releasePayload struct {
	PullSpec string `json:"pullSpec"`
}

func DetermineReleaseImageFromSource(sourceURL string) (string, error) {
	resp, err := http.Get(sourceURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	payload := &releasePayload{}
	err = json.Unmarshal(data, payload)
	if err != nil {
		return "", err
	}
	return payload.PullSpec, nil
}

func GetResourceHelper(logger log.FieldLogger) (resource.Helper, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		logger.WithError(err).Error("Cannot get client config")
		return nil, err
	}
	return resource.NewHelperFromRESTConfig(cfg, logger)
}

func DefaultNamespace() (string, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	ns, _, err := kubeconfig.Namespace()
	return ns, err
}

func GetPullSecret(logger log.FieldLogger, pullSecret string, pullSecretFile string) (string, error) {
	envPullSecret := os.Getenv("PULL_SECRET")
	if len(envPullSecret) > 0 {
		return envPullSecret, nil
	}
	if len(pullSecret) > 0 {
		return pullSecret, nil
	}
	if len(pullSecretFile) > 0 {
		data, err := os.ReadFile(pullSecretFile)
		if err != nil {
			logger.Error("Cannot read pull secret file")
			return "", err
		}
		pullSecret = strings.TrimSpace(string(data))
		return pullSecret, nil
	}
	return "", nil
}

func NewLogger(logLevel string) (*log.Entry, error) {

	// Set log level
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		log.WithError(err).Error("cannot parse log level")
		return nil, err
	}

	logger := log.NewEntry(&log.Logger{
		Out: os.Stdout,
		Formatter: &log.TextFormatter{
			FullTimestamp: true,
		},
		Hooks: make(log.LevelHooks),
		Level: level,
	})

	// Decorate with additional log fields, if requested
	logger = utils.AddLogFields(utils.StringLogTagger{S: os.Getenv(constants.AdditionalLogFieldsEnvVar)}, logger)

	return logger, nil
}

// LoadSecretOrDie looks for environment variables named CLUSTERDEPLOYMENT_NAMESPACE and `secretName`.
// If either is not found, this indicates we are not supposed to use this mode and we return nil. Otherwise, we
// attempt to load and return the designated secret. We panic if the secret fails to load for any reason.
func LoadSecretOrDie(c client.Client, secretNameEnvKey string) *corev1.Secret {
	secret := &corev1.Secret{}
	if !loadOrDie(c, secretNameEnvKey, secret) {
		return nil
	}
	return secret
}

// LoadConfigMapOrDie looks for environment variables named CLUSTERDEPLOYMENT_NAMESPACE and `secretName`.
// If either is not found, this indicates we are not supposed to use this mode and we return nil. Otherwise, we
// attempt to load and return the designated ConfigMap. We panic if the ConfigMap fails to load for any reason.
func LoadConfigMapOrDie(c client.Client, cmNameEnvKey string) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{}
	if !loadOrDie(c, cmNameEnvKey, cm) {
		return nil
	}
	return cm
}

func loadOrDie(c client.Client, nameEnvKey string, obj client.Object) bool {
	ns, name := os.Getenv("CLUSTERDEPLOYMENT_NAMESPACE"), os.Getenv(nameEnvKey)
	if ns == "" || name == "" {
		return false
	}
	logger := log.
		WithField("namespace", ns).
		WithField("name", name).
		WithField("type", fmt.Sprintf("%T", obj))

	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: name}, obj); err != nil {
		logger.WithError(err).Fatal("Failed to load object")
	}
	logger.Info("Using loaded object")
	return true

}

// ProjectToDir simulates what happens when you mount a secret or configmap as a volume on a pod, creating
// files named after each key under `dir` and populating them with the contents represented by the values.
func ProjectToDir(obj client.Object, dir string, keys ...string) {
	write := func(filename string, bytes []byte) {
		if len(keys) != 0 && !slice.ContainsString(keys, filename, nil) {
			// Skip this key
			return
		}
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, bytes, 0400); err != nil {
			log.WithError(err).WithField("path", path).Fatal("Failed to write file")
		}
	}
	switch o := obj.(type) {
	case *corev1.ConfigMap:
		// ConfigMaps have data in two places:
		// Data has string values, so we have to cast them.
		for k, v := range o.Data {
			write(k, ([]byte)(v))
		}
		// BinaryData's values are already []byte
		for k, v := range o.BinaryData {
			write(k, v)
		}
	case *corev1.Secret:
		for k, v := range o.Data {
			write(k, v)
		}
	case nil:
		log.Fatal("Can't project nil object to directory")
	default:
		log.WithField("type", fmt.Sprintf("%T", o)).WithField("name", o.GetName()).Fatal("Can't project object to directory")
	}
}

// InstallCerts copies the contents of `sourceDir` into the appropriate directory and updates the trust configuration.
// If `sourceDir` does not exist, this func is a no-op. Other errors (e.g. `sourceDir` is a file, or you don't have
// appropriate permissions) are fatal.
func InstallCerts(sourceDir string) {
	logger := log.WithField("certsDir", sourceDir)
	fi, err := os.Stat(sourceDir)
	if err != nil && os.IsNotExist(err) {
		logger.Info("Certs directory does not exist -- skipping")
		return
	}
	if !fi.Mode().IsDir() {
		logger.Fatal("Not a directory")
	}

	// No built-in recursive directory copy??
	b, err := exec.Command("cp", "-vr", sourceDir+"/.", caTrustDir).CombinedOutput()
	if err != nil {
		logger.WithError(err).WithField("output", string(b)).Fatal("failed to copy certs")
	}
	logger.WithField("output", string(b)).Info("copied certs")

	b, err = exec.Command("update-ca-trust").CombinedOutput()
	if err != nil {
		logger.WithError(err).WithField("output", string(b)).Fatal("failed to update CA trust")
	}
	logger.WithField("output", string(b)).Info("updated CA trust")
}
