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
	"time"
)

const (
	// EventGUID is sent by Github in a header of every webhook request.
	// Used as a log field across prow.
	EventGUID = "event-GUID"
	// PrLogField is the number of a PR.
	// Used as a log field across prow.
	PrLogField = "pr"
	// OrgLogField is the organization of a PR.
	// Used as a log field across prow.
	OrgLogField = "org"
	// RepoLogField is the repository of a PR.
	// Used as a log field across prow.
	RepoLogField = "repo"
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
	ReactionThumbsUp                  = "+1"
	ReactionThumbsDown                = "-1"
	ReactionLaugh                     = "laugh"
	ReactionConfused                  = "confused"
	ReactionHeart                     = "heart"
	ReactionHooray                    = "hooray"
	stateCannotBeChangedMessagePrefix = "state cannot be changed."
)

// PullRequestMergeType enumerates the types of merges the GitHub API can
// perform
// https://developer.github.com/v3/pulls/#merge-a-pull-request-merge-button
type PullRequestMergeType string

// Possible types of merges for the GitHub merge API
const (
	MergeMerge  PullRequestMergeType = "merge"
	MergeRebase PullRequestMergeType = "rebase"
	MergeSquash PullRequestMergeType = "squash"
)

// ClientError represents https://developer.github.com/v3/#client-errors
type ClientError struct {
	Message string `json:"message"`
	Errors  []struct {
		Resource string `json:"resource"`
		Field    string `json:"field"`
		Code     string `json:"code"`
		Message  string `json:"message,omitempty"`
	} `json:"errors,omitempty"`
}

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
	ID    int    `json:"id"`
}

// NormLogin normalizes GitHub login strings
var NormLogin = strings.ToLower

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
	Repo        Repo                   `json:"repository"`
	Label       Label                  `json:"label"`
	Sender      User                   `json:"sender"`

	// GUID is included in the header of the request received by Github.
	GUID string
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
	State              string            `json:"state"`
	Merged             bool              `json:"merged"`
	CreatedAt          time.Time         `json:"created_at,omitempty"`
	UpdatedAt          time.Time         `json:"updated_at,omitempty"`
	// ref https://developer.github.com/v3/pulls/#get-a-single-pull-request
	// If Merged is true, MergeSHA is the SHA of the merge commit, or squashed commit
	// If Merged is false, MergeSHA is a commit SHA that github created to test if
	// the PR can be merged automatically.
	MergeSHA *string `json:"merge_commit_sha"`
	// ref https://developer.github.com/v3/pulls/#response-1
	// The value of the mergeable attribute can be true, false, or null. If the value
	// is null, this means that the mergeability hasn't been computed yet, and a
	// background job was started to compute it. When the job is complete, the response
	// will include a non-null value for the mergeable attribute.
	Mergable *bool `json:"mergeable,omitempty"`
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

// PullRequestFileStatus enumerates the statuses for this webhook payload type.
type PullRequestFileStatus string

const (
	PullRequestFileModified PullRequestFileStatus = "modified"
	PullRequestFileAdded                          = "added"
	PullRequestFileRemoved                        = "removed"
	PullRequestFileRenamed                        = "renamed"
)

// PullRequestChange contains information about what a PR changed.
type PullRequestChange struct {
	SHA       string `json:"sha"`
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Changes   int    `json:"changes"`
	Patch     string `json:"patch"`
	BlobURL   string `json:"blob_url"`
}

