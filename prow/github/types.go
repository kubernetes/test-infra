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
	"strings"
)

// These are possible State entries for a Status.
const (
	StatusPending = "pending"
	StatusSuccess = "success"
	StatusError   = "error"
	StatusFailure = "failure"
)

// Possible contents for reactions.
const (
	ReactionThumbsUp   = "+1"
	ReactionThumbsDown = "-1"
	ReactionLaugh      = "laugh"
	ReactionConfused   = "confused"
	ReactionHeart      = "heart"
	ReactionHooray     = "hooray"
)

type Reaction struct {
	Content string `json:"content"`
}

// Status is used to set a commit status line.
type Status struct {
	State       string `json:"state"`
	TargetURL   string `json:"target_url,omitempty"`
	Description string `json:"description,omitempty"`
	Context     string `json:"context,omitempty"`
}

// CombinedStatus is the latest statuses for a ref.
type CombinedStatus struct {
	Statuses []Status `json:"statuses"`
}

// User is a GitHub user account.
type User struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// PullRequestEventAction enumerates the triggers for this
// webhook payload type. See also:
// https://developer.github.com/v3/activity/events/types/#pullrequestevent
type PullRequestEventAction string

const (
	PullRequestActionAssigned             PullRequestEventAction = "assigned"
	PullRequestActionUnassigned                                  = "unassigned"
	PullRequestActionReviewRequested                             = "review_requested"
	PullRequestActionReviewRequestRemoved                        = "review_request_removed"
	PullRequestActionLabeled                                     = "labeled"
	PullRequestActionUnlabeled                                   = "unlabeled"
	PullRequestActionOpened                                      = "opened"
	PullRequestActionEdited                                      = "edited"
	PullRequestActionClosed                                      = "closed"
	PullRequestActionReopened                                    = "reopened"
	PullRequestActionSynchronize                                 = "synchronize"
)

// PullRequestEvent is what GitHub sends us when a PR is changed.
type PullRequestEvent struct {
	Action      PullRequestEventAction `json:"action"`
	Number      int                    `json:"number"`
	PullRequest PullRequest            `json:"pull_request"`
	Label       Label                  `json:"label"`
}

