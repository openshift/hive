# Module atlas

## Responsibility

Shared utility library for hiveutil contrib commands: provides controller-runtime client construction, pull secret loading, release image resolution, certificate management, and data projection helpers.

## Public Interface/API

- `GetClient() (client.Client, error)` — creates a controller-runtime client with Watch and FieldOwner support
- `GetResourceHelper() (resource.Helper, error)` — creates a resource helper from kubeconfig
- `GetPullSecret(logger) (string, error)` — loads pull secret from env var or default file path
- `DetermineReleaseImageFromSource(src) (string, error)` — resolves release image reference from a source string (URL, env, or `oc adm release info`)
- `ProjectToDir(dir, obj, filter) error` — projects a Secret/ConfigMap's data keys to individual files in a directory (simulates volume mounts)
- `ProjectToDirFileFilter` — callback type for filtering/transforming projected keys
- `LoadSecretOrDie(secretName) *corev1.Secret` — loads a Secret from env-configured namespace, returns nil if env not set
- `LoadConfigMapOrDie(cmName) *corev1.ConfigMap` — loads a ConfigMap from env-configured namespace, returns nil if env not set
- `InstallCerts(sourceDir) error` — copies PEM certificates into system trust store and updates trust config
- `BuildCertBundleFromDir(dir) (string, error)` — reads and concatenates PEM files from a directory
- `NewLogger(debug) *logrus.Logger` — creates a configured logrus logger
- `DefaultNamespace() string` — returns the current kubeconfig namespace

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
- Resolves release images from multiple sources (direct URL, env var, `oc` CLI)
- Projects Secret/ConfigMap data to filesystem directories with optional key filtering
- Manages TLS certificate installation into system trust stores
- Provides namespace-aware Secret/ConfigMap loading with graceful nil-if-not-configured behavior

## Understanding Score

0.85
