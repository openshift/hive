<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`contrib/pkg/certificate/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `HookOptions` — HookOptions is the set of options to create/delete DNS authentication entries
- `HookOptions.Complete` — Complete finalizes options by using arguments and environment variables
- `HookOptions.Create` — Create creates an authentication DNS record in the specified zone
- `HookOptions.Delete` — Delete removes the authentication DNS record from the specified zone
- `NewAuthHookCommand` — NewAuthHookCommand returns a command that will create a DNS authentication entry in the given DNS hosted zone
- `NewCertificateCommand` — NewCertificateCommand returns a utility command to help create a certificate for OpenShift clusters on AWS
- `NewCleanupHookCommand` — NewCleanupHookCommand returns a command that will cleanup a DNS authentication entry in the given DNS hosted zone
- `NewCreateCertifcateCommand` — NewCreateCertifcateCommand returns a command that will create a letsencrypt serving cert for OpenShift clusters on AWS.
- `Options` — Options is the set of options to create a new server certificate
- `Options.Complete` — Complete finalizes command options
- `Options.Run` — Run executes the command

## Internal Dependencies

- `bytes`
- `fmt`
- `github.com/aws/aws-sdk-go-v2/aws`
- `github.com/aws/aws-sdk-go-v2/service/route53`
- `github.com/aws/aws-sdk-go-v2/service/route53/types`
- `github.com/openshift/hive/pkg/awsclient`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `github.com/spf13/cobra`
- `os`
- `os/exec`
- `os/user`
- `path/filepath`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **certificate**.
- Go **`import`** edges listed below (15 unique path(s)).
- Package ID(s): `github.com/openshift/hive/contrib/pkg/certificate`.

## Understanding Score

0.0
