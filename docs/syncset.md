# SyncSet

## Overview

`SyncSet` and `SelectorSyncSet` objects facilitate resource management (create, update, delete, patch) in hive-managed clusters.

To use `SyncSet` objects to manage resources, you must create them in the same namespace as the `ClusterDeployment` resource that they manage. If you want to manage resources in clusters that match a specific label use `SelectorSyncSet` instead. These objects apply changes to clusters in any namespace that match the `clusterDeploymentSelector` that you set.

With both `SyncSets` and `SelectorSyncSets`, the individual resources and patches are reapplied when 2 hours have passed since their last reconciliation or if their contents are updated in the `SyncSet` or `SelectorSyncSets`.

## SyncSet Object Definition

`SyncSets` may contain a list of resource object definitions to create and a list of patches to be applied to existing objects.

```yaml
---
apiVersion: hive.openshift.io/v1alpha1
kind: SyncSet
metadata:
  name: mygroup
spec:
  clusterDeploymentRefs:
  - name: ClusterName

  resourceApplyMode: Upsert

  resources:
  - apiVersion: user.openshift.io/v1
    kind: Group
    metadata:
      name: mygroup
    users:
    - myuser

  patches:
  - kind: ConfigMap
    apiVersion: v1
    name: foo
    namespace: default
    applyMode: AlwaysApply
    patch: |-
      { "data": { "foo": "new-bar" } }
    patchType: merge
```

| Field | Usage |
|-------|-------|
| `clusterDeploymentRefs` | List of `ClusterDeployment` names in the current namespace which the `SyncSet` will apply to. |
| `resourceApplyMode` | Defaults to `"Upsert"`, which indicates that objects will be created and updated to match the `SyncSet`. Existing resources that are not listed in the `SyncSet` are retained. Specify `"Sync"` to delete existing objects that were previously in the `resources` list. |
| `resources` | A list of resource object definitions. Resources will be created in the referenced clusters. |
| `patches` | A list of patches to apply to existing resources in the referenced clusters. You can include any valid cluster object type in the list. By default, the `patch` `applyMode` value is `"AlwaysApply"`, which applies the patch every 2 hours. You can also specify`"ApplyOnce"` to apply the patch only once. | 



## SelectorSyncSet Object Definition

`SelectorSyncSet` functions identically to `SyncSet` but is applied to clusters matching `clusterDeploymentSelector` in any namespace.

```yaml
---
apiVersion: hive.openshift.io/v1alpha1
kind: SelectorSyncSet
metadata:
  name: mygroup
spec:
  resources:
  - apiVersion: user.openshift.io/v1
    kind: Group
    metadata:
      name: mygroup
    users:
    - abutcher
  clusterDeploymentSelector:
    cluster-group: abutcher
```

| Field | Usage |
|-------|-------|
| `clusterDeploymentSelector` | A key/value label pair which selects matching `ClusterDeployments` in any namespace. |

## Diagnosing SyncSet problems

`SyncSet` status is stored within `ClusterDeployment` status for each cluster the `SyncSet` applies to. Status will be kept per resource and patch unless an error was encountered gathering resource info. In this case, status contains the index of the broken resource along with an error message.

```yaml
  syncSetStatus:
  - conditions:
    - lastProbeTime: 2019-04-09T21:06:29Z
      lastTransitionTime: 2019-04-09T21:06:29Z
      message: 'Unable to gather Info for SyncSet resource at index 0 in resources:
        could not get info from passed resource: unable to recognize "object": no
        matches for kind "Group" in version "v1"'
      reason: UnknownObjectFound
      status: "True"
      type: UnknownObject
    name: mygroup
```
