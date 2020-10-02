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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

const (
	// EventGUID is sent by GitHub in a header of every webhook request.
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

	// SearchTimeFormat is a time.Time format string for ISO8601 which is the
	// format that GitHub requires for times specified as part of a search query.
	SearchTimeFormat = "2006-01-02T15:04:05Z"

	// DefaultAPIEndpoint is the default GitHub API endpoint.
	DefaultAPIEndpoint = "https://api.github.com"

	// DefaultHost is the default GitHub base endpoint.
	DefaultHost = "github.com"

	// DefaultGraphQLEndpoint is the default GitHub GraphQL API endpoint.
	DefaultGraphQLEndpoint = "https://api.github.com/graphql"
)

var (
	// FoundingYear is the year GitHub was founded. This is just used so that
	// we can lower bound dates related to PRs and issues.
	FoundingYear, _ = time.Parse(SearchTimeFormat, "2007-01-01T00:00:00Z")
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

func unmarshalClientError(b []byte) error {
	var errors []error
	clientError := ClientError{}
	err := json.Unmarshal(b, &clientError)
	if err == nil {
		return clientError
	}
	errors = append(errors, err)
	alternativeClientError := AlternativeClientError{}
	err = json.Unmarshal(b, &alternativeClientError)
	if err == nil {
		return alternativeClientError
	}
	errors = append(errors, err)
	return utilerrors.NewAggregate(errors)
}

// ClientError represents https://developer.github.com/v3/#client-errors
type ClientError struct {
	Message string                `json:"message"`
	Errors  []clientErrorSubError `json:"errors,omitempty"`
}

type clientErrorSubError struct {
	Resource string `json:"resource"`
	Field    string `json:"field"`
	Code     string `json:"code"`
	Message  string `json:"message,omitempty"`
}

func (r ClientError) Error() string {
	return r.Message
}

// AlternativeClientError represents an alternative format for https://developer.github.com/v3/#client-errors
// This is probably a GitHub bug, as documentation_url should appear only in custom errors
type AlternativeClientError struct {
	Message          string   `json:"message"`
	Errors           []string `json:"errors,omitempty"`
	DocumentationURL string   `json:"documentation_url,omitempty"`
}

func (r AlternativeClientError) Error() string {
	return r.Message
}

// Reaction holds the type of emotional reaction.
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
	SHA      string   `json:"sha"`
	Statuses []Status `json:"statuses"`
	State    string   `json:"state"`
}

// User is a GitHub user account.
type User struct {
	Login       string          `json:"login"`
	Name        string          `json:"name"`
	Email       string          `json:"email"`
	ID          int             `json:"id"`
	HTMLURL     string          `json:"html_url"`
	Permissions RepoPermissions `json:"permissions"`
	Type        string          `json:"type"`
}

const (
	// UserTypeUser identifies an actual user account in the User.Type field
	UserTypeUser = "User"
	// UserTypeBot identifies a github app bot user in the User.Type field
	UserTypeBot = "Bot"
)

// NormLogin normalizes GitHub login strings
func NormLogin(login string) string {
	return strings.TrimPrefix(strings.ToLower(login), "@")
}

// PullRequestEventAction enumerates the triggers for this
// webhook payload type. See also:
// https://developer.github.com/v3/activity/events/types/#pullrequestevent
type PullRequestEventAction string

