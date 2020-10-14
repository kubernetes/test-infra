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

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

// Client can be used to run commands again GitHub API
type Client struct {
	Token     string
	TokenFile string
	Org       string
	Project   string

	githubClient *github.Client
}

const (
	tokenLimit = 50 // We try to stop that far from the API limit
)

// AddFlags parses options for github client
func (client *Client) AddFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&client.Token, "token", "",
		"The OAuth Token to use for requests.")
	cmd.PersistentFlags().StringVar(&client.TokenFile, "token-file", "",
		"The file containing the OAuth Token to use for requests.")
	cmd.PersistentFlags().StringVar(&client.Org, "organization", "",
		"The github organization to scan")
	cmd.PersistentFlags().StringVar(&client.Project, "project", "",
		"The github project to scan")
}

// CheckFlags looks for organization and project flags to configure the client
func (client *Client) CheckFlags() error {
	if client.Org == "" {
		return fmt.Errorf("organization flag must be set")
	}
	client.Org = strings.ToLower(client.Org)

	if client.Project == "" {
		return fmt.Errorf("project flag must be set")
	}
	client.Project = strings.ToLower(client.Project)

	return nil
}

// getGitHubClient create the github client that we use to communicate with github
func (client *Client) getGitHubClient() (*github.Client, error) {
	if client.githubClient != nil {
		return client.githubClient, nil
	}
	token := client.Token
	if len(token) == 0 && len(client.TokenFile) != 0 {
		data, err := ioutil.ReadFile(client.TokenFile)
		if err != nil {
			return nil, err
		}
		token = strings.TrimSpace(string(data))
	}

	if len(token) > 0 {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		tc := oauth2.NewClient(context.Background(), ts)
		client.githubClient = github.NewClient(tc)
	} else {
		client.githubClient = github.NewClient(nil)
	}
	return client.githubClient, nil
}

// limitsCheckAndWait make sure we have not reached the limit or wait
func (client *Client) limitsCheckAndWait() {
	var sleep time.Duration
	githubClient, err := client.getGitHubClient()
	if err != nil {
		glog.Error("Failed to get RateLimits: ", err)
		sleep = time.Minute
	} else {
		limits, _, err := githubClient.RateLimits(context.Background())
		if err != nil {
			glog.Error("Failed to get RateLimits:", err)
			sleep = time.Minute
		}
		if limits != nil && limits.Core != nil && limits.Core.Remaining < tokenLimit {
			sleep = limits.Core.Reset.Sub(time.Now())
			glog.Warning("RateLimits: reached. Sleeping for ", sleep)
		}
	}

	time.Sleep(sleep)
}

// ClientInterface describes what a client should be able to do
type ClientInterface interface {
	RepositoryName() string
	FetchIssues(last time.Time, c chan *github.Issue)
	FetchIssueEvents(issueID int, last *int, c chan *github.IssueEvent)
	FetchIssueComments(issueID int, last time.Time, c chan *github.IssueComment)
	FetchPullComments(issueID int, last time.Time, c chan *github.PullRequestComment)
}

// RepositoryName returns github's repository name in the form of org/project
func (client *Client) RepositoryName() string {
	return fmt.Sprintf("%s/%s", client.Org, client.Project)
}

// FetchIssues from GitHub, until 'latest' time
func (client *Client) FetchIssues(latest time.Time, c chan *github.Issue) {
	opt := &github.IssueListByRepoOptions{Since: latest, Sort: "updated", State: "all", Direction: "asc"}

	githubClient, err := client.getGitHubClient()
	if err != nil {
		close(c)
		glog.Error(err)
		return
	}

	count := 0
	for {
		client.limitsCheckAndWait()

		issues, resp, err := githubClient.Issues.ListByRepo(
			context.Background(),
			client.Org,
			client.Project,
			opt,
		)
		if err != nil {
			close(c)
			glog.Error(err)
			return
		}

		for _, issue := range issues {
			c <- issue
			count++
		}

		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}

	glog.Infof("Fetched %d issues updated issue since %v.", count, latest)
	close(c)
}

// hasID look for a specific id in a list of events
func hasID(events []*github.IssueEvent, id int) bool {
	for _, event := range events {
		if *event.ID == int64(id) {
			return true
		}
	}
	return false
}

// FetchIssueEvents from github and return the full list, until it matches 'latest'
// The entire last page will be included so you can have redundancy.
func (client *Client) FetchIssueEvents(issueID int, latest *int, c chan *github.IssueEvent) {
	opt := &github.ListOptions{PerPage: 100}

	githubClient, err := client.getGitHubClient()
	if err != nil {
		close(c)
		glog.Error(err)
		return
	}

	count := 0
	for {
		client.limitsCheckAndWait()

		events, resp, err := githubClient.Issues.ListIssueEvents(
			context.Background(),
			client.Org,
			client.Project,
			issueID,
			opt,
		)
		if err != nil {
			glog.Errorf("ListIssueEvents failed: %s. Retrying...", err)
			time.Sleep(time.Second)
			continue
		}

		for _, event := range events {
			c <- event
			count++
		}
		if resp.NextPage == 0 || (latest != nil && hasID(events, *latest)) {
			break
		}
		opt.Page = resp.NextPage
	}

	glog.Infof("Fetched %d events.", count)
	close(c)
}

// FetchIssueComments fetches comments associated to given Issue (since latest)
func (client *Client) FetchIssueComments(issueID int, latest time.Time, c chan *github.IssueComment) {
	opt := &github.IssueListCommentsOptions{Since: latest, Sort: "updated", Direction: "asc"}

	githubClient, err := client.getGitHubClient()
	if err != nil {
		close(c)
		glog.Error(err)
		return
	}

	count := 0
	for {
		client.limitsCheckAndWait()

		comments, resp, err := githubClient.Issues.ListComments(
			context.Background(),
			client.Org,
			client.Project,
			issueID,
			opt,
		)
		if err != nil {
			close(c)
			glog.Error(err)
			return
		}

		for _, comment := range comments {
			c <- comment
			count++
		}
		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}

	glog.Infof("Fetched %d issue comments updated since %v for issue #%d.", count, latest, issueID)
	close(c)
}

// FetchPullComments fetches comments associated to given PullRequest (since latest)
func (client *Client) FetchPullComments(issueID int, latest time.Time, c chan *github.PullRequestComment) {
	opt := &github.PullRequestListCommentsOptions{Since: latest, Sort: "updated", Direction: "asc"}

	githubClient, err := client.getGitHubClient()
	if err != nil {
		close(c)
		glog.Error(err)
		return
	}

	count := 0
	for {
		client.limitsCheckAndWait()

		comments, resp, err := githubClient.PullRequests.ListComments(
			context.Background(),
			client.Org,
			client.Project,
			issueID,
			opt,
		)
		if err != nil {
			close(c)
			glog.Error(err)
			return
		}

		for _, comment := range comments {
			c <- comment
			count++
		}
		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}

	glog.Infof("Fetched %d review comments updated since %v for issue #%d.", count, latest, issueID)
	close(c)
}
