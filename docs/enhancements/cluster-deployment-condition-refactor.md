# Hive ClusterDeployment Conditions Refactor

## Summary

Reduce the number of conditions on ClusterDeployment and provide callers better options to monitor progress.

## Motivation

ClusterDeployment now carries over 20 conditions by default, from a variety of controllers. This is difficult for users to parse and there is not presently one condition that can be monitored for overall progress of an install.

## Goals

  1. Reduce the number of conditions on ClusterDeployment.
  1. Provide callers high level conditions they can monitor.


## Background

Current 22 conditions:

Unreachable
ClusterImageSetNotFound
ControlPlaneCertificateNotFound
Hibernating
RelocationFailed
SyncSetFailed
AWSPrivateLinkFailed
AWSPrivateLinkReady
ActiveAPIURLOverride
AuthenticationFailure
ClusterInstallCompleted
ClusterInstallFailed
ClusterInstallRequirementsMet
ClusterInstallStopped
DNSNotReady
DeprovisionLaunchError
IngressCertificateNotFound
InstallImagesNotResolved
InstallLaunchError
InstallerImageResolutionFailed
ProvisionFailed
ProvisionStopped

## Proposal

Focusing only on the 12 conditions controlled by the clusterdeployment controller:

ClusterImageSetNotFound - Move to RequirementsMet
AuthenticationFailure - Move to RequirementsMet
ClusterInstallCompleted - Merge into Provisioned.
ClusterInstallFailed - Merge into ProvisionFailed.
ClusterInstallRequirementsMet - Merge into RequirementsMet.
ClusterInstallStopped - Merge into ProvisionStopped.
DNSNotReady - Move to RequirementsMet
DeprovisionLaunchError -  Rename to DeprovisionError as a general bucket for any deprovision problems we can encounter.
InstallLaunchError - Move to ProvisionFailed
InstallerImageResolutionFailed - Move to ProvisionFailed
ProvisionFailed - Keep as a primary condition.
ProvisionStopped - Keep as a primary condition.

Implied here is a new utility to check if a grouped condition reason is ours, and if we know we have now cleared that state, switch the condition back to false.


### RequirementsMet

Indicates a problem with pre-flight checks. Unknown until we're ready to launch an install, then True. False if we've found a problem.

Can be set via clusterdeployment_controller up until we hand over to a ClusterInstall, or when we detect a RequirementsMet condition on the ClusterInstall. Would need to ensure that the code that copies from ClusterInstall -> ClusterDeployment only runs after we've cleared all our possible Reasons for this condition to be False in the clusterdeployment controller. ClusterDeployment controller should never set to True if we're using a ClusterInstallRef, leave unknown until the ClusterInstall says so.

Possible Reasons when False from clusterdeployment_controller:
  - ClusterImageSetNotFound
  - AuthenticationFailure
  - DNSNotReady
  - InstallerImageResolutionFailed
  - InstallLaunchError

Each of the above should use a utility to ensure they are not the reason the condition is False, once we know that problem is resolved. (set status back to Unknown)

### ProvisionFailed

Indicates a problem provisioning the cluster, and only used when we encounter an error after all RequirementsMet. (i.e. ClusterProvision failures, or problems from a ClusterInstall)

State is unknown until we hit our first error, true as soon as we detect any problem, false once we successfully install.
This condition is only used for errors

### ProvisionStopped

True when the ClusterInstall implementation has given up. (via Stopped condition on ClusterInstall)

### Provisioned

Net new condition, true once cd.Spec.Installed = true. Allows callers to oc wait.

