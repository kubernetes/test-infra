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
	"strconv"
	"strings"
	"time"
)

type Logger interface {
	Printf(s string, v ...interface{})
}

type Client struct {
	// If Logger is non-nil, log all method calls with it.
	Logger Logger

	client  *http.Client
	botName string
	token   string
	base    string
	dry     bool
	fake    bool
}

const (
	githubBase   = "https://api.github.com"
	maxRetries   = 8
	maxSleepTime = 2 * time.Minute
	initialDelay = 2 * time.Second
)

// NewClient creates a new fully operational GitHub client.
func NewClient(botName, token string) *Client {
	return &Client{
		client:  &http.Client{},
		botName: botName,
		token:   token,
		base:    githubBase,
		dry:     false,
	}
}

// NewDryRunClient creates a new client that will not perform mutating actions
// such as setting statuses or commenting, but it will still query GitHub and
// use up API tokens.
func NewDryRunClient(botName, token string) *Client {
	return &Client{
		client:  &http.Client{},
		botName: botName,
		token:   token,
		base:    githubBase,
		dry:     true,
	}
}

// NewFakeClient creates a new client that will not perform any actions at all.
func NewFakeClient(botName string) *Client {
	return &Client{
		botName: botName,
		fake:    true,
		dry:     true,
	}
}

func (c *Client) log(methodName string, args ...interface{}) {
	if c.Logger == nil {
		return
	}
	var as []string
	for _, arg := range args {
		as = append(as, fmt.Sprintf("%v", arg))
	}
	c.Logger.Printf("%s(%s)", methodName, strings.Join(as, ", "))
}

var timeSleep = time.Sleep

// Retry on transport failures. Retries on 500s and retries after sleep on
// ratelimit exceeded.
func (c *Client) request(method, path string, body interface{}) (*http.Response, error) {
	var resp *http.Response
	var err error
	backoff := initialDelay
	for retries := 0; retries < maxRetries; retries++ {
		resp, err = c.doRequest(method, path, body)
		if err == nil {
			// If we are out of API tokens, sleep first. The X-RateLimit-Reset
			// header tells us the time at which we can request again.
			if resp.StatusCode == 403 && resp.Header.Get("X-RateLimit-Remaining") == "0" {
				resp.Body.Close()
				var t int
				if t, err = strconv.Atoi(resp.Header.Get("X-RateLimit-Reset")); err == nil {
					// Sleep an extra second plus how long GitHub wants us to
					// sleep. If it's going to take too long, then break.
					sleepTime := time.Unix(int64(t), 0).Sub(time.Now()) + time.Second
					if sleepTime > 0 && sleepTime < maxSleepTime {
						timeSleep(sleepTime)
					} else {
						break
					}
				}
			} else if resp.StatusCode < 500 {
				// Normal, happy case.
				break
			} else {
				// Retry 500 after a break.
				resp.Body.Close()
				timeSleep(backoff)
				backoff *= 2
			}
		} else {
			timeSleep(backoff)
			backoff *= 2
		}
	}
	return resp, err
}

func (c *Client) doRequest(method, path string, body interface{}) (*http.Response, error) {
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewBuffer(b)
	}
	req, err := http.NewRequest(method, path, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+c.token)
	if strings.HasSuffix(path, "reactions") {
		req.Header.Add("Accept", "application/vnd.github.squirrel-girl-preview")
	} else {
		req.Header.Add("Accept", "application/vnd.github.v3+json")
	}
	// Disable keep-alive so that we don't get flakes when GitHub closes the
	// connection prematurely.
	// https://go-review.googlesource.com/#/c/3210/ fixed it for GET, but not
	// for POST.
	req.Close = true
	return c.client.Do(req)
}

func (c *Client) BotName() string {
	return c.botName
}

