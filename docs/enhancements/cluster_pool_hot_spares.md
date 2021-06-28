# Maintaining Running Clusters in a ClusterPool

[HIVE-1576](https://issues.redhat.com/browse/HIVE-1576)

- [Maintaining Running Clusters in a ClusterPool](#maintaining-running-clusters-in-a-clusterpool)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [User Story](#user-story)
  - [Proposal](#proposal)
    - [`spec.runningCount`](#specrunningcount)
    - [Decouple PowerState on Creation](#decouple-powerstate-on-creation)
    - [ClusterClaim Assignment by `creationTimestamp`](#clusterclaim-assignment-by-creationtimestamp)
    - [Hibernation Controller Must Account For `hibernateAfter` in Pool Clusters](#hibernation-controller-must-account-for-hibernateafter-in-pool-clusters)
    - [A Note About Tuning](#a-note-about-tuning)
  - [Alternatives](#alternatives)
    - [Don't Hibernate New ClusterDeployments](#dont-hibernate-new-clusterdeployments)
    - [`runningPercent`](#runningpercent)

## Summary

Support configuring ClusterPools to maintain some number of clusters (`0 <= N <= spec.size`) in Running state.

## Motivation

Today ClusterPools immediately hibernate new clusters.
This means a ClusterClaim is guaranteed to suffer at least Resuming time before the cluster is available for use.
While this is still faster than having to create a cluster from scratch, it is a potentially-unnecessary delay.
As an example, CI runs could be reduced by ~5m if a ClusterClaim were able to grab a Running cluster rather than having to Resume a Hibernating one.

### Goals

Ability to tune a ClusterPool such that a ClusterClaim gets a Running cluster immediately in the common case.

(Stretch goal) Solving world peace.

### User Story

As a consumer of a ([properly tuned](#a-note-about-tuning)) ClusterPool,

I want ClusterClaim to give me a Running cluster immediately in the common case,

So that I can get my work done more quickly.

(Ideally without sacrificing the cost savings of having some clusters in the pool Hibernating.)

## Proposal

### `spec.runningCount`

Add a field to ClusterPool indicating a portion of the pool to maintain in Running state.
`spec.runningCount` is an integer absolute number of Running clusters to strive for.
It is constrained to `0 <= runningCount <= Size`.
(We could enforce this by webhook; or we could update the spec accordingly (explicitly set `runningCount = min(runningCount, size)`); but instead we just follow the pattern of the other limits and use the min of `runningCount` and `size` where appropriate.)

If unspecified, the default behavior is to hibernate all CDs in the pool, as today.
This corresponds to `runningCount=0`, conveniently the golang nil value of an int.

### Decouple PowerState on Creation

Today we create all new ClusterDeployments with `powerState=Hibernating` and update that to `Running` at claim time.
With this feature, we will create with `powerState` unset.
Then in a separate code path:
- Sort unassigned (installing + ready) CDs by `creationTimestamp`
- Update the oldest `runningCount` CDs with `powerState=Running`
- Update the remainder (the newest `size - runningCount`) with `powerState=Hibernating`

To be safe, and to continue correctly supporting pools with no/zero `runningCount`, we will still always set `powerState=Running` when assigning a claim.

(In both cases, we should only perform an actual `Update()` for a given CD if its state is changed.)

(Note: This means it is possible for a CD's PowerState to be updated several times even while it's installing.
That's fine -- though we should make sure the hibernation controller handles it sanely.)

### ClusterClaim Assignment by `creationTimestamp`

When determining which ClusterDeployments to assign to new claims, we can simply sort the list by `creationTimestamp`.
Since we're [prioritizing PowerState the same way](#decouple-powerstate-on-creation), this should serve to satisfy both:
- FIFO behavior. Oldest clusters will be claimed first.
- Minimum response time.
  We always activate the oldest cluster, and we always claim the oldest cluster, so the first claim will pick up the least-recently-resumed cluster, which is the most likely to be Running, or will be there the soonest.

Caveats:
- Due to variability in install or resume time, it is technically possible that there would be a "better" (faster) choice for a given assignment.
  But it'll be real close; and we prefer to code more strictly to the FIFO behavior.
- Manually resuming a pool CD will not cause it to be claimed earlier.
  In fact, it is subject to being re-hibernated by the controller.
  This is an acceptable design point: You can manage power states by configuring your ClusterPool; don't expect to be able to do so manually.

### Hibernation Controller Must Account For `hibernateAfter` in Pool Clusters

If the ClusterPool has `hibernateAfter` set, that clock starts on a given ClusterDeployment from the time its
[Hibernating condition is Running](https://github.com/openshift/hive/blob/28f67bab3dac5fb9dec405e0536d64187faf964d/pkg/controller/hibernation/hibernation_controller.go#L240-L244);
or, if the status of that condition is Unknown, from
[when installation completed](https://github.com/openshift/hive/blob/28f67bab3dac5fb9dec405e0536d64187faf964d/pkg/controller/hibernation/hibernation_controller.go#L229-L239).
If we don't account for this, Running unassigned CDs in the pool will hibernate, and then the [new logic](#decouple-powerstate-on-creation) will immediately wake them back up, resulting in unnecessary churn and potential claim delays.
So we'll:
- Add Status field to ClusterDeployment called `ClaimedTimestamp`, corollary to `InstalledTimestamp`.
  We'll set this to `Now()` at the same time we set the `Spec.ClusterPoolRef.ClaimName`.
  Like `InstalledTimestamp` (and `ClaimName`, for that matter), once set, it never changes.
- Add logic to the hibernation controller as follows:
  - If the CD is part of a ClusterPool
    - If it is unclaimed, ignore `hibernateAfter`* -- never hibernate.
    - If it is claimed, use as the base time the _later_ of
      - the `ClaimedTimestamp`
      - the existing algorithm's base time (this accounts for hibernating/resuming during the normal post-claim lifecycle of the CD).
  - Else (not part of a ClusterPool) do what we do today.

*This also means a manually-resumed unclaimed CD from a pool will never hibernate at the behest of `hibernateAfter`.
As noted [above](#clusterclaim-assignment-by-creationtimestamp), you shouldn't be trying to manage the PowerState of pool-owned CDs manually.
This goes double if you're using `hibernateAfter` with ClusterPools.

### A Note About Tuning

Proper tuning will vary wildly depending on the consumer, of course.
We're giving them the knob to tune with.
As a starting point, perhaps:
```
runningCount = round_up(claim_frequency * install_time)
```
For example, if your pool sees one claim every 15 minutes, and it takes 40 minutes to install a cluster:
```
runningCount = round_up(1/15 * 40) = 3
```
Further tuning would depend on e.g.
- How "smoothly" claims come in. If your average of 1/15m results from 11 per hour from 9a-5p and 8 total overnight, maybe you want to use the 11/h (`runningCount = 8`) to ensure you can handle peak demand.
- Whether your priority is cost savings or responsiveness. If the former, perhaps you want to lean towards the 0.5/h (`runningCount = 1`) -- or not use a `runningCount` at all.
- How much it "costs" you to retune the pool frequently. E.g. if you have the means for automation and a predictable usage curve, perhaps you configure a job to set `runningCount = 8` at 8:50am M-F and `runningCount = 0` at 5p M-F.


## Alternatives

The following alternatives were considered:

### Don't Hibernate New ClusterDeployments

Today when ClusterDeployments are created to fill out the pool, they are created with `PowerState = HibernatingClusterPowerState`.
Add an option to ClusterPool to not do that.

**Pros:**
- Dead simple

**Cons:**
- In periods of low activity, you're burning money by having Running vs. Hibernating clusters.
  (I suppose you could manually hibernate clusters in the pool. Not the best UX.)

### `runningPercent`

Use a percentage instead of an absolute number to indicate how many clusters in the pool should remain Running.

Quoting a conversation with [Gurney Buchanan](https://github.com/gurnben) (ACM):

> Eric Fried:
> The main decision being: by percentage or by count?
>
> Gurney Buchanan:
> Hmm - we use small clusterpools so I would wager count (percentages would get messy) - plus with count, you can know (without maths):
> - How many running instances you’ll have in the pool
> - How much vCPU you need available (for future vsphere pools)
> - Control cost
> - **Critically** you can control cost/number of cold spares without resizing the pools.  If a prod deploy needs 3 clusters for example and you know you might need to roll one at any time, you can keep 3 hot spares and as long as another deploy doesn’t have to happen in the next 5 minutes you’ll have hot spares for the next,  and the user can tune the size of the pool to ensure that they always have enough clusters ready for deploys to cycle.
>
> Yepp I’m convinced it should be count
>
> Eric Fried:
> I had been thinking percentages could be more convenient if you're frequently resizing the pool -- but compared to what you've said, the minor inconvenience of having to edit two fields instead of one when resizing is insignificant.
>
> Gurney Buchanan:
> That’s what I realized as I was typing up the use-case @Eric Fried :laugh1:
