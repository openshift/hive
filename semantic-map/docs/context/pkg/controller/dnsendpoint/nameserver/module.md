<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/dnsendpoint/nameserver/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Query` — Query is used to perform queries for name servers.

## Internal Dependencies

- `context`
- `fmt`
- `github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns`
- `github.com/Azure/go-autorest/autorest/to`
- `github.com/aws/aws-sdk-go-v2/aws`
- `github.com/aws/aws-sdk-go-v2/service/route53`
- `github.com/aws/aws-sdk-go-v2/service/route53/types`
- `github.com/openshift/hive/pkg/awsclient`
- `github.com/openshift/hive/pkg/azureclient`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/gcpclient`
- `github.com/pkg/errors`
- `google.golang.org/api/dns/v1`
- `google.golang.org/api/googleapi`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/util/sets`
- `net/http`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **nameserver**.
- Go **`import`** edges listed below (20 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver`.

## Understanding Score

0.0
