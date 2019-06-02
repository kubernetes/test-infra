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

package inrepoconfig

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/inrepoconfig/api"
)

// TestHandlePullRequest tests the HandlePullRequest func
func TestHandlePullRequest(t *testing.T) {
	const org, repo = "org", "repo"
	testCases := []struct {
		name        string
		baseContent map[string][]byte
		headContent map[string][]byte
		prepareFunc func(*fakegithub.FakeClient)
		verifyFunc  func(*fakegithub.FakeClient, string, []config.Presubmit) error
		errExpected bool
	}{
		{
			name: "Verify happy status gets created after success",
			verifyFunc: func(fgh *fakegithub.FakeClient, headSHA string, presubmits []config.Presubmit) error {
				statuses, err := fgh.ListStatuses("", "", headSHA)
				if err != nil {
					return fmt.Errorf("Error listing statuses: %v", err)
				}
				if n := len(statuses); n != 1 {
					return fmt.Errorf("Expected to find exactly one status, got %d", n)
				}
				if statuses[0].Context != api.ContextName {
					return fmt.Errorf("Expexted context name to be %q but was %q",
						api.ContextName, statuses[0].Context)
				}
				if statuses[0].State != "success" {
					return fmt.Errorf(`Expected context status to be "success" but was %q`, statuses[0].State)
				}
				if n := len(presubmits); n != 0 {
					return fmt.Errorf("Didn't expect any presubmits, got %d", n)
				}
				return nil
			},
		},
		{
			name:        "Verify sad status gets created after failure",
			baseContent: map[string][]byte{api.ConfigFileName: []byte("this-is-certainly-invalid")},
			verifyFunc: func(fgh *fakegithub.FakeClient, headSHA string, ps []config.Presubmit) error {
				statuses, err := fgh.ListStatuses("", "", headSHA)
				if err != nil {
					return fmt.Errorf("Error listing statuses: %v", err)
				}
				if n := len(statuses); n != 1 {
					return fmt.Errorf("Expected to find exactly one status, got %d", n)
				}
				if statuses[0].Context != api.ContextName {
					return fmt.Errorf("Expexted context name to be %q but was %q",
						api.ContextName, statuses[0].Context)
				}
				if statuses[0].State != "failure" {
					return fmt.Errorf(`Expected context status to be "failure" but was %q`, statuses[0].State)
				}
				if n := len(ps); n != 0 {
					return fmt.Errorf("Didn't expect any presubmits, got %d", n)
				}
				return nil
			},
			errExpected: true,
		},
		{
			name:        "Verify comment gets created after parse failure",
			baseContent: map[string][]byte{api.ConfigFileName: []byte("this-is-certainly-invalid")},
			verifyFunc: func(fgh *fakegithub.FakeClient, headSHA string, ps []config.Presubmit) error {
				comments, err := fgh.ListIssueComments("", "", 0)
				if err != nil {
					return fmt.Errorf("error listing comments: %v", err)
				}
				if n := len(comments); n != 1 {
					return fmt.Errorf("expected exactly one comment, got %d", n)
				}
				expectedBody := "<!-- inrepoconfig report -->\n@: Loading `prow.yaml` failed with the following error:\n```\nfailed to parse \"prow.yaml\": error unmarshaling JSON: while decoding JSON: json: cannot unmarshal string into Go value of type api.InRepoConfig\n```"
				if comments[0].Body != expectedBody {
					return fmt.Errorf("Expected body to be %q but was %q", expectedBody, comments[0].Body)
				}
				if n := len(ps); n != 0 {
					return fmt.Errorf("Didn't expect any presubmits, got %d", n)
				}
				return nil
			},
			errExpected: true,
		},
		{
			name:        "Verify comment gets updated after parse failure",
			baseContent: map[string][]byte{api.ConfigFileName: []byte("this-is-certainly-invalid")},
			prepareFunc: func(ghc *fakegithub.FakeClient) {
				// Needed because comment id zero means "no comment found"
				ghc.IssueCommentID = 1
				_ = ghc.CreateComment(org, repo, 0, commentTag)
			},
			verifyFunc: func(fgh *fakegithub.FakeClient, headSHA string, ps []config.Presubmit) error {
				comments, err := fgh.ListIssueComments("", "", 0)
				if err != nil {
					return fmt.Errorf("error listing comments: %v", err)
				}
				if n := len(comments); n != 1 {
					return fmt.Errorf("expected exactly one comment, got %d", n)
				}
				expectedBody := "<!-- inrepoconfig report -->\n@: Loading `prow.yaml` failed with the following error:\n```\nfailed to parse \"prow.yaml\": error unmarshaling JSON: while decoding JSON: json: cannot unmarshal string into Go value of type api.InRepoConfig\n```"
				if comments[0].Body != expectedBody {
					return fmt.Errorf("Expected body to be %q but was %q", expectedBody, comments[0].Body)
				}
				if n := len(ps); n != 0 {
					return fmt.Errorf("Didn't expect any presubmits, got %d", n)
				}
				return nil
			},
			errExpected: true,
		},
		{
			name: "Verify comment gets removed on success",
			prepareFunc: func(ghc *fakegithub.FakeClient) {
				// Needed because comment id zero means "no comment found"
				ghc.IssueCommentID = 1
				_ = ghc.CreateComment(org, repo, 0, commentTag)
			},
			verifyFunc: func(fgh *fakegithub.FakeClient, headSHA string, ps []config.Presubmit) error {
				comments, err := fgh.ListIssueComments("", "", 0)
				if err != nil {
					return fmt.Errorf("error listing comments: %v", err)
				}
				if n := len(comments); n != 0 {
					return fmt.Errorf("expected exactly zero comments, got %d", n)
				}
				if n := len(ps); n != 0 {
					return fmt.Errorf("Didn't expect any presubmits, got %d", n)
				}
				return nil
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.baseContent == nil {
				tc.baseContent = map[string][]byte{"some-file": []byte("some-content")}
			}
			if tc.headContent == nil {
				tc.headContent = map[string][]byte{"another-file": []byte("some-content")}
			}
			lg, gc, err := localgit.New()
			if err != nil {
				t.Fatalf("Making local git repo: %v", err)
			}
			defer func() {
				if err := lg.Clean(); err != nil {
					t.Errorf("Error cleaning LocalGit: %v", err)
				}
				if err := gc.Clean(); err != nil {
					t.Errorf("Error cleaning Client: %v", err)
				}
			}()
			if err := lg.MakeFakeRepo(org, repo); err != nil {
				t.Fatalf("Making fake repo: %v", err)
			}

			if err := lg.AddCommit(org, repo, tc.baseContent); err != nil {
				t.Fatalf("failed to add commit to base: %v", err)
			}
			if err := lg.CheckoutNewBranch(org, repo, "can-I-haz-pulled"); err != nil {
				t.Fatalf("failed to create new branch: %v", err)
			}
			if err := lg.AddCommit(org, repo, tc.headContent); err != nil {
				t.Fatalf("failed to add head commit: %v", err)
			}
			baseSHA, err := lg.RevParse(org, repo, "master")
			if err != nil {
				t.Fatalf("failed to get baseSHA: %v", err)
			}
			headSHA, err := lg.RevParse(org, repo, "HEAD")
			if err != nil {
				t.Fatalf("failed to head headSHA: %v", err)
			}

			cfg := &config.Config{
				ProwConfig: config.ProwConfig{
					PodNamespace: "my-pod-ns",
				},
			}
			logger := logrus.WithField("testcase", tc.name)
			pr := github.PullRequest{
				Base: github.PullRequestBranch{
					Repo: github.Repo{
						Name: repo,
						Owner: github.User{
							Login: org,
						},
					},
				},
				Head: github.PullRequestBranch{
					SHA: headSHA,
				},
			}

			fgh := &fakegithub.FakeClient{}
			fakegithub.TestRef = baseSHA
			if tc.prepareFunc != nil {
				tc.prepareFunc(fgh)
			}

			_, presubmits, err := HandlePullRequest(logger, cfg, fgh, gc, pr)
			if err != nil {
				if !tc.errExpected {
					t.Fatalf("Unexpected error: %v", err)
				}
			}
			if tc.errExpected && err == nil {
				t.Fatalf("Expected error but didn't get it")
			}

			// We execute this even after an error to test the failure cases
			if err := tc.verifyFunc(fgh, headSHA, presubmits); err != nil {
				t.Fatalf("Error executing verify: %v", err)
			}
		})
	}
}