const (
	// PullRequestActionAssigned means assignees were added.
	PullRequestActionAssigned PullRequestEventAction = "assigned"
	// PullRequestActionUnassigned means assignees were removed.
	PullRequestActionUnassigned PullRequestEventAction = "unassigned"
	// PullRequestActionReviewRequested means review requests were added.
	PullRequestActionReviewRequested PullRequestEventAction = "review_requested"
	// PullRequestActionReviewRequestRemoved means review requests were removed.
	PullRequestActionReviewRequestRemoved PullRequestEventAction = "review_request_removed"
	// PullRequestActionLabeled means labels were added.
	PullRequestActionLabeled PullRequestEventAction = "labeled"
	// PullRequestActionUnlabeled means labels were removed
	PullRequestActionUnlabeled PullRequestEventAction = "unlabeled"
	// PullRequestActionOpened means the PR was created
	PullRequestActionOpened PullRequestEventAction = "opened"
	// PullRequestActionEdited means the PR body changed.
	PullRequestActionEdited PullRequestEventAction = "edited"
	// PullRequestActionClosed means the PR was closed (or was merged).
	PullRequestActionClosed PullRequestEventAction = "closed"
	// PullRequestActionReopened means the PR was reopened.
	PullRequestActionReopened PullRequestEventAction = "reopened"
	// PullRequestActionSynchronize means the git state changed.
	PullRequestActionSynchronize PullRequestEventAction = "synchronize"
	// PullRequestActionReadyForReview means the PR is no longer a draft PR.
	PullRequestActionReadyForReview PullRequestEventAction = "ready_for_review"
	// PullRequestActionConvertedToDraft means the PR is now a draft PR.
	PullRequestActionConvertedToDraft PullRequestEventAction = "converted_to_draft"
	// PullRequestActionLocked means labels were added.
	PullRequestActionLocked PullRequestEventAction = "locked"
	// PullRequestActionUnlocked means labels were removed
	PullRequestActionUnlocked PullRequestEventAction = "unlocked"
)

// GenericEvent is a lightweight struct containing just Sender, Organization and Repo as
// they are allWebhook payload object common properties:
// https://developer.github.com/webhooks/event-payloads/#webhook-payload-object-common-properties
type GenericEvent struct {
	Sender User         `json:"sender"`
	Org    Organization `json:"organization"`
	Repo   Repo         `json:"repository"`
}

// PullRequestEvent is what GitHub sends us when a PR is changed.
type PullRequestEvent struct {
	Action      PullRequestEventAction `json:"action"`
	Number      int                    `json:"number"`
	PullRequest PullRequest            `json:"pull_request"`
	Repo        Repo                   `json:"repository"`
	Label       Label                  `json:"label"`
	Sender      User                   `json:"sender"`

	// Changes holds raw change data, which we must inspect
	// and deserialize later as this is a polymorphic field
	Changes json.RawMessage `json:"changes"`

	// GUID is included in the header of the request received by GitHub.
	GUID string
}

// PullRequest contains information about a PullRequest.
type PullRequest struct {
	ID                 int               `json:"id"`
	Number             int               `json:"number"`
	HTMLURL            string            `json:"html_url"`
	User               User              `json:"user"`
	Labels             []Label           `json:"labels"`
	Base               PullRequestBranch `json:"base"`
	Head               PullRequestBranch `json:"head"`
	Title              string            `json:"title"`
	Body               string            `json:"body"`
	RequestedReviewers []User            `json:"requested_reviewers"`
	Assignees          []User            `json:"assignees"`
	State              string            `json:"state"`
	Draft              bool              `json:"draft"`
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
	// If the PR doesn't have any milestone, `milestone` is null and is unmarshaled to nil.
	Milestone *Milestone `json:"milestone,omitempty"`
	Commits   int        `json:"commits"`
}

// PullRequestBranch contains information about a particular branch in a PR.
type PullRequestBranch struct {
	Ref  string `json:"ref"`
	SHA  string `json:"sha"`
	Repo Repo   `json:"repo"`
}

// Label describes a GitHub label.
type Label struct {
	URL         string `json:"url"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

// PullRequestFileStatus enumerates the statuses for this webhook payload type.
type PullRequestFileStatus string

const (
	// PullRequestFileModified means a file changed.
	PullRequestFileModified PullRequestFileStatus = "modified"
	// PullRequestFileAdded means a file was added.
	PullRequestFileAdded = "added"
	// PullRequestFileRemoved means a file was deleted.
	PullRequestFileRemoved = "removed"
	// PullRequestFileRenamed means a file moved.
	PullRequestFileRenamed = "renamed"
)

// PullRequestChange contains information about what a PR changed.
type PullRequestChange struct {
	SHA              string `json:"sha"`
	Filename         string `json:"filename"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch"`
	BlobURL          string `json:"blob_url"`
	PreviousFilename string `json:"previous_filename"`
}