// Repo contains general repository information.
type Repo struct {
	Owner    User   `json:"owner"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
	Fork     bool   `json:"fork"`
}

type Branch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

// https://developer.github.com/v3/repos/branches/#update-branch-protection
type BranchProtectionRequest struct {
	RequiredStatusChecks       RequiredStatusChecks        `json:"required_status_checks"`
	EnforceAdmins              bool                        `json:"enforce_admins"`
	RequiredPullRequestReviews *RequiredPullRequestReviews `json:"required_pull_request_reviews"`
	Restrictions               Restrictions                `json:"restrictions"`
}

type RequiredStatusChecks struct {
	Strict   bool     `json:"strict"`
	Contexts []string `json:"contexts"`
}

type RequiredPullRequestReviews struct{}

type Restrictions struct {
	Users []string `json:"users"`
	Teams []string `json:"teams"`
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

// IssueEvent represents an issue event from a webhook payload (not from the events API).
type IssueEvent struct {
	Action IssueEventAction `json:"action"`
	Issue  Issue            `json:"issue"`
	Repo   Repo             `json:"repository"`
	// Label is specified for IssueActionLabeled and IssueActionUnlabeled events.
	Label Label `json:"label"`

	// GUID is included in the header of the request received by Github.
	GUID string
}

// ListedIssueEvent represents an issue event from the events API (not from a webhook payload).
// https://developer.github.com/v3/issues/events/
type ListedIssueEvent struct {
	Event     IssueEventAction `json:"event"` // This is the same as IssueEvent.Action.
	Actor     User             `json:"actor"`
	Label     Label            `json:"label"`
	CreatedAt time.Time        `json:"created_at"`
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

	// GUID is included in the header of the request received by Github.
	GUID string
}

type Issue struct {
	User      User      `json:"user"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	HTMLURL   string    `json:"html_url"`
	Labels    []Label   `json:"labels"`
	Assignees []User    `json:"assignees"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Milestone Milestone `json:"milestone"`

	// This will be non-nil if it is a pull request.
	PullRequest *struct{} `json:"pull_request,omitempty"`
}

func (i Issue) IsAssignee(login string) bool {
	for _, assignee := range i.Assignees {
		if NormLogin(login) == NormLogin(assignee.Login) {
			return true
		}
	}
	return false
}

func (i Issue) IsAuthor(login string) bool {
	return NormLogin(i.User.Login) == NormLogin(login)
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
	ID        int       `json:"id,omitempty"`
	Body      string    `json:"body"`
	User      User      `json:"user,omitempty"`
	HTMLURL   string    `json:"html_url,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
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

	// GUID is included in the header of the request received by Github.
	GUID string
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

	// GUID is included in the header of the request received by Github.
	GUID string
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

	// GUID is included in the header of the request received by Github.
	GUID string
}

// Review describes a Pull Request review.
type Review struct {
	ID          int       `json:"id"`
	User        User      `json:"user"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	HTMLURL     string    `json:"html_url"`
	SubmittedAt time.Time `json:"submitted_at"`
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

	// GUID is included in the header of the request received by Github.
	GUID string
}

// ReviewComment describes a Pull Request review.
type ReviewComment struct {
	ID        int       `json:"id"`
	ReviewID  int       `json:"pull_request_review_id"`
	User      User      `json:"user"`
	Body      string    `json:"body"`
	Path      string    `json:"path"`
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

// Team is a github organizational team
type Team struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// TeamMember is a member of an organizational team
type TeamMember struct {
	Login string `json:"login"`
}

type GenericCommentEventAction string

// Comments indicate values that are coerced to the specified value.
const (
	GenericCommentActionCreated GenericCommentEventAction = "created" // "opened", "submitted"
	GenericCommentActionEdited                            = "edited"
	GenericCommentActionDeleted                           = "deleted" // "dismissed"
)

// GenericCommentEvent is a fake event type that is instantiated for any github event that contains
// comment like content.
// The specific events that are also handled as GenericCommentEvents are:
// - issue_comment events
// - pull_request_review events
// - pull_request_review_comment events
// - pull_request events with action in ["opened", "edited"]
// - issue events with action in ["opened", "edited"]
//
// Issue and PR "closed" events are not coerced to the "deleted" Action and do not trigger
// a GenericCommentEvent because these events don't actually remove the comment content from GH.
type GenericCommentEvent struct {
	IsPR         bool
	Action       GenericCommentEventAction
	Body         string
	HTMLURL      string
	Number       int
	Repo         Repo
	User         User
	IssueAuthor  User
	Assignees    []User
	IssueState   string
	IssueBody    string
	IssueHTMLURL string
}

// Milestone is a milestone defined on a github repository
type Milestone struct {
	Title  string `json:"title"`
	Number int    `json:"number"`
}
