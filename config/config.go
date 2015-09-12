package config

import (
	"time"

	"k8s.io/contrib/github"

	github_api "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

type MungeConfig struct {
	github.GithubConfig
	MinIssueNumber   int
	IssueMungersList []string
	PRMungersList    []string
	Once             bool
	Period           time.Duration

	IssueMungers []IssueMunger
	PRMungers    []PRMunger
}

type IssueMunger interface {
	MungeIssue(config *MungeConfig, issue *github_api.Issue)
	AddFlags(cmd *cobra.Command)
	Name() string
}

type PRMunger interface {
	// Take action on a specific pull request includes:
	//   * The config for mungers
	//   * The PR object
	//   * The issue object for the PR, github stores some things (e.g. labels) in an "issue" object with the same number as the PR
	//   * The commits for the PR
	//   * The events on the PR
	MungePullRequest(config *MungeConfig, pr *github_api.PullRequest, issue *github_api.Issue, commits []github_api.RepositoryCommit, events []github_api.IssueEvent)
	AddFlags(cmd *cobra.Command)
	Name() string
}
