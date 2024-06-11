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

	// restConfigClientGetterKey is an optional field, and iff set will be used as a cache key
	// for reusing restConfigClientGetters (see restconfig_factory.go)
	restConfigClientGetterKey string
	// restConfigClientGetterVersion is an optional field, and iff set will be used as a cache version
	// for invalidating restConfigClientGetters (see restconfig_factory.go)
	restConfigClientGetterVersion string
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

type HelperOption func(*helper)

func WithKubeconfig(kubeconfig []byte) HelperOption {
	return func(r *helper) {
		r.kubeconfig = kubeconfig
		r.getFactory = r.getKubeconfigFactory
	}
}

func WithRESTConfig(restConfig *rest.Config) HelperOption {
	return func(r *helper) {
		r.restConfig = restConfig
		r.getFactory = r.getRESTConfigFactory
	}
}

func WithMetrics(controllerName hivev1.ControllerName) HelperOption {
	return func(r *helper) {
		r.metricsEnabled = true
		r.controllerName = controllerName
	}
}

func WithCacheKeyAndVersion(restConfigClientGetterKey string, restConfigClientGetterVersion string) HelperOption {
	return func(r *helper) {
		r.restConfigClientGetterKey = restConfigClientGetterKey
		r.restConfigClientGetterVersion = restConfigClientGetterVersion
	}
}

// NewHelper returns a new object that allows apply and patch operations
func NewHelper(logger log.FieldLogger, options ...HelperOption) (Helper, error) {
	r := &helper{
		logger:   logger,
		cacheDir: getCacheDir(logger),
	}

	for _, f := range options {
		f(r)
	}

	err := r.cacheOpenAPISchema()
	return r, err
}

func getCacheDir(logger log.FieldLogger) string {
	if envCacheDir := os.Getenv(cacheDirEnvKey); len(envCacheDir) > 0 {
		return envCacheDir
	}
	return defaultCacheDir
}
