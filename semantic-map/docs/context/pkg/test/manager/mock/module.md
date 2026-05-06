<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/test/manager/mock/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `MockManager` — MockManager is a mock of Manager interface.
- `MockManager.Add` — Add mocks base method.
- `MockManager.AddHealthzCheck` — AddHealthzCheck mocks base method.
- `MockManager.AddMetricsServerExtraHandler` — AddMetricsServerExtraHandler mocks base method.
- `MockManager.AddReadyzCheck` — AddReadyzCheck mocks base method.
- `MockManager.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockManager.Elected` — Elected mocks base method.
- `MockManager.GetAPIReader` — GetAPIReader mocks base method.
- `MockManager.GetCache` — GetCache mocks base method.
- `MockManager.GetClient` — GetClient mocks base method.
- `MockManager.GetConfig` — GetConfig mocks base method.
- `MockManager.GetControllerOptions` — GetControllerOptions mocks base method.
- `MockManager.GetEventRecorderFor` — GetEventRecorderFor mocks base method.
- `MockManager.GetFieldIndexer` — GetFieldIndexer mocks base method.
- `MockManager.GetHTTPClient` — GetHTTPClient mocks base method.
- `MockManager.GetLogger` — GetLogger mocks base method.
- `MockManager.GetRESTMapper` — GetRESTMapper mocks base method.
- `MockManager.GetScheme` — GetScheme mocks base method.
- `MockManager.GetWebhookServer` — GetWebhookServer mocks base method.
- `MockManager.Start` — Start mocks base method.
- `MockManagerMockRecorder` — MockManagerMockRecorder is the mock recorder for MockManager.
- `MockManagerMockRecorder.Add` — Add indicates an expected call of Add.
- `MockManagerMockRecorder.AddHealthzCheck` — AddHealthzCheck indicates an expected call of AddHealthzCheck.
- `MockManagerMockRecorder.AddMetricsServerExtraHandler` — AddMetricsServerExtraHandler indicates an expected call of AddMetricsServerExtraHandler.
- `MockManagerMockRecorder.AddReadyzCheck` — AddReadyzCheck indicates an expected call of AddReadyzCheck.
- `MockManagerMockRecorder.Elected` — Elected indicates an expected call of Elected.
- `MockManagerMockRecorder.GetAPIReader` — GetAPIReader indicates an expected call of GetAPIReader.
- `MockManagerMockRecorder.GetCache` — GetCache indicates an expected call of GetCache.
- `MockManagerMockRecorder.GetClient` — GetClient indicates an expected call of GetClient.
- `MockManagerMockRecorder.GetConfig` — GetConfig indicates an expected call of GetConfig.
- `MockManagerMockRecorder.GetControllerOptions` — GetControllerOptions indicates an expected call of GetControllerOptions.
- `MockManagerMockRecorder.GetEventRecorderFor` — GetEventRecorderFor indicates an expected call of GetEventRecorderFor.
- `MockManagerMockRecorder.GetFieldIndexer` — GetFieldIndexer indicates an expected call of GetFieldIndexer.
- `MockManagerMockRecorder.GetHTTPClient` — GetHTTPClient indicates an expected call of GetHTTPClient.
- `MockManagerMockRecorder.GetLogger` — GetLogger indicates an expected call of GetLogger.
- `MockManagerMockRecorder.GetRESTMapper` — GetRESTMapper indicates an expected call of GetRESTMapper.
- `MockManagerMockRecorder.GetScheme` — GetScheme indicates an expected call of GetScheme.
- `MockManagerMockRecorder.GetWebhookServer` — GetWebhookServer indicates an expected call of GetWebhookServer.
- `MockManagerMockRecorder.Start` — Start indicates an expected call of Start.

## Internal Dependencies

- `context`
- `github.com/go-logr/logr`
- `go.uber.org/mock/gomock`
- `k8s.io/apimachinery/pkg/api/meta`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/client-go/rest`
- `k8s.io/client-go/tools/record`
- `net/http`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/cache`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/config`
- `sigs.k8s.io/controller-runtime/pkg/healthz`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/webhook`

## Capabilities

- **`package`** name(s): **mock**.
- Go **`import`** edges listed below (15 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/test/manager/mock`.

## Understanding Score

0.0
