package yamlpatch

import (
	"fmt"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

// PathFinder can be used to find RFC6902-standard paths given non-standard
// (key=value) pointer syntax
type PathFinder struct {
	root Container
}

// NewPathFinder takes an interface that represents a YAML document and returns
// a new PathFinder
func NewPathFinder(container Container) *PathFinder {
	return &PathFinder{
		root: container,
	}
}

type route struct {
	key string
	value Container
}

// Find expands the given path into all matching paths, returning the canonical
// versions of those matching paths
func (p *PathFinder) Find(path string) []string {
	parts := strings.Split(path, "/")

	if parts[1] == "" {
		return []string{"/"}
	}

	routes := []route {
		route{"", p.root},
	}

	for _, part := range parts[1:] {
		routes = find(decodePatchKey(part), routes)
	}

	var paths []string
	for _, r := range routes {
		paths = append(paths, r.key)
	}

	return paths
}

func find(part string, routes []route) (matches []route) {
	for _, r := range routes {
		prefix := r.key
		container := r.value

		if part == "-" {
			for _, r = range routes {
				matches = append(matches, route {fmt.Sprintf("%s/-", r.key), r.value})
			}
			return
		}

		if kv := strings.Split(part, "="); len(kv) == 2 {
			decoder := yaml.NewDecoder(strings.NewReader(kv[1]))
			var value interface{}
			if decoder.Decode(&value) != nil {
				value = kv[1]
			}

			if newMatches := findAll(prefix, kv[0], value, container); len(newMatches) > 0 {
				matches = newMatches
			}
			continue
		}

		if node, err := container.Get(part); err == nil && node != nil {
			path := fmt.Sprintf("%s/%s", prefix, part)
			matches = append(matches, route {path, node.Container()})
		}
	}

	return
}

func findAll(prefix, findKey string, findValue interface{}, container Container) (matches []route) {
	if container == nil {
		return nil
	}

	if v, err := container.Get(findKey); err == nil && v != nil {
		if v.Value() == findValue {
			return []route {route{prefix, container}}
		}
	}

	switch it := container.(type) {
	case *nodeMap:
		for k, v := range *it {
			for _, r := range findAll(fmt.Sprintf("%s/%s", prefix, k), findKey, findValue, v.Container()) {
				matches = append(matches, r)
			}
		}
	case *nodeSlice:
		for i, v := range *it {
			for _, r := range findAll(fmt.Sprintf("%s/%d", prefix, i), findKey, findValue, v.Container()) {
				matches = append(matches, r)
			}
		}
	}

	return
}
