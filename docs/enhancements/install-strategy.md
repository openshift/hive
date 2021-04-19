# Hive InstallStrategy to ClusterInstall API Refactor

## Summary

Break install strategy out of ClusterDeployment.Spec and allow for multiple ClusterInstall CRD implementations that adhere to an interface, where Hive interacts with these CRDs dynamically via duck typing in the non-install related controllers.


## Motivation

Hive is presently struggling with how to design the API to meet the needs of provisioning standalone, agent based, and hypershift clusters. The current code base evolved quite specific to running openshift-install pods and has some growing pains to accomodate new paths to provision clusters. Changes for one raise a lot of questions and inconsistencies for the others as everything is lumped into ClusterDeployment. The line between day 1 and day 2 is very unclear and mostly just enforced via increasingly complicated validating webhooks.

### Goals

- Clarify Hive APIs. (day 1 vs day 2)
- Breakout clusterDeployment.spec.installStrategy.agent to a separate AgentClusterInstall CRD. This is still guarded by an alpha feature gate and thus we should still be able to move it around.
- Provide space for implementations to define the spec and status they need without causing uncomfortable complications for other implementations.
- Do not break API compatability for existing standalone OpenShift cluster provisioning. We hope to slowly migrate those installs to this system by offering it in parallel and eventually deprecating the old, but for now the focus is on breaking out the AgentClusterInstall CRD.

### Non-Goals

- Replace ClusterProvision. This CRD represents an attempt to install a standalone cluster, it's highly specific to running openshift-install in a pod, and we do not want to modify anything in this area. Even if we move standalone provisions to an OpenShiftClusterInstall, ClusterProvision will likely be used beneath it as the retry mechanism.
- Implementing retries within Hive itself. Retries will be left to specific implementations of ClusterInstall interface, if desired.

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
spec.stage | string | Enum string for stage of installation. (Initializing, RequirementsNotMet, Provisioning, RetryProvisioning, Complete, Failed) More details in conditions. *TODO*: Should we just work with conditions? Loss of state in disaster recovery?
spec.clusterMetadata.adminKubeconfigSecretRef | LocalObjectReference | Admin kubeconfig secret for current install attempt underway, set as soon as known. Copied back to Hive automatically once Ready.
spec.clusterMetadata.adminPasswordSecretRef | LocalObjectReference | Admin password secret for current install attempt underway, set as soon as known. Copied back to Hive automatically once Ready.
spec.clusterMetadata.clusterID | string | Cluster UUID generated during install, set as soon as known. Copied back to Hive automatically once Ready.
spec.clusterMetadata.infraID | string | Cluster infra ID generated during install, set as soon as known. Copied back to Hive automatically once Ready.
spec.manifestsConfigMapRef | LocalObjectReference | ConfigMap containing manifests to inject into the cluster during installation. *TODO*: Hive technically doesn't need this to be there, do we require it to ensure consistent support or leave it up to install strategies?

The following ClusterDeployment fields are not carrying over to this interface as they are not generic enough to assume for all ClusterInstall implementations. They can of course be used by specific stratgies, but Hive will not need to assume they are there.

  * SSHPrivateKeySecretRef: Some install strategies may not want to support this.
  * SSHKnownHosts
  * InstallEnv: Too specific to openshift-install.
  * InstallConfigSecretRef: One of the more contentious parts of the current API.
  * ReleaseImage: Legacy Hive feature, use ClusterImageSet instead.

All conditions should initialize to Unknown status as soon as possible, and then be set to True/False once known.

Condition | Description
--------- | -----------
Ready | Important overall condition indicating if the cluster is ready or not. Reason and Message provide as much detail as possible for the user. Message could contain attempt x of y if the ClusterInstall support retries.
RequirementsMet | True once we are ready to launch the install. False if we hit a problem and can't start. Reasons: ClusterImageSetNotFound, etc.
Failed | True once an install attempt has failed. Remains true if retries are in play even if another attempt is in progress. Move to False once Ready goes to True. Reason/Message should contain details on what happened if it's possible to detect, otherwise Unknown. *TODO*: Unfortunate naming here, this matches current ProvisionFailed condition, which means a failure has been encountered, not necessarily that we've given up. Failing would be a better term, but it's risky to change the ProvisionFailed condition meaning on ClusterDeployment when we transfer. Thoughts?
Retrying | Unknown initially, True if we've hit an error and are retrying, False if we've given up or don't support retries. Message should indicate attempt x of y.

These interface conditions will be copied back onto ClusterDeployment by Hive controllers with a "Provision" prefix, the ClusterInstall controller does not need to worry about this and in general should never write to ClusterDeployment. example: Failed becomes ProvisionFailed (which exists today on ClusterDeployment for this purpose)

ClusterInstall controllers can add their own conditions, but these will not transfer back to the ClusterDeployment. UIs around this process may need to show ClusterInstall conditions explicitly.

The core ClusterInstall CRDs and controllers discussed in this document should live in Hive. However, in theory, it would be possible for an external application to implement it's own ClusterInstall outside of Hive and still interface with ClusterDeployment using this system.

### Migration Plan

No breakage for current installation API is planned. (with exception of as yet unreleased Agent install strategy)

Initial goal would be to breakout ClusterDeployment.Spec.InstallStrategy.Agent. The only implementation here is agent based and this is hidden behind an alpha feature gate, and not yet released to customers. Replace this with a new hive.openshift.io.AgentClusterInstall CRD, and a ClusterDeployment.Spec.ClusterInstallRef link. Refactor the Agent controllers (currently living in the assisted service repo, coming to hive in future) to watch this resource instead of ClusterDeployment.

In the future, we could migrate the current standalone openshift-install implementation to an OpenShiftClusterInstall, which continues to use ClusterProvision for it's retry mechanism. To do this we would allow a parallel path to specify where you cannot use both ClusterInstallRef and provisioning sections together. Provisioning then becomes deprecated and after some period of time, could be removed.

In the future we could add an additional implementation which offers InstallConfig generation, and could be used by users who are ok with a simpler interface and don't want to specify an install config secret.

HypershiftClusterInstall would be another likely future implementation.

### Examples

```yaml
apiVersion: hive.openshift.io/v1
kind: AgentClusterInstall
metadata:
  name: mycluster
  namespace: hive
spec:
  stage: RetryProvisioning
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


### Risks and Mitigations

  * Can Hive controllers watch a dynamic set of types to reconcile when a ClusterInstall changes? Can we watch unstructured?

### Test Plan

## Alternatives

### EC2instance in user account to run installer

### installer that can create infrastructure without waiting

### New managed NLB instead of using installer created one
