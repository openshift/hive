# Store OpenShift Install Logs in S3 Compatible Storage


## Summary

The purpose of this enhancement is to change how Hive stores OpenShift install logs. Currently Hive stores OpenShift install logs in a persistent volume (PV).

This enhancement changes the OpenShift install log location from placing them in PVs to placing them in S3 compatible object storage.


## Motivation
Storing OpenShift install logs inside of a PV works, but has several drawbacks:
1. The cluster running Hive must have PVs enabled (with or without storage classes). This, for example, affects developer Hive instances.
1. The number of OpenShift install pods is limited by the number of available PVs. Hive could exhaust a cluster's available PVs by doing too many OpenShift installs.
1. Hive users must get the OpenShift install log through Hive instead of going directly to the storage. When OpenShift install logs are written to S3, then a Hive administrator can go directly to S3 to get the logs.
1. OpenShift install logs are relatively small, but minimum PV sizes are usually large. For instance, in AWS, the minimum size of a PV is 1 GiB, but OpenShift install logs tend to be less than 100 MiB.

### Goals
Store OpenShift install logs inside of S3 object storage.

### Non-Goals
None

## Open Questions
1. What is the best way to pass the S3 parameters from the Hive operator to Hive controllers to the OpenShift install pod?
   1. MUST allow for other object storage machanisms to be added in the future.
   1. We'll look to how other projects like Velero do this as a reference.
1. What object should store the URL of the OpenShift install log in S3?
   1. Something like ClusterProvision.Status.S3.URI.

## Proposal
Change Hive to store OpenShift install logs in S3 compatible object storage instead of persistent volumes.

Ensure that future enhancements can use similar mechanisms to upload to other object storage systems.

## User Stories

### Story 1
As a Hive administrator,
I want OpenShift install logs to be stored in S3 compatible object storage,
So that simultaneous Hive triggered OpenShift installs can scale with available compute and memory instead of PVs.

### Story 2
As an Hive Developer,
I want OpenShift install logs to be stored in S3 compatible object storage,
So that while developing, I can use Minio to capture OpenShift install logs.


## Implementation Details / Notes / Constraints
1. Configure S3 URL and credentials in HiveConfig. Have these passed down into OpenShift install pods.
  1. If no S3 information is given, do not gather OpenShift install logs. This will use the functionality already present for skipGatherLogs.
1. From the OpenShift install pods, upload the OpenShift install log to S3.
  1. Organize install logs using clustername, namespace. Then, name the install log with the cluster provision name. Use a pattern like: <clustername>-<namespace>/<clusterprovisionname>-install-log.tgz
    1. Example: osd-e2e-uhc-staging-1fmf7erifle5sibk8gs7puvuun7fcv1o/rbt45-0-hrt7w-install-log.tgz
1. Remove all PV logic from the Hive code that launches the OpenShift install pod.
1. Update the log retrieval script to gather the logs from S3 possibly using the URL and credentials in HiveConfig
1. Update Hive documentation for troubleshooting failed OpenShift installs to point to the OpenShift install logs in S3.
1. Store the URL of the OpenShift install log in S3 in a TBD Hive object.
  1. Something like ClusterProvision.Status.S3.URI

## Risks and Mitigations
There is a risk of using a large amount of data in S3 and having these OpenShift install logs live forever.

Mitigations:
- Set Object lifecycle policies on the S3 compatible storage bucket. These can automatically clean up this storage based on end user needs. The recommended amount of time to keep failed install logs is 1 week. Since these policies can only be applied to the whole bucket, 1 week for all install logs is recommended.
  - S3 has [object lifecycle policies](https://docs.aws.amazon.com/AmazonS3/latest/dev/object-lifecycle-mgmt.html)
  - MinIO has [object lifecycle policies](https://docs.min.io/docs/minio-bucket-lifecycle-guide.html)
- End user can remove OpenShift install logs from S3 if desired (won't affect Hive)

### Test Plan

- Unit tests for:
  - ensuring S3 details are added to the controllers pod
  - ensuring S3 details are added to the OpenShift install pod
  - ensuring S3 upload is triggered after OpenShift install has completed

- Manual tests for:
  - ensuring S3 details make it to OpenShift install pod
  - ensuring OpenShift install log pod uploads OpenShift install log to S3

## Drawbacks
- End user must setup and maintain S3 compatible object storage.
- Current default in Hive is to automatically store logs in PVs and failing to launch installs if no PVs are available. This new method defaults to storing logs in S3 compatible object storage and defaults to not storing logs if the object storage information isn't set up.

## Alternatives
- Store OpenShift install logs in persistent volume.
- Store OpenShift install logs in a different type of database (and maintain that database).
- Upload using different protocol like FTP.


## Infrastructure Needed
S3 bucket created with URL and credentials. This can also be minio or any other S3 compatible object storage.
