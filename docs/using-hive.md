# Using Hive

## Cluster Provisioning

Cluster provisioning begins when a caller creates a `ClusterDeployment` CR, which is the core Hive resource used to control the lifecycle of a cluster and the Hive API entrypoint.

Hive comes with an optional `hiveutil` binary to assist creating the `ClusterDeployment` and its dependencies. See the [hiveutil documentation](hiveutil.md) for more information.

### DNS

OpenShift installation requires a live and functioning DNS zone in the cloud account into which you will be installing the new cluster(s). For example if you own example.com, you could create a hive.example.com subdomain in Route53, and ensure that you have made the appropriate NS entries under example.com to delegate to the Route53 zone. When creating a new cluster, the installer will make future DNS entries under hive.example.com as needed for the cluster(s).

In addition to the default OpenShift DNS support, Hive offers a DNS feature called Managed DNS. With Managed DNS, Hive can automatically create delegated zones for approved base domains. For example, if hive.example.com exists and is specified as your managed domain, you can specify a base domain of cluster1.hive.example.com on your `ClusterDeployment`, and Hive will create this zone for you, wait for it to resolve, and then proceed with installation.

### Pull Secret

OpenShift installation requires a pull secret obtained from try.openshift.com. You can specify an individual pull secret for each cluster Hive creates, or you can use a global pull secret that will be used by all of the clusters Hive creates.

```bash
oc create secret generic mycluster-pull-secret .dockerconfigjson=/path/to/pull-secret --type=kubernetes.io/dockerconfigjson --namespace hive
```

```yaml
apiVersion: v1
data:
  .dockerconfigjson: REDACTED
kind: Secret
metadata:
  name: mycluster-pull-secret
  namespace: hive
type: kubernetes.io/dockerconfigjson
```

When a global pull secret is defined in the `hive` namespace and a `ClusterDeployment`-specific pull secret is specified, the registry authentication in both secrets will be merged and used by the new OpenShift cluster.
When a registry exists in both pull secrets, precedence will be given to the contents of the cluster-specific pull secret.

The global pull secret must live in the `hive` namespace and is referenced in the `HiveConfig` (also in the `hive` namespace).

```yaml
apiVersion: v1
data:
  .dockerconfigjson: REDACTED
kind: Secret
metadata:
  name: global-pull-secret
  namespace: hive
type: kubernetes.io/dockerconfigjson

```

```bash
oc edit hiveconfig hive
```

```yaml
spec:
  globalPullSecret:
    name: global-pull-secret
```

### OpenShift Version

Hive needs to know what version of OpenShift to install. You can specify an individual OpenShift release image for each cluster Hive creates, or you can use a global `ClusterImageSet` that will be used by all of the clusters Hive creates.

An example `ClusterImageSet`:

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterImageSet
metadata:
  name: openshift-v4.3.0
spec:
  releaseImage: quay.io/openshift-release-dev/ocp-release:4.3.0
```
Alternatively you can specify an OpenShift 4 release image directly on your `ClusterDeployment`. This value will override any value set in your `ClusterImageSet`.

### Cloud credentials

Hive requires credentials to the cloud account into which it will install OpenShift clusters.

#### AWS

Create a `secret` containing your AWS access key and secret access key:

```yaml
apiVersion: v1
data:
  aws_access_key_id: REDACTED
  aws_secret_access_key: REDACTED
kind: Secret
metadata:
  name: mycluster-aws-creds
  namespace: hive
type: Opaque
```

#### Azure

Create a `secret` containing your Azure service principal:

```yaml
apiVersion: v1
data:
  osServicePrincipal.json: REDACTED
kind: Secret
metadata:
  name: mycluster-azure-creds
  namespace: hive
type: Opaque
```

#### GCP

Create a `secret` containing your GCP service account key:

```yaml
apiVersion: v1
data:
  osServiceAccount.json: REDACTED
kind: Secret
metadata:
  name: mycluster-gcp-creds
  namespace: hive
type: Opaque
```

### SSH Key Pair

(Optional) Hive uses the provided ssh key pair to ssh into the machines in the remote cluster. Hive connects via ssh to gather logs in the event of an installation failure. The ssh key pair is optional, but neither the user nor Hive will be able to ssh into the machines if it is not supplied.

Create a Kubernetes secret containing a ssh key pair (typically generated with `ssh-keygen`)

```yaml
apiVersion: v1
data:
  ssh-privatekey: REDACTED
  ssh-publickey: REDACTED
kind: Secret
metadata:
  name: mycluster-ssh-key
  namespace: hive
type: Opaque
```

### InstallConfig

The OpenShift installer `InstallConfig` must be base64 encoded into a `secret` and referenced in the `ClusterDeployment`.

```yaml
apiVersion: v1
baseDomain: hive.example.com
compute:
- name: worker
  platform:
    aws:
      rootVolume:
        iops: 100
        size: 22
        type: gp2
      type: m4.xlarge
    gcp:
      type: n1-standard-4
    azure:
      osDisk:
        diskSizeGB: 128
      type: Standard_D2s_v3
  replicas: 3
