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

// Package github wraps go-github for ease of use and testing.
package github

import (
	"log"
	"net/http"

	"github.com/google/go-github/github"
)

// These are possible State entries for a Status.
const (
	Pending = "pending"
	Success = "success"
	Error   = "error"
	Failure = "failure"
)

// Status is used to set a commit status line.
type Status struct {
	State       string
	TargetURL   string
	Description string
	Context     string
}

// User is a GitHub user account.
type User struct {
	Login string `json:"login"`
}

// PullRequestEvent is what GitHub sends us when a PR is changed.
type PullRequestEvent struct {
	Action      string      `json:"action"`
	Number      int         `json:"number"`
	PullRequest PullRequest `json:"pull_request"`
}

// PullRequest contains information about a PullRequest.
type PullRequest struct {
	Number int               `json:"number"`
	User   User              `json:"user"`
	Base   PullRequestBranch `json:"base"`
	Head   PullRequestBranch `json:"head"`
}

// PullRequestBranch contains information about a particular branch in a PR.
type PullRequestBranch struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
	Repo Repo   `json:"repo"`
}

// Repo contains general repository information.
type Repo struct {
	Owner   User   `json:"owner"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

func logRateLimit(desc string, resp *github.Response) {
	log.Printf("GitHub API Tokens: %d/%d (resets at %v) (%s)", resp.Remaining, resp.Limit, resp.Reset, desc)
}

// TODO: Be aware of rate limits.
type Client struct {
	cl  *github.Client
	dry bool
}

// NewClient creates a new fully operational GitHub client.
func NewClient(httpClient *http.Client) *Client {
	return &Client{
		cl:  github.NewClient(httpClient),
		dry: false,
	}
}

// NewDryRunClient creates a new client that will not perform mutating actions
// such as setting statuses or commenting, but it will still query GitHub and
// use up API tokens.
func NewDryRunClient(httpClient *http.Client) *Client {
	return &Client{
		cl:  github.NewClient(httpClient),
		dry: true,
	}
}

// IsMember returns whether or not the user is a member of the org.
func (c *Client) IsMember(org, user string) (bool, error) {
	member, resp, err := c.cl.Organizations.IsMember(org, user)
	if err != nil {
		return false, err
	}
	logRateLimit("IsMember", resp)
	return member, nil
}

// IsTeamMember returns whether or not the user is a member of the team.
func (c *Client) IsTeamMember(team int, user string) (bool, error) {
	member, resp, err := c.cl.Organizations.IsTeamMember(team, user)
	if err != nil {
		return false, err
	}
	logRateLimit("IsTeamMember", resp)
	return member, nil
}

// CreateStatus creates or updates the status of a commit.
func (c *Client) CreateStatus(owner, repo, ref string, s Status) error {
	if c.dry {
		return nil
	}
	_, resp, err := c.cl.Repositories.CreateStatus(owner, repo, ref, &github.RepoStatus{
		State:       github.String(s.State),
		TargetURL:   github.String(s.TargetURL),
		Description: github.String(s.Description),
		Context:     github.String(s.Context),
	})
	if err != nil {
		return err
	}
	logRateLimit("CreateStatus", resp)
	return nil
}

// ValidatePayload ensures that the request payload signature matches the key.
func ValidatePayload(r *http.Request, secretKey []byte) (payload []byte, err error) {
	return github.ValidatePayload(r, secretKey)
}
