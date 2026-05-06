<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/gcpclient/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Client` — Client is a wrapper object for actual GCP libraries to allow for easier mocking/testing.
- `InstancesStopCallOption`
- `ListAddressesOptions` — ListAddressesOptions are the options for listing compute images.
- `ListComputeImagesOptions` — ListComputeImagesOptions are the options for listing compute images.
- `ListComputeInstancesOptions`
- `ListComputeZonesOptions` — ListComputeZonesOptions are the options for listing compute zones.
- `ListManagedZonesOptions` — ListManagedZonesOptions are the options for listing managed zones.
- `ListResourceRecordSetsOptions` — ListResourceRecordSetsOptions are the options for listing resource record sets.
- `ProjectID` — ProjectID returns the GCP project ID specified in the GCP creds. The supplied byte slice contains the GCP creds.
- `ProjectIDFromFile` — ProjectIDFromFile returns the GCP project ID specified in the GCP creds. The GCP creds are read from the specified file.
- `ProjectIDFromSecret` — ProjectIDFromSecret returns the GCP project ID specified in the GCP creds. The GCP creds are read from the specified secret.

## Internal Dependencies

- `context`
- `fmt`
- `github.com/openshift/hive/pkg/constants`
- `github.com/pkg/errors`
- `golang.org/x/oauth2/google`
- `google.golang.org/api/cloudresourcemanager/v1`
- `google.golang.org/api/compute/v1`
- `google.golang.org/api/dns/v1`
- `google.golang.org/api/googleapi`
- `google.golang.org/api/option`
- `google.golang.org/api/serviceusage/v1`
- `k8s.io/api/core/v1`
- `net/http`
- `os`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **gcpclient**.
- Go **`import`** edges listed below (16 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/gcpclient`.

## Understanding Score

0.0
