# Module atlas

## Responsibility

Exposes build-time version metadata for the Hive binary. Version fields (commit, tag, major, minor, build date, git tree state) are set via `-ldflags` at compile time and packaged into the standard `k8s.io/apimachinery/pkg/version.Info` struct for consumption by version endpoints and logging.

## Public Interface/API

- `Get() version.Info` -- returns a `k8s.io/apimachinery/pkg/version.Info` struct populated with Major, Minor, GitCommit, GitVersion, GitTreeState, and BuildDate. All values default to empty strings (or "unknown" for GitVersion) if not injected at build time.
- `String() string` -- returns `"openshift/hive <versionFromGit>"`, a human-friendly version string.

Package-level variables (set via `-ldflags`, not exported):
- `commitFromGit`, `versionFromGit`, `majorFromGit`, `minorFromGit`, `buildDate`, `gitTreeState`

## Internal Dependencies

- `fmt` -- string formatting for `String()`.
- `k8s.io/apimachinery/pkg/version` -- `Info` struct type.

## Capabilities

- **`package`** name(s): **version**.
- Go **`import`** edges listed below (2 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/version`.
- Single file (`version.go`), no tests.

## Understanding Score

0.9
