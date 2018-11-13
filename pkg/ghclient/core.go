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

// Package ghclient provides a github client that wraps go-github with retry logic, rate limiting,
// and depagination where necessary.
package ghclient

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/golang/glog"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

// Client is an augmentation of the go-github client that adds retry logic, rate limiting, and pagination
// handling to applicable the client functions.
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

// NewClient makes a new Client with the specified token and dry-run status.
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
		}
		glog.Errorf("error %s: %v. Will retry.\n", action, err)
		c.sleepForAttempt(retryCount)
	}
	return resp, err
}

// depaginate adds depagination on top of the retry and rate limiting logic provided by retry.
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
