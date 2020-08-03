package resource

import (
	"os"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
)

const (
	defaultCacheDir = "/tmp"
	cacheDirEnvKey  = "CLI_CACHE_DIR"
)

// Helper contains configuration for apply and patch operations
type Helper struct {
	logger         log.FieldLogger
	cacheDir       string
	metricsEnabled bool
	controllerName string
	remote         bool
	kubeconfig     []byte
	restConfig     *rest.Config
	factory        *namespacedFactory
}

// NewHelperFromRESTConfig returns a new object that allows apply and patch operations
func NewHelperFromRESTConfig(restConfig *rest.Config, logger log.FieldLogger) (*Helper, error) {
	return newHelperFromRESTConfig(restConfig, false, "", logger)
}

// NewHelperWithMetricsFromRESTConfig returns a new object that allows apply and patch operations, with metrics tracking enabled.
func NewHelperWithMetricsFromRESTConfig(restConfig *rest.Config, controllerName string, logger log.FieldLogger) (*Helper, error) {
	return newHelperFromRESTConfig(restConfig, true, controllerName, logger)
}

func newHelperFromRESTConfig(restConfig *rest.Config, metricsEnabled bool, controllerName string, logger log.FieldLogger) (*Helper, error) {
	r := &Helper{
		logger:         logger,
		metricsEnabled: metricsEnabled,
		controllerName: controllerName,
		cacheDir:       getCacheDir(logger),
		restConfig:     restConfig,
	}
	f, err := r.getRESTConfigFactory()
	if err != nil {
		return nil, err
	}
	r.factory = f
	return r, nil
}

// NewHelperWithMetrics returns a new object that allows apply and patch operations, with metrics tracking enabled.
func NewHelperWithMetrics(kubeconfig []byte, controllerName string, remote bool, logger log.FieldLogger) (*Helper, error) {
	return newHelperFromKubeconfig(kubeconfig, true, controllerName, logger)
}

// NewHelper returns a new object that allows apply and patch operations
func NewHelper(kubeconfig []byte, logger log.FieldLogger) (*Helper, error) {
	return newHelperFromKubeconfig(kubeconfig, false, "", logger)
}

func newHelperFromKubeconfig(kubeconfig []byte, metricEnabled bool, controllerName string, logger log.FieldLogger) (*Helper, error) {
	r := &Helper{
		logger:         logger,
		metricsEnabled: metricEnabled,
		controllerName: controllerName,
		cacheDir:       getCacheDir(logger),
		kubeconfig:     kubeconfig,
		remote:         true,
	}
	f, err := r.getKubeconfigFactory()
	if err != nil {
		return nil, err
	}
	r.factory = f
	return r, nil
}

func getCacheDir(logger log.FieldLogger) string {
	if envCacheDir := os.Getenv(cacheDirEnvKey); len(envCacheDir) > 0 {
		return envCacheDir
	}
	return defaultCacheDir
}
