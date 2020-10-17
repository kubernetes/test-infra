/*
Copyright 2020 The Kubernetes Authors.

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

package layeredsets

import (
	"math/rand"
	"sort"

	"k8s.io/apimachinery/pkg/util/sets"
)

// String is a layered set implemented as a slice of sets.String.
type String []sets.String

// NewString creates a String from a list of values.
func NewString(items ...string) String {
	ss := String{}
	ss.Insert(0, items...)
	return ss
}

// NewStringFromSlices creates a String from a list of sets.
func NewStringFromSlices(items ...[]string) String {
	ss := String{}
	for i, s := range items {
		ss.Insert(i, s...)
	}
	return ss
}

// Insert adds items to the set.
func (s *String) Insert(layerID int, items ...string) {
	for len(*s) < layerID+1 {
		*s = append(*s, sets.NewString())
	}
	for _, item := range items {
		if !s.Has(item) {
			(*s)[layerID].Insert(item)
		}
	}
}

// Delete removes all items from the set.
func (s String) Delete(items ...string) {
	for _, layer := range s {
		layer.Delete(items...)
	}
}

// Has returns true if and only if item is contained in the set.
func (s String) Has(item string) bool {
	for _, layer := range s {
		if layer.Has(item) {
			return true
		}
	}
	return false
}

// Difference returns a set of objects that are not in s2
func (s String) Difference(s2 sets.String) String {
	result := NewString()
	for layerID, layer := range s {
		for _, key := range layer.List() {
			if !s2.Has(key) {
				result.Insert(layerID, key)
			}
		}
	}
	return result
}

// Union returns a new set which includes items in either s1 or s2.
func (s String) Union(s2 String) String {
	result := NewString()
	for layerID, layer := range s {
		result.Insert(layerID, layer.List()...)
	}
	for layerID, layer := range s2 {
		result.Insert(layerID, layer.List()...)
	}
	return result
}

// PopRandom randomly selects an element and pops it.
func (s String) PopRandom() string {
	for _, layer := range s {
		if layer.Len() > 0 {
			list := layer.List()
			sort.Strings(list)
			sel := list[rand.Intn(len(list))]
			s.Delete(sel)
			return sel
		}
	}
	return ""
}

// Equal returns true if and only if s1 is equal (as a set) to s2.
func (s String) Equal(s2 String) bool {
	if s.Len() != s2.Len() {
		return false
	}
	for layerID, layer := range s {
		if !s2[layerID].Equal(layer) {
			return false
		}
	}
	return true
}

// List returns the contents as a sorted string slice, respecting layers.
func (s String) List() []string {
	var res []string
	for _, layer := range s {
		res = append(res, layer.List()...)
	}
	return res
}

// UnsortedList returns the slice with contents in random order, respecting layers.
func (s String) UnsortedList() []string {
	var res []string
	for _, layer := range s {
		res = append(res, layer.UnsortedList()...)
	}
	return res
}

// Set converts the multiset into a regular sets.String for compatibility.
func (s String) Set() sets.String {
	ss := sets.String{}
	return ss.Insert(s.List()...)
}

// Len returns the size of the set.
func (s String) Len() int {
	var i int
	for _, layer := range s {
		i += layer.Len()
	}
	return i
}
