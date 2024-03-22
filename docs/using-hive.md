<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Cluster Provisioning](#cluster-provisioning)
  - [DNS](#dns)
    - [Native](#native)
      - [Managed DNS](#managed-dns)
    - [Non-native](#non-native)
  - [Pull Secret](#pull-secret)
  - [OpenShift Version](#openshift-version)
  - [Cloud credentials](#cloud-credentials)
    - [AWS](#aws)
    - [Azure](#azure)
    - [GCP](#gcp)
    - [IBM Cloud](#ibm-cloud)
      - [IBM Cloud Credential Manifests](#ibm-cloud-credential-manifests)
    - [vSphere](#vsphere)
    - [OpenStack](#openstack)
  - [SSH Key Pair](#ssh-key-pair)
  - [InstallConfig](#installconfig)
  - [ClusterDeployment](#clusterdeployment)
  - [Machine Pools](#machine-pools)
    - [Configuring Availability Zones](#configuring-availability-zones)
    - [Auto-scaling](#auto-scaling)
      - [Integration with Horizontal Pod Autoscalers](#integration-with-horizontal-pod-autoscalers)
  - [Create Cluster on Bare Metal](#create-cluster-on-bare-metal)
- [Monitor the Install Job](#monitor-the-install-job)
  - [Saving Logs for Failed Provisions](#saving-logs-for-failed-provisions)
  - [Cluster Admin Kubeconfig](#cluster-admin-kubeconfig)
  - [Access the Web Console](#access-the-web-console)
- [Managed DNS](#managed-dns-1)
- [Cluster Adoption](#cluster-adoption)
  - [Example Adoption ClusterDeployment](#example-adoption-clusterdeployment)
  - [Adopting with hiveutil](#adopting-with-hiveutil)
  - [Transferring ownership](#transferring-ownership)
- [Configuration Management](#configuration-management)
  - [Vertical Scaling](#vertical-scaling)
  - [SyncSet](#syncset)
  - [Scaling ClusterSync](#scaling-clustersync)
  - [Identity Provider Management](#identity-provider-management)
- [Cluster Deprovisioning](#cluster-deprovisioning)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Using Hive

## Cluster Provisioning

Cluster provisioning begins when a caller creates a `ClusterDeployment` CR, which is the core Hive resource used to control the lifecycle of a cluster and the Hive API entrypoint.

Hive comes with an optional `hiveutil` binary to assist creating the `ClusterDeployment` and its dependencies. See the [hiveutil documentation](hiveutil.md) for more information.

### DNS

#### Native

For clouds where there is support for automated IP allocation and DNS configuration, (AWS, Azure, IBM Cloud and GCP) an OpenShift installation requires a live and functioning DNS zone in the cloud account into which you will be installing the new cluster(s). For example if you own example.com, you could create a hive.example.com subdomain in Route53, and ensure that you have made the appropriate NS entries under example.com to delegate to the Route53 zone. When creating a new cluster, the installer will make future DNS entries under hive.example.com as needed for the cluster(s).

##### Managed DNS

In addition to the default OpenShift DNS support, Hive offers a DNS feature called Managed DNS. With Managed DNS, Hive can automatically create delegated zones for approved base domains. For example, if hive.example.com exists and is specified as your managed domain, you can specify a base domain of cluster1.hive.example.com on your `ClusterDeployment`, and Hive will create this zone for you, add forwarding records in the base domain, wait for it to resolve, and then proceed with installation. Read [here](#managed-dns-1) for more details.

#### Non-native

For other platforms/clouds (OpenStack and VSphere), there is presently no native DNS auto-configuration available. This requires some up-front DNS configuration before a cluster can be installed.  It will typically be necessary to reserve virtual IPs (VIPs) that will be used for the cluster's management (eg `api.mycluster.hive.example.com`) and for the cluster's default ingress routes (eg `\*.apps.mycluster.hive.example.com`). Each platform/cloud's configuration will have its own system for alocating or reserving these IPs. Once the IPs are reserved, DNS entries must be published as A records (or simply making local host entries to manage the DNS-to-IP translations on the host(s) running Hive) so that the cluster's API endpoint will be accessible to Hive.

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
oc patch hiveconfig hive --type=merge --patch '{"spec": {"globalPullSecretRef": {"name": "global-pull-secret"}}}'
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

Hive requires credentials to the cloud account into which it will install OpenShift clusters. Refer to the [installer](https://github.com/openshift/installer) documentation for required level of permissions for each cloud.

#### AWS

Create a `secret` containing your AWS access key and secret access key:

```bash
oc create secret generic <mycluster>-aws-creds -n hive --from-literal=aws_access_key_id=<AWS_ACCESS_KEY_ID> --from-literal=aws_secret_access_key=<AWS_SECRET_ACCESS_KEY>
```
Take care when using the yaml below, you need to use base64 to encode the data values.
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

#### IBM Cloud

Create a `secret` containing your IBM Cloud API key:

```yaml
apiVersion: v1
stringData:
  ibmcloud_api_key: IBMCLOUDAPIKEY
kind: Secret
metadata:
  name: mycluster-ibm-creds
  namespace: mynamespace
type: Opaque
```

##### IBM Cloud Credential Manifests

IBM Cloud credential secrets must be provided as manifests for installation. Follow [instructions for using ccoctl to generate IBM Cloud service IDs](https://github.com/openshift/cloud-credential-operator/blob/master/docs/ccoctl.md#ibmcloud) and place manifests generated from running `ccoctl ibmcloud create-service-id` within a secret that will be referenced by the `ClusterDeployment`.

Create a manifests secret containing secrets generated by `ccoctl`:

```bash
oc create secret generic mycluster-manifests -n mynamespace --from-file=<manifests directory>
```

#### vSphere
Create a `secret` containing your vSphere credentials information:

```yaml
apiVersion: v1
stringData:
  password: vsphereuser
  username: secretpassword
kind: Secret
metadata:
  name: mycluster-vsphere-creds
  namespace: mynamespace
type: Opaque
```

Create a `secret` containing your vSphere CA certificate.

1. From the vCenter home page, download the vCenter’s root CA certificates. Click **Download trusted root CA certificates** in the vSphere Web Services SDK section. Download, wget or curl the <vCenter>/certs/download.zip file.
```
wget https://<vCenter>/certs/download.zip
```
2. Extract the compressed file that contains the vCenter root CA certificates. The contents of the compressed file resemble the following file structure:
```
certs
├── lin
│   ├── 108f4d17.0
│   ├── 108f4d17.r1
│   ├── 7e757f6a.0
│   ├── 8e4f8471.0
│   └── 8e4f8471.r0
├── mac
│   ├── 108f4d17.0
│   ├── 108f4d17.r1
│   ├── 7e757f6a.0
│   ├── 8e4f8471.0
│   └── 8e4f8471.r0
└── win
    ├── 108f4d17.0.crt
    ├── 108f4d17.r1.crl
    ├── 7e757f6a.0.crt
    ├── 8e4f8471.0.crt
    └── 8e4f8471.r0.crl

3 directories, 15 files
```
3. Create a single file by concatenating all the files in certs/lin. Save the file somewhere permanent -- you'll need it for each vSphere cluster you want to create.
```
cat certs/lin/* > /home/me/vsphere/ca.cert
```

Create a secret containing the combined CA bundle data within a `.cacert` key:

```
oc create secret generic mycluster-vsphere-certs --from-file=.cacert=/home/me/vsphere/ca.cert
```

```yaml
apiVersion: v1
stringData:
  .cacert: |
    -----BEGIN CERTIFICATE-----
    CA BUNDLE DATA HERE
    -----END CERTIFICATE-----
kind: Secret
metadata:
  name: mycluster-vsphere-certs
  namespace: mynamespace
type: Opaque
```

#### OpenStack

Create a `secret` containing your OpenStack clouds.yaml file:

```yaml
apiVersion: v1
data:
  clouds.yaml: REDACTED
kind: Secret
metadata:
  name: mycluster-openstack-creds
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
        size: 120
        type: gp3
      type: m5.xlarge
  replicas: 3
controlPlane:
  name: master
  platform:
    aws:
      rootVolume:
        iops: 100
        size: 120
        type: gp3
      type: m5.xlarge
metadata:
  name: mycluster
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineNetwork: 10.0.0.0/16
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
    cloudName: AzurePublicCloud
    baseDomainResourceGroupName: my-bdrgn
    region: centralus
```
Note: `cloudName` specifies the Azure Cloud in which to create the cluster e.g. `AzurePublicCloud` or `AzureUSGovernmentCloud`.

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

For IBM Cloud, replace the contents of `compute.platform` and `controlPlane.platform`. Note that `type` is any valid IBM Cloud instance type. `type` may be omitted to use OpenShift installation defaults.

```yaml
  ibmcloud:
    type: bx2-4x16
```

and populate the top-level `platform` fields with the appropriate information:

```yaml
platform:
  ibmcloud:
    region: us-east
```

and ensure that the top-level `credentialsMode` field has been set to `Manual`.

```yaml
credentialsMode: Manual
```

For vSphere, ensure the `compute` and `controlPlane` fields are empty.
```yaml
controlPlane:
compute:
```

and populate the top-level `platform` fields with the appropriate information:
```yaml
platform:
  vsphere:
    apiVIP: 192.168.1.10
    cluster: devel
    datacenter: dc1
    defaultDatastore: ds1
    folder: /dc1/vm/CLUSTER_NAME
    ingressVIP: 192.168.1.11
    network: "VM Network"
    password: secretpassword
    username: vsphereuser
    vCenter: vcenter.example.com
```

For Openstack, replace the contents of `compute.platform` with:
```yaml
  openstack:
    type: m1.large
```
Note: Use an instance type that meets the minimum requirement for the version of OpenShift being installed.

and replace the contents of `controlPlane.platform` with:
```yaml
  openstack:
    type: ci.m4.xlarge
```
Note: Use an instance type that meets the minimum requirement for the version of OpenShift being installed.

and replace the contents of `platform` with:
```yaml
  openstack:
    cloud: mycloud
    computeFlavor: m1.large
    externalNetwork: openstack_network_name
    lbFloatingIP: 10.0.111.158
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
  cloudName: AzurePublicCloud
  region: centralus
```
Note: `cloudName` specifies the Azure Cloud in which to create the cluster e.g. `AzurePublicCloud` or `AzureUSGovernmentCloud`.


For GCP, replace the contents of `spec.platform` with:

```yaml
gcp:
  credentialsSecretRef:
    name: mycluster-gcp-creds
  region: us-east1
```

For IBM Cloud, replace the contents of `spec.platform` with:

```yaml
ibmcloud:
  credentialsSecretRef:
    name: mycluster-ibm-creds
  region: us-east
```

and add a manifests secret reference to `spec.provisioning`:

```yaml
provisioning:
  manifestsSecretRef:
    name: mycluster-manifests
```

For vSphere, replace the contents of `spec.platform` with:
```yaml
vsphere:
  certificatesSecretRef:
    name: mycluster-vsphere-certs
  cluster: devel
  credentialsSecretRef:
    name: mycluster-vsphere-creds
  datacenter: dc1
  defaultDatastore: ds1
  folder: /dc1/vm/CLUSTER_NAME
  network: "VM Network"
  vCenter: vsphere.example.com
```

For OpenStack, replace the contents of `spec.platform` with:
```yaml
openstack:
  cloud: mycloud
  credentialsSecretRef:
    name: mycluster-openstack-creds
```

### Machine Pools

`MachinePool` is a YAML configuration by which you can create and scale worker nodes on a deployed cluster. A `MachinePool` will create `MachineSet` resources on the deployed cluster. If supported on your cloud, those MachineSets will automatically span all AZs, or you can specify an explicit list.

A MachinePool for the worker machinesets is not required. If the user creates a MachinePool for the worker MachineSets, then Hive will manage the worker MachineSets.

MachinePool reconciliation is limited to updating MachineSet replicas to match the replicas configured for the MachinePool. Additionally, any existing `Labels` or `Taints` on the MachineSets will be overridden if they clash with those on the MachinePool. In case of duplicate taints, the taint encountered first will be preserved and the rest collapsed on the MachineSets.

MachinePool platform is immutable and any changes made to `MachinePool.spec.platform` are blocked by a validating webhook. The Machine Config Operator does not support updating existing machines when platform details are changed in a MachineSet and consequently Hive does not support making such changes to MachinePool platform, see [HIVE-2024](https://issues.redhat.com/browse/HIVE-2024).

The recommended workaround when platform details must be changed is to replace the MachinePool by creating an adjacent MachinePool with the desired configuration.
* Create replacement MachinePool with desired configuration and `MachinePool.spec.replicas = 0`.
* Scale down the old MachinePool while scaling up the replacement MachinePool.

`InstallConfig` is limited to the one worker pool, but Hive can sync additional `MachinePools` Day 2.

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
        size: 120
        type: gp3
      type: m5.xlarge
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

For IBM Cloud, replace the contents of `spec.platform`. Note that `type` is any valid IBM Cloud instance type. `type` may be omitted to use OpenShift installation defaults.

```yaml
ibmcloud:
  type: bx2-4x16
```

For vSphere, replace the contents of `spec.platform` with the settings you want for the instances:
```yaml
vsphere:
  coresPerSocket: 1
  cpus: 2
  memoryMB: 8192
  osDisk:
    diskSizeGB: 120
```

For OpenStack, replace the contents of `spec.platform` with the settings you want for the instances:
```yaml
openstack:
  rootVolume:
    size: 10
    type: ceph
  flavor: m1.large
```

#### Configuring Availability Zones

The desired Availability Zones (AZ) to create new worker nodes in can be specified in the `MachinePool` YAML (`spec.platform.<provider>.zones`), for example:

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
        size: 120
        type: gp3
      type: m5.xlarge
      zones:
        - us-east-1a
        - us-east-1b
  replicas: 3
```

If the Availability Zones are not configured in the `MachinePool`, then all of the AZs in the region will be used and a `MachineSet` resource will be created for each AZ (only relevant for public cloud providers).

#### Auto-scaling

`MachinePools` can be configured to auto-scale the number of worker nodes as needed based on resource utilization of the deployed cluster (this feature creates a `ClusterAutoscaler` resource in the deployed cluster).

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
        size: 120
        type: gp3
      type: m5.xlarge
  autoscaling:
    minReplicas: 3
    maxReplicas: 6
```

The number of minimum replicas must be equivalent to the number of configured Availability Zones.

The `spec.replicas` and `spec.autoscaling` configurations cannot be configured simultaneously.

The `spec.autoscaling.maxReplicas` is an optional field. If it is not configured, then nodes will be auto-scaled without restriction based on resource utilization needs.

##### Integration with Horizontal Pod Autoscalers

A `MachinePool` configured to auto-scaling mode creates a `ClusterAutoscaler` on the deployed cluster. `ClusterAutoscalers` can co-exist and work with Horiztonal Pod Autoscalers to ensure that there are enough available nodes to meet the auto-scaled pod replica count requirements. See excerpt from OpenShift [documentation](https://docs.openshift.com/container-platform/4.8/machine_management/applying-autoscaling.html):

> The horizontal pod autoscaler (HPA) and the cluster autoscaler modify cluster resources in different ways. The HPA changes the deployment’s or replica set’s number of replicas based on the current CPU load. If the load increases, the HPA creates new replicas, regardless of the amount of resources available to the cluster. If there are not enough resources, the cluster autoscaler adds resources so that the HPA-created pods can run. If the load decreases, the HPA stops some replicas. If this action causes some nodes to be underutilized or completely empty, the cluster autoscaler deletes the unnecessary nodes.

### Create Cluster on Bare Metal

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
    manifestsSecretRef:
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

### Saving Logs for Failed Provisions

Hive can be configured as follows to upload logs to an AWS S3 bucket when provisioning fails.

1. **Create an S3 bucket.** The bucket must be accessible from the environment from which your
   cluster will be provisioned, using credentials you will specify (below).
   Take note of the name of the bucket and the region in which you created it.
2. **Create a credentials secret.** This secret will need to exist in the target namespace of your hive deployment (HiveConfig.spec.targetNamespace, default `hive`), and contain AWS credentials sufficient to write to your bucket.
   The secret should data contain base64-encoded values for "aws_access_key_id" and "aws_secret_access_key".
   (You may wish to reuse the secret from your cluster deployment.)
3. **Create an SSH private key secret.** The secret data must contain a key called `ssh-privatekey`
   whose value is the base64-encoded contents of the private key file corresponding to the public
   key in your install config.
   Create this secret in the namespace of your ClusterDeployment.
4. **Tell HiveConfig where to find your bucket.** Under `.spec.failedProvisionConfig.aws`, add the
   bucket name, the reference to the AWS credentials secret, and the region. For example:
   ```yaml
   spec:
     failedProvisionConfig:
       aws:
         bucket: failed-provision-logs
         credentialsSecretRef:
           name: test-retry-aws-creds
         region: us-east-1
   ```
5. **Ensure your ClusterDeployment is configured with your SSH private key secret.** Reference the
   SSH private key secret in your ClusterDeployment's `.spec.provisioning.sshPrivateKeySecretRef.
   For example:
   ```yaml
   spec:
     provisioning:
       sshPrivateKeySecretRef:
         name: mycluster-ssh-key
   ```
   (If using [hiveutil](hiveutil.md), you can provide the key pair from your file system via `--ssh-private-key-file` and `--ssh-public-key-file`.)

The [troubleshooting doc](troubleshooting.md#cluster-install-failure-logs) provides more information about extracting and processing the logs.

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

NOTE: This feature only works for provisioning to AWS, GCP, and Azure.

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
       The following AWS IAM permissions should be associated with these credentials:
       ```
       route53:ChangeResourceRecordSets
       route53:ChangeTagsForResource
       route53:CreateHostedZone
       route53:DeleteHostedZone
       route53:GetHostedZone
       route53:ListHostedZonesByName
       route53:ListResourceRecordSets
       route53:ListTagsForResource
       tag:GetResources
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
     - Azure
       Service principal needs DNS Zone Contributor role on DNS zone resource.
       ```yaml
       apiVersion: v1
       data:
         osServicePrincipal.json: REDACTED
       kind: Secret
       metadata:
         name: azure-creds
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
       ```
     - Azure
       ```yaml
       apiVersion: hive.openshift.io/v1
       kind: HiveConfig
       metadata:
         name: hive
       spec:
         managedDomains:
         - azure:
             credentialsSecretRef:
               name: azure-creds
           domains:
           - hive.example.com
       ```
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

## Cluster Adoption

It is possible to adopt cluster deployments into Hive.
This will allow you to manage the cluster as if it had been provisioned by Hive, including:
- [MachinePools](#machine-pools)
- [SyncSets and SelectorSyncSets](syncset.md)
- [Deprovisioning](#cluster-deprovisioning)

To do so you will need to create a ClusterDeployment with Spec.Installed set to True, no Spec.Provisioning section, and include the following:

  * cluster INFRAID (obtained from `oc get infrastructure cluster -o json | jq .status.infrastructureName`)
  * cluster ID (obtained from `oc get clusterversion version -o json | jq .spec.clusterID`)
  * reference to a properly formatted admin kubeconfig Secret:  `oc create secret generic mycluster-admin-kubeconfig --from-file=kubeconfig=/tmp/admin.kubeconfig`
  * Spec.Platform.YourCloudProvider for your cluster, most importantly region and a properly formatted credentials Secret

Use `Spec.PreserveOnDelete = true` if you do not want Hive to deprovision resources when the ClusterDeployment is deleted.

### Example Adoption ClusterDeployment

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterDeployment
metadata:
  name: my-gcp-cluster
  namespace: hive
spec:
  baseDomain: gcp.example.com
  clusterMetadata:
    adminKubeconfigSecretRef:
      name: my-gcp-cluster-admin-kubeconfig
    clusterID: 61010205-c91d-44c9-8394-3e1790bd76f3
    infraID: my-gcp-cluster-wsvdn
  clusterName: my-gcp-cluster
  installed: true
  platform:
    gcp:
      credentialsSecretRef:
        name: my-gcp-creds
      region: us-east1
  pullSecretRef:
    name: pull-secret
```

If the cluster you are looking to adopt is on AWS and leverages Privatelink, you'll also need to include that setting under `spec.platform.aws` to ensure the VPC Endpoint Service for the cluster is tracked in the ClusterDeployment.

```yaml
  platform:
    aws:
      credentialsSecretRef:
        name: my-aws-cluster-creds
      privateLink:
        enabled: true
      region: us-east-1
```

If the cluster you are looking to adopt is on AWS and uses a shared VPC, you will also need to include the name of the hosted zone role in `spec.clusterMetadata.platform.aws.hostedZoneRole`.

```yaml
  clusterMetadata:
    adminKubeconfigSecretRef:
      name: my-gcp-cluster-admin-kubeconfig
    clusterID: 61010205-c91d-44c9-8394-3e1790bd76f3
    infraID: my-gcp-cluster-wsvdn
    platform:
      aws:
        hostedZoneRole: account-b-zone-role
```

If the cluster you are looking to adopt is on GCP and uses a shared VPC, you will also need to include the name of the network project ID in `spec.clusterMetadata.platform.gcp.networkProjectID`.

```yaml
  clusterMetadata:
    adminKubeconfigSecretRef:
      name: my-gcp-cluster-admin-kubeconfig
    clusterID: 61010205-c91d-44c9-8394-3e1790bd76f3
    infraID: my-gcp-cluster-wsvdn
    platform:
      gcp:
        networkProjectID: some@project.id
```

### Adopting with hiveutil

[hiveutil](hiveutil.md) is a development focused CLI tool which can be built from the hive repo. To adopt a cluster specify the following flags:

```bash
bin/hiveutil create-cluster --namespace=namespace-to-adopt-into --base-domain=example.com mycluster --adopt --adopt-admin-kubeconfig=/path/to/cluster/admin/kubeconfig --adopt-infra-id=[INFRAID] --adopt-cluster-id=[CLUSTERID]
```

### Transferring ownership

If you wish to transfer ownership of a cluster which is already managed by hive, and have access to the ClusterDeployment, there is no need to create a new ClusterDeployment using `hiveutil`. Instead, simply do the following:

1. Save the current `ClusterDeployment` and relevant creds and certs manifests locally.
    ``` bash
    oc get cd <clusterdeployment_name> -n <namespace> -o yaml > clusterdeployment.yaml
    oc get secrets <clusterdeployment_name_creds> -n <namespace> -o yaml > clusterdeployment_creds.yaml
    ```
1. Edit the `ClusterDeployment`, setting `spec.preserveOnDelete` to `true`. This ensures that the next step will only release the hive resources without destroying the cluster in the cloud infrastructure.
1. Delete the `ClusterDeployment`
1. From the hive instance that will adopt the cluster, `oc apply` the `ClusterDeployment`, creds and certs manifests you saved in the first step.
## Configuration Management

### Vertical Scaling
hive-operator deploys each component (the [hive-controllers](../config/controllers/deployment.yaml) and [hiveadmission](../config/hiveadmission/deployment.yaml) Deployments; and the [hive-clustersync](../config/clustersync/statefulset.yaml) StatefulSet) with default resource requests.
If you need to scale any of these components vertically, you may add one or more `deploymentConfig` sections to HiveConfig's `spec`. For example:

```yaml
  deploymentConfig:
  - deploymentName: hive-controllers
    resources:
      requests:
        memory: 256Mi
  - deploymentName: hive-clustersync
    resources:
      requests:
        cpu: 30m
        memory: 257Mi
      limits:
        cpu: 50m
  - deploymentName: hiveadmission
    resources:
      requests:
        cpu: 20m
```

For each entry, the `deploymentName` must match the `metadata.name` of the Deployment/StatefulSet.
The `resources` is a standard corev1.ResourceRequirements.

See [below](#scaling-clustersync) for information on _horizontally_ scaling the clustersync controller.

Note: The hive-operator itself must be scaled by directly editing its Deployment.

### SyncSet

Hive offers two CRDs for applying configuration in a cluster once it is installed: `SyncSet` for config destined for specific clusters in a specific namespace, and `SelectorSyncSet` for config destined for any cluster matching a label selector.

For more information please see the [SyncSet](syncset.md) documentation.

### Scaling ClusterSync
The clustersync controller is designed to scale horizontally, so increasing the number of clustersync controller replicas will scale the number of clustersync pods running, thereby increasing the number of simultaneous clusters getting syncsets applied to them.

In order to scale the clustersync controller, a section like the following should be added to HiveConfig:

```yaml
spec:
  controllersConfig:
    controllers:
    - config:
        replicas: 3
      name: clustersync
```



### Identity Provider Management

Hive offers explicit API support for configuring identity providers in the OpenShift clusters it provisions. This is technically powered by the above `SyncSet` mechanism, but is provided directly in the API to support configuring per cluster identity providers, merged with global identity providers, all of which must land in the same object in the cluster.

For more information please see the [SyncIdentityProvider](syncidentityprovider.md) documentation.

## Cluster Deprovisioning

```bash
oc delete clusterdeployment ${CLUSTER_NAME} --wait=false
```

Deleting a `ClusterDeployment` will create a `ClusterDeprovision` resource, which in turn will launch a pod to attempt to delete all cloud resources created for and by the cluster. This is done by scanning the cloud provider for resources tagged with the cluster's generated `InfraID`. (i.e. `kubernetes.io/cluster/mycluster-fcp4z=owned` or  `sigs.k8s.io/cluster-api-provider-aws/cluster/mycluster-fcp4z=owned`) Once all resources have been deleted the pod will terminate, finalizers will be removed, and the `ClusterDeployment` and dependent objects will be removed. The deprovision process is powered by vendoring the same code from the OpenShift installer used for `openshift-install cluster destroy`.
