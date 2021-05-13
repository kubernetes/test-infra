package main

import (
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
			groupsString: "# UUID\tGroup Name\n#\n",
			orderedIDs:   []string{},
		},
		{
			name: "all uuids same size",
			groupsMap: map[string]string{
				"123456": "Test Project 1",
				"567890": "Test Project 2",
			},
			groupsString: "# UUID\tGroup Name\n#\n123456\tTest Project 1\n567890\tTest Project 2\n",
			orderedIDs:   []string{"123456", "567890"},
		},
		{
			name: "different sized uuid",
			groupsMap: map[string]string{
				"1234":      "Test Project 1",
				"123456789": "Test Project 2",
			},
			groupsString: "# UUID   \tGroup Name\n#\n1234     \tTest Project 1\n123456789\tTest Project 2\n",
			orderedIDs:   []string{"1234", "123456789"},
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
			groupsString: "# UUID\tGroup Name\n#\n",
			orderedIDs:   []string{},
		},
		{
			name: "all uuids same size",
			groupsMap: map[string]string{
				"123456": "Test Project 1",
				"567890": "Test Project 2",
			},
			groupsString: "# UUID\tGroup Name\n#\n123456\tTest Project 1\n567890\tTest Project 2\n",
			orderedIDs:   []string{"123456", "567890"},
		},
		{
			name: "different sized uuid",
			groupsMap: map[string]string{
				"1234":      "Test Project 1",
				"123456789": "Test Project 2",
			},
			groupsString: "# UUID   \tGroup Name\n#\n1234     \tTest Project 1\n123456789\tTest Project 2\n",
			orderedIDs:   []string{"1234", "123456789"},
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
	}{
		{
			name:           "already exists",
			id:             "123456",
			group:          "Test Project 1",
			groupsString:   "# UUID\tGroup Name\n#\n123456\tTest Project 1\n567890\tTest Project 2\n",
			expectedString: "# UUID\tGroup Name\n#\n123456\tTest Project 1\n567890\tTest Project 2\n",
		},
		{
			name:           "add new ID with new spacing",
			id:             "123456789",
			group:          "Test Project 3",
			groupsString:   "# UUID\tGroup Name\n#\n123456\tTest Project 1\n567890\tTest Project 2\n",
			expectedString: "# UUID   \tGroup Name\n#\n123456   \tTest Project 1\n567890   \tTest Project 2\n123456789\tTest Project 3\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.expectedString, ensureUUID(tc.groupsString, tc.id, tc.group)); diff != "" {
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

func TestLineInRightHeaderFunc(t *testing.T) {
	cases := []struct {
		name      string
		configMap map[string][]string
		header    string
		line      string
		expected  bool
	}{
		{
			name:      "empty config",
			configMap: map[string][]string{},
			header:    "[access]",
			line:      "owner = group Test Group",
			expected:  false,
		},
		{
			name:      "line in config",
			configMap: map[string][]string{"[access]": {"\towner = group Test Group"}},
			header:    "[access]",
			line:      "owner = group Test Group",
			expected:  true,
		},
		{
			name: "line not under header",
			configMap: map[string][]string{
				"[access]":            {"\towner = group Test Group", "\towner = group Test Group 2"},
				"[access \"refs/*\"]": {"\tread = group Test Group 3"},
			},
			header:   "[access]",
			line:     "Not here",
			expected: false,
		},
		{
			name: "header not present",
			configMap: map[string][]string{
				"[access]":            {"\towner = group Test Group", "\towner = group Test Group 2"},
				"[access \"refs/*\"]": {"\tread = group Test Group 3"},
			},
			header:   "[not here]",
			line:     "Not here",
			expected: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := lineInRightHeaderFunc(tc.header, tc.line)
			if diff := cmp.Diff(tc.expected, res(tc.configMap)); diff != "" {
				t.Errorf("lineInConfig returned unexpected value(-want +got):\n%s", diff)
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
			configMap: map[string][]string{LABEL_HEADER: {"\towner = group Test Group"}},
			expected:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
			res := labelAccessExistsFunc(tc.groupName)
			if diff := cmp.Diff(tc.expected, res(tc.configMap)); diff != "" {
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
