/*
Copyright 2016 The Kubernetes Authors.

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

package main

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

func expectEqual(t *testing.T, msg string, have interface{}, want interface{}) {
	if !reflect.DeepEqual(have, want) {
		t.Errorf("bad %s: got %v, wanted %v",
			msg, have, want)
	}
}

func TestVend(t *testing.T) {
	tmp, err := ioutil.TempFile("", "tot_test_")
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(tmp.Name()) // json decoding an empty file throws an error
	defer os.Remove(tmp.Name())

	store, err := newStore(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}

	expectEqual(t, "empty vend", store.vend("a"), 1)
	expectEqual(t, "second vend", store.vend("a"), 2)
	expectEqual(t, "third vend", store.vend("a"), 3)
	expectEqual(t, "second empty", store.vend("b"), 1)

	store2, err := newStore(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	expectEqual(t, "fourth vend, different instance", store2.vend("a"), 4)

}
