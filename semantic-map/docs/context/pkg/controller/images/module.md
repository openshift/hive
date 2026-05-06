# Module atlas

## Responsibility

Provides utility functions for determining the Hive container image and pull policy used across controllers, sourced from environment variables with hardcoded defaults.

## Public Interface/API

- `HiveImageEnvVar` -- constant `"HIVE_IMAGE"`
- `DefaultHiveImage` -- constant `"registry.ci.openshift.org/openshift/hive-v4.0:hive"`
- `HiveImagePullPolicyEnvVar` -- constant `"HIVE_IMAGE_PULL_POLICY"`
- `HiveClusterProvisionImagePullPolicyEnvVar` -- constant `"HIVE_CLUSTER_PROVISION_IMAGE_PULL_POLICY"`
- `GetHiveImage() string` -- returns hive image from env var or default
- `GetHiveImagePullPolicy() corev1.PullPolicy` -- returns pull policy from env var or `PullAlways`

## Internal Dependencies

- `k8s.io/api/core/v1` -- PullPolicy type
- `os` -- environment variable lookup

## Capabilities

- Reads `HIVE_IMAGE` environment variable to determine the container image for Hive controllers
- Reads `HIVE_IMAGE_PULL_POLICY` for general pull policy and `HIVE_CLUSTER_PROVISION_IMAGE_PULL_POLICY` for cluster provision-specific pull policy
- Falls back to hardcoded defaults when environment variables are not set

## Understanding Score

0.9
