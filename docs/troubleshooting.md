# Troubleshooting

- [ClusterDeployment status conditions](#clusterdeployment-status-conditions)
  - [Cluster Install fails](#cluster-install-fails)
  - [Hibernation](#hibernation)
- [Cluster Install Failure Logs](#cluster-install-failure-logs)
  - [Setup](#setup)
  - [Listing stored install logs directories](#listing-stored-install-logs-directories)
  - [Retrieving stored install logs for a specific cluster provision](#retrieving-stored-install-logs-for-a-specific-cluster-provision)
- [Deprovision](#deprovision)
- [HiveAdmission](#hiveadmission)
- [Cluster's API Certificate Changed](#clusters-api-certificate-changed)
  - [Updating Certificate Authorities within the Admin Kubeconfig Secret](#updating-certificate-authorities-within-the-admin-kubeconfig-secret)
  - [Setting a new CA certificate globally on the managing cluster with hiveconfig](#setting-a-new-ca-certificate-globally-on-the-managing-cluster-with-hiveconfig)

## ClusterDeployment status conditions

The best way of gathering the initial information about the cluster would be to look at the `clusterdeployment.status.conditions[]`. Hive always attempts to bubble up the errors as a part of the relevant `condition.reason` and `condition.message`
There are 20+ clusterdeployment conditions, so to make parsing easier, these conditions are always sorted  in undesired > desired > initialized/unknown state. This means,  all the conditions which indicate a status that could potentially point at an error will be at the top. Furthermore, within the 3 subtypes, all conditions will also be sorted in alphabetical order of their `condition.type`

### Cluster Install fails

In the event of Cluster install failing:

- `RequirementsMet` condition set as true would indicate that all pre-provision requirements have been met
- `ProvisionFailed` condition would indicate if the provision has failed. Installer logs will be available in the hive container of the related clusterProvision pod. For logs from the cluster itself, see [Cluster Install Failure Logs](#cluster-install-failure-logs)
- `ProvisionStopped` set to true will indicate that a provision will no longer be attempted.

### Hibernation

For clusters that do support [hibernation](./hibernating-clusters.md), `Hibernating` and `Ready` conditions work in tandem to report the accurate status when the cluster is transitioning from one powerState to another. In case the transition is taking too long, look at the `clusterDeployment.powerState` as well as the reason+message of these conditions.
Note: when the cluster is hibernating, the `Unreachable` condition is expected to be set to true.

## Cluster Install Failure Logs

In the event a cluster is brought up but overall installation fails, either during bootstrap or cluster initialization, Hive will attempt to gather logs from the cluster itself. If configured, these logs are stored in an S3 compatible object store under a directory created for each cluster provision. If the install succeeds on the first attempt, then nothing will be stored. If the install has had any errors that cause an install log to be created, then it will uploaded to the configured object store.

### Setup

In order for Hive to gather and upload install logs on cluster provision failure, the object store must have a place for Hive to store data and Hive must be configured with the object store information.
See [this section](using-hive.md#saving-logs-for-failed-provisions) for setup details.

### Listing stored install logs directories

The logs gathered from the cluster can be accessed with the `logextractor.sh` script found in the Hive git repository.

In order to sync down the correct install logs, it is necessary to know which directory to sync. This can be determined by listing the install log directories with the following command:

```bash
$ hack/logextractor.sh ls
```

### Retrieving stored install logs for a specific cluster provision

To sync down the desired install logs, run the following command using the directory name determined in the command above. In this example, `cluster1-6a85a345-namespace` is used as an example directory name:

```bash
$ hack/logextractor.sh sync cluster1-6a85a345-namespace /path/to/store/the/logs
```

## Deprovision

After deleting your cluster deployment you will see an uninstall job created. If for any reason this job gets stuck you can:

 1. Delete the uninstall job. It will be recreated and tried again.
 2. Manually delete the uninstall finalizer allowing the cluster deployment to be deleted, but note that this may leave artifacts in your AWS account.
 3. You can manually run the uninstall code with `hiveutil` to delete AWS resources based on their tags.
    * Run `make build`
    * Get your cluster tag i.e. `infraID` from the following command output.
      ```bash
      $ oc get cd ${CLUSTER_NAME} -o jsonpath='{ .status.infraID }'
      ```
    * In case your cluster deployment is not available, you can find the tag in AWS console on any object from that cluster.
    * Run following command to deprovision artifacts in the AWS.
      ```bash
      $ bin/hiveutil aws-tag-deprovision --loglevel=debug kubernetes.io/cluster/<infraID>=owned sigs.k8s.io/cluster-api-provider-aws/cluster/<infraID>=owned
      ```

## HiveAdmission

To diagnose a hiveadmission failure, try running the operation directly against the registered hiveadmission API server.

For instance, try this:
```sh
# oc create --raw /apis/admission.hive.openshift.io/v1/dnszones -f config/samples/hiveadmission-review-failure.json -v 8 | jq
```

## Cluster's API Certificate Changed

When the API certificate of a Hive managed cluster changes, and is signed by a different certificate authority, this can result in the cluster becoming unreachable as denoted by the `unreachabe` condition within `ClusterDeployment` status. If your cluster is unreachable and you are seeing `x509: certificate signed by unknown authority` within a `ClusterDeployments'` `unreachable` condition message then the API certificate of the cluster has likely changed.

Hive uses a kubeconfig (a secret referenced from the `ClusterDeployment` object as `cd.spec.clusterMetadata.adminKubeconfigSecretRef`) to communicate with managed clusters. The kubeconfig is generated by the installer and contains the certificate authority which was used to sign the cluster's API server cerficiate.

To restore Hive communication with the cluster we will need to ensure that the certificate authority (full chain) of the new API server certificate has been added to `"certificate-authority-data"` within the kubeconfig referenced by the `ClusterDeployment`.

**In all cases, you will need the certificate authority (CA) file for your new API certificate** in standard PEM format. It should look something like this; multiple `BEGIN/END` `CERTIFICATE` sections may denote a full trust chain of multiple certificate authorities which will be needed for subsequent steps.
```sh
$ cat new-ca-certificate.pem
```

```
-----BEGIN CERTIFICATE-----
<base64 encoded certificate blob>
-----END CERTIFICATE-----
...
-----BEGIN CERTIFICATE-----
<potentially more base64 encoded certificate blobs>
-----END CERTIFICATE-----
```

### Updating Certificate Authorities within the Admin Kubeconfig Secret

The CA certificate used to sign the new API certificate must be added to the kubeconfig, alongside or replacing any existing certificates. Start by inspecting your current kubeconfig:

```sh
$ export MY_KUBECONFIG_NAME=$(oc get cd $CLUSTERNAME -o "jsonpath={.spec.clusterMetadata.adminKubeconfigSecretRef.name}")
$ oc extract secret/$MY_KUBECONFIG_NAME --keys=kubeconfig --to=- > my-kubeconfig-file
$ cat my-kubeconfig-file
```

```yaml
...
clusters:
- cluster:
    certificate-authority-data: <base64 encoded blob>
    server: https://api.<your cluster>.<your domain>:6443
  name: <your cluster>
...
```

The field `certificate-authority-data` is what needs to be updated. When base64 decoded, it looks something like this:

```sh
$ echo <base64 encoded blob> | base64 --decode > decoded-existing-certs.pem
$ cat decoded-existing-certs.pem
```

```
-----BEGIN CERTIFICATE-----
<base64 encoded certificate blob>
-----END CERTIFICATE-----
...
-----BEGIN CERTIFICATE-----
<potentially more base64 encoded certificate blobs>
-----END CERTIFICATE-----
```

Concatenate your new CA certificate with the existing decoded CA certificates:

```sh
$ cat decoded-existing-certs.pem new-ca-certificate.pem | openssl base64 -A
```

Copy the contents of this new base64 encoded blob and use it to replace the `certificate-authority-data` field in your kubeconfig. After the kubeconfig file is ready, patch your cluster to use the new kubeconfig:

```sh
$ oc patch secret $MY_KUBECONFIG_NAME --type='json' -p="[{'op': 'replace', 'path': '/data/kubeconfig', 'value': '$(openssl base64 -A -in my-updated-kubeconfig-file)'},{'op': 'replace', 'path': '/data/raw-kubeconfig', 'value': '$(openssl base64 -A -in my-updated-kubeconfig-file)'}]"
```

### Setting a new CA certificate globally on the managing cluster with hiveconfig

Hive can use its own configuration file to configure an additional CA certificate which will be added to kubeconfigs referenced by _ALL_ ClusterDeployments.  Use the `hack/set-additional-ca.sh` script to accomplish this:

```sh
$ hack/set-additional-ca.sh new-ca-certificate.pem
```

Note that this technique will replace any existing additional CA certificates that Hive currently is using.  If you wish to add your new CA to the existing additional CA, you must merge the new and old CAs together:

```sh
$ oc extract secret/additional-ca -n hive --to=- | base64 --decode > old-ca-certificate.pem
$ cat old-ca-certificate.pem new-ca-certificate.pem > combined-ca-certificates.pem
$ hack/set-additional-ca.sh combined-ca-certificates.pem
```