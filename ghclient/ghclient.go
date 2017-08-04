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

package ghclient

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/golang/glog"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type issueService interface {
	Create(ctx context.Context, owner string, repo string, issue *github.IssueRequest) (*github.Issue, *github.Response, error)
	ListByRepo(ctx context.Context, org, repo string, opt *github.IssueListByRepoOptions) ([]*github.Issue, *github.Response, error)
	ListLabels(ctx context.Context, owner, repo string, opt *github.ListOptions) ([]*github.Label, *github.Response, error)
}

type pullRequestService interface {
	List(ctx context.Context, org, repo string, opt *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error)
}

type repositoryService interface {
	CreateStatus(ctx context.Context, org, repo, ref string, status *github.RepoStatus) (*github.RepoStatus, *github.Response, error)
	GetCombinedStatus(ctx context.Context, org, repo, ref string, opt *github.ListOptions) (*github.CombinedStatus, *github.Response, error)
	ListCollaborators(ctx context.Context, owner, repo string, opt *github.ListOptions) ([]*github.User, *github.Response, error)
}

type usersService interface {
	Get(ctx context.Context, login string) (*github.User, *github.Response, error)
}

// Client is an augmentation of the go-github client that adds retry logic, rate limiting, and pagination
// handling to some of the client functions.
type Client struct {
	issueService issueService
	prService    pullRequestService
	repoService  repositoryService
	userService  usersService

	retries             int
	retryInitialBackoff time.Duration

	tokenReserve int
	dryRun       bool
}

// NewClient makes a new githubutil client that does rate limiting and retries.
func NewClient(token string, dryRun bool) *Client {
	httpClient := &http.Client{
		Transport: &oauth2.Transport{
			Base:   http.DefaultTransport,
			Source: oauth2.ReuseTokenSource(nil, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})),
		},
	}
	client := github.NewClient(httpClient)
	return &Client{
		issueService:        client.Issues,
		prService:           client.PullRequests,
		repoService:         client.Repositories,
		userService:         client.Users,
		retries:             5,
		retryInitialBackoff: time.Second,
		tokenReserve:        50,
		dryRun:              dryRun,
	}
}

func (c *Client) sleepForAttempt(retryCount int) {
	maxDelay := 20 * time.Second
	delay := c.retryInitialBackoff * time.Duration(math.Exp2(float64(retryCount)))
	if delay > maxDelay {
		delay = maxDelay
	}
	time.Sleep(delay)
}

func (c *Client) limitRate(r *github.Rate) {
	if r.Remaining <= c.tokenReserve {
		sleepDuration := time.Until(r.Reset.Time) + (time.Second * 10)
		if sleepDuration > 0 {
			glog.Infof("--Rate Limiting-- Tokens reached minimum reserve %d. Sleeping until reset in %v.\n", c.tokenReserve, sleepDuration)
			time.Sleep(sleepDuration)
		}
	}
}

type retryAbort struct{ error }

func (r *retryAbort) Error() string {
	return fmt.Sprintf("aborting retry loop: %v", r.error)
}

// retry handles rate limiting and retry logic for a github API call.
func (c *Client) retry(action string, call func() (*github.Response, error)) (*github.Response, error) {
	var err error
	var resp *github.Response

	for retryCount := 0; retryCount <= c.retries; retryCount++ {
		if resp, err = call(); err == nil {
			c.limitRate(&resp.Rate)
			return resp, nil
		}
		switch err := err.(type) {
		case *github.RateLimitError:
			c.limitRate(&err.Rate)
		case *github.TwoFactorAuthError:
			return resp, err
		case *retryAbort:
			return resp, err
		}

		if retryCount == c.retries {
			return resp, err
		} else {
			glog.Errorf("error %s: %v. Will retry.\n", action, err)
			c.sleepForAttempt(retryCount)
		}
	}
	return resp, err
}

