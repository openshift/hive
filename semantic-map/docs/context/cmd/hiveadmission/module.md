# Module atlas

## Responsibility

Entry-point binary that starts the Hive CRD validating admission webhook server. Registers webhook handlers for all Hive custom resources and serves them via the generic admission server framework.

## Public Interface/API

`main` package — no exported identifiers. Produces the `hiveadmission` binary.

- `main()` — creates a decoder from the Hive scheme and launches `RunAdmissionServer` with nine validating admission hooks
- `createDecoder()` — unexported helper; builds an `admission.Decoder` from the aggregated Hive scheme

Registered webhook hooks (all from `pkg/validating-webhooks/hive/v1`):
- `NewDNSZoneValidatingAdmissionHook`
- `NewClusterDeploymentValidatingAdmissionHook`
- `NewClusterPoolValidatingAdmissionHook`
- `NewClusterImageSetValidatingAdmissionHook`
- `NewClusterProvisionValidatingAdmissionHook`
- `NewMachinePoolValidatingAdmissionHook`
- `NewSyncSetValidatingAdmissionHook`
- `NewSelectorSyncSetValidatingAdmissionHook`
- `NewClusterDeploymentCustomizationValidatingAdmissionHook`

## Internal Dependencies

- `github.com/openshift/generic-admission-server/pkg/cmd` — admission server runner
- `github.com/openshift/hive/pkg/util/scheme` — aggregated CRD scheme
- `github.com/openshift/hive/pkg/validating-webhooks/hive/v1` — webhook handler implementations
- `github.com/openshift/hive/pkg/version` — build version string
- `github.com/sirupsen/logrus` — structured logging
- `sigs.k8s.io/controller-runtime/pkg/webhook/admission` — decoder for admission requests

## Capabilities

- Starts an HTTPS admission webhook server for Hive CRD validation
- Validates create/update requests for nine Hive CRD types
- Logs version info at startup

## Understanding Score

0.9
