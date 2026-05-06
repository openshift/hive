<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/installmanager/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `InstallManager` — InstallManager coordinates executing the openshift-install binary, modifying generated assets, and uploading artifacts to the kube API after completion.
- `InstallManager.Complete` — Complete sets remaining fields on the InstallManager based on command options and arguments. ...except for the clusterProvision field. That's loaded up by Run(), since it involves…
- `InstallManager.Run` — Run is the entrypoint to start the install process
- `InstallManager.Validate` — Validate ensures the given options and arguments are valid.
- `LogUploaderActuator` — LogUploaderActuator interface is the interface that is used to add provider support for uploading logs.
- `NewInstallManagerCommand` — NewInstallManagerCommand is the entrypoint to create the 'install-manager' subcommand

## Internal Dependencies

- `bufio`
- `bytes`
- `context`
- `encoding/base64`
- `encoding/json`
- `fmt`
- `github.com/aws/aws-sdk-go-v2/aws`
- `github.com/aws/aws-sdk-go-v2/service/s3`
- `github.com/google/uuid`
- `github.com/json-iterator/go`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/contrib/pkg/utils`
- `github.com/openshift/hive/pkg/awsclient`
- `github.com/openshift/hive/pkg/azureclient`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/dnszone`
- `github.com/openshift/hive/pkg/controller/machinepool`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/creds`
- `github.com/openshift/hive/pkg/creds/azure`
- `github.com/openshift/hive/pkg/creds/gcp`
- `github.com/openshift/hive/pkg/gcpclient`
- `github.com/openshift/hive/pkg/ibmclient`
- `github.com/openshift/hive/pkg/resource`
- `github.com/openshift/hive/pkg/util/labels`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/openshift/hive/pkg/util/yaml`
- `github.com/openshift/installer/pkg/destroy/aws`
- `github.com/openshift/installer/pkg/destroy/azure`
- `github.com/openshift/installer/pkg/destroy/gcp`
- `github.com/openshift/installer/pkg/destroy/ibmcloud`
- `github.com/openshift/installer/pkg/destroy/nutanix`
- `github.com/openshift/installer/pkg/destroy/openstack`
- `github.com/openshift/installer/pkg/destroy/providers`
- `github.com/openshift/installer/pkg/destroy/vsphere`
- `github.com/openshift/installer/pkg/types`
- `github.com/openshift/installer/pkg/types/aws`
- `github.com/openshift/installer/pkg/types/azure`
- `github.com/openshift/installer/pkg/types/gcp`
- `github.com/openshift/installer/pkg/types/ibmcloud`
- `github.com/openshift/installer/pkg/types/nutanix`
- `github.com/openshift/installer/pkg/types/openstack`
- `github.com/openshift/installer/pkg/types/vsphere`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `github.com/spf13/cobra`
- `github.com/tidwall/gjson`
- `github.com/tidwall/sjson`
- `io`
- `io/fs`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/meta`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/fields`
- `k8s.io/apimachinery/pkg/runtime/serializer`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/errors`
- `k8s.io/apimachinery/pkg/util/rand`
- `k8s.io/apimachinery/pkg/util/wait`
- `k8s.io/apimachinery/pkg/watch`
- `k8s.io/client-go/rest`
- `k8s.io/client-go/tools/cache`
- `k8s.io/client-go/tools/watch`
- `k8s.io/client-go/util/retry`
- `k8s.io/utils/ptr`
- `os`
- `os/exec`
- `path/filepath`
- `regexp`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/client/apiutil`
- `sigs.k8s.io/yaml`
- `strconv`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **installmanager**.
- Go **`import`** edges listed below (75 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/installmanager`.

## Understanding Score

0.0
