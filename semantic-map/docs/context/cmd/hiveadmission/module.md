# Module atlas

## Responsibility

Binary entry point for the Hive validating-admission webhook server. Registers all CRD-specific webhook handlers and starts the admission server process.

## Public Interface/API

This is a `main` package — no exported identifiers. The binary provides:

- An HTTPS admission-webhook endpoint (served via `generic-admission-server`)
- Nine validating admission hooks registered at startup:
  - `DNSZoneValidatingAdmissionHook`
  - `ClusterDeploymentValidatingAdmissionHook`
  - `ClusterPoolValidatingAdmissionHook`
  - `ClusterImageSetValidatingAdmissionHook`
  - `ClusterProvisionValidatingAdmissionHook`
  - `MachinePoolValidatingAdmissionHook`
  - `SyncSetValidatingAdmissionHook`
  - `SelectorSyncSetValidatingAdmissionHook`
  - `ClusterDeploymentCustomizationValidatingAdmissionHook`

## Internal Dependencies

- `github.com/openshift/hive/pkg/validating-webhooks/hive/v1` — webhook handler implementations
- `github.com/openshift/hive/pkg/util/scheme` — shared Kubernetes scheme registration
- `github.com/openshift/hive/pkg/version` — build-time version string
- `github.com/openshift/generic-admission-server/pkg/cmd` — admission server framework (external)
- `sigs.k8s.io/controller-runtime/pkg/webhook/admission` — decoder for admission requests (external)
- `github.com/sirupsen/logrus` — structured logging (external)

## Capabilities

- Starts a standalone admission webhook server for Hive CRD validation
- Decodes incoming admission requests using the Hive API scheme
- Delegates validation logic to per-CRD hooks in `pkg/validating-webhooks`
- Logs version at startup for operational traceability

## Understanding Score

0.8
