# Module: pkg/operator/assets

## Responsibility

Provides embedded (go-bindata generated) binary assets containing Kubernetes YAML manifests used by the Hive operator to deploy and configure Hive components. The assets include deployment specs, service definitions, RBAC roles/bindings, webhook configurations, service accounts, and configmaps for: hive-controllers, hiveadmission, sharded controllers (statefulsets for clustersync/machinepool), and RBAC setup (admin, reader, frontend, clusterpool roles).

## Public Interface/API

- `Asset(name string) ([]byte, error)` -- loads and returns the embedded asset by name. Returns error if not found.
- `MustAsset(name string) []byte` -- like `Asset` but panics on error. Used extensively by `pkg/operator/hive`.
- `AssetDir(name string) ([]string, error)` -- returns file names under a given embedded directory path.
- `AssetInfo(name string) (os.FileInfo, error)` -- returns file info for a named asset.
- `AssetNames() []string` -- returns all embedded asset names.
- `RestoreAsset(dir, name string) error` -- writes an asset to disk under the given directory.
- `RestoreAssets(dir, name string) error` -- recursively restores assets under a directory.

## Embedded Asset Categories

1. **config/controllers/** -- hive-controllers Deployment, ClusterRole, ClusterRoleBinding, ServiceAccount, Service
2. **config/hiveadmission/** -- hiveadmission Deployment, Service, ServiceAccount, APIService, 9 ValidatingWebhookConfigurations (clusterdeployment, clusterimageset, clusterpool, clusterprovision, dnszones, machinepool, syncset, selectorsyncset, clusterdeploymentcustomization), RBAC role/binding, SA token secret
3. **config/sharded_controllers/** -- StatefulSet and Service templates (parameterized by `{{.ControllerName}}`)
4. **config/rbac/** -- hive_admin, hive_reader, hive_frontend, hive_clusterpool_admin roles and bindings
5. **config/configmaps/** -- install-log-regexes ConfigMap

## Internal Dependencies

No Hive-internal dependencies. Uses only Go stdlib (`fmt`, `io/ioutil`, `os`, `path/filepath`, `strings`, `time`).

## Capabilities

- **Package**: `assets`
- Single source file: `bindata.go` (auto-generated, very large).
- 6 unique import paths.
- This is a generated file -- do not edit manually. Regenerate via `go-bindata`.

## Understanding Score

0.88
