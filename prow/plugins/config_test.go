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
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
	"sigs.k8s.io/yaml"
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
				"prow/config.yaml":  {Name: "config", Namespaces: []string{""}},
				"prow/plugins.yaml": {Name: "plugins", Namespaces: []string{""}},
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
				"hello.yaml": {Name: "my-cm", Namespaces: []string{""}},
				"world.yaml": {Name: "you-cm", Namespaces: []string{""}},
			},
		},
		{
			name: "deprecated config",
			config: ConfigUpdater{
				ConfigFile: "foo.yaml",
			},
			expected: map[string]ConfigMapSpec{
				"foo.yaml":          {Name: "config", Namespaces: []string{""}},
				"prow/plugins.yaml": {Name: "plugins", Namespaces: []string{""}},
			},
		},
		{
			name: "deprecated plugins",
			config: ConfigUpdater{
				PluginFile: "bar.yaml",
			},
			expected: map[string]ConfigMapSpec{
				"bar.yaml":         {Name: "plugins", Namespaces: []string{""}},
				"prow/config.yaml": {Name: "config", Namespaces: []string{""}},
			},
		},
		{
			name: "deprecated both",
			config: ConfigUpdater{
				ConfigFile: "foo.yaml",
				PluginFile: "bar.yaml",
			},
			expected: map[string]ConfigMapSpec{
				"foo.yaml": {Name: "config", Namespaces: []string{""}},
				"bar.yaml": {Name: "plugins", Namespaces: []string{""}},
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
				ConfigFile: "config.yaml",
				PluginFile: "plugins.yaml",
			},
			expected: map[string]ConfigMapSpec{
				"config.yaml":        {Name: "overwrite-config", Namespaces: []string{""}},
				"plugins.yaml":       {Name: "overwrite-plugins", Namespaces: []string{""}},
				"unconflicting.yaml": {Name: "ignored", Namespaces: []string{""}},
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
	defaultComment := `This PR is not for the master branch but does not have the ` + "`cherry-pick-approved`" + `  label. Adding the ` + "`do-not-merge/cherry-pick-not-approved`" + `  label.

To approve the cherry-pick, please ping the *kubernetes/patch-release-team* in a comment when ready.

See also [Kubernetes Patch Releases](https://github.com/kubernetes/sig-release/blob/master/releases/patch-releases.md)`

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
	modified, verified := []string{"VERIFIED"}, []string{"MODIFIED"}
	post, pre := "POST", "PRE"
	var testCases = []struct {
		name          string
		parent, child BugzillaBranchOptions
		expected      BugzillaBranchOptions
	}{
		{
			name: "no parent or child means no output",
		},
		{
			name:     "no child means a copy of parent is the output",
			parent:   BugzillaBranchOptions{ValidateByDefault: &yes, IsOpen: &open, TargetRelease: &one, Statuses: &modified, DependentBugStatuses: &verified, StatusAfterValidation: &post},
			expected: BugzillaBranchOptions{ValidateByDefault: &yes, IsOpen: &open, TargetRelease: &one, Statuses: &modified, DependentBugStatuses: &verified, StatusAfterValidation: &post},
		},
		{
			name:     "no parent means a copy of child is the output",
			child:    BugzillaBranchOptions{ValidateByDefault: &yes, IsOpen: &open, TargetRelease: &one, Statuses: &modified, DependentBugStatuses: &verified, StatusAfterValidation: &post},
			expected: BugzillaBranchOptions{ValidateByDefault: &yes, IsOpen: &open, TargetRelease: &one, Statuses: &modified, DependentBugStatuses: &verified, StatusAfterValidation: &post},
		},
		{
			name:     "child overrides parent on IsOpen",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, Statuses: &modified, StatusAfterValidation: &post},
			child:    BugzillaBranchOptions{IsOpen: &closed},
			expected: BugzillaBranchOptions{IsOpen: &closed, TargetRelease: &one, Statuses: &modified, StatusAfterValidation: &post},
		},
		{
			name:     "child overrides parent on target release",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, Statuses: &modified, StatusAfterValidation: &post},
			child:    BugzillaBranchOptions{TargetRelease: &two},
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &two, Statuses: &modified, StatusAfterValidation: &post},
		},
		{
			name:     "child overrides parent on statuses",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, Statuses: &modified, StatusAfterValidation: &post},
			child:    BugzillaBranchOptions{Statuses: &verified},
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, Statuses: &verified, StatusAfterValidation: &post},
		},
		{
			name:     "child overrides parent on status after validation",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, Statuses: &modified, StatusAfterValidation: &post},
			child:    BugzillaBranchOptions{StatusAfterValidation: &pre},
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, Statuses: &modified, StatusAfterValidation: &pre},
		},
		{
			name:     "child overrides parent on validation by default",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, Statuses: &modified, StatusAfterValidation: &post},
			child:    BugzillaBranchOptions{ValidateByDefault: &yes},
			expected: BugzillaBranchOptions{ValidateByDefault: &yes, IsOpen: &open, TargetRelease: &one, Statuses: &modified, StatusAfterValidation: &post},
		},
		{
			name:     "child overrides parent on dependent bug statuses",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, Statuses: &modified, DependentBugStatuses: &verified, StatusAfterValidation: &post},
			child:    BugzillaBranchOptions{DependentBugStatuses: &modified},
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, Statuses: &modified, DependentBugStatuses: &modified, StatusAfterValidation: &post},
		},
		{
			name:     "child overrides parent on status after mege",
			parent:   BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, Statuses: &modified, StatusAfterValidation: &post, StatusAfterMerge: &post},
			child:    BugzillaBranchOptions{StatusAfterMerge: &pre},
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &one, Statuses: &modified, StatusAfterValidation: &post, StatusAfterMerge: &pre},
		},
		{
			name:     "child overrides parent on all fields",
			parent:   BugzillaBranchOptions{ValidateByDefault: &yes, IsOpen: &open, TargetRelease: &one, Statuses: &verified, DependentBugStatuses: &verified, StatusAfterValidation: &post, StatusAfterMerge: &post},
			child:    BugzillaBranchOptions{ValidateByDefault: &no, IsOpen: &closed, TargetRelease: &two, Statuses: &modified, DependentBugStatuses: &modified, StatusAfterValidation: &pre, StatusAfterMerge: &pre},
			expected: BugzillaBranchOptions{ValidateByDefault: &no, IsOpen: &closed, TargetRelease: &two, Statuses: &modified, DependentBugStatuses: &modified, StatusAfterValidation: &pre, StatusAfterMerge: &pre},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := ResolveBugzillaOptions(testCase.parent, testCase.child), testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: resolved incorrect options for parent and child: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}