// IsMember returns whether or not the user is a member of the org.
func (c *Client) IsMember(org, user string) (bool, error) {
	c.log("IsMember", org, user)
	if c.fake {
		return true, nil
	}
	resp, err := c.request(http.MethodGet, fmt.Sprintf("%s/orgs/%s/members/%s", c.base, org, user), nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 {
		return true, nil
	} else if resp.StatusCode == 404 {
		return false, nil
	} else if resp.StatusCode == 302 {
		return false, fmt.Errorf("requester is not %s org member", org)
	}
	return false, fmt.Errorf("unexpected status: %s", resp.Status)
}

// CreateComment creates a comment on the issue.
func (c *Client) CreateComment(org, repo string, number int, comment string) error {
	c.log("CreateComment", org, repo, number, comment)
	if c.dry {
		return nil
	}

	ic := IssueComment{
		Body: comment,
	}
	resp, err := c.request(http.MethodPost, fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.base, org, repo, number), ic)
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
func (c *Client) DeleteComment(org, repo string, ID int) error {
	c.log("DeleteComment", org, repo, ID)
	if c.dry {
		return nil
	}

	resp, err := c.request(http.MethodDelete, fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d", c.base, org, repo, ID), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		return fmt.Errorf("response not 204: %s", resp.Status)
	}
	return nil
}

func (c *Client) CreateCommentReaction(org, repo string, ID int, reaction string) error {
	c.log("CreateCommentReaction", org, repo, ID, reaction)
	if c.dry {
		return nil
	}
	r := Reaction{Content: reaction}
	resp, err := c.request(http.MethodPost, fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d/reactions", c.base, org, repo, ID), r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return fmt.Errorf("response not 201: %s", resp.Status)
	}
	return nil
}

func (c *Client) CreateIssueReaction(org, repo string, ID int, reaction string) error {
	c.log("CreateIssueReaction", org, repo, ID, reaction)
	if c.dry {
		return nil
	}
	r := Reaction{Content: reaction}
	resp, err := c.request(http.MethodPost, fmt.Sprintf("%s/repos/%s/%s/issues/%d/reactions", c.base, org, repo, ID), r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("response not 200 or 201: %s", resp.Status)
	}
	return nil
}

// ListIssueComments returns all comments on an issue. This may use more than
// one API token.
func (c *Client) ListIssueComments(org, repo string, number int) ([]IssueComment, error) {
	c.log("ListIssueComments", org, repo, number)
	if c.fake {
		return nil, nil
	}
	nextURL := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments?per_page=100", c.base, org, repo, number)
	var comments []IssueComment
	for nextURL != "" {
		resp, err := c.request(http.MethodGet, nextURL, nil)
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
func (c *Client) GetPullRequest(org, repo string, number int) (*PullRequest, error) {
	c.log("GetPullRequest", org, repo, number)
	if c.fake {
		return &PullRequest{}, nil
	}
	resp, err := c.request(http.MethodGet, fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.base, org, repo, number), nil)
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

// GetPullRequestChanges gets a list of files modified in a pull request.
func (c *Client) GetPullRequestChanges(pr PullRequest) ([]PullRequestChange, error) {
	c.log("GetPullRequestChanges", pr.Number)
	if c.fake {
		return []PullRequestChange{}, nil
	}
	nextURL := fmt.Sprintf("%s/repos/%s/pulls/%d/files", c.base, pr.Base.Repo.FullName, pr.Number)
	var changes []PullRequestChange
	for nextURL != "" {
		resp, err := c.request(http.MethodGet, nextURL, nil)
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

		var newChanges []PullRequestChange
		if err := json.Unmarshal(b, &newChanges); err != nil {
			return nil, err
		}
		changes = append(changes, newChanges...)
		nextURL = parseLinks(resp.Header.Get("Link"))["next"]
	}
	return changes, nil
}

// CreateStatus creates or updates the status of a commit.
func (c *Client) CreateStatus(org, repo, ref string, s Status) error {
	c.log("CreateStatus", org, repo, ref, s)
	if c.dry {
		return nil
	}

	resp, err := c.request(http.MethodPost, fmt.Sprintf("%s/repos/%s/%s/statuses/%s", c.base, org, repo, ref), s)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return fmt.Errorf("response not 201: %s", resp.Status)
	}
	return nil
}

func (c *Client) AddLabel(org, repo string, number int, label string) error {
	c.log("AddLabel", org, repo, number, label)
	if c.dry {
		return nil
	}
	resp, err := c.request(http.MethodPost, fmt.Sprintf("%s/repos/%s/%s/issues/%d/labels", c.base, org, repo, number), []string{label})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("response not 200: %s", resp.Status)
	}
	return nil
}

func (c *Client) RemoveLabel(org, repo string, number int, label string) error {
	c.log("RemoveLabel", org, repo, number, label)
	if c.dry {
		return nil
	}
	resp, err := c.request(http.MethodDelete, fmt.Sprintf("%s/repos/%s/%s/issues/%d/labels/%s", c.base, org, repo, number, label), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// GitHub sometimes returns 200 for this call, which is a bug on their end.
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		return fmt.Errorf("response not 204: %s", resp.Status)
	}
	return nil
}

func (c *Client) CloseIssue(org, repo string, number int) error {
	c.log("CloseIssue", org, repo, number)
	if c.dry {
		return nil
	}
	resp, err := c.request(http.MethodPatch, fmt.Sprintf("%s/repos/%s/%s/issues/%d", c.base, org, repo, number), map[string]string{"state": "closed"})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("response not 200: %s", resp.Status)
	}
	return nil
}

// GetRef returns the SHA of the given ref, such as "heads/master".
func (c *Client) GetRef(org, repo, ref string) (string, error) {
	c.log("GetRef", org, repo, ref)
	if c.fake {
		return "", nil
	}
	resp, err := c.request(http.MethodGet, fmt.Sprintf("%s/repos/%s/%s/git/refs/%s", c.base, org, repo, ref), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("response not 200: %s", resp.Status)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var res struct {
		Object map[string]string `json:"object"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return "", err
	}
	return res.Object["sha"], nil
}

// FindIssues uses the github search API to find issues which match a particular query.
// TODO(foxish): we should accept map[string][]string and use net/url properly.
func (c *Client) FindIssues(query string) ([]Issue, error) {
	c.log("FindIssues", query)
	if c.fake {
		return nil, nil
	}
	resp, err := c.request(http.MethodGet, fmt.Sprintf("%s/search/issues?q=%s", c.base, query), nil)
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
	var issSearchResult IssuesSearchResult
	if err := json.Unmarshal(b, &issSearchResult); err != nil {
		return nil, err
	}
	return issSearchResult.Issues, nil
}
