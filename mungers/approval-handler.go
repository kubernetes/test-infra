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
	"path"
	"strings"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/contrib/mungegithub/mungers/mungerutil"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

const maxDepth = 3

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
// - Initially, we set up approverSet
//   - Go through all comments after latest commit. If any approver said "/approve", add him to approverSet.
// - For each file, we see if any approver of this file is in approverSet.
//   - An approver of a file is defined as:
//     - It's known that each dir has a list of approvers. (This might not hold true. For usability, current situation is enough.)
//     - Approver of a dir is also the approver of child dirs.
//   - We look at top N (default 3) level dir approvers. For example, for file "/a/b/c/d/e", we might search for approver from
//     "/", "/a/", "/a/b/"
// - Iff all files has been approved, the bot will add "approved" label.
func (h *ApprovalHandler) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}
	files, err := obj.ListFiles()
	if err != nil {
		glog.Errorf("failed to list files in this PR: %v", err)
		return
	}

	comments, err := getCommentsAfterLastModified(obj)
	if err != nil {
		glog.Errorf("failed to get comments in this PR: %v", err)
		return
	}

	approverSet := sets.String{}

	// from oldest to latest
	for i := len(comments) - 1; i >= 0; i-- {
		c := comments[i]

		if !mungerutil.IsValidUser(c.User) {
			continue
		}

		fields := strings.Fields(strings.TrimSpace(*c.Body))

		if len(fields) == 1 && strings.ToLower(fields[0]) == "/approve" {
			approverSet.Insert(*c.User.Login)
			continue
		}

		if len(fields) == 2 && strings.ToLower(fields[0]) == "/approve" && strings.ToLower(fields[1]) == "cancel" {
			approverSet.Delete(*c.User.Login)
		}
	}

	for _, file := range files {
		if !h.hasApproval(*file.Filename, approverSet, maxDepth) {
			return
		}
	}
	obj.AddLabel(approvedLabel)
}

func (h *ApprovalHandler) hasApproval(filename string, approverSet sets.String, depth int) bool {
	paths := strings.Split(filename, "/")
	p := ""
	for i := 0; i < len(paths) && i < depth; i++ {
		fileOwners := h.features.Repos.LeafAssignees(p)
		if fileOwners.Len() == 0 {
			glog.Warningf("Couldn't find an owner for path (%s)", p)
			continue
		}

		if h.features.Aliases != nil && h.features.Aliases.IsEnabled {
			fileOwners = h.features.Aliases.Expand(fileOwners)
		}

		for _, owner := range fileOwners.List() {
			if approverSet.Has(owner) {
				return true
			}
		}
		p = path.Join(p, paths[i])
	}
	return false
}

func getCommentsAfterLastModified(obj *github.MungeObject) ([]*githubapi.IssueComment, error) {
	afterLastModified := func(opt *githubapi.IssueListCommentsOptions) *githubapi.IssueListCommentsOptions {
		// Only comments updated at or after this time are returned.
		// One possible case is that reviewer might "/lgtm" first, contributor updated PR, and reviewer updated "/lgtm".
		// This is still valid. We don't recommend user to update it.
		lastModified := *obj.LastModifiedTime()
		opt.Since = lastModified
		return opt
	}
	return obj.ListComments(afterLastModified)
}
