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

// These are possible State entries for a Status.
const (
	StatusPending = "pending"
	StatusSuccess = "success"
	StatusError   = "error"
	StatusFailure = "failure"
)

// Status is used to set a commit status line.
type Status struct {
	State       string `json:"state"`
	TargetURL   string `json:"target_url,omitempty"`
	Description string `json:"description,omitempty"`
	Context     string `json:"context,omitempty"`
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
	Label       Label       `json:"label"`
}

// PullRequest contains information about a PullRequest.
type PullRequest struct {
	Number  int               `json:"number"`
	HTMLURL string            `json:"html_url"`
	User    User              `json:"user"`
	Base    PullRequestBranch `json:"base"`
	Head    PullRequestBranch `json:"head"`
}

// PullRequestBranch contains information about a particular branch in a PR.
type PullRequestBranch struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
	Repo Repo   `json:"repo"`
}

type Label struct {
	URL   string `json:"url"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Repo contains general repository information.
type Repo struct {
	Owner    User   `json:"owner"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
}

type IssueCommentEvent struct {
	Action  string       `json:"action"`
	Issue   Issue        `json:"issue"`
	Comment IssueComment `json:"comment"`
	Repo    Repo         `json:"repository"`
}

type Issue struct {
	User      User    `json:"user"`
	Number    int     `json:"number"`
	Title     string  `json:"title"`
	State     string  `json:"state"`
	HTMLURL   string  `json:"html_url"`
	Labels    []Label `json:"labels"`
	Assignees []User  `json:"assignees"`

	// This will be non-nil if it is a pull request.
	PullRequest *struct{} `json:"pull_request,omitempty"`
}

func (i Issue) IsAssignee(login string) bool {
	for _, assignee := range i.Assignees {
		if login == assignee.Login {
			return true
		}
	}
	return false
}

func (i Issue) IsAuthor(login string) bool {
	return i.User.Login == login
}

func (i Issue) IsPullRequest() bool {
	return i.PullRequest != nil
}

type IssueComment struct {
	ID      int    `json:"id,omitempty"`
	Body    string `json:"body"`
	User    User   `json:"user,omitempty"`
	HTMLURL string `json:"html_url,omitempty"`
}

type StatusEvent struct {
	SHA         *string `json:"sha,omitempty"`
	State       *string `json:"state,omitempty"`
	Description *string `json:"description,omitempty"`
	TargetURL   *string `json:"target_url,omitempty"`
	ID          *int    `json:"id,omitempty"`
	Name        *string `json:"name,omitempty"`
	Context     *string `json:"context,omitempty"`
	Sender      *User   `json:"sender,omitempty"`
	Repo        *Repo   `json:"repository,omitempty"`
}

// IssuesSearchResult represents the result of an issues search.
type IssuesSearchResult struct {
	Total  *int    `json:"total_count,omitempty"`
	Issues []Issue `json:"items,omitempty"`
}
