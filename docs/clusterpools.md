# Cluster Pools

- [Overview](#overview)
- [Supported Cloud Platforms](#supported-cloud-platforms)
- [Sample Cluster Pool](#sample-cluster-pool)
- [Sample Cluster Claim](#sample-cluster-claim)
- [Managing admins for Cluster Pools](#managing-admins-for-cluster-pools)
- [Install Config Template](#install-config-template)
- [Updating Cluster Pools](#updating-cluster-pools)
  - [Rotating Cloud Credentials](#rotating-cloud-credentials)
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
  * vSphere (with [inventory](enhancements/clusterpool-inventory.md))

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

## Updating Cluster Pools
The ClusterPool CR can be edited like any kubernetes resource. In such cases, hive will
automatically replace unclaimed pool clusters one at a time, oldest first. This slow refresh is
intended to maximize availability, allowing existing workflows to continue claiming clusters
even if they were created with the previous configuration. To effect more aggressive replacement,
you can scale the pool to `size: 0` and back to the desired size.

**NOTES:**
- Hive will only detect changes to the ClusterPool manifest itself. Changes to artifacts referenced
  therefrom, such as the install-config.yaml template or ImageSet, are not detected. Manual pool
  refresh via down- and up-scaling may be desired in such cases.
- Upgrading hive may trigger the slow refresh described above. This is a known artifact of the
  update detection mechanism, caused by changes in the ClusterPool CRD schema, even if no explicit
  changes are made.

### Rotating Cloud Credentials
When a ClusterPool creates a ClusterDeployment, it does so in a fresh namespace, along with the
other artifacts necessary for provisioning and management.
This includes a copy of the cloud credentials Secret (referenced by e.g.
`ClusterPool.Spec.Platform.AWS.CredentialsSecretRef.Name` for AWS).
When the ClusterPool's credential Secret is updated or replaced, existing CDs' Secrets are not
affected.
This must be accounted for when rotating cloud credentials.
There are several options available:

1. **Scale Down; Rotate; Scale Up**
   1. Scale your ClusterPool to `size: 0`.
      Existing clusters are deprovisioned (using the old credentials).
   1. Wait for all claimed clusters to be released and deprovisioned.
      These clusters are still relying on the old credentials.
   1. Rotate the credentials via your cloud provider.
   1. Edit your ClusterPool's creds Secret, replacing the old credentials with the new.
   1. Scale your ClusterPool back to its original `size`.
      New clusters are provisioned with the new credentials.

   Pros:
   - No special RBAC needed.
     If you were tasked with rotating credentials, you probably have the RBAC to edit the credentials Secret in the ClusterPool's namespace.
   - Old and new credentials never coexist in the cluster.

   Cons:
   - Disruptive.
     The pool is devoid of ready clusters while the new batch is provisioning.
   - Requires monitoring.
     You need a way to know when all the claimed clusters with the old Secret are gone.

1. **Replace ClusterPool's Secret and Wait**

   As described [above](#updating-cluster-pools), hive will automatically replace pool clusters
   when certain configuration changes are made to the ClusterPool manifest.
   This includes changing the name of the credentials Secret -- e.g. editing
   `ClusterPool.Spec.Platform.AWS.CredentialsSecretRef.Name` for AWS.
   1. Create a new credential in the cloud provider.
      (Do not disable/delete the old one yet!)
   1. Create a new credentials Secret in your ClusterPool's namespace.
   1. Edit your ClusterPool, modifying the credentials secret reference to point to the new Secret.
      This will cause hive to start deleting existing clusters and replacing them with new ones.
      **NOTE:** As previously mentioned, clusters are replaced one at a time.
      It can thus be quite a while before all of the old clusters are gone.
      You can speed up this process by scaling down and up.
   1. Wait for all claimed clusters to be released and deprovisioned.
      These clusters are still relying on the old credentials.
   1. Once all old clusters have been deleted, delete the old credentials Secret.
   1. Disable/delete the old credential in the cloud provider.

   Pros:
   - No special RBAC needed.
     You must be able to manage Secrets in the ClusterPool namespace as well as the ClusterPool
     itself.
   - High availability.
     The pool remains populated during the replacement process.
     Old clusters can continue to satisfy incoming ClusterClaims.

   Cons:
   - Slow.
   - Requires monitoring.
     You need a way to know when all claimed *and* unclaimed clusters with the old Secret are gone.

1. **Patch All Existing Secrets**
   1. Create a new credential in the cloud provider.
      (Do not disable/delete the old one yet!)
   1. Edit your ClusterPool's creds Secret, replacing the old credentials with the new.
      (Doing this first ensures any new clusters get the new creds, avoiding race conditions.)
   1. Patch the credentials Secrets for all existing clusters belonging to this pool, both claimed
      and unclaimed.
      You may wish to use [refresh-clusterpool-creds.sh](../hack/refresh-clusterpool-creds.sh) for
      this step.
   1. Disable/delete the old credential in the cloud provider.

   Pros:
   - High availability.
     No clusters are deprovisioned as a result of this process.
   - Immediate.
     No waiting for clusters to be replaced or for claimed clusters to be released.
     Can be done synchronously.

   Cons:
   - Requires special RBAC.
     The ClusterPool admin will usually not have permission to modify Secrets in arbitrary
     namespaces.

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