# Module atlas

## Responsibility

Small utility package that provides functions to determine which container image and pull policy to use for Hive provisioning/deprovisioning jobs. Values come from environment variables with hardcoded defaults.

## Public Interface/API

- `GetHiveImage` -- Returns the Hive image from `HIVE_IMAGE` env var, or the default `registry.ci.openshift.org/openshift/hive-v4.0:hive`.
- `GetHiveImagePullPolicy` -- Returns pull policy from `HIVE_IMAGE_PULL_POLICY` env var, or `Always`.
- `HiveImageEnvVar` -- Constant `"HIVE_IMAGE"`.
- `HiveImagePullPolicyEnvVar` -- Constant `"HIVE_IMAGE_PULL_POLICY"`.
- `HiveClusterProvisionImagePullPolicyEnvVar` -- Constant `"HIVE_CLUSTER_PROVISION_IMAGE_PULL_POLICY"` (separate policy for cluster provisions to support local cache fallback).
- `DefaultHiveImage` -- Constant with default image reference.

## Internal Dependencies

- `k8s.io/api/core/v1` -- PullPolicy type.

## Capabilities

Pure utility package with no controller or reconciler. Provides image configuration for other controllers that create jobs.

## Understanding Score

0.90
