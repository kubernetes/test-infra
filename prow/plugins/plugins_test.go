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

package plugins

import (
	"reflect"
	"testing"
)

func TestGetPlugins(t *testing.T) {
	var testcases = []struct {
		name            string
		pluginMap       map[string][]string // this is read from the plugins.yaml file typically.
		owner           string
		repo            string
		expectedPlugins []string
	}{
		{
			name: "All plugins enabled for org should be returned for any org/repo query",
			pluginMap: map[string][]string{
				"org1": []string{"plugin1", "plugin2"},
			},
			owner:           "org1",
			repo:            "repo",
			expectedPlugins: []string{"plugin1", "plugin2"},
		},
		{
			name: "All plugins enabled for org/repo should be returned for a org/repo query",
			pluginMap: map[string][]string{
				"org1":      []string{"plugin1", "plugin2"},
				"org1/repo": []string{"plugin3"},
			},
			owner:           "org1",
			repo:            "repo",
			expectedPlugins: []string{"plugin1", "plugin2", "plugin3"},
		},
		{
			name: "Plugins for org1/repo should not be returned for org2/repo query",
			pluginMap: map[string][]string{
				"org1":      []string{"plugin1", "plugin2"},
				"org1/repo": []string{"plugin3"},
			},
			owner:           "org2",
			repo:            "repo",
			expectedPlugins: nil,
		},
		{
			name: "Plugins for org1 should not be returned for org2/repo query",
			pluginMap: map[string][]string{
				"org1":      []string{"plugin1", "plugin2"},
				"org2/repo": []string{"plugin3"},
			},
			owner:           "org2",
			repo:            "repo",
			expectedPlugins: []string{"plugin3"},
		},
	}
	for _, tc := range testcases {
		pa := PluginAgent{}
		pa.ps = tc.pluginMap

		plugins := pa.getPlugins(tc.owner, tc.repo)
		if !reflect.DeepEqual(plugins, tc.expectedPlugins) {
			t.Errorf("Expected: %#v, Got %#v in case %s.", tc.expectedPlugins, plugins, tc.name)
		}
	}
}
