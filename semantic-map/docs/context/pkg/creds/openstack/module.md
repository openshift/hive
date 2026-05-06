<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/creds/openstack/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `ConfigureCreds` — ConfigureCreds loads secrets designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE, CREDS_SECRET_NAME, and CERTS_SECRET_NAME and configures OpenStack credential conf…
- `GetCreds` — GetCreds reads OpenStack credentials either from the specified credentials file, ~/.config/openstack/clouds.yaml, or /etc/openstack/clouds.yaml

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

- **`package`** name(s): **openstack**.
- Go **`import`** edges listed below (8 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/creds/openstack`.

## Understanding Score

0.0
