# Module atlas

## Responsibility

Defines the `Query` interface for DNS name server operations (get, create/update, delete) against cloud provider hosted zones, with implementations for AWS Route53, Azure DNS, and GCP Cloud DNS.

## Public Interface/API

- `Query` -- interface with methods:
  - `Get(rootDomain string) (map[string]sets.Set[string], error)` -- returns NS records keyed by domain
  - `CreateOrUpdate(rootDomain, domain string, values sets.Set[string]) error`
  - `Delete(rootDomain, domain string, values sets.Set[string]) error`
- `NewAWSQuery(c client.Client, credsSecretName, region string) Query`
- `NewAzureQuery(c client.Client, credsSecretName, resourceGroupName, cloudName string) Query`
- `NewGCPQuery(c client.Client, credsSecretName string) Query`

## Internal Dependencies

- `github.com/openshift/hive/pkg/awsclient` -- AWS client factory
- `github.com/openshift/hive/pkg/azureclient` -- Azure client factory
- `github.com/openshift/hive/pkg/gcpclient` -- GCP client factory
- `github.com/openshift/hive/pkg/controller/utils` -- Dotted/Undotted helpers, GetHiveNamespace
- `github.com/aws/aws-sdk-go-v2/service/route53` -- AWS Route53 API
- `github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns` -- Azure DNS API
- `google.golang.org/api/dns/v1` -- GCP Cloud DNS API

## Capabilities

- AWS implementation: queries hosted zones by name, lists/creates/deletes NS ResourceRecordSets via Route53
- Azure implementation: manages NS RecordSets via Azure DNS, uses resource group scoping
- GCP implementation: manages NS ResourceRecordSets via Cloud DNS managed zones, handles create-or-update with conflict detection
- All implementations lazily construct cloud clients from Kubernetes secrets

## Understanding Score

0.85
