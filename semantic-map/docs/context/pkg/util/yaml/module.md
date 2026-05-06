# Module atlas

## Responsibility

Provides utilities for applying JSON patch operations to YAML documents and testing YAML values at JSON Pointer paths. Converts between YAML and JSON internally using the sigs.k8s.io/yaml library.

## Public Interface/API

- `ApplyPatches(yamlBytes []byte, patches []hivev1.PatchEntity) ([]byte, error)` -- applies a list of PatchEntity operations to a YAML document, returning patched YAML
- `Test(c []byte, path, val string) (match bool, err error)` -- tests whether a YAML document has a given value at a JSON Pointer path

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- PatchEntity type
- `gopkg.in/evanphx/json-patch.v4` -- JSON patch application
- `sigs.k8s.io/yaml` -- YAML/JSON conversion

## Capabilities

- Apply RFC 6902 JSON Patch operations to YAML content
- Test for value existence/equality at a path in YAML
- YAML-to-JSON and JSON-to-YAML round-trip conversion

## Understanding Score

0.85
