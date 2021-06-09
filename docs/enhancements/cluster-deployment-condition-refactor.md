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

ClusterImageSetNotFound - Move to RequirementsMet or possibly ClusterInstallRequirementsMet.
AuthenticationFailure - Move to RequirementsMet or ClusterInstallRequirementsMet.
ClusterInstallCompleted - Merge into Installed.
ClusterInstallFailed - Merge into ProvisionFailed.
ClusterInstallRequirementsMet - Merge into RequirementsMet.
ClusterInstallStopped - Merge into ProvisionStopped.
DNSNotReady - Move to RequirementsMet
DeprovisionLaunchError -  Rename to DeprovisionError as a general bucket for any deprovision problems we can encounter.
InstallLaunchError - Move to RequirementsMet or ProvisionFailed
InstallerImageResolutionFailed - Move to RequirementsMet or ProvisionFailed
ProvisionFailed - Keep as a primary condition.
ProvisionStopped - Keep as a primary condition.

Implied here is a new utility to check if a grouped condition reason is ours, and if we know we have now cleared that state, switch the condition back to false.


### RequirementsMet

Unknown until we're ready to launch an install, then True. False if we've found a problem.

Can be set via clusterdeployment_controller up until we hand over to a ClusterInstall, or when we detect a RequirementsMet condition on the ClusterInstall. Would need to ensure that the code that copies from ClusterInstall -> ClusterDeployment only runs after we've cleared all our possible Reasons for this condition to be False in the clusterdeployment controller.

Possible Reasons when False from clusterdeployment_controller:
  - ClusterImageSetNotFound
  - AuthenticationFailure
  - DNSNotReady
  - InstallerImageResolutionFailed
  - InstallLaunchError

TODO: should RequirementsMet status=False reasons also translate to a ProvisionFailed = True with the same Reason?

Each of the above should use a utility to ensure they are not the reason the condition is False, once we know that problem is resolved. (set status back to Unknown)

ProvisionStopped - True if we've given up. Normally should just be set via the ClusterInstall's ProvisionStopped condition.

### ProvisionFailed

Unknown until we hit our first error, true as soon as we detect any problem, false once we successfully install.

TODO: per above, should we set this for preflight'ish requirements met errors? Would give callers one place to watch to know something is wrong.

### ProvisionStopped

True when the ClusterInstall implementation has given up. (via Stopped condition on ClusterInstall)

### Provisioned

Net new condition, true once cd.Spec.Installed = true. Allows callers to oc wait.

