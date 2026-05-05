# Module: pkg/operator

## Responsibility

Top-level registration point for the Hive operator's controllers. Maintains a slice of controller-add functions (`AddToOperatorFuncs`) and provides `AddToOperator` to iterate through them and register each controller with the operator's controller-runtime Manager. The `init()` function in `add_hive.go` populates the slice with the two operator sub-controllers: `hive.Add` (HiveConfig reconciler) and `metrics.Add` (metrics calculator).

## Public Interface/API

- `AddToOperatorFuncs []func(manager.Manager) error` -- package-level variable; a list of functions that each add a controller to the Manager.
- `AddToOperator(m manager.Manager) error` -- iterates `AddToOperatorFuncs`, calling each to register controllers. Returns on first error.

## Internal Dependencies

- `pkg/operator/hive` -- the HiveConfig reconciler controller
- `pkg/operator/metrics` -- the metrics calculator controller
- `sigs.k8s.io/controller-runtime/pkg/manager` -- Manager interface

## Capabilities

- **Package**: `operator`
- Two source files: `controller.go` (AddToOperator function + variable), `add_hive.go` (init registration).
- 3 unique import paths.

## Understanding Score

0.92
