# SyncIdentityProvider

## Overview

Use `SyncIdentityProvider` and `SelectorSyncIdentityProvider` objects to manage cluster authentication for hive-managed clusters. You use these objects to share your identity providers among all valid clusters.

Like `SyncSets` and `SelectorSyncSets`, you use a `SyncIdentityProvider` to manage identity providers in clusters that are in the same namespace as the `SyncIdentityProvider` object. To manage identity providers in clusters that match a specific label, use a `SelectorSyncIdentityProvider`.

## SyncIdentityProvider Object Definition

A `SyncIdentityProvider` contains a list of `identityProviders` to apply to specific clusters in that namespace. When you apply the `SyncIdentityProvider`, a `SyncSet` is generated for each `IdentityProvider` that contains a patch that modifies the cluster `oauth` object.

```yaml
---
apiVersion: hive.openshift.io/v1
kind: SyncIdentityProvider
metadata:
  name: allowall-identity-provider
spec:
  identityProviders:
  - name: my_allow_provider
    challenge: true
    login: true
    mappingMethod: claim
    type: AllowAllPasswordIdentityProvider
  clusterDeploymentRefs:
  - name: "MyCluster"
```

| Field | Usage |
| ----- | ----- |
| `identityProviders` | List of identity providers to be used for matching clusters. |
| `clusterDeploymentRefs` | List of `ClusterDeployment` names in the current namespace which the `SyncIdentityProvider` will apply to. |

## SelectorSyncIdentityProvider Object Definition

`SelectorSyncIdentityProvider` functions identically to `SyncIdentityProvider` but is applied to clusters matching `clusterDeploymentSelector` in any namespace.

```yaml
---
apiVersion: hive.openshift.io/v1
kind: SelectorSyncIdentityProvider
metadata:
  name: allowall-identity-provider
spec:
  identityProviders:
  - name: my_allow_provider
    challenge: true
    login: true
    mappingMethod: claim
    type: AllowAllPasswordIdentityProvider
  clusterDeploymentSelector:
    cluster-group: abutcher
```

| Field | Usage |
| ----- | ----- |
| `clusterDeploymentSelector` | A key/value label pair which selects matching `ClusterDeployments` in any namespace. |
