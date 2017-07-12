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

package mungers

import (
	"regexp"
	"sort"
	"strconv"

	githubapi "github.com/google/go-github/github"

	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungers/approvers"
	c "k8s.io/test-infra/mungegithub/mungers/matchers/comment"
	"k8s.io/test-infra/mungegithub/mungers/matchers/event"
	"k8s.io/test-infra/mungegithub/options"
)

const (
	approveCommand  = "APPROVE"
	lgtmCommand     = "LGTM"
	cancelArgument  = "cancel"
	noIssueArgument = "no-issue"
)

var AssociatedIssueRegex = regexp.MustCompile(`(?:kubernetes/[^/]+/issues/|#)(\d+)`)

// ApprovalHandler will try to add "approved" label once
// all files of change has been approved by approvers.
type ApprovalHandler struct {
	features *features.Features
}

func init() {
	h := &ApprovalHandler{}
	RegisterMungerOrDie(h)
}

// Name is the name usable in --pr-mungers
func (*ApprovalHandler) Name() string { return "approval-handler" }

// RequiredFeatures is a slice of 'features' that must be provided
func (*ApprovalHandler) RequiredFeatures() []string {
	return []string{features.RepoFeatureName, features.AliasesFeature}
}

// Initialize will initialize the munger
func (h *ApprovalHandler) Initialize(config *github.Config, features *features.Features) error {
	h.features = features
	return nil
}

// EachLoop is called at the start of every munge loop
func (*ApprovalHandler) EachLoop() error { return nil }

// RegisterOptions registers config options for this munger.
func (*ApprovalHandler) RegisterOptions(opts *options.Options) {}

// Returns associated issue, or 0 if it can't find any.
// This is really simple, and could be improved later.
func findAssociatedIssue(body *string) int {
	if body == nil {
		return 0
	}
	match := AssociatedIssueRegex.FindStringSubmatch(*body)
	if len(match) == 0 {
		return 0
	}
	v, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return v
}

// Munge is the workhorse the will actually make updates to the PR
// The algorithm goes as:
// - Initially, we build an approverSet
//   - Go through all comments in order of creation.
//		 - (Issue/PR comments, PR review comments, and PR review bodies are considered as comments)
//	 - If anyone said "/approve" or "/lgtm", add them to approverSet.
// - Then, for each file, we see if any approver of this file is in approverSet and keep track of files without approval
//   - An approver of a file is defined as:
//     - Someone listed as an "approver" in an OWNERS file in the files directory OR
//     - in one of the file's parent directorie
// - Iff all files have been approved, the bot will add the "approved" label.
// - Iff a cancel command is found, that reviewer will be removed from the approverSet
// 	and the munger will remove the approved label if it has been applied
func (h *ApprovalHandler) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}
	filenames := []string{}
	files, ok := obj.ListFiles()
	if !ok {
		return
	}
	for _, fn := range files {
		filenames = append(filenames, *fn.Filename)
	}
	issueComments, ok := obj.ListComments()
	if !ok {
		return
	}
	reviewComments, ok := obj.ListReviewComments()
	if !ok {
		return
	}
	reviews, ok := obj.ListReviews()
	if !ok {
		return
	}
	commentsFromIssueComments := c.FromIssueComments(issueComments)
	comments := append(c.FromReviewComments(reviewComments), commentsFromIssueComments...)
	comments = append(comments, c.FromReviews(reviews)...)
	sort.SliceStable(comments, func(i, j int) bool {
		return comments[i].CreatedAt.Before(*comments[j].CreatedAt)
	})
	approveComments := getApproveComments(comments)

	approversHandler := approvers.NewApprovers(
		approvers.NewOwners(
			filenames,
			approvers.NewRepoAlias(h.features.Repos, *h.features.Aliases),
			int64(*obj.Issue.Number)))
	approversHandler.AssociatedIssue = findAssociatedIssue(obj.Issue.Body)
	addApprovers(&approversHandler, approveComments)
	// Author implicitly approves their own PR
	if obj.Issue.User != nil && obj.Issue.User.Login != nil {
		url := ""
		if obj.Issue.HTMLURL != nil {
			// Append extra # so that it doesn't reload the page.
			url = *obj.Issue.HTMLURL + "#"
		}
		approversHandler.AddAuthorSelfApprover(*obj.Issue.User.Login, url)
	}

	for _, user := range obj.Issue.Assignees {
		if user != nil && user.Login != nil {
			approversHandler.AddAssignees(*user.Login)
		}
	}

	notificationMatcher := c.MungerNotificationName(approvers.ApprovalNotificationName)

	notifications := c.FilterComments(commentsFromIssueComments, notificationMatcher)
	latestNotification := notifications.GetLast()
	latestApprove := approveComments.GetLast()
	newMessage := h.updateNotification(obj.Org(), obj.Project(), latestNotification, latestApprove, approversHandler)
	if newMessage != nil {
		for _, notif := range notifications {
			obj.DeleteComment(notif.Source.(*githubapi.IssueComment))
		}
		obj.WriteComment(*newMessage)
	}

	if !approversHandler.IsApprovedWithIssue() {
		if obj.HasLabel(approvedLabel) && !humanAddedApproved(obj) {
			obj.RemoveLabel(approvedLabel)
		}
	} else {
		//pr is fully approved
		if !obj.HasLabel(approvedLabel) {
			obj.AddLabel(approvedLabel)
		}
	}

}

