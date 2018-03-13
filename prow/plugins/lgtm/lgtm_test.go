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

package lgtm

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestLGTMComment(t *testing.T) {
	var testcases = []struct {
		name          string
		body          string
		commenter     string
		hasLGTM       bool
		shouldToggle  bool
		shouldComment bool
		shouldAssign  bool
	}{
		{
			name:         "non-lgtm comment",
			body:         "uh oh",
			commenter:    "o",
			hasLGTM:      false,
			shouldToggle: false,
		},
		{
			name:         "lgtm comment by reviewer, no lgtm on pr",
			body:         "/lgtm",
			commenter:    "reviewer1",
			hasLGTM:      false,
			shouldToggle: true,
		},
		{
			name:         "LGTM comment by reviewer, no lgtm on pr",
			body:         "/LGTM",
			commenter:    "reviewer1",
			hasLGTM:      false,
			shouldToggle: true,
		},
		{
			name:         "lgtm comment by reviewer, lgtm on pr",
			body:         "/lgtm",
			commenter:    "reviewer1",
			hasLGTM:      true,
			shouldToggle: false,
		},
		{
			name:          "lgtm comment by author",
			body:          "/lgtm",
			commenter:     "author",
			hasLGTM:       false,
			shouldToggle:  false,
			shouldComment: true,
		},
		{
			name:         "lgtm cancel by author",
			body:         "/lgtm cancel",
			commenter:    "author",
			hasLGTM:      true,
			shouldToggle: true,
		},
		{
			name:          "lgtm comment by non-reviewer",
			body:          "/lgtm",
			commenter:     "o",
			hasLGTM:       false,
			shouldToggle:  true,
			shouldComment: false,
			shouldAssign:  true,
		},
		{
			name:          "lgtm comment by non-reviewer, with trailing space",
			body:          "/lgtm ",
			commenter:     "o",
			hasLGTM:       false,
			shouldToggle:  true,
			shouldComment: false,
			shouldAssign:  true,
		},
		{
			name:          "lgtm comment by non-reviewer, with no-issue",
			body:          "/lgtm no-issue",
			commenter:     "o",
			hasLGTM:       false,
			shouldToggle:  true,
			shouldComment: false,
			shouldAssign:  true,
		},
		{
			name:          "lgtm comment by non-reviewer, with no-issue and trailing space",
			body:          "/lgtm no-issue \r",
			commenter:     "o",
			hasLGTM:       false,
			shouldToggle:  true,
			shouldComment: false,
			shouldAssign:  true,
		},
		{
			name:          "lgtm comment by rando",
			body:          "/lgtm",
			commenter:     "not-in-the-org",
			hasLGTM:       false,
			shouldToggle:  false,
			shouldComment: true,
			shouldAssign:  false,
		},
		{
			name:          "lgtm cancel by non-reviewer",
			body:          "/lgtm cancel",
			commenter:     "o",
			hasLGTM:       true,
			shouldToggle:  true,
			shouldComment: false,
			shouldAssign:  true,
		},
		{
			name:          "lgtm cancel by rando",
			body:          "/lgtm cancel",
			commenter:     "not-in-the-org",
			hasLGTM:       true,
			shouldToggle:  false,
			shouldComment: true,
			shouldAssign:  false,
		},
		{
			name:         "lgtm cancel comment by reviewer",
			body:         "/lgtm cancel",
			commenter:    "reviewer1",
			hasLGTM:      true,
			shouldToggle: true,
		},
		{
			name:         "lgtm cancel comment by reviewer, with trailing space",
			body:         "/lgtm cancel \r",
			commenter:    "reviewer1",
			hasLGTM:      true,
			shouldToggle: true,
		},
		{
			name:         "lgtm cancel comment by reviewer, no lgtm",
			body:         "/lgtm cancel",
			commenter:    "reviewer1",
			hasLGTM:      false,
			shouldToggle: false,
		},
	}
	for _, tc := range testcases {
		fc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
		}
		e := &github.GenericCommentEvent{
			Action:      github.GenericCommentActionCreated,
			IssueState:  "open",
			IsPR:        true,
			Body:        tc.body,
			User:        github.User{Login: tc.commenter},
			IssueAuthor: github.User{Login: "author"},
			Number:      5,
			Assignees:   []github.User{{Login: "author"}, {Login: "reviewer1"}, {Login: "reviewer2"}},
			Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			HTMLURL:     "<url>",
		}
		if tc.hasLGTM {
			fc.LabelsAdded = []string{"org/repo#5:" + lgtmLabel}
		}
		if err := handle(fc, logrus.WithField("plugin", pluginName), e); err != nil {
			t.Errorf("For case %s, didn't expect error from lgtmComment: %v", tc.name, err)
			continue
		}
		if tc.shouldAssign {
			found := false
			for _, a := range fc.AssigneesAdded {
				if a == fmt.Sprintf("%s/%s#%d:%s", "org", "repo", 5, tc.commenter) {
					found = true
					break
				}
			}
			if !found || len(fc.AssigneesAdded) != 1 {
				t.Errorf("For case %s, should have assigned %s but added assignees are %s", tc.name, tc.commenter, fc.AssigneesAdded)
			}
		} else if len(fc.AssigneesAdded) != 0 {
			t.Errorf("For case %s, should not have assigned anyone but assigned %s", tc.name, fc.AssigneesAdded)
		}
		if tc.shouldToggle {
			if tc.hasLGTM {
				if len(fc.LabelsRemoved) == 0 {
					t.Errorf("For case %s, should have removed LGTM.", tc.name)
				} else if len(fc.LabelsAdded) > 1 {
					t.Errorf("For case %s, should not have added LGTM.", tc.name)
				}
			} else {
				if len(fc.LabelsAdded) == 0 {
					t.Errorf("For case %s, should have added LGTM.", tc.name)
				} else if len(fc.LabelsRemoved) > 0 {
					t.Errorf("For case %s, should not have removed LGTM.", tc.name)
				}
			}
		} else if len(fc.LabelsRemoved) > 0 {
			t.Errorf("For case %s, should not have removed LGTM.", tc.name)
		} else if (tc.hasLGTM && len(fc.LabelsAdded) > 1) || (!tc.hasLGTM && len(fc.LabelsAdded) > 0) {
			t.Errorf("For case %s, should not have added LGTM.", tc.name)
		}
		if tc.shouldComment && len(fc.IssueComments[5]) != 1 {
			t.Errorf("For case %s, should have commented.", tc.name)
		} else if !tc.shouldComment && len(fc.IssueComments[5]) != 0 {
			t.Errorf("For case %s, should not have commented.", tc.name)
		}
	}
}

