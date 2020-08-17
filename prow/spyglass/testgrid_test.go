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

package spyglass

import (
	"testing"

	tgconf "github.com/GoogleCloudPlatform/testgrid/pb/config"
)

func TestFindQuery(t *testing.T) {
	testCases := []struct {
		name       string
		dashboards []*tgconf.Dashboard
		jobName    string
		expected   []string
		expectErr  bool
	}{
		{
			name: "simple lookup",
			dashboards: []*tgconf.Dashboard{
				{
					Name: "dashboard-a",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab-a",
							TestGroupName: "test-group-a",
						},
					},
				},
				{
					Name: "dashboard-b",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab-b",
							TestGroupName: "test-group-b",
						},
						{
							Name:          "tab-c",
							TestGroupName: "test-group-c",
						},
						{
							Name:          "tab-d",
							TestGroupName: "test-group-d",
						},
					},
				},
				{
					Name: "dashboard-c",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab-e",
							TestGroupName: "test-group-e",
						},
					},
				},
			},
			jobName:  "test-group-c",
			expected: []string{"dashboard-b#tab-c"},
		},
		{
			name: "ambiguous dashboard",
			dashboards: []*tgconf.Dashboard{
				{
					Name: "dashboard-a",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab-a",
							TestGroupName: "popular-group",
						},
					},
				},
				{
					Name: "dashboard-b",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab-b",
							TestGroupName: "popular-group",
						},
					},
				},
				{
					Name: "dashboard-c",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab-c",
							TestGroupName: "test-group-c",
						},
					},
				},
			},
			jobName:  "popular-group",
			expected: []string{"dashboard-a#tab-a", "dashboard-b#tab-b"},
		},
		{
			name: "ambiguous tab",
			dashboards: []*tgconf.Dashboard{
				{
					Name: "dashboard-a",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab-a",
							TestGroupName: "popular-group",
						},
						{
							Name:          "tab-b",
							TestGroupName: "popular-group",
						},
					},
				},
				{
					Name: "dashboard-b",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab-c",
							TestGroupName: "test-group-c",
						},
					},
				},
			},
			jobName:  "popular-group",
			expected: []string{"dashboard-a#tab-a", "dashboard-a#tab-b"},
		},
		{
			name: "dashboard with more tabs is preferred",
			dashboards: []*tgconf.Dashboard{
				{
					Name: "dashboard-a",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab",
							TestGroupName: "different-group",
						},
						{
							Name:          "tab-b",
							TestGroupName: "popular-group",
						},
					},
				},
				{
					Name: "dashboard-b",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab",
							TestGroupName: "popular-group",
						},
					},
				},
				{
					Name: "dashboard-c",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab-d",
							TestGroupName: "something-else",
						},
					},
				},
			},
			jobName:  "popular-group",
			expected: []string{"dashboard-a#tab-b"},
		},
		{
			name: "tab with base_options can be selected",
			dashboards: []*tgconf.Dashboard{
				{
					Name: "dashboard-a",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab",
							TestGroupName: "group-a",
							BaseOptions:   "some sort of options",
						},
					},
				},
				{
					Name: "dashboard-b",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab",
							TestGroupName: "group-b",
						},
					},
				},
			},
			jobName:  "group-a",
			expected: []string{"dashboard-a#tab"},
		},
		{
			name: "tab without base_options is preferred even if dashboard has fewer tabs",
			dashboards: []*tgconf.Dashboard{
				{
					Name: "dashboard-a",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab-a",
							TestGroupName: "popular-group",
							BaseOptions:   "some sort of options",
						},
						{
							Name:          "tab-b",
							TestGroupName: "somewhere-else",
						},
					},
				},
				{
					Name: "dashboard-b",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab-c",
							TestGroupName: "popular-group",
						},
					},
				},
			},
			jobName:  "popular-group",
			expected: []string{"dashboard-b#tab-c"},
		},
		{
			name: "nonexistent job errors",
			dashboards: []*tgconf.Dashboard{
				{
					Name: "dashboard-a",
					DashboardTab: []*tgconf.DashboardTab{
						{
							Name:          "tab-a",
							TestGroupName: "some-group",
						},
					},
				},
			},
			jobName:   "missing-group",
			expectErr: true,
		},
		{
			name: "configuration with no tabs errors",
			dashboards: []*tgconf.Dashboard{
				{
					Name: "dashboard-a",
				},
			},
			jobName:   "missing-group",
			expectErr: true,
		},
		{
			name:       "configuration with no dashboards errors",
			dashboards: []*tgconf.Dashboard{},
			jobName:    "missing-group",
			expectErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tg := TestGrid{c: &tgconf.Configuration{
				Dashboards: tc.dashboards,
			}}
			result, err := tg.FindQuery(tc.jobName)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("Expected an error, but instead got result %q", result)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			found := false
			for _, expectation := range tc.expected {
				if result == expectation {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("Expected one of %v, but got %q", tc.expected, result)
			}
		})
	}
}
