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
	"fmt"
	"gopkg.in/yaml.v2"

	"github.com/golang/protobuf/proto"
	"k8s.io/test-infra/testgrid/config/pb"
)

// Set up unfilled field in a TestGroup using the default TestGroup
func ValidateTestGroup(currentTestGroup *config.TestGroup, defaultTestGroup *config.TestGroup) {
	if currentTestGroup.DaysOfResults == 0 {
		currentTestGroup.DaysOfResults = defaultTestGroup.DaysOfResults
	}

	if currentTestGroup.TestsNamePolicy == config.TestGroup_TESTS_NAME_MIN {
		currentTestGroup.TestsNamePolicy = defaultTestGroup.TestsNamePolicy
	}

	if currentTestGroup.ColumnHeader == nil {
		currentTestGroup.ColumnHeader = defaultTestGroup.ColumnHeader
	}

	// is_external and user_kubernetes_client should always be true
	currentTestGroup.IsExternal = true
	currentTestGroup.UseKubernetesClient = true
}

// Set up unfilled field in a DashboardTab using the default DashboardTab
func ValidateDashboardtab(currentTab *config.DashboardTab, defaultTab *config.DashboardTab) {
	if currentTab.BugComponent == 0 {
		currentTab.BugComponent = defaultTab.BugComponent
	}

	if currentTab.CodeSearchPath == "" {
		currentTab.CodeSearchPath = defaultTab.CodeSearchPath
	}

	if currentTab.OpenTestTemplate == nil {
		currentTab.OpenTestTemplate = defaultTab.OpenTestTemplate
	}

	if currentTab.FileBugTemplate == nil {
		currentTab.FileBugTemplate = defaultTab.FileBugTemplate
	}

	if currentTab.AttachBugTemplate == nil {
		currentTab.AttachBugTemplate = defaultTab.AttachBugTemplate
	}

	if currentTab.ResultsText == "" {
		currentTab.ResultsText = defaultTab.ResultsText
	}

	if currentTab.ResultsUrlTemplate == nil {
		currentTab.ResultsUrlTemplate = defaultTab.ResultsUrlTemplate
	}

	if currentTab.CodeSearchUrlTemplate == nil {
		currentTab.CodeSearchUrlTemplate = defaultTab.CodeSearchUrlTemplate
	}
}

func Yaml2Proto(yamlData []byte) ([]byte, error) {

	// Unmarshal yaml to config
	curConfig := &config.Configuration{}
	defaultConfig := &config.DefaultConfiguration{}
	err := yaml.Unmarshal(yamlData, &curConfig)
	if err != nil {
		fmt.Printf("Unmarshal Error for config : %v\n", err)
		return nil, err
	}

	err = yaml.Unmarshal(yamlData, &defaultConfig)
	if err != nil {
		fmt.Printf("Unmarshal Error for defaultconfig : %v\n", err)
		return nil, err
	}

	// Reject empty yaml ( No testgroups or dashboards )
	if len(curConfig.TestGroups) == 0 {
		return nil, errors.New("Invalid Yaml : No Valid Testgroups")
	}

	if len(curConfig.Dashboards) == 0 {
		return nil, errors.New("Invalid Yaml : No Valid Dashboards")
	}

	if defaultConfig.DefaultTestGroup == nil || defaultConfig.DefaultDashboardTab == nil {
		return nil, errors.New("Please Include The Default Testgroup & Dashboardtab")
	}

	// validating testgroups
	for _, testgroup := range curConfig.TestGroups {
		ValidateTestGroup(testgroup, defaultConfig.DefaultTestGroup)
	}

	// validating dashboards
	for _, dashboard := range curConfig.Dashboards {
		// validate dashboard tabs
		for _, dashboardtab := range dashboard.DashboardTab {
			ValidateDashboardtab(dashboardtab, defaultConfig.DefaultDashboardTab)
		}
	}

	// Marshal config to protobuf
	return proto.Marshal(curConfig)
}
