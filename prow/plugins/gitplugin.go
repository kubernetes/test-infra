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

package plugins

import (
	"k8s.io/test-infra/prow/github"
)

type GithubClient interface {
	// Git Use and Bot management.
	IsMember(org, user string) (bool, error)
	BotName() string

	// Git Issue Comments.
	CreateComment(owner, repo string, number int, comment string) error
	CreateCommentReaction(org, repo string, ID int, reaction string) error
	DeleteComment(org, repo string, ID int) error
	EditComment(org, repo string, ID int, comment string) error

	// Git Issue Labels.
	AddLabel(owner, repo string, number int, label string) error
	GetRepoLabels(owner, repo string) ([]github.Label, error)
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	RemoveLabel(owner, repo string, number int, label string) error

	// Git Issue Management.
	AssignIssue(org, repo string, number int, logins []string) error
	CreateIssueReaction(org, repo string, ID int, reaction string) error
	CloseIssue(org, repo string, number int) error
	FindIssues(query string) ([]github.Issue, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	ReopenIssue(org, repo string, number int) error
	UnassignIssue(org, repo string, number int, logins []string) error

	// Git PRs.
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetPullRequestChanges(pr github.PullRequest) ([]github.PullRequestChange, error)

	//Git Reviews.
	RequestReview(org, repo string, number int, logins []string) error
	UnrequestReview(org, repo string, number int, logins []string) error

	// Git Issue or Commit Status.
	CreateStatus(org, repo, ref string, s github.Status) error
	GetRef(org, repo, ref string) (string, error)
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
}
