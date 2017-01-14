/*
Copyright 2016 The Kubernetes Authors.

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
	"strings"
	"testing"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/jobs"
	"k8s.io/test-infra/prow/line"
)

func TestFailureComment(t *testing.T) {
	comments := []github.IssueComment{
		{
			User: github.User{Login: "unrelated"},
			Body: "looks nice",
			ID:   0,
		},
		{
			User: github.User{Login: "k8s-ci-robot"},
			Body: "Jenkins test failed for commit abcdef",
			ID:   1,
		},
		{
			User: github.User{Login: "unrelated2"},
			Body: "Jenkins test is strange, what's going on there?",
			ID:   3,
		},
		{
			User: github.User{Login: "k8s-ci-robot"},
			Body: "Jenkins test failed for commit qwerty",
			ID:   8,
		},
	}
	ghc := &fakegithub.FakeClient{
		IssueComments: map[int][]github.IssueComment{
			5: comments,
		},
		IssueCommentID: 9,
	}
	cl := testClient{
		Presubmit: jobs.Presubmit{
			Name:    "test-job",
			Context: "Jenkins test",
		},
		PRNumber:     5,
		PullSHA:      "abcde",
		Report:       true,
		GitHubClient: ghc,
	}
	cl.tryCreateFailureComment("url")
	newComments, _ := ghc.ListIssueComments("", "", 5)
	if len(newComments) != 3 {
		t.Errorf("Expected 3 comments after creating failed comment, got %+v", newComments)
	}
	for _, comment := range newComments {
		if comment.ID == 1 || comment.ID == 8 {
			t.Errorf("Comment not deleted: %v", comment.ID)
		}
	}
}

func TestFormatFailureComment(t *testing.T) {
	var testcases = []struct {
		owner    string
		name     string
		expected string
	}{
		{"kubernetes", "kubernetes", "pr-test.k8s.io/12"},
		{"kubernetes", "kops", "pr-test.k8s.io/kops/12"},
		{"google", "cadvisor", "pr-test.k8s.io/google_cadvisor/12"},
	}
	for _, tc := range testcases {
		c := testClient{
			RepoOwner: tc.owner,
			RepoName:  tc.name,
			PRNumber:  12,
		}
		body := c.formatFailureComment("someurl")
		if !strings.Contains(body, "someurl") {
			t.Error("body doesn't contain url")
		}
		if !strings.Contains(body, tc.expected) {
			t.Errorf("body missing string %v: %v", tc.expected, body)
		}
	}
}

func TestGuberURL(t *testing.T) {
	var testcases = []struct {
		IsPresubmit bool
		PRNumber    int
		RepoOwner   string
		RepoName    string
		ExpectedURL string
	}{
		{
			true,
			5,
			"kubernetes",
			"kubernetes",
			guberBasePR + "/5/j/1/",
		},
		{
			true,
			5,
			"kubernetes",
			"charts",
			guberBasePR + "/charts/5/j/1/",
		},
		{
			true,
			5,
			"other",
			"kubernetes",
			guberBasePR + "/other_kubernetes/5/j/1/",
		},
		{
			true,
			5,
			"other",
			"other",
			guberBasePR + "/other_other/5/j/1/",
		},
		{
			true,
			0,
			"kubernetes",
			"kubernetes",
			guberBasePR + "/batch/j/1/",
		},
		{
			false,
			0,
			"kubernetes",
			"kubernetes",
			guberBasePush + "/j/1/",
		},
		{
			false,
			0,
			"o",
			"o",
			guberBasePush + "/o_o/j/1/",
		},
	}
	for _, tc := range testcases {
		c := &testClient{
			IsPresubmit: tc.IsPresubmit,
			Presubmit:   jobs.Presubmit{Name: "j"},
			Postsubmit:  jobs.Postsubmit{Name: "j"},
			PRNumber:    tc.PRNumber,
			RepoOwner:   tc.RepoOwner,
			RepoName:    tc.RepoName,
		}
		actual := c.guberURL("1")
		if actual != tc.ExpectedURL {
			t.Errorf("Gubernator URL wrong. Got %s, expected %s", actual, tc.ExpectedURL)
		}
	}
}

func TestBuildReq(t *testing.T) {
	testcases := []struct {
		ref string
		br  line.BuildRequest
	}{
		{
			ref: "master:abcd",
			br: line.BuildRequest{
				BaseRef: "master",
				BaseSHA: "abcd",
			},
		},
		{
			ref: "master:abcd,123:qwer",
			br: line.BuildRequest{
				BaseRef: "master",
				BaseSHA: "abcd",
				Pulls: []line.Pull{
					{
						Number: 123,
						SHA:    "qwer",
					},
				},
			},
		},
		{
			ref: "master:abcd,123:qwer,456:wow",
			br: line.BuildRequest{
				BaseRef: "master",
				BaseSHA: "abcd",
				Pulls: []line.Pull{
					{
						Number: 123,
						SHA:    "qwer",
					},
					{
						Number: 456,
						SHA:    "wow",
					},
				},
			},
		},
	}
	for _, tc := range testcases {
		br, err := buildReq("org", "repo", "author", tc.ref)
		if err != nil {
			t.Fatalf("Didn't expect error in buildReq: %v", err)
		}
		if br.BaseRef != tc.br.BaseRef {
			t.Errorf("Got wrong base ref. Got %s, expected %s.", br.BaseRef, tc.br.BaseRef)
		}
		if br.BaseSHA != tc.br.BaseSHA {
			t.Errorf("Got wrong base SHA. Got %s, expected %s.", br.BaseSHA, tc.br.BaseSHA)
		}
		if len(br.Pulls) != len(tc.br.Pulls) {
			t.Fatalf("Got different sized pulls. Got %v, expected %v.", br.Pulls, tc.br.Pulls)
		}
		for i := range br.Pulls {
			if br.Pulls[i].Number != tc.br.Pulls[i].Number {
				t.Errorf("Got wrong pull number. Got %d, expected %d.", br.Pulls[i].Number, tc.br.Pulls[i].Number)
			}
			if br.Pulls[i].SHA != tc.br.Pulls[i].SHA {
				t.Errorf("Got wrong pull sha. Got %s, expected %s.", br.Pulls[i].SHA, tc.br.Pulls[i].SHA)
			}
		}
	}
}
