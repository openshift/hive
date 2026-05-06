<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/awsclient/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `AssumeRoleCredentialsSource` — AssumeRole credentials source uses AWS session configured using credentials in the SecretRef, and then uses that to assume the role provided in Role. AWS client is created using t…
- `Client`
- `ContainsCredentialProcess`
- `CredentialsSource` — CredentialsSource defines how the credentials will be loaded. It supports various methods of sourcing credentials. But if none of the supported sources are configured such that th…
- `ErrCodeEquals` — ErrCodeEquals returns true if the error matches all these conditions: - err is of type smithy.APIError - Error.Code() equals code
- `IPaginator`
- `NewAPIError`
- `Options` — Options provides the means to control how a client is created and what configuration values will be loaded.
- `Paginator`
- `SecretCredentialsSource` — Secret credentials source loads the credentials from a secret. It supports static credentials in the secret provided by aws_access_key_id, and aws_access_secret key. It also suppo…

## Internal Dependencies

- `bytes`
- `context`
- `fmt`
- `github.com/aws/aws-sdk-go-v2/aws`
- `github.com/aws/aws-sdk-go-v2/aws/middleware`
- `github.com/aws/aws-sdk-go-v2/config`
- `github.com/aws/aws-sdk-go-v2/credentials/stscreds`
- `github.com/aws/aws-sdk-go-v2/feature/s3/manager`
- `github.com/aws/aws-sdk-go-v2/service/ec2`
- `github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2`
- `github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi`
- `github.com/aws/aws-sdk-go-v2/service/route53`
- `github.com/aws/aws-sdk-go-v2/service/s3`
- `github.com/aws/aws-sdk-go-v2/service/sts`
- `github.com/aws/smithy-go`
- `github.com/aws/smithy-go/endpoints`
- `github.com/aws/smithy-go/middleware`
- `github.com/openshift/hive/apis/hive/v1/aws`
- `github.com/openshift/hive/pkg/constants`
- `github.com/pkg/errors`
- `github.com/prometheus/client_golang/prometheus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/types`
- `net/url`
- `os`
- `regexp`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/metrics`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **awsclient**.
- Go **`import`** edges listed below (30 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/awsclient`.

## Understanding Score

0.0