func TestOptionsForBranch(t *testing.T) {
	open, closed := true, false
	yes, no := true, false
	globalDefault, globalBranchDefault, orgDefault, orgBranchDefault, repoDefault, repoBranch := "global-default", "global-branch-default", "my-org-default", "my-org-branch-default", "my-repo-default", "my-repo-branch"
	verified, modified := []string{"VERIFIED"}, []string{"MODIFIED"}
	post, pre, release, notabug := "POST", "PRE", "RELEASE_PENDING", "NOTABUG"

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
        status_after_validation: "PRE"
      "my-org-branch":
        target_release: my-org-branch-default
        status_after_validation: "POST"
    repos:
      my-repo:
        branches:
          "*":
            is_open: false
            target_release: my-repo-default
            statuses:
            - VERIFIED
            validate_by_default: false
            status_after_merge: RELEASE_PENDING
          "my-repo-branch":
            target_release: my-repo-branch
            statuses:
            - MODIFIED
            validate_by_default: true
            status_after_merge: NOTABUG`
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
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &orgDefault, StatusAfterValidation: &pre},
		},
		{
			name:     "branch on configured org but not repo gets org branch default",
			org:      "my-org",
			repo:     "some-repo",
			branch:   "my-org-branch",
			expected: BugzillaBranchOptions{IsOpen: &open, TargetRelease: &orgBranchDefault, StatusAfterValidation: &post},
		},
		{
			name:     "branch on configured org and repo gets repo default",
			org:      "my-org",
			repo:     "my-repo",
			branch:   "some-branch",
			expected: BugzillaBranchOptions{ValidateByDefault: &no, IsOpen: &closed, TargetRelease: &repoDefault, Statuses: &verified, StatusAfterValidation: &pre, StatusAfterMerge: &release},
		},
		{
			name:     "branch on configured org and repo gets branch config",
			org:      "my-org",
			repo:     "my-repo",
			branch:   "my-repo-branch",
			expected: BugzillaBranchOptions{ValidateByDefault: &yes, IsOpen: &closed, TargetRelease: &repoBranch, Statuses: &modified, StatusAfterValidation: &pre, StatusAfterMerge: &notabug},
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
				"*":             {IsOpen: &open, TargetRelease: &orgDefault, StatusAfterValidation: &pre},
				"my-org-branch": {IsOpen: &open, TargetRelease: &orgBranchDefault, StatusAfterValidation: &post},
			},
		},
		{
			name: "configured repo gets repo config",
			org:  "my-org",
			repo: "my-repo",
			expected: map[string]BugzillaBranchOptions{
				"*":              {ValidateByDefault: &no, IsOpen: &closed, TargetRelease: &repoDefault, Statuses: &verified, StatusAfterValidation: &pre, StatusAfterMerge: &release},
				"my-repo-branch": {ValidateByDefault: &yes, IsOpen: &closed, TargetRelease: &repoBranch, Statuses: &modified, StatusAfterValidation: &pre, StatusAfterMerge: &notabug},
				"my-org-branch":  {ValidateByDefault: &no, IsOpen: &closed, TargetRelease: &repoDefault, Statuses: &verified, StatusAfterValidation: &post, StatusAfterMerge: &release},
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
