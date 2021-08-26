# Hive v2

- [Hive v2](#hive-v2)
  - [Overview](#overview)
    - [This is Not](#this-is-not)
    - [How To Use](#how-to-use)
  - [APIs](#apis)
    - [Checkpoint](#checkpoint)
    - [ClusterClaim](#clusterclaim)
    - [ClusterDeployment](#clusterdeployment)
      - [`powerState` Sub-Struct](#powerstate-sub-struct)
    - [ClusterDeprovision](#clusterdeprovision)
    - [ClusterImageSet](#clusterimageset)
    - [ClusterPool](#clusterpool)
    - [ClusterProvision](#clusterprovision)
    - [ClusterRelocate](#clusterrelocate)
    - [ClusterState](#clusterstate)
    - [DNSZone](#dnszone)
    - [HiveConfig](#hiveconfig)
      - [Remove VeleroBackup Controller](#remove-velerobackup-controller)
    - [MachinePoolNameLease](#machinepoolnamelease)
    - [MachinePool](#machinepool)
    - [SelectorSyncIdentityProvider](#selectorsyncidentityprovider)
    - [SelectorSyncSet](#selectorsyncset)
    - [SyncIdentityProvider](#syncidentityprovider)
    - [SyncSet](#syncset)

## Overview
This document exists to collect ideas for version 2 of the Hive API.

### This is Not
- A formal design (yet).
- A commitment to implement anything herein.
- For internals (e.g. ClusterSync[Lease]).

### How To Use
Have an idea for v2 of a Hive API?
See an idea already listed that you can improve upon?
Propose a PR editing this document.
Discuss the idea in PR review.
Merge once it's deemed feasible and at least somewhat desirable, and with enough details to springboard a design.
(It need not be fully fleshed out.)

## APIs

### Checkpoint

### ClusterClaim

### ClusterDeployment
#### `powerState` Sub-Struct
Coming out of [HIVE-1602](https://issues.redhat.com/browse/HIVE-1602) and [this series of comments](https://github.com/openshift/hive/pull/1506#discussion_r691644196) on [#1506](https://github.com/openshift/hive/pull/1506):

We want to be able to indicate multiple things related to `powerState`, including:
- The desired power state of the ClusterDeployment
- A timeout for Resuming from hibernation after which we declare the CD to be broken
- Likewise for Hibernating

Draft pseudo-design:
```
spec:
  ...
  powerState:
    desired: [String] {enum: Running|Hibernating}
    transitionToRunningTimeout: [Duration] {nil means "wait forever" -- current behavior}
    transitionToHibernatingTimeout: [Duration] {ditto}
  ...
```

### ClusterDeprovision

### ClusterImageSet

### ClusterPool

### ClusterProvision

### ClusterRelocate

### ClusterState

### DNSZone

### HiveConfig
#### Remove VeleroBackup Controller
This is not used or useful, so we should get rid of it.

**Card:** [HIVE-1563](https://issues.redhat.com/browse/HIVE-1563)

**Prototype:** https://github.com/openshift/hive/pull/1411

...but we need to deprecate it properly first.

### MachinePoolNameLease

### MachinePool

### SelectorSyncIdentityProvider

### SelectorSyncSet

### SyncIdentityProvider

### SyncSet
