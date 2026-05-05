# Module atlas

## Responsibility

Provides the AWS implementation of the private link actuator interface. Manages the full chain of AWS PrivateLink resources: VPC Endpoint Service (backed by NLB), VPC Endpoint in hub account, Private Hosted Zone with DNS records, and VPC associations. Similar functionality to the older `awsprivatelink` controller but implemented as a pluggable actuator.

## Public Interface/API

- `AWSHubActuator` -- AWS implementation of the `actuator.Actuator` interface.
- `AWSHubActuator.Reconcile` -- Creates/manages VPC Endpoint Service, VPC Endpoint, Hosted Zone, DNS records.
- `AWSHubActuator.Cleanup` -- Removes all AWS PrivateLink resources.
- `AWSHubActuator.CleanupRequired` -- Checks if cleanup is needed.
- `AWSHubActuator.ShouldSync` -- Rate-limits cloud API calls.
- `AWSHubActuator.ReconcileHostedZoneRecords` -- Manages DNS records in the hosted zone.
- `AssumeRole` -- Struct for AWS STS assume role configuration.
- `ReadAWSPrivateLinkControllerConfigFile` -- Reads AWS-specific PrivateLink config from file.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, ClusterMetadata types.
- `github.com/openshift/hive/apis/hive/v1/aws` -- AWS-specific PrivateLink configuration types.
- `github.com/openshift/hive/pkg/constants` -- Environment variable names.
- `github.com/openshift/hive/pkg/awsclient` -- AWS SDK client wrapper.
- `github.com/openshift/hive/pkg/controller/privatelink/actuator` -- Actuator interface and types.
- `github.com/openshift/hive/pkg/controller/privatelink/conditions` -- Condition management.
- `github.com/openshift/hive/pkg/controller/utils` -- Secret loading, error scrubbing, controller config.

## Capabilities

- Implements: `actuator.Actuator` interface.
- Key logic: Discovers cluster NLB, creates VPC Endpoint Service, creates VPC Endpoint in hub VPC, creates Private Hosted Zone with A/Alias records, manages VPC associations and allowed principals. Uses tag-based resource discovery.

## Understanding Score

0.80