// Repo contains general repository information: it includes fields available
// in repo records returned by GH "List" methods but not those returned by GH
// "Get" method.
// See also https://developer.github.com/v3/repos/#list-organization-repositories
type Repo struct {
	Owner         User   `json:"owner"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	Fork          bool   `json:"fork"`
	DefaultBranch string `json:"default_branch"`
	Archived      bool   `json:"archived"`
	Private       bool   `json:"private"`
	Description   string `json:"description"`
	Homepage      string `json:"homepage"`
	HasIssues     bool   `json:"has_issues"`
	HasProjects   bool   `json:"has_projects"`
	HasWiki       bool   `json:"has_wiki"`
	// Permissions reflect the permission level for the requester, so
	// on a repository GET call this will be for the user whose token
	// is being used, if listing a team's repos this will be for the
	// team's privilege level in the repo
	Permissions RepoPermissions `json:"permissions"`
}

// Repo contains detailed repository information, including items
// that are not available in repo records returned by GH "List" methods
// but are in those returned by GH "Get" method.
// See https://developer.github.com/v3/repos/#list-organization-repositories
// See https://developer.github.com/v3/repos/#get
type FullRepo struct {
	Repo

	AllowSquashMerge bool `json:"allow_squash_merge,omitempty"`
	AllowMergeCommit bool `json:"allow_merge_commit,omitempty"`
	AllowRebaseMerge bool `json:"allow_rebase_merge,omitempty"`
}

// RepoRequest contains metadata used in requests to create or update a Repo.
// Compared to `Repo`, its members are pointers to allow the "not set/use default
// semantics.
// See also:
// - https://developer.github.com/v3/repos/#create
// - https://developer.github.com/v3/repos/#edit
type RepoRequest struct {
	Name             *string `json:"name,omitempty"`
	Description      *string `json:"description,omitempty"`
	Homepage         *string `json:"homepage,omitempty"`
	Private          *bool   `json:"private,omitempty"`
	HasIssues        *bool   `json:"has_issues,omitempty"`
	HasProjects      *bool   `json:"has_projects,omitempty"`
	HasWiki          *bool   `json:"has_wiki,omitempty"`
	AllowSquashMerge *bool   `json:"allow_squash_merge,omitempty"`
	AllowMergeCommit *bool   `json:"allow_merge_commit,omitempty"`
	AllowRebaseMerge *bool   `json:"allow_rebase_merge,omitempty"`
}

// RepoCreateRequest contains metadata used in requests to create a repo.
// See also: https://developer.github.com/v3/repos/#create
type RepoCreateRequest struct {
	RepoRequest `json:",omitempty"`

	AutoInit          *bool   `json:"auto_init,omitempty"`
	GitignoreTemplate *string `json:"gitignore_template,omitempty"`
	LicenseTemplate   *string `json:"license_template,omitempty"`
}

func (r RepoRequest) ToRepo() *FullRepo {
	setString := func(dest, src *string) {
		if src != nil {
			*dest = *src
		}
	}
	setBool := func(dest, src *bool) {
		if src != nil {
			*dest = *src
		}
	}

	var repo FullRepo
	setString(&repo.Name, r.Name)
	setString(&repo.Description, r.Description)
	setString(&repo.Homepage, r.Homepage)
	setBool(&repo.Private, r.Private)
	setBool(&repo.HasIssues, r.HasIssues)
	setBool(&repo.HasProjects, r.HasProjects)
	setBool(&repo.HasWiki, r.HasWiki)
	setBool(&repo.AllowSquashMerge, r.AllowSquashMerge)
	setBool(&repo.AllowMergeCommit, r.AllowMergeCommit)
	setBool(&repo.AllowRebaseMerge, r.AllowRebaseMerge)

	return &repo
}

// Defined returns true if at least one of the pointer fields are not nil
func (r RepoRequest) Defined() bool {
	return r.Name != nil || r.Description != nil || r.Homepage != nil || r.Private != nil ||
		r.HasIssues != nil || r.HasProjects != nil || r.HasWiki != nil || r.AllowSquashMerge != nil ||
		r.AllowMergeCommit != nil || r.AllowRebaseMerge != nil
}

// RepoUpdateRequest contains metadata used for updating a repository
// See also: https://developer.github.com/v3/repos/#edit
type RepoUpdateRequest struct {
	RepoRequest `json:",omitempty"`

	DefaultBranch *string `json:"default_branch,omitempty"`
	Archived      *bool   `json:"archived,omitempty"`
}

func (r RepoUpdateRequest) ToRepo() *FullRepo {
	repo := r.RepoRequest.ToRepo()
	if r.DefaultBranch != nil {
		repo.DefaultBranch = *r.DefaultBranch
	}
	if r.Archived != nil {
		repo.Archived = *r.Archived
	}

	return repo
}

func (r RepoUpdateRequest) Defined() bool {
	return r.RepoRequest.Defined() || r.DefaultBranch != nil || r.Archived != nil
}

// RepoPermissions describes which permission level an entity has in a
// repo. At most one of the booleans here should be true.
type RepoPermissions struct {
	// Pull is equivalent to "Read" permissions in the web UI
	Pull bool `json:"pull"`
	// Push is equivalent to "Edit" permissions in the web UI
	Push  bool `json:"push"`
	Admin bool `json:"admin"`
}

// RepoPermissionLevel is admin, write, read or none.
//
// See https://developer.github.com/v3/repos/collaborators/#review-a-users-permission-level
type RepoPermissionLevel string

const (
	// Read allows pull but not push
	Read RepoPermissionLevel = "read"
	// Write allows Read plus push
	Write RepoPermissionLevel = "write"
	// Admin allows Write plus change others' rights.
	Admin RepoPermissionLevel = "admin"
	// None disallows everything
	None RepoPermissionLevel = "none"
)

var repoPermissionLevels = map[RepoPermissionLevel]bool{
	Read:  true,
	Write: true,
	Admin: true,
	None:  true,
}

// MarshalText returns the byte representation of the permission
func (l RepoPermissionLevel) MarshalText() ([]byte, error) {
	return []byte(l), nil
}

// UnmarshalText validates the text is a valid string
func (l *RepoPermissionLevel) UnmarshalText(text []byte) error {
	v := RepoPermissionLevel(text)
	if _, ok := repoPermissionLevels[v]; !ok {
		return fmt.Errorf("bad repo permission: %s not in %v", v, repoPermissionLevels)
	}
	*l = v
	return nil
}

type TeamPermission string

const (
	RepoPull  TeamPermission = "pull"
	RepoPush  TeamPermission = "push"
	RepoAdmin TeamPermission = "admin"
)

// Branch contains general branch information.
type Branch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"` // only included for ?protection=true requests
	// TODO(fejta): consider including undocumented protection key
}

