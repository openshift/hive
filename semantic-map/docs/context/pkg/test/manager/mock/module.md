# Module atlas

## Responsibility

Generated GoMock implementation of the `controller-runtime/pkg/manager.Manager` interface for use in unit tests that need a mock controller manager.

## Public Interface/API

- `MockManager` -- mock struct implementing `manager.Manager`
- `NewMockManager(ctrl *gomock.Controller) *MockManager` -- creates a new mock instance
- `MockManager.EXPECT() *MockManagerMockRecorder` -- returns the mock recorder for setting expectations
- `MockManagerMockRecorder` -- recorder struct for registering expected calls
- Mocked methods on `MockManager`: `Add`, `AddHealthzCheck`, `AddMetricsServerExtraHandler`, `AddReadyzCheck`, `Elected`, `GetAPIReader`, `GetCache`, `GetClient`, `GetConfig`, `GetControllerOptions`, `GetEventRecorderFor`, `GetFieldIndexer`, `GetHTTPClient`, `GetLogger`, `GetRESTMapper`, `GetScheme`, `GetWebhookServer`, `Start`
- Corresponding recorder methods on `MockManagerMockRecorder` for each mocked method

## Internal Dependencies

- `go.uber.org/mock/gomock`
- `github.com/go-logr/logr`
- `k8s.io/apimachinery/pkg/api/meta`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/client-go/rest`
- `k8s.io/client-go/tools/record`
- `sigs.k8s.io/controller-runtime/pkg/cache`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/config`
- `sigs.k8s.io/controller-runtime/pkg/healthz`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/webhook`

## Capabilities

- Full GoMock mock of `manager.Manager` with all standard methods (Add, Start, GetClient, GetScheme, GetCache, etc.)
- Supports expectation recording for each method via `MockManagerMockRecorder`
- Auto-generated code -- not hand-maintained

## Understanding Score

0.85
