# Hive InstallStrategy to ClusterInstall API Refactor

## Summary

Break out ClusterDeployment.Spec.Provisioning.InstallStrategy and allow for multiple ClusterInstall CRD implementations that adhere to an interface, where Hive interacts with these CRDs dynamically via duck typing in the non-install related controllers.


## Motivation

Hive is presently struggling with how to design the API to meet the needs of provisioning standalone, agent based, and hypershift clusters. The current code base evolved quite specific to running openshift-install pods and has some growing pains to accomodate new paths to provision clusters. Changes for one raise a lot of questions and inconsistencies for the others as everything is lumped into ClusterDeployment. The line between day 1 and day 2 is very unclear and mostly just enforced via increasingly complicated validating webhooks.

### Goals

- Clarify Hive APIs. (day 1 vs day 2)
- Breakout clusterDeployment.spec.installStrategy.agent to a separate AgentClusterInstall CRD. This is still guarded by an alpha feature gate and thus we should still be able to move it around.
- Provide space for implementations to define the spec and status they need without causing uncomfortable complications for other implementations.
- Do not break API compatability for existing standalone OpenShift cluster provisioning. We hope to slowly migrate those installs to this system by offering it in parallel and eventually deprecating the old, but for now the focus is on breaking out the AgentClusterInstall CRD.
- Plan for a future of integration of ClusterInstalls with ClusterPools. (see Future Considerations)

### Non-Goals

- Replace ClusterProvision. This CRD represents an attempt to install a standalone cluster, it's highly specific to running openshift-install in a pod, and we do not want to modify anything in this area. Even if we move standalone provisions to an OpenShiftClusterInstall, ClusterProvision will likely be used beneath it as the retry mechanism.
- Implementing retries within Hive itself. Retries will be left to specific implementations of ClusterInstall interface, if desired.
- Implementing ClusterPool support for ClusterInstalls. (see Future Considerations)

## Proposal

Introduce a new ClusterInstall CRD interface/contract in Hive API, breaking out of the current ClusterDeployment.Spec and instead linking to one and use duck typing to interact. Initial implementation would be AgentClusterInstall. In the future we would hopefully migrate current openshift-install pods to OpenShiftClusterInstall, and add HypershiftClusterInstall, etc.

Hive ClusterInstall implementations are a day 1 provisioning API. They would adhere to a contract with hive for what fields/conditions they must contain and update as the installation proceeds. Hive would copy relevant data back to the ClusterDeployment as appropriate at various stages of the provisioning.

ClusterDeployment becomes a central index of clusters and kubeconfigs, used for day 2 management.

AgentClusterInstall becomes a day 1 resource and will offer more flexibility for the agent controllers to define settings they need in isolation, and track their relevant status.


### User Stories

#### Story 1

#### Story 2

## Design Details


### ClusterDeployment Changes

ClusterDeployment should now contain only information needed to connect to the cluster and settings for day 2 management. (admin kubeconfig, admin password, ingress, control plane certs, etc) Platform section will remain for information we need, including cloud provider credentials for things like hibernation and managed DNS.

ClusterDeployment.Spec.InstallStrategy goes away, and is instead replaced with ClusterInstallRef, which would look like the following:

```yaml
  clusterInstallRef:
    apiVersion: hive.openshift.io/v1
    kind: AgentClusterInstall
    name: mycluster
```

ClusterInstallRef is optional, it would not be specified for an adopted cluster.

Hive controllers would then interact with this ClusterInstall via duck typing, similar to Cluster API, Knative, etc. This is likely only necessary in the Hive clusterdeployment_controller so it can watch, and know when to copy information back to the cluster deployment.

ClusterDeployment.Spec.Provisioning remains but will eventually be deprecated. Validate this is not used in conjunction with ClusterDeployment.Spec.ClusterInstallRef, must choose one path or the other.

### ClusterInstall Interface


Field | Type | Description
----- | ---- | -----------
spec.clusterDeploymentRef | LocalObjectReference | Link to the required ClusterDeployment for this install. Bidirectional reference. Common data should live on ClusterDeployment and never be duplicated on ClusterInstall.
spec.imageSetRef | ClusterImageSetReference | Core mechanism in Hive for choosing what version of OpenShift to install.
spec.clusterMetadata.adminKubeconfigSecretRef | LocalObjectReference | Admin kubeconfig secret for current install attempt underway, set as soon as known. Copied back to Hive automatically once Ready.
spec.clusterMetadata.adminPasswordSecretRef | LocalObjectReference | Admin password secret for current install attempt underway, set as soon as known. Copied back to Hive automatically once Ready.
spec.clusterMetadata.clusterID | string | Cluster UUID generated during install, set as soon as known. Copied back to Hive automatically once Ready.
spec.clusterMetadata.infraID | string | Cluster infra ID generated during install, set as soon as known. Copied back to Hive automatically once Ready.
status.installRestarts | int | Current number of install retries.
status.conditions | []ClusterInstallCondition | Standard conditions array. See below for required conditions.

