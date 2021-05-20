/*
Copyright 2021 The Kubernetes Authors.

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
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestMapToGroups(t *testing.T) {
	cases := []struct {
		name         string
		groupsMap    map[string]string
		groupsString string
		orderedIDs   []string
	}{
		{
			name:         "empty map",
			groupsMap:    map[string]string{},
			groupsString: "# UUID\tGroup Name\n",
			orderedIDs:   []string{},
		},
		{
			name: "all uuids same size",
			groupsMap: map[string]string{
				"123456": "Test Project 1",
				"567890": "Test Project 2",
			},
			groupsString: "# UUID\tGroup Name\n123456\tTest Project 1\n567890\tTest Project 2\n",
			orderedIDs:   []string{"123456", "567890"},
		},
		{
			name: "different sized uuid",
			groupsMap: map[string]string{
				"1234":      "Test Project 1",
				"123456789": "Test Project 2",
			},
			groupsString: "# UUID   \tGroup Name\n1234     \tTest Project 1\n123456789\tTest Project 2\n",
			orderedIDs:   []string{"1234", "123456789"},
		},
		{
			name: "Keeps comments",
			groupsMap: map[string]string{
				"1234":      "Test Project 1",
				"123456789": "Test Project 2",
			},
			groupsString: "# UUID   \tGroup Name\n#\n1234     \tTest Project 1\n123456789\tTest Project 2\n",
			orderedIDs:   []string{"#", "1234", "123456789"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.groupsString, mapToGroups(tc.groupsMap, tc.orderedIDs)); diff != "" {
				t.Errorf("mapToGroup returned unexpected value (-want +got):\n%s", diff)
			}
		})
	}

}

func TestGroupsToMap(t *testing.T) {
	cases := []struct {
		name         string
		groupsMap    map[string]string
		groupsString string
		orderedIDs   []string
	}{
		{
			name:         "empty groups",
			groupsMap:    map[string]string{},
			groupsString: "# UUID\tGroup Name\n",
			orderedIDs:   []string{},
		},
		{
			name: "all uuids same size",
			groupsMap: map[string]string{
				"123456": "Test Project 1",
				"567890": "Test Project 2",
			},
			groupsString: "# UUID\tGroup Name\n123456\tTest Project 1\n567890\tTest Project 2\n",
			orderedIDs:   []string{"123456", "567890"},
		},
		{
			name: "different sized uuid",
			groupsMap: map[string]string{
				"1234":      "Test Project 1",
				"123456789": "Test Project 2",
			},
			groupsString: "# UUID   \tGroup Name\n1234     \tTest Project 1\n123456789\tTest Project 2\n",
			orderedIDs:   []string{"1234", "123456789"},
		},
		{
			name: "keeps comments",
			groupsMap: map[string]string{
				"1234":      "Test Project 1",
				"123456789": "Test Project 2",
			},
			groupsString: "# UUID   \tGroup Name\n#\n1234     \tTest Project 1\n123456789\tTest Project 2\n",
			orderedIDs:   []string{"#", "1234", "123456789"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			groupsMap, orderedKeys := groupsToMap(tc.groupsString)
			if diff := cmp.Diff(tc.groupsMap, groupsMap, cmpopts.SortSlices(func(x, y string) bool { return strings.Compare(x, y) > 0 })); diff != "" {
				t.Errorf("groupsToMap returned unexpected map value(-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tc.orderedIDs, orderedKeys); diff != "" {
				t.Errorf("groupsToMap returned unexpected ordered keys value (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConfigToMap(t *testing.T) {
	cases := []struct {
		name         string
		configMap    map[string][]string
		configString string
		orderedIDs   []string
	}{
		{
			name:         "empty config",
			configMap:    map[string][]string{},
			configString: "",
			orderedIDs:   []string{},
		},
		{
			name:         "one section",
			configMap:    map[string][]string{"[access]": {"\towner = group Test Group"}},
			configString: "[access]\n\towner = group Test Group",
			orderedIDs:   []string{"[access]"},
		},
		{
			name:         "multiple sections and lines",
			configMap:    map[string][]string{"[access]": {"\towner = group Test Group", "\towner = group Test Group 2"}, "[access \"refs/*\"]": {"\tread = group Test Group 3"}},
			configString: "[access]\n\towner = group Test Group\n\towner = group Test Group 2\n[access \"refs/*\"]\n\tread = group Test Group 3",
			orderedIDs:   []string{"[access]", "[access \"refs/*\"]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configMap, orderedKeys := configToMap(tc.configString)
			if diff := cmp.Diff(tc.configMap, configMap, cmpopts.SortSlices(func(x, y string) bool { return strings.Compare(x, y) > 0 })); diff != "" {
				t.Errorf("configToMap returned unexpected map value(-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.orderedIDs, orderedKeys, cmpopts.SortSlices(func(x, y string) bool { return strings.Compare(x, y) > 0 })); diff != "" {
				t.Errorf("configToMap returned unexpected ordredKeys value(-want +got):\n%s", diff)
			}
		})
	}
}

func TestMapToConfig(t *testing.T) {
	cases := []struct {
		name         string
		configMap    map[string][]string
		configString string
		orderedIDs   []string
	}{
		{
			name:         "empty config",
			configMap:    map[string][]string{},
			configString: "",
			orderedIDs:   []string{},
		},
		{
			name:         "one section",
			configMap:    map[string][]string{"[access]": {"\towner = group Test Group"}},
			configString: "[access]\n\towner = group Test Group\n",
			orderedIDs:   []string{"[access]"},
		},
		{
			name:         "multiple sections and lines",
			configMap:    map[string][]string{"[access]": {"\towner = group Test Group", "\towner = group Test Group 2"}, "[access \"refs/*\"]": {"\tread = group Test Group 3"}},
			configString: "[access]\n\towner = group Test Group\n\towner = group Test Group 2\n[access \"refs/*\"]\n\tread = group Test Group 3\n",
			orderedIDs:   []string{"[access]", "[access \"refs/*\"]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := mapToConfig(tc.configMap, tc.orderedIDs)
			if diff := cmp.Diff(tc.configString, config); diff != "" {
				t.Errorf("mapToConfig returned unexpected value(-want +got):\n%s", diff)
			}
		})
	}
}

func TestEnsureUUID(t *testing.T) {
	cases := []struct {
		name           string
		id             string
		group          string
		groupsString   string
		expectedString string
		err            bool
	}{
		{
			name:           "already exists",
			id:             "123456",
			group:          "Test Project 1",
			groupsString:   "# UUID\tGroup Name\n#\n123456\tTest Project 1\n567890\tTest Project 2\n",
			expectedString: "# UUID\tGroup Name\n#\n123456\tTest Project 1\n567890\tTest Project 2\n",
			err:            false,
		},
		{
			name:           "add new ID with new spacing",
			id:             "123456789",
			group:          "Test Project 3",
			groupsString:   "# UUID\tGroup Name\n#\n123456\tTest Project 1\n567890\tTest Project 2\n",
			expectedString: "# UUID   \tGroup Name\n#\n123456   \tTest Project 1\n567890   \tTest Project 2\n123456789\tTest Project 3\n",
			err:            false,
		},
		{
			name:           "conflicting ID",
			id:             "123456",
			group:          "Test Project 3",
			groupsString:   "# UUID\tGroup Name\n#\n123456\tTest Project 1\n567890\tTest Project 2\n",
			expectedString: "",
			err:            true,
		},
		{
			name:           "conflicting groupName",
			id:             "12345678",
			group:          "Test Project 1",
			groupsString:   "# UUID\tGroup Name\n#\n123456\tTest Project 1\n567890\tTest Project 2\n",
			expectedString: "",
			err:            true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ensureUUID(tc.groupsString, tc.id, tc.group)
			if err != nil && !tc.err {
				t.Errorf("expected no error but got %w", err)
			}
			if err == nil && tc.err {
				t.Error("expected error but got none")
			}
			if diff := cmp.Diff(tc.expectedString, res); diff != "" {
				t.Errorf("ensureUUID returned unexpected value (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetInheritedRepo(t *testing.T) {
	cases := []struct {
		name      string
		configMap map[string][]string
		expected  string
	}{
		{
			name:      "no inheritance",
			configMap: map[string][]string{"[access]": {"\towner = group Test Group"}},
			expected:  "",
		},
		{
			name:      "inherits",
			configMap: map[string][]string{"[access]": {"\towner = group Test Group", "/tinheritFrom = All-Projects"}},
			expected:  "All-Projects",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := getInheritedRepo(tc.configMap)
			if diff := cmp.Diff(tc.expected, res); diff != "" {
				t.Errorf("getInheritedRepo returned unexpected value(-want +got):\n%s", diff)
			}
		})
	}

}

func TestLineInMatchingHeaderFunc(t *testing.T) {
	sampleConfig := map[string][]string{
		"[access]":                     {"\towner = group Test Group 2", "\towner = group Test Group 3"},
		"[access \"refs/*\"]":          {"\tread = group Test Group 1"},
		"[access \"refs/for/master\"]": {"\tread = group Test Group 4"},
	}
	cases := []struct {
		name      string
		configMap map[string][]string
		line      string
		expected  bool
		regex     *regexp.Regexp
	}{
		{
			name:      "empty config",
			configMap: map[string][]string{},
			line:      "owner = group Test Group",
			expected:  false,
			regex:     accessRefsRegex,
		},
		{
			name:      "line in config",
			configMap: sampleConfig,
			line:      "read = group Test Group 1",
			expected:  true,
			regex:     accessRefsRegex,
		},
		{
			name:      "line not under header",
			configMap: sampleConfig,
			line:      "owner = group Test Group 2",
			expected:  false,
			regex:     accessRefsRegex,
		},
		{
			name:      "header more complicated",
			configMap: sampleConfig,
			line:      "read = group Test Group 4",
			expected:  true,
			regex:     accessRefsRegex,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resFunc := lineInMatchingHeaderFunc(tc.regex, tc.line)
			res := resFunc(tc.configMap)
			if diff := cmp.Diff(tc.expected, res); diff != "" {
				t.Errorf("lineInMatchingHeaderFunc returned unexpected value(-want +got):\n%s", diff)
			}
		})
	}
}

func TestLabelExists(t *testing.T) {
	cases := []struct {
		name      string
		configMap map[string][]string
		expected  bool
	}{
		{
			name:      "empty config",
			configMap: map[string][]string{},
			expected:  false,
		},
		{
			name:      "header not in config",
			configMap: map[string][]string{"[access]": {"\towner = group Test Group"}},
			expected:  false,
		},
		{
			name:      "header in config",
			configMap: map[string][]string{labelHeader: {"\towner = group Test Group"}},
			expected:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// labelExists always returns nil error
			res := labelExists(tc.configMap)
			if diff := cmp.Diff(tc.expected, res); diff != "" {
				t.Errorf("lineInConfig returned unexpected value(-want +got):\n%s", diff)
			}
		})
	}
}

func TestLabelAccessExistsFunc(t *testing.T) {
	cases := []struct {
		name      string
		configMap map[string][]string
		groupName string
		expected  bool
	}{
		{
			name:      "empty config",
			configMap: map[string][]string{},
			groupName: "Test Group",
			expected:  false,
		},
		{
			name:      "line in config",
			configMap: map[string][]string{"[access]": {"\tlabel-Verified = -1..+1 group Test Group"}},
			groupName: "Test Group",
			expected:  true,
		},
		{
			name:      "different values in config",
			configMap: map[string][]string{"[access]": {"\tlabel-Verified = -2..+2 group Test Group"}},
			groupName: "Test Group",
			expected:  true,
		},
		{
			name:      "Different group",
			configMap: map[string][]string{"[access]": {"\tlabel-Verified = -1..+1 group Not Test Group"}},
			groupName: "Test Group",
			expected:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resFunc := labelAccessExistsFunc(tc.groupName)
			res := resFunc(tc.configMap)
			if diff := cmp.Diff(tc.expected, res); diff != "" {
				t.Errorf("lineInConfig returned unexpected value(-want +got):\n%s", diff)
			}
		})
	}
}

func TestAddSection(t *testing.T) {
	cases := []struct {
		name        string
		configMap   map[string][]string
		orderedIDs  []string
		header      string
		adding      []string
		expectedMap map[string][]string
		expectedIDs []string
	}{
		{
			name:        "empty config",
			configMap:   map[string][]string{},
			orderedIDs:  []string{},
			header:      "[access]",
			adding:      []string{"test1", "test2"},
			expectedMap: map[string][]string{"[access]": {"\ttest1", "\ttest2"}},
			expectedIDs: []string{"[access]"},
		},
		{
			name:        "add to already existing section",
			configMap:   map[string][]string{"[access]": {"\towner = group Test Group"}},
			orderedIDs:  []string{"[access]"},
			header:      "[access]",
			adding:      []string{"test1", "test2"},
			expectedMap: map[string][]string{"[access]": {"\towner = group Test Group", "\ttest1", "\ttest2"}},
			expectedIDs: []string{"[access]"},
		},
		{
			name:        "add to already existing section",
			configMap:   map[string][]string{"[access]": {"\towner = group Test Group"}},
			orderedIDs:  []string{"[access]"},
			header:      "[test]",
			adding:      []string{"test1", "test2"},
			expectedMap: map[string][]string{"[access]": {"\towner = group Test Group"}, "[test]": {"\ttest1", "\ttest2"}},
			expectedIDs: []string{"[access]", "[test]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configMap, orderedKeys := addSection(tc.header, tc.configMap, tc.orderedIDs, tc.adding)
			if diff := cmp.Diff(tc.expectedMap, configMap, cmpopts.SortSlices(func(x, y string) bool { return strings.Compare(x, y) > 0 })); diff != "" {
				t.Errorf("configToMap returned unexpected map value(-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.expectedIDs, orderedKeys, cmpopts.SortSlices(func(x, y string) bool { return strings.Compare(x, y) > 0 })); diff != "" {
				t.Errorf("configToMap returned unexpected ordredKeys value(-want +got):\n%s", diff)
			}
		})
	}
}
