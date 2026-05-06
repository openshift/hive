<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/dnszone/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `AWSActuator` — AWSActuator manages getting the desired state, getting the current state and reconciling the two.
- `AWSActuator.Create` — Create makes an AWS Route53 hosted zone given the DNSZone object.
- `AWSActuator.Delete` — Delete removes an AWS Route53 hosted zone, typically because the DNSZone object is in a deleting state.
- `AWSActuator.Exists` — Exists determines if the route53 hosted zone corresponding to the DNSZone exists
- `AWSActuator.GetNameServers` — GetNameServers returns the nameservers listed in the route53 hosted zone NS record.
- `AWSActuator.Refresh` — Refresh gets the AWS object for the zone. If a zone cannot be found or no longer exists, actuator.zoneID remains unset.
- `AWSActuator.SetConditionsForError` — SetConditionsForError sets conditions on the dnszone given a specific error. Returns true if conditions changed.
- `AWSActuator.UpdateMetadata` — UpdateMetadata ensures that the Route53 hosted zone metadata is current with the DNSZone
- `Actuator` — Actuator interface is the interface that is used to add dns provider support to the dnszone controller.
- `Add` — Add creates a new DNSZone Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Started.
- `AzureActuator` — AzureActuator attempts to make the current state reflect the given desired state.
- `AzureActuator.Create` — Create implements the Create call of the actuator interface
- `AzureActuator.Delete` — Delete implements the Delete call of the actuator interface
- `AzureActuator.Exists` — Exists implements the Exists call of the actuator interface
- `AzureActuator.GetNameServers` — GetNameServers implements the GetNameServers call of the actuator interface
- `AzureActuator.Refresh` — Refresh implements the Refresh call of the actuator interface
- `AzureActuator.SetConditionsForError` — SetConditionsForError sets conditions on the dnszone given a specific error. Returns true if conditions changed.
- `AzureActuator.UpdateMetadata` — UpdateMetadata implements the UpdateMetadata call of the actuator interface
- `ControllerName`
- `DeleteAWSRecordSets` — DeleteAWSRecordSets will clean up a DNS zone down to the minimum required record entries
- `DeleteAzureRecordSets` — DeleteAzureRecordSets will remove all non-essential records from the DNSZone provided.
- `DeleteGCPRecordSets` — DeleteGCPRecordSets will delete all non-essential DNS records in the DNSZone provided
- `GCPActuator` — GCPActuator attempts to make the current state reflect the given desired state.
- `GCPActuator.Create` — Create implements the Create call of the actuator interface
- `GCPActuator.Delete` — Delete implements the Delete call of the actuator interface
- `GCPActuator.Exists` — Exists implements the Exists call of the actuator interface
- `GCPActuator.GetNameServers` — GetNameServers implements the GetNameServers call of the actuator interface
- `GCPActuator.Refresh` — Refresh implements the Refresh call of the actuator interface
- `GCPActuator.SetConditionsForError` — SetConditionsForError sets conditions on the dnszone given a specific error. Returns true if conditions changed.
- `GCPActuator.UpdateMetadata` — UpdateMetadata implements the UpdateMetadata call of the actuator interface
- `IsErrorUpdateEvent` — IsErrorUpdateEvent returns true when the update event for DNSZone is from error state.
- `ReconcileDNSZone` — ReconcileDNSZone reconciles a DNSZone object
- `ReconcileDNSZone.Reconcile` — Reconcile reads that state of the cluster for a DNSZone object and makes changes based on the state read and what is in the DNSZone.Spec

## Internal Dependencies

- `context`
- `errors`
- `fmt`
- `github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns`
- `github.com/aws/aws-sdk-go-v2/aws`
- `github.com/aws/aws-sdk-go-v2/aws/arn`
- `github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi`
- `github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types`
- `github.com/aws/aws-sdk-go-v2/service/route53`
- `github.com/aws/aws-sdk-go-v2/service/route53/types`
- `github.com/aws/smithy-go`
- `github.com/miekg/dns`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/awsclient`
- `github.com/openshift/hive/pkg/awsclient/mock`
- `github.com/openshift/hive/pkg/azureclient`
- `github.com/openshift/hive/pkg/azureclient/mock`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/gcpclient`
- `github.com/openshift/hive/pkg/gcpclient/mock`
- `github.com/openshift/hive/pkg/test/fake`
- `github.com/pkg/errors`
- `github.com/prometheus/client_golang/prometheus`
- `github.com/sirupsen/logrus`
- `go.uber.org/mock/gomock`
- `google.golang.org/api/dns/v1`
- `google.golang.org/api/googleapi`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/workqueue`
- `net/http`
- `os`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/event`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/metrics`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `strings`
- `testing`
- `time`

## Capabilities

- **`package`** name(s): **dnszone**.
- Go **`import`** edges listed below (50 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/dnszone`.

## Understanding Score

0.0
