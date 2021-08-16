# SyncSet

## Overview

`SyncSet` and `SelectorSyncSet` objects facilitate resource management (create, update, delete, patch) in hive-managed clusters.

To use `SyncSet` objects to manage resources, you must create them in the same namespace as the `ClusterDeployment` resource that they manage. If you want to manage resources in clusters that match a specific label use `SelectorSyncSet` instead. These objects apply changes to clusters in any namespace that match the `clusterDeploymentSelector` that you set.

With both `SyncSets` and `SelectorSyncSets`, the individual resources and patches are reapplied when 2 hours have passed since their last reconciliation (by default) or if their contents are updated in the `SyncSet` or `SelectorSyncSets`.

The default `syncSetReapplyInterval` can be overridden by specifying a string duration within the `hiveconfig` such as `syncSetReapplyInterval: "1h"` for a one hour reapply interval.

## SyncSet Object Definition

`SyncSets` may contain a list of resource object definitions to create and a list of patches to be applied to existing objects.

```yaml
---
apiVersion: hive.openshift.io/v1
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
    patch: |-
      { "data": { "foo": "new-bar" } }
    patchType: merge
    
  secretMappings:
  - sourceRef:
      name: ad-bind-password
      namespace: default
    targetRef:
      name: ad-bind-password
      namespace: openshift-config
```

| Field | Usage |
|-------|-------|
| `clusterDeploymentRefs` | List of `ClusterDeployment` names in the current namespace which the `SyncSet` will apply to. |
| `resourceApplyMode` | Defaults to `"Upsert"`, which indicates that objects will be created and updated to match the `SyncSet`. Existing `SyncSet` resources that are not listed in the `SyncSet` are not deleted. Specify `"Sync"` to allow deleting existing objects that were previously in the resources list. This includes deleting _all_ resources when the entire SyncSet is deleted. |
| `resources` | A list of resource object definitions. Resources will be created in the referenced clusters. |
| `patches` | A list of patches to apply to existing resources in the referenced clusters. You can include any valid cluster object type in the list. By default, the `patch` `applyMode` value is `"AlwaysApply"`, which applies the patch every 2 hours. |
| `secretMappings` | A list of secret mappings. The secrets will be copied from the existing sources to the target resources in the referenced clusters |

### Example of SyncSet use

In this example you can change the replicaset of a deployment running on top of a Hive managed OpenShift cluster.

* Get the required information of the deployment

```console
$ oc get deployment <deployment name> -o yaml
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    app: sise
  name: sise-deploy
  namespace: default
  replicas: 2
  xxxxxxx
```

* Create the `SyncSet` object in the cluster deployment namespace in Hive managed cluster as mentioned below.

```sh
oc create -f <syncset_file.yaml> -n <namespace>
```

SyncSet File:

```yaml
apiVersion: hive.openshift.io/v1
kind: SyncSet
metadata:
  name: sise-deploy-syncset
spec:
  clusterDeploymentRefs:
  - name: <cluster name>

  patches:
  - kind: Deployment
    apiVersion: extensions/v1beta1
    name: sise-deploy
    namespace: default
    patch: |-
      { "spec": { "replicas": 3 } }
    patchType: strategic
```

* To see the syncset status, run as below.

```sh
oc get clustersync <clusterdeployment name> -o yaml
```

## SelectorSyncSet Object Definition

`SelectorSyncSet` functions identically to `SyncSet` but is applied to clusters matching `clusterDeploymentSelector` in any namespace.

```yaml
---
apiVersion: hive.openshift.io/v1
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
    matchLabels:
      cluster-group: abutcher
```

| Field | Usage |
|-------|-------|
| `clusterDeploymentSelector` | A key/value label pair which selects matching `ClusterDeployments` in any namespace. |

## Diagnosing SyncSet Failures

The failure logs for syncset is present in Hive controller POD logs.

To find the status of the syncset, check the cluster deployment's `ClusterSync` object in the cluster deployment namespace. Every cluster deployment has an associated `ClusterSync` object that records status within `ClusterSync.Status.SyncSets`.

```sh
oc get clustersync -n <namespace>
```

To see details, run as below.

```sh
oc get clustersync <clusterdeployment name> -o yaml
```

## Changing ResourceApplyMode

Changing the `resourceApplyMode` from `"Sync"` to `"Upsert"` will remove `SyncSet` resources tracked for deletion within the corresponding `ClusterSync` object. It is possible that the `ClusterSync` controller could process a resource removal and a `resourceApplyMode` change simultaneously and when this occurs resources no longer tracked in the `SyncSet` will be orphaned rather than deleted.

Likewise, changing the `resourceApplyMode` from `"Upsert"` to `"Sync"` will add `SyncSet` resources to resources tracked for deletion within the corresponding `ClusterSync` object. When the `ClusterSync` controller processes a resource removal and a `resourceApplyMode` change simultaneously, resources removed will be orphaned rather than deleted.
