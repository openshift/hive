# Module atlas

## Responsibility

Provides a controller that syncs DNS name server entries from DNSZone subdomains into the parent/root domain's hosted zone. For each managed domain configured in HiveConfig, a separate controller instance with a nameServerScraper is created. The scraper periodically queries the root domain's hosted zone for current NS records, and the reconciler ensures that the NS entries in the root zone match those reported by each DNSZone's status.

## Public Interface/API

- `Add` -- Creates one controller per managed root domain. Each gets its own nameServerScraper.
- `ControllerName` -- Constant `"dnsendpoint"`.
- `ReconcileDNSEndpoint` -- Reconciler struct with a scraper reference.
- `ReconcileDNSEndpoint.Reconcile` -- Syncs NS records for a DNSZone's subdomain into the root domain's hosted zone.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- DNSZone CR.
- `github.com/openshift/hive/pkg/constants` -- Environment variable names, label keys.
- `github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver` -- Query interface for cloud DNS operations.
- `github.com/openshift/hive/pkg/manageddns` -- Managed domain configuration loading.
- `github.com/openshift/hive/pkg/controller/utils` -- Controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.

## Capabilities

- Reconciles: **DNSZone** (those under managed domains).
- Watches: DNSZone (filtered by root domain).
- Conditions set: None directly on CRs (manages DNS records in cloud).
- Key logic: nameServerScraper runs in background goroutine, periodically scraping NS records from root hosted zone into a cache. Reconciler compares DNSZone.status nameServers against cache; creates/updates/deletes NS entries in root zone as needed.
- Metrics: `hive_dnsendpoint_scrape_duration_seconds`.

## Understanding Score

0.80
