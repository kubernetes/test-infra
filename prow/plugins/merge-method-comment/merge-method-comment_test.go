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

package mergemethodcomment

import (
	"fmt"
	"strings"
	"testing"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
)

const fakeBotName = "k8s-bot"

type ghc struct {
	*testing.T
	comments         map[string][]github.IssueComment
	createCommentErr error
	listCommentsErr  error
}

func (c *ghc) CreateComment(org, repo string, num int, comment string) error {
	c.T.Logf("CreateComment: %s/%s#%d: %s", org, repo, num, comment)
	if c.createCommentErr != nil {
		return c.createCommentErr
	}
	if len(c.comments) == 0 {
		c.comments = make(map[string][]github.IssueComment)
	}
	orgRepoNum := fmt.Sprintf("%s/%s#%d", org, repo, num)
	c.comments[orgRepoNum] = append(c.comments[orgRepoNum],
		github.IssueComment{
			Body: comment,
			User: github.User{Login: fakeBotName},
		},
	)
	return nil
}

func (c *ghc) ListIssueComments(org, repo string, num int) ([]github.IssueComment, error) {
	c.T.Logf("ListIssueComments: %s/%s#%d", org, repo, num)
	if c.listCommentsErr != nil {
		return nil, c.listCommentsErr
	}
	return c.comments[fmt.Sprintf("%s/%s#%d", org, repo, num)], nil
}

func (c *ghc) BotUserChecker() (func(candidate string) bool, error) {
	return func(candidate string) bool { return candidate == fakeBotName }, nil
}