func (c *Client) depaginate(action string, opts *github.ListOptions, call func() ([]interface{}, *github.Response, error)) ([]interface{}, error) {
	var allItems []interface{}
	wrapper := func() (*github.Response, error) {
		items, resp, err := call()
		if err == nil {
			allItems = append(allItems, items...)
		}
		return resp, err
	}

	opts.Page = 1
	opts.PerPage = 100
	lastPage := 1
	for ; opts.Page <= lastPage; opts.Page++ {
		resp, err := c.retry(action, wrapper)
		if err != nil {
			return allItems, fmt.Errorf("error while depaginating page %d/%d: %v", opts.Page, lastPage, err)
		}
		if resp.LastPage > 0 {
			lastPage = resp.LastPage
		}
	}
	return allItems, nil
}

func (c *Client) CreateIssue(org, repo, title, body string, labels, assignees []string) (*github.Issue, error) {
	glog.Infof("CreateIssue(dry=%t) Title:%q, Labels:%q, Assignees:%q\n", c.dryRun, title, labels, assignees)
	if c.dryRun {
		return nil, nil
	}

	issue := &github.IssueRequest{
		Title: &title,
		Body:  &body,
	}
	if len(labels) > 0 {
		issue.Labels = &labels
	}
	if len(assignees) > 0 {
		issue.Assignees = &assignees
	}

	var result *github.Issue
	_, err := c.retry(
		fmt.Sprintf("creating issue '%s'", title),
		func() (*github.Response, error) {
			var resp *github.Response
			var err error
			result, resp, err = c.issueService.Create(context.Background(), org, repo, issue)
			return resp, err
		},
	)
	return result, err
}

// CreateStatus creates or updates a status context on the indicated reference.
// This function limits rate and does retries if needed.
func (c *Client) CreateStatus(owner, repo, ref string, status *github.RepoStatus) (*github.RepoStatus, error) {
	glog.Infof("CreateStatus(dry=%t) ref:%s: %s:%s", c.dryRun, ref, *status.Context, *status.State)
	if c.dryRun {
		return nil, nil
	}
	var result *github.RepoStatus
	msg := fmt.Sprintf("creating status for ref '%s'", ref)
	_, err := c.retry(msg, func() (*github.Response, error) {
		var resp *github.Response
		var err error
		result, resp, err = c.repoService.CreateStatus(context.Background(), owner, repo, ref, status)
		return resp, err
	})
	return result, err
}

type PRMungeFunc func(*github.PullRequest) error

// ForEachPR iterates over all PRs that fit the specified criteria, calling the munge function on every PR.
// This function limits rate, does retries if needed, and handles pagination.
// If the munge function returns a non-nil error, ForEachPR will return immediately with a non-nil
// error unless continueOnError is true in which case an error will be logged and the remaining PRs will be munged.
func (c *Client) ForEachPR(owner, repo string, opts *github.PullRequestListOptions, continueOnError bool, mungePR PRMungeFunc) error {
	var lastPage int
	// Munge each page as we get it (or in other words, wait until we are ready to munge the next
	// page of issues before geting it). We use depaginate to make the calls, but don't care about
	// the slice it returns since we consume the pages as we go.
	_, err := c.depaginate(
		"processing PRs",
		&opts.ListOptions,
		func() ([]interface{}, *github.Response, error) {
			list, resp, err := c.prService.List(context.Background(), owner, repo, opts)
			if err == nil {
				for _, pr := range list {
					if pr == nil {
						glog.Errorln("Received a nil PR from go-github while listing PRs. Skipping...")
					}
					if mungeErr := mungePR(pr); mungeErr != nil {
						if pr.Number == nil {
							mungeErr = fmt.Errorf("error munging pull request with nil Number field: %v", mungeErr)
						} else {
							mungeErr = fmt.Errorf("error munging pull request #%d: %v", *pr.Number, mungeErr)
						}
						if !continueOnError {
							return nil, resp, &retryAbort{mungeErr}
						}
						glog.Errorf("%v\n", mungeErr)
					}
				}
				if resp.LastPage > 0 {
					lastPage = resp.LastPage
				}
				glog.Infof("ForEachPR processed page %d/%d\n", opts.ListOptions.Page, lastPage)
			}
			return nil, resp, err
		},
	)
	return err
}

func (c *Client) GetCollaborators(org, repo string) ([]*github.User, error) {
	opts := &github.ListOptions{}
	collaborators, err := c.depaginate(
		fmt.Sprintf("getting collaborators for '%s/%s'", org, repo),
		opts,
		func() ([]interface{}, *github.Response, error) {
			page, resp, err := c.repoService.ListCollaborators(context.Background(), org, repo, opts)

			var interfaceList []interface{}
			if err == nil {
				interfaceList = make([]interface{}, 0, len(page))
				for _, user := range page {
					interfaceList = append(interfaceList, user)
				}
			}
			return interfaceList, resp, err
		},
	)

	result := make([]*github.User, 0, len(collaborators))
	for _, user := range collaborators {
		result = append(result, user.(*github.User))
	}
	return result, err
}

// GetCombinedStatus retrieves the CombinedStatus for the specified reference.
// This function limits rate, does retries if needed and handles pagination.
func (c *Client) GetCombinedStatus(owner, repo, ref string) (*github.CombinedStatus, error) {
	var result *github.CombinedStatus
	listOpts := &github.ListOptions{}

	statuses, err := c.depaginate(
		fmt.Sprintf("getting combined status for ref '%s'", ref),
		listOpts,
		func() ([]interface{}, *github.Response, error) {
			combined, resp, err := c.repoService.GetCombinedStatus(
				context.Background(),
				owner,
				repo,
				ref,
				listOpts,
			)
			if result == nil {
				result = combined
			}

			var interfaceList []interface{}
			if err == nil {
				interfaceList = make([]interface{}, 0, len(combined.Statuses))
				for _, status := range combined.Statuses {
					interfaceList = append(interfaceList, status)
				}
			}
			return interfaceList, resp, err
		},
	)

	if result != nil {
		result.Statuses = make([]github.RepoStatus, 0, len(statuses))
		for _, status := range statuses {
			result.Statuses = append(result.Statuses, status.(github.RepoStatus))
		}
	}

	return result, err
}

func (c *Client) GetIssues(org, repo string, opts *github.IssueListByRepoOptions) ([]*github.Issue, error) {
	issues, err := c.depaginate(
		fmt.Sprintf("getting issues from '%s/%s'", org, repo),
		&opts.ListOptions,
		func() ([]interface{}, *github.Response, error) {
			page, resp, err := c.issueService.ListByRepo(context.Background(), org, repo, opts)

			var interfaceList []interface{}
			if err == nil {
				interfaceList = make([]interface{}, 0, len(page))
				for _, issue := range page {
					interfaceList = append(interfaceList, issue)
				}
			}
			return interfaceList, resp, err
		},
	)

	result := make([]*github.Issue, 0, len(issues))
	for _, issue := range issues {
		result = append(result, issue.(*github.Issue))
	}
	return result, err
}

func (c *Client) GetRepoLabels(org, repo string) ([]*github.Label, error) {
	opts := &github.ListOptions{}
	labels, err := c.depaginate(
		fmt.Sprintf("getting valid labels for '%s/%s'", org, repo),
		opts,
		func() ([]interface{}, *github.Response, error) {
			page, resp, err := c.issueService.ListLabels(context.Background(), org, repo, opts)

			var interfaceList []interface{}
			if err == nil {
				interfaceList = make([]interface{}, 0, len(page))
				for _, label := range page {
					interfaceList = append(interfaceList, label)
				}
			}
			return interfaceList, resp, err
		},
	)

	result := make([]*github.Label, 0, len(labels))
	for _, label := range labels {
		result = append(result, label.(*github.Label))
	}
	return result, err
}

func (c *Client) GetUser(login string) (*github.User, error) {
	var result *github.User
	_, err := c.retry(
		fmt.Sprintf("getting user '%s'", login),
		func() (*github.Response, error) {
			var resp *github.Response
			var err error
			result, resp, err = c.userService.Get(context.Background(), login)
			return resp, err
		},
	)
	return result, err
}