// BranchProtection represents protections
// currently in place for a branch
// See also: https://developer.github.com/v3/repos/branches/#get-branch-protection
type BranchProtection struct {
	RequiredStatusChecks       *RequiredStatusChecks       `json:"required_status_checks"`
	EnforceAdmins              EnforceAdmins               `json:"enforce_admins"`
	RequiredPullRequestReviews *RequiredPullRequestReviews `json:"required_pull_request_reviews"`
	Restrictions               *Restrictions               `json:"restrictions"`
}

// EnforceAdmins specifies whether to enforce the
// configured branch restrictions for administrators.
type EnforceAdmins struct {
	Enabled bool `json:"enabled"`
}

// RequiredPullRequestReviews exposes the state of review rights.
type RequiredPullRequestReviews struct {
	DismissalRestrictions        *Restrictions `json:"dismissal_restrictions"`
	DismissStaleReviews          bool          `json:"dismiss_stale_reviews"`
	RequireCodeOwnerReviews      bool          `json:"require_code_owner_reviews"`
	RequiredApprovingReviewCount int           `json:"required_approving_review_count"`
}

// Restrictions exposes restrictions in github for an activity to people/teams.
type Restrictions struct {
	Users []User `json:"users,omitempty"`
	Teams []Team `json:"teams,omitempty"`
}

