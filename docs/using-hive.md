# Using Hive

## Cluster Provisioning

### Prerequisites

  1. You will need a live and functioning DNS zone in the cloud account into which you will be installing the new cluster(s). For example if you own example.com, you could create a hive.example.com subdomain in Route53, and ensure that you have made the appropriate NS entries under example.com to delegate to the Route53 zone. When creating a new cluster, the installer will make future DNS entries under hive.example.com as needed for the cluster(s).
    * Note that there is an additional mode of DNS management where Hive can automatically create delegated zones for approved base domains. i.e. if hive.example.com exists already, you can specify a base domain of cluster1.hive.example.com on your ClusterDeployment, and Hive will create this zone for you, wait for it to resolve, and then proceed with installation. See below for additional info.
  1. Determine what OpenShift release image you wish to install.
  1. Create a Kubernetes secret containing a docker registry pull secret (typically obtained from [try.openshift.com](https://try.openshift.com)).
        ```bash
        oc create secret generic mycluster-pull-secret --from-file=.dockerconfigjson=/path/to/pull-secret --type=kubernetes.io/dockerconfigjson
        ```
  1. Create a Kubernetes secret containing a ssh key pair (typically generated with `ssh-keygen`)
        ```bash
        oc create secret generic mycluster-ssh-key --from-file=ssh-privatekey=/path/to/private/key --from-file=ssh-publickey=/path/to/public/key
        ```
     **NOTE**: This step is optional. This will be done automatically if using `hiveutil create-cluster` with `--ssh-public-key-file` and `--ssh-private-key-file` arguments. 
  1. Create a Kubernetes secret containing your AWS credentials:
        ```bash
        oc create secret generic mycluster-aws-creds --from-literal=aws_secret_access_key=$AWS_SECRET_ACCESS_KEY --from-literal=aws_access_key_id=$AWS_ACCESS_KEY_ID
        ```
     **NOTE**: This will be done automatically if using `hiveutil create-cluster`.
  1. Create a [PersistentVolume](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) for your `ClusterDeployment` to store the installation logs. The `accessModes` is `ReadWriteOnce` for your `PersistentVolume`. Note that if you do not want to capture must-gather logs then you can set `.spec.failedProvisionConfig.skipGatherLogs` to `true` in the `HiveConfig`.
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

The global pull secret name must be configured in the HiveConfig CR.

```yaml
spec:
  globalPullSecret:
    name: global-pull-secret
```

### Provisioning

Hive supports two methods of specifying what version of OpenShift you wish to install. Most commonly you can create a ClusterImageSet which references an OpenShift 4 release image.

An example ClusterImageSet:

```yaml
apiVersion: hive.openshift.io/v1
kind: ClusterImageSet
metadata:
  name: openshift-v4.2.0
spec:
  releaseImage: quay.io/openshift-release-dev/ocp-release:4.2.0
```

Alternatively you can specify release image overrides directly on your ClusterDeployment when you create one.

Cluster provisioning begins when a caller creates a ClusterDeployment CR, which is the core Hive resource to control the lifecycle of a cluster.

An example ClusterDeployment:

```yaml
apiVersion: hive.openshift.io/v1
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
    name: openshift-v4.2.0
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

The hiveutil CLI (see `make hiveutil`) offers a create-cluster command for generating a cluster deployment and submitting it to the Hive cluster using your current kubeconfig.

To view what create-cluster generates, *without* submitting it to the API server, add `-o yaml` to the above command. If you need to make any changes not supported by create-cluster options, the output can be saved, edited, and then submitted with `oc apply`.

By default this command assumes the latest Hive master CI build, and the latest OpenShift stable release. `--release-image` can be specified to control which OpenShift release image to install in the cluster.

#### Create Cluster on AWS

Credentials will be read from your AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables. Alternatively you can specify an AWS credentials file with --creds-file.

```bash
bin/hiveutil create-cluster --base-domain=mydomain.example.com --cloud=aws mycluster
```

#### Create Cluster on Azure

Credentials will be read from `~/.azure/osServicePrincipal.json` typically created via the `az login` command.

```bash
bin/hiveutil create-cluster --base-domain=mydomain.example.com --cloud=azure --azure-base-domain-resource-group-name=myresourcegroup --release-image=registry.svc.ci.openshift.org/origin/release:4.2 mycluster
```

`--release-image` is used above as Azure installer support is only present in 4.2 dev preview builds.

#### Create Cluster on GCP

Credentials will be read from `~/.gcp/osServiceAccount.json`, this can be created by:

 1. Login to GCP console at https://console.cloud.google.com/
 1. Create a service account with the owner role.
 1. Create a key for the service account.
 1. Select JSON for the key type.
 1. Download resulting JSON file and save to `~/.gcp/osServiceAccount.json`.

```bash
bin/hiveutil create-cluster --base-domain=mydomain.example.com --cloud=gcp --gcp-project-id=myproject --release-image=registry.svc.ci.openshift.org/origin/release:4.2 mycluster
```

`--release-image` is used above as GCP installer support is only present in 4.2 dev preview builds.

### Monitor the Install Job

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

### Access the WebConsole

* Get the webconsole URL
  ```
  oc get cd ${CLUSTER_NAME} -o jsonpath='{ .status.webConsoleURL }'
  ```

* Retrive the password for `kubeadmin` user
  ```
  oc get secret `oc get cd ${CLUSTER_NAME} -o jsonpath='{ .status.adminPasswordSecret.name }'` -o jsonpath='{ .data.password }' | base64 --decode
  ```

## DNS Management

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

Hive offers two CRDs for applying configuration in a cluster once it is installed: SyncSet for config destined for specific clusters in a specific namespace, and SelectorSyncSet for config destined for any cluster matching a label selector.

For more information please see the [SyncSet](syncset.md) documentation.

### Identity Provider Management

Hive offers explicit API support for configuring identity providers in the OpenShift clusters it provisions. This is technically powered by the above SyncSet mechanism, but is provided directly in the API to support configuring per cluster identity providers, merged with global identity providers, all of which must land in the same object in the cluster.

For more information please see the [SyncIdentityProvider](syncidentityprovider.md) documentation.

## Cluster Deprovisioning

```bash
oc delete clusterdeployment ${CLUSTER_NAME} --wait=false
```

Deleting a ClusterDeployment will create a ClusterDeprovision resource, which in turn will launch a pod to attempt to delete all cloud resources created for and by the cluster. This is done by scanning the cloud provider for resources tagged with the cluster's generated InfraID. (i.e. kubernetes.io/cluster/mycluster-fcp4z=owned) Once all resources have been deleted the pod will terminate, finalizers will be removed, and the ClusterDeployment and dependent objects will be removed. The deprovision process is powered by vendoring the same code from the OpenShift installer used for `openshift-install cluster destroy`.

The ClusterDeprovision resource can also be used to manually run a deprovision pod for clusters which no longer have a ClusterDeployment. (i.e. clusterDeployment.spec.preserveOnDelete=true)
