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

type Issues interface {
	CreateComment(ctx context.Context, owner string, repo string, number int,
		comment *github.IssueComment) (*github.IssueComment, *github.Response, error)
	DeleteComment(ctx context.Context, owner string, repo string, commentID int) (
		*github.Response, error)
	ListComments(ctx context.Context, owner string, repo string, number int,
		opt *github.IssueListCommentsOptions) ([]*github.IssueComment, *github.Response, error)
}

type PullRequests interface {
	ListFiles(ctx context.Context, owner string, repo string, number int, opt *github.ListOptions) (
		[]*github.CommitFile, *github.Response, error)
}
