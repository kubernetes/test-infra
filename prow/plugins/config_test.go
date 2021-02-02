/*
Copyright 2018 The Kubernetes Authors.

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

package plugins

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/util/diff"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/bugzilla"
)

func TestValidateExternalPlugins(t *testing.T) {
	tests := []struct {
		name        string
		plugins     map[string][]ExternalPlugin
		expectedErr error
	}{
		{
			name: "valid config",
			plugins: map[string][]ExternalPlugin{
				"kubernetes/test-infra": {
					{
						Name: "cherrypick",
					},
					{
						Name: "configupdater",
					},
					{
						Name: "tetris",
					},
				},
				"kubernetes": {
					{
						Name: "coffeemachine",
					},
					{
						Name: "blender",
					},
				},
			},
			expectedErr: nil,
		},
		{
			name: "invalid config",
			plugins: map[string][]ExternalPlugin{
				"kubernetes/test-infra": {
					{
						Name: "cherrypick",
					},
					{
						Name: "configupdater",
					},
					{
						Name: "tetris",
					},
				},
				"kubernetes": {
					{
						Name: "coffeemachine",
					},
					{
						Name: "tetris",
					},
				},
			},
			expectedErr: errors.New("invalid plugin configuration:\n\texternal plugins [tetris] are duplicated for kubernetes/test-infra and kubernetes"),
		},
	}

	for _, test := range tests {
		t.Logf("Running scenario %q", test.name)

		err := validateExternalPlugins(test.plugins)
		if !reflect.DeepEqual(err, test.expectedErr) {
			t.Errorf("unexpected error: %v, expected: %v", err, test.expectedErr)
		}
	}
}

func TestSetDefault_Maps(t *testing.T) {
	cases := []struct {
		name     string
		config   ConfigUpdater
		expected map[string]ConfigMapSpec
	}{
		{
			name: "nothing",
			expected: map[string]ConfigMapSpec{
				"config/prow/config.yaml":  {Name: "config", Clusters: map[string][]string{"default": {""}}},
				"config/prow/plugins.yaml": {Name: "plugins", Clusters: map[string][]string{"default": {""}}},
			},
		},
		{
			name: "basic",
			config: ConfigUpdater{
				Maps: map[string]ConfigMapSpec{
					"hello.yaml": {Name: "my-cm"},
					"world.yaml": {Name: "you-cm"},
				},
			},
			expected: map[string]ConfigMapSpec{
				"hello.yaml": {Name: "my-cm", Clusters: map[string][]string{"default": {""}}},
				"world.yaml": {Name: "you-cm", Clusters: map[string][]string{"default": {""}}},
			},
		},
		{
			name: "both current and deprecated",
			config: ConfigUpdater{
				Maps: map[string]ConfigMapSpec{
					"config.yaml":        {Name: "overwrite-config"},
					"plugins.yaml":       {Name: "overwrite-plugins"},
					"unconflicting.yaml": {Name: "ignored"},
				},
			},
			expected: map[string]ConfigMapSpec{
				"config.yaml":        {Name: "overwrite-config", Clusters: map[string][]string{"default": {""}}},
				"plugins.yaml":       {Name: "overwrite-plugins", Clusters: map[string][]string{"default": {""}}},
				"unconflicting.yaml": {Name: "ignored", Clusters: map[string][]string{"default": {""}}},
			},
		},
	}
	for _, tc := range cases {
		cfg := Configuration{
			ConfigUpdater: tc.config,
		}
		cfg.setDefaults()
		actual := cfg.ConfigUpdater.Maps
		if len(actual) != len(tc.expected) {
			t.Errorf("%s: actual and expected have different keys: %v %v", tc.name, actual, tc.expected)
			continue
		}
		for k, n := range tc.expected {
			if an := actual[k]; !reflect.DeepEqual(an, n) {
				t.Errorf("%s - %s: unexpected value. Diff: %v", tc.name, k, diff.ObjectReflectDiff(an, n))
			}
		}
	}
}

func TestTriggerFor(t *testing.T) {
	config := Configuration{
		Triggers: []Trigger{
			{
				Repos:      []string{"kuber"},
				TrustedOrg: "org1",
			},
			{
				Repos:      []string{"k8s/k8s", "k8s/kuber"},
				TrustedOrg: "org2",
			},
			{
				Repos:      []string{"k8s/t-i"},
				TrustedOrg: "org3",
			},
		},
	}
	config.setDefaults()

	testCases := []struct {
		name            string
		org, repo       string
		expectedTrusted string
		check           func(Trigger) error
	}{
		{
			name:            "org trigger",
			org:             "kuber",
			repo:            "kuber",
			expectedTrusted: "org1",
		},
		{
			name:            "repo trigger",
			org:             "k8s",
			repo:            "t-i",
			expectedTrusted: "org3",
		},
		{
			name: "default trigger",
			org:  "other",
			repo: "other",
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			actual := config.TriggerFor(tc.org, tc.repo)
			if tc.expectedTrusted != actual.TrustedOrg {
				t.Errorf("expected TrustedOrg to be %q, but got %q", tc.expectedTrusted, actual.TrustedOrg)
			}
		})
	}
}

func TestSetApproveDefaults(t *testing.T) {
	c := &Configuration{
		Approve: []Approve{
			{
				Repos: []string{
					"kubernetes/kubernetes",
					"kubernetes-client",
				},
			},
			{
				Repos: []string{
					"kubernetes-sigs/cluster-api",
				},
				CommandHelpLink: "https://prow.k8s.io/command-help",
				PrProcessLink:   "https://github.com/kubernetes/community/blob/427ccfbc7d423d8763ed756f3b8c888b7de3cf34/contributors/guide/pull-requests.md",
			},
		},
	}

	tests := []struct {
		name                    string
		org                     string
		repo                    string
		expectedCommandHelpLink string
		expectedPrProcessLink   string
	}{
		{
			name:                    "default",
			org:                     "kubernetes",
			repo:                    "kubernetes",
			expectedCommandHelpLink: "https://go.k8s.io/bot-commands",
			expectedPrProcessLink:   "https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process",
		},
		{
			name:                    "overwrite",
			org:                     "kubernetes-sigs",
			repo:                    "cluster-api",
			expectedCommandHelpLink: "https://prow.k8s.io/command-help",
			expectedPrProcessLink:   "https://github.com/kubernetes/community/blob/427ccfbc7d423d8763ed756f3b8c888b7de3cf34/contributors/guide/pull-requests.md",
		},
		{
			name:                    "default for repo without approve plugin config",
			org:                     "kubernetes",
			repo:                    "website",
			expectedCommandHelpLink: "https://go.k8s.io/bot-commands",
			expectedPrProcessLink:   "https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process",
		},
	}

	for _, test := range tests {

		a := c.ApproveFor(test.org, test.repo)

		if a.CommandHelpLink != test.expectedCommandHelpLink {
			t.Errorf("unexpected commandHelpLink: %s, expected: %s", a.CommandHelpLink, test.expectedCommandHelpLink)
		}

		if a.PrProcessLink != test.expectedPrProcessLink {
			t.Errorf("unexpected prProcessLink: %s, expected: %s", a.PrProcessLink, test.expectedPrProcessLink)
		}
	}
}

func TestSetHelpDefaults(t *testing.T) {
	tests := []struct {
		name              string
		helpGuidelinesURL string

		expectedHelpGuidelinesURL string
	}{
		{
			name:                      "default",
			helpGuidelinesURL:         "",
			expectedHelpGuidelinesURL: "https://git.k8s.io/community/contributors/guide/help-wanted.md",
		},
		{
			name:                      "overwrite",
			helpGuidelinesURL:         "https://github.com/kubernetes/community/blob/master/contributors/guide/help-wanted.md",
			expectedHelpGuidelinesURL: "https://github.com/kubernetes/community/blob/master/contributors/guide/help-wanted.md",
		},
	}

	for _, test := range tests {
		c := &Configuration{
			Help: Help{
				HelpGuidelinesURL: test.helpGuidelinesURL,
			},
		}

		c.setDefaults()

		if c.Help.HelpGuidelinesURL != test.expectedHelpGuidelinesURL {
			t.Errorf("unexpected help_guidelines_url: %s, expected: %s", c.Help.HelpGuidelinesURL, test.expectedHelpGuidelinesURL)
		}
	}
}

func TestSetTriggerDefaults(t *testing.T) {
	tests := []struct {
		name string

		trustedOrg string
		joinOrgURL string

		expectedTrustedOrg string
		expectedJoinOrgURL string
	}{
		{
			name: "url defaults to org",

			trustedOrg: "kubernetes",
			joinOrgURL: "",

			expectedTrustedOrg: "kubernetes",
			expectedJoinOrgURL: "https://github.com/orgs/kubernetes/people",
		},
		{
			name: "both org and url are set",

			trustedOrg: "kubernetes",
			joinOrgURL: "https://git.k8s.io/community/community-membership.md#member",

			expectedTrustedOrg: "kubernetes",
			expectedJoinOrgURL: "https://git.k8s.io/community/community-membership.md#member",
		},
		{
			name: "only url is set",

			trustedOrg: "",
			joinOrgURL: "https://git.k8s.io/community/community-membership.md#member",

			expectedTrustedOrg: "",
			expectedJoinOrgURL: "https://git.k8s.io/community/community-membership.md#member",
		},
		{
			name: "nothing is set",

			trustedOrg: "",
			joinOrgURL: "",

			expectedTrustedOrg: "",
			expectedJoinOrgURL: "",
		},
	}

	for _, test := range tests {
		c := &Configuration{
			Triggers: []Trigger{
				{
					TrustedOrg: test.trustedOrg,
					JoinOrgURL: test.joinOrgURL,
				},
			},
		}

		c.setDefaults()

		if c.Triggers[0].TrustedOrg != test.expectedTrustedOrg {
			t.Errorf("unexpected trusted_org: %s, expected: %s", c.Triggers[0].TrustedOrg, test.expectedTrustedOrg)
		}
		if c.Triggers[0].JoinOrgURL != test.expectedJoinOrgURL {
			t.Errorf("unexpected join_org_url: %s, expected: %s", c.Triggers[0].JoinOrgURL, test.expectedJoinOrgURL)
		}
	}
}

func TestSetCherryPickUnapprovedDefaults(t *testing.T) {
	defaultBranchRegexp := `^release-.*$`
	defaultComment := `This PR is not for the master branch but does not have the ` + "`cherry-pick-approved`" + `  label. Adding the ` + "`do-not-merge/cherry-pick-not-approved`" + `  label.`

	testcases := []struct {
		name string

		branchRegexp string
		comment      string

		expectedBranchRegexp string
		expectedComment      string
	}{
		{
			name:                 "none of branchRegexp and comment are set",
			branchRegexp:         "",
			comment:              "",
			expectedBranchRegexp: defaultBranchRegexp,
			expectedComment:      defaultComment,
		},
		{
			name:                 "only branchRegexp is set",
			branchRegexp:         `release-1.1.*$`,
			comment:              "",
			expectedBranchRegexp: `release-1.1.*$`,
			expectedComment:      defaultComment,
		},
		{
			name:                 "only comment is set",
			branchRegexp:         "",
			comment:              "custom comment",
			expectedBranchRegexp: defaultBranchRegexp,
			expectedComment:      "custom comment",
		},
		{
			name:                 "both branchRegexp and comment are set",
			branchRegexp:         `release-1.1.*$`,
			comment:              "custom comment",
			expectedBranchRegexp: `release-1.1.*$`,
			expectedComment:      "custom comment",
		},
	}

	for _, tc := range testcases {
		c := &Configuration{
			CherryPickUnapproved: CherryPickUnapproved{
				BranchRegexp: tc.branchRegexp,
				Comment:      tc.comment,
			},
		}

		c.setDefaults()

		if c.CherryPickUnapproved.BranchRegexp != tc.expectedBranchRegexp {
			t.Errorf("unexpected branchRegexp: %s, expected: %s", c.CherryPickUnapproved.BranchRegexp, tc.expectedBranchRegexp)
		}
		if c.CherryPickUnapproved.Comment != tc.expectedComment {
			t.Errorf("unexpected comment: %s, expected: %s", c.CherryPickUnapproved.Comment, tc.expectedComment)
		}
	}
}

func TestOptionsForItem(t *testing.T) {
	open := true
	one, two := "v1", "v2"
	var testCases = []struct {
		name     string
		item     string
		config   map[string]BugzillaBranchOptions
		expected BugzillaBranchOptions
	}{
		{
			name:     "no config means no options",
			item:     "item",
			config:   map[string]BugzillaBranchOptions{},
			expected: BugzillaBranchOptions{},
		},
		{
			name:     "unrelated config means no options",
			item:     "item",
			config:   map[string]BugzillaBranchOptions{"other": {IsOpen: &open, TargetRelease: &one}},
			expected: BugzillaBranchOptions{},
		},
		{
			name:     "global config resolves to options",
			item:     "item",
			config:   map[string]BugzillaBranchOptions{"*": {IsOpen: &open, TargetRelease: &one}},
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one},
		},
		{
			name:     "specific config resolves to options",
			item:     "item",
			config:   map[string]BugzillaBranchOptions{"item": {IsOpen: &open, TargetRelease: &one}},
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one},
		},
		{
			name: "global and specific config resolves to options that favor specificity",
			item: "item",
			config: map[string]BugzillaBranchOptions{
				"*":    {IsOpen: &open, TargetRelease: &one},
				"item": {TargetRelease: &two},
			},
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &two},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := OptionsForItem(testCase.item, testCase.config), testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect options for item %q: %v", testCase.name, testCase.item, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestResolveBugzillaOptions(t *testing.T) {
	open, closed := true, false
	yes, no := true, false
	one, two := "v1", "v2"
	modified, verified, post, pre := "MODIFIED", "VERIFIED", "POST", "PRE"
	modifiedState := BugzillaBugState{Status: modified}
	verifiedState := BugzillaBugState{Status: verified}
	postState := BugzillaBugState{Status: post}
	preState := BugzillaBugState{Status: pre}
	var testCases = []struct {
		name          string
		parent, child BugzillaBranchOptions
		expected      BugzillaBranchOptions
	}{
		{
			name: "no parent or child means no output",
		},
		{
			name:   "no child means a copy of parent is the output",
			parent: BugzillaBranchOptions{ValidateByDefault: &yes, IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, DependentBugStates: &[]BugzillaBugState{verifiedState}, DependentBugTargetReleases: &[]string{one}, StateAfterValidation: &postState},
			expected: BugzillaBranchOptions{
				ValidateByDefault:          &yes,
				IsOpen:                     &open,
				TargetRelease:              &one,
				ValidStates:                &[]BugzillaBugState{modifiedState},
				DependentBugStates:         &[]BugzillaBugState{verifiedState},
				DependentBugTargetReleases: &[]string{one},
				StateAfterValidation:       &postState,
			},
		},
		{
			name:  "no parent means a copy of child is the output",
			child: BugzillaBranchOptions{ValidateByDefault: &yes, IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, DependentBugStates: &[]BugzillaBugState{verifiedState}, DependentBugTargetReleases: &[]string{one}, StateAfterValidation: &postState},
			expected: BugzillaBranchOptions{
				ValidateByDefault:          &yes,
				IsOpen:                     &open,
				TargetRelease:              &one,
				ValidStates:                &[]BugzillaBugState{modifiedState},
				DependentBugStates:         &[]BugzillaBugState{verifiedState},
				DependentBugTargetReleases: &[]string{one},
				StateAfterValidation:       &postState,
			},
		},
		{
			name:     "child overrides parent on IsOpen",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, StateAfterValidation: &postState},
			child:    BugzillaBranchOptions{IsOpen: &closed},
			expected: BugzillaBranchOptions{IsOpen: &closed, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, StateAfterValidation: &postState},
		},
		{
			name:     "child overrides parent on target release",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, StateAfterValidation: &postState},
			child:    BugzillaBranchOptions{TargetRelease: &two},
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &two, ValidStates: &[]BugzillaBugState{modifiedState}, StateAfterValidation: &postState},
		},
		{
			name:     "child overrides parent on states",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, StateAfterValidation: &postState},
			child:    BugzillaBranchOptions{ValidStates: &[]BugzillaBugState{verifiedState}},
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{verifiedState}, StateAfterValidation: &postState},
		},
		{
			name:     "child overrides parent on state after validation",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, StateAfterValidation: &postState},
			child:    BugzillaBranchOptions{StateAfterValidation: &preState},
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, StateAfterValidation: &preState},
		},
		{
			name:     "child overrides parent on validation by default",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, StateAfterValidation: &postState},
			child:    BugzillaBranchOptions{ValidateByDefault: &yes},
			expected: BugzillaBranchOptions{ValidateByDefault: &yes, IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, StateAfterValidation: &postState},
		},
		{
			name:   "child overrides parent on dependent bug states",
			parent: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, DependentBugStates: &[]BugzillaBugState{verifiedState}, StateAfterValidation: &postState},
			child:  BugzillaBranchOptions{DependentBugStates: &[]BugzillaBugState{modifiedState}},
			expected: BugzillaBranchOptions{
				IsOpen:               &open,
				TargetRelease:        &one,
				ValidStates:          &[]BugzillaBugState{modifiedState},
				DependentBugStates:   &[]BugzillaBugState{modifiedState},
				StateAfterValidation: &postState,
			},
		},
		{
			name:     "child overrides parent on dependent bug target releases",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, StateAfterValidation: &postState, DependentBugTargetReleases: &[]string{one}},
			child:    BugzillaBranchOptions{DependentBugTargetReleases: &[]string{two}},
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, StateAfterValidation: &postState, DependentBugTargetReleases: &[]string{two}},
		},
		{
			name:   "child overrides parent on state after merge",
			parent: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{modifiedState}, StateAfterValidation: &postState, StateAfterMerge: &postState},
			child:  BugzillaBranchOptions{StateAfterMerge: &preState},
			expected: BugzillaBranchOptions{
				IsOpen:               &open,
				TargetRelease:        &one,
				ValidStates:          &[]BugzillaBugState{modifiedState},
				StateAfterValidation: &postState,
				StateAfterMerge:      &preState,
			},
		},
		{
			name:     "status slices are correctly merged with states slices on parent",
			parent:   BugzillaBranchOptions{Statuses: &[]string{modified}, ValidStates: &[]BugzillaBugState{verifiedState}, DependentBugStatuses: &[]string{pre}, DependentBugStates: &[]BugzillaBugState{postState}},
			expected: BugzillaBranchOptions{ValidStates: &[]BugzillaBugState{modifiedState, verifiedState}, DependentBugStates: &[]BugzillaBugState{postState, preState}},
		},
		{
			name:     "status slices are correctly merged with states slices on child",
			child:    BugzillaBranchOptions{Statuses: &[]string{modified}, ValidStates: &[]BugzillaBugState{verifiedState}, DependentBugStatuses: &[]string{pre}, DependentBugStates: &[]BugzillaBugState{postState}},
			expected: BugzillaBranchOptions{ValidStates: &[]BugzillaBugState{modifiedState, verifiedState}, DependentBugStates: &[]BugzillaBugState{postState, preState}},
		},
		{
			name:     "state fields when not present re inferred from status fields on parent",
			parent:   BugzillaBranchOptions{StatusAfterMerge: &modified, StatusAfterValidation: &verified},
			expected: BugzillaBranchOptions{StateAfterMerge: &modifiedState, StateAfterValidation: &verifiedState},
		},
		{
			name:     "state fields when not present are inferred from status fields on child",
			child:    BugzillaBranchOptions{StatusAfterMerge: &modified, StatusAfterValidation: &verified},
			expected: BugzillaBranchOptions{StateAfterMerge: &modifiedState, StateAfterValidation: &verifiedState},
		},
		{
			name:     "child status overrides all statuses and states of the parent",
			parent:   BugzillaBranchOptions{Statuses: &[]string{modified}, ValidStates: &[]BugzillaBugState{verifiedState}, DependentBugStatuses: &[]string{modified}, DependentBugStates: &[]BugzillaBugState{verifiedState}, StatusAfterMerge: &pre, StateAfterMerge: &preState, StatusAfterValidation: &pre, StateAfterValidation: &preState},
			child:    BugzillaBranchOptions{Statuses: &[]string{post}, DependentBugStatuses: &[]string{post}, StatusAfterMerge: &post, StatusAfterValidation: &post},
			expected: BugzillaBranchOptions{ValidStates: &[]BugzillaBugState{postState}, DependentBugStates: &[]BugzillaBugState{postState}, StateAfterMerge: &postState, StateAfterValidation: &postState},
		},
		{
			name:     "parent dependent target release is merged on child",
			parent:   BugzillaBranchOptions{DeprecatedDependentBugTargetRelease: &one},
			child:    BugzillaBranchOptions{},
			expected: BugzillaBranchOptions{DependentBugTargetReleases: &[]string{one}},
		},
		{
			name:     "parent dependent target release is merged into target releases",
			parent:   BugzillaBranchOptions{DependentBugTargetReleases: &[]string{one}, DeprecatedDependentBugTargetRelease: &two},
			child:    BugzillaBranchOptions{},
			expected: BugzillaBranchOptions{DependentBugTargetReleases: &[]string{one, two}},
		},
		{
			name:   "child overrides parent on all fields",
			parent: BugzillaBranchOptions{ValidateByDefault: &yes, IsOpen: &open, TargetRelease: &one, ValidStates: &[]BugzillaBugState{verifiedState}, DependentBugStates: &[]BugzillaBugState{verifiedState}, DependentBugTargetReleases: &[]string{one}, StateAfterValidation: &postState, StateAfterMerge: &postState},
			child:  BugzillaBranchOptions{ValidateByDefault: &no, IsOpen: &closed, TargetRelease: &two, ValidStates: &[]BugzillaBugState{modifiedState}, DependentBugStates: &[]BugzillaBugState{modifiedState}, DependentBugTargetReleases: &[]string{two}, StateAfterValidation: &preState, StateAfterMerge: &preState},
			expected: BugzillaBranchOptions{
				ValidateByDefault:          &no,
				IsOpen:                     &closed,
				TargetRelease:              &two,
				ValidStates:                &[]BugzillaBugState{modifiedState},
				DependentBugStates:         &[]BugzillaBugState{modifiedState},
				DependentBugTargetReleases: &[]string{two},
				StateAfterValidation:       &preState,
				StateAfterMerge:            &preState,
			},
		},
		{
			name:     "parent target release is excluded on child",
			parent:   BugzillaBranchOptions{TargetRelease: &one},
			child:    BugzillaBranchOptions{ExcludeDefaults: &yes},
			expected: BugzillaBranchOptions{ExcludeDefaults: &yes},
		},
		{
			name:     "parent target release is excluded on child with other options",
			parent:   BugzillaBranchOptions{DependentBugTargetReleases: &[]string{one}},
			child:    BugzillaBranchOptions{TargetRelease: &one, ExcludeDefaults: &yes},
			expected: BugzillaBranchOptions{TargetRelease: &one, ExcludeDefaults: &yes},
		},
		{
			name:     "parent exclude merges with child options",
			parent:   BugzillaBranchOptions{DependentBugTargetReleases: &[]string{one}, ExcludeDefaults: &yes},
			child:    BugzillaBranchOptions{TargetRelease: &one},
			expected: BugzillaBranchOptions{DependentBugTargetReleases: &[]string{one}, TargetRelease: &one, ExcludeDefaults: &yes},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := ResolveBugzillaOptions(testCase.parent, testCase.child), testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: resolved incorrect options for parent and child: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}

	var i int = 0
	managedCol1 := ManagedColumn{ID: &i, Name: "col1", State: "open", Labels: []string{"area/conformance", "area/testing"}, Org: "org1"}
	managedCol3 := ManagedColumn{ID: &i, Name: "col2", State: "open", Labels: []string{}, Org: "org2"}
	managedColx := ManagedColumn{ID: &i, Name: "col2", State: "open", Labels: []string{"area/conformance", "area/testing"}, Org: "org2"}
	invalidCol := ManagedColumn{State: "open", Labels: []string{"area/conformance", "area/testing2"}, Org: "org2"}
	invalidOrg := ManagedColumn{Name: "col1", State: "open", Labels: []string{"area/conformance", "area/testing2"}, Org: ""}
	managedProj2 := ManagedProject{Columns: []ManagedColumn{managedCol3}}
	managedProjx := ManagedProject{Columns: []ManagedColumn{managedCol1, managedColx}}
	managedOrgRepo2 := ManagedOrgRepo{Projects: map[string]ManagedProject{"project1": managedProj2}}
	managedOrgRepox := ManagedOrgRepo{Projects: map[string]ManagedProject{"project1": managedProjx}}

	projectManagerTestcases := []struct {
		name        string
		config      *Configuration
		expectedErr string
	}{
		{
			name: "No projects configured in a repo",
			config: &Configuration{
				ProjectManager: ProjectManager{
					OrgRepos: map[string]ManagedOrgRepo{"org1": {Projects: map[string]ManagedProject{}}},
				},
			},
			expectedErr: fmt.Sprintf("Org/repo: %s, has no projects configured", "org1"),
		},
		{
			name: "No columns configured for a project",
			config: &Configuration{
				ProjectManager: ProjectManager{
					OrgRepos: map[string]ManagedOrgRepo{"org1": {Projects: map[string]ManagedProject{"project1": {Columns: []ManagedColumn{}}}}},
				},
			},
			expectedErr: fmt.Sprintf("Org/repo: %s, project %s, has no columns configured", "org1", "project1"),
		},
		{
			name: "Columns does not have name or ID",
			config: &Configuration{
				ProjectManager: ProjectManager{
					OrgRepos: map[string]ManagedOrgRepo{"org1": {Projects: map[string]ManagedProject{"project1": {Columns: []ManagedColumn{invalidCol}}}}},
				},
			},
			expectedErr: fmt.Sprintf("Org/repo: %s, project %s, column %v, has no name/id configured", "org1", "project1", invalidCol),
		},
		{
			name: "Columns does not have owner Org/repo",
			config: &Configuration{
				ProjectManager: ProjectManager{
					OrgRepos: map[string]ManagedOrgRepo{"org1": {Projects: map[string]ManagedProject{"project1": {Columns: []ManagedColumn{invalidOrg}}}}},
				},
			},
			expectedErr: fmt.Sprintf("Org/repo: %s, project %s, column %s, has no org configured", "org1", "project1", "col1"),
		},
		{
			name: "No Labels specified in the column of the project",
			config: &Configuration{
				ProjectManager: ProjectManager{
					OrgRepos: map[string]ManagedOrgRepo{"org1": managedOrgRepo2},
				},
			},
			expectedErr: fmt.Sprintf("Org/repo: %s, project %s, column %s, has no labels configured", "org1", "project1", "col2"),
		},
		{
			name: "Same Label specified to multiple column in a project",
			config: &Configuration{
				ProjectManager: ProjectManager{
					OrgRepos: map[string]ManagedOrgRepo{"org1": managedOrgRepox},
				},
			},
			expectedErr: fmt.Sprintf("Org/repo: %s, project %s, column %s has same labels configured as another column", "org1", "project1", "col2"),
		},
	}

	for _, c := range projectManagerTestcases {
		t.Run(c.name, func(t *testing.T) {
			err := validateProjectManager(c.config.ProjectManager)
			if err != nil && len(c.expectedErr) == 0 {
				t.Fatalf("config validation error: %v", err)
			}
			if err == nil && len(c.expectedErr) > 0 {
				t.Fatalf("config validation error: %v but expecting %v", err, c.expectedErr)
			}
			if err != nil && c.expectedErr != err.Error() {
				t.Fatalf("Error running the test %s, \nexpected: %s, \nreceived: %s", c.name, c.expectedErr, err.Error())
			}
		})
	}
}

func TestOptionsForBranch(t *testing.T) {
	open, closed := true, false
	yes, no := true, false
	globalDefault, globalBranchDefault, orgDefault, orgBranchDefault, repoDefault, repoBranch, legacyBranch := "global-default", "global-branch-default", "my-org-default", "my-org-branch-default", "my-repo-default", "my-repo-branch", "my-legacy-branch"
	post, pre, release, notabug, new, reset := "POST", "PRE", "RELEASE_PENDING", "NOTABUG", "NEW", "RESET"
	verifiedState, modifiedState := BugzillaBugState{Status: "VERIFIED"}, BugzillaBugState{Status: "MODIFIED"}
	postState, preState, releaseState, notabugState, newState, resetState := BugzillaBugState{Status: post}, BugzillaBugState{Status: pre}, BugzillaBugState{Status: release}, BugzillaBugState{Status: notabug}, BugzillaBugState{Status: new}, BugzillaBugState{Status: reset}
	closedErrata := BugzillaBugState{Status: "CLOSED", Resolution: "ERRATA"}
	orgAllowedGroups, repoAllowedGroups := []string{"test"}, []string{"security", "test"}

	rawConfig := `default:
  "*":
    target_release: global-default
  "global-branch":
    is_open: false
    target_release: global-branch-default
orgs:
  my-org:
    default:
      "*":
        is_open: true
        target_release: my-org-default
        state_after_validation:
          status: "PRE"
        state_after_close:
          status: "NEW"
        allowed_groups:
        - test
      "my-org-branch":
        target_release: my-org-branch-default
        state_after_validation:
          status: "POST"
    repos:
      my-repo:
        branches:
          "*":
            is_open: false
            target_release: my-repo-default
            valid_states:
            - status: VERIFIED
            validate_by_default: false
            state_after_merge:
              status: RELEASE_PENDING
          "my-repo-branch":
            target_release: my-repo-branch
            valid_states:
            - status: MODIFIED
            - status: CLOSED
              resolution: ERRATA
            validate_by_default: true
            state_after_merge:
              status: NOTABUG
            state_after_close:
              status: RESET
            allowed_groups:
            - security
          "my-legacy-branch":
            target_release: my-legacy-branch
            statuses:
            - MODIFIED
            dependent_bug_statuses:
            - VERIFIED
            validate_by_default: true
            status_after_validation: MODIFIED
            status_after_merge: NOTABUG
          "my-special-branch":
            exclude_defaults: true
            validate_by_default: false
      another-repo:
        branches:
          "*":
            exclude_defaults: true
          "my-org-branch":
            target_release: my-repo-branch`
	var config Bugzilla
	if err := yaml.Unmarshal([]byte(rawConfig), &config); err != nil {
		t.Fatalf("couldn't unmarshal config: %v", err)
	}

	var testCases = []struct {
		name              string
		org, repo, branch string
		expected          BugzillaBranchOptions
	}{
		{
			name:     "unconfigured branch gets global default",
			org:      "some-org",
			repo:     "some-repo",
			branch:   "some-branch",
			expected: BugzillaBranchOptions{TargetRelease: &globalDefault},
		},
		{
			name:     "branch on unconfigured org/repo gets global default",
			org:      "some-org",
			repo:     "some-repo",
			branch:   "global-branch",
			expected: BugzillaBranchOptions{IsOpen: &closed, TargetRelease: &globalBranchDefault},
		},
		{
			name:     "branch on configured org but not repo gets org default",
			org:      "my-org",
			repo:     "some-repo",
			branch:   "some-branch",
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &orgDefault, StateAfterValidation: &preState, AllowedGroups: orgAllowedGroups, StateAfterClose: &newState},
		},
		{
			name:     "branch on configured org but not repo gets org branch default",
			org:      "my-org",
			repo:     "some-repo",
			branch:   "my-org-branch",
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &orgBranchDefault, StateAfterValidation: &postState, AllowedGroups: orgAllowedGroups, StateAfterClose: &newState},
		},
		{
			name:     "branch on configured org and repo gets repo default",
			org:      "my-org",
			repo:     "my-repo",
			branch:   "some-branch",
			expected: BugzillaBranchOptions{ValidateByDefault: &no, IsOpen: &closed, TargetRelease: &repoDefault, ValidStates: &[]BugzillaBugState{verifiedState}, StateAfterValidation: &preState, StateAfterMerge: &releaseState, AllowedGroups: orgAllowedGroups, StateAfterClose: &newState},
		},
		{
			name:     "branch on configured org and repo gets branch config",
			org:      "my-org",
			repo:     "my-repo",
			branch:   "my-repo-branch",
			expected: BugzillaBranchOptions{ValidateByDefault: &yes, IsOpen: &closed, TargetRelease: &repoBranch, ValidStates: &[]BugzillaBugState{modifiedState, closedErrata}, StateAfterValidation: &preState, StateAfterMerge: &notabugState, AllowedGroups: repoAllowedGroups, StateAfterClose: &resetState},
		},
		{
			name:     "exclude branch on configured org and repo gets branch config",
			org:      "my-org",
			repo:     "my-repo",
			branch:   "my-special-branch",
			expected: BugzillaBranchOptions{ValidateByDefault: &no, ExcludeDefaults: &yes},
		},
		{
			name:     "exclude branch on repo cascades to branch config",
			org:      "my-org",
			repo:     "another-repo",
			branch:   "my-org-branch",
			expected: BugzillaBranchOptions{TargetRelease: &repoBranch, ExcludeDefaults: &yes},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := config.OptionsForBranch(testCase.org, testCase.repo, testCase.branch), testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: resolved incorrect options for %s/%s#%s: %v", testCase.name, testCase.org, testCase.repo, testCase.branch, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}

	var repoTestCases = []struct {
		name      string
		org, repo string
		expected  map[string]BugzillaBranchOptions
	}{
		{
			name: "unconfigured repo gets global default",
			org:  "some-org",
			repo: "some-repo",
			expected: map[string]BugzillaBranchOptions{
				"*":             {TargetRelease: &globalDefault},
				"global-branch": {IsOpen: &closed, TargetRelease: &globalBranchDefault},
			},
		},
		{
			name: "repo in configured org gets org default",
			org:  "my-org",
			repo: "some-repo",
			expected: map[string]BugzillaBranchOptions{
				"*":             {IsOpen: &open, TargetRelease: &orgDefault, StateAfterValidation: &preState, AllowedGroups: orgAllowedGroups, StateAfterClose: &newState},
				"my-org-branch": {IsOpen: &open, TargetRelease: &orgBranchDefault, StateAfterValidation: &postState, AllowedGroups: orgAllowedGroups, StateAfterClose: &newState},
			},
		},
		{
			name: "configured repo gets repo config",
			org:  "my-org",
			repo: "my-repo",
			expected: map[string]BugzillaBranchOptions{
				"*": {
					ValidateByDefault:    &no,
					IsOpen:               &closed,
					TargetRelease:        &repoDefault,
					ValidStates:          &[]BugzillaBugState{verifiedState},
					StateAfterValidation: &preState,
					StateAfterMerge:      &releaseState,
					AllowedGroups:        orgAllowedGroups,
					StateAfterClose:      &newState,
				},
				"my-repo-branch": {
					ValidateByDefault:    &yes,
					IsOpen:               &closed,
					TargetRelease:        &repoBranch,
					ValidStates:          &[]BugzillaBugState{modifiedState, closedErrata},
					StateAfterValidation: &preState,
					StateAfterMerge:      &notabugState,
					AllowedGroups:        repoAllowedGroups,
					StateAfterClose:      &resetState,
				},
				"my-org-branch": {
					ValidateByDefault:    &no,
					IsOpen:               &closed,
					TargetRelease:        &repoDefault,
					ValidStates:          &[]BugzillaBugState{verifiedState},
					StateAfterValidation: &postState,
					StateAfterMerge:      &releaseState,
					AllowedGroups:        orgAllowedGroups,
					StateAfterClose:      &newState,
				},
				"my-legacy-branch": {
					ValidateByDefault:    &yes,
					IsOpen:               &closed,
					TargetRelease:        &legacyBranch,
					ValidStates:          &[]BugzillaBugState{modifiedState},
					DependentBugStates:   &[]BugzillaBugState{verifiedState},
					StateAfterValidation: &modifiedState,
					StateAfterMerge:      &notabugState,
					AllowedGroups:        orgAllowedGroups,
					StateAfterClose:      &newState,
				},
				"my-special-branch": {
					ValidateByDefault: &no,
					ExcludeDefaults:   &yes,
				},
			},
		},
		{
			name: "excluded repo gets no defaults",
			org:  "my-org",
			repo: "another-repo",
			expected: map[string]BugzillaBranchOptions{
				"*":             {ExcludeDefaults: &yes},
				"my-org-branch": {ExcludeDefaults: &yes, TargetRelease: &repoBranch},
			},
		},
	}
	for _, testCase := range repoTestCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := config.OptionsForRepo(testCase.org, testCase.repo), testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: resolved incorrect options for %s/%s: %v", testCase.name, testCase.org, testCase.repo, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestBugzillaBugState_String(t *testing.T) {
	testCases := []struct {
		name     string
		state    *BugzillaBugState
		expected string
	}{
		{
			name:     "empty struct",
			state:    &BugzillaBugState{},
			expected: "",
		},
		{
			name:     "only status",
			state:    &BugzillaBugState{Status: "CLOSED"},
			expected: "CLOSED",
		},
		{
			name:     "only resolution",
			state:    &BugzillaBugState{Resolution: "NOTABUG"},
			expected: "any status with resolution NOTABUG",
		},
		{
			name:     "status and resolution",
			state:    &BugzillaBugState{Status: "CLOSED", Resolution: "NOTABUG"},
			expected: "CLOSED (NOTABUG)",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.state.String()
			if actual != tc.expected {
				t.Errorf("%s: expected %q, got %q", tc.name, tc.expected, actual)
			}
		})
	}
}

func TestBugzillaBugState_Matches(t *testing.T) {
	modified, closed, errata, notabug := "MODIFIED", "CLOSED", "ERRATA", "NOTABUG"
	testCases := []struct {
		name     string
		state    *BugzillaBugState
		bug      *bugzilla.Bug
		expected bool
	}{
		{
			name: "both pointers are nil -> false",
		},
		{
			name: "state pointer is nil -> false",
			bug:  &bugzilla.Bug{},
		},
		{
			name:  "bug pointer is nil -> false",
			state: &BugzillaBugState{},
		},
		{
			name:     "statuses do not match -> false",
			state:    &BugzillaBugState{Status: modified, Resolution: errata},
			bug:      &bugzilla.Bug{Status: closed, Resolution: errata},
			expected: false,
		},
		{
			name:     "resolutions do not match -> false",
			state:    &BugzillaBugState{Status: closed, Resolution: notabug},
			bug:      &bugzilla.Bug{Status: closed, Resolution: errata},
			expected: false,
		},
		{
			name:     "no state enforced -> true",
			state:    &BugzillaBugState{},
			bug:      &bugzilla.Bug{Status: closed, Resolution: errata},
			expected: true,
		},
		{
			name:     "status match, resolution not enforced -> true",
			state:    &BugzillaBugState{Status: closed},
			bug:      &bugzilla.Bug{Status: closed, Resolution: errata},
			expected: true,
		},
		{
			name:     "status not enforced, resolution match -> true",
			state:    &BugzillaBugState{Resolution: errata},
			bug:      &bugzilla.Bug{Status: closed, Resolution: errata},
			expected: true,
		},
		{
			name:     "status and resolution match -> true",
			state:    &BugzillaBugState{Status: closed, Resolution: errata},
			bug:      &bugzilla.Bug{Status: closed, Resolution: errata},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.state.Matches(tc.bug)
			if actual != tc.expected {
				t.Errorf("%s: expected %t, got %t", tc.name, tc.expected, actual)
			}
		})
	}
}

func TestBugzillaBugState_AsBugUpdate(t *testing.T) {
	modified, closed, errata, notabug := "MODIFIED", "CLOSED", "ERRATA", "NOTABUG"
	testCases := []struct {
		name     string
		state    *BugzillaBugState
		bug      *bugzilla.Bug
		expected *bugzilla.BugUpdate
	}{
		{
			name:     "bug is nil so update contains whole state",
			state:    &BugzillaBugState{Status: closed, Resolution: errata},
			expected: &bugzilla.BugUpdate{Status: closed, Resolution: errata},
		},
		{
			name:     "bug is empty so update contains whole state",
			state:    &BugzillaBugState{Status: closed, Resolution: errata},
			bug:      &bugzilla.Bug{},
			expected: &bugzilla.BugUpdate{Status: closed, Resolution: errata},
		},
		{
			name:     "state is empty so update is nil",
			state:    &BugzillaBugState{},
			bug:      &bugzilla.Bug{Status: closed, Resolution: errata},
			expected: nil,
		},
		{
			name:     "status differs so update contains it",
			state:    &BugzillaBugState{Status: closed},
			bug:      &bugzilla.Bug{Status: modified, Resolution: errata},
			expected: &bugzilla.BugUpdate{Status: closed},
		},
		{
			name:     "resolution differs so update contains it",
			state:    &BugzillaBugState{Status: closed, Resolution: errata},
			bug:      &bugzilla.Bug{Status: closed, Resolution: notabug},
			expected: &bugzilla.BugUpdate{Resolution: errata},
		},
		{
			name:     "status and resolution match so update is nil",
			state:    &BugzillaBugState{Status: closed, Resolution: errata},
			bug:      &bugzilla.Bug{Status: closed, Resolution: errata},
			expected: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.state.AsBugUpdate(tc.bug)
			if tc.expected != actual {
				if actual == nil {
					t.Errorf("%s: unexpected nil", tc.name)
				}
				if tc.expected == nil {
					t.Errorf("%s: expected nil, got %v", tc.name, actual)
				}
			}

			if !reflect.DeepEqual(tc.expected, actual) {
				t.Errorf("%s: BugUpdate differs from expected:\n%s", tc.name, diff.ObjectReflectDiff(*actual, *tc.expected))
			}
		})
	}
}

func TestBugzillaBugStateSet_Has(t *testing.T) {
	bugInProgress := BugzillaBugState{Status: "MODIFIED"}
	bugErrata := BugzillaBugState{Status: "CLOSED", Resolution: "ERRATA"}
	bugWontfix := BugzillaBugState{Status: "CLOSED", Resolution: "WONTFIX"}

	testCases := []struct {
		name   string
		states []BugzillaBugState
		state  BugzillaBugState

		expectedLength int
		expectedHas    bool
	}{
		{
			name:           "empty set",
			state:          bugInProgress,
			expectedLength: 0,
			expectedHas:    false,
		},
		{
			name:           "membership",
			states:         []BugzillaBugState{bugInProgress},
			state:          bugInProgress,
			expectedLength: 1,
			expectedHas:    true,
		},
		{
			name:           "non-membership",
			states:         []BugzillaBugState{bugInProgress, bugErrata},
			state:          bugWontfix,
			expectedLength: 2,
			expectedHas:    false,
		},
		{
			name:           "actually a set",
			states:         []BugzillaBugState{bugInProgress, bugInProgress, bugInProgress},
			state:          bugInProgress,
			expectedLength: 1,
			expectedHas:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			set := NewBugzillaBugStateSet(tc.states)
			if len(set) != tc.expectedLength {
				t.Errorf("%s: expected set to have %d members, it has %d", tc.name, tc.expectedLength, len(set))
			}
			var not string
			if !tc.expectedHas {
				not = "not "
			}
			has := set.Has(tc.state)
			if has != tc.expectedHas {
				t.Errorf("%s: expected set to %scontain %v", tc.name, not, tc.state)
			}
		})
	}
}

func TestStatesMatch(t *testing.T) {
	modified := BugzillaBugState{Status: "MODIFIED"}
	errata := BugzillaBugState{Status: "CLOSED", Resolution: "ERRATA"}
	wontfix := BugzillaBugState{Status: "CLOSED", Resolution: "WONTFIX"}
	testCases := []struct {
		name          string
		first, second []BugzillaBugState
		expected      bool
	}{
		{
			name:     "empty slices match",
			expected: true,
		},
		{
			name:  "one empty, one non-empty do not match",
			first: []BugzillaBugState{modified},
		},
		{
			name:     "identical slices match",
			first:    []BugzillaBugState{modified},
			second:   []BugzillaBugState{modified},
			expected: true,
		},
		{
			name:     "ordering does not matter",
			first:    []BugzillaBugState{modified, errata},
			second:   []BugzillaBugState{errata, modified},
			expected: true,
		},
		{
			name:     "different slices do not match",
			first:    []BugzillaBugState{modified, errata},
			second:   []BugzillaBugState{modified, wontfix},
			expected: false,
		},
		{
			name:     "suffix in first operand is not ignored",
			first:    []BugzillaBugState{modified, errata},
			second:   []BugzillaBugState{modified},
			expected: false,
		},
		{
			name:     "suffix in second operand is not ignored",
			first:    []BugzillaBugState{modified},
			second:   []BugzillaBugState{modified, errata},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := statesMatch(tc.first, tc.second)
			if actual != tc.expected {
				t.Errorf("%s: expected %t, got %t", tc.name, tc.expected, actual)
			}
		})
	}
}

func TestValidateConfigUpdater(t *testing.T) {
	testCases := []struct {
		name        string
		cu          *ConfigUpdater
		expected    error
		expectedMsg string
	}{
		{
			name: "same key of different cms in different ns",
			cu: &ConfigUpdater{
				Maps: map[string]ConfigMapSpec{
					"core-services/prow/02_config/_plugins.yaml": {
						Name:     "plugins",
						Key:      "plugins.yaml",
						Clusters: map[string][]string{"first": {"some-namespace"}},
					},
					"somewhere/else/plugins.yaml": {
						Name:     "plugins",
						Key:      "plugins.yaml",
						Clusters: map[string][]string{"first": {"other-namespace"}},
					},
				},
			},
			expected: nil,
		},
		{
			name: "same key of a cm in the same ns",
			cu: &ConfigUpdater{
				Maps: map[string]ConfigMapSpec{
					"core-services/prow/02_config/_plugins.yaml": {
						Name:     "plugins",
						Key:      "plugins.yaml",
						Clusters: map[string][]string{"first": {"some-namespace"}},
					},
					"somewhere/else/plugins.yaml": {
						Name:     "plugins",
						Key:      "plugins.yaml",
						Clusters: map[string][]string{"first": {"some-namespace"}},
					},
				},
			},
			expected: fmt.Errorf("key plugins.yaml in configmap plugins updated with more than one file"),
		},
		{
			name: "same key of a cm in the same ns different clusters",
			cu: &ConfigUpdater{
				Maps: map[string]ConfigMapSpec{
					"core-services/prow/02_config/_plugins.yaml": {
						Name:     "plugins",
						Key:      "plugins.yaml",
						Clusters: map[string][]string{"first": {"some-namespace"}},
					},
					"somewhere/else/plugins.yaml": {
						Name:     "plugins",
						Key:      "plugins.yaml",
						Clusters: map[string][]string{"other": {"some-namespace"}},
					},
				},
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := validateConfigUpdater(tc.cu)
			if tc.expected == nil && actual != nil {
				t.Errorf("unexpected error: '%v'", actual)
			}
			if tc.expected != nil && actual == nil {
				t.Errorf("expected error '%v'', but it is nil", tc.expected)
			}
			if tc.expected != nil && actual != nil && tc.expected.Error() != actual.Error() {
				t.Errorf("expected error '%v', but it is '%v'", tc.expected, actual)
			}
		})
	}
}

func TestConfigUpdaterResolve(t *testing.T) {
	testCases := []struct {
		name           string
		in             ConfigUpdater
		expectedConfig ConfigUpdater
		exppectedError string
	}{
		{
			name:           "both cluster and cluster_groups is set, error",
			in:             ConfigUpdater{Maps: map[string]ConfigMapSpec{"map": {Clusters: map[string][]string{"cluster": nil}, ClusterGroups: []string{"group"}}}},
			exppectedError: "item maps.map contains both clusters and cluster_groups",
		},
		{
			name:           "inexistent cluster_group is referenced, error",
			in:             ConfigUpdater{Maps: map[string]ConfigMapSpec{"map": {ClusterGroups: []string{"group"}}}},
			exppectedError: "item maps.map.cluster_groups.0 references inexistent cluster group named group",
		},
		{
			name: "successful resolving",
			in: ConfigUpdater{
				ClusterGroups: map[string]ClusterGroup{
					"some-group":    {Clusters: []string{"cluster-a"}, Namespaces: []string{"namespace-a"}},
					"another-group": {Clusters: []string{"cluster-b"}, Namespaces: []string{"namespace-b"}},
				},
				Maps: map[string]ConfigMapSpec{"map": {
					Name:          "name",
					Key:           "key",
					GZIP:          utilpointer.BoolPtr(true),
					ClusterGroups: []string{"some-group", "another-group"}},
				},
			},
			expectedConfig: ConfigUpdater{
				Maps: map[string]ConfigMapSpec{"map": {
					Name: "name",
					Key:  "key",
					GZIP: utilpointer.BoolPtr(true),
					Clusters: map[string][]string{
						"cluster-a": {"namespace-a"},
						"cluster-b": {"namespace-b"},
					}}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			var errMsg string
			err := tc.in.resolve()
			if err != nil {
				errMsg = err.Error()
			}
			if errMsg != tc.exppectedError {
				t.Fatalf("expected error %s, got error %s", tc.exppectedError, errMsg)
			}
			if err != nil {
				return
			}

			if diff := cmp.Diff(tc.expectedConfig, tc.in); diff != "" {
				t.Errorf("expected config differs from actual config: %s", diff)
			}
		})
	}
}

func TestEnabledReposForPlugin(t *testing.T) {
	pluginsYaml := []byte(`
orgA:
 excluded_repos:
 - repoB
 plugins:
 - pluginCommon
 - pluginNotForRepoB
orgA/repoB:
 plugins:
 - pluginCommon
 - pluginOnlyForRepoB
`)
	var p Plugins
	err := yaml.Unmarshal(pluginsYaml, &p)
	if err != nil {
		t.Errorf("cannot unmarshal plugins config: %v", err)
	}
	cfg := Configuration{
		Plugins: p,
	}
	testCases := []struct {
		name              string
		wantOrgs          []string
		wantRepos         []string
		wantExcludedRepos map[string]sets.String
	}{
		{
			name:              "pluginCommon",
			wantOrgs:          []string{"orgA"},
			wantRepos:         []string{"orgA/repoB"},
			wantExcludedRepos: map[string]sets.String{"orgA": {}},
		},
		{
			name:              "pluginNotForRepoB",
			wantOrgs:          []string{"orgA"},
			wantRepos:         nil,
			wantExcludedRepos: map[string]sets.String{"orgA": {"orgA/repoB": {}}},
		},
		{
			name:              "pluginOnlyForRepoB",
			wantOrgs:          nil,
			wantRepos:         []string{"orgA/repoB"},
			wantExcludedRepos: map[string]sets.String{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			orgs, repos, excludedRepos := cfg.EnabledReposForPlugin(tc.name)
			if diff := cmp.Diff(tc.wantOrgs, orgs); diff != "" {
				t.Errorf("expected wantOrgs differ from actual: %s", diff)
			}
			if diff := cmp.Diff(tc.wantRepos, repos); diff != "" {
				t.Errorf("expected repos differ from actual: %s", diff)
			}
			if diff := cmp.Diff(tc.wantExcludedRepos, excludedRepos); diff != "" {
				t.Errorf("expected excludedRepos differ from actual: %s", diff)
			}
		})
	}
}

func TestPluginsUnmarshalFailed(t *testing.T) {
	badPluginsYaml := []byte(`
orgA:
 excluded_repos = [ repoB ]
 plugins:
 - pluginCommon
 - pluginNotForRepoB
orgA/repoB:
 plugins:
 - pluginCommon
 - pluginOnlyForRepoB
`)
	var p Plugins
	err := p.UnmarshalJSON(badPluginsYaml)
	if err == nil {
		t.Error("expected unmarshal error but didn't get one")
	}
}
