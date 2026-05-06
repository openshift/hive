<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/dnsendpoint/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates one controller for DNSZone and one with a nameServerScraper for each root domain in HiveConfig.spec.managedDomains.domains[]. The controllers are added to the Manager …
- `ControllerName`
- `ReconcileDNSEndpoint`
- `ReconcileDNSEndpoint.Reconcile` — Reconcile syncs the name server entries for a DNSZone subdomain in the root domain's hosted zone. Discrepancies are discovered by comparing to the nameServerScraper's cache. The D…

## Internal Dependencies

- `context`
- `fmt`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/manageddns`
- `github.com/pkg/errors`
- `github.com/prometheus/client_golang/prometheus`
- `github.com/sirupsen/logrus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/client-go/util/workqueue`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/event`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/metrics`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `strings`
- `sync`
- `time`

## Capabilities

- **`package`** name(s): **dnsendpoint**.
- Go **`import`** edges listed below (27 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/dnsendpoint`.

## Understanding Score

0.0
