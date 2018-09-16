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
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/repoowners"
)

type fakeOwnersClient struct {
	approvers map[string]sets.String
	reviewers map[string]sets.String
}

var _ repoowners.Interface = &fakeOwnersClient{}

func (f *fakeOwnersClient) LoadRepoAliases(org, repo, base string) (repoowners.RepoAliases, error) {
	return nil, nil
}

func (f *fakeOwnersClient) LoadRepoOwners(org, repo, base string) (repoowners.RepoOwnerInterface, error) {
	return &fakeRepoOwners{approvers: f.approvers, reviewers: f.reviewers}, nil
}

type fakeRepoOwners struct {
	approvers map[string]sets.String
	reviewers map[string]sets.String
}

var _ repoowners.RepoOwnerInterface = &fakeRepoOwners{}

func (f *fakeRepoOwners) FindApproverOwnersForFile(path string) string  { return "" }
func (f *fakeRepoOwners) FindReviewersOwnersForFile(path string) string { return "" }
func (f *fakeRepoOwners) FindLabelsForFile(path string) sets.String     { return nil }
func (f *fakeRepoOwners) IsNoParentOwners(path string) bool             { return false }
func (f *fakeRepoOwners) LeafApprovers(path string) sets.String         { return nil }
func (f *fakeRepoOwners) Approvers(path string) sets.String             { return f.approvers[path] }
func (f *fakeRepoOwners) LeafReviewers(path string) sets.String         { return nil }
func (f *fakeRepoOwners) Reviewers(path string) sets.String             { return f.reviewers[path] }
func (f *fakeRepoOwners) RequiredReviewers(path string) sets.String     { return nil }

var approvers = map[string]sets.String{
	"doc/README.md": {
		"cjwagner": {},
		"jessica":  {},
	},
}

var reviewers = map[string]sets.String{
	"doc/README.md": {
		"alice": {},
		"bob":   {},
		"mark":  {},
		"sam":   {},
	},
}