func humanAddedApproved(obj *github.MungeObject) bool {
	events, ok := obj.GetEvents()
	if !ok {
		return false
	}
	approveAddedMatcher := event.And([]event.Matcher{event.AddLabel{}, event.LabelName(approvedLabel)})
	labelEvents := event.FilterEvents(events, approveAddedMatcher)
	lastAdded := labelEvents.GetLast()
	if lastAdded == nil || lastAdded.Actor == nil || lastAdded.Actor.Login == nil {
		return false
	}
	return *lastAdded.Actor.Login != botName
}

func getApproveComments(comments []*c.Comment) c.FilteredComments {
	approverMatcher := c.CommandName(approveCommand)
	lgtmMatcher := c.CommandName(lgtmLabel)
	return c.FilterComments(comments, c.And{c.HumanActor(), c.Or{approverMatcher, lgtmMatcher}})
}

func (h *ApprovalHandler) updateNotification(org, project string, latestNotification, latestApprove *c.Comment, approversHandler approvers.Approvers) *string {
	if latestNotification != nil && (latestApprove == nil || latestApprove.CreatedAt.Before(*latestNotification.CreatedAt)) {
		// if we have an existing notification AND
		// the latestApprove happened before we updated
		// the notification, we do NOT need to update
		return nil
	}
	return approvers.GetMessage(approversHandler, org, project)
}

// addApprovers iterates through the list of comments on a PR
// and identifies all of the people that have said /approve and adds
// them to the Approvers.  The function uses the latest approve or cancel comment
// to determine the Users intention
func addApprovers(approversHandler *approvers.Approvers, approveComments c.FilteredComments) {
	for _, comment := range approveComments {
		commands := c.ParseCommands(comment)
		for _, cmd := range commands {
			if cmd.Name != approveCommand && cmd.Name != lgtmCommand {
				continue
			}
			if comment.Author == nil {
				continue
			}

			if cmd.Arguments == cancelArgument {
				approversHandler.RemoveApprover(*comment.Author)
			} else {
				url := ""
				if comment.HTMLURL != nil {
					url = *comment.HTMLURL
				}

				if cmd.Name == approveCommand {
					approversHandler.AddApprover(
						*comment.Author,
						url,
						cmd.Arguments == noIssueArgument,
					)
				} else {
					approversHandler.AddLGTMer(
						*comment.Author,
						url,
						cmd.Arguments == noIssueArgument,
					)
				}

			}
		}
	}
}
