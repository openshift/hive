# ClusterPool Inventory

- [ClusterPool Inventory](#clusterpool-inventory)
  - [User Stories](#user-stories)
  - [Problem Statement](#problem-statement)
  - [Scope](#scope)
  - [Proposal](#proposal)
    - [Summary](#summary)
    - [`ClusterPool.Spec.Inventory`](#clusterpoolspecinventory)
    - [How To Use](#how-to-use)
    - [Validation](#validation)
    - [`Size` and `MaxSize`](#size-and-maxsize)
    - [Pool Version](#pool-version)
    - [Handling Inventory Updates](#handling-inventory-updates)
      - [Adding An Inventory](#adding-an-inventory)
      - [Adding An Entry](#adding-an-entry)
      - [Removing An Entry](#removing-an-entry)
      - [Deleting The Inventory](#deleting-the-inventory)
    - [Racing For Names](#racing-for-names)
    - [Fairness](#fairness)

## User Stories
As a cluster administrator, I want to be able to use ClusterPools to provision VSphere clusters.

(Future) As a cluster administrator, I want to be able to use ClusterPools to provision bare metal clusters.

## Problem Statement
Provisioning a cluster requires configuration unique to that cluster.
For example, the cluster's name is used to build the hostnames for the API and console.
_Someone_ has to handle the DNS that resolves those hostnames to the IPs of the services created by the installer.
One option is to create your ClusterDeployment with `ManageDNS=True` and we'll create a DNSZone on the fly.

But that doesn't work for some cloud providers, such as VSphere, the focus of this document.

In the case of VSphere, one solution is to configure DNS manually and then create your ClusterDeployment with the right `Name` and `BaseDomain` so that the assembled hostnames match those entries. This is fine if you're creating ClusterDeployments by hand, but breaks down if ClusterDeployment names are being generated with random slugs, as is the case with ClusterPools.

## Scope
This feature is limited to solving DNS for VSphere.
Bare metal will come later, _but_ we will try to be forward-thinking in this design to allow for future expansion to cover that user story.

## Proposal
### Summary
Allow ClusterPool to accept an inventory of per-cluster details to be used when generating ClusterDeployments for the pool.
For the current design, only the cluster (short) name is supported.

### `ClusterPool.Spec.Inventory`
Add a field to `ClusterDeployment.Spec` called `Inventory`.
It is a dict.
For now, the dict can have one member, `ClusterDeployments`, which is a list of dicts.
For now, each dict in the list can have one member, `Name`, which is a string.
Example:

```yaml
spec:
  inventory:
    clusterDeployments:
    - name: foo
    - name: bar
    - name: baz
```

(This may seem overengineered, but the idea is to leave room for future expansion.
For example, perhaps we'll need to specify IP addresses that are tied to each name.
If we designed the inventory as simply a list of name strings, there would be nowhere to put those IPs.)

When adding a ClusterDeployment, if such an `Inventory` is present, ClusterPool will:
- Load up the inventory list.
- Load up the existing pool CDs (it already does this).
- Subtract the set of names of existing CDs from those in the inventory list.
- Pick a name from the set that remains.
- Use the name for the new CD:
  - As the `.spec.clusterName`.
  - As the seed for the `.metadata.namespace` and `.metadata.name`, to which a random slug will be appended as usual.

Absent an `Inventory`, ClusterPool will continue to use the generated namespace/name (`{pool name}-{random slug}`) as the `.spec.clusterName` as it does today.

(**Note:** We will not restrict the use of `Inventory` to VSphere.
I don't know why you would want to use it for other cloud providers, but you can if you like.
In fact, that's 100% how I plan to test this.)

### How To Use
For the VSphere case, this allows the administrator to:
- Preconfigure DNS.
- Provide the list of (short) hostnames to `ClusterPool.Spec.Inventory`.

For example, if DNS is configured with:
```
10.0.0.10  api.foo.example.com
10.0.0.11  apps.foo.example.com
10.0.0.12  console.foo.example.com

10.0.1.10  api.bar.example.com
10.0.1.11  apps.bar.example.com
10.0.1.12  console.bar.example.com
```
the ClusterPool could be configured with:
```yaml
...
spec:
  baseDomain: example.com
  inventory:
    clusterDeployments:
    - name: foo
    - name: bar
  ...
```

### Validation
Webhook validation will ensure that, if `Inventory.ClusterDeployments` is specified
- Each entry contains the `Name` key.
- The list of `Name` values contains no empties or duplicates.

This validation can be loosened in the future if the need arises without violating the API contract.
Like maybe for baremetal, we need to supply a preconfigured hardware inventory for each cluster, but it's okay to generate the names.

### `Size` and `MaxSize`
If `Inventory` is used, `ClusterPool.Spec.Size` and `.MaxSize` are implicitly constrained to the length of the list of nonempty `ClusterDeployments[].Names`.
Setting either/both to a smaller number still works as you would expect.

### Pool Version
To make [adding](#adding-an-inventory) and [deleting](#deleting-the-inventory) `Inventory` work sanely, we will adjust the computation of the pool version used for [stale CD detection/replacement](https://issues.redhat.com/browse/HIVE-1058) as follows:
- When `Inventory` is present, append an arbitrary fixed string before the [final hash operation](https://github.com/openshift/hive/blob/0b9229b91e6f8a3c2bf095efbdf017226e69d026/pkg/controller/clusterpool/clusterpool_controller.go#L479).
  (We don't want to recalculate the hash any time the inventory changes, as this would treat *all* existing unclaimed CDs as stale and replace them.)
- When `Inventory` is absent, compute the pool version as before.
  This ensures the version does not change (triggering replacement of all unclaimed CDs) for existing pools when hive is upgraded to include this feature.

### Handling Inventory Updates
#### Adding An Inventory
Adding an `Inventory` to a ClusterPool which previously didn't have one will cause the controller to [recompute the pool version](#pool-version), rendering all existing unclaimed clusters stale, causing them to be replaced gradually.
The replacements will then be named according to the inventory as described [above](#clusterpoolspecinventory).

#### Adding An Entry
If `MaxSize` is unset or exceeds the length of the inventory, and `Size` allows, adding a new entry will cause a new ClusterDeployment to be added to the pool.
If the pool is already at `[Max]Size` there is no immediate effect.

#### Removing An Entry
- If the entry is unused (no pool CD with that name exists), this is a no-op.
- If an _unclaimed_ CD exists with that name, we delete it.
  The controller will replace it, assuming an unused entry is available.
- If a _claimed_ CD exists with that name, no-op.

These are conceived to correlate as closely as possible to what happens when editing a pool's `Size`.

#### Deleting The Inventory
This will change the [pool version](#pool-version), rendering existing unclaimed clusters stale and causing the controller to replace them gradually.
Absent the inventory, the replacements will have generated names as "normal" (before this feature).
The administrator may wish to speed up this process by manually deleting CDs, or scaling the pool size to zero and back.

### Racing For Names
In a single-threaded world, we would be able to tell which names from the inventory are used/available -- e.g. for purposes of picking an unused one for the next ClusterDeployment -- by building the set of `.spec.clusterName`s of all the pool's CDs.
However, if multiple controller pods are running concurrently, we can race:
- Controller pod A loads up the CDs and discovers the name `foo` is available
- Controller pod B loads up the CDs and discovers the name `foo` is available
- A generates namespace/name `foo-abc` and creates a new CD with `clusterName=foo` in that namespace
- B generates namespace/name `foo-xyz` and creates a new CD with `clusterName=foo` in _that_ namespace
- Chaos ensues, with two clusters attempting to claim the same hostnames, DNS, etc.

To solve this problem, we will add `ClusterPool.Status.ClusterNameUsage` containing a JSON dict, keyed by the cluster name from the inventory list, of the namespace of the CD consuming it.
- When the value is empty/null/nil, that name hasn't been claimed yet.
- When we need to create a new cluster, we
  - Scan the list for an entry with a nil value
  - Generate a unique namespace name using that name as a stub
  - Post an update to the CD with the annotation updated with that namespace as the value for that entry.
    Here's where we bounce if two controllers try to grab the same name at the same time: the loser will bounce the update with a 409; we'll requeue and start over.
  - Proceed with existing algorithm to create new cluster.
- Add a top-level routine to ensure all replete dict entries have CDs associated with them.
  If not, attempt to create them through the same flow.
  This is to guard against leaking entries if a controller crashes between updating the annotation and creating the CD.

### Fairness
We will assume it is **not** important that we rotate through the list of supplied cluster names in any particular order, or with any nod to "fairness".
For example: If there are five names and the usage pattern happens to only use two at a time, there is no guarantee that we will round-robin through all five.
We may reuse the same two over and over, or pick at random, or something else -- any of which has the potential to "starve" some names.
