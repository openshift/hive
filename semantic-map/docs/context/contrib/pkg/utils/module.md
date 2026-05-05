# Module atlas

## Responsibility

Shared utility library for hiveutil contrib commands: provides controller-runtime client construction, pull secret loading, release image resolution, certificate management, and data projection helpers.

## Public Interface/API

- `GetClient(fieldManager string) (client.WithWatch, error)` — creates a controller-runtime client with Watch and FieldOwner support
- `GetResourceHelper(controllerName hivev1.ControllerName, logger log.FieldLogger) (resource.Helper, error)` — creates a resource helper from kubeconfig
- `GetPullSecret(logger log.FieldLogger, pullSecret, pullSecretFile string) (string, error)` — loads pull secret from env var, flag, or file path
- `DetermineReleaseImageFromSource(sourceURL string) (string, error)` — resolves release image pull spec from a JSON URL
- `ProjectToDir(obj client.Object, dir string, filter ProjectToDirFileFilter)` — projects a Secret/ConfigMap's data keys to individual files in a directory (simulates volume mounts); panics on error
- `ProjectToDirFileFilter` — callback type for filtering/transforming projected keys
- `ProjectOnlyTheseKeys(keys ...string) ProjectToDirFileFilter` — returns a filter that only projects the named keys
- `LoadSecretOrDie(c client.Client, secretNameEnvKey string) *corev1.Secret` — loads a Secret from env-configured namespace, returns nil if env not set
- `LoadConfigMapOrDie(c client.Client, cmNameEnvKey string) *corev1.ConfigMap` — loads a ConfigMap from env-configured namespace, returns nil if env not set
- `InstallCerts(sourceDir string)` — copies PEM certificates into system trust directory (no return value; fatal on error)
- `BuildCertBundleFromDir(dir string) (string, error)` — reads and concatenates PEM certificate files from a directory
- `NewLogger(logLevel string) (*log.Entry, error)` — creates a configured logrus Entry (not Logger)
- `DefaultNamespace() (string, error)` — returns the current kubeconfig namespace (also returns error)

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — Hive API types
- `github.com/openshift/hive/pkg/constants` — constant values (pull secret paths, env vars)
- `github.com/openshift/hive/pkg/controller/utils` — controller utilities
- `github.com/openshift/hive/pkg/resource` — resource.Helper for server-side apply
- `github.com/openshift/hive/pkg/util/scheme` — shared scheme registration
- `sigs.k8s.io/controller-runtime/pkg/client` — Client interface and config
- `k8s.io/client-go/tools/clientcmd` — kubeconfig loading

## Capabilities

- Constructs authenticated controller-runtime clients from kubeconfig
- Resolves release images from a JSON URL endpoint
- Projects Secret/ConfigMap data to filesystem directories with optional key filtering
- Manages TLS certificate installation into system trust stores
- Provides namespace-aware Secret/ConfigMap loading with graceful nil-if-not-configured behavior

## Understanding Score

0.85
