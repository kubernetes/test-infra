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

	responsepb "k8s.io/test-infra/testgrid/cmd/summarizer/response"
	summarypb "k8s.io/test-infra/testgrid/summary"
)

// TableToSummary converts the input queryData collected from test results to
// the DashboardTabSummary proto. It needs the dashboardName from the config
// file and the updateTimestamp of when the updater collected the test results.
func TableToSummary(queryData *responsepb.Response) (summarypb.DashboardTabSummary, error) {
	s := extractBasicSummary(queryData)
	s.FailingTestSummaries = extractTests(queryData)
	return s, nil
}

func extractBasicSummary(queryData *responsepb.Response) summarypb.DashboardTabSummary {
	bugURL := ""
	if queryData.OpenBugTemplate != nil && len(queryData.OpenBugTemplate.Url) > 0 {
		bugURL = queryData.OpenBugTemplate.Url
	}
	var lastRunTimestamp int64 = 0
	for i, e := range queryData.Timestamps {
		if i == 0 || e > lastRunTimestamp {
			lastRunTimestamp = e
		}
	}
	overallStatus := summarypb.DashboardTabSummary_UNKNOWN
	if queryData.OverallStatus > 0 {
		overallStatus = queryData.OverallStatus
	}

	return summarypb.DashboardTabSummary{
		DashboardName:       queryData.DashboardName,
		DashboardTabName:    queryData.DashboardTab.Name,
		Alert:               queryData.Alerts,
		LastUpdateTimestamp: float64(queryData.UpdateTimestamp),
		LastRunTimestamp:    float64(lastRunTimestamp),
		LatestGreen:         queryData.LatestGreen,
		OverallStatus:       overallStatus,
		Status:              queryData.Summary,
		BugUrl:              bugURL,
	}
}

func appendQuery(query, newKey, newVal string) string {
	if len(query) > 0 {
		return fmt.Sprintf("%s&%s=%s", query, newKey, newVal)
	} else {
		return fmt.Sprintf("%s=%s", newKey, newVal)
	}
}

func buildFailingTestSummary(queryData *responsepb.Response, r *responsepb.Row) *summarypb.FailingTestSummary {
	target := r.Target
	failTestID := r.Alert.TestId
	failTestLink := ""
	if len(failTestID) > 0 {
		template := queryData.OpenTestTemplate
		URLOptions := ""
		for _, option := range template.Options {
			URLOptions = appendQuery(URLOptions, option.Key, option.Value)
		}
		queryString := url.QueryEscape(URLOptions)
		failTestLink = fmt.Sprintf("%s?%s", template.Url, queryString)
	}

	return &summarypb.FailingTestSummary{
		DisplayName:    r.Name,
		TestName:       target,
		FailBuildId:    r.Alert.FailBuildId,
		FailTimestamp:  float64(r.Alert.FailTime),
		PassBuildId:    r.Alert.PassBuildId,
		PassTimestamp:  float64(r.Alert.PassTime),
		FailCount:      int32(r.Alert.FailCount),
		BuildLink:      r.Alert.Link,
		BuildLinkText:  r.Alert.LinkText,
		BuildUrlText:   r.Alert.UrlText,
		FailureMessage: r.Alert.Message,
		LinkedBugs:     r.LinkedBugs,
		FailTestLink:   failTestLink,
	}
}

func extractTests(queryData *responsepb.Response) []*summarypb.FailingTestSummary {
	var failingTestSummaries []*summarypb.FailingTestSummary
	for _, test := range queryData.Tests {
		failingTest := buildFailingTestSummary(queryData, test)
		failingTestSummaries = append(failingTestSummaries, failingTest)
	}

	return failingTestSummaries
}
