/*
Copyright 2017 The Kubernetes Authors.

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

package approve

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ghodss/yaml"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/plugins"
)

const prNumber = 1

// TestPluginConfig validates that there are no duplicate repos in the approve plugin config.
func TestPluginConfig(t *testing.T) {
	pa := &plugins.PluginAgent{}

	b, err := ioutil.ReadFile("../../plugins.yaml")
	if err != nil {
		t.Fatalf("Failed to read plugin config: %v.", err)
	}
	np := &plugins.Configuration{}
	if err := yaml.Unmarshal(b, np); err != nil {
		t.Fatalf("Failed to unmarshal plugin config: %v.", err)
	}
	pa.Set(np)

	orgs := map[string]bool{}
	repos := map[string]bool{}
	for _, config := range pa.Config().Approve {
		for _, entry := range config.Repos {
			if strings.Contains(entry, "/") {
				if repos[entry] {
					t.Errorf("The repo %q is duplicated in the 'approve' plugin configuration.", entry)
				}
				repos[entry] = true
			} else {
				if orgs[entry] {
					t.Errorf("The org %q is duplicated in the 'approve' plugin configuration.", entry)
				}
				orgs[entry] = true
			}
		}
	}
	for repo := range repos {
		org := strings.Split(repo, "/")[0]
		if orgs[org] {
			t.Errorf("The repo %q is duplicated with %q in the 'approve' plugin configuration.", repo, org)
		}
	}
}

func newTestComment(user, body string) github.IssueComment {
	return github.IssueComment{User: github.User{Login: user}, Body: body}
}

func newTestCommentTime(t time.Time, user, body string) github.IssueComment {
	c := newTestComment(user, body)
	c.CreatedAt = t
	return c
}

func newTestReview(user, body, state string) github.Review {
	return github.Review{User: github.User{Login: user}, Body: body, State: state}
}

func newTestReviewTime(t time.Time, user, body, state string) github.Review {
	r := newTestReview(user, body, state)
	r.SubmittedAt = t
	return r
}

func newFakeGithubClient(hasLabel, humanApproved bool, files []string, comments []github.IssueComment, reviews []github.Review) *fakegithub.FakeClient {
	labels := []string{"org/repo#1:lgtm"}
	if hasLabel {
		labels = append(labels, fmt.Sprintf("org/repo#%v:approved", prNumber))
	}
	events := []github.ListedIssueEvent{
		{
			Event: github.IssueActionLabeled,
			Label: github.Label{Name: "approved"},
			Actor: github.User{Login: "k8s-merge-robot"},
		},
	}
	if humanApproved {
		events = append(
			events,
			github.ListedIssueEvent{
				Event:     github.IssueActionLabeled,
				Label:     github.Label{Name: "approved"},
				Actor:     github.User{Login: "human"},
				CreatedAt: time.Now(),
			},
		)
	}
	var changes []github.PullRequestChange
	for _, file := range files {
		changes = append(changes, github.PullRequestChange{Filename: file})
	}
	return &fakegithub.FakeClient{
		LabelsAdded:        labels,
		PullRequestChanges: map[int][]github.PullRequestChange{prNumber: changes},
		IssueComments:      map[int][]github.IssueComment{prNumber: comments},
		IssueEvents:        map[int][]github.ListedIssueEvent{prNumber: events},
		Reviews:            map[int][]github.Review{prNumber: reviews},
	}
}

type fakeRepo struct {
	approvers, leafApprovers map[string]sets.String
	approverOwners           map[string]string
}

func (fr fakeRepo) Approvers(path string) sets.String {
	return fr.approvers[path]
}
func (fr fakeRepo) LeafApprovers(path string) sets.String {
	return fr.leafApprovers[path]
}
func (fr fakeRepo) FindApproverOwnersForFile(path string) string {
	return fr.approverOwners[path]
}
func (fr fakeRepo) IsNoParentOwners(path string) bool {
	return false
}

func TestHandle(t *testing.T) {
	// This function does not need to test IsApproved, that is tested in approvers/approvers_test.go.

	// includes tests with mixed case usernames
	// includes tests with stale notifications
	tests := []struct {
		name          string
		branch        string
		prBody        string
		hasLabel      bool
		humanApproved bool
		files         []string
		comments      []github.IssueComment
		reviews       []github.Review

		selfApprove         bool
		needsIssue          bool
		lgtmActsAsApprove   bool
		reviewActsAsApprove bool

		expectDelete    bool
		expectComment   bool
		expectedComment string
		expectToggle    bool
	}{

		// breaking cases
		// case: /approve in PR body
		{
			name:                "initial notification (approved)",
			hasLabel:            false,
			files:               []string{"c/c.go"},
			comments:            []github.IssueComment{},
			reviews:             []github.Review{},
			selfApprove:         true,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  false,
			expectToggle:  true,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **APPROVED**

This pull-request has been approved by: *<a href="#" title="Author self-approved">cjwagner</a>*

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details >
Needs approval from an approver in each of these files:

- ~~[c/OWNERS](https://github.com/org/repo/blob/master/c/OWNERS)~~ [cjwagner]

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":[]} -->`,
		},
		{
			name:                "initial notification (unapproved)",
			hasLabel:            false,
			files:               []string{"c/c.go"},
			comments:            []github.IssueComment{},
			reviews:             []github.Review{},
			selfApprove:         false,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  false,
			expectToggle:  false,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by:
To fully approve this pull request, please assign additional approvers.
We suggest the following additional approver: **cjwagner**

If they are not already assigned, you can assign the PR to them by writing ` + "`/assign @cjwagner`" + ` in a comment when ready.

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details open>
Needs approval from an approver in each of these files:

- **[c/OWNERS](https://github.com/org/repo/blob/master/c/OWNERS)**

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":["cjwagner"]} -->`,
		},
		{
			name:                "no-issue comment",
			hasLabel:            false,
			files:               []string{"a/a.go"},
			comments:            []github.IssueComment{newTestComment("Alice", "stuff\n/approve no-issue \nmore stuff")},
			reviews:             []github.Review{},
			selfApprove:         false,
			needsIssue:          true,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  false,
			expectToggle:  true,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **APPROVED**

This pull-request has been approved by: *<a href="" title="Approved">Alice</a>*

Associated issue requirement bypassed by: *<a href="" title="Approved">Alice</a>*

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details >
Needs approval from an approver in each of these files:

- ~~[a/OWNERS](https://github.com/org/repo/blob/master/a/OWNERS)~~ [Alice]

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":[]} -->`,
		},
		{
			name:                "issue provided in PR body",
			prBody:              "some changes that fix #42.\n/assign",
			hasLabel:            false,
			files:               []string{"a/a.go"},
			comments:            []github.IssueComment{newTestComment("Alice", "stuff\n/approve")},
			reviews:             []github.Review{},
			selfApprove:         false,
			needsIssue:          true,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  false,
			expectToggle:  true,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **APPROVED**

This pull-request has been approved by: *<a href="" title="Approved">Alice</a>*

Associated issue: *#42*

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details >
Needs approval from an approver in each of these files:

- ~~[a/OWNERS](https://github.com/org/repo/blob/master/a/OWNERS)~~ [Alice]

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":[]} -->`,
		},
		{
			name:     "non-implicit self approve no-issue",
			hasLabel: false,
			files:    []string{"a/a.go", "c/c.go"},
			comments: []github.IssueComment{
				newTestComment("ALIcE", "stuff\n/approve"),
				newTestComment("cjwagner", "stuff\n/approve no-issue"),
			},
			reviews:             []github.Review{},
			selfApprove:         false,
			needsIssue:          true,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:    false,
			expectToggle:    true,
			expectComment:   true,
			expectedComment: "",
		},
		{
			name:     "implicit self approve, missing issue",
			hasLabel: false,
			files:    []string{"a/a.go", "c/c.go"},
			comments: []github.IssueComment{
				newTestComment("ALIcE", "stuff\n/approve"),
				newTestCommentTime(time.Now(), "k8s-ci-robot", `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by: *<a href="" title="Approved">ALIcE</a>*, *<a href="#" title="Author self-approved">cjwagner</a>*

*No associated issue*. Update pull-request body to add a reference to an issue, or get approval with `+"`/approve no-issue`"+`

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details >
Needs approval from an approver in each of these files:

- ~~[a/OWNERS](https://github.com/org/repo/blob/master/a/OWNERS)~~ [ALIcE]
- ~~[c/OWNERS](https://github.com/org/repo/blob/master/c/OWNERS)~~ [cjwagner]

Approvers can indicate their approval by writing `+"`/approve`"+` in a comment
Approvers can cancel approval by writing `+"`/approve cancel`"+` in a comment
</details>
<!-- META={"approvers":[]} -->`),
			},
			reviews:             []github.Review{},
			selfApprove:         true,
			needsIssue:          true,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  false,
			expectToggle:  false,
			expectComment: false,
		},
		{
			name:     "remove approval with /approve cancel",
			hasLabel: true,
			files:    []string{"a/a.go"},
			comments: []github.IssueComment{
				newTestComment("Alice", "/approve no-issue"),
				newTestComment("k8s-ci-robot", "[APPROVALNOTIFIER] This PR is **APPROVED**\n\nblah"),
				newTestComment("Alice", "stuff\n/approve cancel \nmore stuff"),
			},
			reviews:             []github.Review{},
			selfApprove:         true, // no-op test
			needsIssue:          true,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  true,
			expectToggle:  true,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by: *<a href="#" title="Author self-approved">cjwagner</a>*
To fully approve this pull request, please assign additional approvers.
We suggest the following additional approver: **alice**

If they are not already assigned, you can assign the PR to them by writing ` + "`/assign @alice`" + ` in a comment when ready.

*No associated issue*. Update pull-request body to add a reference to an issue, or get approval with ` + "`/approve no-issue`" + `

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details open>
Needs approval from an approver in each of these files:

- **[a/OWNERS](https://github.com/org/repo/blob/master/a/OWNERS)**

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":["alice"]} -->`,
		},
		{
			name:     "remove approval after sync",
			prBody:   "Changes the thing.\n fixes #42",
			hasLabel: true,
			files:    []string{"a/a.go", "b/b.go"},
			comments: []github.IssueComment{
				newTestComment("bOb", "stuff\n/approve \nblah"),
				newTestComment("k8s-ci-robot", "[APPROVALNOTIFIER] This PR is **APPROVED**\n\nblah"),
			},
			reviews:             []github.Review{},
			selfApprove:         true, // no-op test
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  true,
			expectToggle:  true,
			expectComment: true,
		},
		{
			name:     "cancel implicit self approve",
			prBody:   "Changes the thing.\n fixes #42",
			hasLabel: true,
			files:    []string{"c/c.go"},
			comments: []github.IssueComment{
				newTestComment("k8s-ci-robot", "[APPROVALNOTIFIER] This PR is **APPROVED**\n\nblah"),
				newTestCommentTime(time.Now(), "CJWagner", "stuff\n/approve cancel \nmore stuff"),
			},
			reviews:             []github.Review{},
			selfApprove:         true,
			needsIssue:          true,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  true,
			expectToggle:  true,
			expectComment: true,
		},
		{
			name:     "cancel implicit self approve (with lgtm-after-commit message)",
			prBody:   "Changes the thing.\n fixes #42",
			hasLabel: true,
			files:    []string{"c/c.go"},
			comments: []github.IssueComment{
				newTestComment("k8s-ci-robot", "[APPROVALNOTIFIER] This PR is **APPROVED**\n\nblah"),
				newTestCommentTime(time.Now(), "CJWagner", "/lgtm cancel //PR changed after LGTM, removing LGTM."),
			},
			reviews:             []github.Review{},
			selfApprove:         true,
			needsIssue:          true,
			lgtmActsAsApprove:   true,
			reviewActsAsApprove: false,

			expectDelete:  true,
			expectToggle:  true,
			expectComment: true,
		},
		{
			name:     "up to date, poked by pr sync",
			prBody:   "Finally fixes kubernetes/kubernetes#1\n",
			hasLabel: true,
			files:    []string{"a/a.go", "a/aa.go"},
			comments: []github.IssueComment{
				newTestComment("alice", "stuff\n/approve\nblah"),
				newTestCommentTime(time.Now(), "k8s-ci-robot", `[APPROVALNOTIFIER] This PR is **APPROVED**

This pull-request has been approved by: *<a href="" title="Approved">alice</a>*

Associated issue: *#1*

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details >
Needs approval from an approver in each of these files:

- ~~[a/OWNERS](https://github.com/org/repo/blob/master/a/OWNERS)~~ [alice]

Approvers can indicate their approval by writing `+"`/approve`"+` in a comment
Approvers can cancel approval by writing `+"`/approve cancel`"+` in a comment
</details>
<!-- META={"approvers":[]} -->`),
			},
			reviews:             []github.Review{},
			selfApprove:         false,
			needsIssue:          true,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  false,
			expectToggle:  false,
			expectComment: false,
		},
		{
			name:     "out of date, poked by pr sync",
			prBody:   "Finally fixes kubernetes/kubernetes#1\n",
			hasLabel: false,
			files:    []string{"a/a.go", "a/aa.go"}, // previous commits may have been ["b/b.go"]
			comments: []github.IssueComment{
				newTestComment("alice", "stuff\n/approve\nblah"),
				newTestCommentTime(time.Now(), "k8s-ci-robot", "[APPROVALNOTIFIER] This PR is **NOT APPROVED**\n\nblah"),
			},
			reviews:             []github.Review{},
			selfApprove:         false,
			needsIssue:          true,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  true,
			expectToggle:  true,
			expectComment: true,
		},
		{
			name:          "human added approve",
			hasLabel:      true,
			humanApproved: true,
			files:         []string{"a/a.go"},
			comments: []github.IssueComment{
				newTestComment("k8s-ci-robot", "[APPROVALNOTIFIER] This PR is **NOT APPROVED**\n\nblah"),
			},
			reviews:             []github.Review{},
			selfApprove:         false,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  true,
			expectToggle:  false,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **APPROVED**

Approval requirements bypassed by manually added approval.

This pull-request has been approved by:

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details >
Needs approval from an approver in each of these files:

- **[a/OWNERS](https://github.com/org/repo/blob/master/a/OWNERS)**

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":["alice"]} -->`,
		},
		{
			name:     "lgtm means approve",
			prBody:   "This is a great PR that will fix\nlots of things!",
			hasLabel: false,
			files:    []string{"a/a.go", "a/aa.go"},
			comments: []github.IssueComment{
				newTestComment("k8s-ci-robot", "[APPROVALNOTIFIER] This PR is **NOT APPROVED**\n\nblah"),
				newTestCommentTime(time.Now(), "alice", "stuff\n/lgtm\nblah"),
			},
			reviews:             []github.Review{},
			selfApprove:         false,
			needsIssue:          false,
			lgtmActsAsApprove:   true,
			reviewActsAsApprove: false,

			expectDelete:  true,
			expectToggle:  true,
			expectComment: true,
		},
		{
			name:     "lgtm does not mean approve",
			prBody:   "This is a great PR that will fix\nlots of things!",
			hasLabel: false,
			files:    []string{"a/a.go", "a/aa.go"},
			comments: []github.IssueComment{
				newTestComment("k8s-ci-robot", `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by:
To fully approve this pull request, please assign additional approvers.
We suggest the following additional approver: **alice**

If they are not already assigned, you can assign the PR to them by writing `+"`/assign @alice`"+` in a comment when ready.

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details open>
Needs approval from an approver in each of these files:

- **[a/OWNERS](https://github.com/org/repo/blob/master/a/OWNERS)**

Approvers can indicate their approval by writing `+"`/approve`"+` in a comment
Approvers can cancel approval by writing `+"`/approve cancel`"+` in a comment
</details>
<!-- META={"approvers":["alice"]} -->`),
				newTestCommentTime(time.Now(), "alice", "stuff\n/lgtm\nblah"),
			},
			reviews:             []github.Review{},
			selfApprove:         false,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  false,
			expectToggle:  false,
			expectComment: false,
		},
		{
			name:                "approve in review body with empty state",
			hasLabel:            false,
			files:               []string{"a/a.go"},
			comments:            []github.IssueComment{},
			reviews:             []github.Review{newTestReview("Alice", "stuff\n/approve", "")},
			selfApprove:         false,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  false,
			expectToggle:  true,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **APPROVED**

This pull-request has been approved by: *<a href="" title="Approved">Alice</a>*

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details >
Needs approval from an approver in each of these files:

- ~~[a/OWNERS](https://github.com/org/repo/blob/master/a/OWNERS)~~ [Alice]

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":[]} -->`,
		},
		{
			name:                "approved review but reviewActsAsApprove disabled",
			hasLabel:            false,
			files:               []string{"c/c.go"},
			comments:            []github.IssueComment{},
			reviews:             []github.Review{newTestReview("cjwagner", "stuff", approvedReviewState)},
			selfApprove:         false,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  false,
			expectToggle:  false,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by:
To fully approve this pull request, please assign additional approvers.
We suggest the following additional approver: **cjwagner**

If they are not already assigned, you can assign the PR to them by writing ` + "`/assign @cjwagner`" + ` in a comment when ready.

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details open>
Needs approval from an approver in each of these files:

- **[c/OWNERS](https://github.com/org/repo/blob/master/c/OWNERS)**

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":["cjwagner"]} -->`,
		},
		{
			name:                "approved review with reviewActsAsApprove enabled",
			hasLabel:            false,
			files:               []string{"a/a.go"},
			comments:            []github.IssueComment{},
			reviews:             []github.Review{newTestReview("Alice", "stuff", approvedReviewState)},
			selfApprove:         false,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: true,

			expectDelete:  false,
			expectToggle:  true,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **APPROVED**

This pull-request has been approved by: *<a href="" title="Approved">Alice</a>*

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details >
Needs approval from an approver in each of these files:

- ~~[a/OWNERS](https://github.com/org/repo/blob/master/a/OWNERS)~~ [Alice]

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":[]} -->`,
		},
		{
			name:     "reviews in non-approving state (should not approve)",
			hasLabel: false,
			files:    []string{"c/c.go"},
			comments: []github.IssueComment{},
			reviews: []github.Review{
				newTestReview("cjwagner", "stuff", "COMMENTED"),
				newTestReview("cjwagner", "unsubmitted stuff", "PENDING"),
			},
			selfApprove:         false,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: true,

			expectDelete:  false,
			expectToggle:  false,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by:
To fully approve this pull request, please assign additional approvers.
We suggest the following additional approver: **cjwagner**

If they are not already assigned, you can assign the PR to them by writing ` + "`/assign @cjwagner`" + ` in a comment when ready.

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details open>
Needs approval from an approver in each of these files:

- **[c/OWNERS](https://github.com/org/repo/blob/master/c/OWNERS)**

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":["cjwagner"]} -->`,
		},
		{
			name:     "review in request changes state means cancel",
			hasLabel: true,
			files:    []string{"c/c.go"},
			comments: []github.IssueComment{
				newTestCommentTime(time.Now().Add(time.Hour), "k8s-ci-robot", "[APPROVALNOTIFIER] This PR is **APPROVED**\n\nblah"), // second
			},
			reviews: []github.Review{
				newTestReviewTime(time.Now(), "cjwagner", "yep", approvedReviewState),                           // first
				newTestReviewTime(time.Now().Add(time.Hour*2), "cjwagner", "nope", changesRequestedReviewState), // third
			},
			selfApprove:         false,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: true,

			expectDelete:  true,
			expectToggle:  true,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by:
To fully approve this pull request, please assign additional approvers.
We suggest the following additional approver: **cjwagner**

If they are not already assigned, you can assign the PR to them by writing ` + "`/assign @cjwagner`" + ` in a comment when ready.

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details open>
Needs approval from an approver in each of these files:

- **[c/OWNERS](https://github.com/org/repo/blob/master/c/OWNERS)**

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":["cjwagner"]} -->`,
		},
		{
			name:     "approve cancel command supersedes earlier approved review",
			hasLabel: true,
			files:    []string{"c/c.go"},
			comments: []github.IssueComment{
				newTestCommentTime(time.Now().Add(time.Hour), "k8s-ci-robot", "[APPROVALNOTIFIER] This PR is **APPROVED**\n\nblah"), // second
				newTestCommentTime(time.Now().Add(time.Hour*2), "cjwagner", "stuff\n/approve cancel \nmore stuff"),                  // third
			},
			reviews: []github.Review{
				newTestReviewTime(time.Now(), "cjwagner", "yep", approvedReviewState), // first
			},
			selfApprove:         false,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: true,

			expectDelete:  true,
			expectToggle:  true,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by:
To fully approve this pull request, please assign additional approvers.
We suggest the following additional approver: **cjwagner**

If they are not already assigned, you can assign the PR to them by writing ` + "`/assign @cjwagner`" + ` in a comment when ready.

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details open>
Needs approval from an approver in each of these files:

- **[c/OWNERS](https://github.com/org/repo/blob/master/c/OWNERS)**

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":["cjwagner"]} -->`,
		},
		{
			name:     "approve cancel command supersedes simultaneous approved review",
			hasLabel: false,
			files:    []string{"c/c.go"},
			comments: []github.IssueComment{},
			reviews: []github.Review{
				newTestReview("cjwagner", "/approve cancel", approvedReviewState),
			},
			selfApprove:         false,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: true,

			expectDelete:  false,
			expectToggle:  false,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **NOT APPROVED**

This pull-request has been approved by:
To fully approve this pull request, please assign additional approvers.
We suggest the following additional approver: **cjwagner**

If they are not already assigned, you can assign the PR to them by writing ` + "`/assign @cjwagner`" + ` in a comment when ready.

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details open>
Needs approval from an approver in each of these files:

- **[c/OWNERS](https://github.com/org/repo/blob/master/c/OWNERS)**

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":["cjwagner"]} -->`,
		},
		{
			name:                "approve command supersedes simultaneous changes requested review",
			hasLabel:            false,
			files:               []string{"a/a.go"},
			comments:            []github.IssueComment{},
			reviews:             []github.Review{newTestReview("Alice", "/approve", changesRequestedReviewState)},
			selfApprove:         false,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: true,

			expectDelete:  false,
			expectToggle:  true,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **APPROVED**

This pull-request has been approved by: *<a href="" title="Approved">Alice</a>*

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details >
Needs approval from an approver in each of these files:

- ~~[a/OWNERS](https://github.com/org/repo/blob/master/a/OWNERS)~~ [Alice]

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":[]} -->`,
		},
		{
			name:                "different branch, initial notification (approved)",
			branch:              "dev",
			hasLabel:            false,
			files:               []string{"c/c.go"},
			comments:            []github.IssueComment{},
			reviews:             []github.Review{},
			selfApprove:         true,
			needsIssue:          false,
			lgtmActsAsApprove:   false,
			reviewActsAsApprove: false,

			expectDelete:  false,
			expectToggle:  true,
			expectComment: true,
			expectedComment: `[APPROVALNOTIFIER] This PR is **APPROVED**

This pull-request has been approved by: *<a href="#" title="Author self-approved">cjwagner</a>*

The full list of commands accepted by this bot can be found [here](https://go.k8s.io/bot-commands).

The pull request process is described [here](https://git.k8s.io/community/contributors/guide/owners.md#the-code-review-process)

<details >
Needs approval from an approver in each of these files:

- ~~[c/OWNERS](https://github.com/org/repo/blob/dev/c/OWNERS)~~ [cjwagner]

Approvers can indicate their approval by writing ` + "`/approve`" + ` in a comment
Approvers can cancel approval by writing ` + "`/approve cancel`" + ` in a comment
</details>
<!-- META={"approvers":[]} -->`,
		},
	}

	fr := fakeRepo{
		approvers: map[string]sets.String{
			"a":   sets.NewString("alice"),
			"a/b": sets.NewString("alice", "bob"),
			"c":   sets.NewString("cblecker", "cjwagner"),
		},
		leafApprovers: map[string]sets.String{
			"a":   sets.NewString("alice"),
			"a/b": sets.NewString("bob"),
			"c":   sets.NewString("cblecker", "cjwagner"),
		},
		approverOwners: map[string]string{
			"a/a.go":   "a",
			"a/aa.go":  "a",
			"a/b/b.go": "a/b",
			"c/c.go":   "c",
		},
	}

	for _, test := range tests {
		fghc := newFakeGithubClient(test.hasLabel, test.humanApproved, test.files, test.comments, test.reviews)
		branch := "master"
		if test.branch != "" {
			branch = test.branch
		}

		if err := handle(
			logrus.WithField("plugin", "approve"),
			fghc,
			fr,
			&plugins.Approve{
				Repos:               []string{"org/repo"},
				ImplicitSelfApprove: test.selfApprove,
				IssueRequired:       test.needsIssue,
				LgtmActsAsApprove:   test.lgtmActsAsApprove,
				ReviewActsAsApprove: test.reviewActsAsApprove,
			},
			&state{
				org:       "org",
				repo:      "repo",
				branch:    branch,
				number:    prNumber,
				body:      test.prBody,
				author:    "cjwagner",
				assignees: []github.User{{Login: "spxtr"}},
			},
		); err != nil {
			t.Errorf("[%s] Unexpected error handling event: %v.", test.name, err)
		}

		if test.expectDelete {
			if len(fghc.IssueCommentsDeleted) != 1 {
				t.Errorf(
					"[%s] Expected 1 notification to be deleted but %d notifications were deleted.",
					test.name,
					len(fghc.IssueCommentsDeleted),
				)
			}
		} else {
			if len(fghc.IssueCommentsDeleted) != 0 {
				t.Errorf(
					"[%s] Expected 0 notifications to be deleted but %d notification was deleted.",
					test.name,
					len(fghc.IssueCommentsDeleted),
				)
			}
		}
		if test.expectComment {
			if len(fghc.IssueCommentsAdded) != 1 {
				t.Errorf(
					"[%s] Expected 1 notification to be added but %d notifications were added.",
					test.name,
					len(fghc.IssueCommentsAdded),
				)
			} else if expect, got := fmt.Sprintf("org/repo#%v:", prNumber)+test.expectedComment, fghc.IssueCommentsAdded[0]; test.expectedComment != "" && got != expect {
				t.Errorf(
					"[%s] Expected the created notification to be:\n%s\n\nbut got:\n%s\n\n",
					test.name,
					expect,
					got,
				)
			}
		} else {
			if len(fghc.IssueCommentsAdded) != 0 {
				t.Errorf(
					"[%s] Expected 0 notifications to be added but %d notification was added.",
					test.name,
					len(fghc.IssueCommentsAdded),
				)
			}
		}

		labelAdded := false
		for _, l := range fghc.LabelsAdded {
			if l == fmt.Sprintf("org/repo#%v:approved", prNumber) {
				if labelAdded {
					t.Errorf("[%s] The approved label was applied to a PR that already had it!", test.name)
				}
				labelAdded = true
			}
		}
		if test.hasLabel {
			labelAdded = false
		}
		toggled := labelAdded
		for _, l := range fghc.LabelsRemoved {
			if l == fmt.Sprintf("org/repo#%v:approved", prNumber) {
				if !test.hasLabel {
					t.Errorf("[%s] The approved label was removed from a PR that doesn't have it!", test.name)
				}
				toggled = true
			}
		}
		if test.expectToggle != toggled {
			t.Errorf(
				"[%s] Expected 'approved' label toggled: %t, but got %t.",
				test.name,
				test.expectToggle,
				toggled,
			)
		}
	}
}

// TODO: cache approvers 'GetFilesApprovers' and 'GetCCs' since these are called repeatedly and are
// expensive.
