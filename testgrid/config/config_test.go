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
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
	"k8s.io/test-infra/testgrid/config/yaml2proto"
	"path/filepath"
)

type SQConfig struct {
	Data map[string]string `yaml:"data,omitempty"`
}

var (
	companies = []string{
		"canonical",
		"cri-o",
		"google",
		"kopeio",
		"tectonic",
		"redhat",
	}
	orgs = []string{
		"presubmits",
		"sig",
		"wg",
	}
	prefixes = [][]string{orgs, companies}
)

func TestConfig(t *testing.T) {
	yamlData, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		t.Errorf("IO Error : Cannot Open File config.yaml")
	}

	c := yaml2proto.Config{}
	if err := c.Update(yamlData); err != nil {
		t.Errorf("Yaml2Proto - Conversion Error %v", err)
	}

	config, err := c.Raw()
	if err != nil {
		t.Errorf("Error validating config: %v", err)
	}

	// Validate config.yaml -

	// testgroup - occurrence map, validate testgroups
	testgroupMap := make(map[string]int32)

	for testgroupidx, testgroup := range config.TestGroups {
		// All testgroup must have a name and a query
		if testgroup.Name == "" || testgroup.GcsPrefix == "" {
			t.Errorf("Testgroup %v: - Must have a name and query", testgroupidx)
		}

		// All testgroup must not have duplicated names
		if testgroupMap[testgroup.Name] > 0 {
			t.Errorf("Duplicated Testgroup: %v", testgroup.Name)
		} else {
			testgroupMap[testgroup.Name] = 1
		}

		if !testgroup.IsExternal {
			t.Errorf("Testgroup %v: IsExternal should always be true!", testgroup.Name)
		}
		if !testgroup.UseKubernetesClient {
			t.Errorf("Testgroup %v: UseKubernetesClient should always be true!", testgroup.Name)
		}

		if strings.HasPrefix(testgroup.GcsPrefix, "kubernetes-jenkins/logs/") {
			// The expectation is that testgroup.Name is the name of a Prow job and the GCSPrefix
			// follows the convention kubernetes-jenkins/logs/.../jobName
			// The final part of the prefix should be the job name.
			expected := filepath.Join(filepath.Dir(testgroup.GcsPrefix), testgroup.Name)
			if expected != testgroup.GcsPrefix {
				t.Errorf("Kubernetes Testgroup %v GcsPrefix; Got %v; Want %v", testgroup.Name, testgroup.GcsPrefix, expected)
			}
		}

		if testgroup.TestNameConfig != nil {
			if testgroup.TestNameConfig.NameFormat == "" {
				t.Errorf("Testgroup %v: NameFormat must not be empty!", testgroup.Name)
			}

			if len(testgroup.TestNameConfig.NameElements) != strings.Count(testgroup.TestNameConfig.NameFormat, "%") {
				t.Errorf("Testgroup %v: TestNameConfig must have number NameElement equal to format count in NameFormat!", testgroup.Name)
			}
		}

		// All PR testgroup has num_recent_column equals 20
		if strings.HasPrefix(testgroup.GcsPrefix, "kubernetes-jenkins/pr-logs/directory/") {
			if testgroup.NumColumnsRecent < 20 {
				t.Errorf("Testgroup %v: num_recent_column must be greater than 20 for presubmit jobs!", testgroup.Name)
			}
		}
	}

	// dashboard name set
	dashboardmap := make(map[string]bool)

	for dashboardidx, dashboard := range config.Dashboards {
		// All dashboard must have a name
		if dashboard.Name == "" {
			t.Errorf("Dashboard %v: - Must have a name", dashboardidx)
		}

		found := false
		for _, kind := range prefixes {
			for _, prefix := range kind {
				if strings.HasPrefix(dashboard.Name, prefix+"-") || dashboard.Name == prefix {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("Dashboard %v: must prefix with one of: %v", dashboard.Name, prefixes)
		}

		// All dashboard must not have duplicated names
		if dashboardmap[dashboard.Name] {
			t.Errorf("Duplicated dashboard: %v", dashboard.Name)
		} else {
			dashboardmap[dashboard.Name] = true
		}

		// All dashboard must have at least one tab
		if len(dashboard.DashboardTab) == 0 {
			t.Errorf("Dashboard %v: - Must have more than one dashboardtab", dashboard.Name)
		}

		// dashboardtab name set, to check duplicated tabs within each dashboard
		dashboardtabmap := make(map[string]bool)

		// All notifications in dashboard must have a summary
		if len(dashboard.Notifications) != 0 {
			for notificationindex, notification := range dashboard.Notifications {
				if notification.Summary == "" {
					t.Errorf("Notification %v in dashboard %v: - Must have a summary", notificationindex, dashboard.Name)
				}
			}
		}

		for tabindex, dashboardtab := range dashboard.DashboardTab {

			// All dashboardtab must have a name and a testgroup
			if dashboardtab.Name == "" || dashboardtab.TestGroupName == "" {
				t.Errorf("Dashboard %v, tab %v: - Must have a name and a testgroup name", dashboard.Name, tabindex)
			}

			// All dashboardtab within a dashboard must not have duplicated names
			if dashboardtabmap[dashboardtab.Name] {
				t.Errorf("Duplicated dashboardtab: %v", dashboardtab.Name)
			} else {
				dashboardtabmap[dashboardtab.Name] = true
			}

			// All testgroup in dashboard must be defined in testgroups
			if testgroupMap[dashboardtab.TestGroupName] == 0 {
				t.Errorf("Dashboard %v, tab %v: - Testgroup %v must be defined first",
					dashboard.Name, dashboardtab.Name, dashboardtab.TestGroupName)
			} else {
				testgroupMap[dashboardtab.TestGroupName] += 1
			}

			if dashboardtab.AlertOptions != nil && (dashboardtab.AlertOptions.AlertStaleResultsHours != 0 || dashboardtab.AlertOptions.NumFailuresToAlert != 0) {
				for _, testgroup := range config.TestGroups {
					// Disallow alert options in tab but not group.
					// Disallow different alert options in tab vs. group.
					if testgroup.Name == dashboardtab.TestGroupName {
						if testgroup.AlertStaleResultsHours == 0 {
							t.Errorf("Cannot define alert_stale_results_hours in DashboardTab %v and not TestGroup %v.", dashboardtab.Name, dashboardtab.TestGroupName)
						}
						if testgroup.NumFailuresToAlert == 0 {
							t.Errorf("Cannot define num_failures_to_alert in DashboardTab %v and not TestGroup %v.", dashboardtab.Name, dashboardtab.TestGroupName)
						}
						if testgroup.AlertStaleResultsHours != dashboardtab.AlertOptions.AlertStaleResultsHours {
							t.Errorf("alert_stale_results_hours for DashboardTab %v must match TestGroup %v.", dashboardtab.Name, dashboardtab.TestGroupName)
						}
						if testgroup.NumFailuresToAlert != dashboardtab.AlertOptions.NumFailuresToAlert {
							t.Errorf("num_failures_to_alert for DashboardTab %v must match TestGroup %v.", dashboardtab.Name, dashboardtab.TestGroupName)
						}
					}
				}
			}
		}
	}

	// No dup of dashboard groups, and no dup dashboard in a dashboard group
	groups := make(map[string]bool)
	tabs := make(map[string]string)

	for idx, dashboardGroup := range config.DashboardGroups {
		// All dashboard must have a name
		if dashboardGroup.Name == "" {
			t.Errorf("DashboardGroup %v: - DashboardGroup must have a name", idx)
		}

		found := false
		for _, kind := range prefixes {
			for _, prefix := range kind {
				if strings.HasPrefix(dashboardGroup.Name, prefix+"-") || prefix == dashboardGroup.Name {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("Dashboard group %v: must prefix with one of: %v", dashboardGroup.Name, prefixes)
		}

		// All dashboardgroup must not have duplicated names
		if _, ok := groups[dashboardGroup.Name]; ok {
			t.Errorf("Duplicated dashboard: %v", dashboardGroup.Name)
		} else {
			groups[dashboardGroup.Name] = true
		}

		if _, ok := dashboardmap[dashboardGroup.Name]; ok {
			t.Errorf("%v is both a dashboard and dashboard group name.", dashboardGroup.Name)
		}

		for _, dashboard := range dashboardGroup.DashboardNames {
			// All dashboard must not have duplicated names
			if exist, ok := tabs[dashboard]; ok {
				t.Errorf("Duplicated dashboard %v in dashboard group %v and %v", dashboard, exist, dashboardGroup.Name)
			} else {
				tabs[dashboard] = dashboardGroup.Name
			}

			if _, ok := dashboardmap[dashboard]; !ok {
				t.Errorf("Dashboard %v needs to be defined before adding to a dashboard group!", dashboard)
			}

			if !strings.HasPrefix(dashboard, dashboardGroup.Name+"-") {
				t.Errorf("Dashboard %v in group %v must have the group name as a prefix", dashboard, dashboardGroup.Name)
			}
		}
	}

	// All Testgroup should be mapped to one or more tabs
	for testgroupname, occurrence := range testgroupMap {
		if occurrence == 1 {
			t.Errorf("Testgroup %v - defined but not used in any dashboards", testgroupname)
		}
	}

	// make sure items in sq-blocking dashboard matches sq configmap
	sqJobPool := []string{}
	for _, d := range config.Dashboards {
		if d.Name != "sq-blocking" {
			continue
		}

		for _, tab := range d.DashboardTab {
			for _, t := range config.TestGroups {
				if t.Name == tab.TestGroupName {
					job := strings.TrimPrefix(t.GcsPrefix, "kubernetes-jenkins/logs/")
					sqJobPool = append(sqJobPool, job)
					break
				}
			}
		}
	}

	sqConfigPath := "../../mungegithub/submit-queue/deployment/kubernetes/configmap.yaml"
	configData, err := ioutil.ReadFile(sqConfigPath)
	if err != nil {
		t.Errorf("Read Buffer Error for SQ Data : %v", err)
	}

	sqData := &SQConfig{}
	err = yaml.Unmarshal([]byte(configData), &sqData)
	if err != nil {
		t.Errorf("Unmarshal Error for SQ Data : %v", err)
	}

	for _, testgridJob := range sqJobPool {
		t.Errorf("Err : testgrid job %v not found in SQ config", testgridJob)
	}

	sqNonBlockingJobs := strings.Split(sqData.Data["nonblocking-jobs"], ",")
	for _, sqJob := range sqNonBlockingJobs {
		if sqJob == "" { // ignore empty list of jobs
			continue
		}
		found := false
		for _, testgroup := range config.TestGroups {
			if testgroup.Name == sqJob {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Err : %v not found in testgrid config", sqJob)
		}
	}
}
