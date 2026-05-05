# Module atlas

## Responsibility

Provides a controller that syncs custom ingress configuration to remote clusters. When a ClusterDeployment specifies `spec.ingress` entries, the controller creates a SyncSet containing IngressController resources that are applied to the remote cluster to configure additional ingress controllers beyond the default.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to a manager.
- `NewReconciler` -- Returns a new reconciler.
- `ControllerName` -- Constant `"remoteingress"`.
- `GenerateRemoteIngressSyncSetName` -- Generates a predictable SyncSet name for a given CD.
- `ReconcileRemoteClusterIngress` -- Reconciler struct.
- `ReconcileRemoteClusterIngress.Reconcile` -- Creates/updates SyncSets for remote ingress configuration.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, SyncSet CRs.
- `github.com/openshift/hive/apis/helpers` -- API helper utilities.
- `github.com/openshift/api/operator/v1` -- IngressController types.
- `github.com/openshift/hive/pkg/constants` -- Environment variable names, label keys.
- `github.com/openshift/hive/pkg/resource` -- Resource helper.
- `github.com/openshift/hive/pkg/controller/utils` -- Conditions, owner references, controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.
- `github.com/openshift/hive/pkg/util/labels` -- Label management utilities.

## Capabilities

- Reconciles: **ClusterDeployment** (installed, with ingress entries).
- Watches: ClusterDeployment.
- Conditions set: None.
- Key logic: For each ingress entry in `cd.spec.ingress`, generates an IngressController resource. Creates a SyncSet named `{cd-name}-remote-ingress` with owner reference to the CD. Handles the default IngressController update separately. Uses content hash to avoid unnecessary updates.

## Understanding Score

0.80
