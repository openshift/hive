# Module atlas

## Responsibility

Provides a controller that registers provisioned OpenShift clusters with ArgoCD by creating and maintaining ArgoCD cluster secrets. When a ClusterDeployment is installed, the controller generates an ArgoCD-compatible cluster secret in the ArgoCD namespace containing the API URL, cluster name, bearer token, and TLS configuration. On ClusterDeployment deletion, the controller cleans up the ArgoCD secret using a finalizer (`hive.openshift.io/argocd-cluster`). The controller is only active when the `ARGO_CD_ENABLED` environment variable is set.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to a manager with a given reconciler.
- `NewReconciler` -- Returns a new reconciler instance.
- `ControllerName` -- Constant `"argocdregister"`.
- `ArgoCDRegisterController` -- Reconciler struct; reconciles ClusterDeployments to create/update/delete ArgoCD cluster secrets.
- `ArgoCDRegisterController.Reconcile` -- Main reconcile loop. Skips if ArgoCD is not enabled, cluster is not installed, or is paused.
- `ClusterConfig` -- Subset of go-client rest.Config for ArgoCD cluster config serialization (bearer token + TLS).
- `TLSClientConfig` -- TLS settings copied from ArgoCD types for serialization without vendoring ArgoCD.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment CR and finalizer constants.
- `github.com/openshift/hive/pkg/constants` -- Environment variable names for ArgoCD integration.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.
- `github.com/openshift/hive/pkg/controller/utils` -- Client wrappers, log helpers, secret loading, controller config.

## Capabilities

- Reconciles: **ClusterDeployment**.
- Watches: ClusterDeployment (with rate-limited error update handler).
- Conditions set: None (uses finalizer `hive.openshift.io/argocd-cluster` for cleanup).
- Key logic: On installed CD, generates a predictable secret name from API URL hash, creates/updates an ArgoCD cluster secret with labels copied from the CD, adds finalizer. On CD deletion, deletes the ArgoCD secret and removes the finalizer.

## Understanding Score

0.85