The following ClusterDeployment fields are not carrying over to this interface as they are not generic enough to assume for all ClusterInstall implementations. They can of course be used by specific stratgies, but Hive will not need to assume they are there.

  * SSHPrivateKeySecretRef: Some install strategies may not want to support this.
  * SSHKnownHosts
  * InstallEnv: Too specific to openshift-install.
  * InstallConfigSecretRef: One of the more contentious parts of the current API.
  * ReleaseImage: Legacy Hive feature, use ClusterImageSet instead.
  * ManifestsConfigMapRef: Too specific to openshift-install.

All conditions should initialize to Unknown status as soon as possible per latest Kubernetes API guidelines, and then be set to True/False once more explicit status is known.

Condition | Description
--------- | -----------
RequirementsMet | True once all pre-install requirements have been met. False if we hit a problem and can't start. Reasons: ClusterImageSetNotFound, ClusterDeploymentNotFound, etc.
Completed | True once the cluster install has successfully completed. Reason and Message provide as much detail as possible for the user as install progresses. Message could contain attempt x of y if the ClusterInstall support retries.
Failed | True once an install attempt has failed. Does not imply we've stopped trying, if retries are supported by the controller we may be re-trying. Returns to False if we later reach Completed=True.
Stopped | True once the controller is no longer working to reach desired state. Does not imply success or failure by itself, but can be combined with Completed/Failed.

These interface conditions will be copied back onto ClusterDeployment by Hive controllers with a "ClusterInstall" prefix, the ClusterInstall controller does not need to worry about this and in general should never write to ClusterDeployment. example: Failed becomes ClusterInstallFailed (which exists today on ClusterDeployment for this purpose).

In the case of ClusterInstallFailed, we will also copy this condition to ProvisionFailed to maintain the current contract and not affect callers monitoring for this condition.

ClusterInstall controllers can add their own conditions, but these will not transfer back to the ClusterDeployment. UIs around this process may need to show ClusterInstall conditions explicitly.

It will be possible for a ClusterInstall CRD and it's controllers to be entirely external to Hive.

#### External ClusterInstall RBAC

To support an external application providing their own ClusterInstall CRD implementation, Hive will need RBAC to be able to access those objects. Hive will ship with a role granting full access to the apigroup "extensions.hive.openshift.io". CRDs which implement the interface should use this apigroup regardless if they are in Hive or external.

#### Registering a ClusterInstall Implementation

For a CRD to be accepted as a valid ClusterInstall implementation it must be labeled with `contracts.hive.openshift.io/clusterinstall: "true"`. The Hive operator will watch for CRDs with this label and configure an admission webhook to only allow valid implementations to be referenced by a ClusterDeployment.

### Cluster Deprovisioning

Today in Hive each ClusterDeployment is given a finalizer automatically, and when the API resource is deleted we create a ClusterDeprovision resource, which launches a pod to run the openshift-install deprovision process until it completes, after which the finalizer is removed and the ClusterDeployment is removed.

There is presently no deprovision implementation for agent based or bare metal clusters but there likely will be in the future.

It stands to reason that deprovision implementations are fully tied to ClusterInstall implementations, and thus deleting the ClusterInstall should trigger the controllers for that ClusterInstall to complete an uninstall/deprovision. *WARNING: Deleting a ClusterInstall will destroy the cluster.*

Hive will add an OwnerReference from ClusterInstall CR to the owning ClusterDeployment, so deleting a ClusterDeployment will cascade to delete the ClusterInstall.

ClusterInstall controllers who wish to take action on deprovision will add their own finalizer to their ClusterInstall CRs and use this to ensure their deprovision steps complete successfully. The normal Hive ClusterDeployment hive.openshift.io/deprovision finalizer will be skipped for ClusterDeployments which have a ClusterInstallRef.

### Migration Plan

No breakage for current installation API is planned. (with exception of as yet unreleased Agent install strategy)

Initial goal would be to breakout ClusterDeployment.Spec.InstallStrategy.Agent. The only implementation here is agent based and this is hidden behind an alpha feature gate, and not yet released to customers. Replace this with a new hive.openshift.io.AgentClusterInstall CRD, and a ClusterDeployment.Spec.ClusterInstallRef link. Refactor the Agent controllers (currently living in the assisted service repo, coming to hive in future) to watch this resource instead of ClusterDeployment.


