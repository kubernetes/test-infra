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
	"testing"

	"github.com/golang/protobuf/proto"
	"k8s.io/test-infra/testgrid/config/pb"
	"k8s.io/test-infra/testgrid/config/yaml2proto"
)

func TestConfig(t *testing.T) {
	yamlData, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		t.Errorf("IO Error : Cannot Open File config.yaml\n")
	}

	protobufData, err := yaml2proto.Yaml2Proto(yamlData)

	if err != nil {
		t.Errorf("Yaml2Proto - Conversion Error %v\n", err)
	}

	config := &config.Configuration{}
	if err := proto.Unmarshal(protobufData, config); err != nil {
		t.Errorf("Failed to parse config: %v\n", err)
	}

	// Validate config.yaml -

	// testgroup - occurance map, validate testgroups
	testgroupMap := make(map[string]int32)

	for testgroupidx, testgroup := range config.TestGroups {
		// All testgroup must have a name and a query
		if testgroup.Name == "" || testgroup.GcsPrefix == "" {
			t.Errorf("Testgroup %v: - Must have a name and query\n", testgroupidx)
		}

		// All testgroup must not have duplicated names
		if testgroupMap[testgroup.Name] > 0 {
			t.Errorf("Duplicated Testgroup: %v\n", testgroup.Name)
		} else {
			testgroupMap[testgroup.Name] = 1
		}

		if !testgroup.IsExternal {
			t.Errorf("Testgroup %v: IsExternal should always be true!", testgroup.Name)
		}
		if !testgroup.UseKubernetesClient {
			t.Errorf("Testgroup %v: UseKubernetesClient should always be true!", testgroup.Name)
		}
	}

	// dashboard name set
	dashboardmap := make(map[string]bool)

	for dashboardidx, dashboard := range config.Dashboards {
		// All dashboard must have a name
		if dashboard.Name == "" {
			t.Errorf("Dashboard %v: - Must have a name\n", dashboardidx)
		}

		// All dashboard must not have duplicated names
		if dashboardmap[dashboard.Name] {
			t.Errorf("Duplicated dashboard: %v\n", dashboard.Name)
		} else {
			dashboardmap[dashboard.Name] = true
		}

		// All dashboard must have at least one tab
		if len(dashboard.DashboardTab) == 0 {
			t.Errorf("Dashboard %v: - Must have more than one dashboardtab\n", dashboard.Name)
		}

		// dashboardtab name set, to check duplicated tabs within each dashboard
		dashboardtabmap := make(map[string]bool)
			
		// All notifications in dashboard must have a summary
		if len(dashboard.Notifications) != 0 {
		  for notificationindex, notification := range dashboard.Notifications {
		    if notification.Summary == "" {
		      t.Errorf("Notification %v in dashboard %v: - Must have a summary\n", notificationindex, dashboard.Name)
		    }
		  }
		}

		for tabindex, dashboardtab := range dashboard.DashboardTab {

			// All dashboardtab must have a name and a testgroup
			if dashboardtab.Name == "" || dashboardtab.TestGroupName == "" {
				t.Errorf("Dashboard %v, tab %v: - Must have a name and a testgroup name\n", dashboard.Name, tabindex)
			}

			// All dashboardtab within a dashboard must not have duplicated names
			if dashboardtabmap[dashboardtab.Name] {
				t.Errorf("Duplicated dashboardtab: %v\n", dashboardtab.Name)
			} else {
				dashboardtabmap[dashboardtab.Name] = true
			}

			// All testgroup in dashboard must be defined in testgroups
			if testgroupMap[dashboardtab.TestGroupName] == 0 {
				t.Errorf("Dashboard %v, tab %v: - Testgroup %v must be defined first\n",
					dashboard.Name, dashboardtab.Name, dashboardtab.TestGroupName)
			} else {
				testgroupMap[dashboardtab.TestGroupName] += 1
			}
		}
	}

	// All Testgroup should be mapped to one or more tabs
	for testgroupname, occurance := range testgroupMap {
		if occurance == 1 {
			t.Errorf("Testgroup %v - defined but not used in any dashboards", testgroupname)
		}
	}
}
