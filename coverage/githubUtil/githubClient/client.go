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

package githubClient

import (
	"context"

	"github.com/google/go-github/github"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

//GithubClient stores all github client objects used by code coverage tool
type GithubClient struct {
	Issues       Issues
	PullRequests PullRequests
}

//New constructs GithubClient
func New(issues Issues, pullRequests PullRequests) *GithubClient {
	return &GithubClient{issues, pullRequests}
}

// Make makes & gets a github client
func Make(ctx context.Context, githubToken string) *GithubClient {
	if len(githubToken) == 0 {
		logrus.Info("Warning: Github token empty")
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	return New(client.Issues, client.PullRequests)
}

//Issues collects operations on github issues and allows fake implementation to happen
type Issues interface {
	CreateComment(ctx context.Context, owner string, repo string, number int,
		comment *github.IssueComment) (*github.IssueComment, *github.Response, error)
	DeleteComment(ctx context.Context, owner string, repo string, commentID int) (
		*github.Response, error)
	ListComments(ctx context.Context, owner string, repo string, number int,
		opt *github.IssueListCommentsOptions) ([]*github.IssueComment, *github.Response, error)
}

//PullRequests collects methods on github pull requests and allows fake implementation
type PullRequests interface {
	ListFiles(ctx context.Context, owner string, repo string, number int, opt *github.ListOptions) (
		[]*github.CommitFile, *github.Response, error)
}
