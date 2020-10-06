package utils

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/openshift/hive/pkg/resource"
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
	data, err := ioutil.ReadAll(resp.Body)
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
	helper := resource.NewHelperFromRESTConfig(cfg, logger)
	return helper, nil
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
		data, err := ioutil.ReadFile(pullSecretFile)
		if err != nil {
			logger.Error("Cannot read pull secret file")
			return "", err
		}
		pullSecret = strings.TrimSpace(string(data))
		return pullSecret, nil
	}
	return "", nil
}
