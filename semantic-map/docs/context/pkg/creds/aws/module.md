<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/creds/aws/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `ConfigureCreds` — ConfigureCreds loads a secret designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE and CREDS_SECRET_NAME and configures AWS credential environment variables and con…
- `GetAWSCreds` — GetAWSCreds reads AWS credentials either from either the specified credentials file, the standard environment variables, or a default credentials file. (~/.aws/credentials) The de…

## Internal Dependencies

- `errors`
- `github.com/openshift/hive/contrib/pkg/utils`
- `github.com/openshift/hive/pkg/awsclient`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/installer/pkg/types`
- `github.com/sirupsen/logrus`
- `gopkg.in/ini.v1`
- `os`
- `path/filepath`
- `sigs.k8s.io/controller-runtime/pkg/client`

## Capabilities

- **`package`** name(s): **aws**.
- Go **`import`** edges listed below (10 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/creds/aws`.

## Understanding Score

0.0
