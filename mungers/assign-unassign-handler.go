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
	"bytes"
	"fmt"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
	c "k8s.io/contrib/mungegithub/mungers/matchers/comment"
)

const (
	assignCommand   = "assign"
	unassignCommand = "unassign"
	invalidReviewer = "ASSIGN_NOTIFIER"
)

// AssignUnassignHandler will
// - will assign a github user to a PR if they comment "/assign"
// - will unassign a github user to a PR if they comment "/unassign"
type AssignUnassignHandler struct {
	features *features.Features
}

func init() {
	dh := &AssignUnassignHandler{}
	RegisterMungerOrDie(dh)
}

// Name is the name usable in --pr-mungers
func (AssignUnassignHandler) Name() string { return "assign-unassign-handler" }

// RequiredFeatures is a slice of 'features' that must be provided
func (AssignUnassignHandler) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (h *AssignUnassignHandler) Initialize(config *github.Config, features *features.Features) error {
	h.features = features
	return nil
}

// EachLoop is called at the start of every munge loop
func (AssignUnassignHandler) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (h AssignUnassignHandler) AddFlags(cmd *cobra.Command, config *github.Config) {
}

// Munge is the workhorse the will actually make updates to the PR
func (h AssignUnassignHandler) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	comments, err := obj.ListComments()
	if err != nil {
		glog.Errorf("unexpected error getting comments: %v", err)
		return
	}

	fileList, err := obj.ListFiles()
	if err != nil {
		glog.Errorf("Could not list the files for PR %v: %v", obj.Issue.Number, err)
		return
	}

	//get ALL (not just leaf) the people that could potentially own the file based on the blunderbuss.go implementation
	potentialOwners, _ := getPotentialOwners(*obj.Issue.User.Login, h.features, fileList, false)

	toAssign, toUnassign := h.getAssigneesAndUnassignees(obj, comments, fileList, potentialOwners)
	for _, username := range toAssign.List() {
		obj.AssignPR(username)
	}
	obj.UnassignPR(toUnassign.List()...)
}

// getAssigneesAndUnassignees checks to see when someone comments "/assign" or "/unassign"
// returns two sets.String
// 1. github handles to be assigned
// 2. github handles to be unassigned
// Note* Could possibly assign directly in the function call, but easier to test if function returns toAssign, toUnassign
func (h *AssignUnassignHandler) getAssigneesAndUnassignees(obj *github.MungeObject, comments []*githubapi.IssueComment, fileList []*githubapi.CommitFile, potentialOwners weightMap) (toAssign, toUnassign sets.String) {
	toAssign = sets.String{}
	toUnassign = sets.String{}

	assignComments := c.FilterComments(comments, c.CommandName(assignCommand))
	unassignComments := c.FilterComments(comments, c.CommandName(unassignCommand))
	invalidUsers := sets.String{}

	//collect all the people that should be assigned
	for _, cmt := range assignComments {
		if isValidReviewer(potentialOwners, cmt.User) {
			obj.DeleteComment(cmt)
			toAssign.Insert(*cmt.User.Login)
		} else {
			// build the set of people who asked to be assigned but aren't in reviewers
			// use the @ as a prefix so github notifies invalid users
			invalidUsers.Insert("@" + *cmt.User.Login)
		}

	}

	// collect all the people that should be unassigned
	for _, cmt := range unassignComments {
		if isAssignee(obj.Issue.Assignees, cmt.User) {
			obj.DeleteComment(cmt)
			toUnassign.Insert(*cmt.User.Login)
		}
	}

	// Create a notification if someone tried to self assign, but could not because they weren't in the owners files
	if invalidUsers.Len() != 0 {
		previousNotifications := c.FilterComments(comments, c.MungerNotificationName(invalidReviewer))
		if assignComments.Empty() || (!previousNotifications.Empty() && previousNotifications.GetLast().CreatedAt.After(*assignComments.GetLast().CreatedAt)) {
			// if there were no assign comments, no need to notify
			// if the last notification happened after the last assign comment, no need to notify again
			return toAssign, toUnassign
		}
		if !previousNotifications.Empty() {
			for _, c := range previousNotifications {
				obj.DeleteComment(c)
			}
		}
		context := bytes.NewBufferString("The following people cannot be assigned because they are not in the OWNERS files\n")
		for user := range invalidUsers {
			context.WriteString(fmt.Sprintf("- %s\n", user))
		}
		context.WriteString("\n")
		c.Notification{Name: invalidReviewer, Arguments: "", Context: context.String()}.Post(obj)

	}
	return toAssign, toUnassign
}

func isValidReviewer(potentialOwners weightMap, commenter *githubapi.User) bool {
	if commenter == nil || commenter.Login == nil {
		return false
	}
	if _, ok := potentialOwners[*commenter.Login]; ok {
		return true
	}
	return false
}

func isAssignee(assignees []*githubapi.User, someUser *githubapi.User) bool {
	for _, assignee := range assignees {
		if assignee.Login == nil || someUser.Login == nil {
			continue
		}
		if *assignee.Login == *someUser.Login && *someUser.ID == *assignee.ID {
			return true
		}
	}
	return false
}
