<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`contrib/pkg/awsprivatelink/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `NewAWSPrivateLinkCommand`
- `NewDisableAWSPrivateLinkCommand`
- `NewEnableAWSPrivateLinkCommand`
- `NewEndpointVPCCommand`

## Internal Dependencies

- `context`
- `errors`
- `github.com/aws/aws-sdk-go-v2/aws`
- `github.com/aws/aws-sdk-go-v2/service/ec2`
- `github.com/aws/aws-sdk-go-v2/service/ec2/types`
- `github.com/openshift/api/config/v1`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/contrib/pkg/awsprivatelink/common`
- `github.com/openshift/hive/contrib/pkg/awsprivatelink/endpointvpc`
- `github.com/openshift/hive/contrib/pkg/utils`
- `github.com/openshift/hive/pkg/awsclient`
- `github.com/openshift/hive/pkg/creds/aws`
- `github.com/openshift/hive/pkg/operator/hive`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/sirupsen/logrus`
- `github.com/spf13/cobra`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/types`
- `os/user`
- `path/filepath`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `strings`

## Capabilities

- **`package`** name(s): **awsprivatelink**.
- Go **`import`** edges listed below (24 unique path(s)).
- Package ID(s): `github.com/openshift/hive/contrib/pkg/awsprivatelink`.

## Understanding Score

0.0
