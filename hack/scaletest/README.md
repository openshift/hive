# Goal

Simulate 1000 clusters in Hive and observe performance. Because we do not have the budget or resources to actually provision 1000 clusters, we will provision one, and adopt 1000 copies of it's ClusterDeployment into Hive for testing. For the purposes of all API communication we will be talking to one cluster. Hive runs 5 threads per controller, so we should not be making excessive concurrent requests to the cluster. However we need to observe for problems from this caveat with regards to SyncSets possibly competing over the same resources and thrashing. If syncing the same resources however, we should always expect the same result, so thrashing should not be possible.

# Setup

Configure HiveConfig:

```yaml
apiVersion: hive.openshift.io/v1                                                                                                                                                                                                   │
kind: HiveConfig                                                                                                                                                                                                                   │
  metadata:                                                                                                                                                                                                                          │
    annotations:                                                                                                                                                                                                                     │
      hive.openshift.io/fake-applies: "true"   
spec:                                                                                                                                                                                                                              │
  controllersConfig:                                                                                                                                                                                                               │
    controllers:                                                                                                                                                                                                                   │
    - config:                                                                                                                                                                                                                      │
        concurrentReconciles: 25           
      name: clustersync
```

Create 60 SelectorSyncSets, each will contain 10 unique ConfigMaps with a little data, in the default namespace:

```bash
hack/scaletest/setup-selectorsyncsets.sh
```

Provision a real spoke cluster from the Hive hub cluster, or just use a hiveconfig from a spoke you provisioned elsewhere:

```bash
bin/hiveutil create-cluster --base-domain=new-installer.openshift.com --use-image-set=false scaletest-spoke
```

Get the cluster's admin kubeconfig which will be used to adopt this one cluster into thousands of duplicates, all backed by the same real spoke cluster:

```bash
./hack/get-kubeconfig.sh scaletest-spoke > scaletest-spoke.kubeconfig
```

Duplicate the real cluster by adopting copies to simulate cluster load in Hive. This can be run in multiple terminals to parallelize and generate more load, just use different start/end blocks in each.

```
$ time hack/scaletest/test_setup.sh cluster.kubeconfig 1 250
```

Setup a prometheus pod with storage in the hive namespace:
```bash
oc apply -f config/prometheus/prometheus-configmap.yaml
oc apply -f config/prometheus/prometheus-deployment-with-pvc.yaml
oc port-forward svc/prometheus -n hive 9091:9090
```
 
