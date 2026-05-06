# Module atlas

## Responsibility

Registry for Hive operator controllers. Collects controller add-functions and registers them with a controller-runtime manager. The init function registers the hive and metrics controllers.

## Public Interface/API

- `AddToOperatorFuncs` var `[]func(manager.Manager) error` -- list of controller registration functions
- `AddToOperator(m manager.Manager) error` -- iterates AddToOperatorFuncs and adds all controllers to the manager

## Internal Dependencies

- `github.com/openshift/hive/pkg/operator/hive`
- `github.com/openshift/hive/pkg/operator/metrics`
- `sigs.k8s.io/controller-runtime/pkg/manager`

## Capabilities

- Aggregates operator controller registration functions into a single list
- Registers hive reconciler and metrics controllers via init()

## Understanding Score

0.9
