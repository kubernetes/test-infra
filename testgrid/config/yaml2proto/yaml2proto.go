/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"fmt"
	"errors"
	"gopkg.in/yaml.v2"

	"github.com/golang/protobuf/proto"
	pb "k8s.io/test-infra/testgrid/config/pb"
)

// Set up unfilled field in a TestGroup using the default TestGroup
func ValidateTestGroup(currentTestGroup *pb.TestGroup, defaultTestGroup *pb.TestGroup) {
	if currentTestGroup.DaysOfResults == 0 {
		currentTestGroup.DaysOfResults = defaultTestGroup.DaysOfResults
	}

	if currentTestGroup.TestsNamePolicy == pb.TestGroup_TESTS_NAME_MIN {
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
func ValidateDashboardtab(currentTab *pb.DashboardTab, defaultTab *pb.DashboardTab){
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
	config := &pb.Configuration{}
	err := yaml.Unmarshal(yamlData,&config)
	if err != nil {
		fmt.Printf("Unmarshal Error : %v\n", err);
		return nil, err
	}

	// Reject empty yaml ( No testgroups or dashboards )
	if len(config.TestGroups) == 0 {
		return nil, errors.New("Invalid Yaml : No Valid Testgroups")
	}

	if len(config.Dashboards) == 0 {
		return nil, errors.New("Invalid Yaml : No Valid Dashboards")
	}

	if config.DefaultTestGroup == nil || config.DefaultDashboardTab == nil {
		return nil, errors.New("Please Include The Default Testgroup & Dashboardtab")
	}

	// validating testgroups
	for _,testgroup := range config.TestGroups {
		ValidateTestGroup(testgroup,config.DefaultTestGroup)
	}

	// validating dashboards
	for _,dashboard := range config.Dashboards {
		// validate dashboard tabs
		for _,dashboardtab := range dashboard.DashboardTab {
			ValidateDashboardtab(dashboardtab,config.DefaultDashboardTab)
		}
	}

	// Marshal config to protobuf
	return proto.Marshal(config)
}