// PullRequest contains information about a PullRequest.
type PullRequest struct {
	Number             int               `json:"number"`
	HTMLURL            string            `json:"html_url"`
	User               User              `json:"user"`
	Base               PullRequestBranch `json:"base"`
	Head               PullRequestBranch `json:"head"`
	Title              string            `json:"title"`
	Body               string            `json:"body"`
	RequestedReviewers []User            `json:"requested_reviewers"`
	Assignees          []User            `json:"assignees"`
	Merged             bool              `json:"merged"`
	// ref https://developer.github.com/v3/pulls/#get-a-single-pull-request
	// If Merged is true, MergeSHA is the SHA of the merge commit, or squashed commit
	// If Merged is false, MergeSHA is a commit SHA that github created to test if
	// the PR can be merged automatically.
	MergeSHA *string `json:"merge_commit_sha"`
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

// PullRequestChange contains information about what a PR changed.
type PullRequestChange struct {
	SHA       string `json:"sha"`
	Filename  string `json:"filename"`
	Status    string `json:"added"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Changes   int    `json:"changes"`
	Patch     string `json:"patch"`
}

// Repo contains general repository information.
type Repo struct {
	Owner    User   `json:"owner"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
}

// IssueEventAction enumerates the triggers for this
// webhook payload type. See also:
// https://developer.github.com/v3/activity/events/types/#issuesevent
type IssueEventAction string

const (
	IssueActionAssigned     IssueEventAction = "assigned"
	IssueActionUnassigned                    = "unassigned"
	IssueActionLabeled                       = "labeled"
	IssueActionUnlabeled                     = "unlabeled"
	IssueActionOpened                        = "opened"
	IssueActionEdited                        = "edited"
	IssueActionMilestoned                    = "milestoned"
	IssueActionDemilestoned                  = "demilestoned"
	IssueActionClosed                        = "closed"
	IssueActionReopened                      = "reopened"
)

type IssueEvent struct {
	Action IssueEventAction `json:"action"`
	Issue  Issue            `json:"issue"`
	Repo   Repo             `json:"repository"`
}

// IssueCommentEventAction enumerates the triggers for this
// webhook payload type. See also:
// https://developer.github.com/v3/activity/events/types/#issuecommentevent
type IssueCommentEventAction string

const (
	IssueCommentActionCreated IssueCommentEventAction = "created"
	IssueCommentActionEdited                          = "edited"
	IssueCommentActionDeleted                         = "deleted"
)

type IssueCommentEvent struct {
	Action  IssueCommentEventAction `json:"action"`
	Issue   Issue                   `json:"issue"`
	Comment IssueComment            `json:"comment"`
	Repo    Repo                    `json:"repository"`
}

type Issue struct {
	User      User    `json:"user"`
	Number    int     `json:"number"`
	Title     string  `json:"title"`
	State     string  `json:"state"`
	HTMLURL   string  `json:"html_url"`
	Labels    []Label `json:"labels"`
	Assignees []User  `json:"assignees"`
	Body      string  `json:"body"`

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

func (i Issue) HasLabel(labelToFind string) bool {
	for _, label := range i.Labels {
		if strings.ToLower(label.Name) == strings.ToLower(labelToFind) {
			return true
		}
	}
	return false
}

type IssueComment struct {
	ID      int    `json:"id,omitempty"`
	Body    string `json:"body"`
	User    User   `json:"user,omitempty"`
	HTMLURL string `json:"html_url,omitempty"`
}

type StatusEvent struct {
	SHA         string `json:"sha,omitempty"`
	State       string `json:"state,omitempty"`
	Description string `json:"description,omitempty"`
	TargetURL   string `json:"target_url,omitempty"`
	ID          int    `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Context     string `json:"context,omitempty"`
	Sender      User   `json:"sender,omitempty"`
	Repo        Repo   `json:"repository,omitempty"`
}

// IssuesSearchResult represents the result of an issues search.
type IssuesSearchResult struct {
	Total  int     `json:"total_count,omitempty"`
	Issues []Issue `json:"items,omitempty"`
}

type PushEvent struct {
	Ref     string   `json:"ref"`
	Before  string   `json:"before"`
	After   string   `json:"after"`
	Compare string   `json:"compare"`
	Commits []Commit `json:"commits"`
	// Pusher is the user that pushed the commit, valid in a webhook event.
	Pusher User `json:"pusher"`
	// Sender contains more information that Pusher about the user.
	Sender User `json:"sender"`
	Repo   Repo `json:"repository"`
}

func (pe PushEvent) Branch() string {
	refs := strings.Split(pe.Ref, "/")
	return refs[len(refs)-1]
}

type Commit struct {
	ID       string   `json:"id"`
	Message  string   `json:"message"`
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Modified []string `json:"modified"`
}

// ReviewEventAction enumerates the triggers for this
// webhook payload type. See also:
// https://developer.github.com/v3/activity/events/types/#pullrequestreviewevent
type ReviewEventAction string

const (
	ReviewActionSubmitted ReviewEventAction = "submitted"
	ReviewActionEdited                      = "edited"
	ReviewActionDismissed                   = "dismissed"
)

// ReviewEvent is what GitHub sends us when a PR review is changed.
type ReviewEvent struct {
	Action      ReviewEventAction `json:"action"`
	PullRequest PullRequest       `json:"pull_request"`
	Repo        Repo              `json:"repository"`
	Review      Review            `json:"review"`
}

// Review describes a Pull Request review.
type Review struct {
	ID      int    `json:"id"`
	User    User   `json:"user"`
	Body    string `json:"body"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
}

// ReviewCommentEventAction enumerates the triggers for this
// webhook payload type. See also:
// https://developer.github.com/v3/activity/events/types/#pullrequestreviewcommentevent
type ReviewCommentEventAction string

const (
	ReviewCommentActionCreated ReviewCommentEventAction = "created"
	ReviewCommentActionEdited                           = "edited"
	ReviewCommentActionDeleted                          = "deleted"
)

// ReviewCommentEvent is what GitHub sends us when a PR review comment is changed.
type ReviewCommentEvent struct {
	Action      ReviewCommentEventAction `json:"action"`
	PullRequest PullRequest              `json:"pull_request"`
	Repo        Repo                     `json:"repository"`
	Comment     ReviewComment            `json:"comment"`
}

// ReviewComment describes a Pull Request review.
type ReviewComment struct {
	ID       int    `json:"id"`
	ReviewID int    `json:"pull_request_review_id"`
	User     User   `json:"user"`
	Body     string `json:"body"`
	Path     string `json:"path"`
	HTMLURL  string `json:"html_url"`
	// Position will be nil if the code has changed such that the comment is no
	// longer relevant.
	Position *int `json:"position"`
}

// ReviewAction is the action that a review can be made with.
type ReviewAction string

// Possible review actions. Leave Action blank for a pending review.
const (
	Approve        ReviewAction = "APPROVE"
	RequestChanges              = "REQUEST_CHANGES"
	Comment                     = "COMMENT"
)

// DraftReview is what we give GitHub when we want to make a PR Review. This is
// different than what we receive when we ask for a Review.
type DraftReview struct {
	// If unspecified, defaults to the most recent commit in the PR.
	CommitSHA string `json:"commit_id,omitempty"`
	Body      string `json:"body"`
	// If unspecified, defaults to PENDING.
	Action   ReviewAction         `json:"event,omitempty"`
	Comments []DraftReviewComment `json:"comments,omitempty"`
}

// DraftReviewComment is a comment in a draft review.
type DraftReviewComment struct {
	Path string `json:"path"`
	// Position in the patch, not the line number in the file.
	Position int    `json:"position"`
	Body     string `json:"body"`
}

// Content is some base64 encoded github file content
type Content struct {
	Content string `json:"content"`
	SHA     string `json:"sha"`
}
