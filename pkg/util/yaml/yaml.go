package yaml

import (
	"fmt"
	"strings"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"
	yamlk8s "sigs.k8s.io/yaml"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

func Test(c []byte, path, val string) (match bool, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("no such path %s", path)
			match = false
		}
		// Implicit return
	}()
	op := hivev1.PatchEntity{
		Op:    "test",
		Path:  path,
		Value: val,
	}
	if _, err := ApplyPatches(c, []hivev1.PatchEntity{op}); err != nil {
		return false, fmt.Errorf("no match for value %s", val)
	}
	return true, nil
}

func ApplyPatches(yamlBytes []byte, patches []hivev1.PatchEntity) ([]byte, error) {
	jsonBytes, err := yamlk8s.YAMLToJSON(yamlBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to convert from yaml to json: %w", err)
	}
	// []PatchEntity => []string{`{"op": "add", ...}`, `{...}`, ...}
	var patchStrings []string
	for _, p := range patches {
		patchStrings = append(patchStrings, p.Encode())
	}
	// => `[{"op": "add", ...}, {...}, ...]`
	patchJSON := fmt.Sprintf("[\n%s\n]", strings.Join(patchStrings, ",\n"))
	// => Patch
	patcher, err := jsonpatch.DecodePatch([]byte(patchJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to decode patch\n%s: %w", patchJSON, err)
	}
	jsonBytes, err = patcher.Apply(jsonBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to apply patch\n%s: %w", patchJSON, err)
	}
	yamlBytes, err = yamlk8s.JSONToYAML(jsonBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to convert from json to yaml: %w", err)
	}
	return yamlBytes, nil
}
