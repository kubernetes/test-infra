/*
Copyright 2020 The Kubernetes Authors.

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

package flagutil

import (
	"testing"
)

func TestGitHubEnablementValidation(t *testing.T) {
	testCases := []struct {
		name                    string
		gitHubEnablementOptions GitHubEnablementOptions
		expectedErrorString     string
	}{
		{
			name: "Empty config is valid",
		},
		{
			name: "Valid config",
			gitHubEnablementOptions: GitHubEnablementOptions{
				enabledOrgs:   Strings{vals: []string{"org-a"}},
				enabledRepos:  Strings{vals: []string{"org-b/repo-a"}},
				disabledOrgs:  Strings{vals: []string{"org-c"}},
				disabledRepos: Strings{vals: []string{"org-a/repo-b"}},
			},
		},
		{
			name: "Invalid enabled repo",
			gitHubEnablementOptions: GitHubEnablementOptions{
				enabledRepos: Strings{vals: []string{"not-a-valid-repo"}},
			},
			expectedErrorString: `--github-enabled-repo=not-a-valid-repo is invalid: "not-a-valid-repo" is not in org/repo format`,
		},
		{
			name: "Invalid disabled repo",
			gitHubEnablementOptions: GitHubEnablementOptions{
				disabledRepos: Strings{vals: []string{"not-a-valid-repo"}},
			},
			expectedErrorString: `--github-disabled-repo=not-a-valid-repo is invalid: "not-a-valid-repo" is not in org/repo format`,
		},
		{
			name: "Org overlap",
			gitHubEnablementOptions: GitHubEnablementOptions{
				enabledOrgs:  Strings{vals: []string{"org-a", "org-b"}},
				disabledOrgs: Strings{vals: []string{"org-b", "org-c"}},
			},
			expectedErrorString: "[org-b] is in both --github-enabled-org and --github-disabled-org",
		},
		{
			name: "Repo overlap",
			gitHubEnablementOptions: GitHubEnablementOptions{
				enabledRepos:  Strings{vals: []string{"org-a/repo-a", "org-b/repo-b"}},
				disabledRepos: Strings{vals: []string{"org-a/repo-a", "org-b/repo-b"}},
			},
			expectedErrorString: "[org-a/repo-a org-b/repo-b] is in both --github-enabled-repo and --github-disabled-repo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var actualErrMsg string
			actualErr := tc.gitHubEnablementOptions.Validate(false)
			if actualErr != nil {
				actualErrMsg = actualErr.Error()
			}
			if actualErrMsg != tc.expectedErrorString {
				t.Errorf("actual error %v does not match expected error %q", actualErr, tc.expectedErrorString)
			}
		})
	}
}

func TestEnablementChecker(t *testing.T) {
	inOrg, inRepo := "org", "repo"
	testCases := []struct {
		name                    string
		gitHubEnablementOptions GitHubEnablementOptions
		expectAllowed           bool
	}{
		{
			name:          "Defalt allows everything",
			expectAllowed: true,
		},
		{
			name: "Allowed orgs do not include it, forbidden",
			gitHubEnablementOptions: GitHubEnablementOptions{
				enabledOrgs: Strings{vals: []string{"other"}},
			},
		},
		{
			name: "Allowed orgs include it, allowed",
			gitHubEnablementOptions: GitHubEnablementOptions{
				enabledOrgs: Strings{vals: []string{"org"}},
			},
			expectAllowed: true,
		},
		{
			name: "Allowed repos do not include it, forbidden",
			gitHubEnablementOptions: GitHubEnablementOptions{
				enabledRepos: Strings{vals: []string{"other/repo"}},
			},
		},
		{
			name: "Allowed repos include it, allowed",
			gitHubEnablementOptions: GitHubEnablementOptions{
				enabledRepos: Strings{vals: []string{"org/repo"}},
			},
			expectAllowed: true,
		},
		{
			name: "Disabled orgs include it, forbidden",
			gitHubEnablementOptions: GitHubEnablementOptions{
				disabledOrgs: Strings{vals: []string{"org"}},
			},
		},
		{
			name: "Disabled repos include it, forbidden",
			gitHubEnablementOptions: GitHubEnablementOptions{
				disabledRepos: Strings{vals: []string{"org/repo"}},
			},
		},
		{
			name: "Allowed orgs and disabled repos include it, forbidden",
			gitHubEnablementOptions: GitHubEnablementOptions{
				enabledOrgs:   Strings{vals: []string{"org"}},
				disabledRepos: Strings{vals: []string{"org/repo"}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if result := tc.gitHubEnablementOptions.EnablementChecker()(inOrg, inRepo); result != tc.expectAllowed {
				t.Errorf("expected result %t, got result %t", tc.expectAllowed, result)
			}
		})
	}
}
