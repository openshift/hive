# Module atlas

## Responsibility

Defines a `Manager` interface that re-exports `controller-runtime/pkg/manager.Manager`, solely to serve as a source interface for gomock code generation. The generated mock lives in the `mock` sub-package.

## Public Interface/API

- `type Manager interface` -- embeds `manager.Manager`; exists only for the `//go:generate mockgen` directive

## Internal Dependencies

- `sigs.k8s.io/controller-runtime/pkg/manager`

## Capabilities

- **Package**: `manager`
- Code-generation anchor; no runtime logic

## Understanding Score

0.9
