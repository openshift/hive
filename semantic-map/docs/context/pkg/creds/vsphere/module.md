<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/creds/vsphere/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `ConfigureCreds` — ConfigureCreds loads secrets designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE, CREDS_SECRET_NAME, and CERTS_SECRET_NAME and configures VSphere credential enviro…

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/installer/pkg/types`
- `github.com/openshift/installer/pkg/types/vsphere`
- `github.com/sirupsen/logrus`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/yaml`
- `strings`

## Capabilities

- **`package`** name(s): **vsphere**.
- Go **`import`** edges listed below (8 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/creds/vsphere`.

## Understanding Score

0.0
