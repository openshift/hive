package common

import (
	"encoding/json"
	"github.com/evanphx/json-patch"
)

func JSONDiff(a, b interface{}) ([]byte, error) {
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
