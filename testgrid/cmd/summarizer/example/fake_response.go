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
	"time"

	responsepb "k8s.io/test-infra/testgrid/cmd/summarizer/response"
	configpb "k8s.io/test-infra/testgrid/config"
	summarypb "k8s.io/test-infra/testgrid/summary"
)

func fakeResponse() (*responsepb.Response, error) {
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
	return &mockQueryData, nil
}