func TestLGTMComment(t *testing.T) {
	var testcases = []struct {
		name          string
		body          string
		commenter     string
		hasLGTM       bool
		shouldToggle  bool
		shouldComment bool
		shouldAssign  bool
		skipCollab    bool
		prs           map[int]*github.PullRequest
		changes       map[int][]github.PullRequestChange
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
			name:          "lgtm cancel by author",
			body:          "/lgtm cancel",
			commenter:     "author",
			hasLGTM:       true,
			shouldToggle:  true,
			shouldAssign:  false,
			shouldComment: false,
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
		{
			name:         "lgtm comment, based off OWNERS only",
			body:         "/lgtm",
			commenter:    "sam",
			hasLGTM:      false,
			shouldToggle: true,
			skipCollab:   true,
			prs: map[int]*github.PullRequest{
				5: {
					Base: github.PullRequestBranch{
						Ref: "master",
					},
				},
			},
			changes: map[int][]github.PullRequestChange{
				5: {
					{Filename: "doc/README.md"},
				},
			},
		},
	}
	for _, tc := range testcases {
		t.Logf("Running scenario %q", tc.name)
		fc := &fakegithub.FakeClient{
			IssueComments:      make(map[int][]github.IssueComment),
			PullRequests:       tc.prs,
			PullRequestChanges: tc.changes,
		}
		e := &github.GenericCommentEvent{
			Action:      github.GenericCommentActionCreated,
			IssueState:  "open",
			IsPR:        true,
			Body:        tc.body,
			User:        github.User{Login: tc.commenter},
			IssueAuthor: github.User{Login: "author"},
			Number:      5,
			Assignees:   []github.User{{Login: "reviewer1"}, {Login: "reviewer2"}},
			Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
			HTMLURL:     "<url>",
		}
		if tc.hasLGTM {
			fc.LabelsAdded = []string{"org/repo#5:" + LGTMLabel}
		}
		oc := &fakeOwnersClient{approvers: approvers, reviewers: reviewers}
		pc := &plugins.Configuration{}
		if tc.skipCollab {
			pc.Owners.SkipCollaborators = []string{"org/repo"}
		}
		if err := handleGenericComment(fc, pc, oc, logrus.WithField("plugin", PluginName), *e); err != nil {
			t.Errorf("didn't expect error from lgtmComment: %v", err)
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
				t.Errorf("should have assigned %s but added assignees are %s", tc.commenter, fc.AssigneesAdded)
			}
		} else if len(fc.AssigneesAdded) != 0 {
			t.Errorf("should not have assigned anyone but assigned %s", fc.AssigneesAdded)
		}
		if tc.shouldToggle {
			if tc.hasLGTM {
				if len(fc.LabelsRemoved) == 0 {
					t.Errorf("should have removed LGTM.")
				} else if len(fc.LabelsAdded) > 1 {
					t.Errorf("should not have added LGTM.")
				}
			} else {
				if len(fc.LabelsAdded) == 0 {
					t.Errorf("should have added LGTM.")
				} else if len(fc.LabelsRemoved) > 0 {
					t.Errorf("should not have removed LGTM.")
				}
			}
		} else if len(fc.LabelsRemoved) > 0 {
			t.Errorf("should not have removed LGTM.")
		} else if (tc.hasLGTM && len(fc.LabelsAdded) > 1) || (!tc.hasLGTM && len(fc.LabelsAdded) > 0) {
			t.Errorf("should not have added LGTM.")
		}
		if tc.shouldComment && len(fc.IssueComments[5]) != 1 {
			t.Errorf("should have commented.")
		} else if !tc.shouldComment && len(fc.IssueComments[5]) != 0 {
			t.Errorf("should not have commented.")
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
			Assignees:   []github.User{{Login: "reviewer1"}, {Login: "reviewer2"}},
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
		oc := &fakeOwnersClient{approvers: approvers, reviewers: reviewers}
		pc := &plugins.Configuration{}
		if err := handleGenericComment(fc, pc, oc, logrus.WithField("plugin", PluginName), *e); err != nil {
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

func TestLGTMFromApproveReview(t *testing.T) {
	var testcases = []struct {
		name          string
		state         github.ReviewState
		body          string
		reviewer      string
		hasLGTM       bool
		shouldToggle  bool
		shouldComment bool
		shouldAssign  bool
	}{
		{
			name:          "Request changes review by reviewer, no lgtm on pr",
			state:         github.ReviewStateChangesRequested,
			reviewer:      "reviewer1",
			hasLGTM:       false,
			shouldToggle:  false,
			shouldAssign:  false,
			shouldComment: false,
		},
		{
			name:         "Request changes review by reviewer, lgtm on pr",
			state:        github.ReviewStateChangesRequested,
			reviewer:     "reviewer1",
			hasLGTM:      true,
			shouldToggle: true,
			shouldAssign: false,
		},
		{
			name:         "Approve review by reviewer, no lgtm on pr",
			state:        github.ReviewStateApproved,
			reviewer:     "reviewer1",
			hasLGTM:      false,
			shouldToggle: true,
		},
		{
			name:         "Approve review by reviewer, lgtm on pr",
			state:        github.ReviewStateApproved,
			reviewer:     "reviewer1",
			hasLGTM:      true,
			shouldToggle: false,
			shouldAssign: false,
		},
		{
			name:          "Approve review by non-reviewer, no lgtm on pr",
			state:         github.ReviewStateApproved,
			reviewer:      "o",
			hasLGTM:       false,
			shouldToggle:  true,
			shouldComment: false,
			shouldAssign:  true,
		},
		{
			name:          "Request changes review by non-reviewer, no lgtm on pr",
			state:         github.ReviewStateChangesRequested,
			reviewer:      "o",
			hasLGTM:       false,
			shouldToggle:  false,
			shouldComment: false,
			shouldAssign:  true,
		},
		{
			name:          "Approve review by rando",
			state:         github.ReviewStateApproved,
			reviewer:      "not-in-the-org",
			hasLGTM:       false,
			shouldToggle:  false,
			shouldComment: true,
			shouldAssign:  false,
		},
		{
			name:          "Comment review by issue author, no lgtm on pr",
			state:         github.ReviewStateCommented,
			reviewer:      "author",
			hasLGTM:       false,
			shouldToggle:  false,
			shouldComment: false,
			shouldAssign:  false,
		},
		{
			name:          "Comment body has /lgtm on Comment Review ",
			state:         github.ReviewStateCommented,
			reviewer:      "reviewer1",
			body:          "/lgtm",
			hasLGTM:       false,
			shouldToggle:  false,
			shouldComment: false,
			shouldAssign:  false,
		},
		{
			name:          "Comment body has /lgtm cancel on Approve Review",
			state:         github.ReviewStateApproved,
			reviewer:      "reviewer1",
			body:          "/lgtm cancel",
			hasLGTM:       false,
			shouldToggle:  false,
			shouldComment: false,
			shouldAssign:  false,
		},
	}
	for _, tc := range testcases {
		fc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
		}
		e := &github.ReviewEvent{
			Review:      github.Review{Body: tc.body, State: tc.state, HTMLURL: "<url>", User: github.User{Login: tc.reviewer}},
			PullRequest: github.PullRequest{User: github.User{Login: "author"}, Assignees: []github.User{{Login: "reviewer1"}, {Login: "reviewer2"}}, Number: 5},
			Repo:        github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
		}
		if tc.hasLGTM {
			fc.LabelsAdded = []string{"org/repo#5:" + LGTMLabel}
		}
		oc := &fakeOwnersClient{approvers: approvers, reviewers: reviewers}
		pc := &plugins.Configuration{}
		if err := handlePullRequestReview(fc, pc, oc, logrus.WithField("plugin", PluginName), *e); err != nil {
			t.Errorf("For case %s, didn't expect error from pull request review: %v", tc.name, err)
			continue
		}
		if tc.shouldAssign {
			found := false
			for _, a := range fc.AssigneesAdded {
				if a == fmt.Sprintf("%s/%s#%d:%s", "org", "repo", 5, tc.reviewer) {
					found = true
					break
				}
			}
			if !found || len(fc.AssigneesAdded) != 1 {
				t.Errorf("For case %s, should have assigned %s but added assignees are %s", tc.name, tc.reviewer, fc.AssigneesAdded)
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
			labelsRemoved: []string{LGTMLabel},
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
				Label:  LGTMLabel,
			},
			labelsRemoved:    []string{LGTMLabel},
			expectNoComments: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeGitHub := &githubUnlabeler{
				removeLabelErr:   c.removeLabelErr,
				createCommentErr: c.createCommentErr,
			}
			err := handlePullRequest(fakeGitHub, c.event, logrus.WithField("plugin", PluginName))

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
