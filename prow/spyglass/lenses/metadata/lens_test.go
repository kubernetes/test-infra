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

package metadata

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFlattenMetadata(t *testing.T) {
	tests := []struct {
		name        string
		metadata    map[string]interface{}
		expectedMap map[string]string
	}{
		{
			name:        "Empty map",
			metadata:    map[string]interface{}{},
			expectedMap: map[string]string{},
		},
		{
			name: "Test metadata",
			metadata: map[string]interface{}{
				"field1": "value1",
				"field2": "value2",
				"field3": "value3",
			},
			expectedMap: map[string]string{
				"field1": "value1",
				"field2": "value2",
				"field3": "value3",
			},
		},
		{
			name: "Test metadata with non-strings",
			metadata: map[string]interface{}{
				"field1": "value1",
				"field2": 2,
				"field3": true,
				"field4": "value4",
			},
			expectedMap: map[string]string{
				"field1": "value1",
				"field4": "value4",
			},
		},
		{
			name: "Test nested metadata",
			metadata: map[string]interface{}{
				"field1": "value1",
				"field2": "value2",
				"field3": map[string]interface{}{
					"nest1-field1": "nest1-value1",
					"nest1-field2": "nest1-value2",
					"nest1-field3": map[string]interface{}{
						"nest2-field1": "nest2-value1",
						"nest2-field2": "nest2-value2",
					},
				},
				"field4": "value4",
			},
			expectedMap: map[string]string{
				"field1":                           "value1",
				"field2":                           "value2",
				"field3.nest1-field1":              "nest1-value1",
				"field3.nest1-field2":              "nest1-value2",
				"field3.nest1-field3.nest2-field1": "nest2-value1",
				"field3.nest1-field3.nest2-field2": "nest2-value2",
				"field4":                           "value4",
			},
		},
	}

	lens := Lens{}
	for _, test := range tests {
		flattenedMetadata := lens.flattenMetadata(test.metadata)
		if !reflect.DeepEqual(flattenedMetadata, test.expectedMap) {
			t.Errorf("%s: resulting map did not match expected map: %v", test.name, cmp.Diff(flattenedMetadata, test.expectedMap))
		}
	}
}
