<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`contrib/pkg/utils/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `BuildCertBundleFromDir` — BuildCertBundleFromDir reads all non-directory files from the specified directory, assuming each file contains PEM-encoded certificate data, and concatenates their contents into a…
- `DefaultNamespace`
- `DetermineReleaseImageFromSource`
- `GetClient` — GetClient returns a new dynamic controller-runtime client.
- `GetPullSecret`
- `GetResourceHelper`
- `InstallCerts` — InstallCerts copies the contents of `sourceDir` into the appropriate directory and updates the trust configuration. If `sourceDir` does not exist, this func is a no-op. Other erro…
- `LoadConfigMapOrDie` — LoadConfigMapOrDie looks for environment variables named CLUSTERDEPLOYMENT_NAMESPACE and `secretName`. If either is not found, this indicates we are not supposed to use this mode …
- `LoadSecretOrDie` — LoadSecretOrDie looks for environment variables named CLUSTERDEPLOYMENT_NAMESPACE and `secretName`. If either is not found, this indicates we are not supposed to use this mode and…
- `NewLogger`
- `ProjectToDir` — ProjectToDir simulates what happens when you mount a secret or configmap as a volume on a pod, creating files named after each key under `dir` and populating them with the content…
- `ProjectToDirFileFilter` — ProjectToDirFileFilter is run by ProjectToDir for each key found in the obj. If the second return is an error, ProjectToDir will panic with it. Otherwise: If ProjectToDir should c…

## Internal Dependencies

- `context`
- `encoding/json`
- `encoding/pem`
- `fmt`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/resource`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `io`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/watch`
- `k8s.io/client-go/tools/clientcmd`
- `k8s.io/kubectl/pkg/util/slice`
- `net/http`
- `os`
- `os/exec`
- `path/filepath`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/client/config`
- `strings`

## Capabilities

- **`package`** name(s): **utils**.
- Go **`import`** edges listed below (24 unique path(s)).
- Package ID(s): `github.com/openshift/hive/contrib/pkg/utils`.

## Understanding Score

0.0
