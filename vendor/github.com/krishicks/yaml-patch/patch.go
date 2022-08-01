package yamlpatch

import (
	"bytes"
	"fmt"
	"io"

	yaml "gopkg.in/yaml.v2"
)

// Patch is an ordered collection of operations.
type Patch []Operation

// DecodePatch decodes the passed YAML document as if it were an RFC 6902 patch
func DecodePatch(bs []byte) (Patch, error) {
	var p Patch

	err := yaml.Unmarshal(bs, &p)
	if err != nil {
		return nil, err
	}

	return p, nil
}

// Apply returns a YAML document that has been mutated per the patch
func (p Patch) Apply(doc []byte) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(doc))
	buf := bytes.NewBuffer([]byte{})
	encoder := yaml.NewEncoder(buf)

	for {
		var iface interface{}
		err := decoder.Decode(&iface)
		if err != nil {
			if err == io.EOF {
				break
			}

			return nil, fmt.Errorf("failed to decode doc: %s\n\n%s", string(doc), err)
		}

		var c Container
		c = NewNode(&iface).Container()

		for _, op := range p {
			pathfinder := NewPathFinder(c)
			if op.Path.ContainsExtendedSyntax() {
				paths := pathfinder.Find(string(op.Path))
				if paths == nil {
					return nil, fmt.Errorf("could not expand pointer: %s", op.Path)
				}

				for i := len(paths) - 1; i >= 0; i-- {
					path := paths[i]
					newOp := op
					newOp.Path = OpPath(path)
					err := newOp.Perform(c)
					if err != nil {
						return nil, err
					}
				}
			} else {
				err := op.Perform(c)
				if err != nil {
					return nil, err
				}
			}
		}

		err = encoder.Encode(c)
		if err != nil {
			return nil, fmt.Errorf("failed to encode container: %s", err)
		}
	}

	err := encoder.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
