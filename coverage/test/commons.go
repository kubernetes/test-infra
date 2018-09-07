/*
Copyright 2018 The Kubernetes Authors.

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

package test

import (
	"fmt"
	"testing"
)

// StrFailure is used to display discrepancy between expected and actual result in test
func StrFailure(input, expected, actual string) string {
	return fmt.Sprintf("input=%s; expected=%s; actual=%s\n", input, expected, actual)
}

//Fail fails a test and prints out info about expected and actual value
func Fail(t *testing.T, input, expected, actual interface{}) {
	t.Fatalf("input=%s; expected=%v; actual=%v\n", input, expected, actual)
}

//AssertEqual checks equality of expected and actual results, fail the test if not equal
func AssertEqual(t *testing.T, expected, actual interface{}) {
	if expected != actual {
		t.Fatalf("expected='%v'; actual='%v'\n", expected, actual)
	}
}

type stringSet struct {
	data map[string]bool
}

//Add adds a string to the string set
func (set *stringSet) Add(s string) {
	set.data[s] = true
}

//Has checks if the string is a member of the string set
func (set *stringSet) Has(s string) bool {
	return set.data[s]
}

func newStringSet() *stringSet {
	return &stringSet{
		data: make(map[string]bool),
	}
}

//MakeStringSet makes a set of string out of given strings
func MakeStringSet(members ...string) (set *stringSet) {
	set = newStringSet()
	for _, member := range members {
		set.Add(member)
	}
	return set
}

//AllMembers returns all member of the set in a list
func (set *stringSet) AllMembers() (res []string) {
	for item, valid := range set.data {
		if valid {
			res = append(res, item)
		}
	}
	return
}