func TestLGTMCommentWithLGTMNoti(t *testing.T) {
	var testcases = []struct {
		name         string
		body         string
		commenter    string
		shouldDelete bool
	}{
		{
			name:         "non-lgtm comment",
			body:         "uh oh",
			commenter:    "o",
			shouldDelete: false,
		},
		{
			name:         "lgtm comment by reviewer, no lgtm on pr",
			body:         "/lgtm",
			commenter:    "reviewer1",
			shouldDelete: true,
		},
		{
			name:         "LGTM comment by reviewer, no lgtm on pr",
			body:         "/LGTM",
			commenter:    "reviewer1",
			shouldDelete: true,
		},
		{
			name:         "lgtm comment by author",
			body:         "/lgtm",
			commenter:    "author",
			shouldDelete: false,
		},
		{
			name:         "lgtm comment by non-reviewer",
			body:         "/lgtm",
			commenter:    "o",
			shouldDelete: true,
		},
		{
			name:         "lgtm comment by non-reviewer, with trailing space",
			body:         "/lgtm ",
			commenter:    "o",
			shouldDelete: true,
		},
		{
			name:         "lgtm comment by non-reviewer, with no-issue",
			body:         "/lgtm no-issue",
			commenter:    "o",
			shouldDelete: true,
		},
		{
			name:         "lgtm comment by non-reviewer, with no-issue and trailing space",
			body:         "/lgtm no-issue \r",
			commenter:    "o",
			shouldDelete: true,
		},
		{
			name:         "lgtm comment by rando",
			body:         "/lgtm",
			commenter:    "not-in-the-org",
			shouldDelete: false,
		},
		{
			name:         "lgtm cancel comment by reviewer, no lgtm",
			body:         "/lgtm cancel",
			commenter:    "reviewer1",
			shouldDelete: false,
		},
	}
	for _, tc := range testcases {
		fc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
		}
		e := &github.GenericCommentEvent{
			Action:      github.GenericCommentActionCreated,
			IssueState:  "open",
			IsPR:        true,
			Body:        tc.body,
			User:        github.User{Login: tc.commenter},
			IssueAuthor: github.User{Login: "author"},
			Number:      5,
			Assignees:   []github.User{{Login: "author"}, {Login: "reviewer1"}, {Login: "reviewer2"}},
			Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			HTMLURL:     "<url>",
		}
		botName, err := fc.BotName()
		if err != nil {
			t.Fatalf("For case %s, could not get Bot nam", tc.name)
		}
		ic := github.IssueComment{
			User: github.User{
				Login: botName,
			},
			Body: removeLGTMLabelNoti,
		}
		fc.IssueComments[5] = append(fc.IssueComments[5], ic)
		if err := handle(fc, logrus.WithField("plugin", pluginName), e); err != nil {
			t.Errorf("For case %s, didn't expect error from lgtmComment: %v", tc.name, err)
			continue
		}
		found := false
		for _, v := range fc.IssueComments[5] {
			if v.User.Login == botName && v.Body == removeLGTMLabelNoti {
				found = true
				break
			}
		}
		if tc.shouldDelete {
			if found {
				t.Errorf("For case %s, LGTM removed notification should have been deleted", tc.name)
			}
		} else {
			if !found {
				t.Errorf("For case %s, LGTM removed notification should not have been deleted", tc.name)
			}
		}
	}
}

