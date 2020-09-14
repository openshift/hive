# Scaling Hive

This document describes the main places users are likely to see scalability issues with Hive.

## Hive uses CRDs!

Most importantly, be aware that Hive uses CRDs to store its state. The amount of data that Hive can store is limited by the cluster's ability to store CRs. In OpenShift, this boils down to "how much data can we stuff into etcd before the cluster itself becomes unstable?" Best to not chance it. We recommend keeping the total number of Hive CRs in a Hive cluster in the thousands. Anything approaching 10,000 is the danger zone. In practice this means that a single Hive cluster can manage about 1000 clusters.

# Horizontal vs. Vertical Scale

With the exception of install pods (used only when clusters are installing), Hive 1.x is not horizontally scalable at the worker level. Most of the work Hive does happens in the hive-controllers pod, which is one single pod on one single worker. This means that when no installs are running, if you have a cluster with 10 workers, 9 of the workers are very bored. Hive clusters are prime candidates for using worker autoscaling. Keep the worker count as low as you can, but allow bursts of concurrent installs to call for temporary workers to spin up.

In AWS, Hive performs best on c instance types. M instances are fine, but c instances are better.

In general, the faster your etcd storage, the faster Hive will perform. c5.4xlarge has twice the EBS bandwidth as c5.2xlarge, and so Hive performs a bit better on c5.4xlarge (although in our opinion, not so much faster that c5.4xlarge is worth the premium over c5.2xlarge). In AWS, we recommend using io1 with 1200 iops for your masters.

If you need to scale out the number of clusters you need to manage, use multiple Hive clusters -- approximately 1 Hive per 1000 clusters managed. Or fewer if you prefer to keep the blast radius smaller.

## Install Pods

Hive 1.x request 800 Mib of memory for each install pod. If you use m5.xlarge workers, you can support about (15 Gib / 800 Mib) install pods per worker -- so about 16. If you need to support more concurrent installs, you can use more workers, and/or workers with more memory. Install pods use barely any CPU.

## Blocking I/O

hive-controllers (where the syncset controllers run) uses blocking i/o. We may move to scale-out or non-blocking i/o in the future, but for now you should be aware that communication (mostly syncsets) to managed clusters can block other things.

## Threads

Hive supports configuring the number of goroutines per controller by editing values in HiveConfig. See [Using Hive](using-hive.md) for documentation on this. In general, set the number of goroutines to roughly the number of vCPUs on your workers. For c5.2xlarge, we use 10 goroutines. If you run Hive on larger hardware, you need to bump up the goroutines. If you have only 10 goroutines but 100 vCPUs, 90 of the vCPUs will be mostly unused. If Hive manages clusters that are on slow networks or have connectivity issues, you may want to use more goroutines to work around Hive's use of blocking i/o. Don't use too many goroutines, or that will actually cause a slowdown (i.e. don't just set it to 1000 or something).

## SyncSets

Pushing configuation to managed clusters via SyncSets is the most CPU-intensive and network-intensive thing that Hive does. We scale test Hive by mostly looking at how syncsets perform, since post-install this is what Hive spends the majority of its time doing.

The metric we judge syncset performance by is "applies per second". The following table lists some typical applies/sec rates for various hardware combinations we've tested.

|masters|workers|goroutines|applies/sec|
|---|---|---|---|
|c5.2xlarge|c5.2xlarge|20|700|
|c5.2xlarge|c5.2xlarge|40|700|
|c5.4xlarge|c5.4xlarge|20|1200|
|c5.4xlarge|c5.4xlarge|40|1400|
|c5.2xlarge|c5.4xlarge|20|1470|
|c5.2xlarge|c5.9xlarge|20|1750|
|c5.2xlarge|c5.9xlarge|40|2500|
|c5.2xlarge|c5.9xlarge|50|2500|
|c5.4xlarge|c5.24xlarge|100|2400|