controlPlane:
  name: master
  platform:
    aws:
      rootVolume:
        iops: 100
        size: 22
        type: gp2
      type: m4.xlarge
    gcp:
      type: n1-standard-4
    azure:
      osDisk:
        diskSizeGB: 128
      type: Standard_D2s_v3
replicas: 3
metadata:
  creationTimestamp: null
  name: mycluster
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineCIDR: 10.0.0.0/16
  networkType: OpenShiftSDN
  serviceNetwork:
  - 172.30.0.0/16
platform:
  aws:
    region: us-east-1
  gcp:
    projectID: myproject
    region: us-east1
  azure:
    baseDomainResourceGroupName: my-bdrgn
    region: centralus
pullSecret: REDACTED
sshKey: REDACTED
```

```bash
base64 install-config.yaml
```

```yaml
apiVersion: v1
data:
  install-config.yaml: BASE64 ENCODED INSTALLCONFIG
kind: Secret
metadata:
  name: mycluster-install-config
  namespace: hive
type: Opaque
```

### ClusterDeployment

Cluster provisioning begins when a `ClusterDeployment` is created.

Note that some parts are duplicated with the `InstallConfig`.

An example `ClusterDeployment` for AWS:

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterDeployment
metadata:
  name: mycluster
spec:
  baseDomain: hive.example.com
  clusterName: mycluster
  controlPlaneConfig:
    servingCertificates: {}
  installed: false
  platform:
    aws:
      credentialsSecretRef:
        name: mycluster-aws-creds
      region: us-east-1
  provisioning:
    imageSetRef:
      name: openshift-v4.3.0
    installConfigSecretRef:
      name: mycluster-install-config
    sshPrivateKeySecretRef:
      name: mycluster-ssh-key
  pullSecretRef:
    name: mycluster-pull-secret
  compute:
  - name: worker
    replicas: 3
    platform:
      aws:
        rootVolume:
          iops: 100
          size: 22
          type: gp2
        type: m4.xlarge
  controlPlane:
    name: master
    replicas: 3
    platform:
      aws:
        rootVolume:
          iops: 100
          size: 22
          type: gp2
        type: m4.xlarge
```


An example `ClusterDeployment` for AWS:

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterDeployment
metadata:
  name: mycluster
spec:
  baseDomain: hive.example.com
  clusterName: mycluster
  controlPlaneConfig:
    servingCertificates: {}
  installed: false
  platform:
    aws:
      credentialsSecretRef:
        name: mycluster-aws-creds
      region: us-east-1
  provisioning:
    imageSetRef:
      name: openshift-v4.3.0
    installConfigSecretRef:
      name: mycluster-install-config
    sshPrivateKeySecretRef:
      name: mycluster-ssh-key
  pullSecretRef:
    name: mycluster-pull-secret
```

For Azure, replace `spec.platform` with:

```yaml
azure:
  baseDomainResourceGroupName: my-bdrgn
  credentialsSecretRef:
    name: mycluster-azure-creds
  region: centralus
```

For GCP, replace `spec.platform` with:

```yaml
gcp:
  credentialsSecretRef:
    name: mycluster-gcp-creds
  region: us-east1
```

### MachineConfig

To manage `MachinePools` Day 2, you need to define these as well. The definition must match what was specified in the `InstallConfig`.

```yaml
apiVersion: hive.openshift.io/v1
kind: MachinePool
metadata:
  creationTimestamp: null
  name: baremetal-demo-worker
spec:
  clusterDeploymentRef:
    name: baremetal-demo
  name: worker
  platform:
    azure:
      osDisk:
        diskSizeGB: 128
      type: Standard_D2s_v3
    gcp:
      type: n1-standard-4
    aws:
      rootVolume:
        iops: 100
        size: 22
        type: gp2
      type: m4.xlarge
  replicas: 0
