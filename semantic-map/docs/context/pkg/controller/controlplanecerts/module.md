# Module atlas

## Responsibility

Provides a controller that syncs custom control plane TLS certificates to remote clusters. When a ClusterDeployment specifies `spec.certificateBundles`, the controller creates a SyncSet containing the certificate secrets that need to be synced to the remote cluster, along with a serving-cert update for the cluster's API server and default ingress controller.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to a manager.
- `NewReconciler` -- Returns a new reconciler.
- `ControllerName` -- Constant referencing `hivev1.ControlPlaneCertsControllerName`.
- `GenerateControlPlaneCertsSyncSetName` -- Generates a predictable SyncSet name for a given CD.
- `ReconcileControlPlaneCerts` -- Reconciler struct.
- `ReconcileControlPlaneCerts.Reconcile` -- Creates/updates SyncSets for control plane certificate bundles.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, SyncSet CRs.
- `github.com/openshift/hive/apis/helpers` -- API helper utilities.
- `github.com/openshift/hive/pkg/constants` -- Environment variable names, label keys.
- `github.com/openshift/hive/pkg/remoteclient` -- Remote cluster client (for detecting existing serving certs).
- `github.com/openshift/hive/pkg/resource` -- Resource helper.
- `github.com/openshift/hive/pkg/controller/utils` -- Conditions, controller config, owner references.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.
- `github.com/openshift/hive/pkg/util/labels` -- Label management utilities.

## Capabilities

- Reconciles: **ClusterDeployment** (installed only, with certificate bundles).
- Watches: ClusterDeployment.
- Conditions set: None.
- Key logic: For each certificate bundle in `cd.spec.certificateBundles`, reads the referenced secret, generates SyncSet resources including the certificate secret and APIServer/IngressController patches. Creates or updates a single SyncSet named `{cd-name}-cp-certs` with owner reference to the CD.

## Understanding Score

0.80
