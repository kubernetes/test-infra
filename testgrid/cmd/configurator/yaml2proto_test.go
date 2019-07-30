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
	"testing"
)

func TestYaml2Proto_IsExternal_And_UseKuberClient_False(t *testing.T) {
	yaml :=
		`default_test_group:
  name: default
default_dashboard_tab:
  name: default
test_groups:
- name: testgroup_1
dashboards:
- name: dashboard_1`

	c := Config{}

	if err := c.UpdateDefaults([]byte(yaml)); err != nil {
		t.Errorf("Convert Error: %v\n", err)
	}

	if err := c.Update([]byte(yaml)); err != nil {
		t.Errorf("Convert Error: %v\n", err)
	}

	config, err := c.Raw()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		t.FailNow()
	}
	for _, testgroup := range config.TestGroups {
		if !testgroup.IsExternal {
			t.Errorf("IsExternal should always be true!")
		}

		if !testgroup.UseKubernetesClient {
			t.Errorf("UseKubernetesClient should always be true!")
		}
	}
}

func TestUpdateDefaults_Validity(t *testing.T) {
	tests := []struct {
		yaml            string
		expectedMissing string
	}{
		{
			yaml:            ``,
			expectedMissing: "DefaultTestGroup",
		},
		{
			yaml: `default_test_group:
  name: default`,
			expectedMissing: "DefaultDashboardTab",
		},
		{
			yaml: `default_test_group:
  name: default
default_dashboard_tab:
  name: default`,
			expectedMissing: "",
		},
	}

	for index, test := range tests {
		c := Config{}
		err := c.UpdateDefaults([]byte(test.yaml))
		if err == nil && test.expectedMissing == "" {
			continue
		}
		if err != nil {
			if e, ok := err.(MissingFieldError); ok && e.Field == test.expectedMissing {
				continue
			}
		}
		t.Errorf("Test %v fails. expected MissingFieldError(%s), actual error: %v", index, test.expectedMissing, err)
	}
}

func TestUpdate_Validate(t *testing.T) {
	defaultYaml := `default_test_group:
  name: default
default_dashboard_tab:
  name: default`

	tests := []struct {
		yaml            string
		expectedMissing string
	}{
		{
			yaml:            ``,
			expectedMissing: "TestGroups",
		},
		{
			yaml: `dashboards:
- name: dashboard_1`,
			expectedMissing: "TestGroups",
		},
		{
			yaml: `test_groups:
- name: testgroup_1`,
			expectedMissing: "Dashboards",
		},
		{
			yaml: `dashboards:
- name: dashboard_1
test_groups:
- name: testgroup_1`,
			expectedMissing: "",
		},
	}

	for index, test := range tests {
		c := Config{}
		if err := c.UpdateDefaults([]byte(defaultYaml)); err != nil {
			t.Errorf("Unexpected error with default yaml %s", defaultYaml)
		}

		if err := c.Update([]byte(test.yaml)); err != nil {
			t.Errorf("Unexpected error in Update(test[%d].yaml): %v", index, err)
		}
		err := c.validate()
		if err == nil && test.expectedMissing == "" {
			continue
		}
		if err != nil {
			if e, ok := err.(MissingFieldError); ok && e.Field == test.expectedMissing {
				continue
			}
		}
		t.Errorf("Test %v fails. expected MissingFieldError(%s), actual error: %v", index, test.expectedMissing, err)
	}
}

func TestUpdate_DefaultInherits(t *testing.T) {
	defaultYaml := `default_test_group:
  num_columns_recent: 10
default_dashboard_tab:
  num_columns_recent: 20`

	tests := []struct {
		name              string
		yaml              string
		expectedTestGroup int32
		expectedDashTab   int32
	}{
		{
			name: "Default Settings",
			yaml: `dashboards:
- name: dashboard_1
  dashboard_tab:
  - name: tab_1
test_groups:
- name: testgroup_1`,
			expectedTestGroup: 10,
			expectedDashTab:   20,
		},
		{
			name: "DashboardTab Inheritance",
			yaml: `dashboards:
- name: dashboard_1
  dashboard_tab:
  - name: tab_1
    num_columns_recent: 3
test_groups:
- name: testgroup_1`,
			expectedTestGroup: 10,
			expectedDashTab:   3,
		},
		{
			name: "TestGroup Inheritance",
			yaml: `dashboards:
- name: dashboard_1
  dashboard_tab:
  - name: tab_1
test_groups:
- name: testgroup_1
  num_columns_recent: 4`,
			expectedTestGroup: 4,
			expectedDashTab:   20,
		},
		{
			name: "Doesn't inherit imbedded defaults",
			yaml: `default_test_group:
  num_columns_recent: 5
default_dashboard_tab:
  num_columns_recent: 6
dashboards:
- name: dashboard_1
  dashboard_tab:
  - name: tab_1
test_groups:
- name: testgroup_1`,
			expectedTestGroup: 10,
			expectedDashTab:   20,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := Config{}
			if err := c.UpdateDefaults([]byte(defaultYaml)); err != nil {
				t.Fatalf("Unexpected error with default yaml: %v", err)
			}

			if err := c.Update([]byte(test.yaml)); err != nil {
				t.Fatalf("Unexpected error with update: %v", err)
			}

			if c.config.TestGroups[0].NumColumnsRecent != test.expectedTestGroup {
				t.Errorf("Wrong inheritance for TestGroup: got %d, expected %d",
					c.config.TestGroups[0].NumColumnsRecent, test.expectedTestGroup)
			}

			if c.config.Dashboards[0].DashboardTab[0].NumColumnsRecent != test.expectedDashTab {
				t.Errorf("Wrong inheritance for Dashboard Tab: got %d, expected %d",
					c.config.Dashboards[0].DashboardTab[0].NumColumnsRecent, test.expectedDashTab)
			}

		})
	}
}