Load the [Prometheus WebUI](http://localhost:9091/graph).

Use [this link](http://localhost:9091/new/graph?g0.expr=workqueue_depth&g0.tab=0&g0.stacked=0&g0.range_input=2h&g1.expr=hive_syncsetinstance_apply_duration_seconds_sum%20%2F%20hive_syncsetinstance_apply_duration_seconds_count&g1.tab=0&g1.stacked=0&g1.range_input=1h&g2.expr=rate(hive_syncsetinstance_resources_applied_total%5B1m%5D)&g2.tab=0&g2.stacked=0&g2.range_input=1h&g3.expr=sum%20without(instance%2Cstatus%2Cresource)(hive_kube_client_request_seconds_sum%20%2F%20hive_kube_client_request_seconds_count%7Bremote%3D%22true%22%7D)&g3.tab=0&g3.stacked=0&g3.range_input=1h&g4.expr=rate(controller_runtime_reconcile_total%5B1m%5D)&g4.tab=0&g4.stacked=0&g4.range_input=15m&g5.expr=sum%20without(name)(hive_selectorsyncset_apply_duration_seconds_sum)%2Fsum%20without(name)(hive_selectorsyncset_apply_duration_seconds_count)&g5.tab=0&g5.stacked=0&g5.range_input=1h&g6.expr=sum%20without(instance%2Cstatus%2Cresource)(hive_kube_client_request_seconds_sum%7Bremote%3D%22false%22%7D%20%2F%20hive_kube_client_request_seconds_count%7Bremote%3D%22false%22%7D)&g6.tab=0&g6.stacked=0&g6.range_input=1h&g7.expr=rate(hive_kube_client_requests_total%5B5m%5D)&g7.tab=0&g7.stacked=0&g7.range_input=1h) for the graphs I was using for testing.

# 2020-07-16 Observations 

  1. Mass Cluster Adoption Phase
    * Adopted 1000 clusters with 4 concurrent terminals running the script. This took about 30 minutes.
    * SyncSet controller (which generates the SyncSetInstances for every matching cluster)
      * Workqueue grew to about 820.
      * Dropped to 0 after about 50 minutes. (20 minutes longer than it took to load all the cluster deployments)
      * Reconciled at about 0.5/s.
    * SyncSetInstance controller (which syncs resources to remote cluster if necessary)
      * We expect 60,000 SSIs, one per SyncSet per matching Cluster. (60 * 1000)
      * Workqueue grew to 60,000 after 50 minutes, roughly the time the SyncSet controller was busy creating them all.
      * Reconcile rate varied between 0.6/s and 1.2/s.
      * After this the workqueue came down but very slowly, which makes sense given the rate they're being reconciled.
        * 60,000, reconciling 1.2 per second = 14 hours to get through the queue.
      * Average individual resource apply time (for each resource in a syncsetinstance) ranged from 0.54s and 0.71s. Of this very little was spent with network communication for the required GET to ensure the resource already exists. (0.016s avg) I suspect the time is spent writing the temp file to disk and doing the apply logic.
    * Memory usage for hive-controllers pod started out at 96mb, but as we loaded up 1000 clusters and tens of thousands of SyncSetInstances it only peaked around 185mb.
      * I do not understand why this would be so low with everything in etcd, and my understanding of how caching works in controllers.
      * About one hour into the test memory jumped to 320mb.
    * Communication with the remote cluster appears to be very fast and very reliable despite using the same cluster for all of this. (currently only 5 goroutines per controller though, if we up that we may start hitting problems with re-using a real cluster)
      * SyncSetInstance controller GET requests to remote cluster avg about 0.016s.
      * SyncSetInstance controller POST requests to remote cluster avg about 0.008s but these are very rare because of how we're re-using the same resources in every cluster. Most apply attempts to not need a POST because the resource only exists, so we're only doing the GET.
    * CPU usage consistent around 2.4mc.
  1. Day 2 Pod Restart
    * Restarting the pod the next day memory use was 1.22 GB, lowered to 0.848 GB and stayed there for a while after. Not sure why this would be so much higher on restart than when we scaled up.
    * Attempted changing syncsetinstance controller goroutines from 5 to 100:
      * Memory use now up to 5-6GB on pod restart.
        * Dropped to about 2.6GB eventually.
      * SyncSet Resources Started Applying Slower
        * Time making the GETs to the server jumped from 0.01s to 0.03s, likely the load of extra requests.
        * Actual time to apply specific resources on Hive side jumped from 0.6s to 14.5s, major increase.
        * Rate at which we're applying appears unchanged: ~6/s. 
        * More goroutines doesn't seem to help, we're limited elsewhere, guessing filesystem writes.
    * Attempted a test with server side apply code, no more local file writes to apply, 5 goroutines per controller.
      * Big improvemensts.
      * syncsetinstance controller reconciling at 10/s, up from 1.6/s
      * time to apply individual resources now 0.03s, down from 0.659s. 
      * the huge improvement in apply time but lesser improvement in reconcile rate seems to imply we're fighting other controllers for CPU.
      * this was running on m4.xlarge, using about 2.7mc of cpu.
    * Attempt: c5.24xlarge with (96 vcpus), 5 goroutines per controller.
      * Memory started at 1.7GB and stabilized around 1GB.
      * Only used 5 cores according to kube.
      * Apply individual resources 0.02/s, processing 190/s
      * syncsetinstance controller reconciling at 30/s
    * Attempt: c5.24xlarge (96 vcpus), 100 goroutines for syncsetinstance controller, 5 goroutines for all others.
      * syncsetinstance controller reconciling at 90/s
      * Curned through entire 60k work queue in about 16 minutes.
      * Used 11 cores while working through queue, 0.7 cores after. 
      * 1.3 GB memory throughout.
      
## Conclusions

  * Remote server communication seems way faster than we expected. 
  * Current implementation, 1000 clusters, 60 SelectorSyncSets = 60,000 SyncSetInstances, processing 1.2/s would take 13.8 hours, and we would want to do this every 2 hours. Not enough time in the day to handle this amount of syncsets, new resources would be very slow to apply.
  * Current apply writes a temporary file and runs kubectl apply, this appears to be the bottle neck.
  * Switching to server side apply if available provides a dramatic increase.
  * Switching to a cpu optimized instance type and upping the syncsetinstance controller goroutines also provided a dramatic increase.
  * These two things combined took us from processing syncsetinstances at 1.2/s to 90/s with potential to go further.
  * This test helped plan for better syncsetinstance performance but it would be good to repeat with some of these changes and look for issues elsewhere in the code.
  
## Recommendations

  * HIGH: Patch resource/apply.go to use server side apply if the cluster is 4.5+. Test SyncSets carefully to make sure resources are getting applied the way we would expect.
  * HIGH: Patch resource/apply.go to use in-memory cache instead of a temp file, see how this performs vs server side apply. (Cesar a good point of contact for how to proceed with this)
  * HIGH: Switch Hive deployments to cpu optimized instance types with a lot of vcpus. I saw usage up to about 14 cores with 100 goroutines for ssi controller. We might be able to push this further.
  * HIGH: Make syncsetinstance controller goroutines configurable in HiveConfig. (maybe for all controllers?) This is applied in syncsetinstance_controller.go AddToManager.
  * MED: Test even more than 100 goroutines for syncsetinstance controller. Add uuids to contextual controller loggers to help debug logs when we need to.
  * MED: Make hive-controllers cpu/memory requests configurable in HiveConfig.
  * LOW: Consider splitting syncset instance controller into a separate pod to isolate logging and load and prevent resource contention between controllers.
      

# 2019-11-29 Observations

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

