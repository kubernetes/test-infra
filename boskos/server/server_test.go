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

package server

import (
	"reflect"
	"testing"

	"k8s.io/test-infra/boskos/common"
)

func MakeFakeClient(resources []common.Resource) *Ranch {
	newRanch := &Ranch{
		Resources: resources,
	}

	return newRanch
}

func TestSyncConfig(t *testing.T) {
	var testcases = []struct {
		name   string
		oldRes []common.Resource
		newRes []common.Resource
		expect []common.Resource
	}{
		{
			name:   "empty",
			oldRes: []common.Resource{},
			newRes: []common.Resource{},
			expect: []common.Resource{},
		},
		{
			name:   "append",
			oldRes: []common.Resource{},
			newRes: []common.Resource{
				{
					Name: "res",
					Type: "t",
				},
			},
			expect: []common.Resource{
				{
					Name:  "res",
					Type:  "t",
					State: "free",
				},
			},
		},
		{
			name: "delete",
			oldRes: []common.Resource{
				{
					Name: "res",
					Type: "t",
				},
			},
			newRes: []common.Resource{},
			expect: []common.Resource{},
		},
		{
			name: "delete busy",
			oldRes: []common.Resource{
				{
					Name:  "res",
					Type:  "t",
					Owner: "o",
				},
			},
			newRes: []common.Resource{},
			expect: []common.Resource{
				{
					Name:  "res",
					Type:  "t",
					Owner: "o",
				},
			},
		},
	}

	for _, tc := range testcases {
		c := MakeFakeClient(tc.oldRes)
		c.syncConfigHelper(tc.newRes)
		if !reflect.DeepEqual(c.Resources, tc.expect) {
			t.Errorf("Test %v: got %v, expect %v", tc.name, c.Resources, tc.expect)
		}
	}
}
