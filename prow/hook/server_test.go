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

package hook

import (
	"reflect"
	"testing"

	"k8s.io/test-infra/prow/plugins"
)

func TestNeedDemux(t *testing.T) {
	tests := []struct {
		name string

		eventType string
		srcRepo   string
		plugins   map[string][]plugins.ExternalPlugin

		expected []plugins.ExternalPlugin
	}{
		{
			name: "no external plugins",

			eventType: "issue_comment",
			srcRepo:   "kubernetes/test-infra",
			plugins:   nil,

			expected: nil,
		},
		{
			name: "we have variety",

			eventType: "issue_comment",
			srcRepo:   "kubernetes/test-infra",
			plugins: map[string][]plugins.ExternalPlugin{
				"kubernetes/test-infra": {
					{
						Name:   "sandwich",
						Events: []string{"pull_request"},
					},
					{
						Name: "coffee",
					},
				},
				"kubernetes/kubernetes": {
					{
						Name:   "gumbo",
						Events: []string{"issue_comment"},
					},
				},
				"kubernetes": {
					{
						Name:   "chicken",
						Events: []string{"push"},
					},
					{
						Name: "water",
					},
					{
						Name:   "chocolate",
						Events: []string{"pull_request", "issue_comment", "issues"},
					},
				},
			},

			expected: []plugins.ExternalPlugin{
				{
					Name: "coffee",
				},
				{
					Name: "water",
				},
				{
					Name:   "chocolate",
					Events: []string{"pull_request", "issue_comment", "issues"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Logf("Running scenario %q", test.name)

		pa := &plugins.PluginAgent{}
		pa.Set(&plugins.Configuration{
			ExternalPlugins: test.plugins,
		})
		s := &Server{Plugins: pa}

		gotPlugins := s.needDemux(test.eventType, test.srcRepo)
		if len(gotPlugins) != len(test.expected) {
			t.Errorf("expected plugins: %+v, got: %+v", test.expected, gotPlugins)
			continue
		}
		for _, expected := range test.expected {
			var found bool
			for _, got := range gotPlugins {
				if got.Name != expected.Name {
					continue
				}
				if !reflect.DeepEqual(expected, got) {
					t.Errorf("expected plugin: %+v, got: %+v", expected, got)
				}
				found = true
			}
			if !found {
				t.Errorf("expected plugins: %+v, got: %+v", test.expected, gotPlugins)
				break
			}
		}
	}
}
