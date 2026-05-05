# Module atlas

## Responsibility

Provides the AWS API client abstraction used throughout Hive: a `Client` interface wrapping EC2, ELBv2, Route53, S3, STS, and Resource Tagging APIs with credential loading from Secrets, environment, or AssumeRole.

## Public Interface/API

- `Client` — interface with ~45 methods covering EC2 (VPCs, subnets, instances, endpoints, peering, security groups), Route53 (hosted zones, record sets, VPC associations), ELBv2, S3, STS, and resource tagging
- `New(kubeClient, Options) (Client, error)` — top-level constructor dispatching to the appropriate credential source
- `NewClient(kubeClient, secretName, namespace, region) (Client, error)` — from a named Secret
- `NewClientFromSecret(secret, region) (Client, error)` — from a Secret object directly
- `Options` — region + `CredentialsSource` configuration
- `CredentialsSource` — Secret-based or AssumeRole-based credential loading
- `SecretCredentialsSource`, `AssumeRoleCredentialsSource` — credential source structs
- `ErrCodeEquals(err, code) bool` — AWS API error code matching helper
- `NewAPIError(code, message) error` — creates a smithy API error
- `Paginator[P, Out, OptFn](paginator, fn) error` — generic paginator helper using Go generics
- `ContainsCredentialProcess(config) bool` — security check for forbidden `credential_process` in AWS config

## Internal Dependencies

- `github.com/aws/aws-sdk-go-v2` — AWS SDK v2 (EC2, ELBv2, Route53, S3, STS, resource tagging) (external)
- `github.com/openshift/hive/apis/hive/v1/aws` — AssumeRole type
- `github.com/openshift/hive/pkg/constants` — AWS config/credential secret keys, China region constants
- `sigs.k8s.io/controller-runtime/pkg/client` — Kubernetes client for Secret retrieval
- `github.com/prometheus/client_golang/prometheus` — API call metrics (external)

## Capabilities

- Creates authenticated AWS clients from Kubernetes Secrets, environment variables, or STS AssumeRole
- Supports AWS CLI config format in Secrets (including multi-key format)
- Blocks `credential_process` in Secret-sourced configs as a security measure
- Custom endpoint resolver for AWS China Route53
- Prometheus counter `hive_aws_api_calls_total` partitioned by function name
- Generic paginator using Go 1.18+ generics for any AWS paginated API

## Understanding Score

0.85
