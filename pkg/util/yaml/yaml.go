package yaml

import (
	"bytes"
	"fmt"

	yamlpatch "github.com/krishicks/yaml-patch"

	yaml "gopkg.in/yaml.v2"
)

func Decode(ba []byte) (yamlpatch.Container, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(ba))
	var iface interface{}
	err := decoder.Decode(&iface)
	if err != nil {
		return nil, err
	}
	return yamlpatch.NewNode(&iface).Container(), nil
}

func Test(c yamlpatch.Container, path, val string) (match bool, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("no such path %s", path)
			match = false
		}
		// Implicit return
	}()
	valif := interface{}(val)
	op := yamlpatch.Operation{
		Op:    "test",
		Path:  yamlpatch.OpPath(path),
		Value: yamlpatch.NewNode(&valif),
	}
	if err := op.Perform(c); err != nil {
		return false, fmt.Errorf("no match for value %s", val)
	}
	return true, nil
}
