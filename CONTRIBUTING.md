# Contributing to Hive

Thank you for your interest in contributing to OpenShift Hive! We welcome contributions from the community.

## Project Overview

OpenShift Hive is an operator which runs as a service on top of Kubernetes/OpenShift.
The Hive service can be used to provision and perform initial configuration of OpenShift clusters.

Hive uses the [OpenShift installer](https://github.com/openshift/installer) for cluster provisioning.

For detailed design overview and usage, please refer to the [README.md](README.md) and [documentation](./docs/).

## Getting Started

### Prerequisites

- Go (version specified in `go.mod`)
- Make

### Development Environment

You can build the project binaries using the provided `make` targets.

```bash
# Update generated code
make update

# Compile the project binaries
make build

# Clean up build artifacts
make clean
```

See the [Developing Hive](./docs/developing.md) guide for detailed setup instructions.

## Testing

Before submitting a Pull Request, ensure that all tests pass and the code is verified.

```bash
# Verify generated code and formatting
make verify

# Run unit tests (excludes e2e tests)
make test
```

 ### Dependency Management
```bash
make vendor   # Update vendor directory
make modcheck # Check module dependencies
make modfix   # Fix module dependencies
```

### Test Types

- **Unit Tests** (`make test`): Runs unit tests for `./pkg/...`, `./cmd/...`, `./contrib/...`, and submodules. This excludes e2e tests.
- **E2E Tests**: End-to-end tests require a running cluster and are run separately:
  - `make test-e2e`: Run full e2e test suite
  - `make test-e2e-pool`: Run cluster pool e2e tests
  - `make test-e2e-postdeploy`: Run post-deployment e2e tests
  - `make test-e2e-postinstall`: Run post-installation e2e tests

## Project Structure

Understanding the project structure will help you navigate the codebase:

- **`apis/`**: API definitions (separate Go submodule)
  - `hive/v1/`: Hive v1 APIs (ClusterDeployment, SyncSet, etc.)
  - `hiveinternal/v1alpha1/`: Internal APIs
  - `hivecontracts/v1alpha1/`: Contract APIs
- **`cmd/`**: Binary entry points
  - `cmd/manager/`: Main entry point for Hive controllers
  - `cmd/operator/`: Main entry point for Hive operator
  - `cmd/hiveadmission/`: Admission webhook server
- **`pkg/`**: Package source code
  - `pkg/controller/`: Operator controllers
  - `pkg/install/`: Installation logic and OpenShift installer integration
  - `pkg/installmanager/`: Manages cluster installation process
  - `pkg/operator/`: Hive operator logic
  - `pkg/resource/`: Utilities for applying resources to remote clusters
  - `pkg/remoteclient/`: Client for connecting to remote clusters
  - `pkg/{awsclient,azureclient,gcpclient,ibmclient}/`: Cloud provider-specific client implementations
- **`config/`**: Kubernetes YAML manifests for deploying the operator
- **`docs/`**: Developer and user documentation
- **`hack/`**: Developer scripts and tools
- **`test/e2e/`**: End-to-end tests

## Pull Requests

### Commit Messages

All git commits should follow a standard format to ensure clarity and traceability.

**Title format**: `<Subsystem>: <Title>`

**Example**:
```text
HIVE-2980: How to refresh ClusterPool cloud creds
Add doc content describing different ways to rotate a ClusterPool's
cloud credentials.

Add a script, `hack/refresh-clusterpool-creds.sh` to nondisruptively
update the (currently AWS; other platforms TODO) cloud credentials for
all existing ClusterDeployments associated with a given ClusterPool.
- Accepts two args: the clusterpool namespace and name.
- Discovers the current AWS creds Secret from the clusterpool.
- Discovers all existing ClusterDeployments associated with the
  clusterpool.
- Discovers the AWS creds Secret for each CD.
- Patches that Secret with the `.data` of the clusterpool's Secret.
```

### AI Attribution

If AI tools were used to generate or significantly assist with the code or documentation, please include a footer annotation in the commit message:

```text
Assisted-by: <AI Model Name>
```

### Submission Checklist

- [ ] Run `make update` to ensure generated code is up to date.
- [ ] Run `make test` to ensure no regressions.
- [ ] Run `make verify` to ensure code formatting and standards.
- [ ] Ensure commit messages follow the project standards.
- [ ] Update documentation for user-facing changes.

## Additional Resources

- [Developing Hive Guide](./docs/developing.md) - Detailed development setup and workflows
- [Architecture Documentation](./docs/architecture.md) - Understanding Hive's architecture
- [AGENTS.md](./AGENTS.md) - Instructions for AI agents and quick reference
- [Hive Documentation](./docs/) - Complete documentation index
