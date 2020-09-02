package resource

import (
	"fmt"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const (
	defaultCacheDir = "/tmp"
	cacheDirEnvKey  = "CLI_CACHE_DIR"
)

//go:generate mockgen -source=./helper.go -destination=./mock/helper_generated.go -package=mock

type Helper interface {
	// Apply applies the given resource bytes to the target cluster specified by kubeconfig
	Apply(obj []byte) (ApplyResult, error)
	// ApplyRuntimeObject serializes an object and applies it to the target cluster specified by the kubeconfig.
	ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error)
	CreateOrUpdate(obj []byte) (ApplyResult, error)
	CreateOrUpdateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error)
	Create(obj []byte) (ApplyResult, error)
	CreateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error)
	// Info determines the name/namespace and type of the passed in resource bytes
	Info(obj []byte) (*Info, error)
	// Patch invokes the kubectl patch command with the given resource, patch and patch type
	Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType string) error
	Delete(apiVersion, kind, namespace, name string) error
}

// helper contains configuration for apply and patch operations
type helper struct {
	logger         log.FieldLogger
	cacheDir       string
	metricsEnabled bool
	controllerName string
	remote         bool
	kubeconfig     []byte
	restConfig     *rest.Config
	getFactory     func(namespace string) (cmdutil.Factory, error)
}

// NewHelperFromRESTConfig returns a new object that allows apply and patch operations
func NewHelperFromRESTConfig(restConfig *rest.Config, logger log.FieldLogger) Helper {
	r := &helper{
		logger:     logger,
		cacheDir:   getCacheDir(logger),
		restConfig: restConfig,
	}
	r.getFactory = r.getRESTConfigFactory
	return r
}

// NewHelperWithMetricsFromRESTConfig returns a new object that allows apply and patch operations, with metrics tracking enabled.
func NewHelperWithMetricsFromRESTConfig(restConfig *rest.Config, controllerName string, logger log.FieldLogger) Helper {
	r := &helper{
		logger:         logger,
		metricsEnabled: true,
		controllerName: controllerName,
		cacheDir:       getCacheDir(logger),
		restConfig:     restConfig,
	}
	r.getFactory = r.getRESTConfigFactory
	return r
}

// NewHelperWithMetrics returns a new object that allows apply and patch operations, with metrics tracking enabled.
func NewHelperWithMetrics(kubeconfig []byte, controllerName string, remote bool, logger log.FieldLogger) Helper {
	r := &helper{
		logger:         logger,
		controllerName: controllerName,
		metricsEnabled: true,
		cacheDir:       getCacheDir(logger),
		kubeconfig:     kubeconfig,
		remote:         true,
	}
	r.getFactory = r.getKubeconfigFactory
	return r
}

// NewHelper returns a new object that allows apply and patch operations
func NewHelper(kubeconfig []byte, logger log.FieldLogger) Helper {
	r := &helper{
		logger:     logger,
		cacheDir:   getCacheDir(logger),
		kubeconfig: kubeconfig,
	}
	r.getFactory = r.getKubeconfigFactory
	return r
}

func getCacheDir(logger log.FieldLogger) string {
	if envCacheDir := os.Getenv(cacheDirEnvKey); len(envCacheDir) > 0 {
		return envCacheDir
	}
	return defaultCacheDir
}

func (r *helper) createTempFile(prefix string, content []byte) (string, error) {
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
	return f.Name(), nil
}

func (r *helper) deleteTempFile(name string) error {
	err := os.Remove(name)
	if err != nil {
		r.logger.WithError(err).WithField("file", name).Error("cannot delete temp file")
		return fmt.Errorf("cannot delete temporary file %s: %v", name, err)
	}
	return err
}
