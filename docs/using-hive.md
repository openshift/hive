# Using Hive

## Cluster Provisioning

Cluster provisioning begins when a caller creates a `ClusterDeployment` CR, which is the core Hive resource used to control the lifecycle of a cluster and the Hive API entrypoint.

Hive comes with an optional `hiveutil` binary to assist creating the `ClusterDeployment` and its dependencies. See the [hiveutil documentation](hiveutil.md) for more information.

### DNS

OpenShift installation requires a live and functioning DNS zone in the cloud account into which you will be installing the new cluster(s). For example if you own example.com, you could create a hive.example.com subdomain in Route53, and ensure that you have made the appropriate NS entries under example.com to delegate to the Route53 zone. When creating a new cluster, the installer will make future DNS entries under hive.example.com as needed for the cluster(s).

In addition to the default OpenShift DNS support, Hive offers a DNS feature called Managed DNS. With Managed DNS, Hive can automatically create delegated zones for approved base domains. For example, if hive.example.com exists and is specified as your managed domain, you can specify a base domain of cluster1.hive.example.com on your `ClusterDeployment`, and Hive will create this zone for you, add forwarding records in the base domain, wait for it to resolve, and then proceed with installation.

### Pull Secret

OpenShift installation requires a pull secret obtained from try.openshift.com. You can specify an individual pull secret for each cluster Hive creates, or you can use a global pull secret that will be used by all of the clusters Hive creates.

```bash
oc create secret generic mycluster-pull-secret --from-file=.dockerconfigjson=/path/to/pull-secret --type=kubernetes.io/dockerconfigjson --namespace mynamespace
```

```yaml
apiVersion: v1
data:
  .dockerconfigjson: REDACTED
kind: Secret
metadata:
  name: mycluster-pull-secret
  namespace: mynamespace
type: kubernetes.io/dockerconfigjson
```

When a global pull secret is defined in the `hive` namespace and a `ClusterDeployment`-specific pull secret is specified, the registry authentication in both secrets will be merged and used by the new OpenShift cluster.
When a registry exists in both pull secrets, precedence will be given to the contents of the cluster-specific pull secret.

The global pull secret must live in the `hive` namespace and is referenced in the `HiveConfig`.

```bash
oc create secret generic global-pull-secret --from-file=.dockerconfigjson=/path/to/pull-secret --type=kubernetes.io/dockerconfigjson --namespace hive
```

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
  globalPullSecretRef:
    name: global-pull-secret
```

### OpenShift Version

Hive needs to know what version of OpenShift to install. A Hive cluster represents available versions via the `ClusterImageSet` resource, and there can be multiple `ClusterImageSets` available. Each `ClusterImageSet` references an OpenShift release image. A `ClusterDeployment` references a `ClusterImageSet` via the `spec.provisioning.imageSetRef` property.

Alternatively, you can specify an individual OpenShift release image in the `ClusterDeployment` `spec.provisioning.releaseImage` property.

An example `ClusterImageSet`:

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterImageSet
metadata:
  name: openshift-v4.3.0
spec:
  releaseImage: quay.io/openshift-release-dev/ocp-release:4.3.0-x86_64
```

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
  namespace: mynamespace
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
  namespace: mynamespace
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
  namespace: mynamespace
type: Opaque
```

### SSH Key Pair

(Optional) Hive uses the provided ssh key pair to ssh into the machines in the remote cluster. Hive connects via ssh to gather logs in the event of an installation failure. The ssh key pair is optional, but neither the user nor Hive will be able to ssh into the machines if it is not supplied.

Create a Kubernetes secret containing a ssh key pair in PEM format (typically generated with `ssh-keygen -m PEM`)

```yaml
apiVersion: v1
data:
  ssh-privatekey: REDACTED
  ssh-publickey: REDACTED
kind: Secret
metadata:
  name: mycluster-ssh-key
  namespace: mynamespace
type: Opaque
```

### InstallConfig

The OpenShift installer `InstallConfig` must be stored in a `secret` and referenced in the `ClusterDeployment`. This allows Hive to more easily support installing multiple versions of OpenShift.

Example `install-config.yaml` for AWS:

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
metadata:
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
pullSecret: REDACTED
sshKey: REDACTED
```

```bash
oc create secret generic mycluster-install-config --from-file=install-config.yaml=./install-config.yaml
```

