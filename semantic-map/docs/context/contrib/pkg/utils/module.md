# Module atlas

## Responsibility

Provides shared utility functions for `hiveutil` CLI commands, including Kubernetes client creation, secret/configmap loading, pull secret retrieval, release image lookup, certificate management, and filesystem projection of secrets/configmaps.

## Public Interface/API

**Types:**
- `ProjectToDirFileFilter` -- Callback type for filtering/renaming files when projecting secrets/configmaps to directories

**Functions:**
- `GetClient(fieldManager string) (client.WithWatch, error)` -- Creates a controller-runtime WithWatch client from the default kubeconfig
- `GetResourceHelper(controllerName, logger) (resource.Helper, error)` -- Creates a Hive resource helper from cluster config
- `DefaultNamespace() (string, error)` -- Returns the default namespace from kubeconfig
- `GetPullSecret(logger, pullSecret, pullSecretFile string) (string, error)` -- Resolves pull secret from env var, flag, or file
- `DetermineReleaseImageFromSource(sourceURL string) (string, error)` -- Fetches release image pull spec from a URL
- `NewLogger(logLevel string) (*log.Entry, error)` -- Creates a logrus logger with configurable level and additional log fields
- `LoadSecretOrDie(c client.Client, secretNameEnvKey string) *corev1.Secret` -- Loads a Secret from namespace/name env vars or returns nil
- `LoadConfigMapOrDie(c client.Client, cmNameEnvKey string) *corev1.ConfigMap` -- Loads a ConfigMap from namespace/name env vars or returns nil
- `ProjectToDir(obj client.Object, dir string, filter ProjectToDirFileFilter)` -- Writes secret/configmap data as files in a directory (simulates volume mount)
- `ProjectOnlyTheseKeys(keys ...string) ProjectToDirFileFilter` -- Returns a filter that only projects specified keys
- `InstallCerts(sourceDir string)` -- Copies certificates from a source directory to the trusted CA bundle directory
- `BuildCertBundleFromDir(dir string) (string, error)` -- Reads PEM certificate files from a directory and concatenates them into a single bundle string

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ControllerName type
- `github.com/openshift/hive/pkg/constants` -- Env var names
- `github.com/openshift/hive/pkg/controller/utils` -- Additional log fields
- `github.com/openshift/hive/pkg/resource` -- Resource helper
- `github.com/openshift/hive/pkg/util/scheme` -- Scheme registration
- `sigs.k8s.io/controller-runtime/pkg/client` -- Client types
- `k8s.io/client-go/tools/clientcmd` -- Kubeconfig loading

## Capabilities

- Create Kubernetes clients from default kubeconfig with field manager support
- Load pull secrets from environment variables, command-line flags, or files
- Fetch release image metadata from CI URLs
- Load Secrets and ConfigMaps from environment-specified namespace/name pairs
- Project Secret/ConfigMap data to filesystem directories with optional key filtering
- Copy and install CA certificates into the system trust store
- Build PEM certificate bundles from directory contents

## Understanding Score

0.9
