package common

import (
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch"
)

func JSONDiff(a, b any) ([]byte, error) {
	jsonA, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	jsonB, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	return jsonpatch.CreateMergePatch(jsonA, jsonB)
}
