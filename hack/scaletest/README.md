# Goal

Simulate 1000 clusters in Hive and observe performance. Because we do not have the budget or resources to actually provision 1000 clusters, we will provision one, and adopt 1000 copies of it's ClusterDeployment into Hive for testing. For the purposes of all API communication we will be talking to one cluster. Hive runs 5 threads per controller, so we should not be making excessive concurrent requests to the cluster. However we need to observe for problems from this caveat with regards to SyncSets possibly competing over the same resources and thrashing. If syncing the same resources however, we should always expect the same result, so thrashing should not be possible.

# Setup

Hive running on a standard 3 master 3 node OpenShift 4.2 cluster.

Another identical cluster is being used for the duplication.

```
$ time hack/scaletest/test_setup.sh cluster.kubeconfig 1000
```

Once we have 1000 clusters, create 30 SelectorSyncSets with different resources, which match all our clusters. (repeat for 0-29)

```
oc process -f hack/scaletest/syncset-template.yaml RESOURCE_INDEX=0 | oc apply -f -
```

Setup a transient prometheus pod in the hive namespace (see developer docs) and used the following queries to monitor Hive behavior:

```
rate(controller_runtime_reconcile_total[5m])
rate(hive_controller_reconcile_seconds_sum[5m]) / rate(hive_controller_reconcile_seconds_count[5m])
rate(hive_controller_reconcile_seconds_sum[5m]) / rate(hive_controller_reconcile_seconds_count[5m])
```

# Observations Nov 29, 2019

  1. Creating the 1000 duplicate clusters with no SyncSets in play took one hour.
    * Memory usage grew linearly to 560MB.
    * CPU usage was constant around 200mc.
    * clusterversion and clusterstate controllers were making remote/local kube requests around .2/second
    * metrics controller reconciling in 11ms.
  1. Shortly thereafter CPU usage dropped to ~20mc. All controller reconcile rates were showing 0/s with exception of ClusterState.
    * This is unexpected as clusterversion should also be periodically updating.
    * ClusterState uses explicit RequeueAfters. ClusterVersion does not.
    * I believe this is a bug, or rather several bugs. Much of Hive was written on assumption that everything was reconciled every 15 minutes. This does not appear to be the case anymore. Filed as action item.
  1. Deployed first selector syncset that matches all ClusterDeployments: `oc process -f hack/scaletest/syncset-template.yaml RESOURCE_INDEX=0 | oc apply -f -`
    * Processed in a matter of seconds and logs settled down.
    * Memory spiked to 1.27GB.
    * CPU spiked for approx 9 minutes to 950mc.
    * clusterdeployment and syncsetinstance controllers spiked to around 10.5 reconciles per second.
    * syncsetinstance controller reconciles were taking ~ 0.2s.
    * syncsetinstance controller peaked at about 7 local kube API requests per second.
    * syncset controller peaked at about 3.5 local kube API requests per second.
    * No controllers showed remote kube API requests. May indicate a bug in our code that was once capturing this.
  1. Added four more SelectorSyncSets for a total of 5 using the command above with incremental RESOURCE_INDEX values.
    * CPU spiked to 2000mc for about 12 minutes before settling.
    * Memory use actually declined to 945MB. (triggered GC?)
    * Peak controller reconcile rates:
      * clusterdeployment: 37/s
      * syncsetinstance: 24/s
      * syncset: 4/s
    * Peak controller reconcile durations:
      * syncsetinstance: 0.2s - 0.35/s
    * Peak kube API requests:
      * syncsetinstance: 19/s
      * syncset: 17/s
  1. Allowed cluster to settle. Memory 500mb, CPU 36mc.
  1. Added 25 more SelectorSyncSets for a final total of 30. For future tests it would be fine to make these all at once.
    * CPU peaked around 2100mc for 1 hour before things finally settled.
    * Memory use around 800mb throughout, dropped to 460mb once everything was initially processed.
    * Peak controller reconcile rates:
      * clusterdeployment: 8/s
      * syncsetinstance: 43/s
  1. End result, cluster quiet at 36mc CPU and 460MB RAM.
  1. With 31k SyncSetInstances in etcd, kubectl get SelectorSyncSet -A takes 16 seconds. Per namespace where there are only 30, the response is instant.
  1. Adopting a new cluster causes no significant spike in CPU, nor does deleting it.
  1. Adding a resource to a pre-existing SelectorSyncSet matching all 1000 clusters took about 2.5 minutes. Considerably faster than the 10 minutes it takes to add a new SelectorSyncSet.

## Action Items

  1. Audit all controllers. If we expect a periodic requeue, we must explicitly request it when we return from the reconcile loop.
  1. Investigate if syncsetinstance controller remote API apply calls are properly registering in the hive_kube_client_requests_total metric.
  1. Investigate MachinePool management with adopted clusters. Looked like something is a little funky there.
  1. Next test, try scaling back to 15 SelectorSyncSets, and adding in 15 regular non-selector SyncSets generated per cluster with two resources, a namespace, and a configmap in that namespace. This will mean 15k namespaces. This will increase the time to setup from one hour to about 4.

