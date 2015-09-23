/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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
