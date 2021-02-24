# Cluster Relocation

Hive supports the ability to migrate a `ClusterDeployment` and its related custom resources to another Hive cluster through the use of the `ClusterRelocate` custom resource definition.

## Usage

In your source Hive cluster, create a Secret containing a kubeconfig for the destination Hive cluster in the namespace of your choosing:

```bash
$ kubectl create secret generic hub2-migration-kubeconfig -n default --from-file=kubeconfig=./hub2.kubeconfig
```

In your source Hive cluster, create a `ClusterRelocate` custom resource:

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterRelocate
metadata:
  name: migrator
spec:
  kubeconfigSecretRef:
    namespace: default
    name: hub2-migration-kubeconfig
  clusterDeploymentSelector:
    matchLabels:
      migrateme: hub2
```

Apply the desired label to your `ClusterDeployment`:

```bash
$ kubectl label clusterdeployment mycluster migrateme=hub2
```

The `ClusterDeployment` should appear in the destination hive, and be deleted in the source Hive, without triggering any cleanup of cluster resources.

## Caveats

The relocation process will migrate most of the relevant resources in a source namespace, so if you have multiple `ClusterDeployments` in one namespace, it is possible some of their secrets will be copied to the destination cluster even if only one of the `ClusterDeployments` matched the label selector. Best practice for Hive is to use a namespace per `ClusterDeployment`.

## Implementation Details

[Original Pull Request](https://github.com/openshift/hive/pull/1011)

Relocation was implemented via a `hive.openshift.io/relocate` annotation which is added to the `ClusterDeployment`. This annotation's value will be `{cluster_relocate_name}/{relocation_status}`, where status can be "outgoing", "complete", or "incoming". When this annotation is set, most hive controllers are disabled so as not to make changes during the relocation.

Relocation proceeds with several steps:

When a `ClusterRelocate` has a label selector that matches with a `ClusterDeployment`, the clusterrelocate controller will relocate all matching `ClusterDeployments`.

  1. Set the `hive.openshift.io/relocate` annotation to outgoing on the `ClusterDeployment` and the `DNSZone`.
  1. Copy `Secrets`, `ConfigMaps`, `MachinePools`, `SyncSets`, and `SyncIdentityProviders`.
  1. Copy `DNSZone` with the relocate annotation set to incoming.
  1. Copy the `ClusterDeployment`, with the relocate annotation set to incoming.
  1. Set the relocate annotation on the source `DNSZone` and `ClusterDeployemnt` to complete.
  1. Delete the source `DNSZone` and `ClusterDeployment` without running finalizer code that would destroy the cloud resources.

When a ClusterDeployment with a relocate annotation set to incoming is reconciled by the clusterRelocate controller (i.e. on the destination cluster), the relocate annotation is removed from the DNSZone and the ClusterDeployment. This indicates that the relocate has completed on the destination side.

When a ClusterDeployment has the relocated annotation, no controllers will do any mutation of the target cluster (such as processing SyncSets). Controllers will also not run finalizer code when the ClusterDeployment is deleted.


## Metrics

Number of migrations by `ClusterRelocate` name:

```
hive_cluster_relocations{cluster_relocate="migrator"} 2
```

Number of aborted migrations by `ClusterRelocate` name and reason. Possible values for the reason label are "no_match", "multiple_matches", and "new_match".

```
hive_aborted_cluster_relocations{cluster_relocate="",reason="no_match"} 5
```

The number of failing cluster relocates can be found by looking at the `hive_cluster_deployments_conditions` metric and filtering on
a value of RelocationFailed for the condition label.

