# OpenShift Hive

API driven OpenShift 4 cluster provisioning and management.

Hive is an operator which runs as a service on top of Kubernetes/OpenShift.
The Hive service can be used to provision and manage OpenShift clusters.

* For provisioning OpenShift, Hive uses the [OpenShift installer](https://github.com/openshift/installer).
* For `day 2` configuration management, Hive provides the [SyncSet and SelectorSyncSet CRDs](./docs/syncset.md) to deliver arbitrary Kubernetes configuration to the cluster.

Supported cloud providers:
* AWS

In the future Hive will support more cloud providers.

# Documentation

* [Architecture](./docs/architecture.md)
* [Installation](./docs/install.md)
* [Using Hive](./docs/using-hive.md)
* [Developing Hive](./docs/developing.md)
* [SyncSet](./docs/syncset.md)
* [SyncIdentityProvider](./docs/syncidentityprovider.md)
* [Frequently Asked Questions](./docs/FAQs.md)
* [Troubleshooting](./docs/troubleshooting.md)
