package resource

import (
	"fmt"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"

	"k8s.io/client-go/rest"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

const (
	defaultCacheDir = "/tmp"
	cacheDirEnvKey  = "CLI_CACHE_DIR"
)

// Helper contains configuration for apply and patch operations
type Helper struct {
	logger     log.FieldLogger
	cacheDir   string
	kubeconfig []byte
	restConfig *rest.Config
	getFactory func(namespace string) (cmdutil.Factory, error)
}

// NewHelperFromRESTConfig returns a new object that allows apply and patch operations
func NewHelperFromRESTConfig(restConfig *rest.Config, logger log.FieldLogger) *Helper {
	r := &Helper{
		logger:     logger,
		cacheDir:   getCacheDir(logger),
		restConfig: restConfig,
	}
	r.getFactory = r.getRESTConfigFactory
	return r
}

// NewHelper returns a new object that allows apply and patch operations
func NewHelper(kubeconfig []byte, logger log.FieldLogger) *Helper {
	r := &Helper{
		logger:     logger,
		cacheDir:   getCacheDir(logger),
		kubeconfig: kubeconfig,
	}
	r.getFactory = r.getKubeconfigFactory
	return r
}

func getCacheDir(logger log.FieldLogger) string {
	if envCacheDir := os.Getenv(cacheDirEnvKey); len(envCacheDir) > 0 {
		logger.WithField("dir", envCacheDir).WithField("env", cacheDirEnvKey).Debug("using cache directory from environment variable")
		return envCacheDir
	}
	logger.WithField("dir", defaultCacheDir).Debug("using default cache directory")
	return defaultCacheDir
}

func (r *Helper) createTempFile(prefix string, content []byte) (string, error) {
	r.logger.WithField("prefix", prefix).Debug("creating temporary file")
	f, err := ioutil.TempFile(r.cacheDir, prefix)
	if err != nil {
		r.logger.WithError(err).WithField("prefix", prefix).Error("cannot create temporary file")
		return "", fmt.Errorf("cannot create temporary file: %v", err)
	}
	defer f.Close()
	if _, err = f.Write(content); err != nil {
		r.logger.WithError(err).WithField("file", f.Name()).Error("cannot write to temporary file")
		return "", fmt.Errorf("cannot write to temporary file: %v", err)
	}
	r.logger.WithField("file", f.Name()).Debug("created temporary file")
	return f.Name(), nil
}

func (r *Helper) deleteTempFile(name string) error {
	r.logger.WithField("file", name).Debugf("deleting temporary file")
	err := os.Remove(name)
	if err != nil {
		r.logger.WithError(err).WithField("file", name).Error("cannot delete temp file")
		return fmt.Errorf("cannot delete temporary file %s: %v", name, err)
	}
	return err
}
