# Scalable SyncSet Processing

## Summary

In large Hive deployments (current recommendation is to stay below 1000 ClusterDeployments), with fairly heavy SyncSet usage (approximately 60 SyncSets with about 10 resources each, a total of 600 resources to apply), the SyncSet processing has proven to be our only relevant performance bottleneck. This proposal is to implement a system by which we can horizontally scale a set of pods for SyncSet processing to eliminate the bottleneck instead of continuing to fight with it.

## Motivation

Even after implementing dramatic changes to improve SyncSet performance, we are struggling to get past a max of about 20 applies/s. Full clustersync controller reconcile time can range from 2 to 5 minutes to process everything for one cluster under load. At 20 goroutines on an AWS c5.4xlarge, the workqueue is struggling to process.

Individual apply time appears to be approximately 0.2s - 1.6s, although 99% of all applys complete in under 0.5s. 0.5s * 600 = 300 seconds to process an entire cluster.

Attempts to increase the goroutines to 100 lead to pod instability, at present problems with allocating OS threads.

Moving to a design where we can horizontally scale the number of pods processing syncsets should let us scale out to meet the requirements of heavier SyncSet usage. In doing so we may be able to push far beyond the 1k ClusterDeployment limit and possibly save substantial cost for extremely large Hive deployments now requiring fewer shards.


### Goals

  1. Eliminate the SyncSet processing bottleneck in large Hive environments and enable horizontal scaling.

### Non-Goals

1. Moving SyncSets to a separate and independent project. This has been discussed for a long time and would overall be great. However this proposal is a step in this direction, the implementation would still need to be refined and extracted into a separate project to accomplish this fully.
1. Use of a separate datastore. The data implications of processing SyncSets and storing results for the apply operations is still extremely questionable for etcd storage. In the future, it would be nice to have this data in another more suitable datastore. However this is not considered for this proposal and is being explicitly avoided.
1. Opt-in to the new SyncSet method. This would be the only implementation moving forward, we will not maintain both the current and the new.

## Open Questions

RabbitMQ is not presently available on OperatorHub. [An operator](https://www.rabbitmq.com/kubernetes/operator/operator-overview.html) does however exist. How could Hive consume this operator and RabbitMQ safely?

## Proposal

Add a message bus to Hive deployments for processing SyncSets. Consumer pods will subscribe to the message bus queue and consume messages on the bus which are requests to apply resources.

## Implementation Details / Notes / Constraints

Deploy a RabbitMQ message bus along-side Hive.

Hive's ClusterSync controller (or equivalent) will serve as the publisher of messages on the bus to a work queue. Each message will contain an identifier for the ClusterDeployment and possibly the SyncSet to process.

Subscriber pods will process these messages, each message will be delivered to only one of the subscriber pods. (a supported configuration with RabbitMQ) Subscriber pods will initially leverage the normal use of Kubernetes clients and caching such that the actual data for the SyncSet resources and cluster kubeconfig Secrets are ready in memory when the message arrives. 

Messages will initially be a request to sync *everything* for a cluster, exactly as the clustersync controller operates today. 
* Doing this will allow us to have our subscriber pods write to the ClusterSync CR status as the controller does today, which means the API functions as before for users. 
* We know this can take minutes and that is unlikely to change if we process everything serially. However we could potentially roll our own concurrency and spawn multiple goroutines for each SyncSet to serially process the ordered resources within a SyncSet, but process all SyncSets in parallel. This would give us parallel processing, and still allow us to report our status in the ClusterSync controller.

Kubernetes [horizontal pod autoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/) can be used to scale the number of subscriber pods based on current load based on CPU use, or custom metrics.

## Risks and Mitigations

### Status Reporting

Today we store SyncSet apply status per cluster in two places:

  1. ClusterSync.Status, full details of everything we've synced.
  1. ClusterDeployment.Status.Conditions: a single condition indicating if this cluster currently has problems applying any of it's SyncSets.
  
Ideally this would be better suited for a database that can handle more data and faster writes than etcd and Custom Resources. However this is considered out of scope for an MVP of this feature. 

For now, we will assume that the subscriber pods can write both of these resources to etcd. It should be noted that this will cause the ClusterSync controller to immediately re-reconcile, this may prove useful in the error handling section below. 
  
### Error Handling

Today on SyncSet error we can requeue the ClusterSync and leverage the benefits of Kubernetes controller queues with exponential backoff up to a maximum of 6 minutes. In stepping outside this framework we now have to implement our own logic for how to handle errors and retries.

Bus messages can easily contain an index for attempt number which subscriber pods could increment on an error and push back onto the bus. However this does not include any backoff and would result in thrashing attempting to apply resources that may be fundamentally broken in most cases. RabbitMQ does not natively support delayed message delivery, but plugins are available which could be used.

To mitigate


If subscribers are reporting status by writing the ClusterSync resource, it is possible we could include enough information for the publisher to decide when to requeue.

### Test Plan

## Drawbacks

## Alternatives

Regarding the proposal above for full sync to a cluster per message:
* Message per individual resource to apply to a cluster would be a bit too granular and would violate the ordering of resource within a SyncSet.
* Message per SyncSet per cluster would be a nice in-between but will complicate reporting status via writing to ClusterSync, which would frequently collide causing unnecessary failures despire successful applies.

Regarding the overall message bus architecture, we could instead pivot to an in-cluster operator which is responsible for performing SyncSet applies. Hive would need to deploy and maintain this operator across the fleet of clusters registered to it, and be responsible for syncing status back to the hub Hive cluster. Offloading the work to in-cluster would likely scale very well, however there are substantial security and deployment implications to taking on the responsibility of deploying and running a client side operator in every cluster.

Regarding the requeue and backoff mechanism:
An alternative could be to use a dead letter queue to store messages that need future requeuing, and periodically scan for messages due to be moved back to the processing queue.

## Infrastructure Needed

Hive deployments may now require additional resources. We will be running several new pods, one rabbitmq and multiple syncset consumers.
