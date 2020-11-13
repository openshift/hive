# Cluster Pools

## Overview

Hive exposes a `ClusterPool` API which allows users to maintain a pool of "hot"
precreated `ClusterDeployments`, ready to be claimed when needed. The pool size
can be configured and Hive will attempt to maintain that set number of
clusters.

When a user needs a cluster they create a `ClusterClaim` resource which will be
filled immediately with details on where to find their cluster.  (or as soon as
a cluster is available in the pool if none were presently available). Once
claimed, a cluster is removed from the pool and a new one will be created to
replace it. Claimed clusters never return to the pool, they are intended to be
destroyed when no longer needed. The `ClusterClaim.Spec.Namespace` will be
populated once the claim has been filled, and the `ClusterDeployment` will be
present in that namespace with an identical name.

`ClusterPools` are a namespaced resource, and can be used to centralize billing
by using a namespace limited to a team via Kubernetes RBAC. All clusters in the
pool will use the same set of cloud credentials specified in the platform for
the pool. `ClusterClaims` must be created in the same namespace as their
`ClusterPool`, but each actual `ClusterDeployment` is given its own namespace.
The user who claims a cluster can be given RBAC to their clusters namespace to
prevent anyone else from being able to access it.

Presently once a `ClusterDeployment` is ready, it will be
[hibernated](./hibernating-clusters.md) automatically. Once claimed it will be
automatically resumed, meaning that the typical time to claim a cluster and be
ready to go is in the 2-5 minute range while the cluster starts up. An option
to keep the clusters in the pool always running so they can instantly be used
may be added in the future.

When done with a cluster, users can just delete their `ClusterClaim` and the
`ClusterDeployment` will be automatically deprovisioned. An optional
`ClusterClaim.Spec.Lifetime` can be specified after which a cluster claim will
automatically be deleted. The namespace created
for each cluster will eventually be cleaned up once deprovision has finished.

Note that at present, the shared credentials used for a pool will be visible
in-cluster. This may improve in the future for some clouds.

## Supported Cloud Platforms

`ClusterPool` currently supports the following cloud platforms:

  * AWS
  * Azure
  * GCP

## Sample Cluster Pool

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterPool
metadata:
  name: openshift-46-aws-us-east-1
  namespace: hive
spec:
  baseDomain: new-installer.openshift.com
  imageSetRef:
    name: openshift-4.6
  platform:
    aws:
      credentialsSecretRef:
        name: hive-team-aws-creds
      region: us-east-1
  pullSecretRef:
    name: hive-team-pull-secret
  size: 1
```

## Sample Cluster Claim

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterClaim
metadata:
  name: dgood46
  namespace: hive
spec:
  clusterPoolName: openshift-46-aws-us-east-1
  lifetime: 8h
  namespace: openshift-46-aws-us-east-1-j495p # populated by Hive once claim is filled and should not be set by the user on creation
status:
  conditions:
  - lastProbeTime: "2020-11-05T14:49:26Z"
    lastTransitionTime: "2020-11-05T14:49:26Z"
    message: Cluster claimed
    reason: ClusterClaimed
    status: "False"
    type: Pending
```