```

## Monitor the Install Job

* Get the namespace in which your cluster deployment was created
* Get the install pod name
  ```bash
  oc get pods -o json --selector job-name==${CLUSTER_NAME}-install | jq -r '.items | .[].metadata.name'
  ```
* Run following command to watch the cluster deployment
  ```bash
  oc logs -f <install-pod-name> -c hive
  ```
  Alternatively, you can watch the summarized output of the installer using
  ```bash
  oc exec -c hive <install-pod-name> -- tail -f /tmp/openshift-install-console.log
  ```

In the event of installation failures, please see [Troubleshooting](./troubleshooting.md).

### Cluster Admin Kubeconfig

Once the cluster is provisioned you will see a CLUSTER_NAME-admin-kubeconfig secret. You can use this with:

```bash
oc get secret `oc get cd ${CLUSTER_NAME} -o jsonpath='{ .status.adminKubeconfigSecret.name }'` -o jsonpath='{ .data.kubeconfig }' | base64 --decode > ${CLUSTER_NAME}.kubeconfig
export KUBECONFIG=${CLUSTER_NAME}.kubeconfig
oc get nodes
```

### Access the Web Console

* Get the webconsole URL
  ```
  oc get cd ${CLUSTER_NAME} -o jsonpath='{ .status.webConsoleURL }'
  ```

* Retrieve the password for `kubeadmin` user
  ```
  oc extract secret/$(oc get cd -o jsonpath='{.items[].spec.clusterMetadata.adminPasswordSecretRef.name}') --to=-
  ```

## Managed DNS

Hive can optionally create delegated DNS zones for each cluster.

NOTE: This feature is only currently available for AWS and GCP clusters.

To use this feature:

  1. Manually create a DNS zone for your "root" domain (i.e. hive.example.com in the example below) and ensure your DNS is operational.
  1. Create a secret in the "hive" namespace with your cloud credentials with permissions to manage the root zone.
     - AWS
       ```yaml
       apiVersion: v1
       data:
         aws_access_key_id: REDACTED
         aws_secret_access_key: REDACTED
       kind: Secret
       metadata:
         name: route53-aws-creds
       type: Opaque
       ```
     - GCP
       ```yaml
       apiVersion: v1
       data:
         osServiceAccount.json: REDACTED
       kind: Secret
       metadata:
         name: gcp-creds
       type: Opaque
       ```
  1. Update your HiveConfig to enable externalDNS and set the list of managed domains:
     - AWS
       ```yaml
       apiVersion: hive.openshift.io/v1
       kind: HiveConfig
       metadata:
         name: hive
       spec:
         managedDomains:
         - hive.example.com
         externalDNS:
           aws:
             credentialsSecretRef:
               name: route53-aws-creds
       ```
     - GCP
       ```yaml
       apiVersion: hive.openshift.io/v1
       kind: HiveConfig
       metadata:
         name: hive
       spec:
         managedDomains:
         - hive.example.com
         externalDNS:
           gcp:
             credentialsSecretRef:
               name: gcp-creds

  1. Specify which domains Hive is allowed to manage by adding them to the `managedDomains` list. When specifying `managedDNS: true` in a ClusterDeployment, the ClusterDeployment's baseDomain must be a direct child of one of these domains, otherwise the ClusterDeployment creation will result in a validation error. The baseDomain must also be unique to that cluster and must not be used in any other ClusterDeployment, including on separate Hive instances.

     As such, a domain may exist in the `managedDomains` list in multiple Hive instances. Note that the specified credentials must be valid to add and remove NS record entries for all domains listed in `managedDomains`.

You can now create clusters with manageDNS enabled and a basedomain of mydomain.hive.example.com.

```
bin/hiveutil create-cluster --base-domain=mydomain.hive.example.com mycluster --manage-dns
```

Hive will then:

  1. Create a mydomain.hive.example.com DNS zone.
  1. Create NS records in the hive.example.com to forward DNS to the new mydomain.hive.example.com DNS zone.
  1. Wait for the SOA record for the new domain to be resolvable, indicating that DNS is functioning.
  1. Launch the install, which will create DNS entries for the new cluster ("\*.apps.mycluster.mydomain.hive.example.com", "api.mycluster.mydomain.hive.example.com", etc) in the new mydomain.hive.example.com DNS zone.


## Configuration Management

### SyncSet

Hive offers two CRDs for applying configuration in a cluster once it is installed: `SyncSet` for config destined for specific clusters in a specific namespace, and `SelectorSyncSet` for config destined for any cluster matching a label selector.

For more information please see the [SyncSet](syncset.md) documentation.

### Identity Provider Management

Hive offers explicit API support for configuring identity providers in the OpenShift clusters it provisions. This is technically powered by the above `SyncSet` mechanism, but is provided directly in the API to support configuring per cluster identity providers, merged with global identity providers, all of which must land in the same object in the cluster.

For more information please see the [SyncIdentityProvider](syncidentityprovider.md) documentation.

## Cluster Deprovisioning

```bash
oc delete clusterdeployment ${CLUSTER_NAME} --wait=false
```

Deleting a `ClusterDeployment` will create a `ClusterDeprovision` resource, which in turn will launch a pod to attempt to delete all cloud resources created for and by the cluster. This is done by scanning the cloud provider for resources tagged with the cluster's generated `InfraID`. (i.e. `kubernetes.io/cluster/mycluster-fcp4z=owned`) Once all resources have been deleted the pod will terminate, finalizers will be removed, and the `ClusterDeployment` and dependent objects will be removed. The deprovision process is powered by vendoring the same code from the OpenShift installer used for `openshift-install cluster destroy`.

The `ClusterDeprovision` resource can also be used to manually run a deprovision pod for clusters which no longer have a `ClusterDeployment`. (i.e. `clusterDeployment.spec.preserveOnDelete=true`)
