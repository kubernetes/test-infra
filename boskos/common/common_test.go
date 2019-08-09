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
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

type fakeStruct struct {
	Value string `json:"value"`
}

func TestUserData_Extract(t *testing.T) {
	ud := UserData{}
	fs := fakeStruct{"value"}
	if err := ud.Set("test", &fs); err != nil {
		t.Errorf("unable to set data")
	}
	var rfs fakeStruct
	if err := ud.Extract("test", &rfs); err != nil {
		t.Error("unable to extract struct")
	}
	if fs.Value != rfs.Value {
		t.Error("struct don't match")
	}
}

func TestUserData_Update(t *testing.T) {
	ud1 := UserDataFromMap(UserDataMap{"0": "0"})
	ud2 := UserDataFromMap(UserDataMap{"1": "1", "2": "2"})
	ud3 := UserDataFromMap(UserDataMap{"0": "0", "1": "1", "2": "2"})
	ud1.Update(ud2)
	if !reflect.DeepEqual(ud1.ToMap(), ud3.ToMap()) {
		t.Errorf("%v does not match expected %v", ud1, ud3)
	}
	// Testing delete
	ud3.Update(UserDataFromMap(UserDataMap{"0": ""}))
	if !reflect.DeepEqual(ud3.ToMap(), ud2.ToMap()) {
		t.Errorf("%v does not match expected %v", ud3, ud2)
	}
}

func TestUserData_Marshall(t *testing.T) {
	ud := UserDataFromMap(UserDataMap{"0": "0", "1": "1", "2": "2"})
	b, err := ud.MarshalJSON()
	if err != nil {
		t.Errorf("unable to marshall %v", ud.ToMap())
	}
	var udFromJSON UserData
	if err := udFromJSON.UnmarshalJSON(b); err != nil {
		t.Errorf("unable to unmarshall %v", string(b))
	}
	if !reflect.DeepEqual(ud.ToMap(), udFromJSON.ToMap()) {
		t.Errorf("src %v does not match %v", ud.ToMap(), udFromJSON.ToMap())
	}
}

func TestUserData_JSON(t *testing.T) {
	ud := UserDataFromMap(UserDataMap{"0": "0", "1": "1", "2": "2"})
	b := new(bytes.Buffer)
	if err := json.NewEncoder(b).Encode(ud); err != nil {
		t.Errorf("unable to marshall %v", ud.ToMap())
	}
	var decodedUD UserData
	if err := json.NewDecoder(b).Decode(&decodedUD); err != nil {
		t.Errorf("unable to unmarshall %v", b.String())
	}

	if !reflect.DeepEqual(ud.ToMap(), decodedUD.ToMap()) {
		t.Errorf("src %v does not match %v", ud.ToMap(), decodedUD.ToMap())
	}
}

func TestConfig(t *testing.T) {
	config, err := ParseConfig("../resources.yaml")
	if err != nil {
		t.Errorf("parseConfig error: %v", err)
	}

	if err = ValidateConfig(config); err != nil {
		t.Errorf("invalid config: %v", err)
	}
}