For Azure, replace the contents of `compute.platform` and `controlPlane.platform` with:
```yaml
    azure:
      osDisk:
        diskSizeGB: 128
      type: Standard_D2s_v3
```

and replace the contents of `platform` with:

```yaml
  azure:
    baseDomainResourceGroupName: my-bdrgn
    region: centralus
```

For GCP, replace the contents of `compute.platform` and `controlPlane.platform` with:
```yaml
    gcp:
      type: n1-standard-4
```

and replace the contents of `platform` with:
```yaml
  gcp:
    projectID: myproject
    region: us-east1
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
  namespace: mynamespace
spec:
  baseDomain: hive.example.com
  clusterName: mycluster
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

For Azure, replace the contents of `spec.platform` with:

```yaml
azure:
  baseDomainResourceGroupName: my-bdrgn
  credentialsSecretRef:
    name: mycluster-azure-creds
  region: centralus
```

For GCP, replace the contents of `spec.platform` with:

```yaml
gcp:
  credentialsSecretRef:
    name: mycluster-gcp-creds
  region: us-east1
```

### Machine Pools

To manage `MachinePools` Day 2, you need to define these as well. The definition of the worker pool should mostly match what was specified in `InstallConfig` to prevent replacement of all worker nodes.

`InstallConfig` is limited to the one worker pool, but Hive can sync additional machine pools Day 2.

```yaml
apiVersion: hive.openshift.io/v1
kind: MachinePool
metadata:
  name: mycluster-worker
  namespace: mynamespace
spec:
  clusterDeploymentRef:
    name: mycluster
  name: worker
  platform:
    aws:
      rootVolume:
        iops: 100
        size: 22
        type: gp2
      type: m4.xlarge
  replicas: 3
```

For Azure, replace the contents of `spec.platform` with:

```yaml
azure:
  osDisk:
    diskSizeGB: 128
  type: Standard_D2s_v3
```

For GCP, replace the contents of `spec.platform` with:

```yaml
gcp:
  type: n1-standard-4
```

WARNING: Due to some naming restrictions on various components in GCP, Hive will restrict you to a max of 35 MachinePools (including the original worker pool created by default). We are left with only a single character to differentiate the machines and nodes from a pool, and 'm' is already reserved for the master hosts, leaving us with a-z (minus m) and 0-9 for a total of 35. Hive will automatically create a MachinePoolNameLease for GCP MachinePools to grab one of the available characters until none are left, at which point your MachinePool will not be provisioned.

#### Create Cluster on Bare Metal

Hive supports bare metal provisioning as provided by [openshift-install](https://github.com/openshift/installer/blob/master/docs/user/metal/install_ipi.md)

At present this feature requires a separate pre-existing libvirt provisioning host to run the bootstrap node. This host will require very specific network configuration that far exceeds the scope of Hive documentation. See [Bare Metal Platform Customization](https://github.com/openshift/installer/blob/master/docs/user/metal/customization_ipi.md#using-a-remote-hypervisor) for more information.

![Bare metal provisioning with one hypervisor host per cluster](hive-baremetal-hypervisor-per-cluster.png)

![Bare metal provisioning with a shared hypervisor host](hive-baremetal-shared-hypervisor.png)

To provision bare metal clusters with Hive:

Create a `Secret` containing a [bare metal enabled InstallConfig](https://github.com/openshift/installer/blob/master/docs/user/metal/install_ipi.md#install-config). This `InstallConfig` must contain a `libvirtURI` property pointing to the provisioning host.

Create a `Secret` containing the SSH private key that can connect to your libvirt provisioning host, without a passphrase.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: provisioning-host-ssh-private-key
  namespace: mynamespace
stringData:
  ssh-privatekey: |-
    -----BEGIN RSA PRIVATE KEY-----
   REDACTED
    -----END RSA PRIVATE KEY-----
type: Opaque
```

Create a `ConfigMap` for manifests to inject into the installer, containing a nested `ConfigMap` for metal3 config.

*NOTE*: This will no longer be required as of OpenShift 4.4+.

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: my-baremetal-cluster-install-manifests
  namespace: mynamespace
