# Using Hive

## Cluster Provisioning

### Prerequisites

  1. You will need a live and functioning Route53 DNS zone in the AWS account you will be installing the new cluster(s) into. For example if you own example.com, you could create a hive.example.com subdomain in Route53, and ensure that you have made the appropriate NS entries under example.com to delegate to the Route53 zone. When creating a new cluster, the installer will make future DNS entries under hive.example.com as needed for the cluster(s).
    * Note that there is an additional mode of DNS management where Hive can automatically create delegated zones for approved base domains. i.e. if hive.example.com exists already, you can specify a base domain of cluster1.hive.example.com on your ClusterDeployment, and Hive will create this zone for you, wait for it to resolve, and then proceed with installation. See below for additional info.
  1. Determine what OpenShift release image you wish to install, and possibly what Hive image you want to use to install it.
  1. Create a Kubernetes secret containing a docker registry pull secret (typically obtained from [try.openshift.com](https://try.openshift.com)).
        ```bash
        $ oc create secret generic mycluster-pull-secret --from-file=.dockerconfigjson=/path/to/pull-secret --type=kubernetes.io/dockerconfigjson
        ```
  1. Create a Kubernetes secret containing your AWS credentials:
        ```bash
        $ oc create secret generic test-creds --from-literal=aws_secret_access_key=$AWS_SECRET_ACCESS_KEY --from-literal=aws_access_key_id=$AWS_ACCESS_KEY_ID
        ```

### Using Global Pull Secret

GlobalPullSecret can be used to specify a pull secret that will be used globally by all of the cluster deployments created by Hive.
When GlobalPullSecret is defined in Hive namespace and a cluster deployment specific pull secret is specified, the registry authentication in both secrets will be merged and used by the new OpenShift cluster.
When a registry exists in both pull secrets, precedence will be given to the contents of the cluster specific pull secret.

The global pull secret must live in the "hive" namespace, to create one:

```bash
oc create secret generic global-pull-secret --from-file=.dockerconfigjson=<path>/config.json --type=kubernetes.io/dockerconfigjson --namespace hive
```

Edit the HiveConfig to add global pull secret.

```bash
oc edit hiveconfig hive
```

The global pull secret name must be configured in the HiveConfig CRD.

```yaml
spec:
  globalPullSecret:
    name: global-pull-secret
```

### Provisioning

Hive supports two methods of specifying what version of OpenShift you wish to install. Most commonly you can create a ClusterImageSet which references an OpenShift 4 release image, and a corresponding Hive image that is capable of installing it. The two are closely linked to accomodate code changes in the installer over time.

Alternatively you can specify release and Hive image overrides directly on your ClusterDeployment when you create one.

Cluster provisioning begings when a caller creates a ClusterDeployment CRD, which is the core Hive resource to control the lifecycle of a cluster.

An example ClusterDeployment:

```yaml
apiVersion: hive.openshift.io/v1alpha1
kind: ClusterDeployment
metadata:
  name: mycluster
spec:
  baseDomain: hive.example.com
  clusterName: mycluster
  compute:
  - name: worker
    platform:
      aws:
        rootVolume:
          iops: 100
          size: 22
          type: gp2
        type: m4.large
    replicas: 3
  controlPlane:
    name: master
    platform:
      aws:
        rootVolume:
          iops: 100
          size: 22
          type: gp2
        type: m4.large
    replicas: 3
  imageSet:
    name: openshift-v4.1.0-rc.8
  images:
    installerImagePullPolicy: Always
  networking:
    clusterNetworks:
    - cidr: 10.128.0.0/14
      hostSubnetLength: 23
    machineCIDR: 10.0.0.0/16
    serviceCIDR: 172.30.0.0/16
    type: OpenShiftSDN
  platform:
    aws:
      region: us-east-1
  platformSecrets:
    aws:
      credentials:
        name: mycluster-aws-creds
  pullSecret:
    name: mycluster-pull-secret
  sshKey:
    name: mycluster-ssh-key
```

The hiveutil CLI (see `make hiveutil`) offers a create-cluster command for generating a cluster deployment and submitting it to the cluster defined by your current kubeconfig.

```bash
bin/hiveutil create-cluster --base-domain=mydomain.example.com mycluster
```

To view what create-cluster generates, *without* submitting it to the API server, add `-o yaml` to the above command. If you need to make any changes not supported by create-cluster options, the output can be saved, edited, and then submitted with `oc apply`.

By default this command assumes the latest Hive master CI build, and the latest OpenShift stable release. `--hive-image` can be specified to use a specific Hive image to run the install, and `--release-image` can be specified to control which OpenShift release image to install in the cluster.

### Monitor the Install Job

* Get the namespace in which your cluster deployment was created
* Get the install pod name
  ```bash
  $ oc get pods -o json --selector job-name==${CLUSTER_NAME}-install | jq -r '.items | .[].metadata.name'
  ```
* Run following command to watch the cluster deployment
  ```bash
  $ oc logs -f <install-pod-name> -c hive
  ```
  Alternatively, you can watch the summarized output of the installer using
  ```bash
  $ oc exec -c hive <install-pod-name> -- tail -f /tmp/openshift-install-console.log
  ```

In the event of installation failures, please see [Troubleshooting](./troubleshooting.md).

### Cluster Admin Kubeconfig

Once the cluster is provisioned you will see a CLUSTER_NAME-admin-kubeconfig secret. You can use this with:

```bash
oc get secret ${CLUSTER_NAME}-admin-kubeconfig -o jsonpath='{ .data.kubeconfig }' | base64 -d > ${CLUSTER_NAME}.kubeconfig
export KUBECONFIG=${CLUSTER_NAME}.kubeconfig
oc get nodes
```

### Access the WebConsole

* Get the webconsole URL
  ```
  $ oc get cd ${CLUSTER_NAME} -o jsonpath='{ .status.webConsoleURL }'
  ```

* Retrive the password for `kubeadmin` user
  ```
  $ oc get secret ${CLUSTER_NAME}-admin-password -o jsonpath='{ .data.password }' | base64 -d
  ```

## DNS Management

Hive can optionally create delegated Route53 DNS zones for each cluster.

To use this functionality you must first define the base domains you want to manage DNS for in your HiveConfig.

```
apiVersion: hive.openshift.io/v1alpha1
kind: HiveConfig
metadata:
  name: hive
spec:
  managedDomains:
  - hive1.example.com
```

You can now request clusters with clusterDeployment.spec.manageDNS=true and clusterDeployment.spec.baseDomain=mycluster.hive1.example.com. Hive will create the mycluster.hive1.example.com DNS zone, and the OpenShift installer will create DNS entries such as api.mycluser.hive1.example.com.


## Configuration Management

### SyncSet

Hive offers two CRDs for applying configuration in a cluster once it is installed: SyncSet for config destined for specific clusters in a specific namespace, and SelectorSyncSet for config destined for any cluster matching a label selector.

For more information please see the [SyncSet](syncset.md) documentation.

### Identity Provider Management

Hive offers explicit API support for configuring identity providers in the OpenShift clusters it provisions. This is technically powered by the above SyncSet mechanism, but is provided directly in the API to support configuring per cluster identity providers, merged with global identity providers, all of which must land in the same object in the cluster.

For more information please see the [SyncIdentityProvider](syncidentityprovider.md) documentation.

## Cluster Deprovisioning

```bash
$ oc delete clusterdeployment ${CLUSTER_NAME} --wait=false
```

Deleting a ClusterDeployment will create a ClusterDeprovisionRequest resource, which in turn will launch a pod to attempt to delete all cloud resources created for and by the cluster. This is done by scanning the cloud provider for resources tagged with the cluster's generated InfraID. (i.e. kubernetes.io/cluster/mycluster-fcp4z=owned) Once all resources have been deleted the pod will terminate, finalizers will be removed, and the ClusterDeployment and dependent objects will be removed. The deprovision process is powered by vendoring the same code from the OpenShift installer used for `openshift-install cluster destroy`.

The ClusterDeprovisionRequest resource can also be used to manually run a deprovision pod for clusters which no longer have a ClusterDeployment. (i.e. clusterDeployment.spec.preserveOnDelete=true)

