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

package githubutil

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/golang/glog"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type repositoryService interface {
	CreateStatus(org, repo, ref string, status *github.RepoStatus) (*github.RepoStatus, *github.Response, error)
	GetCombinedStatus(org, repo, ref string, opt *github.ListOptions) (*github.CombinedStatus, *github.Response, error)
}

type pullRequestService interface {
	List(org, repo string, opt *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error)
}

// Client is an augmentation of the go-github client that adds retry logic, rate limiting, and pagination
// handling to some of the client functions.
type Client struct {
	prService   pullRequestService
	repoService repositoryService

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
		prService:           client.PullRequests,
		repoService:         client.Repositories,
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

// retry handles rate limiting and retry logic for a github API call.
func (c *Client) retry(action string, call func() (*github.Response, error)) error {
	var err error

	for retryCount := 0; retryCount <= c.retries; retryCount++ {
		var resp *github.Response
		if resp, err = call(); err == nil {
			c.limitRate(&resp.Rate)
			return nil
		} else if rlErr, ok := err.(*github.RateLimitError); ok {
			c.limitRate(&rlErr.Rate)
		} else if tfaErr, ok := err.(*github.TwoFactorAuthError); ok {
			return tfaErr
		}

		if retryCount == c.retries {
			return err
		} else {
			glog.Errorf("error %s: %v. Will retry.\n", action, err)
			c.sleepForAttempt(retryCount)
		}
	}
	return err
}

// CreateStatus creates or updates a status context on the indicated reference.
// This function limits rate and does retries if needed.
func (c *Client) CreateStatus(owner, repo string, pr *github.PullRequest, status *github.RepoStatus) (*github.RepoStatus, *github.Response, error) {
	ref := *pr.Head.SHA
	glog.Infof("CreateStatus(dry=%t) %d:%s: %s:%s", c.dryRun, *pr.Number, ref, *status.Context, *status.State)
	if c.dryRun {
		return nil, nil, nil
	}
	var result *github.RepoStatus
	var resp *github.Response
	msg := fmt.Sprintf("creating status for ref '%s'", ref)
	err := c.retry(msg, func() (*github.Response, error) {
		var err error
		result, resp, err = c.repoService.CreateStatus(owner, repo, ref, status)
		return resp, err
	})
	return result, resp, err
}

// GetCombinedStatus retrieves the CominedStatus for the specified reference.
// This function limits rate, does retries if needed and handles pagination.
func (c *Client) GetCombinedStatus(owner, repo, ref string) (*github.CombinedStatus, *github.Response, error) {
	var status, result *github.CombinedStatus
	var resp *github.Response

	listOpts := &github.ListOptions{Page: 1, PerPage: 100}
	lastPage := 1
	action := fmt.Sprintf("getting combined status for ref '%s'", ref)

	for ; listOpts.Page <= lastPage; listOpts.Page++ {
		err := c.retry(action, func() (*github.Response, error) {
			var err error
			status, resp, err = c.repoService.GetCombinedStatus(owner, repo, ref, listOpts)
			return resp, err
		})
		if err != nil {
			return result, resp, err
		}
		if resp.LastPage > 0 {
			lastPage = resp.LastPage
		}
		if result == nil {
			result = status
		} else {
			result.Statuses = append(result.Statuses, status.Statuses...)
		}
	}
	return result, resp, nil
}

type PRMungeFunc func(*github.PullRequest) error

// ForEachPR iterates over all PRs that fit the specified criteria, calling the munge function on every PR.
// This function limits rate, does retries if needed, and handles pagination.
// If the munge function returns a non-nil error, ForEachPR will return immediately with a non-nil
// error unless continueOnError is true in which case an error will be logged and the remaining PRs will be munged.
func (c *Client) ForEachPR(owner, repo string, opts *github.PullRequestListOptions, continueOnError bool, mungePR PRMungeFunc) error {
	var list []*github.PullRequest
	var resp *github.Response

	opts.ListOptions.Page = 1
	opts.ListOptions.PerPage = 100
	lastPage := 1

	for ; opts.ListOptions.Page <= lastPage; opts.ListOptions.Page++ {
		err := c.retry("processing PRs", func() (*github.Response, error) {
			var err error
			list, resp, err = c.prService.List(owner, repo, opts)
			return resp, err
		})
		if err != nil {
			return err
		}
		if resp.LastPage > 0 {
			lastPage = resp.LastPage
		}
		for _, pr := range list {
			if err := mungePR(pr); err != nil {
				if pr == nil {
					err = fmt.Errorf("received a nil PR from go-github while listing PRs. Munge error: %v", err)
				} else if pr.Number == nil {
					err = fmt.Errorf("error munging pull request with nil Number field: %v", err)
				} else {
					err = fmt.Errorf("error munging pull request #%d: %v", *pr.Number, err)
				}
				if !continueOnError {
					return err
				}
				glog.Errorf("%v\n", err)
			}
		}
		glog.Infof("ForEachPR processed page %d/%d\n", opts.ListOptions.Page, lastPage)
	}
	return nil
}
