# Module atlas

## Responsibility

Exposes build-time version information for the Hive binary, populated via `-ldflags` at compile time. Provides the version as a structured `version.Info` or a human-readable string.

## Public Interface/API

- `Get() version.Info` -- returns structured version info (Major, Minor, GitCommit, GitVersion, GitTreeState, BuildDate)
- `String() string` -- returns `"openshift/hive <version>"` human-readable string

## Internal Dependencies

- `k8s.io/apimachinery/pkg/version` -- version.Info struct

## Capabilities

- Expose git commit, version tag, major/minor, build date, and tree state
- Linker-injected variables for build-time metadata

## Understanding Score

0.9
