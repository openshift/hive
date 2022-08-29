package resource

import (
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/openapi"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
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
	controllerName hivev1.ControllerName
	remote         bool
	kubeconfig     []byte
	restConfig     *rest.Config
	getFactory     func(namespace string) (cmdutil.Factory, error)
	openAPISchema  openapi.Resources
}

// cacheOpenAPISchema builds the very expensive OpenAPISchema (>3s commonly) once, and stores
// the resulting schema on the helper for re-use, particularly in Apply when run many times against
// one cluster.
func (r *helper) cacheOpenAPISchema() error {
	f, err := r.getFactory("")
	if err != nil {
		return errors.Wrap(err, "could not get factory")
	}
	r.openAPISchema, err = f.OpenAPISchema()
	if err != nil {
		return errors.Wrap(err, "error getting OpenAPISchema")
	}
	return nil
}

// NewHelperFromRESTConfig returns a new object that allows apply and patch operations
func NewHelperFromRESTConfig(restConfig *rest.Config, logger log.FieldLogger) (Helper, error) {
	r := &helper{
		logger:     logger,
		cacheDir:   getCacheDir(logger),
		restConfig: restConfig,
	}
	r.getFactory = r.getRESTConfigFactory
	err := r.cacheOpenAPISchema()
	return r, err
}

// NewHelperWithMetricsFromRESTConfig returns a new object that allows apply and patch operations, with metrics tracking enabled.
func NewHelperWithMetricsFromRESTConfig(restConfig *rest.Config, controllerName hivev1.ControllerName, logger log.FieldLogger) (Helper, error) {
	r := &helper{
		logger:         logger,
		metricsEnabled: true,
		controllerName: controllerName,
		cacheDir:       getCacheDir(logger),
		restConfig:     restConfig,
	}
	r.getFactory = r.getRESTConfigFactory
	err := r.cacheOpenAPISchema()
	return r, err
}

// NewHelper returns a new object that allows apply and patch operations
func NewHelper(kubeconfig []byte, logger log.FieldLogger) (Helper, error) {
	r := &helper{
		logger:     logger,
		cacheDir:   getCacheDir(logger),
		kubeconfig: kubeconfig,
	}
	r.getFactory = r.getKubeconfigFactory
	err := r.cacheOpenAPISchema()
	return r, err
}

func getCacheDir(logger log.FieldLogger) string {
	if envCacheDir := os.Getenv(cacheDirEnvKey); len(envCacheDir) > 0 {
		return envCacheDir
	}
	return defaultCacheDir
}