// BranchProtectionRequest represents
// protections to put in place for a branch.
// See also: https://developer.github.com/v3/repos/branches/#update-branch-protection
type BranchProtectionRequest struct {
	RequiredStatusChecks       *RequiredStatusChecks              `json:"required_status_checks"`
	EnforceAdmins              *bool                              `json:"enforce_admins"`
	RequiredPullRequestReviews *RequiredPullRequestReviewsRequest `json:"required_pull_request_reviews"`
	Restrictions               *RestrictionsRequest               `json:"restrictions"`
	RequiredLinearHistory      bool                               `json:"required_linear_history"`
	AllowForcePushes           bool                               `json:"allow_force_pushes"`
	AllowDeletions             bool                               `json:"allow_deletions"`
}

func (r BranchProtectionRequest) String() string {
	bytes, err := json.Marshal(&r)
	if err != nil {
		return fmt.Sprintf("%#v", r)
	}
	return string(bytes)
}

// RequiredStatusChecks specifies which contexts must pass to merge.
type RequiredStatusChecks struct {
	Strict   bool     `json:"strict"` // PR must be up to date (include latest base branch commit).
	Contexts []string `json:"contexts"`
}

// RequiredPullRequestReviewsRequest controls a request for review rights.
type RequiredPullRequestReviewsRequest struct {
	DismissalRestrictions        RestrictionsRequest `json:"dismissal_restrictions"`
	DismissStaleReviews          bool                `json:"dismiss_stale_reviews"`
	RequireCodeOwnerReviews      bool                `json:"require_code_owner_reviews"`
	RequiredApprovingReviewCount int                 `json:"required_approving_review_count"`
}

// RestrictionsRequest tells github to restrict an activity to people/teams.
//
// Use *[]string in order to distinguish unset and empty list.
// This is needed by dismissal_restrictions to distinguish
// do not restrict (empty object) and restrict everyone (nil user/teams list)
type RestrictionsRequest struct {
	// Users is a list of user logins
	Users *[]string `json:"users,omitempty"`
	// Teams is a list of team slugs
	Teams *[]string `json:"teams,omitempty"`
}

// HookConfig holds the endpoint and its secret.
type HookConfig struct {
	URL         string  `json:"url"`
	ContentType *string `json:"content_type,omitempty"`
	Secret      *string `json:"secret,omitempty"`
}

// Hook holds info about the webhook configuration.
type Hook struct {
	ID     int        `json:"id"`
	Name   string     `json:"name"`
	Events []string   `json:"events"`
	Active bool       `json:"active"`
	Config HookConfig `json:"config"`
}

// HookRequest can create and/or edit a webhook.
//
// AddEvents and RemoveEvents are only valid during an edit, and only for a repo
type HookRequest struct {
	Name         string      `json:"name,omitempty"` // must be web or "", only create
	Active       *bool       `json:"active,omitempty"`
	AddEvents    []string    `json:"add_events,omitempty"` // only repo edit
	Config       *HookConfig `json:"config,omitempty"`
	Events       []string    `json:"events,omitempty"`
	RemoveEvents []string    `json:"remove_events,omitempty"` // only repo edit
}

// AllHookEvents causes github to send all events.
// https://developer.github.com/v3/activity/events/types/
var AllHookEvents = []string{"*"}

// IssueEventAction enumerates the triggers for this
// webhook payload type. See also:
// https://developer.github.com/v3/activity/events/types/#issuesevent
type IssueEventAction string

