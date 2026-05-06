<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/creds/azure/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `ConfigureCreds` — ConfigureCreds loads a secret designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE and CREDS_SECRET_NAME and configures Azure credential environment variables and c…
- `GetCreds` — GetCreds reads Azure credentials used for install/uninstall from either the default credentials file (~/.azure/osServiceAccount.json), the standard environment variable, or provid…

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/installer/pkg/types`
- `github.com/sirupsen/logrus`
- `k8s.io/client-go/util/homedir`
- `os`
- `path/filepath`
- `sigs.k8s.io/controller-runtime/pkg/client`

## Capabilities

- **`package`** name(s): **azure**.
- Go **`import`** edges listed below (8 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/creds/azure`.

## Understanding Score

0.0
