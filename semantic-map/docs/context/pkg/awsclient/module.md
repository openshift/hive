# Module atlas

## Responsibility

Provides a unified AWS client interface wrapping AWS SDK v2 services (EC2, ELBv2, S3, Route53, STS, Resource Groups Tagging API) with Prometheus metrics, credential loading from Kubernetes Secrets or environment, and support for AssumeRole-based authentication. Tests exist in downstream consumers.

## Public Interface/API

**Interfaces:**
- `Client` -- Unified interface with ~40 methods covering EC2 (VPCs, subnets, route tables, security groups, instances, VPC peering, VPC endpoints), ELBv2 (load balancers), S3 (upload), Route53 (hosted zones, record sets, VPC associations), Resource Tagging (paginated resource queries), STS (caller identity)
- `IPaginator[Out, OptFn]` -- Generic paginator interface (`HasMorePages`, `NextPage`)

**Types:**
- `Options` -- Client creation options (Region, CredentialsSource)
- `CredentialsSource` -- Credential sourcing config (Secret-based or AssumeRole-based)
- `SecretCredentialsSource` -- Load credentials from a Kubernetes Secret
- `AssumeRoleCredentialsSource` -- Assume an IAM role using a secret or environment credentials

**Functions:**
- `New(kubeClient, Options) (Client, error)` -- Create a Client using the configured credentials source
- `NewClient(kubeClient, secretName, namespace, region string) (Client, error)` -- Create a Client from a named Secret
- `NewClientFromSecret(secret *corev1.Secret, region string) (Client, error)` -- Create a Client from a Secret object
- `NewAPIError(code, message string) error` -- Create a Smithy API error
- `ErrCodeEquals(err error, code string) bool` -- Check if an error matches an AWS API error code
- `ContainsCredentialProcess(config []byte) bool` -- Check if AWS config contains forbidden `credential_process`
- `Paginator[P, Out, OptFn](awsPaginator P, fn func(*Out, bool) bool) error` -- Generic pagination helper for AWS SDK v2 paginators

## Internal Dependencies

- `github.com/aws/aws-sdk-go-v2/*` -- AWS SDK v2 (config, ec2, elbv2, s3, route53, sts, resource groups tagging, stscreds)
- `github.com/aws/smithy-go` -- Error types, endpoint resolution, middleware
- `github.com/openshift/hive/apis/hive/v1/aws` -- AssumeRole type
- `github.com/openshift/hive/pkg/constants` -- Secret keys, Route53 region constants
- `github.com/prometheus/client_golang/prometheus` -- Metrics counter for API calls
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client for secret retrieval
- `sigs.k8s.io/controller-runtime/pkg/metrics` -- Metrics registry

## Capabilities

- Wrap all required AWS API calls behind a single mockable interface
- Track all AWS API calls via Prometheus counter (`hive_aws_api_calls_total`)
- Load credentials from Kubernetes Secrets (static keys or AWS CLI config), environment, or via STS AssumeRole
- Reject secrets containing `credential_process` for security
- Handle AWS China region Route53 endpoint resolution
- Provide generic pagination helper for AWS SDK v2 paginators
- Default region to `us-east-1` when unspecified

## Understanding Score

0.9