const (
	// IssueActionAssigned means assignees were added.
	IssueActionAssigned IssueEventAction = "assigned"
	// IssueActionUnassigned means assignees were added.
	IssueActionUnassigned IssueEventAction = "unassigned"
	// IssueActionLabeled means labels were added.
	IssueActionLabeled IssueEventAction = "labeled"
	// IssueActionUnlabeled means labels were removed.
	IssueActionUnlabeled IssueEventAction = "unlabeled"
	// IssueActionOpened means issue was opened/created.
	IssueActionOpened IssueEventAction = "opened"
	// IssueActionEdited means issue body was edited.
	IssueActionEdited IssueEventAction = "edited"
	// IssueActionDeleted means the issue was deleted.
	IssueActionDeleted IssueEventAction = "deleted"
	// IssueActionMilestoned means the milestone was added/changed.
	IssueActionMilestoned IssueEventAction = "milestoned"
	// IssueActionDemilestoned means a milestone was removed.
	IssueActionDemilestoned IssueEventAction = "demilestoned"
	// IssueActionClosed means issue was closed.
	IssueActionClosed IssueEventAction = "closed"
	// IssueActionReopened means issue was reopened.
	IssueActionReopened IssueEventAction = "reopened"
	// IssueActionPinned means the issue was pinned.
	IssueActionPinned IssueEventAction = "pinned"
	// IssueActionUnpinned means the issue was unpinned.
	IssueActionUnpinned IssueEventAction = "unpinned"
	// IssueActionTransferred means the issue was transferred to another repo.
	IssueActionTransferred IssueEventAction = "transferred"
	// IssueActionLocked means the issue was locked.
	IssueActionLocked IssueEventAction = "locked"
	// IssueActionUnlocked means the issue was unlocked.
	IssueActionUnlocked IssueEventAction = "unlocked"
)

// IssueEvent represents an issue event from a webhook payload (not from the events API).
type IssueEvent struct {
	Action IssueEventAction `json:"action"`
	Issue  Issue            `json:"issue"`
	Repo   Repo             `json:"repository"`
	// Label is specified for IssueActionLabeled and IssueActionUnlabeled events.
	Label Label `json:"label"`

	// GUID is included in the header of the request received by GitHub.
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
	// IssueCommentActionCreated means the comment was created.
	IssueCommentActionCreated IssueCommentEventAction = "created"
	// IssueCommentActionEdited means the comment was edited.
	IssueCommentActionEdited IssueCommentEventAction = "edited"
	// IssueCommentActionDeleted means the comment was deleted.
	IssueCommentActionDeleted IssueCommentEventAction = "deleted"
)

// IssueCommentEvent is what GitHub sends us when an issue comment is changed.
type IssueCommentEvent struct {
	Action  IssueCommentEventAction `json:"action"`
	Issue   Issue                   `json:"issue"`
	Comment IssueComment            `json:"comment"`
	Repo    Repo                    `json:"repository"`

	// GUID is included in the header of the request received by GitHub.
	GUID string
}

