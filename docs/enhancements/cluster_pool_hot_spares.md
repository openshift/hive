# Maintaining Active Clusters in a ClusterPool

[HIVE-1576](https://issues.redhat.com/browse/HIVE-1576)

- [Maintaining Active Clusters in a ClusterPool](#maintaining-active-clusters-in-a-clusterpool)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
    - [User Story](#user-story)
  - [Proposals](#proposals)
    - [ClusterClaim Prioritizes Active Clusters](#clusterclaim-prioritizes-active-clusters)
    - [ClusterClaim Must Account For `hibernateAfter`](#clusterclaim-must-account-for-hibernateafter)
    - [Don't Hibernate New ClusterDeployments](#dont-hibernate-new-clusterdeployments)
    - [`activeCount`/`activePercent`](#activecountactivepercent)
  - [Risks and Mitigations](#risks-and-mitigations)
  - [Test Plan](#test-plan)
  - [Alternatives](#alternatives)

## Summary

Support configuring ClusterPools such that some number of clusters (0 <= N <= all) are Active for some amount of time (0 <= T <= eternity) before hibernating.

## Motivation

Today ClusterPools immediately hibernate new clusters.
This means a ClusterClaim is guaranteed to suffer at least Resuming time before the cluster is available for use.
While this is still faster than having to create a cluster from scratch, it is a potentially-unnecessary delay.
As an example, CI runs could be reduced by ~5m if a ClusterClaim were able to grab an Active cluster rather than having to Resume a Hibernating one.

### Goals

Ability to tune a ClusterPool such that a ClusterClaim gets an Active cluster immediately in the common case.

### Non-Goals

Solving world peace.


### User Story

As a consumer of a (properly tuned) ClusterPool,

I want ClusterClaim to give me an active cluster immediately in the common case,

So that I can get my work done more quickly.

(Ideally without sacrificing the cost savings of having some clusters in the pool hibernating.)

## Proposals

Several implementation alternatives have already come to mind.
These are not necessarily mutually exclusive.

### ClusterClaim Prioritizes Active Clusters

For most/all of the options below, it makes sense to add logic to ClusterClaim to preferentially grab an Active cluster first.
The simplest implementation is to sort the list of ClusterDeployments into three buckets: Active, Resuming, and Hiberating, claimed in that order.
A smarter implementation could take into account when Resuming clusters were asked to resume, with the most recent at the end, on the theory that the least-recently-resumed will be ready first.

### ClusterClaim Must Account For `hibernateAfter`

This applies to most/all of the options below.

If the ClusterPool has `hibernateAfter` set, that clock starts running as soon as the ClusterDeployment is ready.
(TODO: Confirm this -- does the clock start at creation, or `installed == True`, or something else?)
If we don't handle this properly for Active pool members, your cluster might go to sleep unexpectedly while you're working (Narcoleptic Cluster Syndrome).

**Options:**
- Reset the `hibernateAfter` clock at claim time. (How? Is this something hive controls?)
- Make `hibernateAfter` mutually exclusive with whatever solution(s) we pick below.
  (I think we would have to do this with a validation webhook.)

### Don't Hibernate New ClusterDeployments

Today when ClusterDeployments are created to fill out the pool, they are created with `PowerState = HibernatingClusterPowerState`.
Add an option to ClusterPool to not do that.

**Pros:**
- Dead simple

**Cons:**
- In periods of low activity, you're burning money by having Active vs. Hibernating clusters.
  (I suppose you could manually hibernate clusters in the pool. Not the best UX.)

### `activeCount`/`activePercent`

Add knob(s) to ClusterPool indicating a portion of the pool to maintain in Active state.
Either or both (if both, they should be mutually exclusive, or one should take precedence) of:

- `activeCount`: Integer absolute number of Active clusters to strive for.
  Constrained to `0 (default) <= activeCount <= (Max?)Size`.
- `activePercent`: Float `0.0 (default) <= activePercent <= 100.0` percentage of `(Max?)Size` clusters to keep Active.
  (How should we round this?)

Either way, this translates to an integer number of clusters the ClusterPool controller will try to keep Active.
- When creating a new cluster for the pool, set `PowerState = HibernatingClusterPowerState` iff we're at/above that threshold.
- At the end of ClusterPool's `Reconcile`, hibernate or resume the appropriate number of clusters to achieve the desired Active count.

**Pros:**
- The power. The absolute power.

**Cons:**
- Complexity. This could be tricky to get right in code.
  I can see hibernating/resuming clusters unnecessarily (Yo-Yo Syndrome).

## Risks and Mitigations

## Test Plan

## Alternatives