data:
  99_metal3-config.yaml: |
    kind: ConfigMap
    apiVersion: v1
    metadata:
      name: metal3-config
      namespace: openshift-machine-api
    data:
      http_port: "6180"
      provisioning_interface: "enp1s0"
      provisioning_ip: "172.22.0.3/24"
      dhcp_range: "172.22.0.10,172.22.0.100"
      deploy_kernel_url: "http://172.22.0.3:6180/images/ironic-python-agent.kernel"
      deploy_ramdisk_url: "http://172.22.0.3:6180/images/ironic-python-agent.initramfs"
      ironic_endpoint: "http://172.22.0.3:6385/v1/"
      ironic_inspector_endpoint: "http://172.22.0.3:5050/v1/"
      cache_url: "http://192.168.111.1/images"
      rhcos_image_url: "https://releases-art-rhcos.svc.ci.openshift.org/art/storage/releases/rhcos-4.3/43.81.201911192044.0/x86_64/rhcos-43.81.201911192044.0-openstack.x86_64.qcow2.gz"
```

Create a `ClusterDeployment`, note the `libvirtSSHPrivateKeySecretRef` and `sshKnownHosts` for bare metal:

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterDeployment
metadata:
  name: my-baremetal-cluster
  namespace: mynamespace
  annotations:
    hive.openshift.io/try-install-once: "true"
spec:
  baseDomain: test.example.com
  clusterName: my-baremetal-cluster
  controlPlaneConfig:
    servingCertificates: {}
  platform:
    baremetal:
      libvirtSSHPrivateKeySecretRef:
        name: provisioning-host-ssh-private-key
  provisioning:
    installConfigSecretRef:
      name: my-baremetal-cluster-install-config
    sshPrivateKeySecretRef:
      name: my-baremetal-hosts-ssh-private-key
    manifestsConfigMapRef:
      name: my-baremetal-cluster-install-manifests
    imageSetRef:
      name: my-clusterimageset
    sshKnownHosts:
    # SSH known host info for the libvirt provisioning server to avoid a prompt during non-interactive install:
    - "10.1.8.90 ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBKWjJRzeUVuZs4yxSy4eu45xiANFIIbwE3e1aPzGD58x/NX7Yf+S8eFKq4RrsfSaK2hVJyJjvVIhUsU9z2sBJP8="
  pullSecretRef:
    name: my-baremetal-cluster-pull-secret
```

There is not presently support for `MachinePool` management on bare metal clusters. The pools defined in your `InstallConfig` are authoritative.

There is not presently support for "deprovisioning" a bare metal cluster, as such deleting a bare metal `ClusterDeployment` has no impact on the running cluster, it is simply removed from Hive and the systems would remain running. This may change in the future.


## Monitor the Install Job

* Get the namespace in which your cluster deployment was created
* Get the install pod name
  ```bash
  oc get pods -l "hive.openshift.io/job-type=provision,hive.openshift.io/cluster-deployment-name=${CLUSTER_NAME}" -o jsonpath='{.items[0].metadata.name}'
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

Once the cluster is provisioned, the admin kubeconfig will be stored in a secret. You can use this with:

```bash
./hack/get-kubeconfig.sh ${CLUSTER_NAME} > ${CLUSTER_NAME}.kubeconfig
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
  oc extract secret/$(oc get cd ${CLUSTER_NAME} -o jsonpath='{.spec.clusterMetadata.adminPasswordSecretRef.name}') --to=-
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
         - aws:
             credentialsSecretRef:
               name: route53-aws-creds
           domains:
           - hive.example.com
       ```
     - GCP
       ```yaml
       apiVersion: hive.openshift.io/v1
       kind: HiveConfig
       metadata:
         name: hive
       spec:
         managedDomains:
         - gcp:
             credentialsSecretRef:
               name: gcp-creds
           domains:
           - hive.example.com

  1. Specify which domains Hive is allowed to manage by adding them to the `.spec.managedDomains[].domains` list. When specifying `manageDNS: true` in a ClusterDeployment, the ClusterDeployment's baseDomain must be a direct child of one of these domains, otherwise the ClusterDeployment creation will result in a validation error. The baseDomain must also be unique to that cluster and must not be used in any other ClusterDeployment, including on separate Hive instances.

     As such, a domain may exist in the `.spec.managedDomains[].domains` list in multiple Hive instances. Note that the specified credentials must be valid to add and remove NS record entries for all domains listed in `.spec.managedDomains[].domains`.

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
