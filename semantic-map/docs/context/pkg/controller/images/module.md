<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/images/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `DefaultHiveImage`
- `GetHiveImage` — GetHiveImage returns the hive image to use in controllers. Either the one specified in the environment variable or the hardcoded default.
- `GetHiveImagePullPolicy` — GetHiveImagePullPolicy returns the policy to use when pulling the hive image. Either the one specified in the environment variable or the hardcoded default.
- `HiveClusterProvisionImagePullPolicyEnvVar`
- `HiveImageEnvVar`
- `HiveImagePullPolicyEnvVar`

## Internal Dependencies

- `k8s.io/api/core/v1`
- `os`

## Capabilities

- **`package`** name(s): **images**.
- Go **`import`** edges listed below (2 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/images`.

## Understanding Score

0.0
