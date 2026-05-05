# Module atlas

## Responsibility

Generated gomock mock for `pkg/awsclient.Client` interface, used in unit tests across Hive controllers.

## Public Interface/API

- `MockClient` — mock implementation of `awsclient.Client` with all ~45 AWS API methods
- `MockClientMockRecorder` — expectation recorder for `MockClient`
- `NewMockClient(ctrl) *MockClient` — constructor

## Internal Dependencies

- `github.com/golang/mock/gomock` — mock framework (external)
- `github.com/aws/aws-sdk-go-v2/service/{ec2,elasticloadbalancingv2,route53,s3,sts,resourcegroupstaggingapi}` — AWS SDK types for method signatures (external)

## Capabilities

- Auto-generated via `mockgen -source=./client.go`
- Provides test doubles for all AWS API operations used by Hive

## Understanding Score

0.95
