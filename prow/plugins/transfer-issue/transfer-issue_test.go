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

package transferissue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode"

	"github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

const issuerNum = 1

func Test_handleTransfer(t *testing.T) {
	ts := []struct {
		name         string
		event        github.GenericCommentEvent
		expectError  bool
		errorMessage string
		comment      string
		fcFunc       func(client *fakegithub.FakeClient)
		tcFunc       func(client *testClient)
	}{
		{
			name:  "is a pr",
			event: github.GenericCommentEvent{IsPR: true},
		},
		{
			name:  "is not comment added",
			event: github.GenericCommentEvent{Action: github.GenericCommentActionDeleted},
		},
		{
			name: "multiple matches",
			event: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				Body: `/transfer-issue kubectl
/transfer test-infra`,
				HTMLURL: fmt.Sprintf("https://github.com/kubernetes/fake/issues/%d", issuerNum),
				Number:  issuerNum,
				Repo:    github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
				User:    github.User{Login: "user"},
			},
			comment: "single destination",
		},
		{
			name: "no destination",
			event: github.GenericCommentEvent{
				Action:  github.GenericCommentActionCreated,
				Body:    "/transfer",
				HTMLURL: fmt.Sprintf("https://github.com/kubernetes/fake/issues/%d", issuerNum),
				Number:  issuerNum,
				Repo:    github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
				User:    github.User{Login: "user"},
			},
			comment: "single destination",
		},
		{
			name: "dest repo does not exist",
			event: github.GenericCommentEvent{
				Action:  github.GenericCommentActionCreated,
				Body:    "/transfer-issue fake",
				HTMLURL: fmt.Sprintf("https://github.com/kubernetes/fake/issues/%d", issuerNum),
				Number:  issuerNum,
				Repo:    github.Repo{Owner: github.User{Login: "kubernetes"}, Name: "kubectl"},
				User:    github.User{Login: "user"},
			},
			comment: "does not exist",
			fcFunc: func(fc *fakegithub.FakeClient) {
				fc.GetRepoError = errors.New("stub")
			},
		},
		{
			name: "not collaborator",
			event: github.GenericCommentEvent{
				Action:  github.GenericCommentActionCreated,
				Body:    "/transfer-issue test-infra",
				HTMLURL: fmt.Sprintf("https://github.com/kubernetes/fake/issues/%d", issuerNum),
				Number:  issuerNum,
				Repo:    github.Repo{Owner: github.User{Login: "kubernetes"}, Name: "kubectl"},
				User:    github.User{Login: "user"},
			},
			comment: "must be an org member",
			fcFunc: func(fc *fakegithub.FakeClient) {
				fc.OrgMembers["kubernetes"] = []string{}
			},
		},
		{
			name: "trims whitespace from user input",
			event: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				Body:   "/transfer-issue test-infra\r",
				Number: issuerNum,
				Repo:   github.Repo{Owner: github.User{Login: "kubernetes"}, Name: "kubectl"},
				User:   github.User{Login: "user"},
				NodeID: "fakeIssueNodeID",
			},
			fcFunc: func(fc *fakegithub.FakeClient) {
				fc.OrgMembers["kubernetes"] = []string{"user"}
			},
			tcFunc: func(c *testClient) {
				c.repoNodeID = "fakeRepoNodeID"
			},
		},
		{
			name: "happy path",
			event: github.GenericCommentEvent{
				Action: github.GenericCommentActionCreated,
				Body: `This belongs elsewhere
/transfer-issue test-infra
Thanks!`,
				Number: issuerNum,
				Repo:   github.Repo{Owner: github.User{Login: "kubernetes"}, Name: "kubectl"},
				User:   github.User{Login: "user"},
				NodeID: "fakeIssueNodeID",
			},
			fcFunc: func(fc *fakegithub.FakeClient) {
				fc.OrgMembers["kubernetes"] = []string{"user"}
			},
			tcFunc: func(c *testClient) {
				c.repoNodeID = "fakeRepoNodeID"
			},
		},
	}

	for _, tc := range ts {
		t.Run(tc.name, func(t *testing.T) {
			fc := fakegithub.NewFakeClient()
			c := &testClient{fc: fc}
			if tc.tcFunc != nil {
				tc.tcFunc(c)
			}
			if tc.fcFunc != nil {
				tc.fcFunc(fc)
			}
			log := logrus.WithField("plugin", pluginName)
			err := handleTransfer(c, log, tc.event)
			if err != nil {
				if !tc.expectError {
					t.Fatalf("unexpected error: %v", err)
				}
				if m := err.Error(); !strings.Contains(m, tc.errorMessage) {
					t.Fatalf("expected error to contain: %s got: %v", tc.errorMessage, m)
				}
			}
			if err == nil && tc.expectError {
				t.Fatalf("expected error but did not produce")
			}
			if len(tc.comment) != 0 {
				if cm, ok := fc.IssueComments[tc.event.Number]; ok {
					if !strings.Contains(cm[0].Body, tc.comment) {
						t.Errorf("expected comment to contain: %s got: %s", tc.comment, cm[0].Body)
					}
				} else {
					t.Errorf("expected comment to contain: %s but no comment", tc.comment)
				}
			}
			if len(tc.comment) == 0 && len(fc.IssueComments[issuerNum]) != 0 {
				t.Errorf("unexpected comment: %v", fc.IssueComments[issuerNum])
			}
		})
	}
}

type testRoundTripper struct {
	rt func(*http.Request) (*http.Response, error)
}

func (rt testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt.rt(r)
}

type testClient struct {
	fc         *fakegithub.FakeClient
	repoNodeID string
}

func (t *testClient) GetRepo(org, name string) (github.FullRepo, error) {
	r := []rune(name)
	if lastChar := r[len(r)-1]; unicode.IsSpace(lastChar) {
		return github.FullRepo{}, fmt.Errorf("failed creating new request: parse \"https://api.github.com/repos/%s/%s\\r\": net/url: invalid control character in URL", org, name)
	}
	repo, err := t.fc.GetRepo(org, name)
	if len(t.repoNodeID) != 0 {
		repo.NodeID = t.repoNodeID
	}
	return repo, err
}

func (t *testClient) CreateComment(org, repo string, number int, comment string) error {
	return t.fc.CreateComment(org, repo, number, comment)
}

func (t *testClient) IsMember(org, user string) (bool, error) {
	return t.fc.IsMember(org, user)
}

func (t *testClient) MutateWithGitHubAppsSupport(ctx context.Context, m interface{}, input githubv4.Input, vars map[string]interface{}, org string) error {
	mr := `{"data": { "transferIssue": { "issue": { "url": "https://kubernetes.io/fake" } } } }`

	gqlc := githubv4.NewClient(&http.Client{
		Transport: testRoundTripper{rt: func(r *http.Request) (*http.Response, error) {
			defer r.Body.Close()
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			s := string(body)
			if !(strings.Contains(s, "fakeRepoNodeID") && strings.Contains(s, "fakeIssueNodeID")) {
				return nil, fmt.Errorf("unexpected request body: %s", s)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(mr))}, nil
		}},
	})

	return gqlc.Mutate(ctx, m, input, vars)
}

func Test_transferIssue(t *testing.T) {
	c := &testClient{}
	issue, err := transferIssue(c, "k8s", "fakeRepoNodeID", "fakeIssueNodeID")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := issue.TransferIssue.Issue.URL.String(); got != "https://kubernetes.io/fake" {
		t.Fatalf("unexpected url: %v", got)
	}
}
