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

package yaml2proto

import (
	"errors"
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
	err := c.Update([]byte(yaml))

	if err != nil {
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
		yaml          string
		expectedError error
	}{
		{
			yaml:          ``,
			expectedError: errors.New("missing DefaultTestGroup"),
		},
		{
			yaml: `default_test_group:
  name: default`,
			expectedError: errors.New("missing DefaultDashboardTab"),
		},
		{
			yaml: `default_test_group:
  name: default
default_dashboard_tab:
  name: default`,
			expectedError: nil,
		},
	}

	for index, test := range tests {
		c := Config{}
		err := c.Update([]byte(test.yaml))
		if (err == nil && test.expectedError == nil) ||
			(err != nil && test.expectedError != nil && err.Error() == test.expectedError.Error()) {
			continue
		} else {
			t.Errorf("Test %v fails. ExpectedError: %v, actual error: %v", index, test.expectedError, err)
		}
	}
}
func TestUpdate_Validate(t *testing.T) {
	defaultYaml := `default_test_group:
  name: default
default_dashboard_tab:
  name: default`

	tests := []struct {
		yaml          string
		expectedError error
	}{
		{
			yaml:          ``,
			expectedError: errors.New("Invalid YAML : No Valid Testgroups"),
		},
		{
			yaml: `dashboards:
- name: dashboard_1`,
			expectedError: errors.New("Invalid YAML : No Valid Testgroups"),
		},
		{
			yaml: `test_groups:
- name: testgroup_1`,
			expectedError: errors.New("Invalid YAML : No Valid Dashboards"),
		},
		{
			yaml: `dashboards:
- name: dashboard_1
test_groups:
- name: testgroup_1`,
			expectedError: nil,
		},
	}

	for index, test := range tests {
		c := Config{}
		if err := c.Update([]byte(defaultYaml)); err != nil {
			t.Errorf("Unexpected error in Update(defaultYaml): %v", err)
		}
		if err := c.Update([]byte(test.yaml)); err != nil {
			t.Errorf("Unexpected error in Update(test[%d].yaml): %v", index, err)
		}
		err := c.validate()
		if (err == nil && test.expectedError == nil) ||
			(err != nil && test.expectedError != nil && err.Error() == test.expectedError.Error()) {
			continue
		} else {
			t.Errorf("Test %v fails. ExpectedError: %v, actual error: %v", index, test.expectedError, err)
		}
	}
}
