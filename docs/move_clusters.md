# Moving Hive-managed Clusters

## Summary

There is a need to be able to move Hive-managed clusters from one Hive instance to another.
This may be useful if the user want to replace a Hive instance with a new one or if the user
wants to implement sharding between Hive instances.

Hive will manage all of the moving for the user. The user will create a CR indicating with
details about the destination cluster and label selectors to match against the cluster to
move.

## ClusterRelocator CRD

A ClusterRelocator is cluster-scoped.

This is an example of a ClusterRelocator that may be used to move all of the ClusterDeployments
with the label `hive-instance: hive2` to the remote cluster accessed via the kubeconfig secret
at "hive2-remote-kubecofig-secret" in the "hive" namespace.

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterRelocator
metadata:
  name: hive2-cluster-relocator
spec:
  kubeconfigSecretRef:
    name: hive2-remote-kubeconfig-secret
    namespace: hive
  clusterDeploymentSelector:
    matchLabels:
      hive-instance: hive2
```

## Process

There is a controller that will reconcile ClusterDeployments and watch ClusterDeployments
and ClustereRelocators. For ClusterDeployments that match with a ClusterRelocator, the
controller will move the ClusterDeployment to the Hive instance associated with the kubeconfig
referenced in the ClusterRelocator.

1. Add the `hive.openshift.io/relocating=<relocator-name>` annotation to the ClusterDeployment. This informs other controllers not to take action on the cluster.
This will block syncsets and machinesets from being applied and will block the clusterdeployment
controller from running its finalizer code for the ClusterDeployment.
1. Create a namespace (or project?) for the ClusterDeployment in the remote cluster.
1. Copy Secrets, ConfigMaps, MachinePools, SyncSets, and SyncIdentityProviders associated
with the ClusterDeployment to the remote cluster.
1. Copy the ClusterDeployment to the remote cluster.
1. Delete the ClusterDeployment.
1. Remove the `hive.openshift.io/relocating` annotation. Replace it with a
`hive.openshift.io/relocated=<relocator-name>` annotation. This frees the clusterdeployment
controller to run its finalizer code for the ClusterDeployment but informs the controller
to run perform a deprovision.


## Concerns
1. How does the dnszone controller get informed that the DNS records should be preserved?
1. What should happen if there are SelectorSyncSets in the source Hive instance that are not present
in the destination Hive instance?