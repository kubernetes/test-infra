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
	"path/filepath"
	"sort"
	"strings"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	c "k8s.io/contrib/mungegithub/mungers/matchers/comment"
	"k8s.io/kubernetes/pkg/util/sets"
)

const (
	approvalNotificationName = "ApprovalNotifier"
	approveCommand           = "approve"
	cancel                   = "cancel"
	ownersFileName           = "OWNERS"
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
	files, ok := obj.ListFiles()
	if !ok {
		return
	}

	comments, ok := getCommentsAfterLastModified(obj)

	if !ok {
		return
	}

	ownersMap := h.getApprovedOwners(files, createApproverSet(comments))

	if err := h.updateNotification(obj, ownersMap); err != nil {
		return
	}

	for _, approverSet := range ownersMap {
		if approverSet.Len() == 0 {
			if obj.HasLabel(approvedLabel) {
				obj.RemoveLabel(approvedLabel)
			}
			return
		}
	}

	if !obj.HasLabel(approvedLabel) {
		obj.AddLabel(approvedLabel)
	}
}

func (h *ApprovalHandler) updateNotification(obj *github.MungeObject, ownersMap map[string]sets.String) error {
	notificationMatcher := c.MungerNotificationName(approvalNotificationName)
	comments, ok := obj.ListComments()
	if !ok {
		return fmt.Errorf("Unable to ListComments for %d", obj.Number())
	}

	notifications := c.FilterComments(comments, notificationMatcher)
	latestNotification := notifications.GetLast()
	if latestNotification == nil {
		body := h.getMessage(obj, ownersMap)
		obj.WriteComment(body)
	}

	latestApprove := c.FilterComments(comments, c.CommandName(approveCommand)).GetLast()
	if latestApprove == nil || latestApprove.CreatedAt == nil {
		// there was already a bot notification and nothing has changed since
		// or we wouldn't tell when the latestApproval occurred
		return nil
	}
	if latestApprove.CreatedAt.After(*latestNotification.CreatedAt) {
		// if someone approved since the last comment, we should update the comment
		glog.Infof("Latest approve was after last time notified")
		body := h.getMessage(obj, ownersMap)
		return obj.EditComment(latestApprove, body)
	}
	lastModified, ok := obj.LastModifiedTime()
	if !ok {
		return fmt.Errorf("Unable to get LastModifiedTime for %d", obj.Number())
	}
	if latestNotification.CreatedAt.Before(*lastModified) {
		// the PR was modified After our last notification, so we should update the approvers notification
		// i.e. People that have formerly approved haven't necessarily approved of new changes
		glog.Infof("PR Modified After Last Notification")
		body := h.getMessage(obj, ownersMap)
		return obj.EditComment(latestApprove, body)
	}
	return nil
}

// findPeopleToApprove Takes the Owners Files that Are Needed for the PR and chooses a good
// subset of Approvers that are guaranteed to cover all of them (exact cover)
// This is a greedy approximation and not guaranteed to find the minimum number of OWNERS
func (h ApprovalHandler) findPeopleToApprove(ownersPaths sets.String, prAuthor string) sets.String {

	// approverCount contains a map: person -> set of relevant OWNERS file they are in
	approverCount := make(map[string]sets.String)
	for ownersFile := range ownersPaths {
		// LeafApprovers removes the last part of a path for dirs and files, so we append owners to the path
		for approver := range h.features.Repos.LeafApprovers(filepath.Join(ownersFile, ownersFileName)) {
			if approver == prAuthor {
				// don't add the author of the PR to the list of candidates that can approve
				continue
			}
			if _, ok := approverCount[approver]; ok {
				approverCount[approver].Insert(ownersFile)
			} else {
				approverCount[approver] = sets.NewString(ownersFile)
			}
		}
	}

	copyOfFiles := sets.NewString()
	for fn := range ownersPaths {
		copyOfFiles.Insert(fn)
	}

	approverGroup := sets.NewString()
	var bestPerson string
	for copyOfFiles.Len() > 0 {
		maxCovered := 0
		for k, v := range approverCount {
			if v.Intersection(copyOfFiles).Len() > maxCovered {
				maxCovered = len(v)
				bestPerson = k
			}
		}

		approverGroup.Insert(bestPerson)
		toDelete := sets.NewString()
		// remove all files in the directories that our approver approved AND
		// in the subdirectories that s/he approved.  HasPrefix finds subdirs
		for fn := range copyOfFiles {
			for approvedFile := range approverCount[bestPerson] {
				if strings.HasPrefix(fn, approvedFile) {
					toDelete.Insert(fn)
				}

			}
		}
		copyOfFiles.Delete(toDelete.List()...)
	}
	return approverGroup
}

