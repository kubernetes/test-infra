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
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"

	"k8s.io/kubernetes/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungers/approvers"
	c "k8s.io/test-infra/mungegithub/mungers/matchers/comment"
	"k8s.io/test-infra/mungegithub/mungers/matchers/event"
)

const (
	approveCommand = "APPROVE"
	lgtmCommand    = "LGTM"
	cancel         = "cancel"
)

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

// AddFlags will add any request flags to the cobra `cmd`
func (*ApprovalHandler) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
// The algorithm goes as:
// - Initially, we build an approverSet
//   - Go through all comments after latest commit.
//	- If anyone said "/approve", add them to approverSet.
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
	comments, ok := obj.ListComments()
	if !ok {
		return
	}

	var prAuthor *string = nil
	if obj.Issue.User != nil && obj.Issue.User.Login != nil {
		prAuthor = obj.Issue.User.Login
	}

	approverSet := createApproverSet(comments, prAuthor)
	approversHandler := approvers.Approvers{
		approvers.NewOwners(filenames, h.features.Repos, int64(*obj.Issue.Number)),
		approverSet}

	notificationMatcher := c.MungerNotificationName(approvers.ApprovalNotificationName)

	latestNotification := c.FilterComments(comments, notificationMatcher).GetLast()
	latestApprove := getApproveComments(comments).GetLast()
	newMessage := h.updateNotification(obj.Org(), obj.Project(), latestNotification, latestApprove, approversHandler)
	if newMessage != nil {
		if latestNotification != nil {
			obj.DeleteComment(latestNotification)
		}
		obj.WriteComment(*newMessage)
	}

	if !approversHandler.IsApproved() {
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

func getApproveComments(comments []*githubapi.IssueComment) c.FilteredComments {
	approverMatcher := c.CommandName(approveCommand)
	lgtmMatcher := c.CommandName(lgtmLabel)
	return c.FilterComments(comments, c.Or{approverMatcher, lgtmMatcher})
}

func (h *ApprovalHandler) updateNotification(org, project string, latestNotification, latestApprove *githubapi.IssueComment, approversHandler approvers.Approvers) *string {
	if latestNotification != nil && (latestApprove == nil || latestApprove.CreatedAt.Before(*latestNotification.CreatedAt)) {
		// if we have an existing notification AND
		// the latestApprove happened before we updated
		// the notification, we do NOT need to update
		return nil
	}
	s := approvers.GetMessage(approversHandler, org, project)
	return &s
}

// createApproverSet iterates through the list of comments on a PR
// and identifies all of the people that have said /approve and adds
// them to the approverSet.  The function uses the latest approve or cancel comment
// to determine the Users intention
func createApproverSet(comments []*githubapi.IssueComment, prAuthor *string) sets.String {
	approverSet := sets.NewString()

	approveComments := getApproveComments(comments)
	for _, comment := range approveComments {
		commands := c.ParseCommands(comment)
		for _, cmd := range commands {
			if cmd.Name != approveCommand && cmd.Name != lgtmCommand {
				continue
			}
			if comment.User == nil || comment.User.Login == nil {
				continue
			}

			if cmd.Arguments == cancel {
				approverSet.Delete(*comment.User.Login)
			} else {
				approverSet.Insert(*comment.User.Login)
			}
		}
	}

	//prAuthor implicitly approves their own PR
	if prAuthor != nil {
		approverSet.Insert(*prAuthor)
	}

	return approverSet
}
