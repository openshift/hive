<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Frequently Asked Questions](#frequently-asked-questions)
  - [How does Hive relate to the OpenShift 4 installer (openshift-install)?](#how-does-hive-relate-to-the-openshift-4-installer-openshift-install)
  - [Why doesn't Hive use Federation v2 for configuration management?](#why-doesnt-hive-use-federation-v2-for-configuration-management)
  - [How does Hive relate to the sig-cluster-lifecycle Cluster API project?](#how-does-hive-relate-to-the-sig-cluster-lifecycle-cluster-api-project)
  - [Why not merge `SelectorSyncSet` and `SyncSet` into one CRD?](#why-not-merge-selectorsyncset-and-syncset-into-one-crd)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Frequently Asked Questions

## How does Hive relate to the OpenShift 4 installer (openshift-install)?

Hive leverages the OpenShift 4 installer to perform actual cluster provisioning. We expose a similar API, but Hive has been built for managing a large number of clusters at scale, rather than just installing one. Hive offers a programatic API to provision and perform some initial configuration required for anyone bringing up OpenShift clusters at scale.

## Why doesn't Hive use Federation v2 for configuration management?

During development Hive was initially being written to leverage Federation v2 for all day 2 configuration management in the cluster.

However this effort had to be abandoned due to some conflicts in how Hive segregates cluster information by namespace, a lack of API stability in Federation V2 in the required time frame, and some missing functionality around our need to apply cluster configuration, rather than application configuration. These issues have been discussed with Federation developers and we may be able to resume the Federation V2 approach in the future.

## How does Hive relate to the sig-cluster-lifecycle Cluster API project?

Hive is built to leverage the opinionated OpenShift 4 install process. This does not presently have any overlap with the ClusterAPI actuators, although OpenShift 4 itself does leverage a subset of the Cluster API in-cluster with use of the Machine API.

## Why not merge `SelectorSyncSet` and `SyncSet` into one CRD?

`SyncSets` transfer per cluster certificates, identity providers, lists of dedicated admin usernames, etc. Pushing this up to a global `SelectorSyncSet` CRD complicates RBAC and potentially exposes us to reveal more information to someone than we wanted to. The distinction between the two offers better flexibility for RBAC in a multi-tenant use of Hive, and possibly better security as well.

Please refer to [Issue 601](https://github.com/openshift/hive/issues/601) to get more detail.

Testing Konflux