// removeSubdirs takes a list of directories as an input and returns a set of directories with all
// subdirectories removed.  E.g. [/a,/a/b/c,/d/e,/d/e/f] -> [/a, /d/e]
func removeSubdirs(dirList []string) sets.String {
	toDel := sets.String{}
	for i := 0; i < len(dirList)-1; i++ {
		for j := i + 1; j < len(dirList); j++ {
			// ex /a/b has prefix /a so if remove /a/b since its already covered
			if strings.HasPrefix(dirList[i], dirList[j]) {
				toDel.Insert(dirList[i])
			} else if strings.HasPrefix(dirList[j], dirList[i]) {
				toDel.Insert(dirList[j])
			}
		}
	}
	finalSet := sets.NewString(dirList...)
	finalSet.Delete(toDel.List()...)
	return finalSet
}

// getMessage returns the comment body that we want the approval-handler to display on PRs
// The comment shows:
// 	- a list of approvers files (and links) needed to get the PR approved
// 	- a list of approvers files with strikethroughs that already have an approver's approval
// 	- a suggested list of people from each OWNERS files that can fully approve the PR
// 	- how an approver can indicate their approval
// 	- how an approver can cancel their approval
func (h *ApprovalHandler) getMessage(obj *github.MungeObject, ownersMap map[string]sets.String) string {
	// sort the keys so we always display OWNERS files in same order
	sliceOfKeys := make([]string, len(ownersMap))
	i := 0
	for path := range ownersMap {
		sliceOfKeys[i] = path
		i++
	}
	sort.Strings(sliceOfKeys)

	unapprovedOwners := sets.NewString()
	context := bytes.NewBufferString("")
	for _, path := range sliceOfKeys {
		approverSet := ownersMap[path]
		fullOwnersPath := filepath.Join(path, ownersFileName)
		link := fmt.Sprintf("https://github.com/%s/%s/blob/master/%v", obj.Org(), obj.Project(), fullOwnersPath)

		if approverSet.Len() == 0 {
			context.WriteString(fmt.Sprintf("- **[%s](%s)** \n", fullOwnersPath, link))
			unapprovedOwners.Insert(path)
		} else {
			context.WriteString(fmt.Sprintf("- ~~[%s](%s)~~ [%v]\n", fullOwnersPath, link, strings.Join(approverSet.List(), ",")))
		}
	}
	context.WriteString("\n")
	if unapprovedOwners.Len() > 0 {
		context.WriteString("We suggest the following people:\n")
		context.WriteString("cc ")
		toBeAssigned := h.findPeopleToApprove(unapprovedOwners, *obj.Issue.User.Login)
		for person := range toBeAssigned {
			context.WriteString("@" + person + " ")
		}
	}
	context.WriteString("\n You can indicate your approval by writing `/approve` in a comment")
	context.WriteString("\n You can cancel your approval by writing `/approve cancel` in a comment")
	notif := c.Notification{approvalNotificationName, "Needs approval from an approver in each of these OWNERS Files:\n", context.String()}
	return notif.String()
}

// createApproverSet iterates through the list of comments on a PR
// and identifies all of the people that have said /approve and adds
// them to the approverSet.  The function uses the latest approve or cancel comment
// to determine the Users intention
func createApproverSet(comments []*githubapi.IssueComment) sets.String {
	approverSet := sets.NewString()

	approverMatcher := c.CommandName(approveCommand)
	for _, comment := range c.FilterComments(comments, approverMatcher) {
		cmd := c.ParseCommand(comment)
		if cmd.Arguments == cancel {
			approverSet.Delete(*comment.User.Login)
		} else {
			approverSet.Insert(*comment.User.Login)
		}
	}
	return approverSet
}

// getApprovedOwners finds all the relevant OWNERS files for the PRs and identifies all the people from them
// that have approved the PR.  For all files that have not been approved, it finds the minimum number of owners files
// that cover all of them.  E.g. If /a/b/c.txt and /a/d.txt need approval, it will only indicate that an approval from
// someone in /a/OWNERS is needed
func (h ApprovalHandler) getApprovedOwners(files []*githubapi.CommitFile, approverSet sets.String) map[string]sets.String {
	ownersApprovers := make(map[string]sets.String)
	// TODO: go through the files starting at the top of the tree
	needsApproval := sets.NewString()
	for _, file := range files {
		fileOwners := h.features.Repos.Approvers(*file.Filename)
		ownersFile := h.features.Repos.FindOwnersForPath(*file.Filename)
		hasApproved := fileOwners.Intersection(approverSet)
		if len(hasApproved) != 0 {
			ownersApprovers[ownersFile] = hasApproved
		} else {
			needsApproval.Insert(ownersFile)
		}

	}
	needsApproval = removeSubdirs(needsApproval.List())
	for fn := range needsApproval {
		ownersApprovers[fn] = sets.NewString()
	}
	return ownersApprovers
}

// gets the comments since the obj was last changed.  If we can't figure out when the object was last changed
// return all the comments on the issue
func getCommentsAfterLastModified(obj *github.MungeObject) ([]*githubapi.IssueComment, bool) {
	comments, ok := obj.ListComments()
	if !ok {
		return comments, ok
	}
	lastModified, ok := obj.LastModifiedTime()
	if !ok {
		return comments, ok
	}
	return c.FilterComments(comments, c.CreatedAfter(*lastModified)), true
}