func TestHandlePR(t *testing.T) {
	singleCommitPR := github.PullRequest{
		Number: 101,
		Base: github.PullRequestBranch{
			Repo: github.Repo{
				Owner: github.User{
					Login: "kubernetes",
				},
				Name: "kubernetes",
			},
		},
		Commits: 1,
	}
	multipleCommitsPR := github.PullRequest{
		Number: 101,
		Base: github.PullRequestBranch{
			Repo: github.Repo{
				Owner: github.User{
					Login: "kubernetes",
				},
				Name: "kubernetes",
			},
		},
		Commits: 2,
	}

	cases := []struct {
		name               string
		client             *ghc
		event              github.PullRequestEvent
		defaultMergeMethod github.PullRequestMergeType
		squashLabel        string
		mergeLabel         string
		err                error
		comments           []github.IssueComment
	}{
		{
			name:   "single commit",
			client: &ghc{},
			event: github.PullRequestEvent{
				Action:      github.PullRequestActionOpened,
				PullRequest: singleCommitPR,
			},
			squashLabel:        "squash-label",
			defaultMergeMethod: github.MergeSquash,
		},
		{
			name:   "multiple commits, merge by-default, squash label configured",
			client: &ghc{},
			event: github.PullRequestEvent{
				Action:      github.PullRequestActionOpened,
				PullRequest: multipleCommitsPR,
			},
			defaultMergeMethod: github.MergeMerge,
			squashLabel:        "squash-label",
			comments: []github.IssueComment{
				{
					Body: "You can request commits to be squashed using the label: squash-label",
				},
			},
		},
		{
			name:   "multiple commits, merge by-default, squash label not configured",
			client: &ghc{},
			event: github.PullRequestEvent{
				Action:      github.PullRequestActionOpened,
				PullRequest: multipleCommitsPR,
			},
			defaultMergeMethod: github.MergeMerge,
			comments: []github.IssueComment{
				{
					Body: "Commits will be merged, as no squash labels are defined",
				},
			},
		},
		{
			name:   "multiple commits, squash by-default, merge label configured",
			client: &ghc{},
			event: github.PullRequestEvent{
				Action:      github.PullRequestActionOpened,
				PullRequest: multipleCommitsPR,
			},
			defaultMergeMethod: github.MergeSquash,
			mergeLabel:         "merge-label",
			comments: []github.IssueComment{
				{
					Body: "You can request commits to be merged using the label: merge-label",
				},
			},
		},
		{
			name:   "multiple commits, squash by-default, merge label not configured",
			client: &ghc{},
			event: github.PullRequestEvent{
				Action:      github.PullRequestActionOpened,
				PullRequest: multipleCommitsPR,
			},
			defaultMergeMethod: github.MergeSquash,
			comments: []github.IssueComment{
				{
					Body: "Commits will be squashed, as no merge labels are defined",
				},
			},
		},
		{
			name: "do not create comment if already commented",
			client: &ghc{
				comments: map[string][]github.IssueComment{
					"kubernetes/kubernetes#101": {
						{
							User: github.User{Login: "k8s-bot"},
							Body: fmt.Sprintf("This PR has multiple commits, and the default merge method is: %s.\n%s",
								github.MergeMerge,
								"Commits will be merged, as no squash labels are defined"),
						},
					},
				},
			},
			event: github.PullRequestEvent{
				Action:      github.PullRequestActionOpened,
				PullRequest: multipleCommitsPR,
			},
			defaultMergeMethod: github.MergeMerge,
			comments: []github.IssueComment{
				{
					Body: "Commits will be merged, as no squash labels are defined",
				},
			},
		},
		{
			name: "error listing issue comments",
			client: &ghc{
				listCommentsErr: fmt.Errorf("cannot list comments"),
			},
			defaultMergeMethod: github.MergeMerge,
			event: github.PullRequestEvent{
				Action:      github.PullRequestActionOpened,
				PullRequest: multipleCommitsPR,
			},
			err: fmt.Errorf("error listing issue comments: %s", "cannot list comments"),
		},
		{
			name: "error creating comment",
			client: &ghc{
				createCommentErr: fmt.Errorf("cannot create comment"),
			},
			defaultMergeMethod: github.MergeMerge,
			event: github.PullRequestEvent{
				Action:      github.PullRequestActionSynchronize,
				PullRequest: multipleCommitsPR,
			},
			err: fmt.Errorf("cannot create comment"),
		},
		{
			name:   "handle PR sync",
			client: &ghc{},
			event: github.PullRequestEvent{
				Action:      github.PullRequestActionSynchronize,
				PullRequest: multipleCommitsPR,
			},
			defaultMergeMethod: github.MergeMerge,
			comments: []github.IssueComment{
				{
					Body: "Commits will be merged, as no squash labels are defined",
				},
			},
		},
		{
			name:   "handle PR reopen",
			client: &ghc{},
			event: github.PullRequestEvent{
				Action:      github.PullRequestActionReopened,
				PullRequest: multipleCommitsPR,
			},
			defaultMergeMethod: github.MergeMerge,
			comments: []github.IssueComment{
				{
					Body: "Commits will be merged, as no squash labels are defined",
				},
			},
		},
		{
			name:   "handle PR edit",
			client: &ghc{},
			event: github.PullRequestEvent{
				Action:      github.PullRequestActionEdited,
				PullRequest: multipleCommitsPR,
			},
			defaultMergeMethod: github.MergeMerge,
			comments: []github.IssueComment{
				{
					Body: "Commits will be merged, as no squash labels are defined",
				},
			},
		},
		{
			name:   "ignore irrelevant events",
			client: &ghc{},
			event: github.PullRequestEvent{
				Action:      github.PullRequestActionReviewRequested,
				PullRequest: multipleCommitsPR,
			},
			defaultMergeMethod: github.MergeMerge,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.client == nil {
				t.Fatalf("test case can not have nil github client")
			}

			// Set up test logging.
			c.client.T = t

			c.event.Number = 101
			config := config.Tide{
				MergeType: map[string]github.PullRequestMergeType{
					"kubernetes/kubernetes": c.defaultMergeMethod,
				},
				SquashLabel: c.squashLabel,
				MergeLabel:  c.mergeLabel,
			}
			err := handlePR(c.client, config, c.event)

			if err != nil && c.err == nil {
				t.Fatalf("handlePR error: %v", err)
			}

			if err == nil && c.err != nil {
				t.Fatalf("handlePR wanted error %v, got nil", err)
			}

			if got, want := err, c.err; got != nil && got.Error() != want.Error() {
				t.Fatalf("handlePR errors mismatch: got %v, want %v", got, want)
			}

			if c.err != nil {
				return
			}

			if got, want := len(c.client.comments["kubernetes/kubernetes#101"]), len(c.comments); got != want {
				t.Logf("github client comments: got %v; want %v", c.client.comments, c.comments)
				t.Fatalf("issue comments count mismatch: got %d, want %d", got, want)
			}

			for i, comment := range c.comments {
				if !strings.Contains(c.client.comments["kubernetes/kubernetes#101"][i].Body, comment.Body) {
					t.Fatalf("github client comment: got %s, expected it to contain: %s", c.client.comments["kubernetes/kubernetes#101"][i].Body, comment.Body)
				}
			}
		})
	}
}
