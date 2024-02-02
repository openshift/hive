# OpenShift Hive

API driven OpenShift 4 cluster provisioning and management.

Hive is an operator which runs as a service on top of Kubernetes/OpenShift.
The Hive service can be used to provision and perform initial configuration of OpenShift clusters.

* For provisioning OpenShift, Hive uses the [OpenShift installer](https://github.com/openshift/installer).

## Supported cloud providers

* AWS
* Azure
* Google Cloud Platform
* IBM Cloud
* OpenStack
* vSphere

In the future Hive will support more cloud providers.

# Documentation

* [Quick Start Guide](./docs/quick_start.md)
* [Installation](./docs/install.md)
* [Using Hive](./docs/using-hive.md)
  * [Cluster Hibernation](./docs/hibernating-clusters.md)
  * [Cluster Pools](./docs/clusterpools.md)
* [Hiveutil CLI](./docs/hiveutil.md)
* [Scaling Hive](./docs/scaling-hive.md)
* [Developing Hive](./docs/developing.md)
* [Frequently Asked Questions](./docs/FAQs.md)
* [Troubleshooting](./docs/troubleshooting.md)
* Architecture
  * [Hive Architecture](./docs/architecture.md)
  * [SyncSet](./docs/syncset.md)
  * [SyncIdentityProvider](./docs/syncidentityprovider.md)
