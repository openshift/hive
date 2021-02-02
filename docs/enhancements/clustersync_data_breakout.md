# Move SyncSet Status out of Etcd


## Summary
The purpose of this enhancement is to break out SyncSet status out of Etcd and move to external storage

## Motivation
Once a cluster is installed, ClusterSync account for a bulk of Etcd's traffic and storage since it stores per cluster
status for all matching SyncSets. It can easily be reconstructed if lost, and it is not a resource we reconcile on.
In order to avoid such regular large writes to Etcd, Hive SyncSet status is the perfect candidate to move out of Etcd.
This can help Hive clusters scale to the anticipated 1000 per Hive target.

### Goals
Deploy an alternative storage backend via the Hive Operator and eliminate the ClusterSync Etcd storage.

### Non-Goals
None

## Open Questions
1. How to get the details of failing SyncSets for a cluster? Some options to consider:
    1. Add a small REST API and possibly CLI tool to scrape. However, how to secure the REST API?
    1. Add a CRD to request SyncSet status, only used when someone is explicitly debugging a problem, and handle cleaning
    up shortly thereafter.

## Proposal
- Deploy an official Red Hat postgresql image automatically via the Hive operator. On a later date, we may let the users
  point to an external database they control.
- Hive operator managing schema migrations automatically.
- Hive ClusterSync pods continue reconciling on ClusterDeployment as they do today, but now store and load ClusterSync 
  state in/from postgres.

## Implementation Details / Notes / Constraints
1. postgresql is relational unlike Etcd, so we have to set up schema. We can use https://github.com/pressly/goose
1. We can store the status as XML directly? - will we replace directly if it changes?

## Risks and Mitigations
- Role based access - postgresql allows fine-grained control
- Delete protection? Or reconstruction

### Test Plan


## Drawbacks
- End user must maintain external storage, including upgrading it when it is out of space.
- Default is deploying the external storage - what happens when there's an issue with that storage?

## Alternatives
- Use aggregated APIServer, backed by either a separate Etcd, or a different layer of database (and maintain that 
  database), although we have been cautioned this has not been done successfully yet, but some projects like 
  https://github.com/kubernetes-sigs/apiserver-runtime are attempting it.
