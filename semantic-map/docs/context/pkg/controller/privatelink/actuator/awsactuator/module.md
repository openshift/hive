# Module atlas

## Responsibility

Implements the private link Actuator interface for AWS, managing VPC endpoints, hosted zone records, and endpoint services for AWS PrivateLink connectivity between Hive's management cluster and spoke clusters.

## Public Interface/API

- `AWSHubActuator` -- implements `actuator.Actuator` for AWS hub-side private link
  - `Cleanup(cd, metadata, logger) error` -- cleans up hosted zone records
  - `CleanupRequired(cd) bool`
  - `Reconcile(cd, metadata, dnsRecord, logger) (reconcile.Result, error)` -- reconciles VPC endpoint and DNS
  - `ReconcileHostedZoneRecords(...)` -- manages Route53 hosted zone records for API endpoint
  - `ShouldSync(cd) bool`
- `NewAWSHubActuator(client, config, privateLinkEnabled, awsClientFn, logger) (*AWSHubActuator, error)`
- `AssumeRole` -- struct for cross-account IAM role assumption
- `ReadAWSPrivateLinkControllerConfigFile() (*hivev1.AWSPrivateLinkConfig, error)` -- reads legacy config (deprecated)

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `apis/hive/v1/aws`
- `github.com/openshift/hive/pkg/awsclient` -- AWS client factory
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/privatelink/actuator` -- Actuator interface
- `github.com/openshift/hive/pkg/controller/privatelink/conditions` -- condition helpers
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/aws/aws-sdk-go-v2/service/ec2`, `service/route53` -- AWS API clients

## Capabilities

- Creates/manages VPC endpoints for AWS PrivateLink connections to spoke clusters
- Manages Route53 private hosted zone records pointing to VPC endpoint DNS
- Supports cross-account access via IAM AssumeRole
- Builds AWS clients from Kubernetes secrets with optional assume-role configuration
- Falls back to legacy AWSPrivateLinkController config file for backwards compatibility
- Cleans up hosted zone records on ClusterDeployment deletion

## Understanding Score

0.85
