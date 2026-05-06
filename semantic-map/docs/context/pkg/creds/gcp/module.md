<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/creds/gcp/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `ConfigureCreds` — ConfigureCreds loads a secret designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE and CREDS_SECRET_NAME and configures GCP credential environment variables and con…
- `GetCreds` — GetCreds reads GCP credentials either from either the specified credentials file, the standard environment variables, or a default credentials file. (~/.gcp/osServiceAccount.json)…

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

- **`package`** name(s): **gcp**.
- Go **`import`** edges listed below (8 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/creds/gcp`.

## Understanding Score

0.0
