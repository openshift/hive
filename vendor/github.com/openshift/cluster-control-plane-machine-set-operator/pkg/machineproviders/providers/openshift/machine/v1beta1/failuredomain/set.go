/*
Copyright 2022 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package failuredomain

import "sort"

// Set implements a set symantic for a FailureDomain.
// As this is an interface we cannot use a map directly.
// We must compare the failure domains directly.
type Set struct {
	items []FailureDomain
}

// NewSet creates a new set from the given items.
func NewSet(items ...FailureDomain) *Set {
	s := Set{}
	s.Insert(items...)

	return &s
}

// Has returns true if the item is in the set.
func (s *Set) Has(item FailureDomain) bool {
	for _, fd := range s.items {
		if fd.Equal(item) {
			return true
		}
	}

	return false
}

// Insert adds the item to the set.
func (s *Set) Insert(items ...FailureDomain) {
	for _, item := range items {
		if !s.Has(item) {
			s.items = append(s.items, item)
		}
	}
}

// List returns the items in the set as a sorted slice.
func (s *Set) List() []FailureDomain {
	out := []FailureDomain{}
	out = append(out, s.items...)

	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})

	return out
}
