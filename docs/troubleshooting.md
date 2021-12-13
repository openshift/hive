# Troubleshooting

- [Troubleshooting](#troubleshooting)
  - [Cluster Install Failure Logs](#cluster-install-failure-logs)
    - [Setup](#setup)
    - [Listing stored install logs directories](#listing-stored-install-logs-directories)
    - [Retrieving stored install logs for a specific cluster provision](#retrieving-stored-install-logs-for-a-specific-cluster-provision)
  - [Deprovision](#deprovision)
  - [HiveAdmission](#hiveadmission)

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
      $ bin/hiveutil aws-tag-deprovision --loglevel=debug kubernetes.io/cluster/<infraID>=owned
      ```

## HiveAdmission

To diagnose a hiveadmission failure, try running the operation directly against the registered hiveadmission API server.

For instance, try this:
```sh
# oc create --raw /apis/admission.hive.openshift.io/v1/dnszones -f config/samples/hiveadmission-review-failure.json -v 8 | jq
```