type fakeIssueComment struct {
	Owner   string
	Repo    string
	Number  int
	Comment string
}

type githubUnlabeler struct {
	labelsRemoved    []string
	issueComments    []fakeIssueComment
	removeLabelErr   error
	createCommentErr error
}

func (c *githubUnlabeler) RemoveLabel(owner, repo string, pr int, label string) error {
	c.labelsRemoved = append(c.labelsRemoved, label)
	return c.removeLabelErr
}

func (c *githubUnlabeler) CreateComment(owner, repo string, number int, comment string) error {
	ic := fakeIssueComment{
		Owner:   owner,
		Repo:    repo,
		Number:  number,
		Comment: comment,
	}
	c.issueComments = append(c.issueComments, ic)
	return c.createCommentErr
}

func TestHandlePullRequest(t *testing.T) {
	cases := []struct {
		name             string
		event            github.PullRequestEvent
		removeLabelErr   error
		createCommentErr error

		err           error
		labelsRemoved []string
		issueComments []fakeIssueComment

		expectNoComments bool
	}{
		{
			name: "pr_synchronize, no RemoveLabel error",
			event: github.PullRequestEvent{
				Action: github.PullRequestActionSynchronize,
				PullRequest: github.PullRequest{
					Number: 101,
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "kubernetes",
							},
							Name: "kubernetes",
						},
					},
				},
			},
			labelsRemoved: []string{lgtmLabel},
			issueComments: []fakeIssueComment{
				{
					Owner:   "kubernetes",
					Repo:    "kubernetes",
					Number:  101,
					Comment: removeLGTMLabelNoti,
				},
			},
			expectNoComments: false,
		},
		{
			name: "pr_assigned",
			event: github.PullRequestEvent{
				Action: "assigned",
			},
			expectNoComments: true,
		},
		{
			name: "pr_synchronize, with RemoveLabel github.LabelNotFound error",
			event: github.PullRequestEvent{
				Action: github.PullRequestActionSynchronize,
				PullRequest: github.PullRequest{
					Number: 101,
					Base: github.PullRequestBranch{
						Repo: github.Repo{
							Owner: github.User{
								Login: "kubernetes",
							},
							Name: "kubernetes",
						},
					},
				},
			},
			removeLabelErr: &github.LabelNotFound{
				Owner:  "kubernetes",
				Repo:   "kubernetes",
				Number: 101,
				Label:  lgtmLabel,
			},
			labelsRemoved:    []string{lgtmLabel},
			expectNoComments: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeGitHub := &githubUnlabeler{
				removeLabelErr:   c.removeLabelErr,
				createCommentErr: c.createCommentErr,
			}
			err := handlePullRequest(fakeGitHub, c.event, logrus.WithField("plugin", pluginName))

			if err != nil && c.err == nil {
				t.Fatalf("handlePullRequest error: %v", err)
			}

			if err == nil && c.err != nil {
				t.Fatalf("handlePullRequest wanted error: %v, got nil", c.err)
			}

			if got, want := err, c.err; !equality.Semantic.DeepEqual(got, want) {
				t.Fatalf("handlePullRequest error mismatch: got %v, want %v", got, want)
			}

			if got, want := len(fakeGitHub.labelsRemoved), len(c.labelsRemoved); got != want {
				t.Logf("labelsRemoved: got %v, want: %v", fakeGitHub.labelsRemoved, c.labelsRemoved)
				t.Fatalf("labelsRemoved length mismatch: got %d, want %d", got, want)
			}

			if got, want := fakeGitHub.issueComments, c.issueComments; !equality.Semantic.DeepEqual(got, want) {
				t.Fatalf("LGTM revmoved notifications mismatch: got %v, want %v", got, want)
			}
			if c.expectNoComments && len(fakeGitHub.issueComments) > 0 {
				t.Fatalf("expected no comments but got %v", fakeGitHub.issueComments)
			}
			if !c.expectNoComments && len(fakeGitHub.issueComments) == 0 {
				t.Fatalf("expected comments but got none")
			}
		})
	}
}
