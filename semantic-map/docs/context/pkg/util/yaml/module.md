# Module atlas

## Responsibility

Provides utilities for applying JSON Patch (RFC 6902) operations to YAML documents. The workflow converts YAML to JSON, applies one or more patch operations (encoded as `hivev1.PatchEntity`), and converts the result back to YAML. Also provides a `Test` function that uses the JSON Patch `test` operation to check whether a value at a given path matches an expected value in a YAML document.

## Public Interface/API

- `ApplyPatches(yamlBytes []byte, patches []hivev1.PatchEntity) ([]byte, error)` -- converts YAML input to JSON, applies the provided list of `PatchEntity` operations (add/remove/replace/test/etc.), and returns the patched result as YAML. Each `PatchEntity` is encoded via its own `Encode()` method.
- `Test(c []byte, path, val string) (match bool, err error)` -- convenience function that uses a JSON Patch `test` operation to check if the value at `path` in the YAML document `c` equals `val`. Returns `(true, nil)` on match. Uses `recover()` to catch panics from invalid paths.

## Internal Dependencies

- `fmt` -- error formatting.
- `strings` -- joining patch strings.
- `github.com/openshift/hive/apis/hive/v1` -- `PatchEntity` type used for patch operations.
- `gopkg.in/evanphx/json-patch.v4` -- JSON Patch decoding and application.
- `sigs.k8s.io/yaml` -- YAML-to-JSON and JSON-to-YAML conversion.

## Capabilities

- **`package`** name(s): **yaml**.
- Go **`import`** edges listed below (5 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/util/yaml`.
- Single file (`yaml.go`), no tests.

## Understanding Score

0.85
