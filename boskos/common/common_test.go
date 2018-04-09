/*
Copyright 2017 The Kubernetes Authors.

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

package common

import (
	"reflect"
	"testing"
)

type fakeStruct struct {
	Value string `json:"value"`
}

func TestUserData_Extract(t *testing.T) {
	ud := UserData{}
	fs := fakeStruct{"value"}
	ud.Set("test", &fs)
	var rfs fakeStruct
	if err := ud.Extract("test", &rfs); err != nil {
		t.Error("unable to extract struct")
	}
	if fs.Value != rfs.Value {
		t.Error("struct don't match")
	}
}

func TestUserData_Update(t *testing.T) {
	ud1 := UserData{"0": "0"}
	ud2 := UserData{"1": "1", "2": "2"}
	ud3 := UserData{"0": "0", "1": "1", "2": "2"}
	ud1.Update(ud2)
	if !reflect.DeepEqual(ud1, ud3) {
		t.Errorf("%v does not match expected %v", ud1, ud3)
	}
	// Testing delete
	ud3.Update(UserData{"0": ""})
	if !reflect.DeepEqual(ud3, ud2) {
		t.Errorf("%v does not match expected %v", ud3, ud2)
	}
}