// Issue represents general info about an issue.
type Issue struct {
	ID        int       `json:"id"`
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

// IsAssignee checks if a user is assigned to the issue.
func (i Issue) IsAssignee(login string) bool {
	for _, assignee := range i.Assignees {
		if NormLogin(login) == NormLogin(assignee.Login) {
			return true
		}
	}
	return false
}

// IsAuthor checks if a user is the author of the issue.
func (i Issue) IsAuthor(login string) bool {
	return NormLogin(i.User.Login) == NormLogin(login)
}

// IsPullRequest checks if an issue is a pull request.
func (i Issue) IsPullRequest() bool {
	return i.PullRequest != nil
}

// HasLabel checks if an issue has a given label.
func (i Issue) HasLabel(labelToFind string) bool {
	for _, label := range i.Labels {
		if strings.ToLower(label.Name) == strings.ToLower(labelToFind) {
			return true
		}
	}
	return false
}

// IssueComment represents general info about an issue comment.
type IssueComment struct {
	ID        int       `json:"id,omitempty"`
	Body      string    `json:"body"`
	User      User      `json:"user,omitempty"`
	HTMLURL   string    `json:"html_url,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// StatusEvent fires whenever a git commit changes.
//
// See https://developer.github.com/v3/activity/events/types/#statusevent
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

	// GUID is included in the header of the request received by GitHub.
	GUID string
}

// IssuesSearchResult represents the result of an issues search.
type IssuesSearchResult struct {
	Total  int     `json:"total_count,omitempty"`
	Issues []Issue `json:"items,omitempty"`
}

// PushEvent is what GitHub sends us when a user pushes to a repo.
type PushEvent struct {
	Ref     string   `json:"ref"`
	Before  string   `json:"before"`
	After   string   `json:"after"`
	Created bool     `json:"created"`
	Deleted bool     `json:"deleted"`
	Forced  bool     `json:"forced"`
	Compare string   `json:"compare"`
	Commits []Commit `json:"commits"`
	// Pusher is the user that pushed the commit, valid in a webhook event.
	Pusher User `json:"pusher"`
	// Sender contains more information that Pusher about the user.
	Sender User `json:"sender"`
	Repo   Repo `json:"repository"`

	// GUID is included in the header of the request received by GitHub.
	GUID string
}

// Branch returns the name of the branch to which the user pushed.
func (pe PushEvent) Branch() string {
	ref := strings.TrimPrefix(pe.Ref, "refs/heads/") // if Ref is a branch
	ref = strings.TrimPrefix(ref, "refs/tags/")      // if Ref is a tag
	return ref
}

// Commit represents general info about a commit.
type Commit struct {
	ID       string   `json:"id"`
	Message  string   `json:"message"`
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Modified []string `json:"modified"`
}

// SingleCommit is the commit part received when requesting a single commit
// https://developer.github.com/v3/repos/commits/#get-a-single-commit
type SingleCommit struct {
	Commit struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	} `json:"commit"`
}

// ReviewEventAction enumerates the triggers for this
// webhook payload type. See also:
// https://developer.github.com/v3/activity/events/types/#pullrequestreviewevent
type ReviewEventAction string

const (
	// ReviewActionSubmitted means the review was submitted.
	ReviewActionSubmitted ReviewEventAction = "submitted"
	// ReviewActionEdited means the review was edited.
	ReviewActionEdited ReviewEventAction = "edited"
	// ReviewActionDismissed means the review was dismissed.
	ReviewActionDismissed ReviewEventAction = "dismissed"
)

// ReviewEvent is what GitHub sends us when a PR review is changed.
type ReviewEvent struct {
	Action      ReviewEventAction `json:"action"`
	PullRequest PullRequest       `json:"pull_request"`
	Repo        Repo              `json:"repository"`
	Review      Review            `json:"review"`

	// GUID is included in the header of the request received by GitHub.
	GUID string
}

// ReviewState is the state a review can be in.
type ReviewState string

// Possible review states.
const (
	ReviewStateApproved         ReviewState = "APPROVED"
	ReviewStateChangesRequested             = "CHANGES_REQUESTED"
	ReviewStateCommented                    = "COMMENTED"
	ReviewStateDismissed                    = "DISMISSED"
	ReviewStatePending                      = "PENDING"
)

// Review describes a Pull Request review.
type Review struct {
	ID          int         `json:"id"`
	User        User        `json:"user"`
	Body        string      `json:"body"`
	State       ReviewState `json:"state"`
	HTMLURL     string      `json:"html_url"`
	SubmittedAt time.Time   `json:"submitted_at"`
}

// ReviewCommentEventAction enumerates the triggers for this
// webhook payload type. See also:
// https://developer.github.com/v3/activity/events/types/#pullrequestreviewcommentevent
type ReviewCommentEventAction string

const (
	// ReviewCommentActionCreated means the comment was created.
	ReviewCommentActionCreated ReviewCommentEventAction = "created"
	// ReviewCommentActionEdited means the comment was edited.
	ReviewCommentActionEdited ReviewCommentEventAction = "edited"
	// ReviewCommentActionDeleted means the comment was deleted.
	ReviewCommentActionDeleted ReviewCommentEventAction = "deleted"
)

// ReviewCommentEvent is what GitHub sends us when a PR review comment is changed.
type ReviewCommentEvent struct {
	Action      ReviewCommentEventAction `json:"action"`
	PullRequest PullRequest              `json:"pull_request"`
	Repo        Repo                     `json:"repository"`
	Comment     ReviewComment            `json:"comment"`

	// GUID is included in the header of the request received by GitHub.
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

const (
	// PrivacySecret memberships are only visible to other team members.
	PrivacySecret = "secret"
	// PrivacyClosed memberships are visible to org members.
	PrivacyClosed = "closed"
)

// Team is a github organizational team
type Team struct {
	ID           int            `json:"id,omitempty"`
	Name         string         `json:"name"`
	Slug         string         `json:"slug"`
	Description  string         `json:"description,omitempty"`
	Privacy      string         `json:"privacy,omitempty"`
	Parent       *Team          `json:"parent,omitempty"`         // Only present in responses
	ParentTeamID *int           `json:"parent_team_id,omitempty"` // Only valid in creates/edits
	Permission   TeamPermission `json:"permission,omitempty"`
}

// TeamMember is a member of an organizational team
type TeamMember struct {
	Login string `json:"login"`
}

const (
	// RoleAll lists both members and admins
	RoleAll = "all"
	// RoleAdmin specifies the user is an org admin, or lists only admins
	RoleAdmin = "admin"
	// RoleMaintainer specifies the user is a team maintainer, or lists only maintainers
	RoleMaintainer = "maintainer"
	// RoleMember specifies the user is a regular user, or only lists regular users
	RoleMember = "member"
	// StatePending specifies the user has an invitation to the org/team.
	StatePending = "pending"
	// StateActive specifies the user's membership is active.
	StateActive = "active"
)

// Membership specifies the role and state details for an org and/or team.
type Membership struct {
	// admin or member
	Role string `json:"role"`
	// pending or active
	State string `json:"state,omitempty"`
}

// Organization stores metadata information about an organization
type Organization struct {
	// Login has the same meaning as Name, but it's more reliable to use as Name can sometimes be empty,
	// see https://developer.github.com/v3/orgs/#list-organizations
	Login string `json:"login"`
	// BillingEmail holds private billing address
	BillingEmail string `json:"billing_email"`
	Company      string `json:"company"`
	// Email is publicly visible
	Email                        string `json:"email"`
	Location                     string `json:"location"`
	Name                         string `json:"name"`
	Description                  string `json:"description"`
	HasOrganizationProjects      bool   `json:"has_organization_projects"`
	HasRepositoryProjects        bool   `json:"has_repository_projects"`
	DefaultRepositoryPermission  string `json:"default_repository_permission"`
	MembersCanCreateRepositories bool   `json:"members_can_create_repositories"`
}

// OrgMembership contains Membership fields for user membership in an org.
type OrgMembership struct {
	Membership
}

// TeamMembership contains Membership fields for user membership on a team.
type TeamMembership struct {
	Membership
}

// OrgInvitation contains Login and other details about the invitation.
type OrgInvitation struct {
	TeamMember
	Email   string     `json:"email"`
	Inviter TeamMember `json:"inviter"`
}

// GenericCommentEventAction coerces multiple actions into its generic equivalent.
type GenericCommentEventAction string

// Comments indicate values that are coerced to the specified value.
const (
	// GenericCommentActionCreated means something was created/opened/submitted
	GenericCommentActionCreated GenericCommentEventAction = "created" // "opened", "submitted"
	// GenericCommentActionEdited means something was edited.
	GenericCommentActionEdited GenericCommentEventAction = "edited"
	// GenericCommentActionDeleted means something was deleted/dismissed.
	GenericCommentActionDeleted GenericCommentEventAction = "deleted" // "dismissed"
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
	ID           int `json:"id"`
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
	GUID         string
}

// Milestone is a milestone defined on a github repository
type Milestone struct {
	Title  string `json:"title"`
	Number int    `json:"number"`
}

// RepositoryCommit represents a commit in a repo.
// Note that it's wrapping a GitCommit, so author/committer information is in two places,
// but contain different details about them: in RepositoryCommit "github details", in GitCommit - "git details".
type RepositoryCommit struct {
	SHA         string    `json:"sha"`
	Commit      GitCommit `json:"commit"`
	Author      User      `json:"author"`
	Committer   User      `json:"committer"`
	Parents     []Commit  `json:"parents,omitempty"`
	HTMLURL     string    `json:"html_url"`
	URL         string    `json:"url"`
	CommentsURL string    `json:"comments_url"`
}

// GitCommit represents a GitHub commit.
type GitCommit struct {
	SHA     string `json:"sha,omitempty"`
	Message string `json:"message,omitempty"`
}

// Project is a github project
type Project struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

// ProjectColumn is a colunm in a github project
type ProjectColumn struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

// ProjectCard is a github project card
type ProjectCard struct {
	ID          int    `json:"id"`
	ContentID   int    `json:"content_id"`
	ContentType string `json:"content_type"`
	ContentURL  string `json:"content_url"`
}
