# Troubleshooting

## Cluster Install Failure Logs

In the event a cluster is brought up but overall installation fails, either during bootstrap or cluster initialization, Hive will attempt to gather logs from the cluster itself. These logs are stored in a persistent volume created for every install job. If the install succeeds on the first attempt, the persistent volume claim is immediately deleted. If the install has had any errors along the way, it will be preserved for 7 days and then removed.

You can access these logs gathered from the cluster with the following script found in the hive.git repository:

```bash
$ hack/logextractor.sh mycluster ./extracted-logs/
```

## Deprovision

After deleting your cluster deployment you will see an uninstall job created. If for any reason this job gets stuck you can:

 1. Delete the uninstall job. It will be recreated and tried again.
 2. Manually delete the uninstall finalizer allowing the cluster deployment to be deleted, but note that this may leave artifacts in your AWS account.
 3. You can manually run the uninstall code with `hiveutil` to delete AWS resources based on their tags.
    * Run `make hiveutil`
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
# oc create --raw /apis/admission.hive.openshift.io/v1alpha1/dnszones -f config/samples/hiveadmission-review-failure.json -v 8 | jq
```

