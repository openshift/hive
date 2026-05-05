# Module atlas

## Responsibility

The central controller for the ClusterDeployment lifecycle. This is the largest and most complex controller in Hive. It manages provisioning new clusters (creating ClusterProvision objects, managing install jobs), deprovisioning deleted clusters (creating ClusterDeprovision objects, managing uninstall jobs), DNS zone management, release image verification, install-config validation, ClusterImageSet resolution, platform credential validation, ClusterInstall contract support, customization references, metadata.json secret generation, and post-install tasks like kubeconfig CA augmentation and delete-after annotation handling.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager. Reads metrics config and registers metrics.
- `AddToManager` -- Adds the controller with watches for many related resources.
- `NewReconciler` -- Returns a new reconciler with support for release image verification, protected delete, and supported contracts configuration.
- `ControllerName` -- Constant referencing `hivev1.ClusterDeploymentControllerName`.
- `ReconcileClusterDeployment` -- Main reconciler struct with expectations, remote client builder, credential validation, release image verifier, and supported contracts config.
- `ReconcileClusterDeployment.Reconcile` -- Main reconcile loop handling the full CD lifecycle.
- `ReconcileClusterDeployment.SetWatcher` -- Injects a watcher for dynamic watch registration (used by ClusterInstall contracts).
- `ValidateInstallConfig` -- Validates install-config.yaml consistency with CD platform spec.
- `LoadReleaseImageVerifier` -- Loads release image signature verification config.
- `ClusterProvisionManager` -- Interface for managing ClusterProvision objects.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, ClusterProvision, ClusterDeprovision, DNSZone, ClusterImageSet, ClusterDeploymentCustomization CRs.
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1` -- ClusterSync, FakeClusterInstall.
- `github.com/openshift/hive/apis/hivecontracts/v1alpha1` -- ClusterInstall contract types.
- `github.com/openshift/hive/pkg/controller/utils` -- Extensive use of conditions, finalizers, expectations, owner references, secret handling, credential validation.
- `github.com/openshift/hive/pkg/controller/utils/vsphereutils` -- vSphere deprecated field conversion.
- `github.com/openshift/hive/pkg/install` -- Install/uninstall job generation.
- `github.com/openshift/hive/pkg/imageset` -- ClusterImageSet resolution.
- `github.com/openshift/hive/pkg/remoteclient` -- Remote cluster API client for post-install operations.
- `github.com/openshift/installer/pkg/types` -- Install config types for validation.

## Capabilities

- Reconciles: **ClusterDeployment**.
- Watches: ClusterDeployment, ClusterProvision, Jobs (owner), Pods (selector), ClusterDeprovision (owner), DNSZone (owner), ClusterSync (owner), ClusterDeploymentCustomization (mapped).
- Conditions set: `DNSNotReadyCondition`, `InstallImagesNotResolvedCondition`, `ProvisionFailedCondition`, `SyncSetFailedCondition`, `InstallLaunchErrorCondition`, `DeprovisionLaunchErrorCondition`, `ProvisionStoppedCondition`, `AuthenticationFailureClusterDeploymentCondition`, `RequirementsMetCondition`, `ProvisionedCondition`, plus ClusterInstall-mirrored conditions.
- Finalizer: `hive.openshift.io/deprovision`.
- Key logic: Sets platform/region labels, manages DNS zones, resolves ClusterImageSets, validates install-config, creates ClusterProvision for installs (max 3 attempts by default), creates ClusterDeprovision for deletes, handles delete-after TTL, manages metadata.json secrets, supports ClusterInstall contracts for alternative install methods.

## Understanding Score

0.80
