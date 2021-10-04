# Automatically detect and replace broken Clusters

## Summary

Hibernating clusters often fail to transition to running state, but are assigned to ClusterClaims today. We would want to 
validate a cluster can be successfully resumed and is running as expected before assigning it to a claim.

## Motivation

As we move towards being more proactive with having clusters ready as [hot spares](https://github.com/openshift/hive/pull/1434),
we could come across instances where stale hibernated clusters could fail to resume and still be assigned to ClusterClaims.
There's a need to detect broken clusters and replace them automatically.

### Goals

Always strive to assign a running cluster to a claim

### Non Goals

Fixing clusters (or attached claims) once a running cluster has already been assigned.

## Proposal

### Stand-by clusters

Currently, we consider all installed clusters which are not set to be deleted as Assignable clusters. 
Introduce another bucket of stand by clusters, which are installed and are not set for deletion, but are hibernating.
A cluster is only assignable if it is running. A hibernating cluster has to be resumed successfully to be added to the 
assignable clusters bucket.
Some factors that might inhibit successful resume of a cluster that has been hibernating for a while include unapproved 
certificates, expired credentials and operator bugs. Introduction of a separate bucket for hibernating clusters allows 
for a validation before it is assigned to a claim.

### Transition to Assignable clusters

If a claim is created and there are no assignable clusters, we would resume a hibernating cluster and check if the resume
was successful. The validating condition could be the expected powerstate matches the actual powerstate. If we encounter
an error, we can mark that cluster to be deleted and try to transition the next available stand-by cluster.
Figuring out how long we wait for the resume process to complete can be tricky - as there are instances when we can wait
indefinitely. So we can consider a timeout for that wait. Fine-tuning the said timeout would be dependent on the 
platform and the infrastructure, hence left for the admins to decide. Default is no time-out - so waiting indefinitely 
unless an error is encountered.
For a cluster that is determined broken and hence marked to be deleted, the deletion would be immediate if within the
maxConcurrent budget. As per the current ClusterPool logic, that cluster will be deleted eventually and a new cluster
would be brought up in its place.

### Timeouts in Hibernation controller

Introduce the following optional timeouts in cluster deployments and corresponding fields in cluster pool 
- spec.powerstateTransitionToRunningTimeout
  - maximum wait time for a cluster to resume once the hibernating condition reflects cluster resuming reason
- spec.powerstateTransitionToHibernatingTimeout
  - maximum wait time for a cluster to hibernate once the hibernating condition reflects cluster stopping reason

Once the powerstate is changed and hibernating condition reflects Resuming or Stopping reason, controller should allow 
for a maximum wait time counting from the last transition time of the condition equal to the relevant timeout before 
determining that the cluster is broken.
If a cluster is broken, the hibernating condition can be updated with FailedToStart or FailedToStop reason and with a 
message indicating it timed out waiting for the cluster to stop or start, and the cluster can be marked for deletion.

These timeouts are only applicable to non-fake clusters as they begin counting from the interim state - whereas the 
powerstate transition of fake clusters is immediate.

## Risks and Mitigations

### Timeout too little

Cluster admins will be in charge of ensuring the mentioned timeouts are not too little, to give the cluster a fighting 
chance to be successfully transitioned.

### Cluster is running but needs additional validation

If a cluster has never been hibernated or if has been running for a while, it might need some additional validation to 
ensure it is ripe for assignment. Some known reasons for these are expired credentials, unapproved certificates and 
cluster operators in degraded state. Validations for the same are worthwhile but out of the scope for this enhancement
and are being pursued separately.
