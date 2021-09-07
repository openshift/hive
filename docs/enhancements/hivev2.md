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
    - [ClusterTemplate (new)](#clustertemplate-new)
      - [Summary](#summary)
      - [Motivation](#motivation)
      - [By Reference Or By Value](#by-reference-or-by-value)
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

### ClusterTemplate (new)
#### Summary
A CR containing all the configurable fields of ClusterDeployment, to be included (by reference or by value -- see [discussion](#by-reference-or-by-value) below) in place of those fields when defining a ClusterDeployment or ClusterPool.

#### Motivation
Consumers of ClusterPools have discovered an unintended benefit: they're a convenient way to "template" ClusterDeployments.
This has led to a few interesting edge cases, such as using [zero-size pools](https://issues.redhat.com/browse/HIVE-1593) to get this templating behavior without actually using or caring about any of the intended pool-ish features.
Having a separate ClusterTemplate mechanism for creating ClusterDeployments would satisfy this use case without having to retrofit ClusterPools for this purpose.

Another pool-related motivation for this is that we continue to find use cases for adding more of the ClusterDeployment config fields to ClusterPools.
Each time this comes up we have to discuss whether it's worth expanding the ClusterPool spec for *this* field; and the duplication is just generally uncomfortable.
A ClusterTemplate object would provide a single source for *all* the configurable fields of ClusterDeployment.
The two would by definition be in sync from the start; and any time we decided to add a field it would automatically be available for both ClusterPools and ClusterDeployments.

#### By Reference Or By Value
By itself, the ClusterTemplate CR wouldn't result in any cloud objects being created.
(We might decide to have a controller validate some things about it.)
However, its inclusion in a ClusterDeployment spec would inform the configuration of the ClusterDeployment; and its inclusion in a ClusterPool spec would inform the configuration of ClusterDeployments created by that pool.
The question is: should the inclusion be

**by reference:** E.g. `ClusterDeployment.spec.clusterTemplateRef: corev1.ObjectReference` -- or maybe that's overkill and we define a `ClusterTemplateReference` type containing `name` and `namespace`.

or

**by value:** A `ClusterDeployment.Spec.ClusterTemplate` sub-field is actually of type `ClusterTemplate`.
Similar for ClusterPool.
Creating the ClusterDeployment/ClusterPool entails copying the ClusterTemplate's contents into that sub-field.

The consideration is how to handle ClusterTemplate changes.
For ClusterDeployment fields that can be configured after installation, does editing the ClusterTemplate effect those changes on the ClusterDeployment?
Clearly if we include by reference the answer must be yes; otherwise there is no other way to make those changes to the CD.
But for fields that are only used at install time, this could be confusing to debug, if what's in the ClusterTemplate doesn't match what was originally used for the CD.

However, including by value makes it tough to consume: you would have to write code to do the copy-in; otherwise you're no better off than you are with v1: effectively having to specify all the fields explicitly.

What I've seen in similar situations in the past (specifically nova "flavors" used to define VM "instances") is: Upon creation, the template is specified by reference; but the controller then copies it by value in its current form.
For this design that could look like: A spec field containing the reference; and a status sub-field containing the copy.

Or both could be spec fields. This way consumers could decide whether they want to define their CD via a template or "one off".

If we do that, then for ClusterPools we could stick to purely "by reference"; the values would be copied into each CD as it is created by the pool controller.

In any case we need to be careful to take [pool-to-CD versioning](https://issues.redhat.com/browse/HIVE-1058) into account.

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
