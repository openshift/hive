- [Frequently Asked Questions](#Frequently-Asked-Questions)
  - [How does Hive relate to the OpenShift 4 installer (openshift-install)?](#How-does-Hive-relate-to-the-OpenShift-4-installer-openshift-install)
  - [Why doesn't Hive use Federation v2 for configuration management?](#Why-doesnt-Hive-use-Federation-v2-for-configuration-management)
  - [How does Hive relate to the sig-cluster-lifecycle Cluster API project?](#How-does-Hive-relate-to-the-sig-cluster-lifecycle-Cluster-API-project)

# Frequently Asked Questions

## How does Hive relate to the OpenShift 4 installer (openshift-install)?

Hive leverages the OpenShift 4 installer to perform actual cluster provisioning. We expose a similar API, but Hive has been built for managing a large number of clusters at scale, rather than just installing one. Hive offers a programatic API to control a number of day 2 configuration parameters, as well as a mechanism to manage arbitrary Kubernetes config in the resulting cluster.

## Why doesn't Hive use Federation v2 for configuration management?

During development Hive was initially being written to leverage Federation v2 for all day 2 configuration management in the cluster.

However this effort had to be abandoned due to some conflicts in how Hive segregates cluster information by namespace, a lack of API stability in Federation V2 in the required time frame, and some missing functionality around our need to apply cluster configuration, rather than application configuration. These issues have been discussed with Federation developers and we may be able to resume the Federation V2 approach in the future.

## How does Hive relate to the sig-cluster-lifecycle Cluster API project?

Hive is built to leverage the opinionated OpenShift 4 install process. This does not presently have any overlap with the ClusterAPI actuators, although OpenShift 4 itself does leverage a subset of the Cluster API in-cluster with use of the Machine API.
