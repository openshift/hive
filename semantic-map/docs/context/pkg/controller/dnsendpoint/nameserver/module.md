# Module atlas

## Responsibility

Provides cloud-specific implementations of the `Query` interface for managing DNS name server records in root domain hosted zones. Supports AWS Route53, Azure DNS, and GCP Cloud DNS. Each implementation can get existing NS records, create/update NS records for subdomains, and delete NS records.

## Public Interface/API

- `Query` -- Interface with methods: `Get(rootDomain)`, `CreateOrUpdate(rootDomain, domain, values)`, `Delete(rootDomain, domain, values)`.

## Internal Dependencies

- `github.com/openshift/hive/pkg/awsclient` -- AWS client for Route53 operations.
- `github.com/openshift/hive/pkg/azureclient` -- Azure client for DNS zone operations.
- `github.com/openshift/hive/pkg/gcpclient` -- GCP client for Cloud DNS operations.
- `github.com/openshift/hive/pkg/controller/utils` -- Helper utilities.

## Capabilities

- AWS implementation: Uses Route53 to list/create/update/delete NS record sets in hosted zones.
- Azure implementation: Uses Azure DNS to manage NS record sets in DNS zones.
- GCP implementation: Uses Cloud DNS to manage NS record sets in managed zones.
- All implementations handle pagination, error cases, and domain matching.

## Understanding Score

0.80
