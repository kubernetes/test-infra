/*
Copyright 2019 The Kubernetes Authors.

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
	"encoding/json"
	"reflect"
	"testing"
)

func TestMeta(t *testing.T) {
	world := "world"
	const key = "target-key"
	cases := []struct {
		name    string
		in      Metadata
		call    func(actual Metadata) (interface{}, bool)
		val     interface{}
		present bool
	}{
		{
			name: "can match string",
			in: Metadata{
				key: world,
			},
			call: func(actual Metadata) (interface{}, bool) {
				return actual.String(key)
			},
			val:     &world,
			present: true,
		},
		{
			name: "detect value is not a string",
			in: Metadata{
				key: Metadata{"super": "fancy"},
			},
			call: func(actual Metadata) (interface{}, bool) {
				return actual.String(key)
			},
			val:     (*string)(nil),
			present: true,
		},
		{
			name: "can match metadata",
			in: Metadata{
				key: Metadata{
					"super": "fancy",
					"one":   1,
				},
			},
			call: func(actual Metadata) (interface{}, bool) {
				return actual.Meta(key)
			},
			val: &Metadata{
				"super": "fancy",
				"one":   1.0, // LOL json
			},
			present: true,
		},
		{
			name: "detect value is not metadata",
			in: Metadata{
				key: "not metadata",
			},
			call: func(actual Metadata) (interface{}, bool) {
				return actual.Meta(key)
			},
			val:     (*Metadata)(nil),
			present: true,
		},
		{
			name: "detect key absence for string",
			in: Metadata{
				"random-key": "hello",
			},
			call: func(actual Metadata) (interface{}, bool) {
				return actual.String(key)
			},
			val: (*string)(nil),
		},
		{
			name: "detect key absence for metadata",
			in: Metadata{
				"random-key": Metadata{},
			},
			call: func(actual Metadata) (interface{}, bool) {
				return actual.Meta(key)
			},
			val: (*Metadata)(nil),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := json.Marshal(tc.in)
			if err != nil {
				t.Errorf("marshal: %v", err)
			}
			var actual Metadata
			if err := json.Unmarshal(out, &actual); err != nil {
				t.Errorf("unmarshal: %v", err)
			}
			val, present := tc.call(actual)
			if !reflect.DeepEqual(val, tc.val) {
				t.Errorf("%#v != expected %#v", val, tc.val) // Remember json doesn't have ints
			}
			if present != tc.present {
				t.Errorf("present %t != expected %t", present, tc.present)
			}
		})
	}
}
