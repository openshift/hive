<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/util/logrus/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `NewLoggingEventRecorder` — NewLoggingEventRecorder creates an event recorder that logs events via logrus
- `NewLogr` — NewLogr returns a new logger that implements logr.Logger interface using the FieldLogger.

## Internal Dependencies

- `context`
- `fmt`
- `github.com/go-logr/logr`
- `github.com/openshift/library-go/pkg/operator/events`
- `github.com/sirupsen/logrus`

## Capabilities

- **`package`** name(s): **logrus**.
- Go **`import`** edges listed below (5 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/util/logrus`.

## Understanding Score

0.0