### Examples

```yaml
apiVersion: hive.openshift.io/v1
kind: AgentClusterInstall
metadata:
  name: mycluster
  namespace: hive
spec:
  clusterDeploymentRef:
    name: mycluster
  imageSetRef:
    openshift-4.6
  clusterMetadata:
    # set as soon as known even if install is not completed:
    adminKubeconfigSecretRef:
      name: mycluster-3-2sw49-admin-kubeconfig
    adminPasswordSecretRef:
      name: mycluster-3-2sw49-admin-password
    clusterID: 48dec7cf-0c4b-44c6-b840-5c8a6ec93aa6
    infraID: mycluster-4n2t5

  # agent specific
  networking:
    machineNetwork: ...
  sshPublicKey: ...
  provisionRequirements:
    controlPlaneAgents: 3
    workerAgents: 5
  controlPlane:
    hyperthreading: Disabled
  compute:
    hyperthreading: Disabled

  status:
    conditions:
    - lastProbeTime: "2021-04-15T15:50:59Z"
      lastTransitionTime: "2021-04-15T15:50:59Z"
      message: Provision attempt 2 of 3 underway.
      reason: ProvisionRetryUnderway
      status: "False"
      type: Ready
    - lastProbeTime: "2021-04-15T15:50:59Z"
      lastTransitionTime: "2021-04-15T15:50:59Z"
      message: All requirements met.
      reason: AllRequirementsMet
      status: "True"
      type: RequirementsMet
    - lastProbeTime: "2021-04-15T15:50:59Z"
      lastTransitionTime: "2021-04-15T15:50:59Z"
      message: Quota exceeded
      reason: SomeQuotaExceeded
      status: "True"
      type: Failed
    - lastProbeTime: "2021-04-15T15:50:59Z"
      lastTransitionTime: "2021-04-15T15:50:59Z"
      message: Attempt x 2 of 3 underway.
      reason: ProvisionUnderway
      status: "True"
      type: Retrying

    - lastProbeTime: "2021-04-15T15:50:59Z"
      lastTransitionTime: "2021-04-15T15:50:59Z"
      message: Some agent specific condition.
      reason: AgentReason
      status: "True"
      type: AgentSpecificCondition
```

### Future Considerations

#### OpenShiftClusterInstall

Once the concept is proved with AgentClusterInstall, we would want to soon move the current openshift-install ClusterProvision standalone implementation to an OpenShiftClusterInstall CRD to match this flow. Existing ClusterDeployment Provisioning section would be deprecated over time, and not usable in conjunction with a ClusterInstallRef. Eventually the Provisioning section would be removed entirely

#### HypershiftClusterInstall

Hypershift is likely to be our next integration point and would thus benefit from having a HyperShiftClusterInstall CRD on which to define settings needed to install a Hypershift cluster.

#### NoInstallConfigOpenShiftClusterInstall

We've pondered returning to supporting a way to provision ClusterDeployments without requiring users to specify their own InstallConfig Secret and instead offering a fully defined Kubernetes API and generating the install-config as an implementation detail. This could be added as a separate ClusterInstall implementation for those who would like a simpler interface.

#### ClusterPool Support

ClusterInstall implementations could in the future be used in conjunction with ClusterPools. ClusterPool would gain a ClusterInstallRef. Hive will copy this ClusterInstall to the namespace of the ClusterDeployment when created and associate them together.

The requirement here would be that for a ClusterInstall to *function* with ClusterPools, it must not contain anything cluster specific. It can however contain configuration for how to obtain cluster specific information. Hive itself does not care about any of this, and controllers to fullfill that ClusterInstall can be implented outside of Hive. Hive will manage the pool, create new ClusterDeployments as necessary and fulfill claims.

For an example consider our outstanding request to support VMWare cluster pools, but with arbitrary DNS configurations that could change depending on use case. In this scenario an external entity could implement MyCustomVMWareInstall. That CRD could define configuration to obtain DNS from Route53 or other, and then generate an appropriate InstallConfig.

This idea would need to be fleshed out more in the future when the time came to implement.


### Risks and Mitigations

  * Can Hive controllers watch a dynamic set of types to reconcile when a ClusterInstall changes? Can we watch unstructured?

### Test Plan

## Alternatives

### EC2instance in user account to run installer

### installer that can create infrastructure without waiting

### New managed NLB instead of using installer created one
