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

package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

type Client struct {
	client *http.Client
	token  string
	base   string
	dry    bool
}

const githubBase = "https://api.github.com"

// NewClient creates a new fully operational GitHub client.
func NewClient(token string) *Client {
	return &Client{
		client: &http.Client{},
		token:  token,
		base:   githubBase,
		dry:    false,
	}
}

// NewDryRunClient creates a new client that will not perform mutating actions
// such as setting statuses or commenting, but it will still query GitHub and
// use up API tokens.
func NewDryRunClient(token string) *Client {
	return &Client{
		client: &http.Client{},
		token:  token,
		base:   githubBase,
		dry:    true,
	}
}

func (c *Client) request(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, fmt.Sprintf("%s/%s", c.base, path), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+c.token)
	req.Header.Add("Accept", "application/vnd.github.v3+json")
	// Disable keep-alive so that we don't get flakes when GitHub closes the
	// connection prematurely.
	// https://go-review.googlesource.com/#/c/3210/ fixed it for GET, but not
	// for POST.
	req.Close = true
	return req, nil
}

// IsMember returns whether or not the user is a member of the org.
func (c *Client) IsMember(org, user string) (bool, error) {
	req, err := c.request(http.MethodGet, fmt.Sprintf("orgs/%s/members/%s", org, user), nil)
	if err != nil {
		return false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 {
		return true, nil
	} else if resp.StatusCode == 404 {
		return false, nil
	} else if resp.StatusCode == 302 {
		return false, fmt.Errorf("github user is not %s org member", org)
	}
	return false, fmt.Errorf("unexpected status: %s", resp.Status)
}

// CreateComment creates a comment on the issue.
func (c *Client) CreateComment(owner, repo string, number int, comment string) error {
	if c.dry {
		return nil
	}

	ic := IssueComment{
		Body: comment,
	}
	b, err := json.Marshal(ic)
	if err != nil {
		return err
	}
	req, err := c.request(http.MethodPost, fmt.Sprintf("repos/%s/%s/issues/%d/comments", owner, repo, number), bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return fmt.Errorf("response not 201: %s", resp.Status)
	}
	return nil
}

// DeleteComment deletes the comment.
func (c *Client) DeleteComment(owner, repo string, ID int) error {
	if c.dry {
		return nil
	}

	req, err := c.request(http.MethodDelete, fmt.Sprintf("repos/%s/%s/issues/comments/%d", owner, repo, ID), nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		return fmt.Errorf("response not 204: %s", resp.Status)
	}
	return nil
}

// ListIssueComments returns all comments on an issue. This may use more than
// one API token.
func (c *Client) ListIssueComments(owner, repo string, number int) ([]IssueComment, error) {
	nextURL := fmt.Sprintf("repos/%s/%s/issues/%d/comments?per_page=100", owner, repo, number)
	var comments []IssueComment
	for nextURL != "" {
		req, err := c.request(http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("return code not 2XX: %s", resp.Status)
		}

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		var ics []IssueComment
		if err := json.Unmarshal(b, &ics); err != nil {
			return nil, err
		}
		comments = append(comments, ics...)
		nextURL = parseLinks(resp.Header.Get("Link"))["next"]
	}
	return comments, nil
}

// GetPullRequest gets a pull request.
func (c *Client) GetPullRequest(owner, repo string, number int) (*PullRequest, error) {
	req, err := c.request(http.MethodGet, fmt.Sprintf("repos/%s/%s/pulls/%d", owner, repo, number), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("response not 200: %s", resp.Status)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var pr PullRequest
	if err := json.Unmarshal(b, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// CreateStatus creates or updates the status of a commit.
func (c *Client) CreateStatus(owner, repo, ref string, s Status) error {
	if c.dry {
		return nil
	}

	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	req, err := c.request(http.MethodPost, fmt.Sprintf("repos/%s/%s/statuses/%s", owner, repo, ref), bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return fmt.Errorf("response not 201: %s", resp.Status)
	}
	return nil
}
