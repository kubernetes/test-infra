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

type repositoryService interface {
	CreateStatus(ctx context.Context, org, repo, ref string, status *github.RepoStatus) (*github.RepoStatus, *github.Response, error)
	GetCombinedStatus(ctx context.Context, org, repo, ref string, opt *github.ListOptions) (*github.CombinedStatus, *github.Response, error)
}

type pullRequestService interface {
	List(ctx context.Context, org, repo string, opt *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error)
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

// CreateStatus creates or updates a status context on the indicated reference.
// This function limits rate and does retries if needed.
func (c *Client) CreateStatus(owner, repo string, pr *github.PullRequest, status *github.RepoStatus) (*github.RepoStatus, error) {
	ref := *pr.Head.SHA
	glog.Infof("CreateStatus(dry=%t) %d:%s: %s:%s", c.dryRun, *pr.Number, ref, *status.Context, *status.State)
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

func (c *Client) depaginate(action string, opts *github.ListOptions, call func() ([]interface{}, *github.Response, error)) ([]interface{}, error) {
	var allItems []interface{}
	wrapper := func() (*github.Response, error) {
		fmt.Println("wrapper")
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
