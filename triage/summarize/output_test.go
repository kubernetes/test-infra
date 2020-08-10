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

package summarize

import (
	"testing"
)

func TestAnnotateOwners(t *testing.T) {
	testCases := []struct {
		name   string
		test   string // The name of the test, containing an owner name
		owner  string // What the owner should be
		owners map[string][]string
	}{
		{"Prefixed", "[sig-node] Node reboots", "node", make(map[string][]string)},
		{"None", "unknown test name", "testing", make(map[string][]string)},
		{"Suffixed", "Suffixes too [sig-storage]", "storage", make(map[string][]string)},
		{"Old-style prefixed", "Variable test with old-style prefixes", "node", map[string][]string{"node": {"Variable"}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			now := int(1.5e9)
			data := jsonOutput{
				Clustered: []jsonCluster{
					{
						Tests: []test{
							{
								Name: tc.test,
								Jobs: []job{
									{
										Name:         "somejob",
										BuildNumbers: []string{"123", "125"},
									},
								},
							},
						},
					},
				},
				Builds: columns{
					JobPaths: map[string]string{"somejob": "/logs/somejob"},
					Cols: columnarBuilds{
						Started: []int{now},
					},
				},
			}
			builds := map[string]build{"/logs/somejob/123": {Started: now}}

			// Run the test
			err := annotateOwners(&data, builds, tc.owners)
			if err != nil {
				t.Errorf("annotateOwners(%#v, %#v, %#v) returned with an error: %s", data, builds, tc.owners, err)
				return
			}

			if tc.owner != data.Clustered[0].Owner {
				t.Errorf("annotateOwners(%#v, %#v, %#v) annotated an owner of %#v, wanted %#v", data, builds, tc.owners, data.Clustered[0].Owner, tc.owner)
			}
		})
	}
}
