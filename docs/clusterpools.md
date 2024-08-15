# Cluster Pools

- [Overview](#overview)
- [Supported Cloud Platforms](#supported-cloud-platforms)
- [Sample Cluster Pool](#sample-cluster-pool)
- [Sample Cluster Claim](#sample-cluster-claim)
- [Managing admins for Cluster Pools](#managing-admins-for-cluster-pools)
- [Install Config Template](#install-config-template)
- [Time-based scaling of Cluster Pool](#time-based-scaling-of-cluster-pool)
- [ClusterPool Deletion](#clusterpool-deletion)
- [Troubleshooting](#troubleshooting)

## Overview

Hive exposes a `ClusterPool` API which allows users to maintain a pool of "hot"
precreated `ClusterDeployments`, ready to be claimed when needed. The pool size
can be configured and Hive will attempt to maintain that set number of
clusters.

When a user needs a cluster they create a `ClusterClaim` resource which will be
filled with details on where to find their cluster as soon as
one is available from the pool and running. Once
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
The user who claims a cluster can be given RBAC to their cluster's namespace to
prevent anyone else from being able to access it.

By default once a `ClusterDeployment` is ready, it will be
[hibernated](./hibernating-clusters.md) automatically. Once claimed it will be
automatically resumed, meaning that the typical time to claim a cluster and be
ready to go is in the 2-5 minute range while the cluster starts up. You can
keep a subset of clusters active by setting `ClusterPool.Spec.RunningCount`;
such clusters will be ready immediately when claimed.

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
  * OpenStack (with [inventory](enhancements/clusterpool-inventory.md))

## Sample Cluster Pool

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterPool
metadata:
  name: openshift-46-aws-us-east-1
  namespace: my-project
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
  runningCount: 1
  size: 3
```

## Sample Cluster Claim

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterClaim
metadata:
  name: dgood46
  namespace: my-project
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

## Managing admins for Cluster Pools

Role bindings in the **namespace** of a `ClusterPool` that bind to the Cluster Role `hive-cluster-pool-admin`
are used to provide the **subjects** same permission in the namespaces created for various clusterprovisions for the cluster pool.
This allows operators to define adminstrators for a `ClusterPool` allowing them visibility to all the resources created for it. This is
most useful to debug `ClusterProvisions` associated with the pool that have failed and therefore cannot be claimed.

NOTE: You can only define such administrators for the entire namespace and not a specific `ClusterPool`.

To make any `User` or `Group` `hive-cluster-pool-admin` for a namespace you can,

```sh
oc -n <namespace> adm policy add-role-to-group hive-cluster-pool-admin <user>
```

or,

```sh
oc -n <namespace> adm policy add-role-to-group hive-cluster-pool-admin <group>
```

## Install Config Template

To control parts of the cluster deployments that are not directly supported by Hive, such as controlPlane Nodes and types, you can load a valid `install-config.yaml` which will be passed directly to the openshift-installer, only updating `metadata.name` and `baseDomain`

Load the install-config.yaml template as a secret (assuming that the `install-config.yaml` you want to use as a template is in the active directory)

```bash
kubectl  -n my-project create secret generic my-install-config-template --from-file=install-config.yaml=./install-config.yaml
```

With this secret created, you can create a pool that references the install config secret template.
The pool and secret must be in the same namespace.

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterPool
metadata:
  name: openshift-46-aws-us-east-1
  namespace: my-project
spec:
  baseDomain: hive.mytests.io
  imageSetRef:
    name: openshift-v4.5.13
  installConfigSecretTemplateRef: 
    name: my-install-config-template
  skipMachinePools: true
  platform:
    aws:
      credentialsSecretRef:
        name: global-aws-creds
      region: eu-west-1
  size: 1
```

**Note** When using ClusterPools, Hive will by default create a MachinePool for the worker nodes for any ClusterDeployments that are a child of a ClusterPool. When you use an installConfigSecretTemplate that deviates from the MachinePool defaults you will most likely want to disable MachinePools by setting spec.skipMachinePools on the ClusterPool, so that Hive does not reconcile away from the machine config specified in install-config.yaml

## Time-based scaling of Cluster Pool

You can use kubernetes cron jobs to scale clusterpools as per a defined schedule.

The following are the yaml configurations for setting up the permissions: Role, RoleBinding and ServiceAccount. It sets up a role with permissions to get a clusterpool and patch clusterpool’s scale subresource.

```yaml
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  namespace: my-project
  name: scale-clusterpool
rules:
- apiGroups:
  - hive.openshift.io
  resources:
  - clusterpools
  verbs:
  - get
- apiGroups:
  - hive.openshift.io
  resources:
  - clusterpools/scale
  verbs:
  - patch

---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: scale-clusterpool
  namespace: my-project
subjects:
- kind: ServiceAccount
  name: sa-scale-clusterpool
  namespace: my-project
roleRef:
  kind: Role
  name: scale-clusterpool
  apiGroup: rbac.authorization.k8s.io

---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: sa-scale-clusterpool
  namespace: my-project
```

Below is the sample configuration for the CronJob to scale up a clusterpool to size 10 at 6:00 AM everyday. It uses the serviceAccountName `sa-scale-clusterpool` created above.  
```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: scale-up-clusterpool
  namespace: my-project
spec:
  schedule: "0 6 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: sa-scale-clusterpool
          containers:
          - name: scale-clusterpool-size-10
            image: quay.io/openshift/origin-cli:latest
            command:
            - /bin/sh 
            - -c 
            - oc scale clusterpool openshift-46-aws-us-east-1 -n my-project --replicas=10
          restartPolicy: OnFailure
```

Below is the sample configuration for the CronJob to scale down a clusterpool to size 0 at 20:00 (8:00 PM) everyday. It uses the serviceAccountName `sa-scale-clusterpool` created above.  
```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: scale-down-clusterpool
  namespace: my-project
spec:
  schedule: "0 20 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: sa-scale-clusterpool
          containers:
          - name: scale-clusterpool-size-0
            image: quay.io/openshift/origin-cli:latest
            command:
            - /bin/sh 
            - -c 
            - oc scale clusterpool openshift-46-aws-us-east-1 -n my-project --replicas=0
          restartPolicy: OnFailure
```

CronJob’s spec.schedule field can be used to set the exact time when you want to scale the clusterpool. The syntax of the schedule expects a [cron](https://en.wikipedia.org/wiki/Cron) expression made of five fields - minute (0 - 59), hour (0 - 23), day of the month (1 - 31), month (1 - 12) and day of the week (0 - 6) in that order. In our example CronJob to scale up a clusterpool, the schedule is set to `0 6 * * *` which is 6:00 AM everyday. The cron job controller uses the time set for the kube-controller-manager container.

CronJob’s spec.containers[].image is the image with the `oc` binary. We have tested with the [quay.io/openshift/origin-cli](https://quay.io/repository/openshift/origin-cli) image. You can also create your own image.

## ClusterPool Deletion
A `ClusterPool` can be deleted in the usual way (`oc delete` or the API equivalent).
When a `ClusterPool` is deleted, hive will automatically initiate deletion of all *unclaimed* clusters in the pool.
No new clusters will be created, and any new `ClusterClaim`s will not be fulfilled.
However, *existing claimed* clusters will not be affected; and the `ClusterPool` itself will be held extant until those clusters have been deprovisioned through the normal means -- i.e. by deleting their `ClusterClaim`s.

## Troubleshooting
See [this doc](troubleshooting.md#clusterpools).