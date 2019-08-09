/*
Copyright 2019 The Kubernetes Authors.

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
	"fmt"
	"net/url"
	"testing"
	"time"

	responsepb "k8s.io/test-infra/testgrid/cmd/summarizer/response"
	configpb "k8s.io/test-infra/testgrid/config"
	summarypb "k8s.io/test-infra/testgrid/summary"

	"github.com/google/go-cmp/cmp"
)

func TestTableToSummary(t *testing.T) {
	mockTestRows := []*responsepb.Row{
		{
			Name:       "fake-test-1",
			Target:     "fake-test-1-target",
			Messages:   []string{"FooError", "", "BarException"},
			LinkedBugs: []string{"123", "456"},
			Alert: &responsepb.TestAlert{
				FailCount:   1,
				FailTime:    2,
				PassTime:    3,
				Link:        "fake-link-1",
				LinkText:    "fake-link-1-text",
				UrlText:     "url",
				FailBuildId: "100",
				Message:     "BarException",
				TestId:      "fake-id-1",
			},
		},
		{
			Name:       "fake-test-2",
			Target:     "fake-test-2-target",
			Messages:   []string{},
			LinkedBugs: []string{},
			Alert: &responsepb.TestAlert{
				FailCount:   4,
				FailTime:    5,
				PassTime:    6,
				Link:        "fake-link-2",
				LinkText:    "fake-link-2-text",
				UrlText:     "url",
				FailBuildId: "200",
				Message:     "",
				TestId:      "",
			},
		},
	}
	mockAlertOptions := configpb.DashboardTabAlertOptions{
		AlertMailToAddresses: "mock-to@",
		NumFailuresToAlert:   2,
	}
	mockDashboardTabConfig := configpb.DashboardTab{
		Name:           "tab",
		TestGroupName:  "test-group",
		AlertOptions:   &mockAlertOptions,
		CodeSearchPath: "code-path/",
	}
	mockQueryData := responsepb.Response{
		Alerts:        "fake-alert",
		LatestGreen:   "50",
		OverallStatus: summarypb.DashboardTabSummary_FAIL,
		Summary:       "fake-summary",
		Tests:         mockTestRows,
		Timestamps:    []int64{3, 2, 1},
		OpenTestTemplate: &configpb.LinkTemplate{
			Url: "https://url.com/target",
			Options: []*configpb.LinkOptionsTemplate{
				{
					Key:   "id",
					Value: "<test-id>",
				},
				{
					Key:   "target",
					Value: "<test-name>",
				},
			},
		},
		DashboardTab:    &mockDashboardTabConfig,
		DashboardName:   "dashboard",
		UpdateTimestamp: time.Now().Unix(),
	}

	tabSummary, err := TableToSummary(&mockQueryData)

	if err != nil {
		t.Errorf("failed to convert table to summary: %v", err)
	}

	if tabSummary.DashboardName != mockQueryData.DashboardName {
		t.Errorf("incorrect dashboard name, got '%s', expected '%s'", tabSummary.DashboardName, mockQueryData.DashboardName)
	}

	if tabSummary.DashboardTabName != mockDashboardTabConfig.Name {
		t.Errorf("incorrect dashboard tab name, got '%s', expected '%s'", tabSummary.DashboardTabName, mockDashboardTabConfig.Name)
	}

	if tabSummary.Alert != mockQueryData.Alerts {
		t.Errorf("incorrect tab summary alert, got '%s', expected '%s'", tabSummary.Alert, mockQueryData.Alerts)
	}

	if tabSummary.LastRunTimestamp != float64(3) {
		t.Errorf("incorrect tab summary last run timestamp, got '%f', expected '%f'", tabSummary.LastRunTimestamp, float64(3))
	}

	if tabSummary.LatestGreen != mockQueryData.LatestGreen {
		t.Errorf("incorrect tab summary latest green value, got '%s', expected '%s'", tabSummary.LatestGreen, mockQueryData.LatestGreen)
	}

	if tabSummary.OverallStatus != mockQueryData.OverallStatus {
		t.Errorf("incorrect tab summary overall status, got '%d', expected '%d'", tabSummary.OverallStatus, mockQueryData.OverallStatus)
	}

	if tabSummary.Status != mockQueryData.Summary {
		t.Errorf("incorrect tab summary status, got '%s', expected '%s'", tabSummary.Status, mockQueryData.Summary)
	}

	if tabSummary.LastUpdateTimestamp != float64(mockQueryData.UpdateTimestamp) {
		t.Errorf("incorrect tab summary last update timestamp, got '%f', expected '%f'", tabSummary.LastUpdateTimestamp, float64(mockQueryData.UpdateTimestamp))
	}

	failingTestOne := summarypb.FailingTestSummary{
		DisplayName:    mockTestRows[0].Name,
		TestName:       mockTestRows[0].Target,
		FailBuildId:    mockTestRows[0].Alert.FailBuildId,
		FailTimestamp:  float64(mockTestRows[0].Alert.FailTime),
		PassBuildId:    mockTestRows[0].Alert.PassBuildId,
		PassTimestamp:  float64(mockTestRows[0].Alert.PassTime),
		FailCount:      int32(mockTestRows[0].Alert.FailCount),
		BuildLink:      mockTestRows[0].Alert.Link,
		BuildLinkText:  mockTestRows[0].Alert.LinkText,
		BuildUrlText:   mockTestRows[0].Alert.UrlText,
		FailureMessage: mockTestRows[0].Alert.Message,
		LinkedBugs:     mockTestRows[0].LinkedBugs,
		FailTestLink:   fmt.Sprintf("%s?%s", mockQueryData.OpenTestTemplate.Url, url.QueryEscape("id=<test-id>&target=<test-name>")),
	}
	if !cmp.Equal(*tabSummary.FailingTestSummaries[0], failingTestOne) {
		t.Errorf("incorrect failed fake test 1 summary, got '%v', expected '%v'", tabSummary.FailingTestSummaries[0], failingTestOne)
	}
}
